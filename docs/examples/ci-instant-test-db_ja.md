*[English](ci-instant-test-db.md)*

# CI/CDでのInstant Test DB

テーブル構築・シードデータ投入済みのバイナリを1個用意しておけば、CIジョブの
中で1秒未満でメモリ上のテスト環境が立ち上がる。外部DBコンテナの起動待ち、
マイグレーションの実行、テスト後のクリーンアップが一切不要になる。

## シード済みバイナリを作る

空バイナリに対して、テーブル定義とシードデータを`-c`で流し込み、
`.snapshot`で新しい実行ファイルとして固定する。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, role TEXT)" -c "INSERT INTO users VALUES (1, 'alice', 'admin'), (2, 'bob', 'member')" -c ".snapshot seeded_test_db"
Wrote seeded_test_db
```

`seeded_test_db`をリポジトリに含める必要はない——**このコマンド自体をCIの
セットアップ手順に含めれば、実行するたびに同じ状態のバイナリが再生成される**。
テストの前提データを変えたい場合も、このコマンドを直すだけでよい。

## テストから使う

`-c`（またはstdinスクリプト）と終了コードで検証すれば、シェルスクリプトや
Makefileからそのまま呼び出せる（仕様書§5）。

<!-- verify -->
```console
$ chmod +x seeded_test_db && ./seeded_test_db -c "SELECT count(*) FROM users WHERE role = 'admin'" -m json
{"columns":["count(*)"],"rows":[[1]]}
```

期待した1件と一致しなければ、SQL実行エラーと同様に**終了コードが非0**に
なるようアサーションを書けばよい。例えば`sqlite3`の代わりにテスト対象を
呼び出す形でシェルスクリプトに組み込める。

```sh
#!/usr/bin/env bash
set -euo pipefail

count="$(./seeded_test_db -c "SELECT count(*) FROM users WHERE role = 'admin'")"
if [ "$count" != "1" ]; then
  echo "expected 1 admin user, got $count" >&2
  exit 1
fi
```

## ログと結果を分ける

CIのログを結果で汚したくない場合は、`2>/dev/null`でログ・警告を捨てられる
（結果はstdout、ログ・エラーはstderrに分離されている。仕様書§0）。

<!-- verify -->
```console
$ ./seeded_test_db -c "SELECT count(*) FROM users" -m json 2>/dev/null
{"columns":["count(*)"],"rows":[[2]]}
```
