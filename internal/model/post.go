// Package model contains the shared data types used across threads2md.
package model

import "time"

// Post is the normalized representation of a single Threads post after
// fetching, parsing, and enrichment. It is the unit of work that the render
// layer converts into Obsidian-compatible Markdown.
type Post struct {
	Shortcode   string
	URL         string
	Author      string
	AuthorURL   string
	Title       string
	Description string
	Text        string
	PublishedAt time.Time
	Images      []string
	Tags        []string
	Sources     []string
	Replies     []Reply
	Raw         map[string]any
}

// Reply is a single reply to the main post, captured from the rendered
// Threads post page. Only populated when the headless fetcher runs.
type Reply struct {
	Author      string
	AuthorURL   string
	Shortcode   string
	URL         string
	Text        string
	PublishedAt time.Time
}
