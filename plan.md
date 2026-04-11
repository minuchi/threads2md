# threads2md — Go-based Threads → Obsidian Markdown Converter

## Context
We need a tool that converts Threads (Meta) post URLs into **Markdown files ready to drop into an Obsidian Vault**. The output must include metadata (`author`, `description`, `sources`, `tags`, …) as YAML frontmatter so it integrates naturally with Obsidian Properties, Dataview queries, and tag search.

After reviewing four possible paths (frontend JS / Go server / Chrome extension / official Threads API), we're starting with a **Go CLI**. It lets us convert arbitrary URLs directly from the terminal into a Vault directory, and leaves clear extension paths toward an HTTP server, web UI, or headless fallback later.

### Why start with Go
- Pure frontend JS is blocked by CORS on `threads.com`.
- A Chrome extension only works while the user is viewing a Threads tab — it can't accept pasted URLs.
- The official Threads Graph API requires a Meta app, OAuth, and app-review; URL→media_id resolution is not even officially supported.
- A Go HTTP fetch can pull public posts' OG tags + JSON-LD, which is **sufficient** for a single-post body. Minimal dependencies, single-binary distribution, and clear upgrade paths (`chromedp` fallback, `net/http` server mode).

## v1 Scope (MVP)

### In
- CLI: `threads2md <url>` prints **Obsidian-compatible Markdown** (YAML frontmatter + body) to stdout.
- Required frontmatter fields: `title`, `author`, `author_url`, `description`, `date`, `sources`, `tags`, `images`, `shortcode`.
- Extracts **body, author, published date, canonical URL, and cover image** for a public single post.
- URL normalization: accepts `threads.net`, `threads.com`, with/without `www.`; follows 301 redirects.
- Parser fallback chain: JSON-LD (`SocialMediaPosting`) → OG meta tags.
- Auto-extracts `#hashtags` from body into `tags`; `-tag` flag appends manual tags.
- Default User-Agent spoofs desktop Chrome + `Accept-Language` header to reduce 429s.
- Exponential backoff retry once on 429/5xx.
- Output modes: stdout / `-o file.md` / `-save-dir dir/` (auto-generated filename).
- Additional flags: `-plain` (disable frontmatter), `-json` (structured dump), `-v` (verbose logs).
- Unit tests: offline HTML fixtures in `testdata/` validate parsing + rendering (golden files).

### Out (deferred to v2+)
- Carousel multi-image — only partially exposed in JSON-LD.
- Private / follower-only posts — requires cookie injection.
- Video download / embed URL recovery.
- HTTP server mode (`--serve`), web UI.
- Official Threads Graph API integration.
- Caching (runtime cache, file cache).

### Reply support (added post-v1 in the same file)
- Replies and the main-author thread chain are now captured through a
  headless-Chrome fetch (`internal/fetch/headless.go`) wrapped by
  `parse.AttachRendered`. On by default; `-no-replies` and `-no-headless`
  disable it for faster offline runs.
- Requires Chrome/Chromium installed at runtime. `chromedp` auto-discovers
  the binary; missing Chrome is surfaced as a non-fatal warning and the
  main post body still renders.
- A 1 MB rendered-HTML fixture at `testdata/sample_rendered_with_replies.html`
  exercises the extraction path offline via the hidden `-rendered-fixture`
  CLI flag.

## Output Markdown Format (v1) — Obsidian Vault Compatible

YAML frontmatter is **enabled by default** so files drop straight into an Obsidian Vault. The schema below is chosen for seamless integration with Obsidian Properties, Dataview, and tag search.

### Example file
```markdown
---
title: "Building Llama 3 was one of the most..."
author: zuck
author_url: https://www.threads.com/@zuck
description: "Building Llama 3 was one of the most exciting projects I've worked on..."
date: 2024-06-12T14:30:00+09:00
sources:
  - https://www.threads.com/@zuck/post/C8tZ1w7pIDn
tags:
  - threads
  - "@zuck"
  - ai
  - llama
images:
  - https://scontent-ssn1-1.cdninstagram.com/v/t51....jpg
shortcode: C8tZ1w7pIDn
---

Building Llama 3 was one of the most exciting projects I've worked on.
Line breaks are preserved; the original body is passed through as-is.

![](https://scontent-ssn1-1.cdninstagram.com/v/t51....jpg)

---
[View original](https://www.threads.com/@zuck/post/C8tZ1w7pIDn)
```

### Frontmatter field schema

| Field | Type | Source | Notes |
|---|---|---|---|
| `title` | string | First body line truncated to 80 chars (cut at newline) | Used as Obsidian file title / Dataview label. Falls back to `@author — shortcode` if empty |
| `author` | string | JSON-LD `author.name` or parsed from OG `og:title` | Username without `@` |
| `author_url` | string | JSON-LD `author.url` or constructed as `https://www.threads.com/@{author}` | Profile link |
| `description` | string | JSON-LD `articleBody` or `og:description` truncated to 160 chars with `…` suffix | Surfaced in Obsidian Properties as the summary |
| `date` | string (ISO 8601) | JSON-LD `datePublished` → `time.RFC3339` | Default timezone: `Asia/Seoul` |
| `sources` | list[string] | Canonical Threads URL. Quote posts / link attachments get appended later | **Always a list** so the schema stays stable when more sources are added |
| `tags` | list[string] | Fixed `threads`, `@author`, plus `#hashtag`s extracted from body | No `#` prefix (Obsidian tag list convention); special chars normalized to `-` |
| `images` | list[string] | JSON-LD `image` + OG `og:image`, merged & deduped | Also embedded at the end of body as `![](url)` |
| `shortcode` | string | Extracted from URL | For debugging / re-lookup |

### Frontmatter rules (Obsidian-critical)
- **YAML 1.2 compliance**: values containing `:` or `"` are wrapped in double quotes; inner `"` is escaped `\"`.
- **Tag format**: Obsidian reads `tags:` as a YAML list. **Never add `#` prefix.** Values like `@zuck` are quoted (`"@zuck"`) so YAML parses cleanly.
- **Date**: ISO 8601 with timezone. Dataview recognizes it automatically.
- **`sources` is always a list**: single URL still rendered as `- url` so the schema is stable when more sources appear.
- **`description`**: newlines stripped, truncated to 160 chars; HTML entities (`&amp;`) decoded.
- **Body**: emitted after frontmatter. Any literal `---` in the body is escaped to `\---` to avoid prematurely closing the frontmatter block.

### Tag extraction rules
- Fixed tags: `threads`, `"@{author}"`.
- Regex `#[\p{L}\p{N}_-]+` to catch Hangul/emoji-friendly hashtags (e.g. `#AI`, `#오픈소스`).
- Also append `topic_tag` values if exposed via JSON-LD.
- Lowercase + dedupe; capped at 20 tags.

### Filename convention
- Default filename (used by `-save-dir`): `{date:YYYY-MM-DD}-{author}-{shortcode}.md`
- Example: `2024-06-12-zuck-C8tZ1w7pIDn.md`
- Not needed when writing to stdout.

## Architecture

```
URL input
  │
  ▼
[url.Normalize]   ─── threads.net → threads.com, extract shortcode
  │
  ▼
[fetch.Get]       ─── UA spoofing, 301 follow, 429 backoff
  │
  ▼ HTML []byte
  │
  ▼
[parse.Parse]     ─── fallback chain:
  │                     1. JSON-LD (SocialMediaPosting)
  │                     2. OG meta tags
  │                     3. (v2) __NEXT_DATA__ JSON
  │
  ▼ model.Post
  │
  ▼
[render.Markdown] ─── frontmatter + body + images + source link
  │
  ▼
stdout / file
```

## Project layout

```
threads2md/
├── cmd/
│   └── threads2md/
│       └── main.go          # CLI entry point (flag parsing, pipeline)
├── internal/
│   ├── url/
│   │   ├── normalize.go     # NormalizeURL, ExtractShortcode
│   │   └── normalize_test.go
│   ├── fetch/
│   │   ├── fetch.go         # Client, Get(ctx, url)
│   │   └── fetch_test.go    # httptest.Server mocks
│   ├── parse/
│   │   ├── parse.go         # Parse(html []byte) (Post, error) — fallback chain
│   │   ├── jsonld.go        # SocialMediaPosting extraction
│   │   ├── ogtags.go        # OG meta fallback
│   │   └── parse_test.go    # testdata/*.html validation
│   ├── model/
│   │   └── post.go          # type Post struct
│   └── render/
│       ├── markdown.go      # Render(p Post, opts) string
│       ├── markdown_test.go
│       ├── tags.go          # tag extraction helper
│       └── tags_test.go
├── testdata/
│   ├── sample_public_post.html     # text-only fixture
│   ├── sample_with_image.html      # text + image
│   ├── sample_quoted.html          # quoted post
│   └── expected_*.md               # golden outputs
├── scripts/
│   └── smoke.sh             # post-build binary smoke test
├── go.mod
├── go.sum
├── Makefile                 # build, test, lint, smoke, sanity
├── CLAUDE.md                # agent contributor rules (sanity gate)
└── README.md
```

## Core types

```go
// internal/model/post.go
package model

import "time"

type Post struct {
    Shortcode   string    // C8tZ1w7pIDn
    URL         string    // normalized canonical URL (threads.com)
    Author      string    // username without '@'
    AuthorURL   string    // https://www.threads.com/@username
    Title       string    // first-line truncation for Obsidian title
    Description string    // 160-char summary (newlines stripped, HTML decoded)
    Text        string    // full body (newlines preserved)
    PublishedAt time.Time
    Images      []string  // JSON-LD image + OG image, merged & deduped
    Tags        []string  // ["threads", "@zuck", "ai", ...] — no '#' prefix
    Sources     []string  // origin URL (+ quoted/attached URLs later)
    Raw         map[string]any // debug payload (-json output)
}
```

## Implementation details

### 1. `internal/url/normalize.go`
- Accepted pattern: `(?:https?://)?(?:www\.)?threads\.(?:net|com)/@([^/]+)/post/([A-Za-z0-9_-]+)/?`
- Inputs: full URL, scheme-less URL, share URL (with query string).
- Returns: `{Canonical: "https://www.threads.com/@user/post/SHORT", Username: "user", Shortcode: "SHORT"}`.
- Invalid domain → `ErrInvalidURL`.

### 2. `internal/fetch/fetch.go`
- `Client` struct wraps `http.Client`, default headers, max retries.
- Default headers:
  ```
  User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36
  Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
  Accept-Language: en-US,en;q=0.9,ko;q=0.8
  ```
- `Get(ctx, url)` — follow 301/302 automatically (`http.Client.CheckRedirect`); retry once after a short sleep on 429/5xx.
- Reads full body with `io.ReadAll`. Threads pages are typically 1–2 MB.
- Tests: spin up `httptest.NewServer` to validate UA header and retry logic.

### 3. `internal/parse/parse.go`
- `Parse(html []byte) (model.Post, error)` — tries in order:
  1. `parseJSONLD(doc)` — scans every `<script type="application/ld+json">` looking for `@type: "SocialMediaPosting"` (including inside `@graph` containers).
  2. `parseOGTags(doc)` — reads `og:title` (author), `og:description` (text), `og:image` (cover), `og:url` (canonical), `article:published_time` (date).
  3. Both fail → `ErrNotFound` (v2 will add `__NEXT_DATA__` extraction).
- Partial merge allowed: if JSON-LD lacks images but OG has them, merge.
- HTML queries via `github.com/PuerkitoBio/goquery`.

### 4. `internal/parse/jsonld.go`
- Observed JSON-LD shape on Threads:
  ```json
  {
    "@context": "https://schema.org",
    "@type": "SocialMediaPosting",
    "articleBody": "body text",
    "author": { "@type": "Person", "name": "username", "url": "https://www.threads.com/@username" },
    "datePublished": "2024-06-12T14:30:00+0000",
    "image": ["https://scontent...jpg"],
    "url": "https://www.threads.com/@username/post/SHORT"
  }
  ```
- Also handles array roots and `@graph` wrappers.
- `datePublished` tries multiple layouts (`time.RFC3339`, `+0000`, `Z`).

### 5. `internal/parse/ogtags.go`
- `doc.Find("meta[property='og:description']").Attr("content")` etc.
- `og:title` typically renders as `"@username on Threads: ..."` → first segment is username, rest is preview.
- Body comes from `og:description` only when JSON-LD didn't already supply one.

### 6. `internal/render/markdown.go`
- `type Options struct { NoFrontmatter bool; TimeZone *time.Location; ExtraTags []string }`.
- Default timezone: `Asia/Seoul`.
- **Default behavior: frontmatter on** (Obsidian target). `-plain` flag disables it.
- YAML serialization uses `gopkg.in/yaml.v3` — prevents hand-written escape bugs.
- Body processing:
  - Any literal `---` line is escaped to `\---`.
  - Bare URLs are auto-linkified with a simple regex: `[url](url)`.
  - Hashtags/mentions stay as plain text in the body (they're also promoted to `tags`).
- Trailing section:
  - If images exist: `![](url)` embeds.
  - Horizontal rule, then `[View original](...)`.
- Emission order: `YAML frontmatter` → blank line → `body` → blank line → `images` → `---` → `source link`.
- Helper `FileName(p Post) string` produces `{date}-{author}-{shortcode}.md`.

#### Tag helper (`internal/render/tags.go`)
```go
func ExtractTags(author, body string, extra []string) []string {
    tags := []string{"threads", "@" + author}
    re := regexp.MustCompile(`#([\p{L}\p{N}_-]+)`)
    for _, m := range re.FindAllStringSubmatch(body, -1) {
        tags = append(tags, strings.ToLower(m[1]))
    }
    tags = append(tags, extra...)
    return dedupe(tags, 20)
}
```

### 7. `cmd/threads2md/main.go`
```go
flag.StringVar(&outFile, "o", "", "output file path (default stdout)")
flag.StringVar(&saveDir, "save-dir", "", "save into directory using auto filename (Obsidian vault)")
flag.BoolVar(&plain, "plain", false, "disable YAML frontmatter (default: enabled)")
flag.BoolVar(&jsonOut, "json", false, "emit parsed Post as JSON instead of markdown")
flag.Var(&extraTags, "tag", "additional tag (repeatable, e.g. -tag reading -tag ai)")
flag.BoolVar(&verbose, "v", false, "verbose logging to stderr")
flag.DurationVar(&timeout, "timeout", 15*time.Second, "fetch timeout")
flag.StringVar(&fixturePath, "fixture", "", "(testing) read HTML from local file instead of fetching")
flag.Parse()

url := flag.Arg(0)
// normalize → fetch → parse → render → write(stdout | -o | -save-dir)
```

#### Flag summary
- `-o path.md` — write to a specific file.
- `-save-dir ~/ObsidianVault/Threads/` — auto filename (`{date}-{author}-{shortcode}.md`). Ideal for dropping straight into an Obsidian Vault.
- `-plain` — disable frontmatter (pure Markdown only).
- `-tag ai -tag reading` — append tags to frontmatter (repeatable).
- `-json` — dump `model.Post` as JSON (debugging / piping).
- `-v` — verbose fetch/parse logs to stderr.
- `-timeout 15s` — network timeout.
- `-fixture path.html` — **hidden test flag**: skip network fetch and read HTML from a local file. Used by smoke tests.

#### Exit codes
- 0: success
- 1: input / flag error
- 2: network error (DNS, 5xx after retries)
- 3: parse error (both JSON-LD and OG failed)
- 4: file write error

## Dependencies (go.mod)
```
go 1.22

require (
    github.com/PuerkitoBio/goquery v1.9.2  // HTML parsing
    gopkg.in/yaml.v3 v3.0.1                 // Obsidian frontmatter serialization
    github.com/chromedp/chromedp v0.15.1    // headless Chrome driver for reply extraction
)
```
- Standard library `flag` for CLI; cobra would only be worth it once subcommands appear.
- Standard `testing` + `testing/fstest` are enough.
- YAML is never hand-written — always serialized via `yaml.v3` for safe escaping / unicode / list handling.

## Test strategy

Testing has four layers: **unit → smoke → integration (optional) → sanity (gate)**. Every code change must pass `make sanity` before being committed — see the Sanity Check section below.

### 1. Unit tests (offline, mandatory)
No external network. Run with `go test ./... -race -cover`.

#### `internal/url/normalize_test.go`
- Table-driven: `[]struct { name, input, canonical, author, shortcode string; wantErr bool }`.
- Cases:
  - `https://www.threads.com/@zuck/post/ABC123` → valid
  - `http://threads.net/@zuck/post/ABC123` → scheme/host normalization (net→com, http→https)
  - `threads.com/@zuck/post/ABC123?igshid=xxx` → query stripped
  - `www.threads.com/@zuck/post/ABC123/` → trailing slash accepted
  - `https://www.threads.com/@zuck` (no post segment) → `ErrInvalidURL`
  - `https://instagram.com/p/xyz` → `ErrInvalidURL`
  - Empty string → `ErrInvalidURL`
  - Unicode username (`@한글계정`) → accepted
- Coverage target: 100%.

#### `internal/fetch/fetch_test.go` — `httptest.NewServer` mocks
- 200 response: body returned, UA + Accept-Language headers present.
- 301 redirect: Location followed; final URL body returned.
- 429 transient: backoff then retry; success on 2nd attempt returns body.
- 429 persistent: exceeds retry budget → error.
- 5xx transient/persistent: same.
- `context.Cancel`: immediate error (no retry).
- `http.Client.Timeout` behavior.

#### `internal/parse/parse_test.go`
- Three fixtures in `testdata/`:
  - `sample_public_post.html` — text-only post
  - `sample_with_image.html` — text + cover image
  - `sample_quoted.html` — quoted post (v2 exercise)
- JSON-LD path: exact-match `articleBody`, `author.name`, `datePublished`, `image`.
- OG fallback: strip JSON-LD script, parser must reconstruct Post from OG tags alone.
- Merge scenario: JSON-LD text + OG images.
- Failure: HTML without either → `ErrNotFound`.
- Date parsing variants: `2024-06-12T14:30:00+0000`, `2024-06-12T14:30:00Z`, `2024-06-12T14:30:00+09:00`.

#### `internal/render/markdown_test.go`
- **Golden files**: `testdata/expected_public_post.md`, `expected_with_image.md`, `expected_plain.md`, `expected_with_extra_tags.md`.
- `UPDATE_GOLDEN=1 go test ./internal/render` regenerates them.
- **Structural comparison**: string equality is brittle against trivial whitespace differences, so the test also `yaml.Unmarshal`s the generated frontmatter and compares field-by-field.
- `---` escaping in body verified.
- `-plain` option: no frontmatter verified.

#### `internal/render/tags_test.go`
- Hashtag extraction: `"#AI #오픈소스 #llama-3 hello"` → `["ai", "오픈소스", "llama-3"]`.
- Dedupe + lowercase: input `"#AI #ai"` → single `"ai"`.
- Cap: 25 tags in → 20 out.
- Fixed ordering: `threads`, `"@{author}"` always first.

### 2. Smoke tests (post-build binary validation)
Verifies the compiled binary actually runs. **No network required** — a hidden `-fixture` flag feeds a local HTML file into the pipeline so parse/render/write all run the real code path but fetch is bypassed.

#### Hidden flag in `cmd/threads2md/main.go`
```go
flag.StringVar(&fixturePath, "fixture", "", "read HTML from local file instead of fetching (testing only)")
```
- When `-fixture path.html` is set, `fetch.Get` is skipped in favor of `os.ReadFile`.
- The URL argument is still required (normalization is still exercised).

#### `scripts/smoke.sh`
```bash
#!/usr/bin/env bash
set -euo pipefail

BIN=./bin/threads2md
FIXTURE=./testdata/sample_public_post.html
URL="https://www.threads.com/@zuck/post/C8tZ1w7pIDn"

fail() { echo "SMOKE FAIL: $1" >&2; exit 1; }

# 1. binary exists
test -x "$BIN" || fail "binary missing ($BIN)"

# 2. help flag works
"$BIN" -h 2>&1 | grep -q "save-dir" || fail "help flag broken"

# 3. invalid URL → exit 1
set +e
"$BIN" "not-a-url" 2>/dev/null
rc=$?
set -e
[ $rc -eq 1 ] || fail "invalid URL should exit 1, got $rc"

# 4. fixture conversion → Obsidian frontmatter validation
OUT=$("$BIN" -fixture "$FIXTURE" "$URL")
echo "$OUT" | head -1 | grep -qx "---" || fail "missing frontmatter opener"
echo "$OUT" | grep -q "^author:" || fail "author field missing"
echo "$OUT" | grep -q "^description:" || fail "description field missing"
echo "$OUT" | grep -q "^sources:" || fail "sources field missing"
echo "$OUT" | grep -q "^tags:" || fail "tags field missing"
echo "$OUT" | grep -q "  - threads" || fail "default threads tag missing"

# 5. -save-dir mode: file is created
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
"$BIN" -fixture "$FIXTURE" -save-dir "$TMPDIR" "$URL" >/dev/null
COUNT=$(find "$TMPDIR" -name "*.md" | wc -l | tr -d ' ')
[ "$COUNT" = "1" ] || fail "save-dir expected 1 file, got $COUNT"

# 6. -plain mode: no frontmatter
PLAIN=$("$BIN" -fixture "$FIXTURE" -plain "$URL")
echo "$PLAIN" | head -1 | grep -qx "---" && fail "plain mode should not emit frontmatter"

# 7. -json mode: valid JSON
"$BIN" -fixture "$FIXTURE" -json "$URL" | python3 -c "import sys, json; json.load(sys.stdin)" \
  || fail "json output is not valid JSON"

echo "smoke tests passed"
```

Run via `make smoke` (which depends on `make build`).

### 3. Integration tests (`//go:build integration`, optional)
- Fetches real `threads.com` public posts, only asserts parse succeeded (body content is not stable).
- Excluded from default CI; run locally via `go test -tags=integration ./...`.
- Avoided frequently due to rate-limits.
- Main role: catch upstream Threads HTML changes early.

### 4. Sanity Check — mandatory gate before every commit

**Rule**: any file change must pass `make sanity` before being committed. This rule applies equally when Claude Code edits files — it's codified in `CLAUDE.md`.

#### `Makefile` (core targets)
```makefile
.PHONY: build test lint fmt-check tidy-check smoke sanity clean

BIN := ./bin/threads2md

build:
	go build -o $(BIN) ./cmd/threads2md

test:
	go test ./... -race -cover

lint:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then \
	  echo "unformatted files:"; echo "$$out"; exit 1; fi

tidy-check:
	@cp go.mod go.mod.bak
	@[ -f go.sum ] && cp go.sum go.sum.bak || true
	@go mod tidy
	@if ! diff -q go.mod go.mod.bak >/dev/null; then \
	  mv go.mod.bak go.mod; \
	  [ -f go.sum.bak ] && mv go.sum.bak go.sum || true; \
	  echo "go.mod/go.sum out of date — run 'go mod tidy'"; exit 1; fi
	@rm -f go.mod.bak go.sum.bak

smoke: build
	./scripts/smoke.sh

sanity: fmt-check tidy-check lint test build smoke
	@echo "sanity passed"

clean:
	rm -rf ./bin
```

#### Execution order (fast-fail → slower checks)
| Step | Command | Purpose | Typical time |
|---|---|---|---|
| 1 | `fmt-check` | `gofmt -l .` clean | <1s |
| 2 | `tidy-check` | `go mod tidy` produces no diff | 1–2s |
| 3 | `lint` | `go vet ./...` | 2–5s |
| 4 | `test` | `go test ./... -race -cover` | 5–15s |
| 5 | `build` | CLI compile | 2–5s |
| 6 | `smoke` | binary runtime checks | 2–5s |

Total budget: 10–30 seconds. Stops at the first failure.

#### Commit workflow
```bash
# after changes
make sanity
# only commit on a green gate
git add -p && git commit -m "..."
```

#### Claude Code contributor rule (codified in `CLAUDE.md`)
```markdown
## Code change procedure (mandatory)
1. After editing files, **always** run `make sanity`.
2. On failure, fix the root cause and rerun.
3. Only move on to the next task once `make sanity` is green.
4. Create commits only after a passing sanity run.
```

#### CI integration (post-v1)
`.github/workflows/ci.yml` calls the same entry point:
```yaml
- uses: actions/setup-go@v5
  with: { go-version: '1.22' }
- run: make sanity
```
Local and CI share one gate (`make sanity`) so behavior never diverges.

## End-to-end verification

0. **Sanity check (after every change)**
   ```bash
   make sanity
   ```
   Runs `fmt-check → tidy-check → lint → test → build → smoke` in order. This single target is the commit gate.

1. **Build**
   ```bash
   cd /Users/minu/Projects/github.com/minuchi/threads2md
   go mod init github.com/minuchi/threads2md
   go get github.com/PuerkitoBio/goquery gopkg.in/yaml.v3
   make build
   ```
2. **Unit tests**
   ```bash
   make test   # go test ./... -race -cover
   ```
2.5 **Smoke tests**
   ```bash
   make smoke  # binary execution + fixture pipeline checks
   ```
3. **Real URL conversion (public post, default = Obsidian frontmatter)**
   ```bash
   ./bin/threads2md "https://www.threads.com/@zuck/post/C8tZ1w7pIDn"
   ```
   - Confirm stdout starts with `---` and contains `author`, `description`, `sources`, `tags`.
4. **Drop into an Obsidian Vault**
   ```bash
   ./bin/threads2md -save-dir ~/ObsidianVault/Threads -tag ai "https://..."
   ls ~/ObsidianVault/Threads/2024-06-12-zuck-C8tZ1w7pIDn.md
   ```
   - Open the file in Obsidian and verify the Properties pane auto-shows the fields.
   - Dataview query `LIST FROM #threads` should find the note.
5. **Plain mode (no frontmatter)**
   ```bash
   ./bin/threads2md -plain "https://..."
   ```
6. **JSON debug output**
   ```bash
   ./bin/threads2md -json "https://..." | jq .
   ```
7. **YAML validation**
   ```bash
   ./bin/threads2md "https://..." > out.md
   head -20 out.md | sed -n '/^---$/,/^---$/p' | yq '.'
   ```
   - If `yq` parses cleanly, the frontmatter is valid.
8. **Error cases**
   - Wrong domain → exit 1, stderr says `invalid URL`.
   - Nonexistent shortcode → exit 3, `post not found`.
   - Offline → exit 2.
   - Missing `-save-dir` target → exit 4.

## Risks & mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Threads HTML structure changes | parser failure | JSON-LD → OG fallback chain, integration tests to catch early |
| 429 rate limiting | usage blocked | UA spoofing, backoff retry, v2 per-host token bucket |
| Private / login-only posts | can't convert | v1 emits a clear error; v2 adds cookie injection |
| `og:description` truncation by Meta | body cut off | Prefer JSON-LD `articleBody` first |
| Emoji / unicode normalization | breakage | Go strings are UTF-8 safe, no special handling needed |

## v2 extension paths (for reference)
- `internal/fetch/headless.go` — `chromedp` fallback. Triggered by `--headless` flag or automatic parse failure.
- `cmd/threads2md-server/main.go` — `net/http` server, `POST /convert {url}` returns JSON/Markdown.
- Minimal static web UI (Vercel deploy) — URL input box calling the server API.
- Chrome extension partner — extension POSTs rendered DOM HTML to the Go server, reusing the parser.

## Affected / created files
- **New files** (all under `/Users/minu/Projects/github.com/minuchi/threads2md/`):
  - `go.mod`, `go.sum`
  - `cmd/threads2md/main.go`
  - `internal/url/normalize.go`, `normalize_test.go`
  - `internal/fetch/fetch.go`, `fetch_test.go`
  - `internal/parse/parse.go`, `jsonld.go`, `ogtags.go`, `parse_test.go`
  - `internal/model/post.go`
  - `internal/render/markdown.go`, `markdown_test.go`, `tags.go`, `tags_test.go`
  - `testdata/sample_public_post.html`, `sample_with_image.html`, `sample_quoted.html`
  - `testdata/expected_public_post.md`, `expected_with_image.md`, `expected_plain.md`, `expected_with_extra_tags.md`
  - `Makefile` — `build / test / lint / fmt-check / tidy-check / smoke / sanity` targets
  - `scripts/smoke.sh` — post-build smoke test script
  - `CLAUDE.md` — agent contributor rules, including the `make sanity` requirement
  - `README.md`

- **Modified**: none (greenfield).
