*[English](README.md) | **日本語***

# 実例集

仕様書§9の各ユースケースを、実際に打つコマンド付きで肉付けしたもの。
1本ずつ自己完結しており、読む順序は問わない。索引的なリファレンスが
欲しい場合は[`docs/usage/`](../usage/README_ja.md)、体験しながら順を追って
学びたい場合は[`docs/tour/`](../tour/README_ja.md)（SQL学習用途はこちらが
担う）を参照。

| ファイル | ユースケース |
| :--- | :--- |
| [CI/CDでのInstant Test DB](ci-instant-test-db_ja.md) | シード済みバイナリ1個でテスト環境を秒で立てる |
| [実行可能スナップショットでのバグ共有](executable-snapshots_ja.md) | データ状態付きでバグを再現・共有する |
| [stdio経由での他プログラム結合](stdio-integration_ja.md) | 子プロセスとして起動しJSON Linesで対話する |
| [閲覧用デモ環境の一時公開](read-only-demo_ja.md) | `socat` + `--read-only` で読み取り専用デモを立てる |
| [既存SQLite資産との橋渡し](sqlite-interop_ja.md) | SQLiteファイルの入出力、GUIツール・pandasとの連携 |
