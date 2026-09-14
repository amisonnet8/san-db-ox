*[English](executable-snapshots.md) | **日本語***

# 実行可能スナップショットでのバグ再現・共有

不具合が起きたデータ状態をそのまま実行ファイルとして固定し、チームメンバーへ
共有する。受け取った側は実行するだけで、報告者とまったく同じDB環境を
即座に再現できる——「手元では再現しません」という会話を無くすための機能。

## バグを再現した状態を保存する

不具合が発生した時点のデータ状態のまま`.snapshot`する。ファイル名には
チケット番号等を含めておくと後から追いやすい。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE orders (id INTEGER PRIMARY KEY, status TEXT)" -c "INSERT INTO orders VALUES (1, 'stuck')" -c ".snapshot bug_123"
Wrote bug_123
```

`bug_123`をSlackなりファイル共有なりでそのまま渡す。受け取った側がやることは
1つだけ。

<!-- verify -->
```console
$ chmod +x bug_123 && ./bug_123 -c "SELECT * FROM orders"
1|stuck
```

インストール・接続文字列の共有・ダンプの流し込みが一切不要——**バイナリ
そのものがデータを持っている**ため。

## 異なるOSの相手へ渡す場合

`.snapshot`が作る実行ファイルは、**今動いているプロセス自身のエンジン部分**を
土台にしている。Linux上で作ったファイルはLinux上でしか実行できない。相手が
別のOS（例: macOS、Windows）を使っている場合は、データだけを移し替える。

1. 相手側で、対象OS向けの**空バイナリ**（データを埋め込んでいないもの）を
   [GitHub Releases](https://github.com/amisonnet8/san-db-ox/releases/latest)から
   ダウンロードする。
2. `.load`で共有された実行ファイルからデータだけを取り込む（`.load`は
   取り込み元のエンジン部分を一切参照しないので、OSが違っても問題ない）。
3. `.overwrite`で、今起動している(対象OS向けの)バイナリ自身にデータを書き込む。

```
$ ./san-db-ox-windows.exe
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox> .load bug_123
SanDBox> .overwrite
Overwrite ok, exiting.
```

これで`san-db-ox-windows.exe`自体が、Linux版と同じデータを持つWindows向けの
実行ファイルになる。

## 補足: `.snapshot`は実行ファイル、`.snapshot --sqlite`はSQLiteファイル

`.snapshot`はデフォルトで実行ファイルを作るため、受け取った相手がSanDBoxの
存在すら意識せずに使える。生のSQLiteファイルとして共有したい場合は
[既存SQLite資産との橋渡し](sqlite-interop_ja.md)を参照。
