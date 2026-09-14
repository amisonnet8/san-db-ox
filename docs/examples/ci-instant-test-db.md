*[日本語](ci-instant-test-db_ja.md) | **English***

# CI/CD Instant Test DB

Prepare one binary with its schema built and seed data loaded, and a CI job
gets an in-memory test environment up in under a second. No waiting for an
external DB container to start, no migrations to run, no cleanup afterward.

## Build the seeded binary

Feed the schema and seed data into an empty binary with `-c`, then pin it as
a new executable with `.snapshot`.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, role TEXT)" -c "INSERT INTO users VALUES (1, 'alice', 'admin'), (2, 'bob', 'member')" -c ".snapshot seeded_test_db"
Wrote seeded_test_db
```

There's no need to commit `seeded_test_db` to the repo -- **put this very
command in the CI setup step, and the same-state binary gets regenerated
every run.** Changing the test's baseline data is then just a matter of
editing this command.

## Use it from a test

Judging by `-c` (or a stdin script) and the exit code lets you call it
straight from a shell script or a Makefile (spec §5).

<!-- verify -->
```console
$ chmod +x seeded_test_db && ./seeded_test_db -c "SELECT count(*) FROM users WHERE role = 'admin'" -m json
{"columns":["count(*)"],"rows":[[1]]}
```

If the count doesn't match what you expected, write the assertion so it
exits **non-zero**, the same as a SQL execution error would. For example,
wire it into a shell script in place of calling `sqlite3`:

```sh
#!/usr/bin/env bash
set -euo pipefail

count="$(./seeded_test_db -c "SELECT count(*) FROM users WHERE role = 'admin'")"
if [ "$count" != "1" ]; then
  echo "expected 1 admin user, got $count" >&2
  exit 1
fi
```

## Keeping logs separate from results

If you don't want CI's logs cluttering the results, `2>/dev/null` drops the
logs/warnings (results go to stdout, logs/errors to stderr; spec §0).

<!-- verify -->
```console
$ ./seeded_test_db -c "SELECT count(*) FROM users" -m json 2>/dev/null
{"columns":["count(*)"],"rows":[[2]]}
```
