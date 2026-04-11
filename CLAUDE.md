# CLAUDE.md — Agent Contributor Guide

This file tells Claude Code (and any other AI assistant) how to work in this repository. Read it before editing anything.

## Project at a glance
- **What**: `threads2md` is a Go CLI that converts Threads (Meta) post URLs into Obsidian-compatible Markdown files (YAML frontmatter + body + replies).
- **Where to start**: [`plan.md`](./plan.md) holds the authoritative design, scope, schema, and test strategy.
- **Language**: Go 1.26+ (bumped from 1.22 when `chromedp` was added). Single binary output at `./bin/threads2md`.
- **Runtime**:
  - HTTP path: uses the `facebookexternalhit/1.1` UA against `threads.com` for OG/JSON-LD metadata.
  - Default path: also launches headless Chrome via `chromedp` to capture the full body and replies. **Chrome or Chromium must be installed on the host.** Users can disable this with `-no-replies` / `-no-headless` when Chrome is unavailable or not wanted.
  - Unit tests and smoke tests are fully offline — no test hits the network or launches Chrome.

## Repository layout
```
cmd/threads2md/                    CLI entry point
internal/url/                      URL normalization
internal/fetch/
  fetch.go                         HTTP fetcher with UA spoofing + retry
  headless.go                      chromedp-based headless Chrome fetcher
internal/parse/
  parse.go                         Fallback chain orchestrator
  jsonld.go                        SocialMediaPosting extraction
  ogtags.go                        OG meta fallback
  posts.go                         Rendered-DOM extraction (thread chain + replies)
internal/model/
  post.go                          Post + Reply structs
internal/render/
  markdown.go                      YAML frontmatter + body + replies emitter
  tags.go                          Hashtag extraction and dedupe
testdata/
  sample_public_post.html          JSON-LD fixture
  sample_with_image.html           JSON-LD + multi-image fixture
  sample_og_only.html              OG-only fallback fixture
  sample_threads_live_format.html  Current Threads og:title/meta shape
  sample_rendered_with_replies.html  Full headless-rendered page (≈1 MB)
  expected_*.md                    Render golden files
scripts/smoke.sh                   Post-build binary smoke test
Makefile                           build / test / lint / sanity targets
plan.md                            Full design spec
CLAUDE.md                          Agent contributor rules (this file)
README.md                          User-facing documentation
.gitignore                         Excludes ./bin/, test artifacts, OS junk
```

## Core commands
Always prefer the `Makefile` targets over ad-hoc `go` invocations so local and CI share exactly one entry point.

| Command | Purpose |
|---|---|
| `make build` | Compile `./bin/threads2md` |
| `make test` | `go test ./... -race -cover` |
| `make lint` | `go vet ./...` |
| `make fmt-check` | Fail if any file is unformatted (`gofmt -l .`) |
| `make tidy-check` | Fail if `go mod tidy` would produce a diff |
| `make smoke` | Build then run `scripts/smoke.sh` against the binary |
| `make sanity` | **The commit gate**: runs all of the above in fail-fast order |
| `make clean` | Remove `./bin` |

## Mandatory code-change procedure
Any edit to any file under this repository follows these steps — no exceptions:

1. **Edit** the files.
2. **Run `make sanity`.** This is the commit gate; it chains `fmt-check → tidy-check → lint → test → build → smoke`.
3. **On failure, fix the root cause and rerun `make sanity`** until it passes. Never silence a failing check with `//nolint`, `-skip`, or by deleting assertions.
4. **Only after `make sanity` is green**, move on to the next task or create a commit.
5. **Never commit with a red sanity gate.** Never use `--no-verify` to bypass hooks.

If a check fails and the user hasn't asked you to commit, you still run `make sanity` so the working tree stays green for the next step.

## Design rules
- **Obsidian is the output target.** Frontmatter is **on by default**; the schema (`title`, `author`, `author_url`, `description`, `date`, `sources`, `tags`, `images`, `shortcode`, `reply_count`) is defined in `plan.md` and must stay stable.
- **Two-layer fetch architecture**:
  1. HTTP fetch with `facebookexternalhit/1.1` UA → OG + JSON-LD metadata (author, date, description, cover image, shortcode). This is the frontmatter authority.
  2. Headless Chrome fetch (`chromedp`) → rendered DOM → full body text + thread-chain continuation + replies. Headless output **overwrites** the OG body (OG truncates to 160 chars) and refreshes `Title`/`Description`.
- **Parser fallback chain (HTTP layer)**: JSON-LD (`SocialMediaPosting`) first, OG meta tags second. Both contribute to a single `Post` — partial merges are allowed (e.g. JSON-LD body + OG images).
- **Rendered-DOM extraction rules** (`internal/parse/posts.go`): only top-level `div[data-pressable-container="true"]` cells; direct-child-span text extraction to drop the Translate button; `isUIChrome` filter for view counts, action counters, timestamps, and labels; profile-picture URLs (`t51.82787-19`, `t51.2885-19`) are filtered out of `Images`.
- **Reply classification is positional**: consecutive same-author posts after the main post form the thread chain (merged into body); subsequent non-author posts are replies; the next main-author post terminates reply collection (it marks the "More from @author" recommendation section).
- **YAML serialization always goes through `gopkg.in/yaml.v3`.** Never hand-build YAML strings — escaping is too easy to get wrong.
- **Tags never carry a `#` prefix** in the frontmatter list. Values like `@zuck` are YAML-quoted.
- **`sources` is always a list**, even for a single URL, so the schema doesn't break when quoted posts / attachments get added later.
- **Body pass-through**: preserve the author's line breaks. Only escape literal `---` lines (to `\---`) so they don't prematurely close the frontmatter block.
- **Default timezone is `Asia/Seoul`** unless overridden by `Options.TimeZone`.

## Testing rules
- **Unit tests are offline.** No test may hit `threads.com` by default and **no test may launch Chrome**. Network-backed tests live behind `//go:build integration` and are excluded from CI.
- **Reply extraction is tested via fixtures**: `testdata/sample_rendered_with_replies.html` is a 1 MB saved capture of a real headless render, exercised by `parse.AttachRendered`. Update it only when deliberately refreshing the regression baseline.
- **Golden-file tests** in `internal/render` support `UPDATE_GOLDEN=1` for regeneration. When you intentionally change output format, regenerate the goldens in the same commit and call it out.
- **Smoke tests** run against the compiled binary using two hidden flags:
  - `-fixture path.html` feeds local OG-source HTML to bypass `fetch.Get`.
  - `-rendered-fixture path.html` feeds local rendered HTML to bypass `fetch.GetHeadless`.
  Together they let smoke cover both the HTTP + headless paths without network or Chrome.
- Do **not** delete or disable tests to make a change pass. If a test is genuinely wrong, fix it explicitly and explain why in the commit message.

## Dependency rules
- Go standard library is preferred.
- Approved third-party dependencies:
  - `github.com/PuerkitoBio/goquery` — HTML parsing
  - `gopkg.in/yaml.v3` — YAML serialization
  - `github.com/chromedp/chromedp` — headless Chrome driver used by reply extraction (see "Reply extraction" below)
- Adding any new dependency requires a justification in the PR description and a passing `make tidy-check`.

## Reply extraction
- Replies are captured by launching headless Chrome via `chromedp` against the canonical Threads URL, then parsing the rendered DOM with goquery in `internal/parse/posts.go`.
- This path is **on by default**; disable with `-no-replies` (skip reply pass) or `-no-headless` (skip headless entirely, implies `-no-replies`).
- **Runtime requirement**: Chrome or Chromium must be installed on the host. `chromedp` auto-discovers it; if missing, the CLI surfaces a warning and still emits the main post body.
- Reply extraction is tested offline via a saved `testdata/sample_rendered_with_replies.html` fixture consumed through the hidden `-rendered-fixture` flag; no test hits real Chrome.
- The OG/JSON-LD path remains the metadata authority for author/date/frontmatter; headless only supplies the full body text (replacing OG's 160-char truncation) and the reply list.

## What *not* to do
- Don't hand-write YAML.
- Don't introduce a new package manager, build system, or language runtime.
- Don't expand the CLI surface area beyond the flags listed in `plan.md` without updating `plan.md` in the same change.
- Don't commit binaries (`./bin/` is expected to exist locally but should never be checked in).
- Don't add `// TODO` comments without an owner and a concrete follow-up plan.
- Don't bypass `make sanity` for any reason.

## When you're unsure
- Re-read the relevant section of `plan.md` first.
- If `plan.md` is ambiguous, raise the question instead of guessing — updating `plan.md` is part of the fix.
- If a test fails for a reason you don't understand, investigate the root cause before changing test expectations.
