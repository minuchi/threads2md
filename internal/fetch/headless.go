package fetch

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// HeadlessOptions controls the behavior of the chromedp-backed fetcher.
type HeadlessOptions struct {
	// Timeout for the whole navigation + wait cycle.
	Timeout time.Duration
	// UserAgent advertised to the page. Defaults to a desktop Chrome string.
	UserAgent string
	// WaitSelector is a CSS selector that must exist before returning the HTML.
	// When empty, the fetcher only waits for DOMContentLoaded.
	WaitSelector string
	// ExtraSettle is an additional sleep after the selector matches, giving
	// late-hydrating content (e.g. reply lists) time to finish rendering.
	ExtraSettle time.Duration
}

const defaultHeadlessUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36"

// GetHeadless launches headless Chrome via chromedp, navigates to rawURL, waits
// for the page to hydrate, and returns the full rendered HTML. Chrome/Chromium
// must be installed on the host; chromedp auto-discovers it at runtime.
func GetHeadless(ctx context.Context, rawURL string, opts HeadlessOptions) ([]byte, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = defaultHeadlessUA
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx,
		append(
			chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.UserAgent(opts.UserAgent),
		)...,
	)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, opts.Timeout)
	defer cancelTimeout()

	actions := []chromedp.Action{
		chromedp.Navigate(rawURL),
	}
	if opts.WaitSelector != "" {
		actions = append(actions, chromedp.WaitVisible(opts.WaitSelector, chromedp.ByQuery))
	} else {
		actions = append(actions, chromedp.WaitReady("body", chromedp.ByQuery))
	}
	if opts.ExtraSettle > 0 {
		actions = append(actions, chromedp.Sleep(opts.ExtraSettle))
	}

	var html string
	actions = append(actions, chromedp.OuterHTML("html", &html, chromedp.ByQuery))

	if err := chromedp.Run(timeoutCtx, actions...); err != nil {
		return nil, fmt.Errorf("headless fetch: %w", err)
	}
	return []byte(html), nil
}
