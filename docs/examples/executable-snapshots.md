*[日本語](executable-snapshots_ja.md) | **English***

# Sharing bugs as executable snapshots

Pin the data state that triggered a bug as an executable, and share it with
your team. Whoever receives it just runs it to reproduce exactly the same DB
environment the reporter saw -- a feature meant to end the "can't reproduce
it on my end" conversation.

## Save the state that reproduces the bug

Run `.snapshot` while the data is still in the state that triggered the bug.
Including a ticket number or similar in the filename makes it easy to track
down later.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE orders (id INTEGER PRIMARY KEY, status TEXT)" -c "INSERT INTO orders VALUES (1, 'stuck')" -c ".snapshot bug_123"
Wrote bug_123
```

Hand `bug_123` over as-is, over Slack or a file share. The recipient has
exactly one thing to do.

<!-- verify -->
```console
$ chmod +x bug_123 && ./bug_123 -c "SELECT * FROM orders"
1|stuck
```

No install, no connection string to share, no dump to load -- because **the
binary itself carries the data**.

## Handing it to someone on a different OS

The executable `.snapshot` produces is built on top of **the currently
running process's own engine portion**. A file built on Linux only runs on
Linux. If the other person is on a different OS (macOS, Windows, ...), move
just the data across instead.

1. On their side, download an **empty binary** (no embedded data) for the
   target OS from
   [GitHub Releases](https://github.com/amisonnet8/san-db-ox/releases/latest).
2. Load just the data from the shared executable with `.load` (it never
   touches the source file's engine portion, so a different OS is no
   problem).
3. Write it into the (target-OS) binary that's currently running with
   `.overwrite`.

```
$ ./san-db-ox-windows.exe
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox> .load bug_123
SanDBox> .overwrite
Overwrite ok, exiting.
```

`san-db-ox-windows.exe` is now itself a Windows executable carrying the same
data as the Linux version.

## Note: `.snapshot` produces an executable; `.snapshot --sqlite` produces a SQLite file

`.snapshot` produces an executable by default, so the recipient can use it
without even being aware SanDBox exists. To share it as a plain SQLite file
instead, see
[Bridging to existing SQLite assets](sqlite-interop.md).
