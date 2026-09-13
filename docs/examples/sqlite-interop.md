*[日本語](sqlite-interop_ja.md)*

# Bridging to existing SQLite assets

The only non-executable file format SanDBox deals with is **a plain SQLite
file** (it defines no format of its own; spec §6). This page rounds up ways
to move data back and forth with existing SQLite tooling -- GUI apps,
pandas, and the like.

## Export as a SQLite file (`.snapshot --sqlite`)

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE events (id INTEGER PRIMARY KEY, label TEXT)" -c "INSERT INTO events VALUES (1, 'signup')" -c ".snapshot events --sqlite"
Wrote events.sqlite
```

`events.sqlite` is a plain SQLite file that **opens as-is** in DBeaver,
TablePlus, DB Browser for SQLite and similar GUI tools, the `sqlite3` CLI,
or any language's standard sqlite driver. Omitting the extension appends
`.sqlite` automatically.

### Reading it from pandas

```python
import sqlite3
import pandas as pd

con = sqlite3.connect("events.sqlite")
df = pd.read_sql_query("SELECT * FROM events", con)
```

## Import an existing SQLite database (`.load`)

`.load` determines the file's kind from its contents (the first 16 header
bytes), so the same command that loads a SanDBox executable also loads a
plain SQLite file. Loading **fully replaces** the in-memory DB (never
merges).

<!-- verify -->
```console
$ ./san-db-ox -c ".load events.sqlite" -c "SELECT * FROM events"
Loaded data from events.sqlite
1|signup
```

`.load` itself produces no file. To keep the loaded state, follow it with
`.snapshot` (save under a new name) or `.overwrite` (overwrite itself).

## Migrating via SQL text with `.dump` / `.import`

To move the contents as SQL statement text, without going through a binary
format, use `.dump`/`.import`.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER, note TEXT)" -c "INSERT INTO t VALUES (1, 'hello')" -c ".dump"
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE t (id INTEGER, note TEXT);
INSERT INTO "t" VALUES(1,'hello');
COMMIT;
```

`.dump`'s output can be fed straight into another SanDBox instance (or
SQLite itself) to reproduce the schema and data.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER, note TEXT)" -c "INSERT INTO t VALUES (1, 'hello')" -c ".dump" > dump.sql
$ ./san-db-ox < dump.sql -c "SELECT * FROM t" 2>&1
Error: SQL logic error: no such table: t (1)
```

**Giving `-c` means stdin is never read as a script at all** (so `.import`
and the like can still use stdin; `docs/usage/cli-options.md`). That's why
`t` isn't found above -- the `< dump.sql` content was silently ignored. If
you want to both load the dump and see a result, put the SELECT statement
itself at the end of the dump.

<!-- verify -->
```console
$ (cat dump.sql; echo "SELECT * FROM t;") | ./san-db-ox
1|hello
```

To import a CSV file, use `.import FILE TABLE`. If `TABLE` doesn't exist, it
is `CREATE`d automatically with the first row as all-TEXT column names.

<!-- verify -->
```console
$ printf 'id,note\n1,hello\n2,world\n' > rows.csv
$ ./san-db-ox -c ".import rows.csv t2" -c "SELECT * FROM t2"
Inserted 2 rows into "t2".
1|hello
2|world
```
