*[English](stdio-integration.md)*

# stdio経由での他プログラムへの結合

任意の言語から子プロセスとして起動し、JSON Lines（行区切りのJSON）で対話する。
ネットワーク設定・認証設定・常駐プロセスの管理が一切不要。プロトコルの詳細は
[stdioプロトコル](../usage/stdio-protocol_ja.md)を参照。

## シェルから最小限を試す

一方向に流し込む（応答は読み捨てず、`--serve-stdio`のstdoutへ出力される）。

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

**1行目（hello行）はどのリクエストにも対応しない。** クライアントはこれを
読み飛ばすか、プロトコルバージョンの確認に使う。

## 最小限のGoクライアント

`os/exec`で子プロセスとして起動し、標準入出力をパイプで繋ぐだけで成立する。
専用ライブラリは不要。

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

**stderrを繋ぐ場合は必ず読み切ること。** 読まずに放置すると、ログでパイプの
バッファが埋まった時点で子プロセスがブロックする（[stdioプロトコル#クライアント実装時の注意](../usage/stdio-protocol_ja.md#クライアント実装時の注意)）。

## Dockerでの結合

単一バイナリでネットワークを持たないため、`FROM scratch`で最小のイメージが
作れる。ホスト側からは`docker run`の標準入出力をそのままstdioプロトコルの
接続として使う。

```dockerfile
FROM scratch
COPY san-db-ox /san-db-ox
ENTRYPOINT ["/san-db-ox", "--serve-stdio"]
```

```sh
docker run -i my-sandbox-image
```

ログを永続化したい場合は`2> /path/to/log`へのリダイレクト、またはDocker自身の
既存ログ基盤（`docker logs`等）に委ねる。
