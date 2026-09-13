#!/usr/bin/env bash
# tests/e2e.sh -- end-to-end checks for `make test` (.claude/rules/testing.md).
#
# Exercises the REPL, output modes, and the full dot-command set against
# the binary at $ROOT/san-db-ox[.exe] (built by `make build`, which
# `make test` runs first -- see the Makefile). Batch execution (-c),
# the stdio protocol, and --read-only are still Phase ④ scope (PLAN.md);
# those get their own e2e coverage once that phase lands.
#
# All destructive operations (.overwrite in particular) run against a
# COPY of the built binary in a scratch directory, never the build
# artifact itself (.claude/rules/testing.md).
#
# Grep note: as of Phase 3, non-interactive stdin (piped, as every
# invocation below is) prints no prompt at all (spec §13) -- a result
# line is exactly "<result>", not "SanDBox> <result>". Assertions on a
# single, self-contained value use `grep -qx` (exact line match); ones
# checking for a substring within a larger, multi-line block (schema
# text, .dump output, etc) still use `grep -q`.
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

# native_path converts an MSYS/Unix-style path (e.g. from $WORK, which
# comes from mktemp -d under Git Bash on Windows) into the Windows-native
# form a spawned native process actually needs -- as an argument or
# environment variable, MSYS/Cygwin's CreateProcess-boundary translation
# normally handles this automatically, but that translation never
# applies to bytes flowing through a pipe (confirmed on windows-latest
# CI: an MSYS path piped as ".snapshot <path>" text was misread as
# rooted at the current drive, e.g. "/tmp/x" as "\tmp\x", not "MSYS
# root"). Callers that embed a $WORK-derived path in piped stdin content
# must route it through this first; a path passed as a plain argv/exec
# target does not need it. Identity outside Windows, or if `cygpath`
# (part of Git for Windows) isn't on PATH.
native_path() {
  if [ -n "$EXE" ] && command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1"
  else
    printf '%s' "$1"
  fi
}

[ -x "$BIN" ] || fail "$BIN not found or not executable; run 'make build' first"

# --- REPL: CREATE/INSERT/.tables/.schema/SELECT/error handling (spec §3) ---
cp "$BIN" "$WORK/repl1$EXE"
chmod +x "$WORK/repl1$EXE"
out="$(printf 'CREATE TABLE t(a INTEGER, b TEXT);\nINSERT INTO t VALUES (1, %s);\n.tables\n.schema t\nSELECT * FROM t;\nnot valid sql;\n.exit\n' "'x'" \
  | "$WORK/repl1$EXE" 2>&1)"
echo "$out" | grep -q 'CREATE TABLE t' || fail ".schema t did not show the CREATE statement (got: $out)"
echo "$out" | grep -q '1|x' || fail "SELECT did not return the inserted row (got: $out)"
echo "$out" | grep -q 'Error:' || fail "invalid SQL did not produce an error message (got: $out)"
pass "REPL: CREATE/INSERT/.tables/.schema/SELECT/error handling"

# --- .tables on its own line (checked separately: its output is just the
# bare table name, easy to conflate with prompt/schema noise above) ---
out="$(printf 'CREATE TABLE only_this(a);\n.tables\n.exit\n' | "$WORK/repl1$EXE" 2>&1)"
echo "$out" | grep -qx 'only_this' || fail ".tables did not list table only_this (got: $out)"
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
# .claude/rules/testing.md's e2e pitfalls note). The explicit name is
# given with $EXE already attached: naming.md's extension-completion
# rule (an extension-less name gets ".exe" appended on Windows) is
# covered at the unit level (filename_test.go); giving the name its
# proper extension here keeps this e2e check itself platform-agnostic.
#
# Row 8 is inserted only AFTER the explicit-name snapshot and BEFORE the
# no-name one, so the no-name snapshot's content (must have both 7 and 8)
# is distinguishable from the explicit-name one's (must have only 7) --
# a plain "the file still exists" check would not have caught the
# self-targeting write actually failing (the no-name default targets
# this same running binary's own path, and Windows CI initially found
# that a plain rename onto it is rejected there; engine.Snapshot now
# detects that case and reuses Overwrite's evacuate-then-write path).
cp "$BIN" "$WORK/snapsrc$EXE"
chmod +x "$WORK/snapsrc$EXE"
(cd "$WORK" && printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (7);\n.snapshot explicit-name%s\nINSERT INTO t VALUES (8);\n.snapshot\n.exit\n' "$EXE" | exec "./snapsrc$EXE" >/dev/null)

[ -f "$WORK/explicit-name$EXE" ] || fail ".snapshot <name> did not create a file"
chmod +x "$WORK/explicit-name$EXE"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/explicit-name$EXE" 2>&1)"
echo "$out" | grep -qx '7' || fail "the explicit-name snapshot does not contain the seeded row (got: $out)"
if echo "$out" | grep -qx '8'; then fail "the explicit-name snapshot should not contain row 8, inserted after it ran (got: $out)"; fi

[ -f "$WORK/snapsrc$EXE" ] || fail ".snapshot with no name did not (re)create CWD/<binary-basename>"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/snapsrc$EXE" 2>&1)"
echo "$out" | grep -qx '7' || fail "the self-targeting default-name snapshot lost row 7 (got: $out)"
echo "$out" | grep -qx '8' || fail "the self-targeting default-name snapshot did not persist row 8 -- the write may have silently failed (got: $out)"
pass ".snapshot: explicit name and CWD-relative (self-targeting) default name both produce standalone runnable files with the right data"

# --- .overwrite: persists data, cleans up its sidecar, and repeated
# cycles keep working (spec §4, §11) ---
#
# Sidecar cleanup timing is platform-dependent (spec §11) and this is a
# real difference, not a bug: on Linux the rename-away'd sidecar is
# unlinked immediately, but on Windows it stays locked for as long as
# THIS process (which is still running from that now-renamed-away image)
# is alive, so it can only be removed by the *next* process's OpenSelf()
# at its own startup. Every sidecar check below therefore comes after a
# subsequent launch of the binary, never right after .overwrite itself.
cp "$BIN" "$WORK/ow$EXE"
chmod +x "$WORK/ow$EXE"
before_size=$(wc -c <"$WORK/ow$EXE")
printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (99);\n.overwrite\n' | "$WORK/ow$EXE" >/dev/null
after_size=$(wc -c <"$WORK/ow$EXE")
[ "$after_size" -gt "$before_size" ] || fail ".overwrite did not grow the file as expected ($before_size -> $after_size)"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/ow$EXE" 2>&1)"
echo "$out" | grep -qx '99' || fail "data did not survive .overwrite (got: $out)"
[ ! -e "$WORK/ow$EXE.san-db-ox.old" ] || fail ".san-db-ox.old sidecar was not cleaned up by the next launch's OpenSelf"

# second round: append more data and overwrite again
printf 'INSERT INTO t VALUES (100);\n.overwrite\n' | "$WORK/ow$EXE" >/dev/null
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/ow$EXE" 2>&1)"
echo "$out" | grep -qx '99' || fail "row from round 1 did not survive round 2 (got: $out)"
echo "$out" | grep -qx '100' || fail "row from round 2 is missing (got: $out)"
[ ! -e "$WORK/ow$EXE.san-db-ox.old" ] || fail ".san-db-ox.old sidecar leaked after a second .overwrite cycle"
pass ".overwrite: persists data across two rounds and cleans up its sidecar (by the next launch, at the latest)"

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
grep -qx 'from-proc1' "$WORK/proc1.out" || fail "proc1 did not see its own inserted row"
grep -qx 'from-proc2' "$WORK/proc2.out" || fail "proc2 did not see its own inserted row"
if grep -qx 'from-proc2' "$WORK/proc1.out"; then fail "proc1 unexpectedly saw proc2's data (DBs are not independent)"; fi
if grep -qx 'from-proc1' "$WORK/proc2.out"; then fail "proc2 unexpectedly saw proc1's data (DBs are not independent)"; fi
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
  grep -qx 'shared-data' "$WORK/shared-out-$i" \
    || fail "process $i reading the same binary concurrently got: $(cat "$WORK/shared-out-$i")"
done
pass "multi-process: several processes reading the same binary's footer concurrently all succeed"

# --- concurrent .snapshot to the SAME target path never observes a
# corrupted/partial file (spec §11's atomic temp+rename write).
#
# The target is a BARE relative filename, given to .snapshot from a
# process whose CWD is $WORK (cd'd inside the subshell), not an absolute
# $WORK-prefixed path -- same reason as the explicit-name check above,
# but sharper here: $WORK's value (from mktemp -d under Git Bash) is
# MSYS/Unix-style, and MSYS/Cygwin's automatic path translation only
# rewrites argv when launching a native process, never arbitrary bytes
# flowing through a pipe. Embedding it directly in the piped ".snapshot
# <path>" text confirmed broken on windows-latest CI: Go's Windows path
# handling read the leading "/" as "root of the current drive", not
# "MSYS root", and looked for the file under a nonexistent "\tmp\..."
# rather than the real temp directory. ---
cp "$BIN" "$WORK/racer1$EXE"
cp "$BIN" "$WORK/racer2$EXE"
chmod +x "$WORK/racer1$EXE" "$WORK/racer2$EXE"
TARGET_NAME="raced-snapshot$EXE"
( cd "$WORK" && printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\n.snapshot %s\n.exit\n' "'from-racer1'" "$TARGET_NAME" | exec "./racer1$EXE" >/dev/null ) &
r1=$!
( cd "$WORK" && printf 'CREATE TABLE t(v TEXT);\nINSERT INTO t VALUES (%s);\n.snapshot %s\n.exit\n' "'from-racer2'" "$TARGET_NAME" | exec "./racer2$EXE" >/dev/null ) &
r2=$!
wait "$r1" "$r2"
TARGET="$WORK/$TARGET_NAME"
[ -f "$TARGET" ] || fail "concurrent .snapshot to the same path did not produce a file"
chmod +x "$TARGET"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$TARGET" 2>&1)"
echo "$out" | grep -qxE 'from-racer[12]' || fail "raced snapshot file is corrupt or missing data (got: $out)"
pass ".snapshot: two processes racing to the same target path never leaves a corrupted file"

# --- output modes: -m json / -m csv (spec §3, §7; format.go) ---
out="$(printf 'CREATE TABLE t(a INTEGER, b TEXT);\nINSERT INTO t VALUES (1,%s);\nSELECT a, b FROM t;\n.exit\n' "'x'" \
  | "$BIN" -m json)"
echo "$out" | grep -qx '{"columns":\["a","b"\],"rows":\[\[1,"x"\]\]}' \
  || fail "-m json output did not match the expected shape (got: $out)"

out="$(printf 'CREATE TABLE t(a INTEGER, b TEXT);\nINSERT INTO t VALUES (1,%s);\n.headers on\nSELECT a, b FROM t;\n.exit\n' "'x'" \
  | "$BIN" -m csv | tr -d '\r')"
echo "$out" | grep -qx 'a,b' || fail "-m csv header row missing (got: $out)"
echo "$out" | grep -qx '1,x' || fail "-m csv data row missing (got: $out)"
pass "output modes: -m json and -m csv produce the documented format"

# --- .snapshot --sqlite / .load: round trip through a plain SQLite file,
# and through another SanDBox executable's data (spec §4, §6) ---
cp "$BIN" "$WORK/sqlitesrc$EXE"
chmod +x "$WORK/sqlitesrc$EXE"
(cd "$WORK" && printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (42);\n.snapshot exported --sqlite\n.exit\n' | exec "./sqlitesrc$EXE" >/dev/null)
[ -f "$WORK/exported.sqlite" ] || fail ".snapshot --sqlite did not create exported.sqlite"

cp "$BIN" "$WORK/loadsqlite$EXE"
chmod +x "$WORK/loadsqlite$EXE"
(cd "$WORK" && printf '.load exported.sqlite\n.overwrite\n' | exec "./loadsqlite$EXE" >/dev/null)
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/loadsqlite$EXE" 2>&1)"
echo "$out" | grep -qx '42' || fail ".load from a SQLite file did not bring in the seeded row (got: $out)"

cp "$BIN" "$WORK/sandboxsrc$EXE"
chmod +x "$WORK/sandboxsrc$EXE"
(cd "$WORK" && printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (77);\n.snapshot sandbox-src%s\n.exit\n' "$EXE" | exec "./sandboxsrc$EXE" >/dev/null)
cp "$BIN" "$WORK/loadsandbox$EXE"
chmod +x "$WORK/loadsandbox$EXE"
(cd "$WORK" && printf '.load sandbox-src%s\n.overwrite\n' "$EXE" | exec "./loadsandbox$EXE" >/dev/null)
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/loadsandbox$EXE" 2>&1)"
echo "$out" | grep -qx '77' || fail ".load from a SanDBox executable did not bring in the seeded row (got: $out)"
pass ".snapshot --sqlite / .load: round trip through both a SQLite file and a SanDBox executable"

# --- .dump / .import: dump output replays cleanly, and .import bulk-loads
# a CSV file (spec §3) ---
cp "$BIN" "$WORK/dumper$EXE"
chmod +x "$WORK/dumper$EXE"
dumpfile="$WORK/dump.sql"
printf 'CREATE TABLE t(a INTEGER, b TEXT);\nINSERT INTO t VALUES (1,%s),(2,%s);\n.dump\n.exit\n' "'alice'" "'bob'" \
  | "$WORK/dumper$EXE" >"$dumpfile" 2>&1
grep -q "INSERT INTO \"t\" VALUES(1,'alice')" "$dumpfile" || fail ".dump did not include the expected INSERT (got: $(cat "$dumpfile"))"

cp "$BIN" "$WORK/loadedfromdump$EXE"
chmod +x "$WORK/loadedfromdump$EXE"
out="$( (cat "$dumpfile"; printf 'SELECT * FROM t;\n.exit\n') | "$WORK/loadedfromdump$EXE" 2>&1)"
echo "$out" | grep -qx '1|alice' || fail "replayed .dump output is missing row 1 (got: $out)"
echo "$out" | grep -qx '2|bob' || fail "replayed .dump output is missing row 2 (got: $out)"
pass ".dump: output replays cleanly into a fresh instance"

csvfile="$WORK/people.csv"
printf 'id,name\n1,alice\n2,bob\n' >"$csvfile"
csvarg="$(native_path "$csvfile")"
cp "$BIN" "$WORK/importer$EXE"
chmod +x "$WORK/importer$EXE"
out="$(printf '.import %s people\nSELECT * FROM people;\n.exit\n' "$csvarg" | "$WORK/importer$EXE" 2>&1)"
echo "$out" | grep -q 'Inserted 2 rows' || fail ".import did not report 2 inserted rows (got: $out)"
echo "$out" | grep -qx '1|alice' || fail ".import: row 1 missing (got: $out)"
echo "$out" | grep -qx '2|bob' || fail ".import: row 2 missing (got: $out)"
pass ".import: CSV bulk-load creates the table and inserts its rows"

# --- --snapshot-interval (-i): periodic background saves (spec §12).
# stdin is kept open past the first tick by a trailing `sleep` on the
# producer side of the pipe -- the REPL only exits once stdin actually
# reaches EOF, so this gives the ticker goroutine time to fire at least
# once before the process terminates. ---
cp "$BIN" "$WORK/interval$EXE"
chmod +x "$WORK/interval$EXE"
(
  cd "$WORK" && {
    printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (7);\n'
    sleep 0.5
  } | exec "./interval$EXE" -i 50ms -o "periodic-snap$EXE" -q
) >/dev/null
[ -f "$WORK/periodic-snap$EXE" ] || fail "--snapshot-interval did not produce periodic-snap$EXE"
chmod +x "$WORK/periodic-snap$EXE"
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/periodic-snap$EXE" 2>&1)"
echo "$out" | grep -qx '7' || fail "the periodic snapshot did not contain the seeded row (got: $out)"
pass "--snapshot-interval: periodic background saves work in REPL mode"

# --- Ctrl+C (SIGINT): sqlite3-style interrupt state machine
# (spec §13, interrupt.go). A real PTY is required, since a plain
# pipe never generates a genuine SIGINT the way a terminal's line
# discipline does for its INTR character (0x03) -- this check is skipped
# if `script` (util-linux) isn't available (notably windows-latest). ---
if command -v script >/dev/null 2>&1; then
  cp "$BIN" "$WORK/interrupt$EXE"
  chmod +x "$WORK/interrupt$EXE"
  RAW="$WORK/interrupt.raw"
  OUT="$WORK/interrupt.out"
  : >"$RAW"
  {
    printf 'CREATE TABLE t(a);\n'
    printf 'INSERT INTO t VALUES (1);\n'
    printf 'WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 50000000) SELECT count(*) FROM cnt;\n'
    sleep 1
    printf '\x03'          # Ctrl+C while a long query runs: cancel it, don't exit
    sleep 0.5
    printf 'SELECT 42;\n'  # the session must still work afterward
    sleep 0.3
    printf '\x03'          # Ctrl+C while idle (1st, consecutive count=1): discard input, don't exit
    sleep 0.3
    printf 'SELECT 2;\n'   # proves the process is still alive
    sleep 0.3
    printf '\x03'          # Ctrl+C while idle (2nd consecutive, no line read in between): force-quit
    sleep 0.1
    printf '\x03'
    sleep 0.5
  } | { set +e; timeout 10 script -qec "$WORK/interrupt$EXE -q" "$RAW" >/dev/null 2>&1; echo $? >"$WORK/interrupt.status"; }
  # san-db-ox's own exit code (1 is expected: the two consecutive idle
  # Ctrl+C presses force-quit) is captured to a file by the right-hand
  # side of the pipe above, rather than read from "$?"/PIPESTATUS right
  # after the pipeline -- see .claude/rules/testing.md's `set -e` /
  # pipeline pitfalls for why.
  status="$(cat "$WORK/interrupt.status")"
  tr -d '\r' <"$RAW" >"$OUT"

  grep -q 'context canceled' "$OUT" || fail "Ctrl+C during a long query did not cancel it (got: $(cat "$OUT"))"
  grep -qx '42' "$OUT" || fail "the session did not survive a canceled query (got: $(cat "$OUT"))"
  grep -qx '2' "$OUT" || fail "a single idle Ctrl+C should not have exited the REPL (got: $(cat "$OUT"))"
  [ "$status" -eq 1 ] || fail "two consecutive idle Ctrl+C presses should force-quit with exit code 1 (got $status)"
  pass "REPL: Ctrl+C cancels a running query, and two consecutive idle presses force-quit"
else
  echo "skip - REPL Ctrl+C check ('script' not found)"
fi

# --- go install produces a binary where the footer/.overwrite mechanism
#     still works (.claude/rules/distribution.md) ---
GOBIN="$WORK/gobin"
mkdir -p "$GOBIN"
(cd "$ROOT" && GOBIN="$(native_path "$GOBIN")" go install ./cmd/san-db-ox)
[ -x "$GOBIN/san-db-ox$EXE" ] || fail "go install did not produce a binary"
cp "$GOBIN/san-db-ox$EXE" "$WORK/installed$EXE"
chmod +x "$WORK/installed$EXE"
printf 'CREATE TABLE t(a INTEGER);\nINSERT INTO t VALUES (5);\n.overwrite\n' | "$WORK/installed$EXE" >/dev/null
out="$(printf 'SELECT * FROM t;\n.exit\n' | "$WORK/installed$EXE" 2>&1)"
echo "$out" | grep -qx '5' || fail "a go install-produced binary's .overwrite did not persist data (got: $out)"
pass "go install: footer/.overwrite mechanism works on an installed binary"

echo "e2e: all checks passed"
