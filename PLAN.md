# PLAN

SanDBoxの実装計画・進捗管理ドキュメント。実装が進むにつれて随時更新すること
（特に「現在地」「保留事項」は、セッションをまたぐたびに参照・更新する）。

## 開発フェーズ

実装は以下の5フェーズで進める。各フェーズとも、まずLinuxで開発し、完成後に
Windows/macOSや複数CPUアーキテクチャでの動作確認（GitHub Actions等）を行う
サイクルを繰り返す（詳細は `.claude/rules/testing.md` 参照）。

1. **①ミニマム実装（技術検証フェーズ）【完了】**: `engine`ライブラリ・REPLの
   2要素を、最小限の機能で薄く繋げて実装し、全体が技術的に成立するかを
   確認する。網羅性は求めない。`.overwrite`（自己上書き）もこのフェーズに
   含める（一番ハック的な部分であり、早期に全体の中で動作確認する価値が
   高いため）。
2. **②`engine`ライブラリ開発【完了】**: ①の土台の上で本格的に作り込む。
   `Session`、`Load`（種別自動判別）、`Export`、`Inspect`、`LoadFrom`、
   `Complete`。
3. **③REPL開発【完了】**: REPLコマンド体系を本格的に作り込む。ドットコマンド
   一式、出力モード5種、Ctrl+Cの状態機械。
4. **④バッチ実行・stdioプロトコル開発【完了】**: `-c`/stdinによる非対話実行、
   終了コード、JSON Lines プロトコル、op一式、`--read-only`。
5. **⑤ドキュメント・配布【完了】**: `docs/examples/`・`docs/tour/`の作成、
   README、`release.yml`、英語版ドキュメントの整備。

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

## フェーズ②のステップ

スコープは `engine` パッケージのみ（`cmd/san-db-ox`・`tests/e2e.sh` は
フェーズ③まで変更しない）。

0. **Step 0: 仕様書の更新＋技術検証スパイク** — `docs/spec/san-db-ox_spec_ja.md`
   §4/§6/§10/§11 を実装前に更新。あわせて2点を使い捨てテストで実測。
1. **Step 1: 内部リファクタ＋バグ修正**（公開API変更なし）— `backupInto` の
   `Finish()` 漏れ修正、`decodeFooter` の切り出し、`tempFileFor`/
   `removeTempArtifacts`/`fileDSN` の追加、新規エラー変数の追加（未使用）。
2. **Step 2: `Inspect` / `FileInfo`** — `engine/inspect.go`。
3. **Step 3: `Load` / `LoadFrom`＋`Open`の載せ替え** — `engine/load.go`。
4. **Step 4: `Export`** — `engine/export.go`。
5. **Step 5: `Session`** — `engine/session.go`。
6. **Step 6: `Complete`** — `engine/complete.go`（ExecDB踏襲、`Xsqlite3_complete`）。
7. **Step 7: 並行性テストと仕上げ** — `make race`、`PLAN.md`/ルールファイル更新。

## フェーズ③のステップ

スコープは `cmd/san-db-ox` と `tests/e2e.sh`（`engine`は原則変更しない。
例外は`TestSessionSurvivesCanceledQuery`の追加のみ）。

0. **Step 0: 仕様書・ルールファイルの更新** — 継続プロンプト、REPLが
   `engine.Session`を1本保持する設計、`.mode`/`.headers`の相互作用、
   `--snapshot-interval`失敗時の挙動、非対話時のプロンプト抑制を実装前に
   仕様書へ反映。
1. **Step 1: REPL基盤の作り直し** — 単一`Session`・複数行/複数文入力
   （`engine.Complete`ベースの`splitComplete`）・非対話判定
   （`isInteractive`）。
2. **Step 2: 出力モード5種 + `.mode`/`.headers`** — `format.go`/`value.go`。
3. **Step 3: CLI起動オプション** — `-m`/`-o`/`-t`/`-q`/`-i`、
   `naming.md`のファイル名規則の完全実装、`--snapshot-interval`の
   バックグラウンドgoroutine。
4. **Step 4: `.snapshot`（`--sqlite`/`--timestamp`）完成＋`.load`追加。**
5. **Step 5: `.dump`/`.import`追加。**
6. **Step 6: Ctrl+Cの状態機械** — `interrupt.go`（ExecDB踏襲）。
7. **Step 7: `tests/e2e.sh`拡張・ドキュメント更新・仕上げ。**

## フェーズ④のステップ

スコープは仕様書§5（バッチ実行）・§7（stdioプロトコル）・§2/§12
（`--read-only`）。前身プロジェクトExecDBに`-c`・stdio・`--read-only`の
対応実装が無いため、全て新規設計（`PLAN.md`本文とは別に立てた実装計画の
要点のみここに記録する）。

0. **Step 0: 仕様書の更新** — stdioのhello行・`dump` op・エラーコード
   割り当て方針を仕様書§7へ追記。`-c`/stdinスクリプトの「各引数は独立した
   実行単位、末尾の`;`省略時は引数の終わりで暗黙補完」というルールを
   §5へ追記。`--read-only`の実装方針（`query_only`プラグマ＋個別チェック）
   を§2へ追記。
1. **Step 1: CLI起動オプション拡張** — `-c`/`--command`（`stringSliceFlag`
   による複数回指定対応）、`--serve-stdio`、`-r`/`--read-only`、および
   両者の排他関係（`--serve-stdio`+`-c`、`--read-only`+`-i`）を
   `parseOptions`へ追加。
2. **Step 2: ドットコマンド実装のロジック/出力分離** — `cmdSnapshot`→
   `doSnapshot`、`cmdLoad`→`doLoad`、`cmdOverwrite`→`doOverwrite`、
   `cmdTables`→`listTables`、`cmdSchema`→`schemaSQL`、`cmdDump`→
   `dumpSQL`（`dump.go`の各関数を`io.Writer`引数化）。stdioのop実装が
   REPLの`cmd*`と同じロジックを再利用できるようにする
   （`directory-structure.md`の原則をバッチ・stdioへ拡張）。
3. **Step 3: バッチ実行モード** — 新規`batch.go`。`handleDotCommand`/
   `execSQL`の戻り値へ`error`を追加（REPLは無視して継続、バッチは中断に
   使う）。`splitComplete`を`*repl`メソッドからパッケージ関数へ格上げ。
   `main.go`のモード判定を`--serve-stdio`／`-c`／非対話stdin／REPLの
   4分岐に再構成。
4. **Step 4: stdioプロトコル** — 新規`stdio.go`。hello行、JSON Lines
   の読み書き（1行ごとの`Flush`）、op一覧（`query`/`exec`/`snapshot`/
   `load`/`inspect`/`tables`/`schema`/`dump`/`overwrite`/`close`）。
   `value.go`に`sqlValue`（JSON→SQLite値の逆変換）を追加。
5. **Step 5: `--read-only`** — `openSession`が`opts.readOnly`なら
   `PRAGMA query_only = ON`を適用。新規`readonly.go`（`ErrReadOnly`、
   `(*repl) readOnly()`）。`doSnapshot`/`doLoad`/`doOverwrite`の個別
   チェック、バナー・`.help`の読み取り専用バリアント。
6. **Step 6: 仕上げ** — `tests/e2e.sh`拡張（`-c`・stdin scriptのバッチ
   中断、モード排他、stdio往復、`--read-only`）、ユニットテスト
   （`batch_test.go`/`stdio_test.go`/`readonly_test.go`）、
   `docs/usage/`の実測突き合わせ、`PLAN.md`更新。

## フェーズ⑤のステップ

スコープは`docs/examples/`・`docs/tour/`の新設、英語版ドキュメント一式、
ルート`README.md`/`README_ja.md`、`.github/workflows/release.yml`。実装
機能は無く（`cmd/san-db-ox`のバージョン解決まわりのみ小さな変更）、
ドキュメント・配布が主題。

0. **Step 0: 仕様書の更新** — `-v`/バナー/stdio hello行が返すバージョン
   文字列の決定順序（リリースビルドの`-ldflags`→`go install`のビルド情報
   →`dev`）を§12/§13へ追記。
1. **Step 1: 配布経路の仕上げ** — `go install`経由で入れたバイナリでも
   意味のあるバージョンが出るよう`resolvedVersion()`を追加
   （`runtime/debug.ReadBuildInfo()`使用）。`.github/workflows/release.yml`
   新設（`v*`タグpush→`make check`→6通りcrossビルド→`gh release create`）。
2. **Step 2: ドキュメント検証ハーネス `tests/docs.sh`新設** — `<!-- verify -->`
   マーカー付きコードブロックを実バイナリへ流し込んで検証。`make test`へ
   組み込み。
3. **Step 3: ルート`README.md`/`README_ja.md`** — インストール・30秒デモ・
   4モード表・ネットワーク越し利用時の注意（仕様書§9-4の2点）・ドキュメント
   索引。
4. **Step 4: `docs/examples/`（日本語版）** — 仕様書§9の7ユースケースを
   5ファイルに肉付け（CI/CD、実行可能スナップショット、stdio結合、
   read-onlyデモ、SQLite相互運用）。
5. **Step 5: `docs/tour/`（日本語版）** — 8章構成の入門ガイド（体験型）。
6. **Step 6: 英語版一式** — `docs/spec/`・`docs/usage/`・`docs/examples/`・
   `docs/tour/`すべてを英訳（ファイル名は`_ja`を落とした形）。コード・
   コマンド・出力例は翻訳しない。
7. **Step 7: 仕上げ** — `.claude/rules/`への新知見追記、`PLAN.md`更新、
   `docs/usage/README_ja.md`の索引にtour/examplesを追加。

## 現在地

**フェーズ⑤（ドキュメント・配布）完了。実装計画上の全5フェーズが完了。**
`docs/examples/`（5本）・`docs/tour/`（8章）を新設し、`docs/spec/`・
`docs/usage/`・`docs/examples/`・`docs/tour/`・ルート`README`の英語版一式を
整備。ドキュメント中のコマンド・出力例は新設した`tests/docs.sh`
（`<!-- verify -->`マーカー付きブロックを実バイナリへ流し込んで検証、
`make test`から実行）で機械的に検証しており、40ファイルすべてgreen。
`.github/workflows/release.yml`を新設（`v*`タグpush→6通りcrossビルド→
`gh release create`）。`go install`経由で入れたバイナリでも意味のある
バージョン文字列が出るよう`resolvedVersion()`を追加（`-ldflags`→
`go install`のビルド情報→`dev`の優先順位、実測で`go build`がVCS疑似
バージョンを埋め込む挙動を発見し対処）。`make check`・`make race`・
`make test`ともにgreen。

**`v0.1.0`タグをpush→`release.yml`の実地確認も完了。** [GitHub Releases
v0.1.0](https://github.com/amisonnet8/san-db-ox/releases/tag/v0.1.0)に
6アセット（`linux`/`darwin`/`windows` × `amd64`/`arm64`、各`.sha256`付き）が
ドラフトでなく即時公開された状態を`curl`で実測確認済み（アセットサイズは
約10〜11MB、`binary-size.md`の想定レンジ通り）。README各所の
`/releases/latest`リンクもこれで実体を持つ。

**リリース後の保守: `san-db-ox-clients` 側のconformanceスイートが
`v0.1.0`バイナリに対して見つけたstdioプロトコルのバグ2件を修正
（[Issue #1](https://github.com/amisonnet8/san-db-ox/issues/1)、
コミット`8e6ac2c`・`9e2ddea`）。** (1) `exec`に`sql`が無いと
`db.Exec("")`がnilの`sql.Result`を返し、`opExec`が無条件で
`.RowsAffected()`を呼んでプロセス全体がpanicしていた——`exec`/`query`
双方に`sql`必須チェックを追加し`bad_request`を返すよう修正
（仕様書§7「必須パラメータの欠落はbad_request」）。(2) `params`が
`json.Decoder.UseNumber()`無しでデコードされていたため、
`2^53-1`を超える整数が`float64`精度に落ちてSQLite側の型まで
INTEGERからREALへ書き換わる、というサイレントなデータ破損が
あった——`sqlValue`（value.go）を`json.Number`対応に修正。
回帰テストは`cmd/san-db-ox/stdio_test.go`に追加済み、`make test`
green。**`san-db-ox-clients`側のconformanceスイートが green
になるまでIssue #1はopenのまま**（クローズはそちらの確認後）。

### フェーズ⑤の進捗

- **Step 0（仕様書の更新）**: §12へバージョン文字列の決定順序
  （`-ldflags`→`ReadBuildInfo().Main.Version`→`dev`）を追記。§13の
  起動バナー節へ、バナーの表示するバージョンが`-v`・stdio hello行と
  同じ解決結果であることを追記。
- **Step 1（配布経路の仕上げ）**: `main.go`に`resolvedVersion()`/
  `isDirtyBuild()`を追加。**実装中に想定と異なる挙動を発見**——
  gitリポジトリ内での素の`go build`は`(devel)`ではなく、Go 1.18以降の
  `-buildvcs=auto`既定によりVCS由来の疑似バージョン
  （`vX.Y.Z-yyyymmddhhmmss-<commit>`、dirtyなら`+dirty`付き）を
  `Main.Version`へ埋め込む。当初の想定（`go install`のみがこの値を持つ）
  を修正し、「dirtyな場合のみ`dev`へフォールバックする」実装へ変更
  （`vcs.modified`キーで判定）。`git stash`で退避してから検証しようと
  したところ、退避後のコードが実装前のものになっていて誤った結果を
  見てしまう罠を実際に踏み、`.claude/rules/distribution.md`へ記録した。
  `.github/workflows/release.yml`新設。6通りのクロスビルドを手元で実行し
  成功を確認、`actions/upload-artifact`・`actions/download-artifact`の
  バージョンタグが実在することを`curl`で確認済み。
- **Step 2（`tests/docs.sh`新設）**: `<!-- verify -->`マーカー・heredoc対応・
  バージョン/タイムスタンプの正規化を実装。`Makefile`の`test`ターゲットへ
  組み込み。既存の`docs/usage/`のシェル例にもマーカーを追加し実測差分
  （`.mode json`の実際のキー順・`.load`/`.import`の確認メッセージ欠落等）
  を複数発見・修正。
- **Step 3（README）**: `README.md`/`README_ja.md`新設。仕様書§9-4が
  「READMEでも必ず併記する」と定める2点（接続ごとに別DB・認証無し）を含む。
- **Step 4（`docs/examples/`）**: 5ファイル新設。実測時に`.dump`の実際の
  出力（`PRAGMA foreign_keys=OFF`/`BEGIN TRANSACTION`/`COMMIT`を含む）や
  `-c`と非対話stdinの併用時に後者が無視されるだけで使用法エラーには
  ならないこと等、ドキュメント執筆時の思い込みと実挙動の差分を複数発見。
- **Step 5（`docs/tour/`）**: 8ファイル新設（索引＋7章）。
- **Step 6（英語版）**: `docs/spec/san-db-ox_spec.md`（922行）を含む全19
  ファイルを英訳。コード・コマンド・出力例は翻訳せず、検証マーカーも
  日本語版と同じ位置に付与——翻訳時のコマンド打ち間違いが`make test`で
  機械的に検出される。
- **Step 7（仕上げ）**: `.claude/rules/testing.md`へ`tests/docs.sh`の運用
  ノート（正規化を2つの関心事に限る理由、heredoc対応、実測で見つかった
  罠）を追記。`.claude/rules/distribution.md`へ`go install`経由の
  バージョン解決の実測結果を追記。`docs/usage/README_ja.md`の索引に
  `docs/tour/`・`docs/examples/`へのリンクを追加。全ローカルリンクが
  解決することをスクリプトで確認済み。

### フェーズ④の進捗

- **Step 0（仕様書の更新）**: §7へhello行・`dump` op・エラーコード割り当て
  方針（`sqlite_error`/`io_error`/`bad_request`/`unsupported_op`/
  `read_only`の使い分け）を追記。§5へ「`-c`の各値・stdinスクリプト全体は
  それぞれ独立した実行単位」というルール（末尾`;`省略時はその単位の終わりで
  暗黙補完、複数`-c`を跨いだ文の継続はしない）を追記——最初は「入力全体の
  終端でのみ補完」と書いたが、実装中に自プロジェクトの`docs/usage/
  cli-options_ja.md`の複数`-c`サンプル（いずれも`;`を付けていない）と
  矛盾することに気づき、各`-c`引数を独立単位とする方式へ訂正した。§2へ
  `--read-only`の実装方針（`query_only`プラグマ＋`.snapshot`/`.overwrite`/
  `.load`の個別チェック）を追記。
- **Step 1（CLI起動オプション拡張）**: `options.go`に`stringSliceFlag`
  （`flag.Value`実装、`-c`/`--command`の複数回指定に対応）、
  `--serve-stdio`、`-r`/`--read-only`を追加。`--serve-stdio`+`-c`、
  `--read-only`+`-i`の排他検証。`options`構造体がスライスフィールドを
  持つようになったため、既存テストの構造体比較（`!=`）を
  `reflect.DeepEqual`へ修正。
- **Step 2（ドットコマンドのロジック/出力分離）**: `dotcmd.go`の
  `cmdSnapshot`/`cmdLoad`/`cmdOverwrite`/`cmdTables`/`cmdSchema`を、
  engine呼び出し部分（`doSnapshot`/`doLoad`/`doOverwrite`/`listTables`/
  `schemaSQL`）とテキスト整形部分に分離。`dump.go`の`dumpTables`等を
  `io.Writer`引数化し`dumpSQL`（文字列を返す版）を追加。REPL用の`cmd*`は
  この上の薄いラッパーへ変更——`directory-structure.md`の「3つの実行
  モードは同じドットコマンド実装を共有する」をバッチ・stdioへ拡張する
  ための下地。
- **Step 3（バッチ実行モード）**: 新規`batch.go`。`handleDotCommand`/
  `execSQL`が`error`を返すようシグネチャ変更（REPLは無視して継続、
  バッチは中断判定に使う）。`splitComplete`を`*repl`メソッドから
  パッケージ関数へ格上げ。`main.go`のモード判定を
  `--serve-stdio`／`-c`／非対話stdin／REPLの4分岐に再構成し、
  `--snapshot-interval`はバッチでは起動せず警告のみ出すよう変更。
  **実装中に発見した実バグ**: `runBatchChunk`で`splitComplete`が返す
  空白のみの`remainder`（文末`;`直後の`\n`等）をそのまま`buf`へ
  書き戻していたため、`buf.Len() == 0`判定が二度と成立せず、後続行の
  ドットコマンドがSQL文として実行されて構文エラーになっていた
  （`TestRunBatchChunkDotCommandAfterCompleteStatement`で回帰確認。
  意図的に修正を戻して同じ壊れ方を再現済み）。
- **Step 4（stdioプロトコル）**: 新規`stdio.go`。hello行、1行ごとの
  `Flush`、`query`/`exec`/`snapshot`/`load`/`inspect`/`tables`/`schema`/
  `dump`/`overwrite`/`close`の全op。`value.go`に`sqlValue`
  （`jsonValue`の逆変換、JSONの値→SQLiteバインド引数）を追加。
- **Step 5（`--read-only`）**: `repl.go`に`openSession`（`Session`生成＋
  `opts.readOnly`なら`PRAGMA query_only = ON`適用、REPL/バッチ/stdio
  共通）を追加。新規`readonly.go`（`ErrReadOnly`、`(*repl) readOnly()`）。
  `doSnapshot`/`doLoad`/`doOverwrite`が`ErrReadOnly`を返すよう変更
  （`.snapshot --sqlite`も`doSnapshot`経由のため自動的に含まれる）。
  バナーへの`(read-only)`表示、`.help`の読み取り専用バリアント
  （`cmdHelpReadOnly`）。
- **Step 6（仕上げ）**: `tests/e2e.sh`に`-c`（単発／複数／エラー中断／
  `.exit`早期終了）、stdinスクリプト（エラー中断／暗黙`;`補完）、
  モード排他（`--serve-stdio`+`-c`、`--read-only`+`-i`）、`--read-only`
  （書き込みSQL・`.snapshot`拒否、`.help`差し替え）、stdioプロトコル
  （`coproc`によるhello行/exec/query/snapshot/close往復、不正リクエスト後の
  継続、`--read-only`との組み合わせ）を追加。既存の2ブロック
  （`.tables`等のエラーハンドリング確認・`--snapshot-interval`確認）は
  フェーズ④の仕様変更（非対話stdinがバッチ実行になりエラー時即中断する
  ようになったこと、`--snapshot-interval`がバッチで無効化されたこと）に
  合わせて改修——**実装中に新たに踏んだ落とし穴**: `coproc`で相手
  プロセスへ`close` opを送って先方を終了させた後に`exec {NAME[1]}>&-`/
  `wait "$NAME_PID"`を呼ぶと、bashが子プロセスの終了を検知した時点で
  `NAME`配列と`NAME_PID`を自動的にクリアしてしまうため
  `ambiguous redirect`になる（`.claude/rules/testing.md`へ追記）。
  ユニットテスト`batch_test.go`（新規）・`stdio_test.go`（新規、
  `io.Pipe`+タイムアウト付き）・`readonly_test.go`（新規）を追加。
  `docs/usage/cli-options_ja.md`・`stdio-protocol_ja.md`を実測と
  突き合わせ（`-c`の暗黙`;`補完ルール・`dump` op・エラーコード表を反映、
  既存のシェル例は実行して動作を再確認済み）。

### フェーズ③の進捗

- **Step 0（仕様書・ルールファイルの更新）**: 継続プロンプト`   ...> `
  （仕様書§0）、REPLが`engine.Session`を1本保持する設計とその根拠
  （`database/sql`の`ResetSession`は開いたトランザクションをロールバック
  しない。`sqlite-quirks.md`へ新規追記、出所: ExecDB）、`.mode`/`.headers`の
  相互作用表（仕様書§3）、SQL文は`;`で終端すること、
  `--snapshot-interval`の保存失敗時はstderrへ警告して継続すること
  （仕様書§12）、非対話時はプロンプト・バナーを一切出さないこと
  （仕様書§13、`cli-output.md`にstdout/stderrの分担も明文化）を反映
  （コミット`8a53746`）。
- **Step 1（REPL基盤の作り直し）**: `repl`構造体を導入し起動時に
  `db.Session(ctx)`を1本開いてプロセス終了まで保持する設計に変更
  （フェーズ①の`db.Query`一発呼びは複数コネクションをまたぎうる実バグ
  だった）。SQL文の分割は自前のBEGIN/END解析ではなく`engine.Complete`を
  文候補ごとに呼ぶ`splitComplete`に一本化。継続プロンプト・非対話時の
  プロンプト/バナー抑制（`isInteractive`）を実装。この結果、SQL文は
  `;`終端が必須になる仕様変更を伴った（`tests/e2e.sh`の1箇所を修正）。
  `engine/session_test.go`に`TestSessionSurvivesCanceledQuery`を追加
  （コミット`c288fe6`）。
- **Step 2（出力モード5種+`.mode`/`.headers`）**: `format.go`
  （list/column/csv/json/line）、`value.go`（`jsonValue`/`jsonReal`、
  stdioプロトコルとの共用を見越した値変換）。REALのNaN/±Infの
  エンコード方針（`9e999`/`-9e999`/`null`、sqlite3 CLI踏襲）をこの場で
  決定し仕様書§7・`cli-output.md`へ反映（コミット`949eccb`）。
- **Step 3（CLI起動オプション）**: `options.go`（`flag.FlagSet`+
  `flag.ContinueOnError`、短縮/長い形式を同じ変数へ束縛）。
  `filename.go`を`naming.md`の完全なルール（タイムスタンプの重複除去、
  `.sqlite`/`.exe`拡張子補完）へ拡張。`interval.go`
  （`--snapshot-interval`）実装中に**`stop()`が非同期のまま返っていた
  ため、テスト用一時CWDが復元された後にバックグラウンドgoroutineの
  書き込みが実行され、パッケージディレクトリへスナップショットファイルが
  漏れ出す実レース**を発見・修正——`stop()`をgoroutineの終了を待つ同期的な
  実装に変更し、`TestStartSnapshotIntervalStopBlocksUntilGoroutineExits`で
  回帰確認（意図的に非同期版へ戻して5回連続失敗することを確認済み）
  （コミット`5aab3c1`）。
- **Step 4（`.snapshot`完成+`.load`追加）**: `--sqlite`/`--timestamp`の
  順不同パース、`.load`（`engine.Inspect`によるフッターVersion不一致警告）
  （コミット`3af4bd4`）。
- **Step 5（`.dump`/`.import`）**: ExecDBの`cmd/execdb/{dump,import}.go`を
  移植し、i/oを本プロジェクトの`r.out`/`r.errw`規約へ適合
  （コミット`2210520`）。
- **Step 6（Ctrl+Cの状態機械）**: `interrupt.go`（ExecDBの
  `cmd/execdb/interrupt.go`をほぼそのまま移植、sqlite3 `shell.c`と同じ
  状態機械）。`repl.run()`を`startLineReader`+`select`ベースへ再構成。
  `script`（util-linux）経由の実バイナリへの手動PTYテストで、クエリ実行中の
  キャンセル・アイドル時の1回破棄・連続2回でのforce-quit（終了コード1）を
  すべて実測確認（コミット`6b7f3cd`）。
- **Step 7（仕上げ）**: `tests/e2e.sh`に出力モード（`-m json`/`-m csv`）・
  `.snapshot --sqlite`/`.load`（SQLiteファイル・SanDBox実行ファイル双方）・
  `.dump`/`.import`・`--snapshot-interval`・Ctrl+C（PTY、`script`が無い環境は
  skip）を追加。非対話時のプロンプト抑制（Step 1）により可能になった
  `grep -qx`（完全一致）への置き換えも実施し、`testing.md`の該当の落とし穴を
  「解消済み」に更新。**この過程で`.load`の実バグを発見**——`FileInfo.Version`は
  `KindSQLite`では常に`0`なので、`Kind`チェックを入れずに
  `engine.FormatVersion`と比較すると、SQLiteファイルを`.load`するたびに
  無意味な「フォーマットバージョン不一致」警告が出ていた。`info.Kind ==
  engine.KindExecutable`のチェックを追加し、
  `TestCmdLoadFromSQLiteFileDoesNotWarnAboutVersion`で回帰確認（意図的に
  チェックを外して再現することを確認済み）。`docs/usage/repl-commands_ja.md`・
  `cli-options_ja.md`の出力例は実測と完全一致することを確認済み（変更不要）。

### フェーズ②の進捗

- **Step 0（仕様書更新＋技術検証スパイク）**: 使い捨てテストで2点を実測。
  - **検証A**: `journal_mode=WAL`のSQLiteファイルのメイン部分だけを
    `Deserialize()`に渡すと、Deserialize自体は成功するが、その後のクエリが
    `unable to open database file (14)`で失敗する（インメモリDBに対して
    WALファイルを探そうとして失敗するとみられる）。**`LoadFrom`にWALモードの
    SQLiteイメージを渡すと使い物にならないことを確認**——`Load`/`LoadFrom`は
    `NewRestore`（ファイル経由）と`Deserialize`（バイト列経由）で経路を
    分ける設計判断の裏付けになった。
  - **検証A追加**: SQLiteファイルヘッダのオフセット18/19バイト目
    （write/read format version）が`2`ならWALモードと事前検出できることを
    実測確認（rollbackモードは`1`）。`LoadFrom`がWALイメージを明示的な
    エラーで拒否する実装の根拠。
  - **検証B**: `NewRestore(path)`のコピー先が生きた接続で、他の接続が
    書き込みトランザクションを保持中の場合、busy_timeoutが正しく効き
    （約200ms待って解放されたケースで228ms後に成功）、即エラーにはならない
    ことを確認。`ErrBusy`へのマッピング方針の裏付け。
- **Step 3（`Load`/`LoadFrom`実装中）で追加発覚した制約**: `NewRestore`で
  WALモードのSQLiteファイルを**生きているインメモリDB（`memdb`）へ直接**
  Backupすると、`Step 0`の検証A/Bだけでは見えなかった問題が起きることが
  実装中に判明した。Backup APIはコピー先が空の場合、ソースのヘッダバイト
  （journal_modeフラグ含む）をそのまま複製する。通常のファイルVFSは
  対応する`-wal`ファイルが無いWALヘッダのDBでも問題なく開けるが、
  **`memdb`はこれを開けず「データベースファイルを開けません」で失敗する**
  （`restoreFrom`自体は成功を返すのに、直後のクエリで失敗するという
  分かりにくい壊れ方をした）。対処として、ソース→一時ファイル（ファイル
  VFS）→`journal_mode=DELETE`でロールバックモードへ変換→生きているDB、
  という2段階のBackup構成に変更した（仕様書§11に反映済み）。
  - 仕様書§4/§6/§10/§11を更新（`.load`のWAL挙動、`NewRestore`/`Deserialize`の
    使い分け、`FileInfo`/`FileKind`/`LoadFrom`/`Complete`のAPI追加、
    `Session`とSnapshot/Export/Loadの`ErrBusy`関係、`Complete`の実装方式）
    （コミット `13c8950`）。
- **Step 1（内部リファクタ＋バグ修正）**: `backupInto`が`Step`失敗時に
  `Finish()`を呼んでいなかった実バグを発見・修正（`runBackup`に集約）。
  `NewRestore`用の`restorer`/`restoreFrom`、`SQLITE_BUSY`→`ErrBusy`の
  `mapBusy`、`decodeFooter`（`readFooter`から切り出し）、
  `tempFileFor`/`removeTempArtifacts`（`writeImageAtomic`から切り出し）、
  `fileDSN`（OSパス→ドライバDSN変換）を追加。バグ修正は「壊す→検出できる
  ことを確認→戻す」で実際に回帰テストが機能することを確認済み
  （コミット `632fa06`）。
- **Step 2（`Inspect`/`FileInfo`）**: `FileKind`（Unknown/SQLite/
  Executable）と`FileInfo`を追加。判別順は先頭16バイトのSQLiteヘッダを
  優先。実装中に「ディレクトリがKindUnknownをエラー無しで返してしまう」
  不具合（`os.Open`/`Stat`はディレクトリでも成功し、`Size()`が0になりうる
  ため）を発見し`IsRegular()`チェックを追加（コミット `09ae896`）。
- **Step 3（`Load`/`LoadFrom`、`Open`の載せ替え）**: 実装中に発覚した設計
  変更（上記「Step 3で追加発覚した制約」参照、2段階Backup方式）を反映。
  `Open`も同じ判別ロジックへ載せ替え、SanDBox実行ファイルも受け付けるよう
  になった。`HasData`は`Load`では変化しない（確定判断3）
  （コミット `400b925`）。
- **Step 4（`Export`）**: `backupInto`を使いSQLiteファイルへ書き出し。
  パーミッションは`0644`（`Snapshot`の`0755`とは異なる）。
  `tempFileFor`/`removeTempArtifacts`を`persist.go`と共有
  （コミット `a5d8766`）。
- **Step 5（`Session`）**: 専有コネクション。`Begin`/`BeginTx`は持たない
  設計（ExecDB踏襲）。`newLiveDB`のコネクションプールに上限を設けない
  理由をコメント化。実装中に2件、テストの前提が実際の`memdb`挙動と
  食い違うことが判明——(1) `memdb`のSHAREDロックは書き込み中の他接続を
  ブロックしうる（ExecDBが`sqlite-quirks.md`で既に記録済みの落とし穴と
  同じ）、(2) `DB.Close()`は開いたままの`Session`の接続を即座には奪わない
  （`database/sql`の仕様どおりで、docコメントに書いた設計と一致）。
  いずれも実装のバグではなくテストの期待を実際の挙動へ修正
  （コミット `5724cb4`）。
- **Step 6（`Complete`）**: ExecDBの実装（`Xsqlite3_complete`を
  `modernc.org/libc`のTLS経由で呼ぶ）をそのまま移植。`go mod tidy`で
  `modernc.org/libc`がdirect依存へ昇格（`go.sum`は不変）。`cmd/san-db-ox`が
  `modernc.org/sqlite/lib`・`modernc.org/libc`を直接importしていないことを
  確認（コミット `f50a421`）。
- **Step 7（並行性テストと仕上げ）**: `engine/concurrency_test.go`
  （`make race`専用、6テスト）、`engine/doc.go`更新、本節の更新。

### フェーズ①の記録

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
- **Step 5（E2E自動化・CI）**: `tests/e2e.sh`（REPL基本操作、`-h`/`-v`/
  不正引数、`.exit CODE`/EOF、`.snapshot`、`.overwrite`、複数プロセス
  独立性・並行読み取り・並行`.snapshot`、`go install`）、
  `.github/workflows/test.yml`（3OS`check`+`race`+`trivy`）、
  `make netcheck`（`net`/`net/http`非依存の検証）を新設。
  `Makefile`の`build`が`-o san-db-ox`のままだとWindowsで`.exe`拡張子が
  付かない不具合を発見・修正（`BIN := san-db-ox$(shell go env GOEXE)`）。

### Windows CIで踏んだ落とし穴（push→CI確認を4周、すべて解消済み）

Step 5実装後、ユーザーがpush→CI実行→エラー報告を4周繰り返し、そのたびに
実バグ・テスト不備を発見・修正した。**詳細な経緯はgitログ（コミット
`3113bbd`〜`3db1ecb`）に残しているため、ここでは結論のみ:**

1. テストの期待ファイル名がWindowsの`.exe`自動付与を考慮していなかった
   （`cmd/san-db-ox/dotcmd_test.go`・`tests/e2e.sh`の両方）。
2. **`.snapshot`（デフォルト名）が実行中バイナリ自身のパスと一致すると、
   Windowsでは一時ファイル＋`rename`方式が「アクセスが拒否されました」で
   失敗する**（Linuxでは偶然成功する）。`engine.Snapshot`が保存先を自分自身と
   検知したら`.overwrite`と同じ退避方式に自動切替するよう修正——本物の
   製品バグ（仕様書§11に追記）。
3. 上記の自己検知ロジック（`samePath`）が**パス文字列比較だけ**では
   不十分だった（`os.Executable()`とパス結合で得たパスが、同じファイルを
   指していても文字列としては異なりうる——短縮8.3形式のパス要素等）。
   `os.SameFile`（OSレベルのファイル同一性）を使うよう修正。
4. `.san-db-ox.old`退避ファイルの削除タイミングはOS依存（Windowsは次回
   起動時）という**仕様書に最初から書かれていた挙動**を、テストの
   アサーションのタイミングが考慮していなかった（バグではなくテスト不備）。
5. Git Bash（MSYS）のパス自動変換は`argv`・環境変数経由でネイティブ
   プロセスを起動する時にしか働かず、**パイプ経由の標準入力テキストには
   効かない**——`tests/e2e.sh`が`$WORK`ベースの絶対パスを標準入力に
   埋め込んでいた箇所で発覚。

**教訓（`testing.md`に反映済み）:** Windows特有の問題は1回の修正で
仕留められるとは限らない。パスの同一性判定は最初から`os.SameFile`を
使うべきだった。テストスクリプトでOS依存の絶対パスをネイティブ
プロセスへ渡す際は、それが`argv`/環境変数経由か、パイプ経由かで
MSYSの自動変換が効くかどうかが変わることを意識する。

## 未確認事項（実装前に決める・確かめる）

1. ~~`Serialize()` の出力が、そのまま有効なSQLiteファイルとして開けるか。~~
   **Step 2で解消（実測確認済み。仕様書§6参照）。**
2. ~~REPLのプロンプト文字列。~~ **Step 4で解消。** `SanDBox> ` に確定
   （表示名をそのまま使用し、起動バナーの`SanDBox v0.1.0`と表記を揃える）。
   仕様書§0の表記スロット表に追記済み。
3. ~~`.dump` と `.import` に stdio op を用意するか。~~ **フェーズ④Step 0/4で
   解消。** `.dump`は`dump` opとして追加（仕様書§7）、`.import`は意図的に
   非対応のまま（サーバー側の任意パスのCSVを読む操作であり、
   `inspect(path)`を非公開にしている理由と同じ懸念——外部公開時に
   サーバー側のファイルシステムを探る手段になるため）。
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
- ~~`docs/examples/`・`docs/tour/` は未作成。~~ **フェーズ⑤で作成済み**
  （それぞれ5ファイル・8ファイル）。
- ~~英語版ドキュメントは未作成。~~ **フェーズ⑤で整備済み**（`docs/spec/`・
  `docs/usage/`・`docs/examples/`・`docs/tour/`・ルート`README`すべて）。
- ~~`release.yml` が未作成。~~ **フェーズ⑤で作成済み、`v0.1.0`タグpushで
  実地稼働も確認済み**（GitHub Releasesへの初回リリース公開に成功。
  現在地参照）。
- **`san-db-ox-clients`（各言語向けドライバ）は別リポジトリ。** 本リポジトリ
  では扱わない。本体側に残す接続テストはGoで書いたstdioクライアントのみ
  （`.claude/rules/testing.md`）。
