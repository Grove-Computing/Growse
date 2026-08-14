#!/usr/bin/env bash
set -euo pipefail

workflow=".github/workflows/release.yml"
package_script="scripts/package-gui.sh"
desktop_entry="packaging/linux/io.github.grovecomputing.Growse.desktop"
desktop_icon="packaging/linux/io.github.grovecomputing.Growse.png"
macos_plist="packaging/macos/Info.plist"
windows_icon="packaging/windows/Growse.ico"
windows_shortcut="packaging/windows/install-shortcut.ps1"

require() {
    if ! grep -Fq -- "$1" "$workflow" && ! grep -Fq -- "$1" "$package_script"; then
        echo "release workflowまたはpackage scriptに必須設定がありません: $1" >&2
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
    'application_id="io.github.grovecomputing.Growse"' \
    '-X gioui.org/app.ID=${application_id}' \
    "packaging/linux/io.github.grovecomputing.Growse.desktop" \
    "packaging/linux/io.github.grovecomputing.Growse.png" \
    "Growse.app/Contents/MacOS/growse" \
    "packaging/macos/Info.plist" \
    "iconutil -c icns" \
    "codesign --force --deep --sign -" \
    "packaging/windows/Growse.ico" \
    "packaging/windows/install-shortcut.ps1" \
    "github.com/tc-hib/go-winres@v0.3.3 simply" \
    '--manifest gui' \
    'tar -czf "dist/$archive_name"' \
    "Compress-Archive" \
    ".sha256" \
    "if-no-files-found: error"
do
    require "$value"
done

for asset in \
    "$desktop_entry" \
    "$desktop_icon" \
    "$macos_plist" \
    "$windows_icon" \
    "$windows_shortcut"
do
    if [[ ! -s "$asset" ]]; then
        echo "Linux Desktop統合Assetがありません: $asset" >&2
        exit 1
    fi
done

for value in \
    "CFBundleExecutable" \
    "CFBundleIconFile" \
    "CFBundleIdentifier" \
    "io.github.grovecomputing.Growse" \
    "@GROWSE_SHORT_VERSION@" \
    "@GROWSE_BUILD_VERSION@"
do
    if ! grep -Fq -- "$value" "$macos_plist"; then
        echo "macOS Info.plistに必須設定がありません: $value" >&2
        exit 1
    fi
done

for value in \
    'GetFolderPath("Programs")' \
    'CreateShortcut($shortcutPath)' \
    '$shortcut.TargetPath = $TargetPath' \
    '$shortcut.IconLocation = "$IconPath,0"'
do
    if ! grep -Fq -- "$value" "$windows_shortcut"; then
        echo "Windows Shortcut Scriptに必須設定がありません: $value" >&2
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

echo "release成果物matrix検証成功: Linux Desktop, macOS App Bundle, Windows Start Menu"
