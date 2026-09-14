*[日本語](README_ja.md) | **English***

# Examples

Each use case from spec §9, fleshed out with the actual commands to run.
Each one is self-contained, so read them in any order. Looking for a
quick-reference index instead? See [`docs/usage/`](../usage/README.md). Want
a guided, step-by-step introduction (SQL learning included)? See
[`docs/tour/`](../tour/README.md).

| File | Use case |
| :--- | :--- |
| [CI/CD Instant Test DB](ci-instant-test-db.md) | Stand up a test environment in seconds from one seeded binary |
| [Sharing bugs as executable snapshots](executable-snapshots.md) | Reproduce and share a bug together with the data state that triggered it |
| [Embedding via stdio](stdio-integration.md) | Launch as a child process and talk JSON Lines |
| [A temporary read-only demo](read-only-demo.md) | Stand up a read-only demo with `socat` + `--read-only` |
| [Bridging to existing SQLite assets](sqlite-interop.md) | SQLite file import/export, working with GUI tools and pandas |
