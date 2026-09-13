# 1. Getting started

*[Guided tour index](README.md) / [日本語](01-getting-started_ja.md)*

## Get it

No Go toolchain? Download the executable for your OS/architecture from
[GitHub Releases](https://github.com/amisonnet8/san-db-ox/releases/latest).
Have Go installed? `go install github.com/amisonnet8/san-db-ox/cmd/san-db-ox@latest`
works too. From here on, this executable is called `san-db-ox`
(`san-db-ox.exe` on Windows).

```sh
chmod +x san-db-ox   # Linux/macOS only
```

## Start it

Run it with no arguments, and an interactive SQL console (REPL) starts up.

```
$ ./san-db-ox
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox>
```

Line 1 is the version. Line 2 means "this binary has no data embedded yet"
-- always the case right after downloading. As line 3 hints, type `.help`
any time to see the command list.

`SanDBox> ` is the prompt. This is where you type SQL statements and dot
commands (control commands starting with `.`).

## Your first SELECT

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT 1 + 1"
2
```

(From here on, an interactive prompt's look is shown in a box like the one
above, but a command that's been confirmed to actually work is also shown
in this `-c` form -- batch execution. Whether interactive or batch, the SQL
itself and how results look are the same.)

Trying the same thing in an interactive session:

```
SanDBox> SELECT 1 + 1;
2
```

**A SQL statement ends with `;`.** Forget it, and SanDBox assumes the
statement isn't finished yet and waits for more input on the next line (the
continuation prompt `   ...> ` appears).

## Exit

`.exit` (or `.quit`, or Ctrl+D) exits.

<!-- verify -->
```console
$ ./san-db-ox -c ".exit"
```

**Important: nothing is saved at this point.** SanDBox is an in-memory DB --
unless you explicitly run `.snapshot` or `.overwrite`, all data vanishes the
moment the process exits. How to save is covered in
[Chapter 3](03-saving-your-data.md).

The next chapter creates an actual table and moves data in and out of it.

*Next: [2. Tables and queries](02-tables-and-queries.md)*
