#!/usr/bin/env bash

set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

version=${GROWSE_TEST_VERSION:-v0.5.0}
release_dir="${test_root}/releases/${version}"
package_dir="${test_root}/package"
install_dir="${test_root}/bin"
archive="growse_${version}_linux_amd64.tar.gz"
mkdir -p "$release_dir" "$package_dir"

printf '%s\n' '#!/usr/bin/env sh' "printf '%s\\n' '${version}'" > "${package_dir}/growse"
chmod 0755 "${package_dir}/growse"
tar -czf "${release_dir}/${archive}" -C "$package_dir" .
(
  cd "$release_dir"
  sha256sum "$archive" > "${archive}.sha256"
)

GROWSE_VERSION="$version" \
GROWSE_INSTALL_DIR="$install_dir" \
GROWSE_RELEASE_BASE_URL="file://${test_root}/releases" \
bash "${repository_root}/install.sh"

if [[ ! -x "${install_dir}/growse" ]]; then
  echo "growseがインストールされていません。" >&2
  exit 1
fi

installed_version=$("${install_dir}/growse")
if [[ "$installed_version" != "$version" ]]; then
  echo "インストール結果が不正です: ${installed_version}" >&2
  exit 1
fi

echo "インストーラー検証成功: ${installed_version}"
