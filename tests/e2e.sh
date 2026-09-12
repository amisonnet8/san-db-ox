#!/usr/bin/env bash
# tests/e2e.sh -- end-to-end checks for `make test` (.claude/rules/testing.md).
#
# Exercises the REPL and .snapshot/.overwrite against the binary at
# $ROOT/san-db-ox[.exe] (built by `make build`, which `make test` runs
# first -- see the Makefile). Phase 1 has no batch execution, stdio
# protocol, or --read-only yet (PLAN.md); those get their own e2e
# coverage once the phases that implement them land.
#
# All destructive operations (.overwrite in particular) run against a
# COPY of the built binary in a scratch directory, never the build
# artifact itself (.claude/rules/testing.md).
#
# Grep note: the REPL prints its prompt with no trailing newline, so when
# stdin isn't a terminal (as here), a result line is actually
# "SanDBox> <result>", not "<result>" alone -- there is no isatty-based
# prompt suppression yet (that is batch-mode/Phase ④ territory). Every
# assertion below therefore uses substring `grep -q PATTERN`, never
# `grep -qx` (exact line match), which would spuriously fail.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXE="$(go env GOEXE 2>/dev/null || true)"
BIN="$ROOT/san-db-ox$EXE"
WORK="$(mktemp -d)"

cleanup() {
  local status=$?
  rm -rf "$WORK"
  exit "$status"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok - $*"; }

[ -x "$BIN" ] || fail "$BIN not found or not executable; run 'make build' first"

# --- REPL: CREATE/INSERT/.tables/.schema/SELECT/error handling (spec §3) ---
cp "$BIN" "$WORK/repl1$EXE"
chmod +x "$WORK/repl1$EXE"
out="$(printf 'CREATE TABLE t(a INTEGER, b TEXT);\nINSERT INTO t VALUES (1, %s);\n.tables\n.schema t\nSELECT * FROM t;\nnot valid sql\n.exit\n' "'x'" \
  | "$WORK/repl1$EXE" 2>&1)"
echo "$out" | grep -q 'CREATE TABLE t' || fail ".schema t did not show the CREATE statement (got: $out)"
echo "$out" | grep -q '1|x' || fail "SELECT did not return the inserted row (got: $out)"
echo "$out" | grep -q 'Error:' || fail "invalid SQL did not produce an error message (got: $out)"
pass "REPL: CREATE/INSERT/.tables/.schema/SELECT/error handling"

# --- .tables on its own line (checked separately: its output is just the
# bare table name, easy to conflate with prompt/schema noise above) ---
out="$(printf 'CREATE TABLE only_this(a);\n.tables\n.exit\n' | "$WORK/repl1$EXE" 2>&1)"
echo "$out" | grep -q 'only_this' || fail ".tables did not list table only_this (got: $out)"
pass "REPL: .tables"

# --- -h/-v/bad flag (spec §12 usage-error convention) ---
"$BIN" -h >/dev/null || fail "-h should exit 0"
"$BIN" --help >/dev/null || fail "--help should exit 0"
"$BIN" -v 2>&1 | grep -q 'SanDBox' || fail "-v did not print a version banner"
set +e
"$BIN" --nope >/dev/null 2>&1
code=$?
set -e
[ "$code" -eq 2 ] || fail "an unrecognized flag should exit 2 (usage error), got $code"
pass "CLI: -h/--help, -v, and an unrecognized flag (exit 2)"

# --- .exit CODE / EOF termination (spec §3) ---
set +e
printf '.exit 7\n' | "$BIN" >/dev/null 2>&1
code=$?
set -e
[ "$code" -eq 7 ] || fail ".exit 7 should exit with code 7, got $code"

printf 'SELECT 1;\n' | "$BIN" >/dev/null 2>&1
[ "$?" -eq 0 ] || fail "EOF with no .exit should exit 0"
pass "REPL: .exit CODE and EOF termination"

# --- .snapshot: explicit name and CWD-relative default name (spec §4; the
# default-name behavior is CWD-relative, not next to the binary --
# .claude/rules/testing.md's e2e pitfalls note) ---
cp "$BIN" "$WORK/snapsrc$EXE"
chmod +x "$WORK/snapsrc$EXE"
(cd "$WORK" && printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (7);\n.snapshot explicit-name\n.snapshot\n.exit\n' | exec "./snapsrc$EXE" >/dev/null)
[ -f "$WORK/explicit-name" ] || fail ".snapshot <name> did not create a file"
[ -f "$WORK/snapsrc$EXE" ] || fail ".snapshot with no name did not (re)create CWD/<binary-basename>"
chmod +x "$WORK/explicit-name"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/explicit-name" 2>&1)"
echo "$out" | grep -q '|7\|> 7' || fail "the snapshot file does not contain the seeded row (got: $out)"
pass ".snapshot: explicit name and CWD-relative default name both produce standalone runnable files"

# --- .overwrite: persists data, cleans up its sidecar, and repeated
# cycles keep working (spec §4, §11) ---
cp "$BIN" "$WORK/ow$EXE"
chmod +x "$WORK/ow$EXE"
before_size=$(wc -c <"$WORK/ow$EXE")
printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (99);\n.overwrite\n' | "$WORK/ow$EXE" >/dev/null
after_size=$(wc -c <"$WORK/ow$EXE")
[ "$after_size" -gt "$before_size" ] || fail ".overwrite did not grow the file as expected ($before_size -> $after_size)"
[ ! -e "$WORK/ow$EXE.san-db-ox.old" ] || fail ".san-db-ox.old sidecar was not cleaned up after .overwrite"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/ow$EXE" 2>&1)"
echo "$out" | grep -q '99' || fail "data did not survive .overwrite (got: $out)"

# second round: append more data and overwrite again
printf 'INSERT INTO t VALUES (100);\n.overwrite\n' | "$WORK/ow$EXE" >/dev/null
[ ! -e "$WORK/ow$EXE.san-db-ox.old" ] || fail ".san-db-ox.old sidecar leaked after a second .overwrite"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/ow$EXE" 2>&1)"
echo "$out" | grep -q '99' || fail "row from round 1 did not survive round 2 (got: $out)"
echo "$out" | grep -q '100' || fail "row from round 2 is missing (got: $out)"
pass ".overwrite: persists data across two rounds and cleans up its sidecar each time"

# --- concurrent processes from copies of the same binary have
# independent in-memory databases (spec §8's multi-process precondition,
# .claude/rules/testing.md) ---
cp "$BIN" "$WORK/proc1$EXE"
cp "$BIN" "$WORK/proc2$EXE"
chmod +x "$WORK/proc1$EXE" "$WORK/proc2$EXE"
( printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\nSELECT * FROM t;\n.exit\n' "'from-proc1'" | "$WORK/proc1$EXE" >"$WORK/proc1.out" 2>&1 ) &
p1=$!
( printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\nSELECT * FROM t;\n.exit\n' "'from-proc2'" | "$WORK/proc2$EXE" >"$WORK/proc2.out" 2>&1 ) &
p2=$!
wait "$p1" "$p2"
grep -q 'from-proc1' "$WORK/proc1.out" || fail "proc1 did not see its own inserted row"
grep -q 'from-proc2' "$WORK/proc2.out" || fail "proc2 did not see its own inserted row"
if grep -q 'from-proc2' "$WORK/proc1.out"; then fail "proc1 unexpectedly saw proc2's data (DBs are not independent)"; fi
if grep -q 'from-proc1' "$WORK/proc2.out"; then fail "proc2 unexpectedly saw proc1's data (DBs are not independent)"; fi
pass "multi-process: two processes from copies of the same binary have independent in-memory DBs"

# --- multiple processes reading the SAME binary file concurrently (footer
# read) do not race or corrupt each other's read (.claude/rules/testing.md) ---
cp "$BIN" "$WORK/shared$EXE"
chmod +x "$WORK/shared$EXE"
printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\n.overwrite\n' "'shared-data'" | "$WORK/shared$EXE" >/dev/null
for i in 1 2 3 4; do
  ( printf 'SELECT * FROM t;\n.exit\n' | "$WORK/shared$EXE" >"$WORK/shared-out-$i" 2>&1 ) &
done
wait
for i in 1 2 3 4; do
  grep -q 'shared-data' "$WORK/shared-out-$i" \
    || fail "process $i reading the same binary concurrently got: $(cat "$WORK/shared-out-$i")"
done
pass "multi-process: several processes reading the same binary's footer concurrently all succeed"

# --- concurrent .snapshot to the SAME target path never observes a
# corrupted/partial file (spec §11's atomic temp+rename write) ---
cp "$BIN" "$WORK/racer1$EXE"
cp "$BIN" "$WORK/racer2$EXE"
chmod +x "$WORK/racer1$EXE" "$WORK/racer2$EXE"
TARGET="$WORK/raced-snapshot"
( printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\n.snapshot %s\n.exit\n' "'from-racer1'" "$TARGET" | "$WORK/racer1$EXE" >/dev/null ) &
r1=$!
( printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\n.snapshot %s\n.exit\n' "'from-racer2'" "$TARGET" | "$WORK/racer2$EXE" >/dev/null ) &
r2=$!
wait "$r1" "$r2"
[ -f "$TARGET" ] || fail "concurrent .snapshot to the same path did not produce a file"
chmod +x "$TARGET"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$TARGET" 2>&1)"
echo "$out" | grep -qE 'from-racer[12]' || fail "raced snapshot file is corrupt or missing data (got: $out)"
pass ".snapshot: two processes racing to the same target path never leaves a corrupted file"

# --- go install produces a binary where the footer/.overwrite mechanism
#     still works (.claude/rules/distribution.md) ---
GOBIN="$WORK/gobin"
mkdir -p "$GOBIN"
(cd "$ROOT" && GOBIN="$GOBIN" go install ./cmd/san-db-ox)
[ -x "$GOBIN/san-db-ox$EXE" ] || fail "go install did not produce a binary"
cp "$GOBIN/san-db-ox$EXE" "$WORK/installed$EXE"
chmod +x "$WORK/installed$EXE"
printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (5);\n.overwrite\n' | "$WORK/installed$EXE" >/dev/null
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/installed$EXE" 2>&1)"
echo "$out" | grep -q '5' || fail "a go install-produced binary's .overwrite did not persist data (got: $out)"
pass "go install: footer/.overwrite mechanism works on an installed binary"

echo "e2e: all checks passed"
