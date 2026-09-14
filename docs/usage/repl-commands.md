*[日本語](repl-commands_ja.md) | **English***

# REPL commands

Besides SQL statements, you can run control commands that start with `.`
("dot commands"). The command set follows the SQLite CLI (`sqlite3`).

**Dot commands work the same way in batch execution mode (`-c` / stdin).**
The stdio protocol exposes each as a corresponding op.

## List

### Saving and loading

| Command | Role |
| :--- | :--- |
| `.snapshot [FILE] [--sqlite] [--timestamp]` | Save the current data. By default produces **another executable**; with `--sqlite`, a **SQLite file** |
| `.overwrite` | Overwrite this process's own executable with the current data, then **exit the process** |
| `.load FILE` | Load data from another SanDBox executable or a SQLite file, replacing the in-memory DB |

### Schema and data

| Command | Role |
| :--- | :--- |
| `.tables` | List tables |
| `.schema [TABLE]` | Show CREATE statements (all tables if omitted) |
| `.dump [PATTERN]` | Dump schema and data as SQL statements (`PATTERN` is a SQL LIKE pattern; all tables if omitted) |
| `.import FILE TABLE` | Load a CSV file into a table |

### Display settings

| Command | Role |
| :--- | :--- |
| `.mode MODE` | Change the output format (`list` / `column` / `csv` / `json` / `line`) |
| `.headers on\|off` | Whether to show column names in results |

### Other

| Command | Role |
| :--- | :--- |
| `.help` | Show the command list |
| `.exit [CODE]` / `.quit [CODE]` | Exit (`CODE` defaults to 0) |

## Output modes (`.mode`)

```
SanDBox> SELECT id, name FROM users;
```

**`list`** (default) -- `|`-separated, no header, NULL as an empty string

```
1|alice
2|bob
```

**`column`** -- column widths aligned to the widest value, left-justified.
Switching to it turns `.headers` on automatically

```
id  name
--  -----
1   alice
2   bob
```

**`csv`** -- RFC 4180. Lines end in CRLF

```
id,name
1,alice
2,bob
```

**`json`** -- uses the same value representation as the [stdio protocol](stdio-protocol.md)

```json
{"columns":["id","name"],"rows":[[1,"alice"],[2,"bob"]]}
```

**`line`** -- one column per line, a blank line between records

```
  id = 1
name = alice

  id = 2
name = bob
```

All of the above are interactive transcripts assuming `users` already holds
`(1,'alice')` and `(2,'bob')`, but `-c` produces identical output (`csv` is
omitted here since its CRLF line endings don't render correctly as plain
text in this document -- `tests/e2e.sh` verifies the actual behavior).

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m list
1|alice
2|bob
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m column
id  name
--  -----
1   alice
2   bob
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m json
{"columns":["id","name"],"rows":[[1,"alice"],[2,"bob"]]}
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m line
  id = 1
name = alice

  id = 2
name = bob
```

## How saving works

**Saving is always explicit.** Exiting via `.exit` / `.quit` / Ctrl+D /
SIGTERM never saves; the data is simply gone. There is no confirmation
prompt either.

```
SanDBox> .snapshot mydb          # produces ./mydb (an executable)
SanDBox> .snapshot mydb --sqlite # produces ./mydb.sqlite (a SQLite file)
SanDBox> .snapshot --timestamp   # produces ./san-db-ox_20260901120000
SanDBox> .overwrite              # rewrites itself and exits
```

A file produced by `.snapshot --sqlite` **opens as-is** in DBeaver, DB
Browser for SQLite, pandas, the `sqlite3` CLI, and the like. Omitting the
extension appends `.sqlite`.

`.load` determines the file's kind from its contents. The same command
accepts either a SanDBox executable or a SQLite file. Loading **fully
replaces** the in-memory DB (never merges). No file is produced, so run
`.snapshot` or `.overwrite` afterward if you want to keep the result.

### Moving data to a binary for a different OS

```
# 1. Start an empty binary for the target OS (e.g. the Windows build, on Windows)
./san-db-ox.exe

# 2. Load the data
SanDBox> .load mydb_from_linux

# 3. Write it into this binary itself
SanDBox> .overwrite
```

`.load` never touches the source file's engine portion, which is what makes
this cross-OS/cross-architecture data migration possible.

## `.import` semantics

- **Always reads as CSV, regardless of the `.mode` setting.**
- If `TABLE` doesn't exist, it is `CREATE`d automatically with the first row
  as all-TEXT column names. If it exists, the first row is treated as data.
- **A row whose field count doesn't match the column count aborts the whole
  operation with an error naming that row, inserting nothing.** (`sqlite3`
  instead pads/truncates with a warning and continues; SanDBox deliberately
  aborts.)

## Key bindings (interactive)

| Key | Action |
| :--- | :--- |
| Ctrl+D | Exit (EOF) |
| Ctrl+C (query running) | Cancel just that query, return to the prompt |
| Ctrl+C (idle, once) | Discard the unfinished SQL being typed, reprint the prompt |
| Ctrl+C (idle, twice in a row) | Exit the process (exit code 1) |

The "twice in a row" detection resets as soon as even one new input line is
read in between.

**When stdin isn't a terminal, no Ctrl+C handler is installed.** A SanDBox
started via a pipe or a script exits immediately on SIGINT, as usual.

## Commands not adopted

Since SanDBox always deals with exactly one in-memory DB, it omits these
`sqlite3`-derived commands.

| Command | Reason |
| :--- | :--- |
| `.open` / `.databases` | No concept of switching between multiple DBs |
| `.backup` / `.restore` | Overlaps with `.snapshot`'s role |
| `.session` | Change-tracking session feature; out of scope |

Among the output modes, `quote` / `insert` / `tabs` / `markdown` / `box` /
`html` are likewise not adopted (to keep decoration minimal and stay
pipe-friendly).
