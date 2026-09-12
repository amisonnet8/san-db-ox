#!/usr/bin/env bash
set -euo pipefail

sudo apt-get update
sudo apt-get install -y make wget apt-transport-https gnupg lsb-release

# sqlite3: 出力フォーマットの参照実装(.claude/rules/cli-output.md「出力スタイル」)。
#          .snapshot --sqlite が生成したファイルの検証にも使う。
# socat:   stdioプロトコルをTCP/UNIXソケットへ外付けする運用の動作確認
#          (仕様書§8、.claude/rules/testing.md の複数プロセス同時起動の検証)。
# jq:      JSON Lines のデバッグ。対話パイプラインで使う場合は --unbuffered が要る。
sudo apt-get install -y sqlite3 socat jq

# trivy: 依存ライブラリの脆弱性・ライセンスチェック(.claude/rules/testing.md)。
# aquasecurity公式のaptリポジトリからインストールする。
wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor | sudo tee /usr/share/keyrings/trivy.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb $(lsb_release -sc) main" | sudo tee /etc/apt/sources.list.d/trivy.list
sudo apt-get update
sudo apt-get install -y trivy

go install golang.org/x/tools/gopls@latest
go install golang.org/x/tools/cmd/goimports@latest
go install golang.org/x/tools/cmd/stringer@latest
