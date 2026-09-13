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

## 現在地

**フェーズ③（REPL開発）完了。** ドットコマンド全種
（`.tables`/`.schema`/`.mode`/`.headers`/`.snapshot`/`.overwrite`/`.load`/
`.dump`/`.import`/`.exit`/`.quit`/`.help`）、出力モード5種、CLI起動オプション
（`-m`/`-o`/`-t`/`-q`/`-i`/`-v`/`-h`）、Ctrl+Cの状態機械をすべて実装し、
`make check`・`make race`・`make test`ともにgreen。次はフェーズ④
（バッチ実行・stdioプロトコル開発）に着手する。

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
