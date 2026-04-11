package render

import (
	"reflect"
	"testing"
)

func TestExtractTags_Fixed(t *testing.T) {
	got := ExtractTags("zuck", "hello world", nil)
	want := []string{"threads", "@zuck"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractTags_HashtagsAndDedupe(t *testing.T) {
	got := ExtractTags("zuck", "hi #AI #ai #llama-3 #오픈소스", nil)
	want := []string{"threads", "@zuck", "ai", "llama-3", "오픈소스"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractTags_Extra(t *testing.T) {
	got := ExtractTags("alice", "body", []string{"reading", "#AI"})
	want := []string{"threads", "@alice", "reading", "ai"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractTags_Cap(t *testing.T) {
	body := ""
	extra := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		extra = append(extra, "t"+string(rune('a'+i)))
	}
	got := ExtractTags("zuck", body, extra)
	if len(got) != maxTags {
		t.Errorf("expected cap at %d, got %d", maxTags, len(got))
	}
	if got[0] != "threads" || got[1] != "@zuck" {
		t.Errorf("fixed leading tags missing: %v", got[:2])
	}
}
