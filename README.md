# threads2md

Convert [Threads](https://www.threads.com) post URLs into **Obsidian-ready Markdown** with YAML frontmatter, the full thread body, and every reply.

- Single Go binary (`./bin/threads2md`).
- Uses the Facebook-crawler UA against `threads.com` for OG/JSON-LD metadata, then renders the page in headless Chrome via `chromedp` to capture the full body and replies.
- Emits Obsidian Properties: `title`, `author`, `author_url`, `description`, `date`, `sources`, `tags`, `images`, `shortcode`, `reply_count`.
- Merges the author's multi-post thread chain into a single body.
- Captures every reply under a `## Replies` section with per-reply author link, timestamp, blockquoted body, and source link.
- Auto-extracts `#hashtags` from the body and merges them with tags supplied via `-tag`.
- `-save-dir` mode drops files straight into an Obsidian Vault with a deterministic `YYYY-MM-DD-author-shortcode.md` name.

## Install

```bash
git clone https://github.com/minuchi/threads2md.git
cd threads2md
make build
# binary at ./bin/threads2md
```

**Requirements**
- Go 1.26+ (needed by `chromedp`)
- Chrome or Chromium on `$PATH` (only when using the default reply-capture path — not needed with `-no-replies` / `-no-headless`)

## Usage

```bash
# stdout (default: Obsidian frontmatter enabled)
./bin/threads2md "https://www.threads.com/@zuck/post/C8tZ1w7pIDn"

# drop into an Obsidian Vault
./bin/threads2md -save-dir ~/ObsidianVault/Threads -tag ai "https://..."

# plain Markdown without frontmatter
./bin/threads2md -plain "https://..."

# structured JSON dump (debugging / piping)
./bin/threads2md -json "https://..." | jq .
```

### Flags

| Flag | Description |
|---|---|
| `-o path.md` | Write output to a specific file |
| `-save-dir dir/` | Auto-name file and write into `dir/` |
| `-plain` | Disable YAML frontmatter |
| `-tag name` | Append tag (repeatable: `-tag ai -tag reading`) |
| `-json` | Emit the parsed `Post` as JSON instead of Markdown |
| `-no-replies` | Skip reply extraction (faster, no Chrome launch) |
| `-no-headless` | HTTP-only mode; implies `-no-replies` |
| `-v` | Verbose logging to stderr |
| `-timeout 45s` | Fetch timeout (includes headless render) |

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Input / flag error |
| 2 | Network error |
| 3 | Parse error |
| 4 | File write error |

## Example output

```markdown
---
title: Building Llama 3 was one of the most exciting projects I've worked on.
author: zuck
author_url: https://www.threads.com/@zuck
description: Building Llama 3 was one of the most exciting projects I've worked on...
date: "2024-06-12T23:30:00+09:00"
sources:
    - https://www.threads.com/@zuck/post/C8tZ1w7pIDn
tags:
    - threads
    - '@zuck'
    - ai
    - llama-3
images:
    - https://cdn.example.com/cover.jpg
shortcode: C8tZ1w7pIDn
reply_count: 2
---

Building Llama 3 was one of the most exciting projects I've worked on.
Shoutout to the whole team. #AI #llama-3

Here are the benchmarks we cared about...
(author thread continuation merged inline)

![](https://cdn.example.com/cover.jpg)

## Replies

### [@alice](https://www.threads.com/@alice) — 2024-06-12 15:00

> Great post! Thanks for sharing.
>
> [↗ source](https://www.threads.com/@alice/post/R1)


### [@bob](https://www.threads.com/@bob) — 2024-06-12 15:07

> Congrats on the launch!
>
> [↗ source](https://www.threads.com/@bob/post/R2)

---
[View original](https://www.threads.com/@zuck/post/C8tZ1w7pIDn)
```

## Development

```bash
make sanity   # fmt-check → tidy-check → lint → test → build → smoke
make test     # go test ./... -race -cover
make smoke    # post-build smoke test against ./bin/threads2md
```

`make sanity` is the commit gate — it must pass before every commit. See [`CLAUDE.md`](./CLAUDE.md) for contributor rules and [`plan.md`](./plan.md) for the full design.

## How it works

1. **HTTP fetch** with the `facebookexternalhit/1.1` UA → parses OG meta tags and JSON-LD to populate the frontmatter (author, date, images, shortcode, description).
2. **Headless Chrome** (via `chromedp`) navigates to the same URL, waits for the reply DOM to hydrate, and returns the rendered HTML.
3. **DOM walker** in `internal/parse/posts.go` pulls every `div[data-pressable-container="true"]` cell, classifies posts positionally into (main post → thread continuation → replies → "more from author" cutoff), and attaches them to the Markdown output.
4. **Render** emits YAML frontmatter, body, images, `## Replies` section, and the canonical source link.

Use `-no-replies` to skip step 2 (no Chrome needed, OG-truncated body only). Use `-no-headless` to force pure HTTP mode.

## Limitations

- **Public posts only.** Login-required / follower-only content is out of scope.
- **Carousel multi-image and video media aren't unrolled.**
- **Reply ordering** follows the rendered DOM order (Threads' default); we don't re-sort.
- **Threads rate-limits** unauthenticated fetches (HTTP 429). The HTTP client uses the `facebookexternalhit` UA (the supported Meta crawler UA) and retries once on transient failures.

## License

MIT
