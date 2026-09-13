# 6. Using it from scripts

*[Guided tour index](README.md) / Previous: [5. Working with SQLite files](05-sqlite-files.md) / [日本語](06-scripting_ja.md)*

Everything so far has assumed interactive (REPL) use, but batch execution
with `-c` or a script over stdin lets you call it straight from a shell
script or a CI pipeline.

## Run one statement (or several) with `-c`

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT 1 + 1"
2
```

`-c` can be given multiple times, and runs in the order given.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER)" -c "INSERT INTO t VALUES (1)" -c "SELECT * FROM t"
1
```

**Each `-c` value is treated as its own independent unit of execution.** A
missing trailing `;` is implicitly supplied at the end of that value. Put
another way, a single statement never continues across multiple `-c`
values.

## Reading a script from stdin

Feed SQL into stdin with no arguments, and it runs as a script (when stdin
isn't an interactive terminal -- a pipe or redirect qualifies).

<!-- verify -->
```console
$ printf 'CREATE TABLE t (id INTEGER);\nINSERT INTO t VALUES (42);\nSELECT * FROM t;\n' | ./san-db-ox
42
```

## Aborting immediately on error

Batch execution aborts the entire run the moment an error occurs; no later
statement runs. Exit code `1`.

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT 1" -c "not valid sql" -c "SELECT 2" 2>&1; echo "exit=$?"
1
Error: SQL logic error: near "not": syntax error (1)
exit=1
```

The second `-c` failed, so the third one (`SELECT 2`) never ran.

## Using it in a CI pipeline

Results go to stdout and logs/errors to stderr, so `2>/dev/null` drops the
logs and lets you judge by exit code alone.

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT count(*) FROM sqlite_master" -m json 2>/dev/null; echo "exit=$?"
{"columns":["count(*)"],"rows":[[0]]}
exit=0
```

For a more realistic example, see
[Example: CI/CD Instant Test DB](../examples/ci-instant-test-db.md).

*Next: [7. stdio and read-only mode](07-stdio-and-read-only.md)*
