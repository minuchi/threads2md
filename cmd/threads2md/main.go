// Command threads2md converts a Threads post URL into an Obsidian-compatible
// Markdown file.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minuchi/threads2md/internal/fetch"
	"github.com/minuchi/threads2md/internal/model"
	"github.com/minuchi/threads2md/internal/parse"
	"github.com/minuchi/threads2md/internal/render"
	nurl "github.com/minuchi/threads2md/internal/url"
)

const (
	exitOK        = 0
	exitInput     = 1
	exitNetwork   = 2
	exitParse     = 3
	exitFileWrite = 4
)

type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("threads2md", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		outFile         string
		saveDir         string
		plain           bool
		jsonOut         bool
		verbose         bool
		timeout         time.Duration
		fixturePath     string
		renderedFixture string
		extraTags       stringSlice
		noReplies       bool
		noHeadless      bool
	)

	fs.StringVar(&outFile, "o", "", "output file path (default stdout)")
	fs.StringVar(&saveDir, "save-dir", "", "save into directory using auto filename (Obsidian vault)")
	fs.BoolVar(&plain, "plain", false, "disable YAML frontmatter (default: enabled)")
	fs.BoolVar(&jsonOut, "json", false, "emit parsed Post as JSON instead of markdown")
	fs.Var(&extraTags, "tag", "additional tag (repeatable)")
	fs.BoolVar(&verbose, "v", false, "verbose logging to stderr")
	fs.DurationVar(&timeout, "timeout", 45*time.Second, "fetch timeout (includes headless render)")
	fs.BoolVar(&noReplies, "no-replies", false, "skip reply extraction (faster, no Chrome needed)")
	fs.BoolVar(&noHeadless, "no-headless", false, "force HTTP-only fetch (implies -no-replies)")
	fs.StringVar(&fixturePath, "fixture", "", "read OG-source HTML from a local file instead of fetching (testing only)")
	fs.StringVar(&renderedFixture, "rendered-fixture", "", "read rendered HTML from a local file for reply extraction (testing only)")

	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: threads2md [flags] <threads-post-url>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitInput
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return exitInput
	}
	rawURL := fs.Arg(0)

	normalized, err := nurl.Normalize(rawURL)
	if err != nil {
		fmt.Fprintf(stderr, "invalid URL: %v\n", err)
		return exitInput
	}

	if verbose {
		fmt.Fprintf(stderr, "canonical: %s\n", normalized.Canonical)
	}

	var html []byte
	if fixturePath != "" {
		html, err = os.ReadFile(fixturePath)
		if err != nil {
			fmt.Fprintf(stderr, "read fixture: %v\n", err)
			return exitInput
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		client := fetch.NewClient(timeout)
		html, err = client.Get(ctx, normalized.Canonical)
		if err != nil {
			fmt.Fprintf(stderr, "fetch: %v\n", err)
			return exitNetwork
		}
	}

	post, err := parse.Parse(html)
	if err != nil {
		fmt.Fprintf(stderr, "parse: %v\n", err)
		return exitParse
	}

	if !noReplies && !noHeadless {
		if err := enrichWithReplies(&post, normalized.Canonical, renderedFixture, timeout, verbose, stderr); err != nil {
			fmt.Fprintf(stderr, "warning: reply extraction failed: %v\n", err)
		}
	}

	// Ensure downstream URL + shortcode are always aligned with the input,
	// even if the upstream document lacked them.
	if post.URL == "" {
		post.URL = normalized.Canonical
	}
	if post.Shortcode == "" {
		post.Shortcode = normalized.Shortcode
	}
	if post.Author == "" {
		post.Author = normalized.Username
	}
	if post.AuthorURL == "" {
		post.AuthorURL = "https://www.threads.com/@" + post.Author
	}
	if len(post.Sources) == 0 {
		post.Sources = []string{post.URL}
	}

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(post); err != nil {
			fmt.Fprintf(stderr, "json encode: %v\n", err)
			return exitFileWrite
		}
		return exitOK
	}

	md, err := render.Markdown(post, render.Options{
		NoFrontmatter: plain,
		ExtraTags:     []string(extraTags),
	})
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return exitParse
	}

	switch {
	case saveDir != "":
		if err := os.MkdirAll(saveDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "mkdir: %v\n", err)
			return exitFileWrite
		}
		fname := render.FileName(post, nil)
		target := filepath.Join(saveDir, fname)
		if err := os.WriteFile(target, []byte(md), 0o644); err != nil {
			fmt.Fprintf(stderr, "write: %v\n", err)
			return exitFileWrite
		}
		fmt.Fprintln(stderr, target)
	case outFile != "":
		if err := os.WriteFile(outFile, []byte(md), 0o644); err != nil {
			fmt.Fprintf(stderr, "write: %v\n", err)
			return exitFileWrite
		}
	default:
		if _, err := io.WriteString(stdout, md); err != nil {
			return exitFileWrite
		}
	}

	return exitOK
}

// enrichWithReplies fetches the fully-rendered Threads page via headless
// Chrome and attaches the parent thread continuation + replies to post.
// When renderedFixture is set, it bypasses Chrome entirely and reads the
// local file — used by tests and the smoke script.
func enrichWithReplies(post *model.Post, canonicalURL, renderedFixture string, timeout time.Duration, verbose bool, stderr io.Writer) error {
	var rendered []byte
	if renderedFixture != "" {
		data, err := os.ReadFile(renderedFixture)
		if err != nil {
			return fmt.Errorf("read rendered fixture: %w", err)
		}
		rendered = data
	} else {
		if verbose {
			fmt.Fprintln(stderr, "headless fetch: launching Chrome for reply extraction")
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		data, err := fetch.GetHeadless(ctx, canonicalURL, fetch.HeadlessOptions{
			Timeout:      timeout,
			WaitSelector: `div[data-pressable-container="true"]`,
			ExtraSettle:  3 * time.Second,
		})
		if err != nil {
			return err
		}
		rendered = data
	}

	before := len(post.Replies)
	if err := parse.AttachRendered(post, rendered); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(stderr, "headless enrichment: %d → %d replies\n", before, len(post.Replies))
	}
	return nil
}
