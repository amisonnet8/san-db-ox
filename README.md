# SanDBox

*[日本語](README_ja.md)*

**A portable, single-binary RDBMS with no setup required.** The DB engine and
the data itself live in one executable file -- download it and run it. Written
in Go, backed internally by [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite).
**It never listens on a network.** Talking to it from another program means
launching it as a child process and speaking the stdio protocol over its
standard streams, or reading/writing a plain SQLite file.

## Install

**No Go toolchain?** Grab a prebuilt binary for your OS/architecture from
[GitHub Releases](https://github.com/amisonnet8/san-db-ox/releases/latest).

```sh
chmod +x san-db-ox   # Linux/macOS only -- downloads don't keep the execute bit
./san-db-ox
```

**Have Go installed?**

```sh
go install github.com/amisonnet8/san-db-ox/cmd/san-db-ox@latest
```

## 30-second demo

```
$ ./san-db-ox
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox> CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
SanDBox> INSERT INTO users VALUES (1, 'alice');
SanDBox> SELECT * FROM users;
1|alice
SanDBox> .snapshot mydb
Wrote mydb
SanDBox> .exit
```

`mydb` is now **itself a runnable executable**. Hand it to someone else, and
running it brings up the same database, data included.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".snapshot mydb"
Wrote mydb
$ chmod +x mydb && ./mydb -c "SELECT * FROM users"
1|alice
```

## Four ways to run it

| Mode | How to start it | For |
| :--- | :--- | :--- |
| REPL | no arguments | interactive use by a human |
| Batch execution | `-c "SQL"` / a script on stdin | CI/CD, shell scripts |
| stdio protocol | `--serve-stdio` | embedding in another program |
| Library | the `engine` package, in your own Go app | an embedded DB layer |

None of them listen on a network.

## A note on using it over a network

SanDBox itself never imports `net`, but bolting on a transport with `socat` or
similar lets it be reached over TCP/a Unix socket. Two things to keep in mind
if you do:

1. **Each connection gets its own process, and its own database.** `socat`'s
   `fork` spawns a new process per connection, so there is no way to share one
   database across multiple connections this way.
2. **There is no authentication at all.** Anyone who can reach it can run any
   SQL, DDL included. For read-only demos, pair it with `--read-only`, and
   restrict who can reach it at all (`bind=127.0.0.1`, an SSH tunnel, etc).

## Learn more

| Document | What's in it |
| :--- | :--- |
| [Guided tour](docs/tour/README.md) | a hands-on, step-by-step tutorial |
| [Examples](docs/examples/README.md) | practical, task-oriented recipes |
| [CLI options, REPL commands, stdio protocol](docs/usage/README.md) | a quick-reference index |
| [Specification](docs/spec/san-db-ox_spec.md) | the design rationale behind it all |

## License

[MIT](LICENSE)
