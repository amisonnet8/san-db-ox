*[日本語](san-db-ox_spec_ja.md) | **English***

# 🗃️ SanDBox Specification

**SanDBox** is a portable, single-binary RDBMS that keeps both the DB engine and the data area inside a single executable file, requiring no environment setup.

**License:** MIT

> **What this document is:** a design/reference specification for developers and advanced users. It records **what is being built and how**, and **why it was decided that way**.
>
> * For a quick-reference index for "using it right now," see `docs/usage/` (the list of startup options, REPL commands, and stdio protocol ops).
> * **Development rules** such as testing policy, development phases, distribution method, and naming conventions are out of scope for this document.
> * Client drivers for each language are provided by a separate project, [`san-db-ox-clients`](https://github.com/amisonnet8/san-db-ox-clients). This document defines only **up through the protocol**.
>
> **This document is grown alongside the implementation; when the implementation and the spec drift apart, update this document.** When changing a design decision, update this document first, then start work.

---

## 0. Naming and notation

A coined word embedding "DB" inside "Sandbox." Notation follows a 3-tier priority order. **Always use the higher tier wherever it can be used.**

| Priority | Notation | Where it's used |
| :--- | :--- | :--- |
| 1st | `san-db-ox` | Every unconstrained context (repo name, executable name, module path, socket path, image name) |
| 2nd | `san_db_ox` | Where hyphens can't be used (environment variables, C identifiers) |
| 3rd | `sandbox` | Where neither of the above fits, for reasons like length (fixed-length fields) |

| Slot | Canonical form | Applicable tier |
| :--- | :--- | :--- |
| Display name (banner, README, this spec) | `SanDBox` | -- |
| Repository / Go module path | `github.com/amisonnet8/san-db-ox` | 1st |
| Executable name | `san-db-ox` (Windows: `san-db-ox.exe`) | 1st |
| Backup file during self-overwrite | `<path>.san-db-ox.old` | 1st |
| Environment variable (none yet; for future additions) | `SAN_DB_OX_*` | 2nd |
| C client identifiers | `san_db_ox_*` | 2nd |
| Go package name | `engine` | -- |
| Footer magic (fixed 8 bytes) | `SANDBOX1` | 3rd (9 characters doesn't fit the field) |
| REPL prompt | `SanDBox> ` | -- (uses the display name as-is, matching the startup banner's `SanDBox v0.1.0`; settled in Phase ① Step 4) |
| REPL continuation prompt (mid multi-line SQL input) | `   ...> ` | -- (aligned to the same width as `SanDBox> `, following the `sqlite3` CLI's `   ...> `; settled in Phase ③) |

**No alias is provided.** Users are expected to say "sandbox" out loud and be tempted to type `sandbox` in a terminal, but `sandbox` is the 3rd-tier fallback for when nothing else fits, and providing it as a command name users type would violate the rule above. No symlink or wrapper script is bundled; this is stated plainly in the README instead (also consistent with the single-binary-and-done concept).

**Hyphens and the timestamp rule:** A hyphen in the executable name doesn't conflict with §12's "appending a timestamp" rule (the separator is fixed as `_`). It comes out as `san-db-ox_20260901120000.exe`. When implementing, base-name matching must assume the string can contain hyphens.

**File extensions never include the product name.** A non-executable file is a plain SQLite file (§6), following the existing `.db` / `.sqlite` convention. No custom extension like `.san-db-ox` or `.sandbox` is used.

**Not a "security sandbox":** This software provides no isolation, permission restriction, or resource limiting. As §1's Zero-Auth states, it has no concept of privilege separation at all. It has **nothing whatsoever** to do with the security-containment feature the name might suggest.

**CLI output language:** REPL messages, error messages, `--help`, log output, and other strings the CLI produces are, as a rule, in **English**, on the assumption that this is published and used as international OSS.

**Where logs go:** Log/diagnostic messages go **only to standard error (stderr)**; there is no mechanism for generating or rotating log files. In keeping with the core concept of being self-contained in a single binary, the design deliberately produces no log file as a byproduct. Query results and protocol responses go to standard output (stdout), so the two are always kept apart. **This separation is also a precondition for §7's stdio protocol to work at all.** Output itself can also be suppressed with `-q`/`--quiet`.

---

## 1. Core concepts

* **Data-in-Binary (self-contained in a single binary):** Keeps a data area inside the executable file itself, eliminating any need for an external DB file, elaborate environment setup, or a Docker volume mount.
* **In-Memory Operations:** Expands all data into memory at startup and handles every query at memory speed (ultra-low latency).
* **Snapshot Persistence (outputting a differently-named executable):** On stop (or on an in-flight save instruction), persists by generating a **new executable (under a different name)** that holds the latest in-memory data. Keeping the original executable intact makes it easy to create safe snapshots and distribute data.
* **Zero-Auth & Zero-Network:** Has no concept of user management, permissions, or logins. **It never listens on a network itself.**
* **Persistence only through explicit operations:** Data is only ever saved on the user's explicit instruction (`.snapshot` / `.overwrite`). Automatic saving on process stop is done **with no exception** (aside from the opt-in periodic save via `--snapshot-interval`).

### The choice not to have a network

SanDBox implements no network protocol. It has no authentication mechanism because the product itself never creates a path reachable from the outside in the first place. This is safety from "there structurally is no way in," not from "defensive logic that blocks it" -- and not importing `net`/`net/http` is something verifiable at the code level (§10).

The cost of this choice is that "drop one binary and you have a DB server" isn't a shape SanDBox can take. Instead, integration with other systems happens through two channels:

1. **Launched as a process, talking over standard input/output** (§5, §7). Anything that can spawn a subprocess and handle pipes can integrate from any language, with no dedicated library.
2. **Read and written as a plain SQLite file** (§6). Connects with zero protocol implementation to existing ecosystems -- GUI tools, each language's sqlite driver, pandas, and the rest.

To publish it over a network, bolt on a transport with an existing tool like `socat` (§8). The decision and responsibility for opening that door sit with the operator, not with the product.

---

## 2. Interfaces

SanDBox has four run modes. **None of them involves listening on a network** (the only entity that can reach it is whoever can launch the process -- the local user).

| Mode | How to start it | For | I/O |
| :--- | :--- | :--- | :--- |
| **Interactive console (REPL)** | Started with no arguments (default) | Interactive use by a human | Terminal |
| **Batch execution** | `-c "SQL"` / a script on stdin | CI/CD, use from shell scripts | stdout (per `--mode`) |
| **stdio protocol** | `--serve-stdio` | Embedding in another program (§7) | stdout (fixed as JSON Lines) |
| **Library** | The `engine` package, embedded in a Go app | An embedded DB layer (§10) | Go function calls |

> See `docs/usage/` for the command/option/op list for each mode. This document covers the design decisions and their reasoning.

### No access control

No access restriction by path or statement kind (rejecting DDL, for instance) is implemented. **Every mode can run DDL / DML / TCL in full.**

The reasoning: restricting at the SQL level provides no actual defense. Since every mode is launched as a local process, whoever can launch it already has read/write access to the executable itself. Blocking DDL changes nothing -- that same party can rewrite the binary directly, or run a different binary altogether. So the restriction would only add complexity, protecting nothing.

### The one exception: read-only mode

The single exception to the above: specifying `--read-only` (`-r`, §12) at startup rejects every write operation.

This doesn't contradict the previous section. **It's not a restriction by path or statement kind -- it's a process-wide startup mode.** Whoever launches the process declares, of their own will, "this process shall not write" -- it is not a defense against someone outside the process. Its main use is external publication via `socat` (§8), for safely offering a read-only demo environment.

**Implementation note:** Write-SQL rejection is achieved by running `PRAGMA query_only = ON` on the `Session` (see "Concurrency Control" below) that `cmd/san-db-ox` holds, right after startup -- SanDBox doesn't implement its own statement-kind classification and rejection logic; it simply defers to the mechanism SQLite itself already has. `.snapshot` / `.overwrite` / `.load` (including `.snapshot --sqlite`), on the other hand, are not SQL statements, so they fall outside `query_only`'s reach and are rejected by an explicit check on the `cmd/san-db-ox` side instead.

The rejected operations:

* Write SQL (INSERT / UPDATE / DELETE / DDL / writes inside a transaction). Implementation is deferred to SQLite's `query_only` pragma
* Save operations (`.snapshot` / `.overwrite`)
* Data replacement (`.load`)
* **Exporting a SQLite file (`.snapshot --sqlite`)**. It doesn't change the DB, but it writes to an arbitrary path on the server-side filesystem, making it just as dangerous as a write operation when exposed externally
* Combining it with `--snapshot-interval` is a usage error (exit code 2)

In REPL mode, `.help`'s output is swapped for a read-only variant that omits the rejected operations from its listing. The startup banner also shows that it is read-only (§13). In the stdio protocol, a rejected operation returns the `read_only` error code.

### Concurrency Control

Concurrency control such as locking and transaction isolation levels is not implemented by SanDBox itself; it defers entirely to whatever mechanism the internal SQL-compatible engine (§11) provides by default.

Every mode of `cmd/san-db-ox` has **only ever one client at a time** (in the REPL, the process itself is the sole client; stdio is one process per session; batch execution is short-lived). So there is never a case where the application side needs to arbitrate between multiple clients.

Multiple clients can only coexist when `engine` is embedded as a library in a host app, and in that case SQLite's own exclusion mechanism operates per dedicated connection returned by `DB.Session(ctx)` (internally, a separate connection pointing at the same in-memory DB on `modernc.org/sqlite`'s `memdb` VFS; see §11). `BEGIN`/`COMMIT`/`ROLLBACK` run as plain SQL statements on that connection, and waiting/failing on a lock conflict is deferred to SQLite's own busy-handler mechanism (`busy_timeout`) -- SanDBox never implements its own locking, queueing, or arbitration between connections.

**The `cmd/san-db-ox` REPL itself also follows this dedicated-connection principle.** Even though the REPL is the process's own sole client, running `BEGIN` on `db.Exec`/`db.Query` (a disposable connection borrowed on the spot from `DB`'s internal connection pool) doesn't guarantee the following `INSERT`/`COMMIT` land on that same connection -- `database/sql`'s connection pool can return a different connection per call, so several statements that look like one continuous stretch of SQL can actually end up spread across separate SQLite connections. `ResetSession` (called when the pool reuses a connection) doesn't roll back an open transaction (see the implementation note in §11), so this doesn't just fail to work -- it leaves a tainted connection behind in the pool. **The REPL opens one `db.Session(ctx)` at startup, holds it until the process exits, and runs every SQL statement through this dedicated connection.** `.snapshot`/`.overwrite`/`.load` (operations that replace or persist the whole DB) call `*DB` directly rather than going through `Session` -- these are operations on the DB's entire lifecycle, independent of any individual Session's transaction state.

Note that the exclusion `engine.DB` holds at the Go level (a `sync.RWMutex`) exists to protect the DB's own lifecycle/metadata -- swapping connections, `Close`, the information `Info()` returns -- not to control concurrency of SQL execution itself (the two are clearly separate layers, and this doesn't contradict the principle above).

---

## 3. REPL command set

The interactive console (REPL) can run dedicated control commands ("dot commands") starting with `.`, in addition to SQL statements. The command set follows the SQLite CLI (`sqlite3`), picking and choosing to fit SanDBox's single-in-memory-DB concept.

**Dot commands work the same way in batch execution mode (§5).** The stdio protocol (§7) exposes each as a corresponding `op`.

### Basic commands adopted (following SQLite)

| Command | Role |
| :--- | :--- |
| `.tables` | Show the table list |
| `.schema [table]` | Show CREATE statements (schema) |
| `.exit [CODE]` / `.quit [CODE]` | Exit (no auto-save; exits immediately with no save-confirmation prompt either -- see §4. `CODE` omitted means a clean exit; given, the process exits immediately with that code) |
| `.help` | Show the command list |
| `.headers on\|off` | Whether to show column names in results |
| `.mode MODE` | Output format. Only 5 are adopted: `list` (default, `\|`-separated, no header) / `column` (aligned column widths, auto-turns `.headers` on when switched to) / `csv` (RFC 4180, CRLF) / `json` (array) / `line` (one column per line) (decorative modes like `quote`/`insert`/`tabs`/`markdown`/`box`/`html` are not adopted). Value representation in JSON output is shared with §7. See the table below for its interaction with `.headers` |
| `.import FILE TABLE` | Load a CSV file into a table. **Always reads as CSV regardless of the `.mode` setting** (`sqlite3` follows `.mode`, but this is deliberately simplified to "`.import` is always CSV" -- an intentional difference). If `TABLE` doesn't exist, it's `CREATE`d automatically with the first row as all-TEXT column names; if it exists, the first row is treated as data. **If a row's field count doesn't match the column count, the whole operation aborts with an error naming that row, inserting nothing** (`sqlite3` instead pads/truncates with a warning and continues, but for the primary use case of loading seed data, silently inserting distorted data was judged more harmful -- an intentional difference) |
| `.dump [PATTERN]` | Dumps, as SQL statements, the schema and data of tables matching `PATTERN` (a SQL LIKE pattern; all tables if omitted), along with any indexes/views/triggers belonging to them. Literalizing values is deferred to SQLite's own `quote()` function |

**Interaction between `.mode` and `.headers`** (following the `sqlite3` CLI):

| mode | How `.headers` takes effect |
| :--- | :--- |
| `list` / `csv` | Follows the `.headers` setting (off by default) |
| `column` | Switching to `.mode column` turns `.headers` on automatically (as stated in the `.mode MODE` row above). It can then be individually disabled with `.headers off` |
| `json` / `line` | Ignores the `.headers` value, always including column names (since the output format itself embeds them) |

**A SQL statement is terminated by `;`** (following the `sqlite3` CLI). Until `;` appears, input is considered to be mid-statement, and the continuation prompt (§0's notation-slot table) waits for the next line. **When multiple SQL statements are typed on one line** (e.g. `SELECT 1; SELECT 2;`), each is shown as its own separate result set. The statement boundary (which `;` actually ends a statement) is decided by `Complete`'s judgment (see §11), not by a homemade scanner, so as not to mistake a `;` inside a string literal, a comment, or the `BEGIN...END` body of a `CREATE TRIGGER` (such as the `END` of a `CASE ... END` expression) for the end of the statement.

### Commands SanDBox adds on its own

| Command | Role |
| :--- | :--- |
| `.snapshot [FILENAME] [--sqlite] [--timestamp]` | Save a snapshot (see §4, §6, §12). By default produces a differently-named **executable**; with `--sqlite`, produces a **SQLite file**. With `--timestamp`, appends a timestamp to the filename |
| `.overwrite` | Overwrites its own executable with the latest data, then exits the process on success (see §4, §11) |
| `.load <FILE>` | Loads **only the data** from another SanDBox executable, **or a SQLite file**, replacing the in-memory DB state (produces no file; see §4, §6) |

### Commands not adopted

Since SanDBox always deals with a single, simple in-memory DB, these SQLite-derived commands are not adopted.

* `.open` / `.databases` (no concept of switching between multiple DBs)
* `.backup` / `.restore` (overlaps with `.snapshot`'s role)
* `.session` (change-tracking session feature; too much for what's needed, so out of scope)

---

## 4. Persistence specification (Snapshot Mechanism)

Data is retained by **"producing a new executable (under a different name)."**

* **When it happens:**
  * **Only on an explicit `.snapshot` command:** a snapshot is produced only when manually instructed.
  * **No automatic save on process stop:** if SIGTERM, or Ctrl+C (SIGINT) actually terminating the process (the detailed interactive-mode Ctrl+C behavior is in §13; it doesn't always exit immediately) occurs, the in-memory data is not saved and is simply gone. SanDBox treats itself as a volatile in-memory DB by design and consolidates persistence into an explicit user action alone, to avoid both the risk of corruption from a write interrupted mid-shutdown and the unbounded proliferation of automatic snapshots.
* **Filename convention:**
  * By default, automatically generates an executable with a timestamp appended (e.g. `san-db-ox_20260831_143000`).
  * Also supports outputting under an arbitrary filename, or overwriting an existing file, via a command-line argument or an interactive-console instruction.
* **Managing snapshot files:** since each `.snapshot` produces a fresh full copy of both the engine and the data, the number of files and the disk space used both grow with every run. Cleaning up (deleting) old snapshots is out of scope for the spec and left to the operator's (the user's) judgment.

### The exception of self-overwrite (`.overwrite`)

`.overwrite` is, like `.snapshot`, a save triggered by an explicit user action, but it differs in that it **exits the process at the same time it saves**. It is the one operation that deliberately deviates from the "no automatic save on process stop" principle above, allowed on purpose to give a learning tool or lightweight CLI tool the intuitive "edit it and just close it" workflow. See §11 for the write method (the steps that avoid overwriting the file currently running).

**It can also run in batch execution and the stdio protocol.** In batch execution, `san-db-ox -c ".overwrite"` is useful for the automation use case of "load data, embed it, and finish the build." In the stdio protocol it's provided as the `overwrite` op, and **returns one response line before exiting the process** (the same treatment as the `close` op). Avoid using it over `socat`, though (multiple child processes would end up writing to the same path at once; see §8). It is rejected when `--read-only` is given (§2).

### Loading data from another file (`.load`)

`.load <FILE>` loads **only the data contained in a separate file** and **replaces the currently running process's in-memory DB state with it**. It produces no file (it only changes the in-memory state).

**The file kind is determined automatically from its contents** (see §6). Two kinds can be loaded:

1. **A SanDBox executable** (identified by the trailing footer's magic)
2. **A SQLite file** (identified by the 16-byte header string `SQLite format 3\0`)

The primary use cases are moving data between SanDBox files built for different OSes/architectures (e.g. loading data created on Linux into an empty Windows binary before distributing it) and importing an existing SQLite database.

* **Behavior:** reads only the data portion out of the file and loads it into the SQLite engine (internal implementation in §11), replacing the currently running process's in-memory DB state. Whatever data the currently running process originally held is **completely replaced** (never merged).
* **A SQLite file in WAL mode:** when `.load` opens the source SQLite file, it lets SQLite itself open the file through its own normal procedure (see §11), so even commits that exist only in the `-wal` sidecar (not yet checkpointed into the main file, under `journal_mode=WAL` operation) are correctly loaded. As a side effect, the source file is **opened for read-write** (once `.load` finishes, closing the connection may checkpoint the WAL and clean up the `-wal`/`-shm` sidecars). A SQLite file on a read-only file or mount cannot be `.load`ed.
* **Effect on the file:** `.load` itself produces no file. To save the loaded state as a file, follow it with `.snapshot` (save under a new name) or `.overwrite` (overwrite itself). The typical flow for moving data to a binary for a different OS is: start an empty binary for the target OS → load the data into memory with `.load <FILE>` → write it into that binary itself with `.overwrite`.
* **The engine portion is untouched:** `.load` never references the source file's engine portion at all. It always loads only the data, using the currently running process's own engine. The engine portion of a binary built for a different OS can never be loaded directly.
* **Effect on existing sessions:** since `.load` replaces the in-memory DB by overwriting the live database (see §11), any other session (`engine.Session`) already open at that point keeps its connection without losing it, and any statement issued afterward simply sees the replaced data.
* **A file that can't be loaded:** if the file is neither a SanDBox executable nor a SQLite file, or is a SanDBox file with a data length of 0, it's an error and the currently running process's in-memory state is left unchanged. If the load itself fails partway through (a disk I/O error, say), the standard SQLite online Backup API copies as a single write transaction, so on failure that transaction is rolled back and the in-memory state is unchanged (see §11).
* **Handling a version mismatch:** if the source file's footer (the `Version` field; see §11 -- the format version of the data blob, a separate value from SanDBox-the-software's own version, e.g. a Git tag) differs from the `Version` the currently running process would write to its own footer, a warning is shown and processing continues (it is not rejected). Showing this warning is `cmd/san-db-ox`'s (the caller's) responsibility -- the `engine` package itself never logs anything (§10's division of responsibility: the library doesn't write to stderr). `engine` provides `Inspect(path)`, an API that reads only the file kind and footer information, so `cmd/san-db-ox` calls it before running `.load` and prints its own message if it detects a mismatch.
* **Relation to the general principle:** like `.snapshot`/`.overwrite`, `.load` too runs only on an explicit user action. There is no mechanism that automatically loads another file's data.

---

## 5. Non-interactive run mode (batch execution)

To let CI/CD, shell scripts, Makefiles, and the like use SanDBox as a "run it once, get a result" tool, a run path that never starts the REPL is provided, following the `sqlite3` CLI's equivalent feature.

### Forms of invocation

| Form | Behavior |
| :--- | :--- |
| `san-db-ox -c "SQL"` | Runs the given SQL and exits. `-c` can be given multiple times and runs in the order given |
| `san-db-ox < script.sql` | When stdin isn't a terminal, reads the whole of stdin as a SQL script, runs it, and exits |
| `san-db-ox -c "SQL" < input` | When `-c` is given, stdin is never read as a script (leaving room for `.import` and the like to use stdin) |

In every form, no banner is shown (equivalent to `-q`) and no prompt is shown. It never transitions into the REPL.

### Execution and output

* Both SQL statements and dot commands can be run. `.snapshot` / `.load` work as-is too.
* Results go to stdout, formatted per `--mode` (`-m`); the default is `list`. **`--mode json` is recommended when reading the output mechanically from a script.**
* Logs, warnings, and error messages go to stderr (§0).
* **On error, the entire run aborts immediately.** No later SQL statement runs (aligned with `.import`'s own design philosophy -- avoiding a mid-run failure being silently swallowed in CI). If a dot command requests termination via `.exit`/`.quit`, the same applies: processing is cut off at that point, without running the remaining `-c` arguments or the rest of stdin.
* **Each `-c` argument, and the stdin script as a whole, is treated as its own independent unit of execution.** When `-c` is given multiple times, the next `-c`'s content is never concatenated onto the previous one as a continuation -- at the end of each argument's text, if the not-yet-run remainder contains anything besides whitespace, a single `;` is appended before retrying the completeness check (`Complete`; see §11). (This mirrors how the `sqlite3` CLI's `-cmd` argument doesn't require a trailing `;`, and keeps this consistent with `docs/usage/cli-options_ja.md`'s multiple-`-c` examples, none of which append `;`.) **Multiple statements separated within a single argument** (as in `SELECT 1; SELECT 2`) are still decided by `Complete`, as before. If it's still judged incomplete even then (an unclosed string literal, say), it's treated as a syntax error and aborts with exit code 1. For a stdin script, reaching EOF (the end of that one execution unit -- the whole script) gets the same treatment.

### Exit codes

| Code | Meaning |
| :--- | :--- |
| `0` | Every statement ran successfully |
| `1` | A SQL execution error, or a dot command failed |
| `2` | A usage error (a bad flag, a file that couldn't be opened, etc) |

An explicit `.exit CODE` / `.quit CODE` overrides this.

### Relation to periodic snapshots

`--snapshot-interval` (§12) is **disabled** in batch execution mode, since the process is short-lived and it would have no meaning. If given, a warning is printed to stderr and it is ignored.

---

## 6. SQLite file import/export

### No data-file format of its own exists

The only non-executable file SanDBox deals with is **a plain SQLite file**. It defines no data-file format of its own.

This is also a natural consequence of the implementation. The byte sequence `sqlite3_serialize()` returns (adopted in §11) is identical to the image you'd get by writing that database out to disk as a file, so **the "data blob" embedded at the tail of the executable is, as-is, the contents of a SQLite file.** So "slicing out the data blob and writing it to a file" and "exporting as a SQLite file" turn out to be the same operation.

> **Confirmed by measurement (Phase ① Step 2):** this equivalence (that `Serialize()`'s output opens as-is as a valid SQLite file) was confirmed by measurement against `modernc.org/sqlite v1.58.0`. The byte sequence `Serialize()` returned was written out to a plain file and opened with an independent `sql.Open` (a normal file-path DSN, not `file:`) that never went through `vfs=memdb`, and `SELECT` was confirmed to work. The first 16 bytes matching `SQLite format 3\0` was also confirmed.

As a consequence, only two file formats are ever dealt with:

| Format | How it's identified | Extension | Command that produces it |
| :--- | :--- | :--- | :--- |
| SanDBox executable | The trailing 32-byte footer's magic | None (`.exe` on Windows) | `.snapshot` / `.overwrite` |
| SQLite file | The leading 16 bytes, `SQLite format 3\0` | `.db` / `.sqlite` | `.snapshot --sqlite` |

### `.snapshot --sqlite <FILE>` -- exporting

Writes out the current in-memory DB state as a SQLite database file. `.snapshot` produces an executable by default, but adding `--sqlite` produces a SQLite file instead. **Output format selection is consolidated into this one flag; no dedicated export command is provided** (mirroring how `.load` on the input side accepts both formats through a single command).

* The exported file **opens as-is** in GUI tools like DBeaver / TablePlus / DB Browser for SQLite, any language's standard sqlite driver, the `sqlite3` CLI, pandas, and the rest.
* The implementation calls §11's standard SQLite online Backup API (`modernc.org/sqlite`'s `NewBackup`) with a file DB as the destination. Writing `Serialize()`'s output directly is also possible, but the Backup API is chosen instead because it guarantees consistency with any in-flight write transaction.
* Writing uses the atomic approach of writing to a temp file in the same directory, then `rename`ing it (the same as snapshot writing in §11).
* If the file already exists, it's overwritten.
* **Permission is `0644`.** Different from `.snapshot`'s (producing an executable) `0755` -- a SQLite file isn't an executable.
* **If the extension is omitted, `.sqlite` is appended.** The `.exe` completion on Windows (§12) does not apply with `--sqlite` given (it isn't an executable).
* Can be combined with `--timestamp` (§12).
* `--sqlite` cannot be applied to `.overwrite` (turning itself into a SQLite file would make it unable to run).

### `.load <FILE>` -- importing

As in §4, `.load` determines the kind automatically from the file's contents. The order of determination:

1. If the leading 16 bytes are `SQLite format 3\0`, treat it as a **SQLite file**
2. If the trailing 32-byte footer's magic is found, treat it as a **SanDBox executable**
3. If neither applies, it's an error (the in-memory state is left unchanged)

Importing a SQLite file, like importing a SanDBox file, copies into the "live" in-memory DB via the Backup API, so an existing session never loses its connection. However the implementation path differs depending on whether the source is a SQLite file or a SanDBox executable (see §11) -- **a SQLite file uses the standard SQLite online Backup API in the direction that reads from the file side** (calling `sqlite3_backup_init` with the source as the file and the destination as the live in-memory DB), whereas **a data blob embedded in a SanDBox executable is loaded through the two-step `Deserialize` + Backup API approach** (see §11's "Data blob serialization method"). The former is chosen because letting SQLite open the file through its normal procedure avoids missing the WAL sidecar mentioned above.

**Known limitation:** the source file path cannot contain `?` (a limitation of the internal DSN format -- it would be interpreted as a query-string separator in the path).

### Connecting to the existing ecosystem

`.snapshot --sqlite` is the primary channel for connecting to existing tooling assets without implementing a network protocol. Any software that can read SQLite -- GUI clients, BI tools, data-analysis libraries, and the rest -- becomes, as-is, a way to view SanDBox's data. It has an advantage over a custom protocol or a wire-protocol-compatible implementation in that it costs nothing at all to keep protocol compatibility.

Only when you need to connect live and read/write, use §7's stdio protocol.

---

## 7. stdio protocol (external integration)

The mechanism for using SanDBox from another program. **SanDBox is launched as a child process, and talks over standard input/output.**

Since no network is involved, this lets any language integrate while keeping §1's Zero-Network and §10's design principles intact. C, Python, Ruby, Node.js, PHP, Rust -- anything that can handle a subprocess and pipes **connects directly, with no dedicated library needed** (client drivers for each language are provided by a separate project as thin wrappers built on top of this contract; see the end of this section).

### Startup and basic flow

```
san-db-ox --serve-stdio
```

* No banner, no prompt at all. It never transitions into the REPL.
* **stdout carries only this protocol's responses.** Logs and diagnostics go only to stderr (§0).
* **A session's lifetime is the process's lifetime.** Transactions are naturally maintained for the life of the process. It holds onto a single `engine.Session` throughout.
* Exits the process once stdin reaches EOF (nothing is saved, following §4's principle).
* **Right after connecting, one line (the hello line) is printed before waiting for the first request.** It lets the client confirm the protocol version, and is the one line that answers no request.

```json
{"protocol":1,"version":"v0.1.0","product":"SanDBox"}
```

`protocol` is this JSON Lines protocol's own version number (an integer; `1` in the first version), a separate lineage from the footer's saved-format version (§11's `Version`). Clients must always skip (or otherwise handle) this line.

### Message format

**Line-delimited JSON (JSON Lines)** is adopted. One line, one message. Simpler to implement than a length-prefix scheme, and any language can handle it just by "reading a line."

Request (client → SanDBox):

```json
{"op":"query","sql":"SELECT id, name FROM users WHERE id = ?","params":[1]}
```

Response (SanDBox → client):

```json
{"ok":true,"columns":["id","name"],"rows":[[1,"alice"]]}
```

* If a request includes an `id` field, it's echoed back verbatim in the matching response (optional). **Responses always come back in the same order as their requests, so matching is possible even without `id`.**
* Even if a request is malformed JSON, an error response is returned in one line and processing continues (the connection isn't closed).

### Op list

| op | Parameters | Response |
| :--- | :--- | :--- |
| `query` | `sql`, `params` | `columns`, `rows` |
| `exec` | `sql`, `params` | `rows_affected`, `last_insert_id` |
| `snapshot` | `filename` (optional), `sqlite` (bool, optional), `timestamp` (bool, optional) | `path` (the path of the file produced) |
| `load` | `path` | -- |
| `inspect` | -- | this process's own state (below) |
| `tables` | -- | array of table names |
| `schema` | `table` (optional) | CREATE statements |
| `dump` | `pattern` (optional) | `sql` (the full SQL text `.dump PATTERN` would produce; see §3) |
| `overwrite` | -- | overwrites its own executable, **returns one response line, then exits the process** (§4) |
| `close` | -- | the process exits normally after responding |

There is no dedicated op for `BEGIN` / `COMMIT` / `ROLLBACK` -- send them through `exec` as-is. DDL runs through `exec` too (§2, no access control).

**There is no op corresponding to `.import`.** `.import FILE TABLE` reads a CSV file from an arbitrary path on the server side -- the same concern that keeps `inspect(path)` from being exposed as an op (end of this section) applies -- a way to probe or read the server's filesystem when exposed externally (§8) -- so no corresponding op is deliberately provided. To load CSV data, parse it on the client side and send repeated `exec` calls with `INSERT`.

`query` returns every row of the result set in a single response. There is no cursor mechanism for fetching rows incrementally.

**`inspect` reports only this process's own state.** In REPL mode, the startup banner (§13) presents the same information, but since stdio mode has no banner, this is the only way a connected driver can learn the process's state.

```json
{"op":"inspect"}
→ {"ok":true,"has_data":true,"version":1,"data_length":1048576,"source":"mydb_20260901120000","read_only":false}
```

| Field | Contents |
| :--- | :--- |
| `has_data` | Whether data was embedded at startup (`false` means it started from an empty binary) |
| `version` | The footer's format version (§11). `null` when `has_data` is `false` |
| `data_length` | The length (in bytes) of the data blob loaded at startup. Same condition |
| `source` | The filename of the executable that was started |
| `read_only` | Whether started with `--read-only`. Can be checked before attempting a write |

**`has_data`/`version`/`data_length` are facts as of startup, and never change even after running `.load` afterward.** `engine`'s `DB.HasData()` behaves the same way (unaffected by `.load`) -- `.load` replaces the in-memory DB's content, but that's a separate matter from "which file this process was started from," a fact fixed at startup.

**There is no facility for inspecting an arbitrary path.** `engine.Inspect(path)` is used internally for the version-mismatch warning before `.load` runs (§4), but exposing it as an op would give a way to probe the server's filesystem when published externally (§8). Whether a file can be `.load`ed is discoverable by running `load` and looking at the error.

### Error responses

```json
{"ok":false,"error":{"code":"sqlite_error","message":"no such table: users"}}
```

* `code` is a simplified identifier (`sqlite_error` / `bad_request` / `io_error` / `unsupported_op` / `read_only`, etc), with no elaborate scheme like SQLSTATE.
* The process does not exit on an error. The client can keep sending further requests. This is the opposite of batch execution (§5), which aborts immediately -- but that's because this is an interactive, continuing session.

**How `code` is assigned:** `sqlite_error` when `query`/`exec` fails to execute the SQL itself. `io_error` when `snapshot`/`load`/`overwrite`/`dump` fails for a file-I/O- or path-related reason (no write permission, a corrupted source file, etc). `bad_request` for malformed JSON, an unknown field, a value that violates §7's "JSON representation of values" rules (an array with a length other than 1, say), or a missing required parameter. `unsupported_op` for an unknown `op`. `read_only` for a rejection due to `--read-only`. When a single failure could fit more than one category (e.g. a file given to `load` that's recognized as neither a SQLite file nor an executable), **the category closest to the invoked op's primary purpose wins** (`load` is a file I/O operation, so it gets `io_error`).

### JSON representation of values

**Uses JSON's types as-is, wrapping only BLOB -- the one type with no JSON equivalent -- in a single-element array.**

| SQLite value | JSON representation | Example |
| :--- | :--- | :--- |
| NULL | `null` | `null` |
| INTEGER | number | `42` |
| REAL | number | `88.5` |
| TEXT | string | `"alice"` |
| BLOB | single-element array | `["iVBORw0KGgo="]` |

The rule fits in one line. **If the value is a JSON array, it's a BLOB; otherwise the JSON type is the SQLite type as-is.**

```json
{"ok":true,"columns":["id","name","score","avatar","note"],"rows":[[1,"alice",88.5,["iVBORw0KGgo="],null]]}
```

* A BLOB's contents are encoded in **Base64** (RFC 4648 standard alphabet, with padding, no line breaks). An empty BLOB is `[""]`.
* **An array with a length other than 1 is a protocol violation** (returns `bad_request`).
* **`params` accepts the same representation.** This keeps the round trip symmetric, so a value read out can be written straight back in.
  ```json
  {"op":"exec","sql":"INSERT INTO profiles VALUES (?, ?)","params":[1,["iVBORw0KGgo="]]}
  ```
* **A REAL is always output with a decimal point** (`88.0`, not `88`). JSON's numeric syntax makes no distinction between integers and decimals, so skipping this would let a REAL be read back as an INTEGER. This isn't perfect either (JavaScript holds `88.0` as `88`).
* Type information is never returned in a separate array (`types`, say). It's unnecessary since the value itself is self-descriptive, and this representation doesn't break down when a column's type varies within a UNION and the like.

This representation follows the same rules as §3's `.mode json` (the two never have differing representations).

**Known limitation:** integers up to 2^53 - 1 are exactly representable as JSON numbers, which doesn't cover SQLite's full 64-bit INTEGER range. An integer beyond 2^53 loses precision in a runtime with no native 64-bit integer type, like JavaScript (silently drifting, with no error). Care is needed when `.load`ing an existing database that contains distributed IDs or nanosecond-precision timestamps. A driver in a language that does handle 64-bit integers (Go, Rust, Java, C#, ...) should avoid decoding JSON numbers into a double-precision float, and instead read the numeric token as a string before parsing it as an integer (Go's `json.Number`, Rust's `serde_json` arbitrary-precision feature). That preserves the value even though the server side emits it as a plain number.

**Representing a REAL that becomes NaN/±Inf:** JSON's numeric syntax has no representation for NaN/Infinity, and a standard JSON encoder errors out if handed one. SanDBox follows the `sqlite3` CLI's convention and approximates with **a numeric literal every JSON parser can accept** -- `+Inf` becomes `9e999`, `-Inf` becomes `-9e999` (numeric literals beyond IEEE 754 double precision's range, which round to ±Inf on decoding). **NaN has no equivalent literal, so it's output as `null`** (indistinguishable from NULL in this representation, but adopted as the closest thing in JSON's vocabulary to expressing "this value cannot be represented").

### Notes for implementers (a contract for both the core and drivers)

* **Flush after writing every line.** The core must flush after writing each response, and a client must flush after sending each request. Pipes are buffered, so skipping this leaves both sides waiting forever for the other's input. When using Go's `bufio.Writer`, call `Flush()` after every request.
* **Never leave stderr unread.** If a client leaves the child process's stderr piped but unread, the process blocks once the pipe buffer fills up with logs. Route it to `/dev/null` if you don't want it.
* **EOF means the connection is gone.** If the child process dies abnormally, the client's line reads return EOF. A driver must detect it and reap the exit code (skip this and a zombie process is left behind).
* **Exit by closing stdin.** The child process exits on its own on either the `close` op or stdin being closed. A driver's `close()` should ideally close stdin, wait a bit, then send SIGTERM if it's still alive.

### Client drivers

Client drivers for each language are **not included in this repository -- they're provided by a separate project, `san-db-ox-clients`**. This repository defines only up through this section's protocol specification.

This separation keeps the core "self-contained with just the Go toolchain": bringing in runtimes for multiple languages is never needed to build or test the core. The only connection test kept on the core side is a stdio client written in Go.

> Note that this protocol can be handled **directly, with no dedicated library**, by anything that can manage a subprocess and pipes. A driver is a thin convenience wrapper, not a prerequisite for using it.

---

## 8. How it's expected to be used

Although SanDBox never listens on a network, **it's designed to be used over a network by bolting on a transport with an external tool.** The canonical form pairs `socat`'s `EXEC` address with `--serve-stdio`, which makes a TCP/Unix-domain-socket service work **with zero implementation on SanDBox's side.** The decision and responsibility for opening that door rest entirely with the operator; SanDBox itself never imports `net` (§10).

**This usage pattern imposes two premises on design, implementation, and testing.**

**1. Each connection gets its own process, and its own database.** This isn't a mechanism of SanDBox's -- it's the behavior of `socat`'s `fork` option (`socat` given `fork` forks itself each time it accepts a connection, and execs the `EXEC:` command on the child side; a classic pattern going back to `inetd`, the same regardless of what's on the other end). Since SanDBox is an in-memory DB and **process = session** (§7), separate processes mean separate databases. A's INSERT is invisible to B, and A's `snapshot` result is never reflected in B's memory.

Dropping `fork` doesn't fix this either -- there's then only one child process and one database, but once that connection drops, `socat` itself exits and can't wait for the next one. In other words, **"keep one database running and serve multiple connections to it in turn" is simply not a shape this bolt-on-transport approach can produce.** This is an unavoidable limitation; if sharing is needed, the user has to write their own long-running app that launches and holds exactly one child process (that app is then the one providing the network side).

**2. Multiple processes from the same binary start at the same time.** As a consequence of the above, it's an everyday occurrence for multiple processes to load the same executable simultaneously. That footer reads don't race, and that `.snapshot` run concurrently against the same path never leaves a corrupted file observable (§11's atomic writing), both need to hold under this premise. **Testing must cover this situation too.**

Also worth noting: there is no authentication at all when published externally. Anyone who can reach it can run any SQL, DDL included, so it's meant to be paired with `--read-only` (§2). **The specific connection method and security configuration are out of scope for this document** and are covered in `san-db-ox-clients`'s documentation instead.

### Docker

```dockerfile
FROM scratch
COPY san-db-ox /san-db-ox
ENTRYPOINT ["/san-db-ox", "--serve-stdio"]
```

Being a single binary, this works with `FROM scratch`. To persist logs, either redirect with `2> /path/to/log` or hand it off to Docker/systemd's existing logging setup (§0).

---

## 9. Primary use cases (Use Cases)

1. **Supercharging CI/CD and E2E tests (Instant Test DB)**
   * Drop one binary with the schema built and seed data loaded, and a test environment comes up in memory in under a second, with no cleanup needed afterward.
   * Verify it straight from a shell script or Makefile, using batch execution via `-c` / stdin (§5) and the exit code.
2. **Reproducing and sharing bugs "with the data state attached" (Executable Snapshots)**
   * Turn the data state that triggered a bug into a binary with `.snapshot bug_123` and share it. Whoever receives it just runs it to instantly reproduce the exact same DB environment.
   * If the recipient is on a different OS, start an empty binary for that OS, load the data into memory with `.load`, then run `.overwrite` -- this rebuilds the same data as an executable for that OS (see §4).
3. **Embedding in another program (Embeddable via stdio)**
   * Launch it as a child process from any language and talk over JSON Lines (§7). No network setup, auth setup, or daemon management needed at all.
   * Client drivers for each language are provided by a separate project (§7).
4. **A temporary read-only demo (Read-only Demo)**
   * Bolt on a transport with `socat` (§8), and lock out writes with `--read-only`, to stand up a read-only demo environment with one binary and one line. Hand someone a binary with sample data embedded and say "run this and connect."
   * **Each connection still gets its own independent database** (§8), though -- this can't be used for multiple people sharing and editing the same data together. There's no authentication either, so restrict who can reach it with `bind=127.0.0.1` or an SSH tunnel. Both of these points must always be stated together in the README too.
5. **Portable state management for edge/CLI tools (Portable Data Capsules)**
   * Achieves fast processing and file-level snapshot saving in a single executable, with no dependency on an external DB.
6. **Zero-setup SQL learning and hands-on practice (Zero-Setup SQL Sandbox)**
   * Learn and experiment with full-featured SQL (views, indexes, triggers, transactions) instantly, with no install or auth setup -- just run the binary.
   * Works the same on Windows, macOS, and Linux -- double-click the executable (or launch it from a terminal) and the REPL comes straight up, needing no extra setup like WSL2, so even a beginner can try it immediately.
7. **Bridging to existing SQLite assets (SQLite Interop)**
   * Produce a SQLite file with `.snapshot --sqlite` and hand it to an existing ecosystem -- GUI tools, pandas, and the rest. Import an existing SQLite database with `.load` (§6).

---

## 10. Library architecture

SanDBox is centered on **a library for an in-memory SQL engine (the `engine` package)**; the standalone executable is positioned as "one reference implementation" built using that `engine`.

### Layer structure

```
san-db-ox/
├── engine/              ← [Library core] the in-memory SQL engine
│                            (a modernc.org/sqlite wrapper, a Go-function-call
│                             API, the persistence interface, and Overwrite --
│                             which rewrites its own executable)
│
└── cmd/san-db-ox/       ← [App] the single-binary RDBMS built on engine
                             (REPL, batch execution, stdio protocol)
```

The concrete file layout (`engine.go`, `persist.go`, `main.go`, `repl.go`, `stdio.go`, etc) is decided during implementation. This shows only the division of responsibility between the two directories.

### Division of responsibility

| | `engine` (the library) | `cmd/san-db-ox` (the app) |
| :--- | :--- | :--- |
| SQL execution | Provides Go function calls (one-shot `db.Query()`/`db.Exec()`, and `db.Session(ctx)` for an independent client unit spanning `BEGIN`/`COMMIT`) | Just calls `engine`'s API |
| Network I/F | **Has none** (no direct dependency on `net`/`net/http`) | **Has none** (same as the library; external exposure is done by the operator bolting on a transport; §8) |
| Access control | None | None (§2) |
| Persistence | General-purpose read/write (an arbitrary file, `io.Writer`/`io.Reader`), plus overwriting its own executable (`Overwrite`), plus exporting to a SQLite file (`Export`) | Just calls it from `.snapshot` / `.overwrite` / `.load` (§4, §6) |
| Logging | **Does none** (errors are returned as values) | Writes to stderr (§0) |
| Purpose | An embedded DB layer for other Go apps (a substitute for a sqlite file) | CI/CD testing, integration from other languages, portable distribution |

### Design principle: safety from having no network I/F

The `engine` package is reachable only through Go function calls. Since it implements no network-based I/O, when embedded in another Go app, **no external communication path exists at all unless that app itself explicitly creates one.** This is safety from "there structurally is no way in," not from "defensive logic that blocks it," and not importing `net`/`net/http` is verifiable at the code level.

**This same principle applies equally to `cmd/san-db-ox`.** The product as a whole never listens on a network. CI verifies both that `net/http` never appears even transitively, and that `net` is never imported directly.

Note that `net` does appear as a transitive dependency (via `modernc.org/libc`, which the internal SQL engine `modernc.org/sqlite` uses). This is purely a consequence of the SQLite driver's own implementation and doesn't mean it carries code that performs network I/O.

### Primary API

```go
// Opening (kind auto-detected -- reads either a SQLite file or a SanDBox executable; see §6)
func Open(path string) (*DB, error)
func OpenSelf() (*DB, error)            // the data embedded in its own executable

// Persistence
func (db *DB) Snapshot(path string) error  // produce a differently-named executable
func (db *DB) Overwrite() error            // overwrite its own executable (the process does not exit)
func (db *DB) Export(path string) error    // write out as a SQLite file
func (db *DB) Load(path string) error      // load only the data from another file (kind auto-detected)
func (db *DB) LoadFrom(r io.Reader) error  // Load's io.Reader version (see "The difference between Load and LoadFrom" below)

// Inspection (get only footer information and kind, without opening the file)
func Inspect(path string) (*FileInfo, error)

// Session
func (db *DB) Session(ctx context.Context) (*Session, error)
func (db *DB) Close() error

// Completeness check for multi-line input (used by the REPL's internal implementation; callable without opening a DB)
func Complete(sql string) (bool, error)
```

**What's not exposed:** an API for registering a user-defined scalar function (a wrapper around `modernc.org/sqlite`'s custom-function registration mechanism) is not exposed, to keep the API surface minimal. If it's ever needed, register it directly with the driver on the host app's side.

**When to use `Snapshot` versus `Export`:** the CLI consolidates this into the single `.snapshot --sqlite` phrase, but **the Go library keeps the two as separate methods** (a boolean argument like `Snapshot(path, sqlite bool)` reads poorly at the call site; the CLI's command set doesn't need to map 1:1 to the library's API). `Snapshot` produces an **executable** made of "engine bytes + data + footer." When embedded as a library, the "engine bytes" here are the host app's entire binary, so what's produced is a copy of the host app. Use `Export` if you want to leave just the data in a separate file. This distinction doesn't depend on whether the DB was opened with `Open` or `OpenSelf` (`Snapshot` always produces an executable, `Export` always produces a SQLite file).

#### `Inspect` and `FileInfo`

`Inspect(path)` never opens `path` as a database -- it reads only the kind and footer information. It's meant to be used by `cmd/san-db-ox` for the version-mismatch warning before running `.load` (§4), and any tool is free to use it for determining a file's kind.

```go
type FileKind int

const (
    KindUnknown    FileKind = iota // neither of the two formats engine can read
    KindSQLite                     // a plain SQLite database file
    KindExecutable                 // a SanDBox executable (trailing footer)
)

type FileInfo struct {
    Kind       FileKind
    Size       int64  // the size of the whole file, in bytes
    HasData    bool   // DataLength > 0
    Version    uint32 // the footer's format version. Only meaningful for KindExecutable (0 otherwise)
    DataOffset int64  // the start offset of the data blob. Always 0 for KindSQLite
    DataLength int64  // the length of the data blob. Always equal to Size for KindSQLite
}
```

The order of determination is as in §6 (the leading 16-byte SQLite header takes priority, then the trailing 32-byte footer's magic). An error is returned only when the footer's magic matches but something like the offset is internally inconsistent. Otherwise (no header and no footer, the file too short, 0 bytes, etc), it simply returns `KindUnknown` with no error -- deciding "whether it can be loaded" is the caller's (`Load`'s, etc) job.

#### `OpenSelf`

Gets its own path via `os.Executable()`, then loads the data from the trailing footer. It also does a best-effort deletion, at startup, of the `.san-db-ox.old` backup file (see §11) a previous `Overwrite` may have left behind.

#### `Overwrite`

Overwrites the calling process's own executable with content that embeds the current DB state. See §11 for the write method. On success, the file's contents are replaced, but **the calling process itself does not exit** (whether to exit is left to the caller's own judgment).

`engine` never makes the host app aware of "where the engine ends and the data begins in itself." The entire host app is treated as-is as "the engine portion," with the data and footer appended after it.

**Usage notes:**
* If the host app is installed in a directory that requires write permission, such as `/usr/bin/` or `C:\Program Files\`, `Overwrite` fails with a permission error.
* If the host app has its own separate self-update mechanism (Squirrel/Sparkle, etc), the timing of replacing the executable could conflict.
* This API's intended scope is embedding in a Go app.

#### The difference between `Load` and `LoadFrom`

`Load(path)` loads a file on the filesystem (see §4, §6). `LoadFrom(r io.Reader)` is a general-purpose version of `Load` that loads a byte sequence from an arbitrary `io.Reader`.

**Both auto-detect the same two formats (a SQLite file / a SanDBox executable) in the same order, but there is one point where the results can diverge.** An `io.Reader` can't carry a side file outside the byte sequence handed to it, like a `-wal` sidecar file. So:

* `Load(path)` lets SQLite open the file through its normal procedure even if the SQLite file `path` points to is operating under `journal_mode=WAL`, correctly loading even a committed transaction that exists only in `-wal`.
* `LoadFrom(r)` judges based solely on the byte sequence read from `r`. **Passing the contents of a WAL-mode SQLite file (just the main file's bytes) to `LoadFrom` gets it explicitly rejected with an error** -- from the byte sequence alone, there's no way to tell whether an uncommitted-to-the-main-file transaction exists, and rather than silently overlooking it (effectively behaving as if it had already been checkpointed), it's safer to reject it and point toward using `Load` with a file path instead. Whether a file is in WAL mode can be determined from the SQLite file header's format-version byte (offsets 18, 19).

The asymmetry where `Load(path)` and `LoadFrom(os.Open(path))` on the same file can produce different results is the flip side of this constraint. Use `Load(path)` for the ordinary case of loading a file on disk. `LoadFrom` is meant for input that has no file path to begin with, such as a byte sequence received over a network.

#### `Session`

`db.Query()`/`db.Exec()` are one-shot calls that borrow a connection from the pool for a single statement and return it -- running `BEGIN` through one of these gives no guarantee that the following `COMMIT` reaches the same connection. Use `Session` for "a single independent client spans multiple statements in one transaction over the same connection."

```go
func (db *DB) Session(ctx context.Context) (*Session, error)

func (s *Session) Exec(query string, args ...any) (sql.Result, error)
func (s *Session) Query(query string, args ...any) (*sql.Rows, error)
func (s *Session) QueryRow(query string, args ...any) *sql.Row
// Each of the above has a *Context variant, and db.Exec and friends also have their own *Context variants.

// For a caller that runs the same statement repeatedly (bulk-loading CSV via .import, etc)
func (s *Session) Prepare(query string) (*sql.Stmt, error)
func (s *Session) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)

func (s *Session) Close() error  // idempotent
```

**A `Session` must be explicitly closed.** `DB.Close()` doesn't wait for an open `Session`, nor does it force one closed -- exactly the division of responsibility stated at the top of §10: `engine` just returns errors, it doesn't police lifecycles, so cleaning up after forgetting to close a `Session` before calling `DB.Close()` is the caller's responsibility.

**Relation to the persistence API:** the kind of transaction a `Session` is holding changes how `Snapshot`/`Overwrite`/`Export`/`Load` behave. Each of these waits up to `busy_timeout` before returning `ErrBusy`.

| `Session` state | `Snapshot` / `Overwrite` | `Export` | `Load` |
| :--- | :--- | :--- | :--- |
| No transaction | OK | OK | OK |
| Read transaction in progress | OK | OK | `ErrBusy` |
| Write transaction in progress | `ErrBusy` | `ErrBusy` | `ErrBusy` |

`Load` returns `ErrBusy` even during a read transaction because it needs exclusive access to the destination (the live DB) -- `Snapshot`/`Export` only read the live DB as the source, so they never conflict with another read transaction.

**Why the connection pool has no upper bound:** since `Session` never returns a connection while holding it exclusively, setting a cap on the connection pool (`SetMaxOpenConns`, etc) would cause a deadlock: "open `Session`s up to the cap, and every `Session()`/`db.Query()`/etc call after that blocks forever." So `engine` deliberately never caps the pool.

### Example usage as a library

```go
import "github.com/amisonnet8/san-db-ox/engine"

// Open a SQLite file (as a drop-in substitute for a plain sqlite file)
db, err := engine.Open("myapp.db")

// Ordinary SQL operations
db.Exec("CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)")
db.Query("SELECT * FROM users")

// Write the data out as a SQLite file
db.Export("myapp_backup.db")

// Produce a copy of the host app itself, as an executable carrying the current data
db.Snapshot("myapp_with_data")

// Overwrite the host app's own executable
db.Overwrite()

db.Close()
```

---

## 11. Architecture and technology choices

| Item | Content | Notes |
| :--- | :--- | :--- |
| **Development language** | **Go (Golang)** | Static binary generation, cross-compilation, multi-platform support. **CGO is not used** |
| **Target OS** | **Cross-platform (Linux / macOS / Windows)** | Every mode works identically across every OS. Container operation (a lightweight `FROM scratch` image, etc) prioritizes Linux. Distribution ships pre-built, empty (no embedded data) binaries for each OS/architecture |
| **Internal DB engine** | `modernc.org/sqlite` (a transpiled Pure-Go build of SQLite) | No CGO needed, so cross-compilation is easy; conforms to the standard `database/sql` interface. Guarantees full SQL compatibility -- views, indexes, triggers, transactions included -- cheaply, via a battle-tested implementation |
| **External integration** | stdio (JSON Lines) + SQLite files | See §6, §7. No network protocol is implemented |

### Binary embedding method (fixed-length footer)

Embedding data at the tail of the executable adopts the **fixed-length footer (trailer)** approach.

```
[The Go binary itself (the engine portion)] [The data blob (DB state)] [Footer (fixed 32 bytes)]
```

| Field | Size | Contents |
| :--- | :--- | :--- |
| Magic | 8 bytes | The identifier `"SANDBOX1"` |
| Version | 4 bytes | The format version (a **big-endian** unsigned integer). `1` in the first version |
| DataOffset | 8 bytes | The data blob's start offset (an absolute position from the start of the file, **big-endian**) |
| DataLength | 8 bytes | The data blob's length (**big-endian**) |
| Reserved | 4 bytes | Reserved for future extension |

Every integer field in the footer is encoded **big-endian**.

* **Reading:** get its own path with `os.Executable()`, then `Seek` to read 32 bytes from the end of the file -- that's all it takes to locate the footer. No ELF/Mach-O section-header parsing is needed.
* **Writing (running `.snapshot`, saving under a different name):** at startup, only its own engine portion's offset (0 through DataOffset, or the whole file's size if there's no trailing footer) is kept in memory, and the actual engine byte sequence is re-read right before saving (a binary including `modernc.org/sqlite` runs 10-15MB, so the byte sequence itself is not kept resident). At save time, a new snapshot is produced from "engine bytes + new data + new footer." No diffing or relinking is needed. Writing uses the atomic approach of writing to a temp file created in the same directory, then `rename`ing it, so a reader never observes a half-written, incomplete file. Also, since acquiring the data blob (`Serialize`, below) is done only after confirming no other session has a write transaction in flight, it always produces a consistent snapshot even while a write is happening concurrently.
  **An exception (confirmed by measurement on Windows CI in Phase ① Step 5):** when the destination path is identical to the running executable's own file, this temp-file-plus-`rename` approach **fails on Windows, with `rename` itself returning "access is denied"** (it succeeds on Linux/macOS). Since the default name used when `FILENAME` is omitted (based on the running binary's name; §12) inevitably falls into this situation whenever the CWD is the same as where the executable sits, this can happen often enough to matter. In that case it automatically switches to the same backup-based approach `.overwrite` uses (below) -- effectively, `.snapshot` ends up behaving as "an `.overwrite` that doesn't exit the process." Whether the destination is itself is determined **using OS-level file identity (`os.SameFile` -- volume + file index on Windows, device + inode on Unix) when both files actually exist.** Confirmed by measurement on Windows CI that comparing normalized/absolute/case-folded path strings alone is not enough -- the path `os.Executable()` returns and a path built from the current directory can point to the same file yet come out as different string representations (e.g. only one of them containing a short 8.3-format path component). It falls back to comparing path strings only when the destination path doesn't yet exist (the typical way `.snapshot` is used).
* **Writing (running `.overwrite`, self-overwrite):** a direct overwrite onto the same file (itself) is rejected at the OS level by the write lock on a currently-running file (Linux's `ETXTBSY`, Windows's `ERROR_SHARING_VIOLATION`). To work around this, the following steps are taken (proof-of-concept verified on both Linux and Windows):
  1. `rename` the currently running self, `<path>` → `<path>.san-db-ox.old`, as a backup (renaming a running file itself is permitted on both Linux and Windows)
  2. Freshly write the new content (engine bytes + new data + new footer) to the now-vacant original path
  3. Attempt to delete the backup file (succeeds on the spot on Linux; on Windows it's normal for this to fail since it's locked while running -- it's deleted on a best-effort basis at the next startup instead)

  If step 2's write fails, the file backed up in step 1 is restored to the original path on a best-effort basis. Also, if the running process is a temporary binary produced by `go run` (the OS deletes the file itself the moment the process exits, making an overwrite pointless), `.overwrite` explicitly rejects it with an error.
* **A freshly distributed binary with no data:** if no magic bytes are found at the end, it starts up as "engine only, empty data." A plain `go build`ed binary runs as-is, and only becomes a binary with data once `.snapshot` or `.overwrite` is run.
* This approach is a well-proven technique used by self-extracting installers and JAR files (searching backward from the end for a ZIP central directory), and doesn't interfere with how the OS loader runs it either.

### Data blob serialization method (settled)

For turning the "data blob" -- the state of an in-memory SQLite DB -- into a byte sequence, **the standard SQLite `Serialize`/`Deserialize` API (`sqlite3_serialize()` / `sqlite3_deserialize()`) is adopted** (confirmed by measurement; judged that no fallback is needed). No custom serialization format is invented. As stated in §6, this output is, as-is, the contents of a SQLite file.

* **The actual API:** what `modernc.org/sqlite` (confirmed at v1.58.0) provides is `func (c *conn) Serialize() ([]byte, error)` and `func (c *conn) Deserialize(buf []byte) error`, and **there is no argument for passing a schema name** (it always targets the main schema). The `conn` type itself is unexported, but its methods are exported, so it's reached by calling `conn.Raw(func(driverConn any) error { ... })` on `database/sql`'s `*sql.Conn`, then type-asserting `driverConn` to a locally defined interface (`interface{ Serialize() ([]byte, error) }`, etc).
* **When writing (`.snapshot` / `.overwrite`):** the value `Serialize()` returns, obtained via the method above, is written as-is as the footer scheme's "data blob."
* **At startup:** the data blob read from the tail of the binary is passed straight into `Deserialize(data)`, obtained the same way, letting SQLite re-expand it internally as an in-memory DB. Since `modernc.org/sqlite` internally calls `sqlite3_deserialize()` with `SQLITE_DESERIALIZE_RESIZEABLE|SQLITE_DESERIALIZE_FREEONCLOSE` specified, **the restored DB is just as expandable as any ordinary DB** (a large INSERT works fine -- confirmed by measurement that it doesn't become a fixed-size, restore-only DB).
* **The connection-sharing model (the `memdb` VFS):** the DSN adopted is `file:/<n>?vfs=memdb&_busy_timeout=<N>` (the `memdb` VFS `modernc.org/sqlite` implements). A DSN whose name starts with `/` lets multiple connections within the same process share the same in-memory store. Since `memdb`'s locking is implemented with the same kind of SHARED/RESERVED/EXCLUSIVE mechanism as a file-based DB, `busy_timeout` (SQLite's standard busy-handler mechanism) correctly kicks in on a lock conflict between connections, waiting a bounded time before giving up. The originally considered `file:<n>?mode=memory&cache=shared` (shared-cache) was rejected after measurement showed a lock conflict produces the separate `SQLITE_LOCKED_SHAREDCACHE` error, on which `busy_timeout` has no effect, and the wait itself can't even be canceled from the application side via `context`, potentially hanging indefinitely. A single "keeper connection" is held open until `Close()`, so the store isn't released even if every other connection closes (the keeper exists solely to keep the store alive; it's never used to run a SQL statement itself).
* **`Deserialize`'s relationship with multiple connections:** `Deserialize` is implemented to internally reopen the schema as an anonymous `memdb` store, so the result is only reflected on the connection that called it -- it's invisible even from another connection opened later on the same DSN. So `Open`/`OpenSelf`/`Load` all take a two-step approach: first `Deserialize` into a disposable connection, then copy its contents into the "live" DB (the store the keeper connection holds) using the standard SQLite online Backup API (the `sqlite3_backup_*` family, exposed by `modernc.org/sqlite` as `NewBackup`/`NewRestore`). Copying via the Backup API is a genuine copy through SQLite's own B-tree/pager path, and the result becomes visible from every connection to the destination (including ones that were already open before the Backup ran). Because `Load` uses this mechanism to overwrite the "live" DB on the spot, any other session that was already open before `.load` ran can see the new data too, without losing its connection.
* **`Export`'s implementation:** calls the same Backup API with a file DB as the destination (§6). Writing uses the atomic approach of writing to a temp file in the same directory, then `rename`ing it (the same idea as `.snapshot`'s writing), with permission set to `0644` (explicitly changed, since the default permission `os.CreateTemp` gives a temp file would otherwise leave it unreadable by other users). On failure, both the temp file itself and any `-journal`/`-wal`/`-shm` sidecar SQLite might create while writing are cleaned up.
* **`Load`'s implementation path differs by the kind of source:**
  * **A SQLite file (`.db`/`.sqlite`)** goes through the reverse of `Export` -- the standard SQLite online Backup API is called with the source file as the **source** and the destination as the copy target (exposed by `modernc.org/sqlite` as `NewRestore`). Since it lets SQLite open the file through its normal procedure, even a committed transaction that exists only in a `-wal` sidecar (under `journal_mode=WAL` operation) isn't missed. The source file, meanwhile, is opened for read-write (see §4).
    **The destination goes through a temp file (a two-step approach) rather than backing up directly into the "live" in-memory DB** (a `memdb`-VFS-specific limitation discovered by measurement). When the destination is empty, the Backup API copies the source's content as-is, header bytes included, so if the source is in WAL mode, the destination's header ends up with the WAL-mode flag too. A normal file VFS can open a WAL-mode-header DB just fine even with no corresponding `-wal` file, but **the `memdb` VFS cannot -- it fails with "unable to open database file"** (confirmed by measurement in Phase ② Step 3; the design originally called for backing up directly into the live DB, and was changed because of this constraint). So the implementation takes these steps:
    1. Back up from the source file → a temp file (a normal file VFS, not the same directory but the OS's standard temp directory)
    2. Run `PRAGMA journal_mode=DELETE` on the temp file, converting it to rollback mode (rewriting the header's flag)
    3. Back up from the temp file → the live in-memory DB
    4. Delete the temp file

    This same path is taken even when the source file isn't in WAL mode (to keep the implementation simple, with no branching).
  * **A data blob embedded in a SanDBox executable, and a byte sequence handed to `LoadFrom(io.Reader)`**, both use the two-step approach described above -- `Deserialize` into a disposable connection, then copy via the Backup API into the live DB. An embedded blob is always `Serialize()`'s output (a rollback-mode image originating from memdb), so the WAL problem structurally can't happen, making routing it through a file write pointless. An `io.Reader` can't carry a side file (a `-wal` sidecar, etc), so this is the only path available for it -- passing the byte sequence of a WAL-mode SQLite file straight to `LoadFrom` is detected via the SQLite file header's format-version byte (offset 18, 19 being `2`) and explicitly rejected with an error (see §10's "The difference between `Load` and `LoadFrom`").
* **The technical basis for the in-memory state being unchanged on a failed load:** `sqlite3_backup_step(-1)` opens a single write transaction on the destination and doesn't commit until `sqlite3_backup_finish()` is called. Even if `Step` fails, calling `Finish` rolls back that transaction, leaving the destination's (the live DB's) content exactly as it was before the change. **Always calling `Finish` even when `Step` fails is the precondition for this guarantee** (skip `Finish`, and an incomplete write transaction is left on the destination, blocking every subsequent write until `busy_timeout` elapses -- and for `NewRestore`, it also leaves the source file's handle open).
* **Known limitation:** SQLite's own `Serialize`/`Deserialize` can't handle a non-contiguous byte sequence, which in theory caps the database size below 2GB. But the `memdb` VFS adopted here has its own, separate default cap, `SQLITE_MEMDB_DEFAULT_MAXSIZE` (1GiB), and measurement confirmed `SQLITE_FULL` occurs around 960MiB -- **the effective limit is about 1GiB.** This is not expected to be a problem for the primary use cases (a CI/CD test DB, integration from another program, a learning sandbox, etc), but it can be a real constraint for a use case dealing with a large volume of data.

### `Complete`'s implementation method (settled)

Judging whether multi-line REPL input (whether `sql` is a syntactically complete statement) **calls SQLite's own `sqlite3_complete()` C function directly.** A homemade scanner that counts BEGIN/END tokens isn't used, since it risks mistaking the `END` of a `CASE ... END` expression appearing inside a `CREATE TRIGGER ... BEGIN ... END` body for the `END` that closes the trigger (matching the genuine `sqlite3` CLI's own judgment is more reliable).

`modernc.org/sqlite`'s top-level package (the public surface as a `database/sql` driver) doesn't expose this function, but it exists in the generated code package, `modernc.org/sqlite/lib` (package name `sqlite3`), as `Xsqlite3_complete(tls *libc.TLS, zSql uintptr) int32`. It's called in combination with `modernc.org/libc` (`NewTLS()`/`CString()`/`Xfree()`). **Both `modernc.org/libc` and `modernc.org/sqlite/lib` are already existing transitive dependencies of `modernc.org/sqlite` itself, so this adds neither a new dependency nor any binary-size growth.** `sqlite3_complete` is a pure string-scanning function that takes no DB handle, so it can be called even with no DB open.

**Risk and how it's handled:** `modernc.org/sqlite/lib` is generated code, not a public API `modernc.org/sqlite` documents or guarantees the stability of. When upgrading `modernc.org/sqlite`'s version, check whether `Xsqlite3_complete`'s signature, existence, and import path have changed. Should this path ever break, the fallback is to port SQLite's own state-machine logic in pure Go (not adopted in the first version -- it would double the effort of testing that the judgment matches the genuine implementation).

---

## 12. Startup options

Command-line arguments provide both a short flag (1 character) and a long flag. Short flags are kept **entirely lowercase**, to avoid the difficulty of remembering a mix of upper and lower case. Following this policy, a flag with no available lowercase letter to assign, or with little real need for a short form, **gets no short form.**

| Short | Long | Type | Default | Role |
| :--- | :--- | :--- | :--- | :--- |
| `-c` | `--command` | string (repeatable) | (unset) | Runs the given SQL/dot command and exits (§5). Repeatable; runs in the order given |
| `-m` | `--mode` | string | `list` | Output format: `list`/`column`/`csv`/`json`/`line` (the same as §3's `.mode`) |
| `-o` | `--snapshot-as` | string | (unset) | Default filename used when running `.snapshot` |
| `-q` | `--quiet` | bool | `false` | Suppresses the startup banner and log output |
| `-t` | `--timestamp` | bool | `false` | Whether to append a timestamp to saved filenames (see below) |
| `-i` | `--snapshot-interval` | duration | `0` (disabled) | Automatically performs a differently-named snapshot save at the given interval (e.g. `5m`, `1h`). Works in REPL mode and stdio mode; disabled in batch execution |
| -- | `--serve-stdio` | bool | `false` | Starts in stdio protocol mode (§7) |
| `-r` | `--read-only` | bool | `false` | Rejects write SQL, save operations, and file exports (§2) |
| `-v` | `--version` | bool | - | Show version |
| `-h` | `--help` | bool | - | Show help |

**How the version string is decided:** the value `-v`/`--version` displays is decided by this priority order:

1. **A release build**: the value embedded via `-ldflags -X main.version=<tag>` (see `release.yml`).
2. **If 1 gives nothing, and `runtime/debug`'s `ReadBuildInfo()` has a usable `Main.Version`**: use that value. This covers both installing via `go install <module>@<version>` (a path that can't receive `-ldflags`, so 1 can't catch it) and a local build (`go build`) inside a git repository -- as long as the checkout is committed (no untidy changes), the Go toolchain automatically embeds a pseudo-version derived from the commit (`vX.Y.Z-yyyymmddhhmmss-<commit>` form), which gets picked up here too. **A local build from a checkout with uncommitted (dirty) changes is excluded, though, and falls through to 3** -- `Main.Version` carries a `+dirty` suffix in that case, and accepting it would make a binary built from an undistributable state look like it were a real version (detected via the `vcs.modified` key in the `Settings` `ReadBuildInfo()` returns).
3. **When neither of the above yields a value** (a build with `-buildvcs=false`, a dirty local build, a build outside any VCS, etc): fixed at `dev`.

This resolved value is shared (the same implementation) by three places: `-v`'s output, the startup banner (§13 below), and the `version` the stdio protocol's hello line returns right after connecting (§7 -- a different thing from the `inspect` op's `version`, which is the footer's format version; the same kind of distinction as `naming.md`'s "`inspect` means something different despite sharing a name").

**Mode exclusivity:** `--serve-stdio` and `-c` cannot be given together (a usage error, exit code 2). With neither given, it's treated as the REPL if stdin is a terminal, or batch execution if it isn't.

**Other exclusivity:** `--read-only` and `--snapshot-interval` cannot be given together (a usage error, exit code 2). Since read-only mode rejects every save operation, it's incompatible with specifying periodic saving (§2).

### Appending a timestamp

Running `.snapshot`, whether the filename gets a date/time timestamp (`_YYYYMMDDHHMMSS`) appended is toggled with `--timestamp` (`-t`). It's a **bool flag** with no other choice of value.

* **When `--timestamp` is given:** the filename is produced with the following rule (a common rule whether the filename is omitted or given explicitly, and whether it's set at CLI startup or via the `.snapshot` command).
  1. If the base filename (the part before the extension) already has a `_YYYYMMDDHHMMSS` pattern, strip it first (avoids double-appending).
  2. Insert a fresh `_YYYYMMDDHHMMSS` right before the extension.
  3. If the extension is omitted, append `.exe` on Windows for executable output (the default), or `.sqlite` on every OS for `--sqlite` output.

  | Base filename | Result |
  | :--- | :--- |
  | `san-db-ox` (omitted; based on the running binary's name) | `san-db-ox_YYYYMMDDHHMMSS` |
  | `mydb_20260101120000` (already has a timestamp) | `mydb_YYYYMMDDHHMMSS` (the old one is stripped and replaced) |
  | `mydb.exe` | `mydb_YYYYMMDDHHMMSS.exe` |
  | `mydb` (given explicitly via `-o mydb`, on Windows) | `mydb_YYYYMMDDHHMMSS.exe` |
  | `mydb` (with `--sqlite`) | `mydb_YYYYMMDDHHMMSS.sqlite` |

  **The base name may contain hyphens** (since the separator is fixed as `_`, `san-db-ox` is handled as-is).

* **When `--timestamp` is not given (the default):** no timestamp is appended; the given filename (extension-completed per the rule above if omitted) is used as-is. Overwriting a same-named file can happen.

This option acts as the startup default, but it can also be overridden on the spot with the same option when running the `.snapshot` command.

```
.snapshot bug_123 --timestamp
.snapshot bug_123 --sqlite --timestamp
```

`.overwrite` has no concept of specifying a filename (it's fixed to its own path), so `--timestamp` doesn't apply to it.

### Periodic snapshots (`--snapshot-interval`)

Given `--snapshot-interval` (`-i`), a differently-named snapshot is automatically saved, repeated at that interval.

* **Purpose:** a safety net against the accident of "forgot to save during a long interactive session, then crashed and lost everything." Positioned as an **opt-in exception** to §1's "persistence only through explicit operations" principle.
* **Save method:** only supports saving under a different name (equivalent to `.snapshot`). Periodically running a self-overwrite (equivalent to `.overwrite`) is not supported, since the risk from frequently rewriting the currently-running file is too high. The output format is always an executable (the default); no periodic saving as a SQLite file is done.
* **Combining it with `--read-only` is a usage error** (exit code 2).
* **Filename:** follows the `--snapshot-as` / `--timestamp` settings. With `--timestamp` given, a timestamped file keeps being produced at every interval, so cleaning up the growing pile of files is, as in §4, left to the operator. To keep overwriting a fixed filename, don't give `--timestamp` (the default).
* **Handling a failed save:** if one periodic save fails (`ErrBusy`, etc), it's reported as a warning to stderr, and **the process continues** (retried at the next interval). Periodic saving is nothing more than a safety net against "forgetting to save" -- there's no reason a single failure should stop the REPL/stdio session itself.

---

## 13. Lifecycle and operational flow

```text
[1. Startup]
   └─> Run ./san-db-ox
   └─> Read data from the tail of the binary, expand it into memory (RAM)
   └─> Best-effort delete of any leftover .san-db-ox.old backup file from a previous run
   └─> Mode determination
        ├─> --serve-stdio given       : into stdio protocol mode (§7)
        ├─> -c given                  : into batch execution mode (§5)
        ├─> stdin not a terminal      : batch execution mode (reads stdin as a script)
        └─> Otherwise                 : shows the startup banner, starts the REPL

[2. Operation, running queries]
   └─> Every mode can run DDL / DML / TCL in full (no access control, §2)

[3. Saving (under a different name)]
   └─> Runs the .snapshot command (or the stdio snapshot op)
   └─> Packs the latest in-memory state together with the engine itself
   └─> Produces and outputs a new, differently-named executable (e.g. san-db-ox_20260831_150000)

[3'. Saving (self-overwrite)]
   └─> Runs the .overwrite command (explicit action only)
   └─> Backs itself up to <path>.san-db-ox.old → writes the new content to the now-vacant original path (§11)
   └─> On success, exits the process right there (the only operation where saving and exiting happen together)

[3''. Exporting to a SQLite file]
   └─> Runs .snapshot --sqlite <FILE> → produces a SQLite file (§6)

[4. Stopping (REPL mode)]
   └─> Normal ways to exit: the .exit / .quit command, or EOF on stdin (Ctrl+D)
   └─> Ctrl+C (SIGINT) is treated, on an interactive terminal, as an interrupt command rather than
        an exit (following the sqlite3 shell's convention):
        ├─> Pressed while a query is running, it cancels just that query and returns to the prompt
        │    (the process does not exit)
        ├─> Pressed once while idle (no query running), it discards the unfinished, possibly
        │    multi-line SQL being typed and simply reprints the prompt; the process does not exit
        ├─> Pressed twice in a row while idle, with no new input line read in between, the
        │    process exits right there (exit code 1; reading even one new input line resets the count)
        └─> When stdin isn't an interactive terminal, this handler is never installed at all, and
             SIGINT is treated as an ordinary process exit
   └─> On receiving SIGTERM, the process exits (no automatic save)

[4'. Stopping (batch execution / stdio mode)]
   └─> Batch: exits either when every statement has run, or aborted by an error (exit code per §5)
   └─> stdio: exits on EOF on stdin, or the close op
   └─> No SIGINT handler is installed (an ordinary process exit)
   └─> Neither one performs an automatic save

[5. In every case]
   └─> Whichever way it exits, the in-memory data is not saved and is simply gone (volatility is assumed)
```

### The startup banner

The startup banner is shown only in REPL mode. It shows the version, and whether data is embedded / the snapshot name. Rather than a dedicated command like `.status`, this is consolidated into showing it once at startup. Given `-q`/`--quiet`, the whole banner is suppressed. Neither batch execution nor stdio mode ever shows it. The version string the banner shows is the result of §12's resolution order (a release build's `-ldflags` → `go install` build info → `dev`), the same value `-v`'s output and the stdio protocol's hello line (§7) return as `version`.

**When stdin isn't an interactive terminal, none of the prompt, the continuation prompt, or the banner is shown at all, even when started in REPL mode.** This is switched via `isInteractive` (whether stdin is a terminal). This suppression is for non-interactive execution (§5, the implicit batch execution when stdin isn't a terminal), a judgment axis independent from `-q`/`--quiet` -- `-q` is an explicit user choice, whereas this one is decided automatically from the run environment. The same judgment is also used for whether to install the Ctrl+C signal handler (below, §2).

**With data:**
```
SanDBox v0.1.0
Loaded snapshot: mydb_20260901120000
Enter ".help" for usage hints.
```

**Without data:**
```
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
```

**Read-only mode (`--read-only`):** adds one line stating it's read-only on top of the above, and also swaps `.help`'s content for one that omits the rejected operations (§2).
```
SanDBox v0.1.0 (read-only)
Loaded snapshot: mydb_20260901120000
Enter ".help" for usage hints.
```

---
