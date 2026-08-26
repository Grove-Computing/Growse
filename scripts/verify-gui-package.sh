#!/usr/bin/env bash
set -euo pipefail

package_dir=${1:?package directory is required}
goos=${GOOS:-$(go env GOOS)}

case "$goos" in
  darwin)
    executable="${package_dir}/Growse.app/Contents/MacOS/growse"
    ;;
  windows)
    executable="${package_dir}/growse.exe"
    ;;
  linux)
    executable="${package_dir}/growse"
    ;;
  *)
    echo "Unsupported GUI package platform: ${goos}" >&2
    exit 1
    ;;
esac

if [[ ! -f "$executable" ]]; then
  echo "GUI package executableがありません: ${executable}" >&2
  exit 1
fi

resources=(
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
for resource in "${resources[@]}"; do
  if [[ ! -d "${package_dir}/examples/${resource}" ]]; then
    echo "GUI package resourceがありません: examples/${resource}" >&2
    exit 1
  fi
done
for fixture in \
  "${package_dir}/examples/external-web-platform/index.html" \
  "${package_dir}/examples/external-web-platform/app.mjs" \
  "${package_dir}/examples/external-web-platform/server.go" \
  "${package_dir}/examples/external-web-platform/sw.js"
do
  if [[ ! -s "$fixture" ]]; then
    echo "External Web Platform resourceがありません: ${fixture}" >&2
    exit 1
  fi
done

go run ./cmd/workercheck "$executable"
echo "GUI package検証成功: ${goos}, Runtime / Service Worker起動・終了, ${#resources[@]} resource"
