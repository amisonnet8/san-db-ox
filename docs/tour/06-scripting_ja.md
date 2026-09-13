# 6. スクリプトから使う

*[入門ガイド目次](README_ja.md) / 前章: [5. SQLiteファイルとの入出力](05-sqlite-files_ja.md) / [English](06-scripting.md)*

ここまでは対話（REPL）を前提にしてきたが、`-c`やstdin経由のスクリプトで
バッチ実行すれば、シェルスクリプトやCIパイプラインからそのまま呼び出せる。

## `-c` で1文（または複数文）を実行する

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT 1 + 1"
2
```

`-c`は複数回指定でき、指定した順に実行される。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER)" -c "INSERT INTO t VALUES (1)" -c "SELECT * FROM t"
1
```

**`-c`の各値は、それぞれ独立した1つの実行単位として扱われる。** 末尾に`;`が
無くても、その値の終わりの時点で自動的に補って実行する。裏を返すと、複数の
`-c`を跨いで1つの文が継続することはない。

## stdin からスクリプトを読む

引数を付けずに標準入力へSQLを流し込むと、それをスクリプトとして実行する
（標準入力が対話端末でない場合。パイプ・リダイレクト経由がこれに当たる）。

<!-- verify -->
```console
$ printf 'CREATE TABLE t (id INTEGER);\nINSERT INTO t VALUES (42);\nSELECT * FROM t;\n' | ./san-db-ox
42
```

## エラーで即座に中断する

バッチ実行では、エラーが発生した時点で処理全体を中断し、後続の文は実行
しない。終了コードは`1`。

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT 1" -c "not valid sql" -c "SELECT 2" 2>&1; echo "exit=$?"
1
Error: SQL logic error: near "not": syntax error (1)
exit=1
```

2番目の`-c`でエラーになったため、3番目の`SELECT 2`は実行されていない。

## CIパイプラインでの利用

結果はstdout、ログ・エラーはstderrに分離されているので、`2>/dev/null`で
ログを捨てつつ終了コードだけで判定できる。

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT count(*) FROM sqlite_master" -m json 2>/dev/null; echo "exit=$?"
{"columns":["count(*)"],"rows":[[0]]}
exit=0
```

より実践的な例は[実例: CI/CDでのInstant Test DB](../examples/ci-instant-test-db_ja.md)を
参照。

*次: [7. stdioと読み取り専用モード](07-stdio-and-read-only_ja.md)*
