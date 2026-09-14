*[日本語](README_ja.md) | **English***

# Guided tour

A hands-on, step-by-step tutorial. All you need is **one binary** -- no Go
toolchain, nothing to install. Each chapter builds on the previous one, so
read them in order (looking for a quick-reference index instead? See
[`docs/usage/`](../usage/README.md); for task-oriented recipes, see
[`docs/examples/`](../examples/README.md)).

About 15-20 minutes altogether.

1. [Getting started](01-getting-started.md) -- getting it, starting it, your first SELECT
2. [Tables and queries](02-tables-and-queries.md) -- CREATE/INSERT/SELECT, multi-line input
3. [Saving your data](03-saving-your-data.md) -- `.snapshot`/`.overwrite`
4. [Output modes](04-output-modes.md) -- `.mode`/`.headers`/`.dump`
5. [Working with SQLite files](05-sqlite-files.md) -- `.snapshot --sqlite`/`.load`/`.import`
6. [Using it from scripts](06-scripting.md) -- `-c`, exit codes, CI integration
7. [stdio and read-only mode](07-stdio-and-read-only.md) -- `--serve-stdio`, `--read-only`

Every command and its output in these chapters was fed into an actual built
binary and checked against the real result (`tests/docs.sh` keeps
confirming this in CI).
