# 起動オプション

```
san-db-ox [OPTIONS]
```

## 一覧

| 短縮 | 長い形式 | 型 | 既定値 | 役割 |
| :--- | :--- | :--- | :--- | :--- |
| `-c` | `--command` | string（複数可） | ─ | 指定したSQL／ドットコマンドを実行して終了する。複数回指定でき、指定順に実行される |
| `-m` | `--mode` | string | `list` | 出力形式。`list` / `column` / `csv` / `json` / `line` |
| `-o` | `--snapshot-as` | string | ─ | `.snapshot` 実行時に使う既定のファイル名 |
| `-q` | `--quiet` | bool | `false` | 起動時のバナー・ログ出力を抑制する |
| `-t` | `--timestamp` | bool | `false` | 保存時にファイル名へタイムスタンプを付与する |
| `-i` | `--snapshot-interval` | duration | `0`（無効） | 指定間隔で自動的に別名スナップショットを保存する（例: `5m`, `1h`） |
| `-r` | `--read-only` | bool | `false` | 書き込みを伴う操作をすべて拒否する |
| ─ | `--serve-stdio` | bool | `false` | stdioプロトコルモードで起動する |
| `-v` | `--version` | bool | ─ | バージョンを表示して終了 |
| `-h` | `--help` | bool | ─ | ヘルプを表示して終了 |

`--serve-stdio` に短縮形はない。

## モードの決まり方

| 条件 | 起動するモード |
| :--- | :--- |
| `--serve-stdio` を指定 | stdioプロトコル |
| `-c` を指定 | バッチ実行 |
| どちらも未指定・標準入力が端末**でない** | バッチ実行（stdin をSQLスクリプトとして読む） |
| どちらも未指定・標準入力が端末 | REPL |

`--serve-stdio` と `-c` の同時指定は**使用法エラー**（終了コード 2）。

`-c` を指定した場合、stdin はスクリプトとして読まれない（`.import` 等が
stdin を使えるようにするため）。

**`-c` の各値、および stdin スクリプト全体は、それぞれ独立した1つの実行単位
として扱われる。** 末尾に `;` が無くても、その値（またはスクリプト全体）の
終わりの時点で自動的に補って実行する（`sqlite3` CLI の `-cmd` と同じ扱い）。
複数の `-c` を跨いで文が継続することはない——`-c "SELECT 1" -c "UNION SELECT
2"` は「2つの独立した文」として扱われるため、`UNION` を含む1つの文には
ならず、2つ目の `-c` は単独では構文エラーになる（1つ目の `-c` は正常に
`1` を返す）。

## 終了コード

| コード | 意味 |
| :--- | :--- |
| `0` | 正常終了 |
| `1` | SQL実行エラー、またはドットコマンドの実行失敗 |
| `2` | 使用法エラー（不正なフラグ、ファイルが開けない等） |

`.exit CODE` / `.quit CODE` で明示指定した場合はその値が優先される。

バッチ実行では**エラーが発生した時点で処理全体を中断**し、後続の文は実行
しない。

## 使用例

```sh
# 対話（REPL）
./san-db-ox

# 1文だけ実行してJSONで受け取る
./san-db-ox -c "SELECT * FROM users" -m json

# 複数の文を順に実行
./san-db-ox -c "CREATE TABLE t (id INTEGER)" -c "INSERT INTO t VALUES (1)" -c ".snapshot seeded"

# スクリプトを流し込む
./san-db-ox < schema.sql

# CIでの利用（ログを捨て、終了コードで判定）
./san-db-ox -c "SELECT count(*) FROM users" -m json 2>/dev/null || exit 1

# 読み取り専用で起動
./san-db-ox --read-only

# 他プログラムから結合する
./san-db-ox --serve-stdio
```

## `--timestamp`（`-t`）

保存時のファイル名に `_YYYYMMDDHHMMSS` を付与する。付与規則は次のとおり。

1. ベース名（拡張子を除く部分）に既存の `_YYYYMMDDHHMMSS` があれば取り除く
   （二重付与を防ぐ）
2. 拡張子の直前に新しい `_YYYYMMDDHHMMSS` を挿入する
3. 拡張子が省略されている場合、実行ファイル出力なら Windows で `.exe` を、
   `--sqlite` 指定時は全OSで `.sqlite` を付与する

| ベース名 | 生成されるファイル名 |
| :--- | :--- |
| `san-db-ox`（省略時、実行中バイナリ名） | `san-db-ox_20260901120000` |
| `mydb_20260101120000` | `mydb_20260901120000`（古い方は差し替え） |
| `mydb.exe` | `mydb_20260901120000.exe` |
| `mydb`（Windows） | `mydb_20260901120000.exe` |
| `mydb`（`--sqlite` 指定時） | `mydb_20260901120000.sqlite` |

`.snapshot` コマンド側でも `--timestamp` を指定でき、その場合は起動時の設定を
上書きする。`.overwrite` には適用されない（保存先が自分自身に固定のため）。

## `--snapshot-interval`（`-i`）

指定間隔で `.snapshot` 相当の保存を自動実行する。長時間の対話セッション中に
保存し忘れて全データを失う事故を防ぐための保険。

- REPLモード・stdioモードで有効。**バッチ実行では無効**（指定すると警告を
  stderr へ出して無視する）。
- 別名保存のみ。`.overwrite` 相当の定期実行はサポートしない。
- 出力形式は実行ファイル固定（SQLiteファイルでの定期保存は行わない）。
- ファイル名は `--snapshot-as` / `--timestamp` の設定に従う。`--timestamp` を
  付けるとファイルが増え続けるので、上書きし続けたい場合は付けない。
- `--read-only` との同時指定は使用法エラー（終了コード 2）。

## `--read-only`（`-r`）

書き込みを伴う操作をすべて拒否する。socat 等で外部へ公開する際に、閲覧専用の
環境を提供するためのもの。

**拒否される操作:**

- 書き込みSQL（`INSERT` / `UPDATE` / `DELETE` / DDL / トランザクション内の書き込み）
- `.snapshot`
- `.snapshot --sqlite`
- `.overwrite`
- `.load`

REPLでは起動バナーに `(read-only)` が付き、`.help` の一覧からも拒否される
操作が消える。stdioモードでは `read_only` エラーコードが返る。

これは経路ごとのアクセス制御ではなく、**プロセス全体の起動時モード**である。
指定しなければ、どのモードからでもDDLを含む任意のSQLを実行できる。

## ログと結果の分離

- 結果（クエリ結果・プロトコル応答）は **stdout**
- ログ・警告・エラーメッセージは **stderr**

ログファイルは生成しない。永続化したい場合はリダイレクトや Docker / systemd の
既存基盤に委ねる。

```sh
./san-db-ox -c "SELECT 1" 2> san-db-ox.log
```
