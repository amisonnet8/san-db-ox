# 7. stdioと読み取り専用モード

*[入門ガイド目次](README_ja.md) / 前章: [6. スクリプトから使う](06-scripting_ja.md) / [English](07-stdio-and-read-only.md)*

これで最後の章。SanDBoxを**他のプログラムから**使う方法と、**書き込みを
封じて**使う方法を見る。

## `--serve-stdio` — 他プログラムから結合する

`--serve-stdio`で起動すると、標準入出力で行区切りのJSON（JSON Lines）を
やり取りするモードになる。ネットワークは一切使わない。

接続すると、まず**hello行**が1行届く（どのリクエストにも対応しない）。

<!-- verify -->
```console
$ ./san-db-ox --serve-stdio <<'EOF'
{"op":"exec","sql":"CREATE TABLE t (id INTEGER)"}
{"op":"exec","sql":"INSERT INTO t VALUES (1)"}
{"op":"query","sql":"SELECT * FROM t"}
EOF
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"last_insert_id":0,"ok":true,"rows_affected":0}
{"last_insert_id":1,"ok":true,"rows_affected":1}
{"columns":["id"],"ok":true,"rows":[[1]]}
```

`BEGIN`/`COMMIT`/`ROLLBACK`やDDLも、専用のopではなく`exec`でそのまま送る。
値の表現（NULL/INTEGER/REAL/TEXT/BLOB）は`.mode json`と共通のルールを使う
（[4章](04-output-modes_ja.md)）。

自プロセスの状態（データの有無、読み取り専用かどうか等）は`inspect` opで
確認できる。

<!-- verify -->
```console
$ echo '{"op":"inspect"}' | ./san-db-ox --serve-stdio
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"data_length":null,"has_data":false,"ok":true,"read_only":false,"source":"san-db-ox","version":null}
```

他のプログラムから使う具体例（最小のGoクライアント、Dockerでの結合等）は
[実例: stdio経由での他プログラム結合](../examples/stdio-integration_ja.md)を参照。

## `--read-only` — 書き込みを封じる

`--read-only`（`-r`）を指定すると、書き込みSQL・`.snapshot`・`.overwrite`・
`.load`（`--sqlite`指定時含む）がすべて拒否される。閲覧用のデモ環境を
安全に公開したいときに使う。

<!-- verify -->
```console
$ ./san-db-ox --read-only -c "CREATE TABLE t (id INTEGER)" 2>&1; echo "exit=$?"
Error: attempt to write a readonly database (8)
exit=1
```

読み取りは通常どおり行える。

<!-- verify -->
```console
$ ./san-db-ox --read-only -c "SELECT 1"
1
```

`--read-only`はプロセス全体の起動時モードであり、経路（REPL・バッチ・stdio）
ごとの権限設定ではない——どの経路から入っても同じ制限がかかる。`socat`で
外部公開する際の具体的な構成は
[実例: 閲覧用デモ環境の一時公開](../examples/read-only-demo_ja.md)を参照。

## ここまでのまとめ

- データはメモリ上にあり、`.snapshot`/`.overwrite`で明示的に保存するまで
  消える（[1章](01-getting-started_ja.md)、[3章](03-saving-your-data_ja.md)）。
- SQL自体はSQLite方言がフル機能で使える（[2章](02-tables-and-queries_ja.md)）。
- 出力形式は`.mode`で切り替えられ、`json`はstdioプロトコルと表現を共有する
  （[4章](04-output-modes_ja.md)、本章）。
- 素のSQLiteファイルとそのまま行き来できる（[5章](05-sqlite-files_ja.md)）。
- `-c`・stdinスクリプト・`--serve-stdio`のいずれでも、REPLと同じドット
  コマンドが使える（[6章](06-scripting_ja.md)、本章）。

より実践的な使い方は[実例集](../examples/README_ja.md)、各コマンド・
オプションの詳細は[起動オプション・REPLコマンド・stdioプロトコル](../usage/README_ja.md)、
設計の背景まで知りたい場合は[仕様書](../spec/san-db-ox_spec_ja.md)を参照。
