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

**フェーズ①Step 3（`engine`パッケージ最小実装）完了。** `cmd/san-db-ox` は
まだ存在しない（Step 4で着手）。

- **Step 1（足場固め）**: `go.mod`/`go.sum`（`modernc.org/sqlite v1.58.0`）・
  `Makefile`・`.gitattributes`/`.gitignore`・`PostToolUse`フック。
- **Step 2（技術検証スパイク）**: 既存の確定事項（上表）が
  `modernc.org/sqlite v1.58.0` でも成立することを使い捨てスパイクで再確認、
  未確認事項1番（`Serialize()`出力の妥当性）を解消。詳細は過去のコミット
  （`33dcd41`）参照。
- **Step 3（`engine`パッケージ最小実装）**: `engine/`に以下を実装。
  - `footer.go`: 32バイト固定長フッターのエンコード/デコード
    （`Magic`/`FooterSize`/`FormatVersion`/`MaxDataSize`を公開定数化、
    パース結果自体は`footerInfo`として非公開——`Inspect`公開APIは
    フェーズ②で追加）。
  - `serialize.go`/`backup.go`: `conn.Raw()`型アサーションによる
    `Serialize`/`Deserialize`/`NewBackup`到達、`loadBlobInto`
    （使い捨て接続へDeserialize→Backup APIでkeeper接続の生きているDBへ
    コピー、の2段構え）。
  - `engine.go`: `DB`型（`mu`/`sdb`/`keeper`/`dsn`/`closed`）、
    `Open`/`OpenSelf`/`Exec`/`ExecContext`/`Query`/`QueryContext`/
    `QueryRow`/`QueryRowContext`/`Close`。`Open`は素のSQLiteファイルの
    バイト列をそのままDeserializeする単純な実装（フッター判定はしない
    ——それは`Load`の役目でフェーズ②）。DB名は`san-db-ox<連番>`で
    プロセス内の複数DBインスタンスの衝突を避ける。
  - `persist.go`: `Snapshot`/`Overwrite`。**設計判断:** `Snapshot`は
    `Open`/`OpenSelf`のどちらで開いたかに依存せず、呼び出し時点の
    `os.Executable()`を毎回読み直してエンジンバイトを決定する（仕様書§10
    「ここでの『エンジンバイト』はホストアプリのバイナリ全体」との整合を
    優先し、ExecDBが採用していた「Open時にsourcePath/engineSizeを
    キャッシュする」方式は採らなかった——設計判断は別物、CLAUDE.md）。
    `serializeBarrier`（`BEGIN IMMEDIATE`で書き込みロックを取ってから
    `Serialize`する、torn snapshot防止）、`overwriteSelf`（rename退避→
    新規書き込み→退避削除）、`looksLikeGoRunTempBinary`（`go run`の
    一時バイナリを拒否）も実装。
  - `errors.go`: `ErrClosed`/`ErrNotOverwritable`/`ErrBusy`/`ErrTooLarge`。
  - テスト: `footer_test.go`/`engine_test.go`/`persist_test.go`
    （20件、`go test`・`-race`とも green）。`.overwrite`の実挙動は
    `go test`からは検証できない（testing.md）ため、ユニットテストでは
    `overwriteSelf`を直接呼んで退避・書き込み・ロールバックの手順を検証し、
    別途 `make build`相当の実バイナリ（使い捨てスパイク、削除済み）で
    `OpenSelf`→`Overwrite`→再起動→`OpenSelf`のループを3周させ、データが
    実際に永続化されること・退避ファイルが残らないことを実機確認した。
  - `net`/`net/http`を直接importしていないことを確認済み（CI化はStep 5）。

**次にやること:** フェーズ①Step 4（`cmd/san-db-ox` — CLI・バナー・REPL
最小実装）。

## 未確認事項（実装前に決める・確かめる）

1. ~~`Serialize()` の出力が、そのまま有効なSQLiteファイルとして開けるか。~~
   **Step 2で解消（実測確認済み。仕様書§6参照）。**
2. **REPLのプロンプト文字列。** 仕様書に記載がない。`sandbox> ` を仮置きして
   いるが（`docs/usage/repl-commands_ja.md` の例）、製品名の表記規則
   （`naming.md`）の第3段を使ってよいかを含めて決める必要がある。
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
