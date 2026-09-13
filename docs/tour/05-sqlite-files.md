# 5. Working with SQLite files

*[Guided tour index](README.md) / Previous: [4. Output modes](04-output-modes.md) / [日本語](05-sqlite-files_ja.md)*

The only non-executable file format SanDBox deals with isn't a format of its
own -- it's **a plain SQLite file**. That means it interoperates directly
with the existing SQLite ecosystem.

## `.snapshot --sqlite` -- export as a SQLite file

`.snapshot` produces an executable by default; add `--sqlite` and it
produces a SQLite file instead.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".snapshot mydb --sqlite"
Wrote mydb.sqlite
```

`mydb.sqlite` is a plain SQLite file that **opens as-is** in DBeaver, DB
Browser for SQLite, the `sqlite3` CLI, pandas, and so on.

## `.load` -- import a SQLite file

`.load` determines the file's kind from its contents, so the same command
loads either a SanDBox executable or a SQLite file. Loading fully replaces
the in-memory DB (never merges).

<!-- verify -->
```console
$ ./san-db-ox -c ".load mydb.sqlite" -c "SELECT * FROM users"
Loaded data from mydb.sqlite
1|alice
```

`.load` itself produces no file. To keep the loaded state, follow it with
`.snapshot` or `.overwrite` ([Chapter 3](03-saving-your-data.md)).

## `.import` -- import a CSV file

`.import FILE TABLE` always reads as CSV, regardless of the `.mode`
setting. If `TABLE` doesn't exist, it's created automatically with the
first row as all-TEXT column names.

<!-- verify -->
```console
$ printf 'id,name\n1,alice\n2,bob\n' > people.csv
$ ./san-db-ox -c ".import people.csv people" -c "SELECT * FROM people"
Inserted 2 rows into "people".
1|alice
2|bob
```

A row whose field count doesn't match the column count aborts the whole
operation with an error naming that row, inserting nothing (`sqlite3`
instead pads/truncates with a warning and continues; SanDBox deliberately
aborts).

*Next: [6. Using it from scripts](06-scripting.md)*
