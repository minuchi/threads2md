package parse

import (
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	"github.com/minuchi/threads2md/internal/model"
)

// rawPost is an intermediate struct capturing everything we can extract from a
// single Threads post cell in the rendered DOM.
type rawPost struct {
	Author      string
	Shortcode   string
	Permalink   string
	Text        string
	PublishedAt time.Time
}

var postHrefRe = regexp.MustCompile(`^/@([^/]+)/post/([A-Za-z0-9_-]+)/?$`)

// extractPosts walks every `div[data-pressable-container="true"]` that is not
// nested inside another such container and returns the posts in document
// order. Used only in headless mode; crawler-UA HTML contains none of these.
func extractPosts(doc *goquery.Document) []rawPost {
	var posts []rawPost

	doc.Find(`div[data-pressable-container="true"]`).Each(func(_ int, s *goquery.Selection) {
		// Skip nested containers — we only want top-level post cells.
		if s.ParentsFiltered(`div[data-pressable-container="true"]`).Length() > 0 {
			return
		}

		rp := rawPost{}

		// Permalink (`<a href="/@user/post/SHORT">`).
		s.Find(`a[href*="/post/"]`).EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, _ := a.Attr("href")
			m := postHrefRe.FindStringSubmatch(href)
			if m == nil {
				return true
			}
			rp.Author = m[1]
			rp.Shortcode = m[2]
			rp.Permalink = "https://www.threads.com" + href
			// Grab the timestamp from the <time> child, if any.
			if t := a.Find(`time[datetime]`).First(); t.Length() > 0 {
				if dt, ok := t.Attr("datetime"); ok {
					if parsed, err := time.Parse(time.RFC3339, dt); err == nil {
						rp.PublishedAt = parsed
					}
				}
			}
			return false
		})

		// Body text extraction from Threads' DOM is finicky — the outer
		// span[dir="auto"] that wraps a paragraph also contains sibling
		// Translate buttons, and other `dir="auto"` spans hold usernames,
		// timestamps, view counts, and action-bar counters we do not want.
		//
		// Rules:
		//   1. Ignore spans that live inside a link (usernames).
		//   2. Ignore spans nested inside another `dir="auto"` span.
		//   3. Ignore spans containing <a> or <time> descendants (timestamp
		//      wrappers, mentions inside the header).
		//   4. Ignore spans whose nearest [role="button"] ancestor is a UI
		//      action (Like / Comment / Repost / Share / Translate).
		//   5. Collect text only from direct-child <span> descendants, so
		//      sibling <div role="button">Translate</div> content is dropped.
		//   6. Apply a final UI-chrome filter (view counts, counters, date
		//      shorthands, "Author", "·", etc).
		var paragraphs []string
		s.Find(`span[dir="auto"]`).Each(func(_ int, sp *goquery.Selection) {
			if sp.Closest("a").Length() > 0 {
				return
			}
			if sp.ParentsFiltered(`span[dir="auto"]`).Length() > 0 {
				return
			}
			if sp.Find("a, time").Length() > 0 {
				return
			}
			if sp.ParentsFiltered(`[role="button"]`).Length() > 0 {
				return
			}
			txt := directChildSpanText(sp)
			if txt == "" {
				return
			}
			if isUIChrome(txt) {
				return
			}
			paragraphs = append(paragraphs, txt)
		})
		rp.Text = strings.Join(paragraphs, "\n\n")

		// Drop cells that had no extractable content at all.
		if rp.Shortcode == "" && rp.Text == "" {
			return
		}
		posts = append(posts, rp)
	})

	return posts
}

// buildThreadAndReplies splits the ordered post list into (a) the main-thread
// chain (consecutive posts by the main author starting at the main post) and
// (b) the immediate replies. Everything from the next main-author post onward
// is treated as unrelated recommendation content and discarded.
func buildThreadAndReplies(posts []rawPost, mainShortcode, mainAuthor string) (thread []rawPost, replies []rawPost) {
	if len(posts) == 0 {
		return nil, nil
	}

	// Locate the main post by shortcode.
	start := -1
	for i, p := range posts {
		if p.Shortcode == mainShortcode {
			start = i
			break
		}
	}
	if start == -1 {
		// Fallback: assume the first post is the main one.
		start = 0
	}

	author := posts[start].Author
	if mainAuthor != "" {
		author = mainAuthor
	}

	// Main thread chain: consecutive posts by the same author starting at start.
	i := start
	for i < len(posts) && posts[i].Author == author {
		thread = append(thread, posts[i])
		i++
	}

	// Replies: next run of non-author posts. Stop when the main author reappears
	// (that marks the "More from @author" section).
	for i < len(posts) {
		if posts[i].Author == author {
			break
		}
		replies = append(replies, posts[i])
		i++
	}
	return thread, replies
}

// directChildSpanText collects text from the direct text-node and <span>
// children of sp, ignoring other element children (like <div role="button">).
// This is the key trick that strips trailing Translate buttons from paragraph
// spans without disturbing text that contains inline spans (e.g. mentions).
func directChildSpanText(sel *goquery.Selection) string {
	if sel.Length() == 0 {
		return ""
	}
	node := sel.Get(0)
	var b strings.Builder
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			b.WriteString(c.Data)
		case c.Type == html.ElementNode && c.Data == "span":
			b.WriteString(nodeInnerText(c))
		}
		// Any other element child (notably <div role="button">) is ignored.
	}
	return strings.TrimSpace(strings.ReplaceAll(b.String(), "\u00a0", " "))
}

// nodeInnerText returns the concatenated text content of a node and all its
// descendants, in document order.
func nodeInnerText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// uiChromePatterns matches strings that look like UI chrome rather than body
// text — view counts, action counters, compact relative dates, and labels.
var uiChromePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\d+(?:[.,]\d+)?[KMB]?\s*views?$`),
	regexp.MustCompile(`^\d{1,3}(?:,\d{3})*$`),      // "144" or "1,234"
	regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{2,4}$`), // "04/04/26"
	regexp.MustCompile(`^\d+\s*[smhdwMy]$`),         // "6d", "2h"
	regexp.MustCompile(`^(?:Author|Translate|Follow|Following)$`),
	regexp.MustCompile(`^[·•∙]$`),
}

// uiChromeExact contains literal strings that should always be skipped.
var uiChromeExact = map[string]struct{}{
	"Liked by original author": {},
	"Edited":                   {},
	"Pin":                      {},
	"Pinned":                   {},
}

func isUIChrome(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return true
	}
	if _, ok := uiChromeExact[trimmed]; ok {
		return true
	}
	for _, re := range uiChromePatterns {
		if re.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// AttachRendered merges reply / thread-chain data extracted from headless HTML
// into an existing Post. It is a no-op when the rendered document yields no
// posts (e.g. Chrome rendered an empty shell / auth wall).
func AttachRendered(post *model.Post, renderedHTML []byte) error {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(renderedHTML)))
	if err != nil {
		return err
	}
	raws := extractPosts(doc)
	if len(raws) == 0 {
		return nil
	}

	thread, replies := buildThreadAndReplies(raws, post.Shortcode, post.Author)

	// Merge thread-chain continuation into the main post body. The rendered
	// version is authoritative — headless Chrome gives us the full body
	// without OG's 160-char truncation — so we overwrite post.Text when the
	// headless extractor succeeded in finding the main post, then rebuild
	// the derived title/description to reflect the full body.
	if len(thread) > 0 {
		head := thread[0]
		textChanged := false
		if head.Text != "" {
			post.Text = head.Text
			textChanged = true
		}
		if post.PublishedAt.IsZero() && !head.PublishedAt.IsZero() {
			post.PublishedAt = head.PublishedAt
		}
		for _, cont := range thread[1:] {
			if cont.Text == "" {
				continue
			}
			if post.Text == "" {
				post.Text = cont.Text
			} else {
				post.Text = post.Text + "\n\n" + cont.Text
			}
			textChanged = true
		}
		if textChanged {
			post.Title = buildTitle(post)
			// Rebuild description from the full body (bypass early-return
			// on existing description).
			flat := strings.Join(strings.Fields(post.Text), " ")
			post.Description = truncate(flat, 160)
		}
	}

	for _, r := range replies {
		if r.Text == "" && r.Shortcode == "" {
			continue
		}
		post.Replies = append(post.Replies, model.Reply{
			Author:      r.Author,
			AuthorURL:   "https://www.threads.com/@" + r.Author,
			Shortcode:   r.Shortcode,
			URL:         r.Permalink,
			Text:        r.Text,
			PublishedAt: r.PublishedAt,
		})
	}

	return nil
}
