# SanDBox

*[English](README.md)*

**環境構築不要のポータブルな単一バイナリRDBMS。** DBエンジンとデータ領域を
1つの実行ファイルに保持し、ダウンロードした実行ファイルをそのまま動かすだけで
使える。Go製、内部エンジンは [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)。
**ネットワーク待受は一切持たない**——他システムとの結合は、子プロセスとして
起動して標準入出力でやり取りする stdio プロトコル、および素のSQLiteファイルの
読み書きで行う。

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
SanDBox> SELECT * FROM users;
1|alice
SanDBox> .snapshot mydb
Wrote mydb
SanDBox> .exit
```

`mydb` は**それ自体が実行可能なファイル**になっている。配れば、受け取った人が
実行するだけで同じデータを持つDBが立ち上がる。

<!-- verify -->
```console
$ ./san-db-ox -c "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" -c "INSERT INTO users VALUES (1, 'alice')" -c ".snapshot mydb"
Wrote mydb
$ chmod +x mydb && ./mydb -c "SELECT * FROM users"
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
