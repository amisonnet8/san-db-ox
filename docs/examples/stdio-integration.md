*[日本語](stdio-integration_ja.md)*

# Embedding via stdio

Launch it as a child process from any language, and talk to it over JSON
Lines (line-delimited JSON). No network setup, no auth setup, no daemon to
manage. See the [stdio protocol](../usage/stdio-protocol.md) for the full
details.

## The bare minimum from a shell

Feed it in one direction (the responses aren't discarded -- they go to
`--serve-stdio`'s stdout).

<!-- verify -->
```console
$ ./san-db-ox --serve-stdio <<'REQUESTS'
{"op":"exec","sql":"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"}
{"op":"exec","sql":"INSERT INTO users VALUES (1, 'alice')"}
{"op":"query","sql":"SELECT id, name FROM users"}
REQUESTS
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"last_insert_id":0,"ok":true,"rows_affected":0}
{"last_insert_id":1,"ok":true,"rows_affected":1}
{"columns":["id","name"],"ok":true,"rows":[[1,"alice"]]}
```

**The first line (the hello line) answers no request.** A client either
skips it or uses it to check the protocol version.

## A minimal Go client

Launching it as a child process with `os/exec` and wiring up its standard
streams with a pipe is all it takes. No dedicated library needed.

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

func main() {
	cmd := exec.Command("./san-db-ox", "--serve-stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	cmd.Stderr = nil // must not be left unread if you do wire it up -- see below
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Scan() // hello line -- skip it
	fmt.Println("hello:", scanner.Text())

	req := map[string]any{"op": "query", "sql": "SELECT 1"}
	line, _ := json.Marshal(req)
	stdin.Write(append(line, '\n')) // must flush -- see the protocol doc

	scanner.Scan()
	fmt.Println("response:", scanner.Text())

	stdin.Close() // closing stdin makes the child process exit on its own
	cmd.Wait()
}
```

**If you do wire up stderr, always read it to completion.** Leave it piped
but unread, and the child process blocks once the pipe buffer fills up with
logs ([stdio protocol#Notes for client implementers](../usage/stdio-protocol.md#notes-for-client-implementers)).

## Embedding with Docker

Being a single binary with no network dependency means `FROM scratch` gives
you the smallest possible image. From the host side, use `docker run`'s
standard streams directly as the stdio protocol connection.

```dockerfile
FROM scratch
COPY san-db-ox /san-db-ox
ENTRYPOINT ["/san-db-ox", "--serve-stdio"]
```

```sh
docker run -i my-sandbox-image
```

To persist logs, either redirect with `2> /path/to/log` or hand it off to
Docker's own logging setup (`docker logs`, etc).
