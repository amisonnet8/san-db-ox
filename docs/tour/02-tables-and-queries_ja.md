# 2. テーブルとクエリ

*[入門ガイド目次](README_ja.md) / 前章: [1. はじめに](01-getting-started_ja.md) / [English](02-tables-and-queries.md)*

## テーブルを作る

SQLite方言のSQLがそのまま使える。CREATE/INSERT/SELECT/UPDATE/DELETE、
View・Index・Trigger・トランザクションまでフル機能で動く。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c "INSERT INTO users VALUES (2, 'bob')" -c "SELECT * FROM users"
1|alice
2|bob
```

既定の出力形式（`list`モード）は`|`区切りでヘッダ無し。他の形式は
[4章](04-output-modes_ja.md)で扱う。

## 複数行にまたがる入力

対話セッションでは、`;`に到達するまで文が完結しないので、SQL文を複数行に
分けて書ける（継続プロンプト`   ...> `が出る）。

```
SanDBox> CREATE TABLE users (
   ...>   id INTEGER PRIMARY KEY,
   ...>   name TEXT
   ...> );
SanDBox>
```

バッチ実行（`-c`やスクリプト）でも同じ規則で動く——複数行にまたがっていても、
`;`が来るまでは1つの文として扱われる。

<!-- verify -->
```console
$ printf 'CREATE TABLE users (\n  id INTEGER PRIMARY KEY,\n  name TEXT\n);\nINSERT INTO users VALUES (1, %s);\nSELECT * FROM users;\n' "'alice'" | ./san-db-ox
1|alice
```

## テーブル一覧とスキーマ

`.tables`でテーブル名の一覧、`.schema [TABLE]`でCREATE文を確認できる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c ".tables" -c ".schema users"
users
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
```

`.tables`・`.schema`はSQL文ではなくドットコマンド（`.`で始まる制御コマンド）
である。SanDBoxのコマンド体系は`sqlite3` CLIを踏襲しているので、`sqlite3`に
触れたことがあれば見覚えのある名前のはずである。

*次: [3. データを保存する](03-saving-your-data_ja.md)*
