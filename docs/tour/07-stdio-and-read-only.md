# 7. stdio and read-only mode

*[Guided tour index](README.md) / Previous: [6. Using it from scripts](06-scripting.md) / [日本語](07-stdio-and-read-only_ja.md)*

The last chapter. It covers using SanDBox **from another program**, and
using it **with writes locked out**.

## `--serve-stdio` -- embed it in another program

Started with `--serve-stdio`, it switches to a mode that exchanges
line-delimited JSON (JSON Lines) over standard input/output. No network is
used at all.

Once connected, the first thing to arrive is a **hello line** (it answers no
request).

<!-- verify -->
```console
$ ./san-db-ox --serve-stdio <<'EOF'
{"op":"exec","sql":"CREATE TABLE t (id INTEGER)"}
{"op":"exec","sql":"INSERT INTO t VALUES (1)"}
{"op":"query","sql":"SELECT * FROM t"}
EOF
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"last_insert_id":0,"ok":true,"rows_affected":0}
{"last_insert_id":1,"ok":true,"rows_affected":1}
{"columns":["id"],"ok":true,"rows":[[1]]}
```

`BEGIN`/`COMMIT`/`ROLLBACK` and DDL are also just sent through `exec`, with
no dedicated op. The value representation (NULL/INTEGER/REAL/TEXT/BLOB) uses
the same rules as `.mode json` ([Chapter 4](04-output-modes.md)).

This process's own state (whether it has data, whether it's read-only, etc)
can be checked with the `inspect` op.

<!-- verify -->
```console
$ echo '{"op":"inspect"}' | ./san-db-ox --serve-stdio
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"data_length":null,"has_data":false,"ok":true,"read_only":false,"source":"san-db-ox","version":null}
```

For concrete examples of using it from another program (a minimal Go
client, embedding with Docker, etc), see
[Example: Embedding via stdio](../examples/stdio-integration.md).

## `--read-only` -- lock out writes

`--read-only` (`-r`) rejects write SQL, `.snapshot`, `.overwrite`, and
`.load` (including with `--sqlite`) entirely. Use it when you want to safely
publish a read-only demo environment.

<!-- verify -->
```console
$ ./san-db-ox --read-only -c "CREATE TABLE t (id INTEGER)" 2>&1; echo "exit=$?"
Error: attempt to write a readonly database (8)
exit=1
```

Reading still works normally.

<!-- verify -->
```console
$ ./san-db-ox --read-only -c "SELECT 1"
1
```

`--read-only` is a process-wide startup mode, not a per-path permission
setting (REPL / batch / stdio) -- the same restriction applies no matter
which path you come in through. For the specifics of publishing it
externally with `socat`, see
[Example: A temporary read-only demo](../examples/read-only-demo.md).

## Recap

- Data lives in memory and is gone unless explicitly saved with
  `.snapshot`/`.overwrite` ([Chapter 1](01-getting-started.md),
  [Chapter 3](03-saving-your-data.md)).
- The SQL itself is full-featured SQLite dialect
  ([Chapter 2](02-tables-and-queries.md)).
- The output format switches with `.mode`, and `json` shares its
  representation with the stdio protocol ([Chapter 4](04-output-modes.md),
  this chapter).
- It interoperates directly with plain SQLite files
  ([Chapter 5](05-sqlite-files.md)).
- The same dot commands as the REPL work through `-c`, a stdin script, or
  `--serve-stdio` ([Chapter 6](06-scripting.md), this chapter).

For more practical usage, see the [examples](../examples/README.md); for
details on every command and option, see
[CLI options, REPL commands, stdio protocol](../usage/README.md); to
understand the design behind it all, see the
[specification](../spec/san-db-ox_spec.md).
