*[English](sqlite-interop.md)*

# 既存SQLite資産との橋渡し

SanDBoxが扱う非実行形式のファイルは、**素のSQLiteファイルのみ**（独自の
データファイル形式は持たない、仕様書§6）。この節では、GUIツールや
pandasのような既存のSQLiteエコシステムとの間でデータを行き来させる方法を
まとめる。

## SQLiteファイルとして書き出す（`.snapshot --sqlite`）

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE events (id INTEGER PRIMARY KEY, label TEXT)" -c "INSERT INTO events VALUES (1, 'signup')" -c ".snapshot events --sqlite"
Wrote events.sqlite
```

`events.sqlite`は、DBeaver / TablePlus / DB Browser for SQLite などの
GUIツール、`sqlite3` CLI、各言語の標準的なsqliteドライバから**そのまま
開ける**、ただのSQLiteファイルである。拡張子を省略すると`.sqlite`が
自動的に付く。

### pandasから読む

```python
import sqlite3
import pandas as pd

con = sqlite3.connect("events.sqlite")
df = pd.read_sql_query("SELECT * FROM events", con)
```

## 既存のSQLiteデータベースを取り込む（`.load`）

`.load`はファイルの中身（先頭16バイトのヘッダ）から種別を自動判別するため、
SanDBoxの実行ファイルと同じコマンドで、素のSQLiteファイルも取り込める。
取り込みはメモリ上のDBを**完全に置き換える**（マージはしない）。

<!-- verify -->
```console
$ ./san-db-ox -c ".load events.sqlite" -c "SELECT * FROM events"
Loaded data from events.sqlite
1|signup
```

`.load`自体はファイルを生成しない。取り込んだ状態を残したい場合は、続けて
`.snapshot`（別名保存）または`.overwrite`（自己上書き）を実行する。

## `.dump` / `.import` によるSQLテキスト経由の移行

バイナリ形式を介さず、SQL文のテキストとして中身を移したい場合は`.dump`/
`.import`を使う。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER, note TEXT)" -c "INSERT INTO t VALUES (1, 'hello')" -c ".dump"
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE t (id INTEGER, note TEXT);
INSERT INTO "t" VALUES(1,'hello');
COMMIT;
```

`.dump`の出力は、そのまま別のSanDBoxインスタンス（またはSQLiteそのもの）へ
流し込んでスキーマとデータを再現できる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE t (id INTEGER, note TEXT)" -c "INSERT INTO t VALUES (1, 'hello')" -c ".dump" > dump.sql
$ ./san-db-ox < dump.sql -c "SELECT * FROM t" 2>&1
Error: SQL logic error: no such table: t (1)
```

**`-c`を指定すると、stdinはスクリプトとして一切読まれない**（`.import`等が
stdinを使えるようにするための仕様、`docs/usage/cli-options_ja.md`）。上の例で
`t`が見つからないのはそのためで、`< dump.sql`の内容は静かに無視されている。
ダンプを流し込んだ上で結果も見たい場合は、SELECT文自体をダンプの末尾に
含めておくとよい。

<!-- verify -->
```console
$ (cat dump.sql; echo "SELECT * FROM t;") | ./san-db-ox
1|hello
```

CSVファイルを取り込みたい場合は`.import FILE TABLE`を使う。`TABLE`が
存在しなければ1行目を列名として全TEXT列で自動的に`CREATE`する。

<!-- verify -->
```console
$ printf 'id,note\n1,hello\n2,world\n' > rows.csv
$ ./san-db-ox -c ".import rows.csv t2" -c "SELECT * FROM t2"
Inserted 2 rows into "t2".
1|hello
2|world
```
