# SanDBox リファレンス

「今すぐ使う」ための索引。設計判断の理由や背景は `docs/spec/` の仕様書を参照。

| ドキュメント | 内容 |
| :--- | :--- |
| [起動オプション](cli-options_ja.md) | コマンドライン引数、動作モードの決まり方、終了コード |
| [REPLコマンド](repl-commands_ja.md) | ドットコマンド、出力モード、キー操作 |
| [stdioプロトコル](stdio-protocol_ja.md) | JSON Lines のリクエスト／レスポンス、op一覧、値の表現 |

## 30秒で試す

```sh
# ダウンロードして実行するだけ（インストール不要）
chmod +x san-db-ox
./san-db-ox
```

```
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox> CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
SanDBox> INSERT INTO users VALUES (1, 'alice');
SanDBox> SELECT * FROM users;
1|alice
SanDBox> .snapshot mydb
SanDBox> .exit
```

`mydb` は**それ自体が実行可能なファイル**になっている。配れば、受け取った人が
実行するだけで同じデータを持つDBが立ち上がる。

```sh
./mydb
```

## 4つの動作モード

| モード | 起動方法 | 用途 |
| :--- | :--- | :--- |
| REPL | 引数なし | 人間による対話操作 |
| バッチ実行 | `-c "SQL"` / stdin からのスクリプト | CI/CD、シェルスクリプト |
| stdioプロトコル | `--serve-stdio` | 他プログラムからの結合 |
| ライブラリ | `engine` パッケージをGoアプリへ組み込み | 内蔵DB層 |

いずれも**ネットワーク待受を行わない**。ネットワーク越しに使いたい場合は
socat 等でトランスポートを外付けする（仕様書§8）。

## 覚えておくとよいこと

- **保存は明示操作のみ。** `.exit` してもデータは保存されない。`.snapshot` か
  `.overwrite` を打つこと。
- **`.snapshot` は実行ファイルを作る。** SQLiteファイルが欲しいときは
  `.snapshot --sqlite`。
- **ログは stderr、結果は stdout。** スクリプトから使うときは `2>/dev/null` で
  ログを落とせる。
