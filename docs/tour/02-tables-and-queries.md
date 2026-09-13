# 2. Tables and queries

*[Guided tour index](README.md) / Previous: [1. Getting started](01-getting-started.md) / [日本語](02-tables-and-queries_ja.md)*

## Create a table

SQLite-dialect SQL works as-is. CREATE/INSERT/SELECT/UPDATE/DELETE, and the
full feature set -- views, indexes, triggers, transactions -- all work.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c "INSERT INTO users VALUES (2, 'bob')" -c "SELECT * FROM users"
1|alice
2|bob
```

The default output format (`list` mode) is `|`-separated with no header.
Other formats are covered in [Chapter 4](04-output-modes.md).

## Input spanning multiple lines

In an interactive session, a statement isn't complete until it reaches `;`,
so you can split a SQL statement across multiple lines (the continuation
prompt `   ...> ` appears).

```
SanDBox> CREATE TABLE users (
   ...>   id INTEGER PRIMARY KEY,
   ...>   name TEXT
   ...> );
SanDBox>
```

Batch execution (`-c` or a script) follows the same rule -- even split
across lines, it's treated as one statement until `;` arrives.

<!-- verify -->
```console
$ printf 'CREATE TABLE users (\n  id INTEGER PRIMARY KEY,\n  name TEXT\n);\nINSERT INTO users VALUES (1, %s);\nSELECT * FROM users;\n' "'alice'" | ./san-db-ox
1|alice
```

## Table listing and schema

`.tables` lists table names; `.schema [TABLE]` shows the CREATE statement.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c ".tables" -c ".schema users"
users
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
```

`.tables` and `.schema` are dot commands (control commands starting with
`.`), not SQL statements. SanDBox's command set follows the `sqlite3` CLI,
so the names should look familiar if you've used it before.

*Next: [3. Saving your data](03-saving-your-data.md)*
