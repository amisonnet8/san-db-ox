#!/usr/bin/env bash
# tests/docs.sh -- verifies that command/output examples in docs/ and the
# root READMEs actually match san-db-ox's real behavior (CLAUDE.md's
# documentation-writing rule: "実際にビルドしたバイナリへ流し込んで実測
# 検証したものをそのまま載せる"). Run by `make test`, after tests/e2e.sh.
#
# Marker convention (opt-in, .claude/rules/testing.md's own "実測実行":
# only blocks worth actually running get marked; socat/Docker/download
# instructions are left as prose and never marked):
#
#   <!-- verify -->
#   ```console
#   $ ./san-db-ox -c "SELECT 1"
#   1
#   ```
#
# A line starting with "$ " (immediately following the opening fence, or
# a previous command's expected-output lines) is a command; every other
# line up to the next "$ " or the closing fence is that command's
# expected STDOUT. A command that wants to show stderr writes its own
# "2>&1" -- this script does not merge stderr on its own, so an example
# without "2>&1" that happens to print an incidental warning will fail
# loudly rather than silently accept it.
#
# A command line ending in a heredoc redirection (e.g. "<<'EOF'") pulls
# in every following raw line up to (and including) the line that is
# exactly the delimiter -- those lines belong to the command itself, not
# to its expected output. This lets stdio-protocol examples read
# naturally instead of being squeezed onto one line.
#
# Execution model: one temp directory per markdown FILE, reused across
# all of that file's verified blocks in document order (so a chapter can
# `.snapshot mydb` in one block and start `./mydb` in the next); state
# does not carry across files. Requires a freshly built $ROOT/san-db-ox
# (`make build`, which `make test` runs first).
#
# Normalization: exactly two *concerns* are normalized in both expected
# and actual text before comparing -- nothing else is, so an example's
# output must otherwise match byte-for-byte.
#   1. The version token (a local dev build reports "dev" or a VCS
#      pseudo-version; docs are written showing a release like "v0.1.0";
#      .claude/rules/distribution.md) -- wherever it textually appears:
#      the "SanDBox <token>" banner/-v form, and the stdio hello line's
#      raw `"version":"<token>"` JSON field (spec §7's hello line, a
#      product version distinct from the "inspect" op's `version`, which
#      is the footer format version and is never a JSON string -- so the
#      pattern below cannot accidentally touch it).
#   2. "_YYYYMMDDHHMMSS" -- --timestamp-generated filename suffixes.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXE="$(go env GOEXE 2>/dev/null || true)"
BIN="$ROOT/san-db-ox$EXE"

# Docs' shell examples are POSIX-shell/Unix-path examples; running them
# through Git Bash on Windows risks exactly the MSYS path-translation
# traps .claude/rules/testing.md already documents for tests/e2e.sh
# (translation applies at the CreateProcess boundary, not to paths
# embedded in piped text). Rather than fight that a second time for
# documentation examples, this script simply doesn't run there --
# Windows-specific setup is prose-only in the docs, never marked
# <!-- verify -->.
if [ -n "$EXE" ]; then
  echo "skip: tests/docs.sh only runs on Linux/macOS (see this file's header)" >&2
  exit 0
fi

[ -x "$BIN" ] || { echo "FAIL: $BIN not found or not executable; run 'make build' first" >&2; exit 1; }

FAILED=0

normalize() {
  sed -E \
    -e 's/SanDBox [^ ]+/SanDBox VERSION/' \
    -e 's/"version":"[^"]*"/"version":"VERSION"/' \
    -e 's/_[0-9]{14}/_TIMESTAMP/g'
}

# flush runs the pending command (if any) against $work and compares its
# normalized stdout to the pending expected block. Reads/clears the
# caller's (check_file's) locals via bash's dynamic scoping -- this is
# only ever called while check_file is on the call stack.
flush() {
  if [ "$have_cmd" -eq 1 ]; then
    local actual norm_actual norm_expected
    actual="$(cd "$work" && eval "$cmd")" || true
    norm_actual="$(printf '%s' "$actual" | normalize)"
    norm_expected="$(printf '%s' "$expected" | normalize)"
    if [ "$norm_actual" != "$norm_expected" ]; then
      echo "FAIL: $file:$block_start: \`$cmd\`" >&2
      echo "--- expected ---" >&2
      printf '%s\n' "$norm_expected" >&2
      echo "--- actual ---" >&2
      printf '%s\n' "$norm_actual" >&2
      FAILED=1
    fi
  fi
  cmd=""
  expected=""
  have_cmd=0
}

check_file() {
  local file="$1"
  local work
  work="$(mktemp -d)"
  cp "$BIN" "$work/san-db-ox"
  chmod +x "$work/san-db-ox"

  local in_block=0 verify_next=0
  local cmd="" expected="" have_cmd=0
  local lineno=0 block_start=0
  local line
  # heredoc_re deliberately doesn't require the opening/closing quote
  # characters to match each other (ERE has no backreferences, and
  # backreferences are non-portable across Linux/macOS regex libraries
  # anyway per .claude/rules/testing.md's cross-platform caution) -- it
  # only needs to recognize that *some* heredoc form is present and
  # extract the delimiter word, since the original line (quotes intact)
  # is what actually gets eval'd.
  local heredoc_re='<<-?[[:space:]]*["'"'"']?([A-Za-z_][A-Za-z0-9_]*)["'"'"']?[[:space:]]*$'
  local in_heredoc=0 heredoc_delim=""

  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$((lineno + 1))
    if [ "$in_block" -eq 0 ]; then
      if [ "$line" = "<!-- verify -->" ]; then
        verify_next=1
        continue
      fi
      if [ "$verify_next" -eq 1 ] && [[ "$line" == '```'* ]]; then
        in_block=1
        verify_next=0
        block_start=$lineno
        continue
      fi
      # Any other non-blank line drops a pending marker that wasn't
      # immediately followed by a fence -- adjacency is required, not
      # just "somewhere above".
      if [ -n "$line" ]; then
        verify_next=0
      fi
      continue
    fi
    # Inside a verified block. A heredoc in progress takes priority over
    # everything else -- its body may coincidentally contain something
    # that looks like a fence or a new command.
    if [ "$in_heredoc" -eq 1 ]; then
      cmd="$cmd"$'\n'"$line"
      if [ "$line" = "$heredoc_delim" ]; then
        in_heredoc=0
      fi
      continue
    fi
    if [[ "$line" == '```'* ]]; then
      flush
      in_block=0
      continue
    fi
    if [[ "$line" == '$ '* ]]; then
      flush
      cmd="${line#\$ }"
      have_cmd=1
      if [[ "$cmd" =~ $heredoc_re ]]; then
        in_heredoc=1
        heredoc_delim="${BASH_REMATCH[1]}"
      fi
      continue
    fi
    if [ "$have_cmd" -eq 1 ]; then
      if [ -n "$expected" ]; then
        expected="$expected"$'\n'"$line"
      else
        expected="$line"
      fi
    fi
  done < "$file"
  flush # a file ending mid-block (malformed) still checks what it has

  rm -rf "$work"
}

shopt -s nullglob
files=(docs/spec/*.md docs/usage/*.md docs/examples/*.md docs/tour/*.md "$ROOT"/README*.md)
shopt -u nullglob

cd "$ROOT"
count=0
for f in "${files[@]}"; do
  check_file "$f"
  count=$((count + 1))
done

if [ "$FAILED" -ne 0 ]; then
  exit 1
fi
echo "ok - docs verified ($count file(s) scanned)"
