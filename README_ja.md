<div align="center">

# SanDBox

**実行ファイル1つに閉じ込めた、使い捨てのSQLサンドボックス。**
自由に試して、メモリ上のDBをそのまま実行可能なファイルへスナップショットし、共有する。
インストール不要、サーバー不要、ネットワーク不要。

[![CI](https://github.com/amisonnet8/san-db-ox/actions/workflows/test.yml/badge.svg)](https://github.com/amisonnet8/san-db-ox/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/amisonnet8/san-db-ox)](https://github.com/amisonnet8/san-db-ox/releases/latest)
[![License: MIT](https://img.shields.io/github/license/amisonnet8/san-db-ox)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/amisonnet8/san-db-ox)](go.mod)

![テーブルを作成し、.overwriteで自分自身にデータを埋め込み、再起動後もSELECTでデータを読み出せることを示すデモ](docs/img/demo.gif)

*[English](README.md)*

</div>

DBエンジンとデータ領域を1つの実行ファイルに保持し、ダウンロードした実行ファイルを
そのまま動かすだけで使えるポータブルなRDBMS。Go製、内部エンジンは
[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)。**ネットワーク待受は
一切持たない**——他システムとの結合は、子プロセスとして起動して標準入出力で
やり取りする stdio プロトコル、および素のSQLiteファイルの読み書きで行う。

## 目次

- [特徴](#特徴)
- [インストール](#インストール)
- [30秒で試す](#30秒で試す)
- [4つの動作モード](#4つの動作モード)
- [ネットワーク越しに使う場合の注意](#ネットワーク越しに使う場合の注意)
- [もっと詳しく](#もっと詳しく)
- [ライセンス](#ライセンス)

## 特徴

- 🧳 **単一バイナリ完結** — エンジンとデータが1つの実行ファイルに同居。追加の
  インストール・依存関係・環境構築が一切ない
- 📸 **実行可能なスナップショット** — `.snapshot`で今のDBをそのまま「動く
  ファイル」として書き出し、そのまま配布できる
- 🔒 **ネットワーク待受ゼロ** — `net`パッケージを一切importしない設計。外部への
  公開はトランスポートの外付け（`socat`等）に委ねる
- 🔌 **stdio連携** — JSON Linesのプロトコルで標準入出力から他言語・他プロセスと
  結合できる
- 🗄️ **SQLiteファイルとの相互運用** — `.snapshot --sqlite`での書き出し、
  `.load`での取り込みが可能
- 🧪 **読み取り専用モード** — `--read-only`でデモ・閲覧用途を安全に公開できる

## インストール

**Go環境が無い場合:** [GitHub Releases](https://github.com/amisonnet8/san-db-ox/releases/latest)
から、お使いのOS/アーキテクチャ向けの実行ファイルをダウンロードする。

```sh
chmod +x san-db-ox   # Linux/macOSのみ。ダウンロード直後は実行ビットが立っていないことがある
./san-db-ox
```

**Go環境がある場合:**

```sh
go install github.com/amisonnet8/san-db-ox/cmd/san-db-ox@latest
```

## 30秒で試す

```
$ ./san-db-ox
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox> CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
SanDBox> INSERT INTO users VALUES (1, 'alice');
SanDBox> .overwrite
Overwrite ok, exiting.
$ ./san-db-ox
SanDBox v0.1.0
Loaded snapshot: san-db-ox
Enter ".help" for usage hints.
SanDBox> SELECT * FROM users;
1|alice
SanDBox> .exit
```

`.overwrite`で**自分自身**を書き換え、`san-db-ox`はそれ自体がデータ入りの
実行可能なファイルになった。配れば、受け取った人が実行するだけで同じデータを
持つDBが立ち上がる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".overwrite"
Overwrite ok, exiting.
$ ./san-db-ox -c "SELECT * FROM users"
1|alice
```

## 4つの動作モード

| モード | 起動方法 | 用途 |
| :--- | :--- | :--- |
| REPL | 引数なし | 人間による対話操作 |
| バッチ実行 | `-c "SQL"` / stdin からのスクリプト | CI/CD、シェルスクリプト |
| stdioプロトコル | `--serve-stdio` | 他プログラムからの結合 |
| ライブラリ | `engine` パッケージをGoアプリへ組み込み | 内蔵DB層 |

いずれも**ネットワーク待受を行わない**。

## ネットワーク越しに使う場合の注意

SanDBox自身は`net`をimportしないが、`socat`等でトランスポートを外付けすれば
TCP/UNIXソケット越しに使うことができる。その場合、以下の2点に注意すること。

1. **接続ごとに別プロセス・別DBになる。** `socat`の`fork`は接続を受理する
   たびにプロセスをforkするため、複数の接続で1つのDBを共有することはできない。
2. **認証が一切存在しない。** 到達できる者は誰でもDDLを含む任意のSQLを実行
   できる。閲覧用途には`--read-only`と組み合わせ、公開範囲は
   `bind=127.0.0.1`やSSHポートフォワードで絞ること。

## もっと詳しく

| ドキュメント | 内容 |
| :--- | :--- |
| [入門ガイド](docs/tour/README_ja.md) | 体験しながら学ぶ、順を追ったチュートリアル |
| [実例集](docs/examples/README_ja.md) | ユースケースごとの実践的なコマンド集 |
| [起動オプション・REPLコマンド・stdioプロトコル](docs/usage/README_ja.md) | すぐ使うための索引的リファレンス |
| [仕様書](docs/spec/san-db-ox_spec_ja.md) | 設計判断の理由まで踏み込んだ仕様書 |

## ライセンス

[MIT](LICENSE)

---

<div align="center">

[目次へ戻る](#目次) ・ [Issues](https://github.com/amisonnet8/san-db-ox/issues)

</div>
