#!/usr/bin/env bash

set -euo pipefail

package_dir=${1:?package directory is required}
release_tag=${2:?release tag is required}
build_version=${3:-1}
goos=${GOOS:-$(go env GOOS)}
goarch=${GOARCH:-$(go env GOARCH)}
application_id="io.github.grovecomputing.Growse"
generated_resource=""

cleanup() {
  if [[ -n "$generated_resource" && -f "$generated_resource" ]]; then
    rm -f "$generated_resource"
  fi
}
trap cleanup EXIT

if [[ ! "$release_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Invalid release tag: $release_tag" >&2
  exit 1
fi
if [[ ! "$build_version" =~ ^[0-9]+$ ]]; then
  echo "Invalid build version: $build_version" >&2
  exit 1
fi

mkdir -p "$package_dir"
output="${package_dir}/growse"
ldflags="-s -w -X gioui.org/app.ID=${application_id} -X github.com/Grove-Computing/Growse/internal/updater.CurrentVersion=${release_tag}"
short_version="${release_tag#v}"
short_version="${short_version%%-*}"

case "$goos" in
  darwin)
    output="${package_dir}/Growse.app/Contents/MacOS/growse"
    mkdir -p \
      "${package_dir}/Growse.app/Contents/MacOS" \
      "${package_dir}/Growse.app/Contents/Resources"
    ;;
  windows)
    if [[ "$goarch" != "amd64" ]]; then
      echo "Unsupported Windows architecture: $goarch" >&2
      exit 1
    fi
    output="${output}.exe"
    ldflags="$ldflags -H windowsgui"
    go run github.com/tc-hib/go-winres@v0.3.3 simply \
      --arch amd64 \
      --out cmd/growse/rsrc \
      --product-version "${short_version}.0" \
      --file-version "${short_version}.0" \
      --manifest gui \
      --file-description "Growse Web Browser" \
      --product-name "Growse" \
      --original-filename "growse.exe" \
      --icon packaging/windows/Growse.ico
    generated_resource="cmd/growse/rsrc_windows_amd64.syso"
    ;;
esac

go build -trimpath -ldflags="$ldflags" -o "$output" ./cmd/growse

example_resources=(
  animation
  counter
  css3-core
  dashboard
  data-app
  devtools
  dual-runtime
  external-web-platform
  flexbox
  multi-tab-workspace
  persistent-app
  todo
)
mkdir -p "${package_dir}/examples"
for example in "${example_resources[@]}"; do
  cp -R "examples/${example}" "${package_dir}/examples/"
done

case "$goos" in
  linux)
    mkdir -p \
      "${package_dir}/share/applications" \
      "${package_dir}/share/icons/hicolor/512x512/apps"
    cp packaging/linux/io.github.grovecomputing.Growse.desktop \
      "${package_dir}/share/applications/"
    cp packaging/linux/io.github.grovecomputing.Growse.png \
      "${package_dir}/share/icons/hicolor/512x512/apps/"
    ;;
  darwin)
    sed \
      -e "s/@GROWSE_SHORT_VERSION@/$short_version/g" \
      -e "s/@GROWSE_BUILD_VERSION@/$build_version/g" \
      packaging/macos/Info.plist \
      > "${package_dir}/Growse.app/Contents/Info.plist"
    iconset_root=$(mktemp -d "${TMPDIR:-/tmp}/growse-iconset.XXXXXX")
    iconset="${iconset_root}/Growse.iconset"
    mkdir -p "$iconset"
    for size in 16 32 128 256 512; do
      double_size=$((size * 2))
      sips -z "$size" "$size" \
        packaging/linux/io.github.grovecomputing.Growse.png \
        --out "$iconset/icon_${size}x${size}.png"
      sips -z "$double_size" "$double_size" \
        packaging/linux/io.github.grovecomputing.Growse.png \
        --out "$iconset/icon_${size}x${size}@2x.png"
    done
    iconutil -c icns "$iconset" \
      -o "${package_dir}/Growse.app/Contents/Resources/Growse.icns"
    rm -rf "$iconset_root"
    plutil -lint "${package_dir}/Growse.app/Contents/Info.plist"
    codesign --force --deep --sign - "${package_dir}/Growse.app"
    ;;
  windows)
    mkdir -p "${package_dir}/share/windows"
    cp packaging/windows/Growse.ico "${package_dir}/share/windows/"
    cp packaging/windows/install-shortcut.ps1 "${package_dir}/share/windows/"
    ;;
esac
