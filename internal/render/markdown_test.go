package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/minuchi/threads2md/internal/model"
)

func utc(t *testing.T, layout, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, s)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return tm
}

func samplePost(t *testing.T) model.Post {
	published := utc(t, time.RFC3339, "2024-06-12T14:30:00Z")
	text := "Building Llama 3 was one of the most exciting projects I've worked on.\nShoutout to the team. #AI #llama-3"
	return model.Post{
		Shortcode:   "C8tZ1w7pIDn",
		URL:         "https://www.threads.com/@zuck/post/C8tZ1w7pIDn",
		Author:      "zuck",
		AuthorURL:   "https://www.threads.com/@zuck",
		Title:       "Building Llama 3 was one of the most exciting projects I've worked on.",
		Description: "Building Llama 3 was one of the most exciting projects I've worked on. Shoutout to the team.",
		Text:        text,
		PublishedAt: published,
		Images:      []string{"https://cdn.example.com/cover.jpg"},
		Sources:     []string{"https://www.threads.com/@zuck/post/C8tZ1w7pIDn"},
	}
}

func TestMarkdown_FrontmatterStructure(t *testing.T) {
	out, err := Markdown(samplePost(t), Options{TimeZone: time.UTC})
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if !strings.HasPrefix(out, "---\n") {
		t.Fatal("expected frontmatter opener")
	}
	// Extract frontmatter between the first two --- lines.
	parts := strings.SplitN(out, "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("frontmatter parse failure: %q", out)
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		t.Fatalf("yaml unmarshal: %v\n%s", err, parts[1])
	}
	if fm.Author != "zuck" {
		t.Errorf("author: %q", fm.Author)
	}
	if fm.AuthorURL != "https://www.threads.com/@zuck" {
		t.Errorf("author_url: %q", fm.AuthorURL)
	}
	if fm.Description == "" {
		t.Error("description missing")
	}
	if len(fm.Sources) != 1 {
		t.Errorf("sources: %v", fm.Sources)
	}
	if len(fm.Images) != 1 {
		t.Errorf("images: %v", fm.Images)
	}
	if !containsAll(fm.Tags, []string{"threads", "@zuck", "ai", "llama-3"}) {
		t.Errorf("tags missing expected entries: %v", fm.Tags)
	}
	if fm.Date != "2024-06-12T14:30:00Z" {
		t.Errorf("date: %q", fm.Date)
	}
}

func TestMarkdown_PlainMode(t *testing.T) {
	out, err := Markdown(samplePost(t), Options{NoFrontmatter: true, TimeZone: time.UTC})
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if strings.HasPrefix(out, "---\n") {
		t.Error("plain mode should omit frontmatter")
	}
	if !strings.Contains(out, "Building Llama 3") {
		t.Error("body missing")
	}
}

func TestMarkdown_ExtraTags(t *testing.T) {
	out, _ := Markdown(samplePost(t), Options{TimeZone: time.UTC, ExtraTags: []string{"reading"}})
	if !strings.Contains(out, "- reading") {
		t.Errorf("extra tag missing in output:\n%s", out)
	}
}

func TestMarkdown_BodyDividerEscaped(t *testing.T) {
	p := samplePost(t)
	p.Text = "before\n---\nafter"
	out, _ := Markdown(p, Options{TimeZone: time.UTC})
	if !strings.Contains(out, `\---`) {
		t.Errorf("---) divider not escaped:\n%s", out)
	}
	// Ensure frontmatter is still delimited correctly: exactly two "---\n" lines in the frontmatter section.
	parts := strings.SplitN(out, "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("frontmatter not properly delimited: %q", out)
	}
}

func TestMarkdown_TrailingSourceLink(t *testing.T) {
	out, _ := Markdown(samplePost(t), Options{TimeZone: time.UTC})
	if !strings.Contains(out, "[View original](https://www.threads.com/@zuck/post/C8tZ1w7pIDn)") {
		t.Errorf("source link missing:\n%s", out)
	}
}

func TestFileName(t *testing.T) {
	p := samplePost(t)
	got := FileName(p, time.UTC)
	want := "2024-06-12-zuck-C8tZ1w7pIDn.md"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMarkdown_RepliesSection(t *testing.T) {
	p := samplePost(t)
	p.Replies = []model.Reply{
		{
			Author:      "alice",
			AuthorURL:   "https://www.threads.com/@alice",
			Shortcode:   "R1",
			URL:         "https://www.threads.com/@alice/post/R1",
			Text:        "Great post!\nThanks for sharing.",
			PublishedAt: utc(t, time.RFC3339, "2024-06-12T15:00:00Z"),
		},
		{
			Author:    "bob",
			AuthorURL: "https://www.threads.com/@bob",
			Shortcode: "R2",
			URL:       "https://www.threads.com/@bob/post/R2",
			Text:      "Congrats!",
		},
	}
	out, err := Markdown(p, Options{TimeZone: time.UTC})
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if !strings.Contains(out, "## Replies") {
		t.Error("missing ## Replies section")
	}
	if !strings.Contains(out, "[@alice](https://www.threads.com/@alice)") {
		t.Error("reply author link missing")
	}
	if !strings.Contains(out, "> Great post!") {
		t.Error("reply body not blockquoted")
	}
	if !strings.Contains(out, "reply_count: 2") {
		t.Error("reply_count not in frontmatter")
	}
}

func TestMarkdown_Goldens(t *testing.T) {
	cases := []struct {
		name     string
		opts     Options
		filename string
	}{
		{"public_post", Options{TimeZone: time.UTC}, "expected_public_post.md"},
		{"plain", Options{NoFrontmatter: true, TimeZone: time.UTC}, "expected_plain.md"},
		{"with_extra_tags", Options{TimeZone: time.UTC, ExtraTags: []string{"reading"}}, "expected_with_extra_tags.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Markdown(samplePost(t), tc.opts)
			if err != nil {
				t.Fatalf("Markdown: %v", err)
			}
			path := filepath.Join("..", "..", "testdata", tc.filename)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to create)", path, err)
			}
			if string(want) != out {
				t.Errorf("golden mismatch for %s\n---got---\n%s\n---want---\n%s", tc.filename, out, string(want))
			}
		})
	}
}

func containsAll(haystack, needles []string) bool {
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[h] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[n]; !ok {
			return false
		}
	}
	return true
}
