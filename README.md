*[日本語](README_ja.md) | **English***

<div align="center">

# SanDBox

**A disposable SQL sandbox in a single binary.**
Experiment freely, then snapshot the in-memory database into a new runnable file and share it.
No install, no server, no network.

[![CI](https://github.com/amisonnet8/san-db-ox/actions/workflows/test.yml/badge.svg)](https://github.com/amisonnet8/san-db-ox/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/amisonnet8/san-db-ox)](https://github.com/amisonnet8/san-db-ox/releases/latest)
[![License: MIT](https://img.shields.io/github/license/amisonnet8/san-db-ox)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/amisonnet8/san-db-ox)](go.mod)

![Demo: creating a table, using .overwrite to embed the data into the running executable itself, then relaunching it and reading the data back out with SELECT](docs/img/demo.gif)

</div>

A portable RDBMS that keeps the DB engine and the data itself in one
executable file -- download it and just run it. Written in Go, backed
internally by [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite).
**It never listens on a network** -- talking to it from another program means
launching it as a child process and speaking the stdio protocol over its
standard streams, or reading/writing a plain SQLite file.

## Table of contents

- [Features](#features)
- [Install](#install)
- [30-second demo](#30-second-demo)
- [Four ways to run it](#four-ways-to-run-it)
- [A note on using it over a network](#a-note-on-using-it-over-a-network)
- [Learn more](#learn-more)
- [License](#license)

## Features

- 🧳 **Single binary, start to finish** -- the engine and the data live in
  one executable. No install, no dependencies, no setup
- 📸 **Runnable snapshots** -- `.snapshot` writes out the current DB as a
  "running" file you can hand to someone else as-is
- 🔒 **Zero network listening** -- never imports the `net` package; exposing
  it externally is left to bolting on a transport (`socat`, etc.)
- 🔌 **stdio integration** -- a JSON Lines protocol over standard streams for
  talking to it from other languages/processes
- 🗄️ **SQLite file interop** -- write out with `.snapshot --sqlite`, or pull
  an existing SQLite file in with `.load`
- 🧪 **Read-only mode** -- `--read-only` lets you expose demos/viewers safely

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
SanDBox> .overwrite
Overwrite ok, exiting.
$ ./san-db-ox
SanDBox v0.1.0
Loaded snapshot: san-db-ox
Enter ".help" for usage hints.
SanDBox> SELECT * FROM users;
1|alice
SanDBox> .exit
```

`.overwrite` rewrites **the running binary itself**, so `san-db-ox` is now
itself a runnable file with the data inside it. Hand it to someone else, and
running it brings up the same database, data included.

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".overwrite"
Overwrite ok, exiting.
$ ./san-db-ox -c "SELECT * FROM users"
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
| [Client drivers](https://github.com/amisonnet8/san-db-ox-clients) | language bindings (C, Python, Go, ...) for the stdio protocol |
| [Interactive guide](https://notebook.google.com/notebook/54a7599d-12a2-4bb5-a194-1bd3e1a4d426) | a Gemini Notebook you can ask questions in |

## License

[MIT](LICENSE)

---

<div align="center">

[Back to top](#table-of-contents) · [Issues](https://github.com/amisonnet8/san-db-ox/issues)

</div>
