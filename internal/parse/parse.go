// Package parse converts a fetched Threads HTML document into a model.Post
// by running a JSON-LD → OG meta tag fallback chain.
package parse

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/minuchi/threads2md/internal/model"
)

// ErrNotFound is returned when neither JSON-LD nor OG meta tags yield
// enough information to build a Post.
var ErrNotFound = errors.New("no parseable Threads post found in document")

// Parse runs the fallback chain over an HTML document and returns a merged Post.
// It prefers JSON-LD SocialMediaPosting content and uses OG tags either as a
// complete fallback (when JSON-LD is missing) or to fill gaps.
func Parse(html []byte) (model.Post, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return model.Post{}, fmt.Errorf("parse html: %w", err)
	}

	og := parseOGTags(doc)
	ld, ldOK := parseJSONLD(doc)

	var post model.Post
	switch {
	case ldOK && hasOG(og):
		post = ld
		mergeOG(&post, og)
	case ldOK:
		post = ld
	case hasOG(og):
		post = ogToPost(og)
	default:
		return model.Post{}, ErrNotFound
	}

	finalize(&post)
	return post, nil
}

func hasOG(o ogTags) bool {
	return o.Description != "" || o.Title != "" || o.URL != ""
}

func finalize(p *model.Post) {
	if p.Author != "" && p.AuthorURL == "" {
		p.AuthorURL = "https://www.threads.com/@" + p.Author
	}
	if len(p.Sources) == 0 && p.URL != "" {
		p.Sources = []string{p.URL}
	}
	p.Images = dedupeNonEmpty(p.Images)
	p.Title = buildTitle(p)
	p.Description = buildDescription(p)
}

func buildTitle(p *model.Post) string {
	if p.Text == "" {
		if p.Author != "" && p.Shortcode != "" {
			return "@" + p.Author + " — " + p.Shortcode
		}
		return ""
	}
	firstLine := strings.TrimSpace(strings.SplitN(p.Text, "\n", 2)[0])
	return truncate(firstLine, 80)
}

func buildDescription(p *model.Post) string {
	if p.Description != "" {
		return p.Description
	}
	if p.Text == "" {
		return ""
	}
	flat := strings.Join(strings.Fields(p.Text), " ")
	return truncate(flat, 160)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimRight(string(runes[:max]), " ") + "…"
}

func dedupeNonEmpty(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it == "" {
			continue
		}
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}
