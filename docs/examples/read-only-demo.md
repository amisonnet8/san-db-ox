*[日本語](read-only-demo_ja.md)*

# A temporary read-only demo

Bolt on a transport with `socat` and lock out writes with `--read-only`, and
one binary plus one line stands up a read-only demo environment. Just hand
someone a binary with sample data embedded and say "run this and connect."

## Embed sample data

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price INTEGER)" -c "INSERT INTO products VALUES (1, 'widget', 500), (2, 'gadget', 1200)" -c ".snapshot demo_db"
Wrote demo_db
```

## Start with `--read-only`

`--read-only` (`-r`) rejects every write operation (write SQL, `.snapshot`,
`.overwrite`, `.load`, `.snapshot --sqlite`; spec §2).

<!-- verify -->
```console
$ chmod +x demo_db && ./demo_db --read-only -c "SELECT * FROM products"
1|widget|500
2|gadget|1200
$ ./demo_db --read-only -c "INSERT INTO products VALUES (3, 'sprocket', 300)" 2>&1
Error: attempt to write a readonly database (8)
```

## Publish it externally with `socat`

Pairing an `EXEC` address with `--serve-stdio` makes a TCP/Unix-socket
service work with zero implementation on SanDBox's side.

```sh
socat TCP-LISTEN:5432,reuseaddr,fork EXEC:"./demo_db --read-only --serve-stdio"
```

**This setup has two unavoidable constraints (spec §8/§9-4):**

1. **Each connection gets its own process, and its own database.** `socat`'s
   `fork` spawns a new process per connection, so connection A and
   connection B hold completely independent in-memory databases. This
   cannot be used for multiple people sharing a view of the same data.
2. **There is no authentication at all.** `--read-only` only blocks writes;
   reading itself is not gated. Anyone who can reach it can run any SELECT.
   Restrict who can reach it with `bind=127.0.0.1` (local only) or an SSH
   tunnel.

Dropping `fork` doesn't fix this either -- there's then only one child
process and one database, but once that connection drops, `socat` itself
exits and can't wait for the next one. "Keep one database running and serve
multiple connections to it in turn" is simply not a shape this approach can
produce.
