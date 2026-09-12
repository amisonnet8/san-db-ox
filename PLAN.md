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

**フェーズ①（ミニマム実装）完了。Step 1〜5すべて完了。** ローカルでの
`make check`・`make test`はgreen。GitHub Actions 3OSマトリクス
（`test.yml`）は今回のセッションで新規作成したのみで、実際のCI実行結果は
（pushはユーザーが行うため）未確認——**次にリポジトリがpushされ、CIが
green であることを確認したら、正式にフェーズ①完了としてフェーズ②へ進む。**

- **Step 1（足場固め）**: `go.mod`/`go.sum`（`modernc.org/sqlite v1.58.0`）・
  `Makefile`・`.gitattributes`/`.gitignore`・`PostToolUse`フック
  （コミット `529e2fd`）。
- **Step 2（技術検証スパイク）**: 確定事項（上表）の再確認、未確認事項1番
  （`Serialize()`出力の妥当性）を解消（コミット `33dcd41`）。
- **Step 3（`engine`パッケージ最小実装）**: `DB`型、
  `Open`/`OpenSelf`/`Exec`系/`Close`/`Snapshot`/`Overwrite`。`Snapshot`は
  `os.Executable()`を呼び出しのたびに読み直す設計（ExecDBの
  sourcePath/engineSizeキャッシュ方式とは意図的に変更）。ユニットテスト20件
  （コミット `c912358`）。
- **Step 4（`cmd/san-db-ox`最小実装）**: `main.go`/`banner.go`/`repl.go`/
  `dotcmd.go`/`filename.go`。`-h`/`-v`のみのCLI、起動バナー、
  `db.Query()`一本化のSQL実行（`.mode list`相当の固定出力）、ドットコマンド
  6種（`.tables`/`.schema`/`.snapshot`/`.overwrite`/`.exit`・`.quit`/`.help`）
  を`dispatchDotCommand`に集約。REPLプロンプトは`SanDBox> `（表示名採用、
  ユーザー指示により`sandbox> `から変更）（コミット `f16a3a4`、`4eb45ed`）。
- **Step 5（E2E自動化・CI）**: 今回のセッションで実施。
  - **`tests/e2e.sh`**（`make test`が`build`に依存して実行）: REPL基本操作
    （CREATE/INSERT/.tables/.schema/SELECT/エラー処理）、`-h`/`-v`/不正引数
    （終了コード2）、`.exit CODE`/EOF終了、`.snapshot`（明示名・既定名
    双方）、`.overwrite`（2周、退避ファイル残留なし確認）、**複数プロセス
    独立性**（同一バイナリのコピーを2プロセス同時起動してDBが独立している
    ことを確認）、**同一バイナリへの並行読み取り**（4プロセスが同じ実行
    ファイルのフッターを同時に読んでも競合しない）、**`.snapshot`の並行
    書き込み**（2プロセスが同じ出力先へ同時に`.snapshot`しても壊れた
    ファイルが観測されない——アトミックなrename方式の実地検証）、
    `go install`で入れたバイナリでの`.overwrite`動作、を検証。3回連続実行で
    フレーキーでないことを確認済み（Linux）。
  - **見つけたバグ:** `Makefile`の`build`が`go build -o san-db-ox`のまま
    だと、Windowsでは`.exe`拡張子が付かずnaming.md違反になることが判明
    （`-o`指定時はGoが拡張子を自動補完しない）。`BIN :=
    san-db-ox$(shell go env GOEXE)`変数を導入して修正。
  - **`make netcheck`を新設**し`check`に組み込み: `engine`・`cmd/san-db-ox`
    が`net`/`net/http`を直接importしていないこと、`net/http`が推移的にも
    現れないことを`go list`で検証（`net`自体は`modernc.org/libc`経由で
    許容——testing.md参照）。
  - **`.github/workflows/test.yml`を新規作成**（ExecDBの構成を参考に、
    SanDBox向けに`make test`（e2e）もCIマトリクスに含める形へ拡張——
    ExecDB自身のCIは`make check`のみでe2eは含めていなかったが、PLAN.mdの
    フェーズ①完了判定が「Windowsでの`.overwrite`含む」e2eのgreenを要求する
    ため）。`check`ジョブ（3OS×`make check`+`make test`）、`race`ジョブ
    （ubuntu/macosのみ）、`trivy`ジョブ（脆弱性・ライセンス）。
  - **ルール追記:** `testing.md`に2件（REPLプロンプトが改行なしのため
    `grep -qx`ではなく部分一致を使うべきこと／`windows-latest`に`make`が
    無く`choco install make -y`が必要なこと）。`distribution.md`の
    `go install`動作確認を実測確認済みに更新（未確認事項4番を解消）。

**次にやること:** リポジトリをpush → GitHub Actions 3OSマトリクスが
greenであることを確認 → フェーズ②（`engine`ライブラリ本格開発:
`Session`/`Load`/`Export`/`Inspect`/`LoadFrom`/`Complete`）に着手。

## 未確認事項（実装前に決める・確かめる）

1. ~~`Serialize()` の出力が、そのまま有効なSQLiteファイルとして開けるか。~~
   **Step 2で解消（実測確認済み。仕様書§6参照）。**
2. ~~REPLのプロンプト文字列。~~ **Step 4で解消。** `SanDBox> ` に確定
   （表示名をそのまま使用し、起動バナーの`SanDBox v0.1.0`と表記を揃える）。
   仕様書§0の表記スロット表に追記済み。
3. **`.dump` と `.import` に stdio op を用意するか。** 仕様書§7のop一覧には
   無いが、「ドットコマンドはstdioでは対応するopとして提供する」と書いた
   手前、この2つだけが例外になる。意図的な非対応か、単なる漏れかを確定させる
   （`.import` を op 化する場合、ファイルパスを受け取るため `--read-only` との
   兼ね合いも決める）。
4. ~~`go install` で入れたバイナリでフッター方式が機能するか。~~
   **Step 5で解消。** `tests/e2e.sh`（`make test`）で`GOBIN`指定の
   `go install`→`.overwrite`→再起動→`SELECT`のループを自動検証する形にし、
   Linuxで実測確認済み。Windows/macOSはGitHub Actions 3OSマトリクスが同じ
   `tests/e2e.sh`を実行することで継続的に担保する（`.claude/rules/
   distribution.md`）。

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
