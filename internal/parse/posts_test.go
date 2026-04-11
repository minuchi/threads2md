package parse

import (
	"strings"
	"testing"

	"github.com/minuchi/threads2md/internal/model"
)

func TestIsUIChrome(t *testing.T) {
	chrome := []string{
		"68K views",
		"1.2M views",
		"144",
		"1,234",
		"04/04/26",
		"6d",
		"2h",
		"Translate",
		"Author",
		"·",
		"Liked by original author",
	}
	for _, s := range chrome {
		if !isUIChrome(s) {
			t.Errorf("expected %q to be filtered as UI chrome", s)
		}
	}

	body := []string{
		"우리가 AI에게 프롬프트를 쓸 때",
		"Building Llama 3 was a joy",
		"150 researchers worked on this",
	}
	for _, s := range body {
		if isUIChrome(s) {
			t.Errorf("expected %q to NOT be filtered", s)
		}
	}
}

func TestBuildThreadAndReplies(t *testing.T) {
	posts := []rawPost{
		{Author: "alice", Shortcode: "A1", Text: "main"},
		{Author: "alice", Shortcode: "A2", Text: "thread continuation"},
		{Author: "bob", Shortcode: "B1", Text: "reply 1"},
		{Author: "carol", Shortcode: "C1", Text: "reply 2"},
		{Author: "alice", Shortcode: "A3", Text: "more from author"},
		{Author: "dave", Shortcode: "D1", Text: "unrelated"},
	}

	thread, replies := buildThreadAndReplies(posts, "A1", "alice")
	if len(thread) != 2 {
		t.Errorf("thread length: got %d want 2", len(thread))
	}
	if len(replies) != 2 {
		t.Errorf("replies length: got %d want 2", len(replies))
	}
	if replies[0].Shortcode != "B1" || replies[1].Shortcode != "C1" {
		t.Errorf("unexpected replies: %+v", replies)
	}
}

func TestAttachRendered_LiveFixture(t *testing.T) {
	data := loadFixture(t, "sample_rendered_with_replies.html")
	post := model.Post{
		Author:    "metalailab",
		Shortcode: "DWr0Jnbkx4m",
	}
	if err := AttachRendered(&post, data); err != nil {
		t.Fatalf("AttachRendered: %v", err)
	}
	if post.Text == "" {
		t.Fatal("main post text should not be empty")
	}
	if !strings.Contains(post.Text, "E-STEER") {
		t.Errorf("main post text missing expected content: %q", firstN(post.Text, 120))
	}
	if strings.Contains(post.Text, "Translate") {
		t.Error("main post text should not contain Translate button chrome")
	}
	if strings.Contains(post.Text, "04/04/26") {
		t.Error("main post text should not contain timestamp chrome")
	}
	if len(post.Replies) == 0 {
		t.Fatal("expected at least one reply extracted")
	}
	// Sanity-check the first reply.
	r := post.Replies[0]
	if r.Author == "" || r.Author == "metalailab" {
		t.Errorf("reply[0].Author = %q — expected a non-author username", r.Author)
	}
	if r.Text == "" {
		t.Error("reply[0].Text should not be empty")
	}
	if strings.Contains(r.Text, "Translate") {
		t.Errorf("reply[0].Text contains chrome: %q", r.Text)
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
