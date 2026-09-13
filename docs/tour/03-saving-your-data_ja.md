# 3. データを保存する

*[入門ガイド目次](README_ja.md) / 前章: [2. テーブルとクエリ](02-tables-and-queries_ja.md) / [English](03-saving-your-data.md)*

1章で触れたとおり、`.exit`してもデータは保存されない。保存は`.snapshot`か
`.overwrite`という明示操作でのみ行う。

## `.snapshot` — 別名の実行ファイルとして保存

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".snapshot mydb"
Wrote mydb
```

`mydb`は**それ自体が実行可能なファイル**になっている。DBエンジンとデータの
両方を含んでいるので、これ単体を人に渡せば、受け取った側は実行するだけで
同じデータを持つDBを起動できる。

<!-- verify -->
```console
$ chmod +x mydb && ./mydb -c "SELECT * FROM users"
1|alice
```

ファイル名を省略すると、実行中のバイナリ自身の名前がベースになる
（`--timestamp`と併用するとタイムスタンプが付く）。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t(x)" -c ".snapshot --timestamp"
Wrote san-db-ox_20260901120000
```

## `.overwrite` — 自分自身を書き換える

`.overwrite`は、今動いているプロセス自身の実行ファイルを、現在のメモリ上の
データで上書きする。ファイル名を指定する必要が無く、**成功すると同時に
プロセスを終了する**。

<!-- verify -->
```console
$ cp mydb mydb2 && chmod +x mydb2 && ./mydb2 -c "INSERT INTO users VALUES (2, 'bob')" -c ".overwrite"
Overwrite ok, exiting.
$ ./mydb2 -c "SELECT * FROM users"
1|alice
2|bob
```

`mydb2`自身に`bob`の行が書き込まれ、次に起動したときにはそれが最初から
埋め込まれている。「データを投入してビルドを完成させる」という自動化に
向いた形——CI上で空バイナリにシードデータを詰めて配布用バイナリを作る、
といった用途に使える（[実例: CI/CDでのInstant Test DB](../examples/ci-instant-test-db_ja.md)）。

## どちらを使うか

- **他人に配る・別バージョンとして残したい** → `.snapshot`（元のファイルは
  変更されない）
- **今のバイナリ自身にデータを確定させたい** → `.overwrite`

*次: [4. 出力モード](04-output-modes_ja.md)*
