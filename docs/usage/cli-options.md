*[日本語](cli-options_ja.md) | **English***

# CLI options

```
san-db-ox [OPTIONS]
```

## List

| Short | Long | Type | Default | Role |
| :--- | :--- | :--- | :--- | :--- |
| `-c` | `--command` | string (repeatable) | -- | Run the given SQL/dot command and exit. Repeatable; runs in the order given |
| `-m` | `--mode` | string | `list` | Output format: `list` / `column` / `csv` / `json` / `line` |
| `-o` | `--snapshot-as` | string | -- | Default filename for `.snapshot` |
| `-q` | `--quiet` | bool | `false` | Suppress the startup banner and logs |
| `-t` | `--timestamp` | bool | `false` | Append a timestamp to saved filenames |
| `-i` | `--snapshot-interval` | duration | `0` (disabled) | Periodically save a differently-named snapshot (e.g. `5m`, `1h`) |
| `-r` | `--read-only` | bool | `false` | Reject all write operations |
| -- | `--serve-stdio` | bool | `false` | Start in stdio protocol mode |
| `-v` | `--version` | bool | -- | Show version and exit |
| `-h` | `--help` | bool | -- | Show help and exit |

`--serve-stdio` has no short form.

## How the run mode is chosen

| Condition | Mode started |
| :--- | :--- |
| `--serve-stdio` given | stdio protocol |
| `-c` given | batch execution |
| Neither given, stdin is **not** a terminal | batch execution (reads stdin as a SQL script) |
| Neither given, stdin is a terminal | REPL |

Giving both `--serve-stdio` and `-c` is a **usage error** (exit code 2).

When `-c` is given, stdin is never read as a script (so `.import` and the
like can still use stdin).

**Each `-c` value, and the stdin script as a whole, is treated as its own
independent unit of execution.** A missing trailing `;` is implicitly
supplied at the end of that value (or of the whole script) before running it
(the same treatment as the `sqlite3` CLI's `-cmd`). A statement never
continues across multiple `-c` values -- `-c "SELECT 1" -c "UNION SELECT 2"`
is treated as two independent statements, not one statement containing
`UNION`, so the second `-c` is a syntax error on its own (the first `-c`
still returns `1` successfully).

## Exit codes

| Code | Meaning |
| :--- | :--- |
| `0` | Success |
| `1` | A SQL execution error, or a dot command failed |
| `2` | Usage error (a bad flag, a file that couldn't be opened, etc) |

An explicit `.exit CODE` / `.quit CODE` overrides this.

Batch execution **aborts the entire run the moment an error occurs**; no
later statement runs.

## Examples

For interactive (REPL) use, start with no arguments -- just `./san-db-ox`.

Run a single statement and get JSON back:

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c "SELECT * FROM users" -m json
{"columns":["id","name"],"rows":[[1,"alice"]]}
```

Run several statements in order:

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER)" -c "INSERT INTO t VALUES (1)" -c ".snapshot seeded"
Wrote seeded
```

Feed in a script:

<!-- verify -->
```console
$ printf 'CREATE TABLE t (id INTEGER);\nINSERT INTO t VALUES (42);\nSELECT * FROM t;\n' > schema.sql
$ ./san-db-ox < schema.sql
42
```

Use in CI (drop the logs, judge by exit code):

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice'), (2, 'bob')" -c "SELECT count(*) FROM users" -m json 2>/dev/null
{"columns":["count(*)"],"rows":[[2]]}
```

To start read-only, or to be embedded by another program, pass one of these
(neither can be shown as a one-shot example, since both keep the process
running):

```sh
./san-db-ox --read-only
./san-db-ox --serve-stdio
```

## `--timestamp` (`-t`)

Appends `_YYYYMMDDHHMMSS` to the saved filename. The rule:

1. If the base name (the part before the extension) already has a
   `_YYYYMMDDHHMMSS` pattern, strip it first (avoids double-appending).
2. Insert a fresh `_YYYYMMDDHHMMSS` right before the extension.
3. If the extension is omitted, append `.exe` on Windows for executable
   output, or `.sqlite` on every OS for `--sqlite` output.

| Base name | Resulting filename |
| :--- | :--- |
| `san-db-ox` (omitted, based on the running binary's name) | `san-db-ox_20260901120000` |
| `mydb_20260101120000` | `mydb_20260901120000` (the old one is replaced) |
| `mydb.exe` | `mydb_20260901120000.exe` |
| `mydb` (Windows) | `mydb_20260901120000.exe` |
| `mydb` (with `--sqlite`) | `mydb_20260901120000.sqlite` |

The `.snapshot` command itself also accepts `--timestamp`, overriding the
startup setting when given. It has no effect on `.overwrite` (whose
destination is always the running binary itself).

## `--snapshot-interval` (`-i`)

Automatically performs a `.snapshot`-equivalent save at a fixed interval -- a
safety net against losing everything in a long interactive session because
nobody remembered to save.

- Works in REPL and stdio mode. **Has no effect in batch execution** (a
  warning is printed to stderr and it is ignored).
- Only ever saves under a different name; there is no periodic equivalent of
  `.overwrite`.
- Always saves as an executable (never periodically saves a SQLite file).
- The filename follows `--snapshot-as` / `--timestamp`. With `--timestamp`,
  files keep accumulating, so leave it off if you want to keep overwriting
  the same file.
- Combining it with `--read-only` is a usage error (exit code 2).

## `--read-only` (`-r`)

Rejects every write operation. Meant for exposing a read-only environment
when publishing over `socat` or similar.

**Rejected operations:**

- Write SQL (`INSERT` / `UPDATE` / `DELETE` / DDL / writes inside a
  transaction)
- `.snapshot`
- `.snapshot --sqlite`
- `.overwrite`
- `.load`

In the REPL, the startup banner gets a `(read-only)` suffix, and `.help`
drops the rejected operations from its listing. In stdio mode, a `read_only`
error code is returned instead.

This is a process-wide startup mode, not per-connection access control. Omit
it, and any path can run arbitrary SQL, DDL included.

## Separating logs from results

- Results (query results, protocol responses) go to **stdout**
- Logs, warnings, and error messages go to **stderr**

No log file is ever created. For persistence, use redirection or an existing
logging setup (Docker, systemd, etc).

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT * FROM missing_table" 2> san-db-ox.log
$ cat san-db-ox.log
Error: SQL logic error: no such table: missing_table (1)
```
