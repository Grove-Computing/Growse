#!/usr/bin/env bash

set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

version=${GROWSE_TEST_VERSION:-v0.15.0}
release_root="${test_root}/releases/${version}"
fixture_bin="${repository_root}/tests/fixtures/platform-bin"
mkdir -p "$release_root"

create_checksum() {
  local archive=$1
  (
    cd "$release_root"
    sha256sum "$archive" > "${archive}.sha256"
  )
}

test_macos_installer() {
  local package_dir="${test_root}/macos-package"
  local install_dir="${test_root}/macos install/bin"
  local applications_dir="${test_root}/Applications"
  local archive="growse_${version}_macos_amd64.tar.gz"
  mkdir -p \
    "${package_dir}/Growse.app/Contents/MacOS" \
    "${package_dir}/Growse.app/Contents/Resources"
  printf '%s\n' '#!/usr/bin/env sh' "printf '%s\\n' '${version}'" \
    > "${package_dir}/Growse.app/Contents/MacOS/growse"
  chmod 0755 "${package_dir}/Growse.app/Contents/MacOS/growse"
  sed \
    -e "s/@GROWSE_SHORT_VERSION@/${version#v}/g" \
    -e "s/@GROWSE_BUILD_VERSION@/1/g" \
    "${repository_root}/packaging/macos/Info.plist" \
    > "${package_dir}/Growse.app/Contents/Info.plist"
  cp "${repository_root}/packaging/linux/io.github.grovecomputing.Growse.png" \
    "${package_dir}/Growse.app/Contents/Resources/Growse.icns"
  tar -czf "${release_root}/${archive}" -C "$package_dir" .
  create_checksum "$archive"

  PATH="${fixture_bin}:${PATH}" \
  GROWSE_TEST_UNAME_S=Darwin \
  GROWSE_TEST_UNAME_M=x86_64 \
  GROWSE_VERSION="$version" \
  GROWSE_INSTALL_DIR="$install_dir" \
  GROWSE_APPLICATIONS_DIR="$applications_dir" \
  GROWSE_RELEASE_BASE_URL="file://${test_root}/releases" \
  bash "${repository_root}/install.sh"

  local application_path="${applications_dir}/Growse.app"
  if [[ ! -x "${application_path}/Contents/MacOS/growse" ||
        ! -f "${application_path}/Contents/Info.plist" ||
        ! -f "${application_path}/Contents/Resources/Growse.icns" ]]; then
    echo "macOS Application Bundleがインストールされていません。" >&2
    exit 1
  fi
  if [[ ! -L "${install_dir}/growse" ||
        $(readlink "${install_dir}/growse") != "${application_path}/Contents/MacOS/growse" ]]; then
    echo "macOS growse commandがApplication Bundleを参照していません。" >&2
    exit 1
  fi
}

test_windows_installer() {
  local package_dir="${test_root}/windows-package"
  local install_dir="${test_root}/windows install/bin"
  local data_home="${test_root}/windows data"
  local programs_dir="${test_root}/Start Menu/Programs"
  local record_file="${test_root}/powershell-arguments"
  local archive="growse_${version}_windows_amd64.zip"
  mkdir -p "${package_dir}/share/windows"
  printf 'fake windows executable\n' > "${package_dir}/growse.exe"
  cp "${repository_root}/packaging/windows/Growse.ico" \
    "${package_dir}/share/windows/"
  cp "${repository_root}/packaging/windows/install-shortcut.ps1" \
    "${package_dir}/share/windows/"
  (
    cd "$package_dir"
    zip -qr "${release_root}/${archive}" .
  )
  create_checksum "$archive"

  PATH="${fixture_bin}:${PATH}" \
  GROWSE_TEST_UNAME_S=MINGW64_NT-10.0 \
  GROWSE_TEST_UNAME_M=x86_64 \
  GROWSE_TEST_POWERSHELL_RECORD="$record_file" \
  GROWSE_VERSION="$version" \
  GROWSE_INSTALL_DIR="$install_dir" \
  GROWSE_DATA_HOME="$data_home" \
  GROWSE_WINDOWS_PROGRAMS_DIR="$programs_dir" \
  GROWSE_RELEASE_BASE_URL="file://${test_root}/releases" \
  bash "${repository_root}/install.sh"

  if [[ ! -x "${install_dir}/growse.exe" || ! -f "${data_home}/growse/Growse.ico" ]]; then
    echo "Windows executableまたはIconがインストールされていません。" >&2
    exit 1
  fi
  for expected in \
    "WIN:${install_dir}/growse.exe" \
    "WIN:${data_home}/growse/Growse.ico" \
    "WIN:${programs_dir}"
  do
    if ! grep -Fq -- "$expected" "$record_file"; then
      echo "Windows Shortcut引数に必須Pathがありません: $expected" >&2
      exit 1
    fi
  done
}

test_macos_installer
test_windows_installer
echo "macOS / Windows GUI Application Installer検証成功"
