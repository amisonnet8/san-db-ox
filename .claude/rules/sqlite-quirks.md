# SQLite / modernc.org/sqlite の落とし穴

`engine`パッケージの実装（フェーズ②）で実際に踏んだ、`modernc.org/sqlite`
および内部SQLiteエンジン特有の挙動をまとめる。仕様書
（`docs/spec/san-db-ox_spec_ja.md`）の記述だけでは判断できない・誤解しやすい
実装上の注意点が対象。新しい落とし穴に気づいたら、このファイルに追記すること。

## Backup APIはコピー先が空の場合、ソースのヘッダバイトをそのまま複製する

`sqlite3_backup_step`はページ単位でソースからコピー先へ複製するが、**コピー先が
空（新規）の場合、ページ1（DBヘッダ）を含むソースの全内容をそのまま上書きする**。
これには journal_mode を表すフラグ（ヘッダのオフセット18, 19バイト目。
`1`=ロールバックジャーナル、`2`=WAL）も含まれる。

**この結果、ソースが`journal_mode=WAL`で運用されているSQLiteファイルの場合、
Backup API経由でコピーしたコピー先のヘッダにもWALモードのフラグがそのまま
書き込まれる。** 通常のファイルVFSは、対応する`-wal`ファイルが存在しない
WALモードヘッダのDBでも問題なく開ける（実測確認済み）が、**`memdb` VFSは
これを開けず「データベースファイルを開けません」（`unable to open database
file`, SQLITE_CANTOPEN）で失敗する。** しかも`restoreFrom`（Backup API）自体は
エラーを返さずに成功するため、失敗が判明するのは直後のクエリ実行時になる
——原因の見当がつきにくい壊れ方をする。

**対処（`engine.Load`のSQLiteファイル取り込み経路、`load.go`の
`stageRollbackModeCopy`）:** ソースを直接生きている`memdb`へBackupするのでは
なく、いったん通常のファイルVFS上の一時ファイルへBackupし、その一時ファイルに
対して`PRAGMA journal_mode=DELETE`を実行してロールバックモードへ変換
（ヘッダを書き換える）してから、その一時ファイルを生きているDBへ改めて
Backupする、という2段階構成にする。`PRAGMA wal_checkpoint(TRUNCATE)`だけでは
WALの内容はメインファイルへマージされるが、**ヘッダのjournal_modeフラグ自体は
変わらない**ため不十分（実測確認済み）。

詳細な設計判断の経緯は `docs/spec/san-db-ox_spec_ja.md` §11
「データブロブのシリアライズ方式（確定）」の `Load` の実装節を参照。

## `NewBackup`と`NewRestore`は`Backup.Finish`が閉じる接続が逆

`modernc.org/sqlite`の`conn`型は`NewBackup(dstUri)`（自分がソース、`dstUri`が
コピー先）と`NewRestore(srcUri)`（自分がコピー先、`srcUri`がソース）の両方を
持つ。`Backup`構造体は常に`srcConn`/`dstConn`の2つの接続を保持するが、
**`Backup.Finish()`が閉じるのは常に`dstConn`側**（`b.dstConn.Close()`）である。

- `NewBackup`（自分がソース）: `dstConn`はドライバが`dstUri`向けに内部で
  新規に開いた接続。`Finish`はこの**新規接続**を閉じる。呼び出し元の接続
  （ソース側）は影響を受けない。
- `NewRestore`（自分がコピー先）: `dstConn`は**呼び出し元自身**ではなく、
  ドライバが`srcUri`向けに内部で新規に開いた接続（`backup()`メソッド内部で
  `restore`フラグに応じて`srcConn`/`dstConn`が入れ替わる）。`Finish`はこの
  **ソース側の新規接続**を閉じる。呼び出し元の接続（コピー先側）は影響を
  受けず、開いたまま使い続けられる。

**この非対称性のおかげで**、`restoreFrom`（`NewRestore`使用）は「既存の
セッションは接続を失わない」（仕様書§4）という要件を自然に満たせる。一方、
**`Step`が失敗しても必ず`Finish`を呼ばないと**、`NewRestore`が内部で開いた
ソース側接続（＝取り込み元ファイルへの接続）がクローズされずリークする。
Windowsではこのリークにより取り込み元ファイルが削除・上書き不能になる
（実際に踏んだバグ。`engine/backup.go`の`runBackup`で対処し、
`engine/backup_test.go`の`TestBackupFinishesEvenOnStepFailure`で回帰確認済み）。

## ドライバの`newConn`は素のパスDSNの最初の`?`以降を切り落とす

`modernc.org/sqlite`の`newConn(dsn)`は、DSNが`file:`で始まらない場合、
**最初の`?`以降をクエリパラメータとして読み取った上で、DSN本体からは
切り落として渡す**（`sqlite3_open_v2`に渡るのは`?`より前の部分のみ）。

これは2つの意味を持つ。

1. **素のOSパスに`?`を含めることはできない。** Unixでは`?`はファイル名として
   合法な文字だが、そのようなパスをDSNとして渡すと、`?`以降が黙って
   切り捨てられ、別のファイルを指すDSNになってしまう（`engine.fileDSN`は
   これを検出して明示的なエラーを返す）。
2. **逆に、この仕組みを利用して`_busy_timeout`等のドライバパラメータを、
   パス文字列自体には手を加えずに付与できる。** `<path>?_busy_timeout=5000`と
   書けば、SQLiteが実際に開くのは`<path>`のまま（バックスラッシュを含む
   Windowsパスでも無変換で渡る）で、`_busy_timeout`だけがパラメータとして
   反映される（`engine.fileDSN`の実装。実測確認済み——素のパスが正しく
   開けること、busy_timeoutが実際に効くこと、`?`を含むパスが拒否される
   ことをすべてスパイクテストで確認済み）。

## SQLiteファイルヘッダのオフセット18/19バイト目でWALモードを判定できる

SQLiteのデータベースファイルヘッダのオフセット18（write version）・19
（read version）バイト目は、`1`ならロールバックジャーナルモード、`2`ならWAL
モードを表す（両者は常に同じ値を取る）。これは`sqlite3_open`すら経由せず、
生のバイト列から判定できる（`engine.LoadFrom`がWALモードのSQLiteイメージを
事前検出して拒否する実装で使用。`engine/load.go`の
`sqliteWALFormatOffset`/`sqliteWALFormatVersion`）。

## `memdb`の`SHARED`ロックは、明示トランザクション中でも文をまたいで保持され続けるとは限らない

> **出所:** 前身プロジェクト **ExecDB** で、`engine.Session`のテストを書く際に
> 実測した事象（SanDBoxのフェーズ②Step 5でも同じ形で再現した）。

「Bセッションで先に`BEGIN`＋ダミーの読み取りを1回行っておけば、その後Aが
書き込み中でもBは`SHARED`ロックを再取得せずに済み、待たされずに読める」と
いう設計は**実測では効かない**。通常のファイルベースSQLiteなら、明示
トランザクション開始後に一度`SHARED`ロックを取得すれば`COMMIT`/`ROLLBACK`
までそのロックを保持し続けるのが一般的な理解だが、**`memdb`ではこの前提が
成り立たない**（`modernc.org/sqlite`側のpager実装が`memdb`に対して積極的に
ロックを解放しにいっている可能性がある。正確な原因箇所は未特定）。

**実務上の帰結:** `memdb`上では、複数の`Session`が同時にアクセスする状況で
「読み取りは書き込みをブロックしない」という期待はできない。読み取りも
書き込み中は`busy_timeout`の範囲でブロックされ、その後（書き込みが
コミット/ロールバックされていれば）確定後の値を見る、という**直列化に近い
挙動**になる。`engine.Session`を使ったテスト・設計では、「同時実行中に
本当に非ブロックで読めるか」を前提にせず、「有界に待った後、正しい最終状態が
見える」ことをテストすること（`engine/session_test.go`の
`TestSessionsAreIndependent`のように、一方の書き込みをgoroutine化し、
コミット後に結果を受け取る形にする）。

一方、**読み取りトランザクションは、`Snapshot`/`Export`のような「生きている
DBを読むだけ」のAPIをブロックしない**（`engine/session_test.go`の
`TestSessionReadTxnDoesNotBlockSnapshotOrExport`で確認済み）。ブロックされる
のは「他の接続の書き込みトランザクションと衝突する場合」および「`Load`が
コピー先への排他アクセスを必要とする場合」に限られる。

## `modernc.org/sqlite/lib`（生成された内部パッケージ）への直接依存

> **出所:** 前身プロジェクト **ExecDB** で確立した対処方針（SanDBoxの
> `engine.Complete`実装でもそのまま踏襲した。フェーズ②Step 6）。

REPLの複数行入力判定（`engine.Complete`）は、SQLite本体の
`sqlite3_complete()` C関数をそのまま呼ぶ。自前でBEGIN/ENDのトークンを
カウントするスキャナは、`CREATE TRIGGER ... BEGIN ... END`の本体内に現れる
`CASE ... END`式の`END`を、トリガーを閉じる`END`と誤認する恐れがあるため
採らない。

`modernc.org/sqlite`のトップレベルパッケージ（`database/sql`ドライバとしての
公開面）はこの関数を公開していないが、生成コードである
`modernc.org/sqlite/lib`（パッケージ名`sqlite3`）に
`Xsqlite3_complete(tls *libc.TLS, zSql uintptr) int32`として存在する。
`modernc.org/libc`（`NewTLS()`/`CString()`/`Xfree()`）と組み合わせて呼び出す
（`engine/complete.go`）。

**このアクセス経路の性質・注意点:**
- **新規の外部依存の追加ではない。** `modernc.org/libc`・`modernc.org/sqlite/lib`は
  どちらも`modernc.org/sqlite`自身が内部で使っている既存の推移的依存
  （`go.sum`は変わらない。`go mod tidy`で`modernc.org/libc`が`go.mod`の
  `// indirect`から外れるのみ）。`Makefile`の`netcheck`が記録している
  「`net`の推移的依存は`modernc.org/libc`経由」という依存そのもの。
- **`lib`は生成コード（transpileされたC実装）であり、`modernc.org/sqlite`が
  ドキュメント化・安定性を保証する公開APIではない。** `directory-structure.md`
  の「`modernc.org/sqlite`ラッパーは`engine`が担う」という一貫した境界に従い、
  `cmd/san-db-ox`側は`engine.Complete()`経由でのみ触れ、
  `modernc.org/sqlite/lib`・`modernc.org/libc`を直接importしない（`go list`で
  確認済み）。
- **`modernc.org/sqlite`をバージョンアップする際は、`Xsqlite3_complete`の
  シグネチャ・存在・`lib`パッケージのimportパスが変わっていないか確認する
  こと。** 既存の`Raw()`＋ローカルinterfaceのトリック（`Serialize`/
  `Deserialize`/`NewBackup`/`NewRestore`）と同様、バージョン間で壊れうる
  箇所として`engine/complete.go`のコメントにも明記済み。
- `sqlite3_complete`はDBハンドルを取らない純粋な文字列走査関数なので、
  生きているDBが無くても呼び出せる（パッケージレベル関数として正しい）。
  C文字列を前提とするため、入力にNULバイトが含まれる場合は明示的にエラーを
  返す（`engine.Complete`が独自に追加したガード。ExecDB版には無い）。

## `database/sql`の`ResetSession`は開いたままのトランザクションをロールバックしない

> **出所:** 前身プロジェクト **ExecDB** で確立した知見（SanDBoxのフェーズ③、
> REPLが`engine.Session`を1本保持する設計の直接の根拠になった）。

`database/sql`のコネクションプールが接続を再利用する際に呼ぶ
`(*driverConn).resetSession`（内部的に`driver.SessionResetter`の
`ResetSession`を呼ぶ）は、**接続上に開いたままのトランザクションを
ロールバックしない**（`database/sql`自身のソースで確認可能な挙動）。

この結果、`db.Exec`/`db.Query`（`*sql.DB`が内部に持つコネクションプールから
その場で借りる使い捨て接続）の上で`BEGIN`を実行し、`COMMIT`せずにその接続が
プールへ返却されると、**次にそのプール接続を引いた別の呼び出しが、意図せず
開いたままのトランザクションを引き継いでしまう**。逆に`BEGIN`の直後の
`INSERT`/`COMMIT`が同じ接続に行くとは限らないため（プールは呼び出しごとに
別の接続を返しうる）、「地続きに見えるSQL文の並びが、実際には別々の
SQLite接続へ散らばる」という壊れ方をする。

**対処: `BEGIN`/`COMMIT`/`ROLLBACK`を伴うSQLは、必ず専有コネクション
（`engine.Session`、プールを経由しないもの）の上でのみ実行すること。**
`cmd/san-db-ox`のREPL実装がフェーズ③でこれに従い、起動時に`db.Session(ctx)`を
1本開いてプロセスの終了まで保持する設計にした（仕様書§2参照）。
