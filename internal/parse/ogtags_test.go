package parse

import "testing"

func TestParseOGAuthor(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"paren format (current)", "Metal AI (METAL AI) (@metalailab) on Threads", "metalailab"},
		{"paren format plain display name", "Mark Zuckerberg (@zuck) on Threads", "zuck"},
		{"legacy format with body preview", "@zuck on Threads: Building Llama 3...", "zuck"},
		{"legacy format without preview", "@alice on Threads", "alice"},
		{"empty", "", ""},
		{"unrelated title", "Some random page", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOGAuthor(tc.title)
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestParse_LiveFormatOGFallback(t *testing.T) {
	post, err := Parse(loadFixture(t, "sample_threads_live_format.html"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Author != "metalailab" {
		t.Errorf("author: got %q want metalailab", post.Author)
	}
	if post.AuthorURL != "https://www.threads.com/@metalailab" {
		t.Errorf("author_url: got %q", post.AuthorURL)
	}
	if post.URL != "https://www.threads.com/@metalailab/post/DWr0Jnbkx4m" {
		t.Errorf("url: got %q", post.URL)
	}
	if post.Text == "" || post.Description == "" {
		t.Errorf("text/description empty: text=%q desc=%q", post.Text, post.Description)
	}
	if len(post.Sources) != 1 {
		t.Errorf("sources: got %v", post.Sources)
	}
	// og:image is a profile picture on Threads — must be filtered out.
	if len(post.Images) != 0 {
		t.Errorf("profile-pic og:image should be filtered out, got %v", post.Images)
	}
}

func TestIsProfilePicURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://scontent.cdninstagram.com/v/t51.82787-19/656186190_profile.jpg", true},
		{"https://scontent.cdninstagram.com/v/t51.2885-19/abc.jpg", true},
		{"https://scontent.cdninstagram.com/v/t51.29350-15/post_image.jpg", false},
		{"https://cdn.example.com/regular_image.jpg", false},
	}
	for _, tc := range cases {
		if got := isProfilePicURL(tc.url); got != tc.want {
			t.Errorf("isProfilePicURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}
