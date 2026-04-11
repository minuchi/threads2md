// Package render serializes a model.Post to Obsidian-compatible Markdown
// with a YAML frontmatter block.
package render

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/minuchi/threads2md/internal/model"
)

// Options controls frontmatter emission and auxiliary behavior.
type Options struct {
	NoFrontmatter bool
	TimeZone      *time.Location
	ExtraTags     []string
}

// DefaultTimeZone is used when Options.TimeZone is nil.
var DefaultTimeZone = mustLoadLocation("Asia/Seoul")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// frontmatter is the YAML-mapped form of the fields we emit. Field order is
// stabilized by the explicit yaml tags + a yaml.Node wrapper below.
type frontmatter struct {
	Title       string   `yaml:"title,omitempty"`
	Author      string   `yaml:"author,omitempty"`
	AuthorURL   string   `yaml:"author_url,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Date        string   `yaml:"date,omitempty"`
	Sources     []string `yaml:"sources,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Images      []string `yaml:"images,omitempty"`
	Shortcode   string   `yaml:"shortcode,omitempty"`
	ReplyCount  int      `yaml:"reply_count,omitempty"`
}

// Markdown renders the given Post as an Obsidian-compatible Markdown document.
func Markdown(p model.Post, opts Options) (string, error) {
	tz := opts.TimeZone
	if tz == nil {
		tz = DefaultTimeZone
	}

	var buf bytes.Buffer

	if !opts.NoFrontmatter {
		fm := frontmatter{
			Title:       p.Title,
			Author:      p.Author,
			AuthorURL:   p.AuthorURL,
			Description: p.Description,
			Sources:     p.Sources,
			Images:      p.Images,
			Shortcode:   p.Shortcode,
			ReplyCount:  len(p.Replies),
		}
		if !p.PublishedAt.IsZero() {
			fm.Date = p.PublishedAt.In(tz).Format(time.RFC3339)
		}
		fm.Tags = ExtractTags(p.Author, p.Text, opts.ExtraTags)

		yamlBytes, err := yaml.Marshal(&fm)
		if err != nil {
			return "", fmt.Errorf("marshal frontmatter: %w", err)
		}
		buf.WriteString("---\n")
		buf.Write(yamlBytes)
		buf.WriteString("---\n\n")
	}

	body := escapeFrontmatterDividers(p.Text)
	body = autoLinkify(body)
	buf.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		buf.WriteString("\n")
	}

	if len(p.Images) > 0 {
		buf.WriteString("\n")
		for _, img := range p.Images {
			fmt.Fprintf(&buf, "![](%s)\n", img)
		}
	}

	if len(p.Replies) > 0 {
		buf.WriteString("\n## Replies\n")
		for i, reply := range p.Replies {
			writeReply(&buf, reply, tz)
			if i < len(p.Replies)-1 {
				buf.WriteString("\n")
			}
		}
	}

	if p.URL != "" {
		buf.WriteString("\n---\n")
		fmt.Fprintf(&buf, "[View original](%s)\n", p.URL)
	}

	return buf.String(), nil
}

func writeReply(buf *bytes.Buffer, r model.Reply, tz *time.Location) {
	buf.WriteString("\n### ")
	if r.AuthorURL != "" {
		fmt.Fprintf(buf, "[@%s](%s)", r.Author, r.AuthorURL)
	} else {
		fmt.Fprintf(buf, "@%s", r.Author)
	}
	if !r.PublishedAt.IsZero() {
		fmt.Fprintf(buf, " — %s", r.PublishedAt.In(tz).Format("2006-01-02 15:04"))
	}
	buf.WriteString("\n\n")

	body := escapeFrontmatterDividers(r.Text)
	body = autoLinkify(body)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for _, line := range lines {
		buf.WriteString("> ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	if r.URL != "" {
		fmt.Fprintf(buf, ">\n> [↗ source](%s)\n", r.URL)
	}
}

// FileName returns the auto-generated filename used by -save-dir mode.
func FileName(p model.Post, tz *time.Location) string {
	if tz == nil {
		tz = DefaultTimeZone
	}
	datePart := "undated"
	if !p.PublishedAt.IsZero() {
		datePart = p.PublishedAt.In(tz).Format("2006-01-02")
	}
	author := p.Author
	if author == "" {
		author = "unknown"
	}
	short := p.Shortcode
	if short == "" {
		short = "post"
	}
	return fmt.Sprintf("%s-%s-%s.md", datePart, author, short)
}

var dividerRe = regexp.MustCompile(`(?m)^---\s*$`)

func escapeFrontmatterDividers(text string) string {
	return dividerRe.ReplaceAllString(text, `\---`)
}

var bareURLRe = regexp.MustCompile(`(?i)(^|[\s(])(https?://[^\s)]+)`)

func autoLinkify(text string) string {
	return bareURLRe.ReplaceAllString(text, "$1[$2]($2)")
}
