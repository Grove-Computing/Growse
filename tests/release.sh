#!/usr/bin/env bash
set -euo pipefail

workflow=".github/workflows/release.yml"

require() {
    if ! grep -Fq -- "$1" "$workflow"; then
        echo "release workflowに必須設定がありません: $1" >&2
        exit 1
    fi
}

for value in \
    "runner: ubuntu-24.04" \
    "runner: macos-15-intel" \
    "runner: macos-15" \
    "runner: windows-2025" \
    "artifact: linux_amd64" \
    "artifact: macos_amd64" \
    "artifact: macos_arm64" \
    "artifact: windows_amd64" \
    "examples/dashboard" \
    'tar -czf "dist/$archive_name"' \
    "Compress-Archive" \
    ".sha256" \
    "if-no-files-found: error"
do
    require "$value"
done

echo "release成果物matrix検証成功: Linux, macOS Intel/Apple Silicon, Windows"
