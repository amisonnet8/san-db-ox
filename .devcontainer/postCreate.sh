#!/usr/bin/env bash
set -euo pipefail

sudo apt-get update
sudo apt-get install -y make wget apt-transport-https gnupg lsb-release

# sqlite3:    出力フォーマットの参照実装(.claude/rules/cli-output.md「出力スタイル」)。
#             .snapshot --sqlite が生成したファイルの検証にも使う。
# socat:      stdioプロトコルをTCP/UNIXソケットへ外付けする運用の動作確認
#             (仕様書§8、.claude/rules/testing.md の複数プロセス同時起動の検証)。
# jq:         JSON Lines のデバッグ。対話パイプラインで使う場合は --unbuffered が要る。
# ShellCheck: tests/*.sh 等の静的解析（`make shellcheck`、.claude/rules/testing.md）。
#             行頭を小文字の "# shellcheck" にすると、ShellCheck自身が
#             インラインディレクティブとして解釈しようとしパースエラーに
#             なるため、大文字始まりで書く（SC1072/SC1073、実際に踏んだ）。
sudo apt-get install -y sqlite3 socat jq shellcheck

# trivy: 依存ライブラリの脆弱性・ライセンスチェック(.claude/rules/testing.md)。
# aquasecurity公式のaptリポジトリからインストールする。
wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor | sudo tee /usr/share/keyrings/trivy.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb $(lsb_release -sc) main" | sudo tee /etc/apt/sources.list.d/trivy.list
sudo apt-get update
sudo apt-get install -y trivy

# gh: GitHub Issues/PRの確認・操作用CLI。san-db-ox-clients(別リポジトリ、
# directory-structure.md参照)側のIssue確認等で使う。GitHub公式のaptリポジトリ
# からインストールする(trivyと同じパターン)。
sudo mkdir -p -m 755 /etc/apt/keyrings
wget -qO - https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg >/dev/null
sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null
sudo apt-get update
sudo apt-get install -y gh

go install golang.org/x/tools/gopls@latest
go install golang.org/x/tools/cmd/goimports@latest
go install golang.org/x/tools/cmd/stringer@latest
