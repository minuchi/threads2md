package parse

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestParse_JSONLDPath(t *testing.T) {
	post, err := Parse(loadFixture(t, "sample_public_post.html"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Author != "zuck" {
		t.Errorf("author: got %q", post.Author)
	}
	if post.AuthorURL != "https://www.threads.com/@zuck" {
		t.Errorf("author_url: got %q", post.AuthorURL)
	}
	if post.URL != "https://www.threads.com/@zuck/post/C8tZ1w7pIDn" {
		t.Errorf("url: got %q", post.URL)
	}
	if post.Shortcode != "C8tZ1w7pIDn" {
		t.Errorf("shortcode: got %q", post.Shortcode)
	}
	if post.Text == "" {
		t.Fatal("text should not be empty")
	}
	if post.PublishedAt.IsZero() {
		t.Error("published_at should be parsed")
	}
	if len(post.Sources) != 1 || post.Sources[0] != post.URL {
		t.Errorf("sources: got %v", post.Sources)
	}
}

func TestParse_WithImage(t *testing.T) {
	post, err := Parse(loadFixture(t, "sample_with_image.html"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(post.Images) < 2 {
		t.Fatalf("expected >=2 images, got %v", post.Images)
	}
	// JSON-LD images preferred over og:image
	if post.Images[0] != "https://cdn.example.com/serengeti_1.jpg" {
		t.Errorf("first image: got %q", post.Images[0])
	}
}

func TestParse_OGFallback(t *testing.T) {
	post, err := Parse(loadFixture(t, "sample_og_only.html"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Author != "alice" {
		t.Errorf("author: got %q", post.Author)
	}
	if post.URL != "https://www.threads.com/@alice/post/OGONLY42" {
		t.Errorf("url: got %q", post.URL)
	}
	if post.Text == "" {
		t.Error("text should come from og:description")
	}
	if len(post.Images) != 1 || post.Images[0] != "https://cdn.example.com/alice.jpg" {
		t.Errorf("images: got %v", post.Images)
	}
	if post.PublishedAt.IsZero() {
		t.Error("published_at should be parsed from article:published_time")
	}
}

func TestParse_EmptyHTMLReturnsNotFound(t *testing.T) {
	_, err := Parse([]byte(`<!DOCTYPE html><html><head></head><body></body></html>`))
	if err == nil {
		t.Fatal("expected ErrNotFound")
	}
}

func TestParseDate_Variants(t *testing.T) {
	inputs := []string{
		"2024-06-12T14:30:00+0000",
		"2024-06-12T14:30:00Z",
		"2024-06-12T14:30:00+09:00",
	}
	for _, s := range inputs {
		if _, err := parseDate(s); err != nil {
			t.Errorf("parseDate(%q) failed: %v", s, err)
		}
	}
}
