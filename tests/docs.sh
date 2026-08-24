#!/usr/bin/env bash
set -euo pipefail

require() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にv0.13.0文書の必須記述がありません: ${value}" >&2
        exit 1
    fi
}

require README.md "examples/dashboard"
require README.md "examples/animation"
require README.md "examples/data-app"
require README.md "examples/persistent-app"
require README.md "examples/multi-tab-workspace"
require README.md "examples/devtools"
require README.md "examples/dual-runtime"
require README.md "GROWSE_VERSION=v0.13.0"
require README.md "growse:v0.13.0"
require README.md "Desktop Entry"
require README.md "GROWSE_DATA_HOME"
require README.md "GROWSE_APPLICATIONS_DIR"
require README.md "GROWSE_WINDOWS_PROGRAMS_DIR"
require README.md "Growse.app"
require README.md "Start Menu Programs"
require README.md "CSS Transition"
require README.md 'growse/fetch'
require README.md 'growse/navigation'
require README.md 'growse/scheduler'
require README.md 'growse/storage'
require README.md 'OnChange'
require README.md "Vertical Tab"
require README.md "CORS"
require README.md "Storage / Cache対応表"
require README.md "v0.9.0リリース定義"
require README.md "v0.10.0リリース定義"
require README.md "v0.12.0リリース定義"
require README.md "v0.13.0リリース定義"
require README.md "WebGo DevTools"
require README.md "Runtime / Web API対応表"
require SECURITY.md "| 0.13.x | Yes |"
require SECURITY.md "| 0.12.x | No |"
require SECURITY.md "JavaScript hostはOS、filesystem、process、Go reflection、Node.js APIを公開しません"
require SECURITY.md "Network recordにはRequest / Response body、Header、Cookie、Authorization、error本文を保持せず"
require docs/devtools.md "Page 500件"
require docs/devtools.md "Browser Session: 4,000件"
require docs/devtools.md '`[REDACTED]`'
require docs/devtools.md "EngineとRuntimeのidle / running / stopped / error状態"
require docs/devtools.md '`script/javascript`'
require SECURITY.md "Page全体で4096件"
require SECURITY.md "最大深度8"
require SECURITY.md "SameSite"
require SECURITY.md "Request Bodyは1 MiB"
require SECURITY.md "Timer 10,000件"
require SECURITY.md "Historyは1,024 entry"
require SECURITY.md "Originごと5 MiB"
require SECURITY.md "diskは1 entry 4 MiB"
require docs/css-support.md "Growse v0.7.0"
require docs/css-support.md '`grid`、`inline-grid`'
require docs/css-support.md '`transform`、`transform-origin`'
require docs/css-support.md '`transition-*`、`transition`'
require docs/css-support.md '`animation-*`、`animation`'
require docs/css-support.md '`prefers-reduced-motion`'
require docs/css-support.md "Subgrid、masonry"
require SECURITY.md "TabはDOM、Runtime、History、Session Storageを分離"
require docs/form-fetch-cookie-support.md "Growse v0.13.0"
require docs/form-fetch-cookie-support.md "AbortController"
require docs/form-fetch-cookie-support.md "JavaScript Promise"
require docs/form-fetch-cookie-support.md "Credentials Mode"
require docs/form-fetch-cookie-support.md "SameSite"
require docs/form-fetch-cookie-support.md "CORS"
require docs/form-fetch-cookie-support.md "HTTP Cache"
require docs/wpt.md "## v0.8.0の選定範囲"
require docs/wpt.md "## v0.9.0の選定範囲"
require docs/wpt.md "## v0.10.0の選定範囲"
require docs/wpt.md "## v0.11.0の選定範囲"
require docs/wpt.md "## v0.13.0の選定範囲"
require docs/wpt.md "CSS Transitions Level 2"
require docs/wpt.md "HTML Forms"
require docs/wpt.md "RFC 9111"
require docs/wpt.md "Storage Event"
require docs/storage-cache-support.md "Growse v0.13.0"
require docs/storage-cache-support.md 'growse/storage'
require docs/storage-cache-support.md 'localStorage'
require docs/storage-cache-support.md "Origin 5 MiB"
require docs/storage-cache-support.md "memory 1,024 entry"
require docs/storage-cache-support.md "Body SHA-256"
require docs/storage-cache-support.md "RFC 9111"
require docs/storage-cache-support.md "Tab終了時に破棄"
require docs/storage-cache-support.md "same-origin Tab"
require docs/runtime-support.md "Growse v0.13.0"
require docs/runtime-support.md "Yaegi"
require docs/runtime-support.md "goja"
require docs/runtime-support.md "Process Sandboxではない"

if grep -Fq "Growse v0.6.0の実装を基準" docs/css-support.md; then
    echo "CSS対応表に古いv0.6.0基準が残っています" >&2
    exit 1
fi

for file in README.md SECURITY.md docs/form-fetch-cookie-support.md docs/storage-cache-support.md; do
    if grep -Eq 'GROWSE_VERSION=v0\.9\.0|growse:v0\.9\.0|Growse v0\.9\.0のYaegi|Growse v0\.9\.0の実装を基準' "$file"; then
        echo "${file}にv0.9.0向けの現行記述が残っています" >&2
        exit 1
    fi
done

echo "v0.13.0文書同期検証成功: README, SECURITY.md, Runtime / Web API対応表, DevTools, Storage / Cache対応表, Form / Fetch / Cookie対応表, CSS対応表, WPT文書, Showcase"
