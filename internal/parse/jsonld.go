package parse

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/minuchi/threads2md/internal/model"
)

// parseJSONLD walks every <script type="application/ld+json"> block and
// returns the first SocialMediaPosting it can construct. The second return
// value indicates whether a posting was found.
func parseJSONLD(doc *goquery.Document) (model.Post, bool) {
	var found model.Post
	var ok bool

	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return true
		}

		var generic any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			return true
		}

		for _, node := range walkLD(generic) {
			m, isMap := node.(map[string]any)
			if !isMap {
				continue
			}
			if !isSocialMediaPosting(m) {
				continue
			}
			post, built := jsonLDToPost(m)
			if built {
				found = post
				ok = true
				return false
			}
		}
		return true
	})

	return found, ok
}

// walkLD flattens the possible JSON-LD container shapes (object, array,
// @graph wrapper) into a slice of candidate nodes to inspect.
func walkLD(v any) []any {
	switch x := v.(type) {
	case []any:
		var out []any
		for _, item := range x {
			out = append(out, walkLD(item)...)
		}
		return out
	case map[string]any:
		out := []any{x}
		if g, hasGraph := x["@graph"]; hasGraph {
			out = append(out, walkLD(g)...)
		}
		return out
	default:
		return nil
	}
}

func isSocialMediaPosting(m map[string]any) bool {
	t, exists := m["@type"]
	if !exists {
		return false
	}
	switch v := t.(type) {
	case string:
		return strings.EqualFold(v, "SocialMediaPosting")
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.EqualFold(s, "SocialMediaPosting") {
				return true
			}
		}
	}
	return false
}

func jsonLDToPost(m map[string]any) (model.Post, bool) {
	post := model.Post{
		Raw: m,
	}

	post.Text = strings.TrimSpace(firstNonEmptyString(m, "articleBody", "text"))
	post.URL = firstNonEmptyString(m, "url", "mainEntityOfPage")
	post.Shortcode = firstNonEmptyString(m, "identifier")

	if author, ok := m["author"].(map[string]any); ok {
		post.Author = firstNonEmptyString(author, "name", "alternateName")
		post.AuthorURL = firstNonEmptyString(author, "url")
	} else if authorStr, ok := m["author"].(string); ok {
		post.Author = authorStr
	}

	if pub := firstNonEmptyString(m, "datePublished", "dateCreated"); pub != "" {
		if t, err := parseDate(pub); err == nil {
			post.PublishedAt = t
		}
	}

	post.Images = extractImages(m["image"])

	if post.Text == "" && post.URL == "" {
		return model.Post{}, false
	}
	return post, true
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case string:
				if s := strings.TrimSpace(x); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func extractImages(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			switch xi := item.(type) {
			case string:
				if xi != "" {
					out = append(out, xi)
				}
			case map[string]any:
				if url := firstNonEmptyString(xi, "url", "contentUrl"); url != "" {
					out = append(out, url)
				}
			}
		}
		return out
	case map[string]any:
		if url := firstNonEmptyString(x, "url", "contentUrl"); url != "" {
			return []string{url}
		}
	}
	return nil
}

var dateLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseDate(s string) (time.Time, error) {
	var lastErr error
	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}
