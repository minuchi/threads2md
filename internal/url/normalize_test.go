package url

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCanon string
		wantUser  string
		wantShort string
		wantErr   bool
	}{
		{
			name:      "canonical https threads.com",
			input:     "https://www.threads.com/@zuck/post/ABC123",
			wantCanon: "https://www.threads.com/@zuck/post/ABC123",
			wantUser:  "zuck",
			wantShort: "ABC123",
		},
		{
			name:      "http threads.net normalized to https threads.com",
			input:     "http://threads.net/@zuck/post/ABC123",
			wantCanon: "https://www.threads.com/@zuck/post/ABC123",
			wantUser:  "zuck",
			wantShort: "ABC123",
		},
		{
			name:      "query string stripped",
			input:     "threads.com/@zuck/post/ABC123?igshid=xxx",
			wantCanon: "https://www.threads.com/@zuck/post/ABC123",
			wantUser:  "zuck",
			wantShort: "ABC123",
		},
		{
			name:      "trailing slash accepted",
			input:     "www.threads.com/@zuck/post/ABC123/",
			wantCanon: "https://www.threads.com/@zuck/post/ABC123",
			wantUser:  "zuck",
			wantShort: "ABC123",
		},
		{
			name:      "unicode username",
			input:     "https://www.threads.com/@한글계정/post/XYZ_789",
			wantCanon: "https://www.threads.com/@한글계정/post/XYZ_789",
			wantUser:  "한글계정",
			wantShort: "XYZ_789",
		},
		{
			name:    "missing post segment",
			input:   "https://www.threads.com/@zuck",
			wantErr: true,
		},
		{
			name:    "wrong host",
			input:   "https://instagram.com/p/xyz",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Canonical != tc.wantCanon {
				t.Errorf("canonical: got %q want %q", got.Canonical, tc.wantCanon)
			}
			if got.Username != tc.wantUser {
				t.Errorf("username: got %q want %q", got.Username, tc.wantUser)
			}
			if got.Shortcode != tc.wantShort {
				t.Errorf("shortcode: got %q want %q", got.Shortcode, tc.wantShort)
			}
		})
	}
}
