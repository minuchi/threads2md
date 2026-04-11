package parse

import (
	"html"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/minuchi/threads2md/internal/model"
)

// ogTags holds the subset of Open Graph / meta tags we care about.
type ogTags struct {
	URL           string
	Title         string
	Description   string
	Image         string
	PublishedTime string
}

func parseOGTags(doc *goquery.Document) ogTags {
	getProp := func(property string) string {
		v, _ := doc.Find(`meta[property="` + property + `"]`).Attr("content")
		return html.UnescapeString(strings.TrimSpace(v))
	}
	getName := func(name string) string {
		v, _ := doc.Find(`meta[name="` + name + `"]`).Attr("content")
		return html.UnescapeString(strings.TrimSpace(v))
	}

	desc := firstNonEmpty(
		getProp("og:description"),
		getName("description"),
		getName("twitter:description"),
	)
	title := firstNonEmpty(
		getProp("og:title"),
		getName("twitter:title"),
	)
	image := firstNonEmpty(
		getProp("og:image"),
		getName("twitter:image"),
	)

	return ogTags{
		URL:           getProp("og:url"),
		Title:         title,
		Description:   desc,
		Image:         image,
		PublishedTime: getProp("article:published_time"),
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// ogToPost converts OG tags into a fresh Post. Called only when JSON-LD is
// unavailable; if JSON-LD is present, mergeOG is used instead.
func ogToPost(o ogTags) model.Post {
	post := model.Post{
		URL:         o.URL,
		Text:        o.Description,
		Description: o.Description,
	}
	post.Author = parseOGAuthor(o.Title)
	if o.Image != "" && !isProfilePicURL(o.Image) {
		post.Images = []string{o.Image}
	}
	if o.PublishedTime != "" {
		if t, err := parseDate(o.PublishedTime); err == nil {
			post.PublishedAt = t
		}
	}
	return post
}

// mergeOG fills in gaps on an existing Post (from JSON-LD) using OG tag values.
func mergeOG(post *model.Post, o ogTags) {
	if post.URL == "" {
		post.URL = o.URL
	}
	if post.Author == "" {
		post.Author = parseOGAuthor(o.Title)
	}
	if post.Description == "" {
		post.Description = o.Description
	}
	if post.Text == "" {
		post.Text = o.Description
	}
	if len(post.Images) == 0 && o.Image != "" && !isProfilePicURL(o.Image) {
		post.Images = []string{o.Image}
	}
	if post.PublishedAt.IsZero() && o.PublishedTime != "" {
		if t, err := parseDate(o.PublishedTime); err == nil {
			post.PublishedAt = t
		}
	}
}

// profilePicPathRe matches Meta/Instagram CDN profile-picture media types.
// Threads post pages serve the author's profile pic as og:image, which is
// misleading when rendered as a post cover — we exclude it here.
var profilePicPathRe = regexp.MustCompile(`/t51\.(?:82787|2885)-19/`)

func isProfilePicURL(u string) bool {
	return profilePicPathRe.MatchString(u)
}

// ogAuthorParenRe matches the newer Threads og:title format:
//
//	"Display Name (@username) on Threads"
var ogAuthorParenRe = regexp.MustCompile(`\(@([^\s)]+)\)`)

// parseOGAuthor extracts the username from either of the two og:title shapes
// Threads emits:
//
//	"Display Name (@username) on Threads"   (current, 2024+)
//	"@username on Threads: body preview"    (legacy)
//
// Returns the empty string when neither format matches.
func parseOGAuthor(title string) string {
	if title == "" {
		return ""
	}
	trimmed := strings.TrimSpace(title)

	if m := ogAuthorParenRe.FindStringSubmatch(trimmed); m != nil {
		return m[1]
	}

	if strings.HasPrefix(trimmed, "@") {
		rest := trimmed[1:]
		for i, r := range rest {
			if r == ' ' || r == '\t' || r == ':' {
				return rest[:i]
			}
		}
		return rest
	}
	return ""
}
