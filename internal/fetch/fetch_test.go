package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient() *Client {
	c := NewClient(2 * time.Second)
	c.Backoff = 10 * time.Millisecond
	return c
}

func TestGet_Success(t *testing.T) {
	var gotUA, gotAccept, gotLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		gotLang = r.Header.Get("Accept-Language")
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	body, err := newTestClient().Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "<html>ok</html>" {
		t.Errorf("body: got %q", string(body))
	}
	if gotUA == "" || gotAccept == "" || gotLang == "" {
		t.Errorf("missing browser headers: UA=%q Accept=%q Lang=%q", gotUA, gotAccept, gotLang)
	}
}

func TestGet_FollowsRedirect(t *testing.T) {
	var finalHit bool
	var finalSrv *httptest.Server
	finalSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalHit = true
		_, _ = w.Write([]byte("final"))
	}))
	defer finalSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalSrv.URL, http.StatusMovedPermanently)
	}))
	defer srv.Close()

	body, err := newTestClient().Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalHit {
		t.Fatal("redirect was not followed")
	}
	if string(body) != "final" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestGet_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	body, err := newTestClient().Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", calls.Load())
	}
	if string(body) != "recovered" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestGet_PersistentFailureReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestClient().Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d", httpErr.StatusCode)
	}
}

func TestGet_No4xxRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newTestClient().Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry on 404), got %d", calls.Load())
	}
}

func TestGet_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newTestClient().Get(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}
