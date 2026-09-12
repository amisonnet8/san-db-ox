# PLAN

SanDBoxの実装計画・進捗管理ドキュメント。実装が進むにつれて随時更新すること
（特に「現在地」「保留事項」は、セッションをまたぐたびに参照・更新する）。

## 開発フェーズ

実装は以下の5フェーズで進める。各フェーズとも、まずLinuxで開発し、完成後に
Windows/macOSや複数CPUアーキテクチャでの動作確認（GitHub Actions等）を行う
サイクルを繰り返す（詳細は `.claude/rules/testing.md` 参照）。

1. **①ミニマム実装（技術検証フェーズ）**: `engine`ライブラリ・REPLの2要素を、
   最小限の機能で薄く繋げて実装し、全体が技術的に成立するかを確認する。
   網羅性は求めない。`.overwrite`（自己上書き）もこのフェーズに含める
   （一番ハック的な部分であり、早期に全体の中で動作確認する価値が高いため）。
2. **②`engine`ライブラリ開発**: ①の土台の上で本格的に作り込む。`Session`、
   `Load`（種別自動判別）、`Export`、`Inspect`、`LoadFrom`、`Complete`。
3. **③REPL開発**: REPLコマンド体系を本格的に作り込む。ドットコマンド一式、
   出力モード5種、Ctrl+Cの状態機械。
4. **④バッチ実行・stdioプロトコル開発**: `-c`/stdinによる非対話実行、終了
   コード、JSON Lines プロトコル、op一式、`--read-only`。
5. **⑤ドキュメント・配布**: `docs/examples/`・`docs/tour/`の作成、README、
   `release.yml`、英語版ドキュメントの整備。

## 前身プロジェクトから引き継いだ確定事項

以下は前身プロジェクト **ExecDB** で実測検証済みの事実であり、SanDBoxでも
そのまま前提としてよい（詳細は仕様書§11）。フェーズ①では**新規の技術検証
ではなく、同じ構成が成立することの再確認**として扱う。

| 項目 | 確定内容 |
| :--- | :--- |
| `Serialize`/`Deserialize` | `modernc.org/sqlite` の `conn` がメソッドとして公開。`database/sql` の `conn.Raw()` からinterface型アサーションで到達する。スキーマ名の引数は無い |
| 復元後の書き込み可否 | `SQLITE_DESERIALIZE_RESIZEABLE` 指定のため、復元後も通常どおり拡張可能 |
| 接続共有モデル | `file:/<n>?vfs=memdb` を採用。`mode=memory&cache=shared` は `SQLITE_LOCKED_SHAREDCACHE` で `busy_timeout` が効かず、`context` でもキャンセルできずハングするため**不採用** |
| keeper接続 | 全接続が閉じるとストアが解放されるため、`Close()` まで保持する接続を1本持つ |
| `Deserialize` の反映範囲 | 呼び出したコネクションにしか反映されない。そのため使い捨て接続へ`Deserialize` → Backup API で生きているDBへコピー、という2段構えが必要 |
| 実効的なサイズ上限 | `memdb` VFS の既定上限により約1GiB（実測で約960MiB付近で `SQLITE_FULL`） |
| `.overwrite` の書き込み手順 | rename退避 → 新規書き込み → 退避削除。Linux/Windows双方でPoC検証済み |
| フッター方式 | 末尾32バイト固定長。OSローダーの実行に影響しない |

## フェーズ①のステップ

1. **Step 1: 足場固め** — `go.mod`、`Makefile`（`build`/`test`/`check`/`fmt`/
   `race`）、`.gitignore`、`.gitattributes`（`* text=auto eol=lf`）、`LICENSE`
   （MIT）。`modernc.org/sqlite` の取得。**`Makefile` ができた時点で
   `PostToolUse` フック（`make build`）の設定を提案する。**
2. **Step 2: 技術検証スパイク【意思決定ゲート】** — 上表の確定事項が
   `modernc.org/sqlite` の現行バージョンでも成立することを実測で再確認する。
   **加えて、SanDBoxで新規に前提としている未確認事項（下記「未確認事項」の
   1番）を検証する。**
3. **Step 3: `engine` パッケージ（最小）** — フッターI/O、`DB` 型のSQL実行API、
   `Open`/`OpenSelf`/`Snapshot`/`Overwrite`/`Close`。
4. **Step 4: `cmd/san-db-ox` — CLI・バナー・REPL（最小）** — 起動オプション
   （`-c`/`--serve-stdio`/`-i`/`-r` は先送り）、ドットコマンド
   （`.tables`/`.schema`/`.snapshot`/`.overwrite`/`.exit`/`.help` のみ）、
   `.overwrite` の実機確認。
5. **Step 5: E2E自動化・CI** — `tests/e2e.sh`、GitHub Actions 3OSマトリクス
   （`test.yml`）、`net`/`net/http` 非依存の検証、仕様書・ルールファイルへの
   確定事項の反映。

**フェーズ①の意図的なスコープ外（フェーズ②〜④で拾う）:** `Session`、
`Load`、`Export`、`Inspect`、`LoadFrom`、`Complete`、`.load`/`.import`/
`.dump`/`.mode`/`.headers`、出力モード、Ctrl+Cの状態機械、バッチ実行、
stdioプロトコル、`--read-only`、`--snapshot-interval`。

**フェーズ①完了の判定:** `make check`・`make test` が通る／REPLから
`.snapshot`・`.overwrite` が機能する／GitHub Actions 3OSマトリクスがgreen
（Windowsでの `.overwrite` 含む）／`go install` で入れたバイナリでも
`.snapshot`/`.overwrite` が機能する／`engine`・`cmd` が `net` を直接
importしていないことをCIが検証している。

## 現在地

**フェーズ①Step 4（`cmd/san-db-ox` — CLI・バナー・REPL最小実装）完了。**
フェーズ①の残りはStep 5（E2E自動化・CI）のみ。

- **Step 1（足場固め）**: `go.mod`/`go.sum`（`modernc.org/sqlite v1.58.0`）・
  `Makefile`・`.gitattributes`/`.gitignore`・`PostToolUse`フック
  （コミット `529e2fd`）。
- **Step 2（技術検証スパイク）**: 確定事項（上表）の再確認、未確認事項1番
  （`Serialize()`出力の妥当性）を解消（コミット `33dcd41`）。
- **Step 3（`engine`パッケージ最小実装）**: `footer.go`/`serialize.go`/
  `backup.go`/`engine.go`/`persist.go`/`errors.go`。`DB`型、
  `Open`/`OpenSelf`/`Exec`系/`Close`/`Snapshot`/`Overwrite`。
  **設計判断:** `Snapshot`は`Open`/`OpenSelf`のどちらで開いたかに依存せず、
  呼び出し時点の`os.Executable()`を毎回読み直してエンジンバイトを決定する
  （仕様書§10「ここでの『エンジンバイト』はホストアプリのバイナリ全体」に
  整合させるため、ExecDBの「Open時にsourcePath/engineSizeをキャッシュする」
  方式とは意図的に変えた——設計判断は別物、CLAUDE.md）。ユニットテスト20件
  （`go test`・`-race`ともgreen）＋実バイナリでの`OpenSelf`→`Overwrite`→
  再起動ループの実機確認（コミット `c912358`）。
- **Step 4（`cmd/san-db-ox`最小実装）**: `main.go`/`banner.go`/`repl.go`/
  `dotcmd.go`/`filename.go`を実装。
  - **起動:** 引数なしでREPL起動（`engine.OpenSelf()`）。`-h`/`--help`、
    `-v`/`--version`のみ対応、それ以外の引数は使用法エラー（終了コード2）。
    `-c`/`--serve-stdio`/`-i`/`-r`/`-m`/`-o`/`-t`/`-q`はすべて未実装
    （後続フェーズ）。
  - **バナー（仕様書§13）:** `engine.DB`に`HasData() bool`を追加
    （`Open`/`OpenSelf`が実データを読み込んだ場合に`true`。`Inspect`/
    `Info()`公開APIはフェーズ②で追加、Step 4のバナー実装に最小限必要な
    ものだけ先に切り出した）。
  - **REPLプロンプト:** `sandbox> `に確定（仕様書§0の表記スロット表・
    未確認事項2番を参照）。
  - **SQL実行方式:** `db.Query()`一本化で実測確認済み——DDL/DML/SELECTの
    いずれも`Query()`が使え、非SELECT文は0カラムの空結果になるだけで
    エラーにならない（modernc.org/sqlite実測）。出力は`.mode list`相当
    （`|`区切り・ヘッダなし・NULLは空文字列）に固定、本格的な出力モード
    切替はフェーズ③。
  - **ドットコマンド（6種、共通処理として`dispatchDotCommand`に集約
    ——directory-structure.mdの「3つの実行モードは同じドットコマンド
    実装を共有する」原則を先取り）:** `.tables`/`.schema [TABLE]`/
    `.snapshot [FILENAME]`（`--sqlite`/`--timestamp`は未対応、
    ファイル名省略時は実行中バイナリ名をベースにカレントディレクトリへ
    生成）/`.overwrite`/`.exit [CODE]`・`.quit [CODE]`/`.help`
    （実装済みコマンドのみ列挙）。
  - **テスト:** `filename_test.go`/`dotcmd_test.go`/`repl_test.go`
    （`go test`・`-race`ともgreen）。`.snapshot`の既定ファイル名が
    「実行ファイルの場所」ではなく「プロセスのカレントディレクトリ」
    基準であることを`t.Chdir`で明示的に検証（testing.mdのe2e落とし穴
    節と同じ注意点）。
  - **実機確認（`.overwrite`）:** `make build`相当の実バイナリをコピーし、
    REPL経由で`.overwrite`→再起動→`SELECT`→追記→`.overwrite`→再起動→
    `SELECT`を3周実行。データの永続化・退避ファイル（`.san-db-ox.old`）が
    残らないことを確認。`.snapshot`（名前省略・明示指定の両方）・
    `-h`/`-v`/不正引数（終了コード2）・`.exit CODE`・EOF終了・`.quit`も
    実機確認済み。
  - **Makefile修正:** `build`ターゲットが`go build ./...`のままだと
    複数パッケージ扱いになりバイナリが一切生成されないことが判明
    （`go help build`: 複数パッケージ指定時は出力を捨てて構文チェックのみ）。
    `go build -o san-db-ox ./cmd/san-db-ox`に修正。

**次にやること:** フェーズ①Step 5（E2E自動化・CI）——`tests/e2e.sh`、
GitHub Actions 3OSマトリクス（`test.yml`）、`net`/`net/http`非依存の検証を
CI化、仕様書・ルールファイルへの確定事項の反映。

## 未確認事項（実装前に決める・確かめる）

1. ~~`Serialize()` の出力が、そのまま有効なSQLiteファイルとして開けるか。~~
   **Step 2で解消（実測確認済み。仕様書§6参照）。**
2. ~~REPLのプロンプト文字列。~~ **Step 4で解消。** `sandbox> ` に確定
   （第3段——対話中に1行ごと手で打つ文字列という長さ制約。`sqlite3` CLI
   自身が製品名ではなく`sqlite> `を使う前例と同じ理由）。仕様書§0の
   表記スロット表に追記済み。
3. **`.dump` と `.import` に stdio op を用意するか。** 仕様書§7のop一覧には
   無いが、「ドットコマンドはstdioでは対応するopとして提供する」と書いた
   手前、この2つだけが例外になる。意図的な非対応か、単なる漏れかを確定させる
   （`.import` を op 化する場合、ファイルパスを受け取るため `--read-only` との
   兼ね合いも決める）。
4. **`go install` で入れたバイナリでフッター方式が機能するか。** ビルド
   キャッシュや `GOOS`/`GOARCH` の扱いに依存するため、実機で一度確認する
   （`.claude/rules/distribution.md`）。

## 保留事項

- **`san-db-ox-clients` 側へ引き継ぐノートが未作成。** 作業ノート
  （`san-db-ox_note_ja.md`、リポジトリ外）の付録A（提供する言語と優先順位、
  ドライバ共通要件、ODBCを対象外とした理由、外部公開時のセキュリティ）は、
  本体の仕様書からは落としてある。`san-db-ox-clients` を起こす際に、そちらの
  ノートとして引き継ぐこと。
- ~~`PostToolUse` フックが未設定。~~ **設定済み**（Step 1完了時）。
  `.claude/hooks/build-on-go-change.sh` が `.go`/`go.mod`/`go.sum` 編集後に
  `make build` を実行し、失敗時は `decision: block` で理由をClaudeへ返す
  （`.claude/settings.json`）。
- **devcontainer.json 反映待ちリスト**: 現行コンテナはリビルドせずに開発を
  進める方針（都度手動でインストール・設定して進め、区切りでまとめて
  `devcontainer.json` へ反映する）。session内で手動インストール・設定を
  行った場合は、忘れずにここへ追記すること。
  - （なし）
- **`docs/examples/`・`docs/tour/` は未作成。** 実装完了後（フェーズ⑤）に
  作成する。
- **英語版ドキュメントは未作成。** 日本語版が固まってから、フェーズ⑤で
  まとめて整える。
- **`san-db-ox-clients`（各言語向けドライバ）は別リポジトリ。** 本リポジトリ
  では扱わない。本体側に残す接続テストはGoで書いたstdioクライアントのみ
  （`.claude/rules/testing.md`）。
