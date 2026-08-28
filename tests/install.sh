#!/usr/bin/env bash

set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

version=${GROWSE_TEST_VERSION:-v0.15.0}
release_dir="${test_root}/releases/${version}"
package_dir="${test_root}/package"
install_dir="${test_root}/install dir/bin"
data_home="${test_root}/data home"
archive="growse_${version}_linux_amd64.tar.gz"
app_id="io.github.grovecomputing.Growse"
mkdir -p \
  "$release_dir" \
  "${package_dir}/share/applications" \
  "${package_dir}/share/icons/hicolor/512x512/apps"

printf '%s\n' '#!/usr/bin/env sh' "printf '%s\\n' '${version}'" > "${package_dir}/growse"
chmod 0755 "${package_dir}/growse"
cp "${repository_root}/packaging/linux/${app_id}.desktop" \
  "${package_dir}/share/applications/"
cp "${repository_root}/packaging/linux/${app_id}.png" \
  "${package_dir}/share/icons/hicolor/512x512/apps/"
tar -czf "${release_dir}/${archive}" -C "$package_dir" .
(
  cd "$release_dir"
  sha256sum "$archive" > "${archive}.sha256"
)

installer_output=$(
  GROWSE_COLOR=never \
  GROWSE_VERSION="$version" \
  GROWSE_INSTALL_DIR="$install_dir" \
  GROWSE_DATA_HOME="$data_home" \
  GROWSE_RELEASE_BASE_URL="file://${test_root}/releases" \
  bash "${repository_root}/install.sh" 2>&1
)
printf '%s\n' "$installer_output"

for expected_output in \
  "Growse Installer" \
  "[1/4] リリースを確認" \
  "[2/4] 実行環境を判定" \
  "[3/4] パッケージを取得して検証" \
  "[4/4] Growseをインストール" \
  "✓ Growse ${version}のインストールが完了しました" \
  "Command  ${install_dir}/growse"
do
  if [[ "$installer_output" != *"$expected_output"* ]]; then
    echo "Installer出力に必須メッセージがありません: $expected_output" >&2
    exit 1
  fi
done

if [[ "$installer_output" == *$'\033['* ]]; then
  echo "GROWSE_COLOR=neverでもANSI Colorが出力されています。" >&2
  exit 1
fi

color_install_dir="${test_root}/color install/bin"
color_data_home="${test_root}/color data home"
color_output=$(
  GROWSE_COLOR=always \
  GROWSE_VERSION="$version" \
  GROWSE_INSTALL_DIR="$color_install_dir" \
  GROWSE_DATA_HOME="$color_data_home" \
  GROWSE_RELEASE_BASE_URL="file://${test_root}/releases" \
  bash "${repository_root}/install.sh" 2>&1
)
if [[ "$color_output" != *$'\033['* ]]; then
  echo "GROWSE_COLOR=alwaysでANSI Colorが出力されていません。" >&2
  exit 1
fi

invalid_color_output="${test_root}/invalid-color-output"
if GROWSE_COLOR=rainbow bash "${repository_root}/install.sh" >"$invalid_color_output" 2>&1; then
  echo "不正なGROWSE_COLORが受理されました。" >&2
  exit 1
fi
if ! grep -Fq "無効なGROWSE_COLORです" "$invalid_color_output"; then
  echo "不正なGROWSE_COLORのErrorが表示されていません。" >&2
  exit 1
fi

if [[ ! -x "${install_dir}/growse" ]]; then
  echo "growseがインストールされていません。" >&2
  exit 1
fi

installed_version=$("${install_dir}/growse")
if [[ "$installed_version" != "$version" ]]; then
  echo "インストール結果が不正です: ${installed_version}" >&2
  exit 1
fi

desktop_file="${data_home}/applications/${app_id}.desktop"
icon_file="${data_home}/icons/hicolor/512x512/apps/${app_id}.png"
if [[ ! -f "$desktop_file" || ! -f "$icon_file" ]]; then
  echo "Linux Desktop統合Assetがインストールされていません。" >&2
  exit 1
fi
if ! grep -Fq "Exec=\"${install_dir}/growse\"" "$desktop_file"; then
  echo "Desktop EntryのExecがインストール先と一致しません。" >&2
  exit 1
fi
if grep -Fxq "Exec=growse" "$desktop_file"; then
  echo "Desktop EntryのExecがインストール先へ展開されていません。" >&2
  exit 1
fi
if ! grep -Fq "Icon=${app_id}" "$desktop_file"; then
  echo "Desktop EntryのIcon IDが不正です。" >&2
  exit 1
fi
if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$desktop_file"
fi

echo "インストーラー検証成功: ${installed_version}, Linux Desktop統合"
