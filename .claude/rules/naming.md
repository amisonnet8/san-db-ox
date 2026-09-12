# 命名規則

## 製品名の表記

製品名は3段階の優先順位で使い分ける。**上の段が使える場面では必ず上の段を
使う**（仕様書§0）。

| 優先 | 表記 | 使う場面 |
| :--- | :--- | :--- |
| 第1 | `san-db-ox` | 制約が無い全ての場面（リポジトリ名、実行ファイル名、モジュールパス、ソケットパス、イメージ名） |
| 第2 | `san_db_ox` | ハイフンが使えない場面（環境変数、C言語の識別子） |
| 第3 | `sandbox` | 上記2つが長さ等の理由で使えない場面（固定長フィールド。フッターのMagic `SANDBOX1` のみ） |

表示名（バナー・README・ドキュメント）は `SanDBox`。**別名（`sandbox` への
シンボリックリンク等）は配布物に含めない。**

## レイヤー間で名前を揃える

SanDBoxには、同じ操作を提供する経路が4つある。**REPLドットコマンド・CLI起動
オプション・stdio op・`engine` API の名前を、できる限り一致させること。**
新しい機能を追加する際は、この4つを同時に確認・設計する。

> **出所:** 前身プロジェクト **ExecDB** で、`Save()`/`SaveSelf()` という
> `engine` APIの命名がコマンド名（`.snapshot`/`.overwrite`）とずれ、後から
> `Snapshot()`/`Overwrite()` へ改名した経緯がある。同じ失敗を繰り返さない
> ための規則。

| REPLコマンド | CLI起動オプション | stdio op | `engine` API |
| :--- | :--- | :--- | :--- |
| `.snapshot` | `-o`/`--snapshot-as`, `-t`/`--timestamp` | `snapshot` | `db.Snapshot(path string)` |
| `.snapshot --sqlite` | ─ | `snapshot`（`sqlite: true`） | `db.Export(path string)` |
| `.overwrite` | ─（自身のパスに固定） | `overwrite` | `db.Overwrite()` |
| `.load` | ─ | `load` | `db.Load(path string)` |
| `.tables` | ─ | `tables` | ─ |
| `.schema` | ─ | `schema` | ─ |
| `.exit` / `.quit` | ─ | `close` | ─ |
| ─（SQL文を直接入力） | ─ | `query` / `exec` | `db.Query()` / `db.Exec()` / `s.Query()` / `s.Exec()` |
| `.mode` | `-m`/`--mode` | ─（JSON Lines固定） | ─ |
| `.headers` | ─ | ─ | ─ |
| `.dump` | ─ | ─ | ─ |
| `.import` | ─ | ─ | `s.Prepare()` / `s.PrepareContext()` |
| ─（起動時に暗黙実行） | ─ | ─ | `engine.OpenSelf()` |
| ─（ライブラリ専用） | ─ | ─ | `engine.Open(path string)` |
| ─（ライブラリ専用） | ─ | ─ | `db.Session(ctx context.Context)` |
| ─（ライブラリ専用、`.load`のio.Reader版） | ─ | ─ | `db.LoadFrom(r io.Reader)` |
| ─（REPLの複数行入力判定に使う内部実装） | ─ | ─ | `engine.Complete(sql string) (bool, error)` |
| ─（バナーとして起動時に1度表示） | ─ | `inspect` | ─（下記参照） |
| ─ | `-c`/`--command` | ─ | ─ |
| ─ | `--serve-stdio` | ─ | ─ |
| ─ | `-r`/`--read-only` | ─（`inspect`応答の`read_only`で確認可） | ─（`engine`は概念を持たない） |
| ─ | `-i`/`--snapshot-interval` | ─ | `db.Snapshot()`（`.snapshot`と共用） |
| ─ | `-q`/`--quiet`, `-v`/`--version`, `-h`/`--help` | ─ | ─ |

`engine.OpenSelf()` / `engine.Open()` / `db.Session()` / `db.LoadFrom()` は
DBの生成・ロード・接続取得方法であり、ユーザー操作に直接対応するコマンドでは
ないため、他の列が空欄になるのは意図通り（4レイヤー対応の原則が崩れている
わけではない）。

`.mode`/`.headers`/`.dump`/`.import`は`engine` APIの対応を持たない、
`cmd/san-db-ox`内で完結するコマンドである（スキーマ内省・出力整形・CSV変換の
ロジックを`engine`側のAPIとして切り出さない、という方針）。`.import`が使う
`s.Prepare`/`s.PrepareContext`のみが例外的に`engine`への追加になる。

### `inspect` は名前が同じでも意味が違う（要注意）

**stdio の `inspect` op と `engine.Inspect(path)` は別物である。**

- **stdio の `inspect`**: 引数を取らず、**自プロセスの状態**（`has_data`,
  `version`, `data_length`, `source`, `read_only`）を返す。REPLの起動バナーと
  同じ情報をstdioクライアントへ提供するためのもの。
- **`engine.Inspect(path)`**: 任意のパスのファイルを検査し、種別とフッター情報を
  返す。`.load`実行前のバージョン不一致警告（仕様書§4）のために`cmd`側が使う。

**任意のパスを検査する機能をstdio opとして露出させないこと。** 外部公開時
（仕様書§8）にサーバー側のファイルシステムを探る手段になるため、意図的に
非対称にしている。この例外は仕様書§7に明記されている。

### 公開しないAPI

**ユーザー定義スカラー関数の登録API（`RegisterScalarFunction`等）は公開しない。**
`engine`のAPI表面積を最小に保つための決定。必要になった場合はホストアプリ側で
ドライバへ直接登録する。

## 短縮フラグ（CLI起動オプション）

- 短縮フラグは1文字、**すべて小文字**で統一する（`-H`/`-S` のような大文字は
  使わない）。大文字小文字の混在は覚えにくさに直結するため。
- `-h` は `--help` の定位置として予約済み。他のオプションで `-h` を使わないこと。
- **すべてのフラグに短縮形を用意する必要はない。** 割り当てられる小文字が無い、
  あるいは短縮する必然性が薄いフラグには短縮形を設けない（現時点では
  `--serve-stdio` が該当）。**「必ずセットで用意する」という規則は採らない**——
  無理に1文字を割り当てると、意味の連想が効かない記号を増やすことになるため。
- 現在の割り当て: `-c` `-m` `-o` `-q` `-t` `-i` `-r` `-v` `-h`。

## ファイル名生成（タイムスタンプ付与）

`.snapshot` や `--snapshot-as` で生成するファイル名にタイムスタンプ
（`_YYYYMMDDHHMMSS`）を付与する際のルールは以下の1つに統一する。新しい保存系
コマンド・オプションを追加する場合も、このルールを踏襲すること。

1. ベースとなるファイル名（拡張子を除く部分）から、既存の `_YYYYMMDDHHMMSS`
   パターンがあれば取り除く（二重付与を防ぐ）。
2. 拡張子の直前に新しい `_YYYYMMDDHHMMSS` を挿入する。
3. 拡張子が省略されている場合、実行ファイル出力（既定）ならWindows環境で
   `.exe` を、SQLiteファイル出力（`--sqlite`）なら全OSで `.sqlite` を付与する。

**ベース名にハイフンが含まれていてもよい。** 区切り文字は `_` 固定のため、
`san-db-ox` はそのまま扱える（`san-db-ox_20260901120000.exe`）。照合の正規表現を
書く際はこれを前提にすること。

`--timestamp`（`-t`）は bool フラグ（付与する/しない）であり、`auto`/`always`/
`never` のような複数値の選択肢は持たない。

## Goコードの命名

- `gofmt` / `go vet` に従う。標準的なGoの慣習（エクスポート名はCamelCase、
  略語は`ID`/`URL`のように全大文字）を踏襲する。
- エラー変数は `Err` プレフィックス（`ErrNoData`, `ErrReadOnly` 等）。
- ドットコマンドの実装関数は、コマンド名に対応させる（`.snapshot` →
  `cmdSnapshot`）。stdio opのハンドラも同様（`snapshot` op → `opSnapshot`）。
  **同じ操作のREPL実装とstdio実装が別々のロジックを持たないよう、共通処理は
  1つの関数に集約する**（`directory-structure.md`参照）。
