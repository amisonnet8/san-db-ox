*[English](README.md)*

# 入門ガイド

体験しながら学ぶ、順を追ったチュートリアル。前提は**バイナリ1個だけ**——
Go環境もインストール作業も不要。各章は前の章を前提に進むので、上から順に
読むことを想定している（索引的なリファレンスが欲しい場合は
[`docs/usage/`](../usage/README_ja.md)、ユースケースごとの実例が欲しい場合は
[`docs/examples/`](../examples/README_ja.md)を参照）。

全体で15〜20分程度。

1. [はじめに](01-getting-started_ja.md) — 入手・起動・最初のSELECT
2. [テーブルとクエリ](02-tables-and-queries_ja.md) — CREATE/INSERT/SELECT、複数行入力
3. [データを保存する](03-saving-your-data_ja.md) — `.snapshot`/`.overwrite`
4. [出力モード](04-output-modes_ja.md) — `.mode`/`.headers`/`.dump`
5. [SQLiteファイルとの入出力](05-sqlite-files_ja.md) — `.snapshot --sqlite`/`.load`/`.import`
6. [スクリプトから使う](06-scripting_ja.md) — `-c`、終了コード、CI連携
7. [stdioと読み取り専用モード](07-stdio-and-read-only_ja.md) — `--serve-stdio`、`--read-only`

各章のコマンド・出力例は、実際にビルドしたバイナリへ流し込んで実測検証した
ものをそのまま載せている（`tests/docs.sh`がCIで継続的に確認する）。
