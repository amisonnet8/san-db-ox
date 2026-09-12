# REPLコマンド

SQL文に加えて、`.` で始まる制御コマンド（ドットコマンド）を実行できる。
コマンド体系は SQLite CLI（`sqlite3`）を踏襲している。

**ドットコマンドはバッチ実行モード（`-c` / stdin）でも同じように使える。**
stdioプロトコルでは、対応する op として提供される。

## 一覧

### 保存・読み込み

| コマンド | 役割 |
| :--- | :--- |
| `.snapshot [FILE] [--sqlite] [--timestamp]` | 現在のデータを保存する。既定では**別名の実行ファイル**、`--sqlite` 指定時は**SQLiteファイル**を生成する |
| `.overwrite` | 自身の実行ファイルを現在のデータで上書きし、**プロセスを終了する** |
| `.load FILE` | 別のSanDBox実行ファイル、またはSQLiteファイルからデータを取り込み、メモリ上のDBを置き換える |

### スキーマ・データ

| コマンド | 役割 |
| :--- | :--- |
| `.tables` | テーブル一覧を表示 |
| `.schema [TABLE]` | CREATE文を表示（省略時は全テーブル） |
| `.dump [PATTERN]` | スキーマとデータをSQL文としてダンプ（`PATTERN` はSQLのLIKEパターン、省略時は全テーブル） |
| `.import FILE TABLE` | CSVファイルをテーブルへ読み込む |

### 表示設定

| コマンド | 役割 |
| :--- | :--- |
| `.mode MODE` | 出力形式を変更（`list` / `column` / `csv` / `json` / `line`） |
| `.headers on\|off` | 結果にカラム名を出すかどうか |

### その他

| コマンド | 役割 |
| :--- | :--- |
| `.help` | コマンド一覧を表示 |
| `.exit [CODE]` / `.quit [CODE]` | 終了する（`CODE` 省略時は 0） |

## 出力モード（`.mode`）

```
SanDBox> SELECT id, name FROM users;
```

**`list`（既定）** — `|` 区切り、ヘッダなし、NULLは空文字列

```
1|alice
2|bob
```

**`column`** — 列幅を揃えて左詰め。切り替えると `.headers` が自動で on になる

```
id  name
--  -----
1   alice
2   bob
```

**`csv`** — RFC 4180。行末は CRLF

```
id,name
1,alice
2,bob
```

**`json`** — 値の表現は [stdioプロトコル](stdio-protocol_ja.md) と共通

```json
{"columns":["id","name"],"rows":[[1,"alice"],[2,"bob"]]}
```

**`line`** — 1列1行、行の間に空行

```
  id = 1
name = alice

  id = 2
name = bob
```

## 保存まわりの動作

**保存は明示操作のみ。** `.exit` / `.quit` / Ctrl+D / SIGTERM のいずれで終了
しても、データは保存されず消える。保存確認のプロンプトも出ない。

```
SanDBox> .snapshot mydb          # ./mydb（実行ファイル）を生成
SanDBox> .snapshot mydb --sqlite # ./mydb.sqlite（SQLiteファイル）を生成
SanDBox> .snapshot --timestamp   # ./san-db-ox_20260901120000 を生成
SanDBox> .overwrite              # 自分自身を書き換えて終了
```

`.snapshot --sqlite` で生成したファイルは、DBeaver / DB Browser for SQLite /
pandas / `sqlite3` CLI などから**そのまま開ける**。拡張子を省略すると
`.sqlite` が補完される。

`.load` はファイルの中身を見て種別を自動判別する。SanDBox実行ファイルでも
SQLiteファイルでも同じコマンドで取り込める。取り込みはメモリ上のDBを
**完全に置き換える**（マージはしない）。ファイルは生成されないので、
結果を残したい場合は続けて `.snapshot` か `.overwrite` を実行する。

### 別のOS向けバイナリへデータを移す

```
# 1. 対象OS向けの空バイナリを起動（例: Windows用を Windows 上で）
./san-db-ox.exe

# 2. データを取り込む
SanDBox> .load mydb_from_linux

# 3. そのバイナリ自身に書き込む
SanDBox> .overwrite
```

`.load` は取り込み元のエンジン部分を一切参照しないため、この手順で
OS/アーキテクチャをまたいだデータ移植ができる。

## `.import` の仕様

- **`.mode` の設定に関わらず、常にCSVとして読む。**
- `TABLE` が存在しなければ、1行目を列名として全TEXT列で自動的に `CREATE` する。
  存在すれば1行目からデータとして扱う。
- **フィールド数が列数と一致しない行があった場合、その行番号を含むエラーで
  処理全体を中断し、1行も投入しない。**（`sqlite3` は警告のうえ補完／切り捨てして
  継続するが、SanDBox は意図的に中断する）

## キー操作（対話時）

| キー | 動作 |
| :--- | :--- |
| Ctrl+D | 終了（EOF） |
| Ctrl+C（クエリ実行中） | そのクエリだけを中断してプロンプトへ戻る |
| Ctrl+C（アイドル時・1回） | 入力中の未確定なSQLを破棄してプロンプトを出し直す |
| Ctrl+C（アイドル時・連続2回） | プロセスを終了する（終了コード 1） |

連続2回の判定は、間に新しい入力行を1行でも読むとリセットされる。

**標準入力が端末でない場合、Ctrl+C ハンドラは登録されない。** パイプや
スクリプト経由で起動した SanDBox は、SIGINT で通常どおり即座に終了する。

## 採用していないコマンド

SanDBox は常に単一のインメモリDBのみを扱うため、以下の `sqlite3` 由来の
コマンドは持たない。

| コマンド | 理由 |
| :--- | :--- |
| `.open` / `.databases` | 複数DBを切り替える概念がない |
| `.backup` / `.restore` | `.snapshot` と役割が重複する |
| `.session` | 変更履歴セッション機能。スコープ外 |

出力モードのうち `quote` / `insert` / `tabs` / `markdown` / `box` / `html` も
採用していない（装飾を最小限にし、パイプ処理との相性を優先するため）。
