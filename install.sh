#!/usr/bin/env bash

set -euo pipefail

repository="${GROWSE_REPOSITORY:-saku0512/growse}"
version="${GROWSE_VERSION:-latest}"
install_dir="${GROWSE_INSTALL_DIR:-${HOME}/.local/bin}"

download() {
  local url=$1
  local output=$2

  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error --output "$output" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
  else
    echo "curlまたはwgetが必要です。" >&2
    exit 1
  fi
}

if [[ "$version" == "latest" ]]; then
  release_json=$(download "https://api.github.com/repos/${repository}/releases/latest" -)
  version=$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p')
  if [[ -z "$version" ]]; then
    echo "最新リリースのバージョンを取得できませんでした。" >&2
    exit 1
  fi
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "無効なバージョンです: $version" >&2
  exit 1
fi

case "$(uname -s)" in
  Linux*)
    platform=linux
    extension=tar.gz
    executable=growse
    ;;
  Darwin*)
    platform=macos
    extension=tar.gz
    executable=growse
    ;;
  MINGW* | MSYS* | CYGWIN*)
    platform=windows
    extension=zip
    executable=growse.exe
    ;;
  *)
    echo "未対応のOSです: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64)
    arch=amd64
    ;;
  arm64 | aarch64)
    arch=arm64
    ;;
  *)
    echo "未対応のCPUアーキテクチャです: $(uname -m)" >&2
    exit 1
    ;;
esac

if [[ "$platform" != "macos" && "$arch" == "arm64" ]]; then
  echo "${platform} arm64向けのリリースはまだありません。" >&2
  exit 1
fi

archive="growse_${version}_${platform}_${arch}.${extension}"
checksum="${archive}.sha256"
release_url="https://github.com/${repository}/releases/download/${version}"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

echo "Growse $version ($platform/$arch) をダウンロードしています..."
download "${release_url}/${archive}" "${work_dir}/${archive}"
download "${release_url}/${checksum}" "${work_dir}/${checksum}"

expected_hash=$(awk '{print $1; exit}' "${work_dir}/${checksum}")
if command -v sha256sum >/dev/null 2>&1; then
  actual_hash=$(sha256sum "${work_dir}/${archive}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual_hash=$(shasum -a 256 "${work_dir}/${archive}" | awk '{print $1}')
elif command -v openssl >/dev/null 2>&1; then
  actual_hash=$(openssl dgst -sha256 "${work_dir}/${archive}" | awk '{print $NF}')
else
  echo "SHA-256を検証できるコマンドがありません。" >&2
  exit 1
fi

if [[ "$actual_hash" != "$expected_hash" ]]; then
  echo "SHA-256チェックサムが一致しません。" >&2
  exit 1
fi

package_dir="${work_dir}/package"
mkdir -p "$package_dir"
if [[ "$extension" == "zip" ]]; then
  if ! command -v unzip >/dev/null 2>&1; then
    echo "Windows版の展開にはunzipが必要です。" >&2
    exit 1
  fi
  unzip -q "${work_dir}/${archive}" -d "$package_dir"
else
  tar -xzf "${work_dir}/${archive}" -C "$package_dir"
fi

mkdir -p "$install_dir"
install -m 0755 "${package_dir}/${executable}" "${install_dir}/${executable}"

echo "Growseを ${install_dir}/${executable} にインストールしました。"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
  echo "${install_dir} をPATHへ追加してください。"
fi
