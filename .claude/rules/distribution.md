# 配布方法

配布経路は以下の2系統とする。いずれも最終的に得られるのは「データなし・
エンジンのみのバイナリ」であり、そこから `.snapshot` または `.overwrite`
を実行して初めてデータ入りバイナリになる、という流れは共通する。

| 配布経路 | 対象ユーザー | 内容 |
| :--- | :--- | :--- |
| GitHub Releases のプリビルド済みバイナリ | Go環境を持たない人、初学者 | 各OS/アーキテクチャ向け（Linux/macOS/Windows × amd64/arm64 の6組み合わせ）の空（データなし）バイナリをあらかじめビルドして配布する |
| `go install github.com/amisonnet8/san-db-ox/cmd/san-db-ox@latest` | Go開発者 | ソースからその場でビルドされた、データなし・エンジンのみのバイナリが `$GOPATH/bin` に生成される |

**注意点:** `go install` で生成されるバイナリも、通常の `go build` と同じ
ビルドプロセスを経るため、フッター方式による末尾へのデータ追記・読み込みは
同様に機能するはずである。

**実測確認済み（フェーズ①Step 5）:** `GOBIN` を指定した `go install
./cmd/san-db-ox` で生成したバイナリに対し、`.overwrite` でデータを埋め込み
→再起動→`SELECT`で読み出せることを`tests/e2e.sh`で確認済み（Linux）。
Windows/macOSでの確認はGitHub Actions 3OSマトリクス（`test.yml`）が
`make test`（同じ`tests/e2e.sh`）をそのまま実行することで担保する。

## `go install` 経由のバージョン表示（`-v`・起動バナー・stdio hello行）

`go install github.com/amisonnet8/san-db-ox/cmd/san-db-ox@latest` は
`-ldflags` を渡せないため、`main.version`（`-ldflags -X`用の変数）は常に
既定値の`dev`のまま残る。そこで`runtime/debug`の`ReadBuildInfo()`が返す
`Main.Version`を併用する（`cmd/san-db-ox/main.go`の`resolvedVersion`）。

**フェーズ⑤の実装中に実測して分かったこと（想定と異なった点）:**
`go install pkg@version`だけでなく、**gitリポジトリ内での素の`go build`も
`Main.Version`に疑似バージョン（`vX.Y.Z-yyyymmddhhmmss-<commit>`形式）を
埋め込む**——Go 1.18以降`-buildvcs`の既定値が`auto`であり、ビルドディレクトリが
VCS管理下にあると自動的にVCS情報を埋め込むため。**`(devel)`になるのは
`-buildvcs=false`指定時、またはVCS管理下に無いディレクトリでビルドした
場合のみ**（実測: リポジトリの`git archive`で取り出した非gitディレクトリで
ビルドすると`(devel)`になることを確認）。

この結果、「コミット済みできれいな状態のローカルビルド」と「本物の
`go install pkg@version`」は`Main.Version`だけでは区別できない（区別する
必要も無い——どちらも「このコミットから作られた」という同じ情報)。
区別が必要なのは**コミットされていない変更がある（dirtyな）ローカル
ビルド**の場合で、この場合`Main.Version`に`+dirty`サフィックスが付く。
`+dirty`のバージョンをそのまま表示すると、配布不可能な状態のビルドが
あたかも実在するバージョンであるかのように見えてしまうため、
`ReadBuildInfo()`が返す`Settings`から`vcs.modified`キー（`"true"`/
`"false"`）を読み、`true`なら`dev`へフォールバックする
（`cmd/san-db-ox/main.go`の`isDirtyBuild`。回帰テスト:
`TestIsDirtyBuildReadsVCSModifiedSetting`）。

**検証時の注意:** `git stash`でコミット前の変更を一時退避してから
ビルドする、という素朴なテスト方法は**罠になる**——退避後に残る`main.go`が
そもそも`resolvedVersion`実装より前のコミットの内容であれば、新しい実装の
動作を検証しているつもりで古いコードをビルド・実行してしまう（本セッションで
実際に踏んだ）。**新しいコードのまま「コミット済みできれいな状態」を再現
したい場合は、リポジトリを丸ごと別ディレクトリへコピーし、そこで
新規に`git init`＋`git add -A`＋`git commit`してからビルドする**こと。

## リポジトリへのバイナリコミットは行わない

各OS/アーキテクチャ向けの空バイナリは、リポジトリに直接コミットせず、
**GitHub Releasesのアセットとしてのみ配布する**。バイナリファイルはGitの
差分管理と相性が悪く、コミットのたびにリポジトリサイズが際限なく増大する
ため。タグ（例: `v1.0.0`）のpushをトリガーに、GitHub Actions
（`.github/workflows/release.yml`）で各OS/アーキテクチャ向けにクロス
コンパイルし、Releaseへ自動アップロードするパイプラインを構築する。

## `release.yml` の方針

- **トリガー:** `v*`にマッチするタグのpush（`push: tags: - 'v*'`）。
- **ビルド方式:** `CGO_ENABLED=0`のクロスコンパイルを`ubuntu-latest`
  1台で完結させる。本体の通常ビルドと同じpure Go方針であり、OSごとの
  ランナーやCコンパイラは不要。
- **バージョンの埋め込み:** `git describe`ではなく、pushされたタグ名
  （`github.ref_name`）をそのまま`-ldflags -X main.version=`へ渡す。
  タグ自体がバージョンの真実の源であるリリースビルドでは、こちらの方が
  `-dirty`サフィックス等の曖昧さがなく確実。
- **ビルド前のゲート:** `build`ジョブの前に`check`ジョブ（`make check`）を
  挟む。タグはmain上のコミットを指すのが通常だが、`test.yml`のCIを
  経ていないコミットへタグを打ってしまう事故を防ぐための安全網。
- **配布形式:** **アーカイブ化せず、生のバイナリをそのままアセットとして
  公開する**（`san-db-ox_<tag>_<goos>_<goarch>`、Windowsのみ`.exe`拡張子）。
  SanDBox自体の「ダウンロードしてそのまま実行するだけ」というゼロセットアップ
  の訴求と一貫させるための意図的な選択——tar.gz/zipへのアーカイブ化は
  README/LICENSE同梱ができる代わりに、ダウンロード後の展開という一手間が
  増える。各アセットには`sha256sum`の出力をそのまま`.sha256`ファイルとして
  添付する。
- **リリース公開:** タグから直接リリースを作成し、生成されたバイナリ・
  チェックサムをアセットとしてアップロードする（リリースノートはコミット
  履歴から自動生成）。ドラフトではなく、タグpush時点で即座に公開する。

## READMEからのリンク

`README.md`/`README_ja.md`からリリースへリンクする際は、個別のタグではなく
`/releases/latest` を指すこと。タグを打つたびに自動的に最新へ向くため、
以後のリリースでURL自体を更新する必要がない。

## 実行ビットについて

Linux/macOS向けアセットは実行ビットが立った状態でダウンロードされるとは
限らない（GitHub Releases経由では失われる）。READMEのクイックスタートには
`chmod +x` を含めること。Windowsには実行ビットという概念が無く、拡張子
（`.exe`）でのみ判定される（`testing.md`参照）。
