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

**フェーズ①（ミニマム実装）はStep 1〜5の実装自体は完了しているが、
GitHub Actions 3OSマトリクス（`test.yml`）はまだgreenになっていない。**
ユーザーがpush→CI実行→エラー報告、を2周行い、**Windows特有の実バグを
2件発見・修正済み**（下記）。ローカル（Linux）では`make check`・
`make test`・`make race`すべてgreen。**次にユーザーが再度pushしてCIが
greenになることを確認できたら、正式にフェーズ①完了としてフェーズ②へ進む。**

### Windows CIで発見した実バグ（Step 5後の修正、pushフィードバックより。現在3周目）

1回目のpush→CI: `check (windows-latest)`が`TestCmdSnapshotDefaultAndExplicitName`
で失敗。原因は`cmd/san-db-ox/dotcmd_test.go`が拡張子なしのファイル名を
期待値に使っていたが、Windowsでは`snapshotFilename`が`.exe`を自動付与する
ため実際の出力先とズレていた（テスト側の不備）。`snapshotFilename(...,
runtime.GOOS)`で期待値自体を計算する形に修正。**この時点でついでに
`tests/e2e.sh`側にも同型の不備（`.snapshot`の明示名・競合テストのターゲット
パスが拡張子なし）を発見し先回りで修正**（コミット`3113bbd`、`f31b45e`）。

2回目のpush→CI: `make test`が2箇所で失敗。
1. **`Error: rename ... Access is denied.`**（`.snapshot`が自分自身の
   パスへ書き込もうとした際）。`FILENAME`省略時のデフォルト名（実行中
   バイナリ名がベース）はCWDが実行ファイルの場所と同じなら自分自身の
   パスと一致するため、これは実際の使用シーンでも起こりうる本物の
   バグだった。**Linuxでは一時ファイル＋`rename`が自分自身のパスに
   対しても成功する（偶然）が、Windowsでは失敗する。** `engine.Snapshot`が
   保存先を自分自身と検知した場合、`.overwrite`と同じ退避方式
   （`overwriteSelf`）に自動的に切り替えるよう修正（`samePath`ヘルパー、
   `go run`一時バイナリガードも同じ分岐に適用）。仕様書§11に追記。
2. **`.san-db-ox.old`退避ファイルが`.overwrite`直後に残っていた。**
   これは**バグではなく仕様書§11で最初から想定されていた挙動**
   （Windowsは実行中は退避ファイルを削除できず、次回起動時に
   ベストエフォートで削除される）。`tests/e2e.sh`のアサーションが
   「`.overwrite`直後」に確認していたのが誤りで、「次にバイナリを
   起動した後」に確認する形へ修正。あわせて`.snapshot`の自己上書き
   検証も「ファイルが存在するだけ」の弱いアサーションから、
   「新しいデータが実際に読めること」まで確認する形に強化（でないと
   1番のバグを検出できなかった）。
   （コミット `c23548c`）

3回目のpush→CI: 2回目の修正（`samePath`）にもかかわらず**同じ
`Access is denied`エラーが再発。** 原因は`samePath`の実装そのものが
甘かったこと——`filepath.Abs`＋`Clean`＋大文字小文字無視の**パス文字列
比較だけ**で「自分自身かどうか」を判定していたが、Windowsでは
`os.Executable()`が返すパスと`os.Getwd()`から組み立てたパスが、
**同じファイルを指していても文字列としては異なる表現になりうる**
（例: 一方にだけ短縮8.3形式のパス要素`RUNNER~1`が含まれる、とCIの
エラーメッセージから推測）。**両ファイルが実在するなら`os.SameFile`
（OSレベルのファイル同一性、Windowsはボリューム＋ファイルインデックス）
で判定するよう修正**——パス文字列比較は、比較対象がまだ存在しない
（`.snapshot`の典型的な使われ方）場合のみのフォールバックとした。
仕様書§11・testing.mdを実装に合わせて更新（コミット `6bddbf9`）。

**教訓:** Windows特有の問題は、1回の修正で仕留められるとは限らない
（今回は3周目でようやく根本原因＝パス比較ロジック自体の甘さに到達した）。
「パスが同じかどうか」の判定は、可能な限り最初から`os.SameFile`を使うべき
だった。

この3周で見つけた教訓は`testing.md`に4件追記済み（`grep -qx`ではなく
部分一致を使うこと、退避ファイルの削除タイミングはOS依存、`.snapshot`の
自己上書き検証は弱いアサーションで済ませないこと、パスの同一性判定は
文字列比較ではなく`os.SameFile`を使うこと）。

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
