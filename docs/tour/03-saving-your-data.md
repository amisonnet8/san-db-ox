# 3. Saving your data

*[Guided tour index](README.md) / Previous: [2. Tables and queries](02-tables-and-queries.md) / [日本語](03-saving-your-data_ja.md)*

As mentioned in Chapter 1, `.exit` doesn't save anything. Saving happens
only through the explicit `.snapshot` or `.overwrite` operations.

## `.snapshot` -- save as a differently-named executable

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".snapshot mydb"
Wrote mydb
```

`mydb` is now **itself a runnable executable**. It carries both the DB
engine and the data, so handing this one file to someone else lets them
bring up the same database just by running it.

<!-- verify -->
```console
$ chmod +x mydb && ./mydb -c "SELECT * FROM users"
1|alice
```

Omit the filename, and the running binary's own name is used as the base
(pair it with `--timestamp` to append a timestamp).

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t(x)" -c ".snapshot --timestamp"
Wrote san-db-ox_20260901120000
```

## `.overwrite` -- rewrite itself

`.overwrite` overwrites the currently running process's own executable with
the data currently in memory. No filename needed, and **it exits the
process the moment it succeeds.**

<!-- verify -->
```console
$ cp mydb mydb2 && chmod +x mydb2 && ./mydb2 -c "INSERT INTO users VALUES (2, 'bob')" -c ".overwrite"
Overwrite ok, exiting.
$ ./mydb2 -c "SELECT * FROM users"
1|alice
2|bob
```

The `bob` row is now written into `mydb2` itself, embedded from the start
the next time it starts up. This shape suits automation like "load data,
then finish the build" -- for instance, filling an empty binary with seed
data in CI to produce a binary ready for distribution (see
[Example: CI/CD Instant Test DB](../examples/ci-instant-test-db.md)).

## Which one to use

- **Handing it to someone else, or keeping it as a separate version** →
  `.snapshot` (the original file is untouched)
- **Committing the data into the current binary itself** → `.overwrite`

*Next: [4. Output modes](04-output-modes.md)*
