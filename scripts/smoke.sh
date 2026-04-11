#!/usr/bin/env bash
set -euo pipefail

BIN=./bin/threads2md
FIXTURE=./testdata/sample_public_post.html
RENDERED_FIXTURE=./testdata/sample_rendered_with_replies.html
LIVE_FIXTURE=./testdata/sample_threads_live_format.html
URL="https://www.threads.com/@zuck/post/C8tZ1w7pIDn"
LIVE_URL="https://www.threads.com/@metalailab/post/DWr0Jnbkx4m"

fail() { echo "SMOKE FAIL: $1" >&2; exit 1; }

# 1. binary exists
test -x "$BIN" || fail "binary missing ($BIN)"

# 2. help flag works
"$BIN" -h 2>&1 | grep -q "save-dir" || fail "help flag broken"

# 3. invalid URL -> exit 1
set +e
"$BIN" "not-a-url" >/dev/null 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "invalid URL should exit 1, got $rc"

# 4. fixture conversion with default frontmatter (no replies — smoke stays offline)
OUT=$("$BIN" -no-replies -fixture "$FIXTURE" "$URL")
echo "$OUT" | head -1 | grep -qx -- "---" || fail "missing frontmatter opener"
echo "$OUT" | grep -q "^author:" || fail "author field missing"
echo "$OUT" | grep -q "^description:" || fail "description field missing"
echo "$OUT" | grep -q "^sources:" || fail "sources field missing"
echo "$OUT" | grep -q "^tags:" || fail "tags field missing"
echo "$OUT" | grep -q "    - threads" || fail "default 'threads' tag missing"

# 5. -save-dir writes exactly one .md file
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
"$BIN" -no-replies -fixture "$FIXTURE" -save-dir "$TMPDIR" "$URL" >/dev/null 2>&1
COUNT=$(find "$TMPDIR" -type f -name "*.md" | wc -l | tr -d ' ')
[ "$COUNT" = "1" ] || fail "save-dir expected 1 .md file, got $COUNT"

# 6. -plain mode: no frontmatter opener
PLAIN=$("$BIN" -no-replies -fixture "$FIXTURE" -plain "$URL")
if echo "$PLAIN" | head -1 | grep -qx -- "---"; then
  fail "plain mode should not emit frontmatter"
fi

# 7. -json mode emits valid JSON
JSON=$("$BIN" -no-replies -fixture "$FIXTURE" -json "$URL")
if ! echo "$JSON" | python3 -c "import sys, json; json.load(sys.stdin)" >/dev/null 2>&1; then
  fail "json output is not valid JSON"
fi

# 8. rendered-fixture path populates Replies section + reply_count frontmatter
REPLY_OUT=$("$BIN" -fixture "$LIVE_FIXTURE" -rendered-fixture "$RENDERED_FIXTURE" "$LIVE_URL")
echo "$REPLY_OUT" | grep -q "^reply_count: " || fail "reply_count frontmatter missing"
echo "$REPLY_OUT" | grep -q "^## Replies$" || fail "Replies section missing"
echo "$REPLY_OUT" | grep -q "^### \[@" || fail "no reply author link rendered"
if echo "$REPLY_OUT" | grep -q "Translate"; then
  fail "reply body should not contain Translate UI chrome"
fi

echo "smoke tests passed"
