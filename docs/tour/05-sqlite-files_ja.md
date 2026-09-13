# 5. SQLiteファイルとの入出力

*[入門ガイド目次](README_ja.md) / 前章: [4. 出力モード](04-output-modes_ja.md) / [English](05-sqlite-files.md)*

SanDBoxが扱う非実行形式のファイルは、独自形式ではなく**素のSQLiteファイル**
である。既存のSQLiteエコシステムとそのまま行き来できる。

## `.snapshot --sqlite` — SQLiteファイルとして書き出す

`.snapshot`は既定で実行ファイルを作るが、`--sqlite`を付けるとSQLiteファイル
になる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".snapshot mydb --sqlite"
Wrote mydb.sqlite
```

`mydb.sqlite`は、DBeaver / DB Browser for SQLite / `sqlite3` CLI / pandas
などから**そのまま開ける**、ただのSQLiteファイルである。

## `.load` — SQLiteファイルを取り込む

`.load`はファイルの中身から種別を自動判別するので、SanDBoxの実行ファイルと
SQLiteファイルのどちらも同じコマンドで取り込める。取り込みはメモリ上のDBを
完全に置き換える（マージはしない）。

<!-- verify -->
```console
$ ./san-db-ox -c ".load mydb.sqlite" -c "SELECT * FROM users"
Loaded data from mydb.sqlite
1|alice
```

`.load`自体はファイルを生成しない。取り込んだ状態を残したい場合は、続けて
`.snapshot`か`.overwrite`を実行する（[3章](03-saving-your-data_ja.md)）。

## `.import` — CSVファイルを取り込む

`.import FILE TABLE`は、`.mode`の設定に関わらず常にCSVとして読む。`TABLE`が
存在しなければ、1行目を列名として全TEXT列で自動的に作成する。

<!-- verify -->
```console
$ printf 'id,name\n1,alice\n2,bob\n' > people.csv
$ ./san-db-ox -c ".import people.csv people" -c "SELECT * FROM people"
Inserted 2 rows into "people".
1|alice
2|bob
```

フィールド数が列数と一致しない行があると、その行番号を含むエラーで処理
全体を中断し、1行も投入しない（`sqlite3`は警告のうえ補完・切り捨てして
継続するが、SanDBoxは意図的に中断する）。

*次: [6. スクリプトから使う](06-scripting_ja.md)*
