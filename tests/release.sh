#!/usr/bin/env bash
set -euo pipefail

workflow=".github/workflows/release.yml"
desktop_entry="packaging/linux/io.github.grovecomputing.Growse.desktop"
desktop_icon="packaging/linux/io.github.grovecomputing.Growse.png"

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
    "examples/animation" \
    "examples/data-app" \
    "-X gioui.org/app.ID=io.github.grovecomputing.Growse" \
    "packaging/linux/io.github.grovecomputing.Growse.desktop" \
    "packaging/linux/io.github.grovecomputing.Growse.png" \
    'tar -czf "dist/$archive_name"' \
    "Compress-Archive" \
    ".sha256" \
    "if-no-files-found: error"
do
    require "$value"
done

for asset in "$desktop_entry" "$desktop_icon"; do
    if [[ ! -s "$asset" ]]; then
        echo "Linux Desktop統合Assetがありません: $asset" >&2
        exit 1
    fi
done

for value in \
    "Type=Application" \
    "Exec=growse" \
    "Icon=io.github.grovecomputing.Growse" \
    "Categories=Network;WebBrowser;" \
    "StartupWMClass=io.github.grovecomputing.Growse"
do
    if ! grep -Fq -- "$value" "$desktop_entry"; then
        echo "Desktop Entryに必須設定がありません: $value" >&2
        exit 1
    fi
done

echo "release成果物matrix検証成功: Linux Desktop統合, macOS Intel/Apple Silicon, Windows"
