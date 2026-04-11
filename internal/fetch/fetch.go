// Package fetch performs HTTP GETs against Threads with browser-like headers
// and a small retry policy for transient 429/5xx responses.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// Threads serves a JavaScript shell (no metadata) to normal browser UAs
	// and only returns populated Open Graph tags to known social crawlers.
	// Using the Facebook external hit UA is the documented, supported way to
	// retrieve post metadata for link unfurling — which is exactly what
	// threads2md needs. See https://developers.facebook.com/docs/sharing/webmasters/crawler.
	defaultUserAgent      = "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)"
	defaultAcceptHeader   = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	defaultAcceptLanguage = "en-US,en;q=0.9,ko;q=0.8"
	defaultMaxRetries     = 1
	defaultBackoff        = 500 * time.Millisecond
)

// Client wraps an *http.Client with Threads-friendly defaults and retry logic.
type Client struct {
	HTTP       *http.Client
	UserAgent  string
	MaxRetries int
	Backoff    time.Duration
}

// NewClient returns a Client with sensible defaults and a configurable timeout.
func NewClient(timeout time.Duration) *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: timeout,
		},
		UserAgent:  defaultUserAgent,
		MaxRetries: defaultMaxRetries,
		Backoff:    defaultBackoff,
	}
}

// HTTPError carries the status code from a failing upstream response so the
// CLI can map it to the correct exit code.
type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("fetch %s: %s", e.URL, e.Status)
}

// Get fetches the URL with retry on 429/5xx. It returns the final response body
// on success or an error describing the last failure.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	attempts := c.MaxRetries + 1

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.Backoff):
			}
		}

		body, err := c.doOnce(ctx, rawURL)
		if err == nil {
			return body, nil
		}

		if !shouldRetry(err) {
			return nil, err
		}
		lastErr = err
	}

	return nil, lastErr
}

func (c *Client) doOnce(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", defaultAcceptHeader)
	req.Header.Set("Accept-Language", defaultAcceptLanguage)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Drain body to allow connection reuse, but keep it small.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        rawURL,
		}
	}

	return io.ReadAll(resp.Body)
}

func shouldRetry(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500
	}
	return false
}
