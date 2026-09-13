*[English](read-only-demo.md)*

# 閲覧用デモ環境の一時公開

`socat`でトランスポートを外付けし、`--read-only`で書き込みを封じることで、
バイナリ1つと1行のコマンドから閲覧用のデモ環境を立てられる。サンプルデータを
埋め込んだバイナリを配って「これを実行して繋いでください」で済む。

## サンプルデータを埋め込む

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price INTEGER)" -c "INSERT INTO products VALUES (1, 'widget', 500), (2, 'gadget', 1200)" -c ".snapshot demo_db"
Wrote demo_db
```

## `--read-only`で起動する

`--read-only`（`-r`）を指定すると、書き込みを伴う操作がすべて拒否される
（書き込みSQL・`.snapshot`・`.overwrite`・`.load`・`.snapshot --sqlite`。
仕様書§2）。

<!-- verify -->
```console
$ chmod +x demo_db && ./demo_db --read-only -c "SELECT * FROM products"
1|widget|500
2|gadget|1200
$ ./demo_db --read-only -c "INSERT INTO products VALUES (3, 'sprocket', 300)" 2>&1
Error: attempt to write a readonly database (8)
```

## `socat`で外部へ公開する

`EXEC`アドレスと`--serve-stdio`を組み合わせると、本体側の実装ゼロで
TCP/UNIXドメインソケットのサービスが成立する。

```sh
socat TCP-LISTEN:5432,reuseaddr,fork EXEC:"./demo_db --read-only --serve-stdio"
```

**この構成には2つの避けられない制約がある（仕様書§8/§9-4）:**

1. **接続ごとに別プロセス・別DBになる。** `socat`の`fork`は接続を受理する
   たびにプロセスをforkするため、Aの接続とBの接続は完全に独立したメモリ上の
   DBを持つ。複数人が同じデータを見ながら共有する用途には使えない。
2. **認証が一切存在しない。** `--read-only`は書き込みを防ぐだけで、読み取り
   自体への認証は無い。到達できる者は誰でも任意のSELECTを実行できる。
   公開範囲は`bind=127.0.0.1`（ローカルのみ）やSSHポートフォワードで絞ること。

`fork`を外しても解決しない——子プロセスは1つだけになりDBも1つになるが、
その接続が切れると`socat`自体が終了し、次の接続を待てなくなる。「1つのDBを
立てっぱなしにして複数の接続を順番に捌く」構成は、この方式では作れない。
