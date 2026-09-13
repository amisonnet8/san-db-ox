# 1. はじめに

*[入門ガイド目次](README_ja.md) / [English](01-getting-started.md)*

## 入手する

Go環境が無ければ、[GitHub Releases](https://github.com/amisonnet8/san-db-ox/releases/latest)
からお使いのOS/アーキテクチャ向けの実行ファイルをダウンロードする。Go環境が
あれば`go install github.com/amisonnet8/san-db-ox/cmd/san-db-ox@latest`でもよい。
以降、この実行ファイルを`san-db-ox`（Windowsでは`san-db-ox.exe`）と呼ぶ。

```sh
chmod +x san-db-ox   # Linux/macOSのみ
```

## 起動する

引数なしで実行すると、対話的なSQLコンソール（REPL）が起動する。

```
$ ./san-db-ox
SanDBox v0.1.0
No embedded data. Starting with an empty in-memory database.
Enter ".help" for usage hints.
SanDBox>
```

1行目はバージョン。2行目は「このバイナリにはまだデータが埋め込まれていない」
という意味——ダウンロードした直後の状態は常にこれである。3行目のヒント通り、
`.help`と打てばいつでもコマンド一覧を確認できる。

`SanDBox> `がプロンプト。ここにSQL文やドットコマンド（`.`で始まる制御
コマンド）を打ち込んでいく。

## 最初のSELECT

<!-- verify -->
```console
$ ./san-db-ox -c "SELECT 1 + 1"
2
```

（この章以降、対話プロンプトの様子は上のような枠で示すが、実際に動くことを
確認したコマンドは`-c`を使ったこの形——バッチ実行——でも示す。対話でも
バッチでも、SQL自体の書き方・結果の見え方は同じである）

対話セッションで同じことを試すなら:

```
SanDBox> SELECT 1 + 1;
2
```

**SQL文は`;`（セミコロン）で終える。** 忘れると、SanDBoxは文がまだ続いている
と判断し、次の行の入力を待つ（継続プロンプト`   ...> `が出る）。

## 終了する

`.exit`（または`.quit`、あるいはCtrl+D）で終了する。

<!-- verify -->
```console
$ ./san-db-ox -c ".exit"
```

**重要: この時点でデータは保存されない。** SanDBoxはインメモリDBであり、
明示的に`.snapshot`か`.overwrite`を実行しない限り、プロセスの終了とともに
すべてのデータが消える。保存の仕方は[3章](03-saving-your-data_ja.md)で扱う。

次章では、実際にテーブルを作ってデータを出し入れする。

*次: [2. テーブルとクエリ](02-tables-and-queries_ja.md)*
