*[日本語](stdio-protocol_ja.md)*

# stdio protocol

```sh
san-db-ox --serve-stdio
```

A mode for using SanDBox from another program. **It launches as a child
process and speaks over standard input/output.** Since no network is
involved, anything that can spawn a subprocess and handle pipes can connect
without a dedicated library.

- No banner, no prompt
- **stdout carries protocol responses only.** Logs and diagnostics go to stderr
- **A session's lifetime is the process's lifetime.** Transactions are held
  for the life of the process
- Exits when stdin reaches EOF (nothing is saved)

## Message format

**Line-delimited JSON (JSON Lines). One line, one message.**

Request:

```json
{"op":"query","sql":"SELECT id, name FROM users WHERE id = ?","params":[1]}
```

Response:

```json
{"ok":true,"columns":["id","name"],"rows":[[1,"alice"]]}
```

- Including `id` in a request gets it echoed back in the matching response
  (optional). **Responses always come back in the same order as their
  requests**, so they can be matched up even without `id`.
- Sending malformed JSON just gets one error response back; the connection
  is not closed.

## The hello line, right after connecting

Right after connecting, one line that answers no request is printed first.

```json
{"protocol":1,"version":"v0.1.0","product":"SanDBox"}
```

**Clients must always skip (or otherwise handle) this line.** `protocol` is
the protocol's own version number, a separate lineage from the saved-format
version.

## Op list

| op | Parameters | Response |
| :--- | :--- | :--- |
| `query` | `sql`, `params` | `columns`, `rows` |
| `exec` | `sql`, `params` | `rows_affected`, `last_insert_id` |
| `snapshot` | `filename` (optional), `sqlite` (bool, optional), `timestamp` (bool, optional) | `path` |
| `load` | `path` | -- |
| `inspect` | -- | see below |
| `tables` | -- | array of table names |
| `schema` | `table` (optional) | CREATE statements |
| `dump` | `pattern` (optional) | `sql` (the full dump text) |
| `overwrite` | -- | **returns one response line, then the process exits** |
| `close` | -- | the process exits normally after responding |

There is no dedicated op for `BEGIN` / `COMMIT` / `ROLLBACK` -- send them
through `exec` as-is. DDL runs through `exec` too.

**There is no op corresponding to `.import`.** It reads a CSV file from an
arbitrary path on the server side, the same concern that keeps
`inspect(path)` from being exposed as an op (a way to probe the server's
filesystem when exposed externally), so it is deliberately unsupported.

`query` returns every row of the result set in a single response. **There is
no cursor mechanism for fetching rows incrementally.**

## `inspect`

Reports this process's own state -- the same information the REPL's startup
banner shows, made available to stdio clients.

```json
{"op":"inspect"}
```

```json
{"ok":true,"has_data":true,"version":1,"data_length":1048576,"source":"mydb_20260901120000","read_only":false}
```

| Field | Contents |
| :--- | :--- |
| `has_data` | Whether data was embedded at startup |
| `version` | The saved-format version (`null` if `has_data` is `false`) |
| `data_length` | The length of the loaded data, in bytes (same condition) |
| `source` | The filename of the executable that was started |
| `read_only` | Whether started with `--read-only` |

**There is no facility for inspecting an arbitrary path.** Whether a file
can be `.load`ed is discoverable by running `load` and looking at the error.

## JSON representation of values

| SQLite value | JSON representation | Example |
| :--- | :--- | :--- |
| NULL | `null` | `null` |
| INTEGER | number | `42` |
| REAL | number (always with a decimal point) | `88.0` |
| TEXT | string | `"alice"` |
| BLOB | **a single-element array** (Base64) | `["iVBORw0KGgo="]` |

The rule fits in one line: **if the value is a JSON array, it's a BLOB;
otherwise the JSON type is the SQLite type.**

```json
{"ok":true,"columns":["id","name","score","avatar","note"],"rows":[[1,"alice",88.5,["iVBORw0KGgo="],null]]}
```

- **`params` accepts the same representation.** A value read out can be
  written straight back in.
  ```json
  {"op":"exec","sql":"INSERT INTO profiles VALUES (?, ?)","params":[1,["iVBORw0KGgo="]]}
  ```
- An array with a length other than 1 is a protocol violation.
- This representation is shared with `.mode json` (REPL and batch execution).

**Known limitation:** JSON numbers can only represent integers exactly up to
2^53 - 1. SQLite's 64-bit INTEGER range isn't fully representable, and in a
language with no native 64-bit integer type (JavaScript, for instance),
**values silently drift with no error**. A driver in a language that does
handle 64-bit integers (Go, Rust, Java, C#, ...) should avoid decoding JSON
numbers into a double-precision float, and instead read the numeric token as
a string before parsing it as an integer (Go's `json.Number`, Rust's
`serde_json` arbitrary-precision feature). That preserves the value even
though the server emits it as a plain number.

## Error responses

```json
{"ok":false,"error":{"code":"sqlite_error","message":"no such table: users"}}
```

| `code` | Meaning |
| :--- | :--- |
| `sqlite_error` | A `query`/`exec` SQL execution error |
| `bad_request` | Malformed JSON, an unknown field, an invalid value representation, a missing required parameter |
| `io_error` | A file I/O or path-related failure in `snapshot`/`load`/`overwrite`/`dump` |
| `unsupported_op` | An unknown op |
| `read_only` | An operation rejected because of `--read-only` |

**The process does not exit on an error.** You can keep sending further
requests (unlike batch execution, which aborts immediately).

## Notes for client implementers

Things that will break if you don't follow them.

- **Flush after writing every line.** Pipes are buffered; skip this and both
  sides end up waiting forever for the other's input.
- **Never leave stderr unread.** If a child process's stderr stays piped but
  unread, the process blocks once the pipe buffer fills up with logs. Route
  it to `/dev/null` if you don't want it.
- **EOF means the connection is gone.** If the child process dies
  abnormally, reading a line returns EOF. Detect it and reap the exit code
  (skip this and a zombie process is left behind).
- **Exit by closing stdin.** The child process exits on its own on either
  the `close` op or stdin being closed. A good `close()` implementation
  closes stdin, waits a bit, then sends SIGTERM if it's still alive.

## Trying it from a shell

Feed it in one direction (no interaction possible):

```sh
san-db-ox --serve-stdio <<'EOF'
{"op":"exec","sql":"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"}
{"op":"exec","sql":"INSERT INTO users VALUES (1, 'alice')"}
{"op":"query","sql":"SELECT id, name FROM users"}
EOF
```

(read `san-db-ox` as `./san-db-ox` if it isn't on PATH; here's a run of it:)

<!-- verify -->
```console
$ ./san-db-ox --serve-stdio <<'EOF'
{"op":"exec","sql":"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"}
{"op":"exec","sql":"INSERT INTO users VALUES (1, 'alice')"}
{"op":"query","sql":"SELECT id, name FROM users"}
EOF
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"last_insert_id":0,"ok":true,"rows_affected":0}
{"last_insert_id":1,"ok":true,"rows_affected":1}
{"columns":["id","name"],"ok":true,"rows":[[1,"alice"]]}
```

Talk to it in both directions (a bash coprocess):

```sh
coproc DB { san-db-ox --serve-stdio; }
read -r hello <&"${DB[0]}"

echo '{"op":"exec","sql":"CREATE TABLE t (id INTEGER)"}' >&"${DB[1]}"
read -r line <&"${DB[0]}"; echo "$line"

echo '{"op":"query","sql":"SELECT * FROM t"}' >&"${DB[1]}"
read -r line <&"${DB[0]}"; echo "$line"

exec {DB[1]}>&-
wait "$DB_PID"
```

Putting `jq` in the middle of an interactive pipeline will hang on
buffering. Use `jq --unbuffered` or `stdbuf -oL`.

## Drivers for each language

Client drivers for C, Python, Go, and the like are provided by a separate
project, `san-db-ox-clients`. This project provides only the protocol
specification above.
