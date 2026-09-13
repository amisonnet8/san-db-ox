# 4. 出力モード

*[入門ガイド目次](README_ja.md) / 前章: [3. データを保存する](03-saving-your-data_ja.md) / [English](04-output-modes.md)*

`.mode`で結果の表示形式を切り替えられる。既定は`list`。

## `list`・`column`・`json`・`line`

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m list
1|alice
2|bob
```

`column`は列幅を揃えて左詰めにする。切り替えると`.headers`（カラム名の
表示）が自動でonになる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m column
id  name
--  -----
1   alice
2   bob
```

`json`はNULL/INTEGER/REAL/TEXT/BLOBの型を保つ表現を使う（stdioプロトコルと
共通の規則、[7章](07-stdio-and-read-only_ja.md)で扱う）。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m json
{"columns":["id","name"],"rows":[[1,"alice"],[2,"bob"]]}
```

`line`は1列1行、レコードの間に空行を挟む。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice'),(2,'bob')" -c "SELECT id, name FROM users" -m line
  id = 1
name = alice

  id = 2
name = bob
```

`csv`（RFC 4180、行末はCRLF）もあるが、CRLFはこのドキュメント上でうまく
表示できないため割愛する。起動オプション`-m`でも同じ形式を指定できる
（[6章](06-scripting_ja.md)）。

## `.headers` を個別に切り替える

`column`以外のモードでは既定でヘッダ無しだが、`.headers on`で明示的に
出せる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1,'alice')" -c ".headers on" -c "SELECT id, name FROM users"
id|name
1|alice
```

## `.dump` — スキーマとデータをSQLとして書き出す

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".dump"
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO "users" VALUES(1,'alice');
COMMIT;
```

出力は別のSanDBox（やSQLite）インスタンスへそのまま流し込める形になって
いる。SQLiteファイルとの相互運用は[5章](05-sqlite-files_ja.md)で扱う。

*次: [5. SQLiteファイルとの入出力](05-sqlite-files_ja.md)*
