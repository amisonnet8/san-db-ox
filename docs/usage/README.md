*[日本語](README_ja.md)*

# SanDBox reference

A "use it right now" index. For the reasoning behind each design decision, see
the specification in `docs/spec/`. New to SanDBox? Start with the
[guided tour](../tour/README.md); for task-oriented recipes, see the
[examples](../examples/README.md).

| Document | Contents |
| :--- | :--- |
| [CLI options](cli-options.md) | Command-line arguments, how the run mode is chosen, exit codes |
| [REPL commands](repl-commands.md) | Dot commands, output modes, key bindings |
| [stdio protocol](stdio-protocol.md) | JSON Lines requests/responses, the op list, value representation |

## Try it in 30 seconds

```sh
# Download and run -- nothing to install
chmod +x san-db-ox
./san-db-ox
```

```
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox> CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
SanDBox> INSERT INTO users VALUES (1, 'alice');
SanDBox> .overwrite
Overwrite ok, exiting.
```

`.overwrite` rewrites **the running binary itself**, so `san-db-ox` is now
itself a runnable file with the data inside it. Hand it to someone else, and
running it brings up the same database.

```sh
./san-db-ox
```

```
SanDBox v0.1.0
Loaded snapshot: san-db-ox
Enter ".help" for usage hints.
SanDBox> SELECT * FROM users;
1|alice
SanDBox> .exit
```

## Four ways to run it

| Mode | How to start it | For |
| :--- | :--- | :--- |
| REPL | no arguments | interactive use by a human |
| Batch execution | `-c "SQL"` / a script on stdin | CI/CD, shell scripts |
| stdio protocol | `--serve-stdio` | embedding in another program |
| Library | the `engine` package, in your own Go app | an embedded DB layer |

None of them listen on a network. To reach one over a network, bolt on a
transport with `socat` or similar (spec §8).

## Worth remembering

- **Saving is always explicit.** `.exit` does not save your data. Run
  `.snapshot` or `.overwrite`.
- **`.snapshot` produces an executable.** Want a plain SQLite file instead?
  Use `.snapshot --sqlite`.
- **Logs go to stderr, results go to stdout.** From a script, `2>/dev/null`
  drops the logs.
