*[English](stdio-protocol.md) | **日本語***

# stdioプロトコル

```sh
san-db-ox --serve-stdio
```

他のプログラムから SanDBox を利用するためのモード。**子プロセスとして起動し、
標準入出力で対話する。** ネットワークを経由しないため、サブプロセスとパイプを
扱える処理系であれば専用ライブラリなしで繋がる。

- バナー・プロンプトは出さない
- **stdout はプロトコルの応答専用**。ログ・診断は stderr
- **セッションの寿命＝プロセスの寿命。** トランザクションはプロセス内で維持される
- stdin が EOF に達すると終了する（保存は行われない）

## メッセージ形式

**行区切りJSON（JSON Lines）。1行＝1メッセージ。**

リクエスト:

```json
{"op":"query","sql":"SELECT id, name FROM users WHERE id = ?","params":[1]}
```

レスポンス:

```json
{"ok":true,"columns":["id","name"],"rows":[[1,"alice"]]}
```

- リクエストに `id` を含めると、対応するレスポンスにそのまま返される（省略可）。
  **応答は常にリクエストと同じ順序で返る**ため、`id` が無くても対応付けられる。
- 不正なJSONを送ってもエラー応答が1行返るだけで、接続は切れない。

## 起動直後の hello 行

接続すると、最初にリクエストに対応しない1行が出力される。

```json
{"protocol":1,"version":"v0.1.0","product":"SanDBox"}
```

**クライアントは必ずこの1行を読み飛ばす（または解釈する）こと。**
`protocol` はプロトコルのバージョン番号で、保存フォーマットのバージョンとは
別系統。

## op 一覧

| op | パラメータ | 応答 |
| :--- | :--- | :--- |
| `query` | `sql`, `params` | `columns`, `rows` |
| `exec` | `sql`, `params` | `rows_affected`, `last_insert_id` |
| `snapshot` | `filename`(省略可), `sqlite`(bool, 省略可), `timestamp`(bool, 省略可) | `path` |
| `load` | `path` | ─ |
| `inspect` | ─ | 下記参照 |
| `tables` | ─ | テーブル名の配列 |
| `schema` | `table`(省略可) | CREATE文 |
| `dump` | `pattern`(省略可) | `sql`（ダンプ全文） |
| `overwrite` | ─ | **応答を1行返してからプロセスが終了する** |
| `close` | ─ | 応答後にプロセスが正常終了する |

`BEGIN` / `COMMIT` / `ROLLBACK` に専用の op はない。`exec` でそのまま送る。
DDL も `exec` で実行できる。

**`.import` に対応する op は無い。** サーバー側の任意パスのCSVファイルを読む
操作であり、`inspect(path)` を op化しない理由と同じ懸念（外部公開時のファイル
システム探索手段になる）のため、意図的に非対応。

`query` は結果セットの全行を1つのレスポンスに含めて返す。**行を逐次取得する
カーソル機構は持たない。**

## `inspect`

自プロセスの状態を返す。REPLの起動バナーに相当する情報を、stdioクライアントが
取得するためのもの。

```json
{"op":"inspect"}
```

```json
{"ok":true,"has_data":true,"version":1,"data_length":1048576,"source":"mydb_20260901120000","read_only":false}
```

| フィールド | 内容 |
| :--- | :--- |
| `has_data` | 起動時にデータが埋め込まれていたか |
| `version` | 保存フォーマットのバージョン（`has_data` が `false` なら `null`） |
| `data_length` | 読み込んだデータの長さ（バイト。同上） |
| `source` | 起動した実行ファイルのファイル名 |
| `read_only` | `--read-only` で起動しているか |

**任意のパスを検査する機能は提供しない。** ファイルが `.load` できるかどうかは
`load` を実行してエラーを見ればわかる。

## 値のJSON表現

| SQLiteの値 | JSON表現 | 例 |
| :--- | :--- | :--- |
| NULL | `null` | `null` |
| INTEGER | 数値 | `42` |
| REAL | 数値（小数点付き） | `88.0` |
| TEXT | 文字列 | `"alice"` |
| BLOB | **1要素配列**（Base64） | `["iVBORw0KGgo="]` |

判別規則は1行で書ける。**値がJSONの配列なら BLOB、それ以外は JSON の型が
そのまま SQLite の型。**

```json
{"ok":true,"columns":["id","name","score","avatar","note"],"rows":[[1,"alice",88.5,["iVBORw0KGgo="],null]]}
```

- **`params` でも同じ表現を使える。** 読み出した値をそのまま書き戻せる。
  ```json
  {"op":"exec","sql":"INSERT INTO profiles VALUES (?, ?)","params":[1,["iVBORw0KGgo="]]}
  ```
- 要素数が1でない配列はプロトコル違反。
- この表現は `.mode json`（REPL・バッチ実行）と共通。

**既知の制約:** JSONの数値で正確に表現できる整数は 2^53 - 1 まで。SQLiteの
INTEGER（64bit）の全範囲は表現できず、JavaScript のように言語自体に64bit
整数型を持たない処理系では**エラーなく値がずれる**。Go / Rust / Java / C# など
64bit整数を扱える言語のドライバは、JSONの数値を倍精度浮動小数点へデコードせず、
数値トークンを文字列として取得してから整数へパースすること（Goなら
`json.Number`、Rustなら `serde_json` の arbitrary precision）。これで
サーバー側が数値で出力していても値は保たれる。

## エラー応答

```json
{"ok":false,"error":{"code":"sqlite_error","message":"no such table: users"}}
```

| `code` | 意味 |
| :--- | :--- |
| `sqlite_error` | `query`/`exec` のSQL実行エラー |
| `bad_request` | 不正なJSON、未知のフィールド、不正な値の表現、必須パラメータ欠落 |
| `io_error` | `snapshot`/`load`/`overwrite`/`dump` のファイルI/O・パス絡みの失敗 |
| `unsupported_op` | 未知の op |
| `read_only` | `--read-only` で拒否された操作 |

**エラーが発生してもプロセスは終了しない。** 続けて別のリクエストを送れる
（バッチ実行が即中断するのとは対照的）。

## クライアント実装時の注意

守らないと動かない項目。

- **1行書いたら必ずフラッシュする。** パイプはバッファリングされるため、
  怠ると双方が相手の入力を待ち続けてデッドロックする。
- **stderr を塞がない。** 子プロセスの stderr をパイプに繋いだまま読み捨てないと、
  ログでバッファが埋まった時点で子プロセスがブロックする。読まないなら
  `/dev/null` へ向けること。
- **EOF は接続断を意味する。** 子プロセスが異常終了した場合、行読み取りは
  EOF を返す。これを検出して終了コードを回収すること（回収しないとゾンビ
  プロセスが残る）。
- **終了は stdin のクローズで行う。** `close` op か stdin のクローズで
  子プロセスは自ら終了する。`close()` は「stdin を閉じる → 一定時間待つ →
  まだ生きていれば SIGTERM」という段階的な実装が望ましい。

## シェルから試す

一方向に流し込む（対話はできない）:

```sh
san-db-ox --serve-stdio <<'EOF'
{"op":"exec","sql":"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"}
{"op":"exec","sql":"INSERT INTO users VALUES (1, 'alice')"}
{"op":"query","sql":"SELECT id, name FROM users"}
EOF
```

（`san-db-ox`がPATH上に無ければ`./san-db-ox`と読み替える。実行例:）

<!-- verify -->
```console
$ ./san-db-ox --serve-stdio <<'EOF'
{"op":"exec","sql":"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"}
{"op":"exec","sql":"INSERT INTO users VALUES (1, 'alice')"}
{"op":"query","sql":"SELECT id, name FROM users"}
EOF
{"product":"SanDBox","protocol":1,"version":"v0.1.0"}
{"last_insert_id":0,"ok":true,"rows_affected":0}
{"last_insert_id":1,"ok":true,"rows_affected":1}
{"columns":["id","name"],"ok":true,"rows":[[1,"alice"]]}
```

双方向にやり取りする（bash のコプロセス）:

```sh
coproc DB { san-db-ox --serve-stdio; }
read -r hello <&"${DB[0]}"

echo '{"op":"exec","sql":"CREATE TABLE t (id INTEGER)"}' >&"${DB[1]}"
read -r line <&"${DB[0]}"; echo "$line"

echo '{"op":"query","sql":"SELECT * FROM t"}' >&"${DB[1]}"
read -r line <&"${DB[0]}"; echo "$line"

exec {DB[1]}>&-
wait "$DB_PID"
```

`jq` を対話パイプラインに挟むとバッファリングでハングする。`jq --unbuffered`
か `stdbuf -oL` を使うこと。

## 各言語向けドライバ

C・Python・Go などのクライアントドライバは、別プロジェクト
`san-db-ox-clients` で提供される。本プロジェクトが提供するのは、上記の
プロトコル仕様までである。
