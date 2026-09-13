# 4. Output modes

*[Guided tour index](README.md) / Previous: [3. Saving your data](03-saving-your-data.md) / [日本語](04-output-modes_ja.md)*

`.mode` switches how results are displayed. The default is `list`.

## `list`, `column`, `json`, `line`

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m list
1|alice
2|bob
```

`column` aligns column widths and left-justifies them. Switching to it turns
`.headers` (column-name display) on automatically.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m column
id  name
--  -----
1   alice
2   bob
```

`json` uses a representation that preserves the NULL/INTEGER/REAL/TEXT/BLOB
type (the same rules as the stdio protocol, covered in
[Chapter 7](07-stdio-and-read-only.md)).

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m json
{"columns":["id","name"],"rows":[[1,"alice"],[2,"bob"]]}
```

`line` puts one column per line, with a blank line between records.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m line
  id = 1
name = alice

  id = 2
name = bob
```

There's also `csv` (RFC 4180, CRLF line endings), omitted here since CRLF
doesn't render well as plain text in this document. The `-m` startup option
selects the same formats (see [Chapter 6](06-scripting.md)).

## Toggling `.headers` on its own

Modes other than `column` default to no header, but `.headers on` shows it
explicitly.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice')" -c ".headers on" -c "SELECT id, name FROM users"
id|name
1|alice
```

## `.dump` -- write schema and data out as SQL

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".dump"
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO "users" VALUES(1,'alice');
COMMIT;
```

The output is shaped to be fed straight into another SanDBox (or SQLite)
instance. Interoperating with SQLite files is covered in
[Chapter 5](05-sqlite-files.md).

*Next: [5. Working with SQLite files](05-sqlite-files.md)*
