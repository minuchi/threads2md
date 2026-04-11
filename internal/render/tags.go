package render

import (
	"regexp"
	"strings"
)

const maxTags = 20

var hashtagRe = regexp.MustCompile(`#([\p{L}\p{N}_\-]+)`)

// ExtractTags returns the final tag list for a post:
//  1. fixed leading tags ("threads", "@author")
//  2. #hashtags found in body
//  3. extra tags supplied via CLI flags
//
// Tags are lowercased, deduped, and capped at 20 entries.
func ExtractTags(author, body string, extra []string) []string {
	tags := make([]string, 0, 8)
	tags = append(tags, "threads")
	if author != "" {
		tags = append(tags, "@"+author)
	}

	for _, m := range hashtagRe.FindAllStringSubmatch(body, -1) {
		tags = append(tags, strings.ToLower(m[1]))
	}

	for _, t := range extra {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, strings.ToLower(strings.TrimPrefix(t, "#")))
		}
	}

	return dedupeCap(tags, maxTags)
}

func dedupeCap(in []string, max int) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}
