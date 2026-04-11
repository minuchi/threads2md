// Package url normalizes Threads post URLs into a canonical form and
// extracts the username + shortcode used throughout the rest of the pipeline.
package url

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidURL is returned when the input cannot be parsed as a Threads post URL.
var ErrInvalidURL = errors.New("invalid Threads post URL")

// Normalized captures the canonical form of a Threads post URL.
type Normalized struct {
	Canonical string
	Username  string
	Shortcode string
}

var threadsPostRe = regexp.MustCompile(`^(?:https?://)?(?:www\.)?threads\.(?:net|com)/@([^/\s?#]+)/post/([A-Za-z0-9_-]+)/?(?:[?#].*)?$`)

// Normalize parses and canonicalizes a Threads post URL. It accepts URLs with
// or without scheme, with threads.net or threads.com, and optionally with
// trailing slashes or query strings. Invalid inputs return ErrInvalidURL.
func Normalize(raw string) (Normalized, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Normalized{}, ErrInvalidURL
	}

	m := threadsPostRe.FindStringSubmatch(trimmed)
	if m == nil {
		return Normalized{}, ErrInvalidURL
	}

	username := m[1]
	shortcode := m[2]

	return Normalized{
		Canonical: "https://www.threads.com/@" + username + "/post/" + shortcode,
		Username:  username,
		Shortcode: shortcode,
	}, nil
}
