#!/usr/bin/env bash
set -euo pipefail

require() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にv0.17.0文書の必須記述がありません: ${value}" >&2
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
require README.md "examples/external-web-platform"
require README.md "examples/modern-web-compat"
require README.md "examples/browser-grade-compat"
require README.md "GROWSE_VERSION=v0.17.0"
require README.md "growse:v0.17.0"
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
require README.md "v0.14.0リリース定義"
require README.md "v0.15.0リリース定義"
require README.md "v0.16.0リリース定義"
require README.md "v0.17.0リリース定義"
require README.md "External JavaScript"
require README.md "WebAssembly"
require README.md "Service Worker"
require README.md "Runtime / Web API対応表"
require README.md "Runtime worker / Web Platform設計"
require README.md "Modern Web Compatibility"
require README.md "明示選択"
require SECURITY.md "| 0.17.x | Yes |"
require SECURITY.md "| 0.16.x | No |"
require SECURITY.md "brokered host I/O"
require SECURITY.md "未知"
require SECURITY.md "Service Worker registrationとCache Storage"
require SECURITY.md "IPC payload"
require docs/devtools.md "Page 2,000件"
require docs/devtools.md "Browser Session: 4,000件"
require docs/devtools.md '`[REDACTED]`'
require docs/devtools.md "EngineとRuntimeのidle / running / stopped / error状態"
require docs/devtools.md '`script/javascript`'
require docs/devtools.md "## Runtime"
require docs/devtools.md "worker generation"
require docs/devtools.md "Service Worker Cache body"
require docs/devtools.md "image cache"
require docs/devtools.md "frame rebuild"
require SECURITY.md "Page全体で4096件"
require SECURITY.md "最大深度8"
require SECURITY.md "SameSite"
require SECURITY.md "Request Bodyは1 MiB"
require SECURITY.md "Timer 10,000件"
require SECURITY.md "Historyは1,024 entry"
require SECURITY.md "Originごと5 MiB"
require SECURITY.md "diskは1 entry 4 MiB"
require docs/css-support.md "Growse v0.17.0"
require docs/css-support.md '`grid`、`inline-grid`'
require docs/css-support.md '`transform`、`transform-origin`'
require docs/css-support.md '`transition-*`、`transition`'
require docs/css-support.md '`animation-*`、`animation`'
require docs/css-support.md '`prefers-reduced-motion`'
require docs/css-support.md "Subgrid、masonry"
require docs/css-support.md "same / cross-origin HTTP(S)"
require docs/css-support.md "Cascade Layer"
require docs/css-support.md "has()"
require docs/css-support.md "CSS Color Level 4 subset"
require docs/css-support.md "Modern Web Compatibility Showcase"
require docs/css-support.md "Tailwind CSS v4.1.12"
require docs/css-support.md "system font fallback"
require SECURITY.md "TabはDOM、Runtime worker、History、Session Storageを分離"
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
require docs/wpt.md "## v0.14.0の選定範囲"
require docs/wpt.md "## v0.15.0の選定範囲"
require docs/wpt.md "CSS Transitions Level 2"
require docs/wpt.md "HTML Forms"
require docs/wpt.md "RFC 9111"
require docs/wpt.md "Storage Event"
require docs/wpt.md "TestWPTModuleSharedDependencyEvaluatesOnce"
require docs/wpt.md "TestWPTWebAssemblyModuleConstructorValidatesBytes"
require docs/wpt.md "TestWPTIframeEmptySandboxBlocksScriptExecution"
require docs/wpt.md "TestWPTServiceWorkerDefaultScopeIsScriptDirectory"
require docs/wpt.md "TestDynamicClassicScriptsSnapshotFetchAndExecuteExactlyOnce"
require docs/wpt.md "TestLevel4SelectorMatchingHasScopeIsWhereAndComplexNot"
require docs/wpt.md "TestResizeAndIntersectionObserversRunAfterFrame"
require docs/wpt.md "TestImageCandidatesSelectPictureSourceByTypeMediaSizesAndScale"
require docs/wpt.md "TestLoadWebFontsValidatesDescriptorsAndDecodesWOFF"
require docs/storage-cache-support.md "Growse v0.13.0"
require docs/storage-cache-support.md 'growse/storage'
require docs/storage-cache-support.md 'localStorage'
require docs/storage-cache-support.md "Origin 5 MiB"
require docs/storage-cache-support.md "memory 1,024 entry"
require docs/storage-cache-support.md "Body SHA-256"
require docs/storage-cache-support.md "RFC 9111"
require docs/storage-cache-support.md "Tab終了時に破棄"
require docs/storage-cache-support.md "same-origin Tab"
require docs/runtime-support.md "Growse v0.17.0"
require docs/runtime-support.md "Yaegi"
require docs/runtime-support.md "goja"
require docs/runtime-support.md "ECMAScript Modules"
require docs/runtime-support.md "WebAssembly"
require docs/runtime-support.md "iframeとService Worker"
require docs/runtime-support.md "fail closed"
require docs/runtime-support.md "Dynamic resource"
require docs/runtime-support.md "CSSOM / media"
require docs/runtime-support.md "Observer"
require docs/runtime-worker-design.md "typed IPC broker"
require docs/runtime-worker-design.md "## sandbox検証"
require docs/runtime-worker-design.md "旧generation"
require docs/runtime-worker-design.md "seccomp"
require docs/runtime-worker-design.md "Growse v0.17.0"
require docs/visual-regression.md "v0.15.0 Modern Web Compatibility"
require docs/visual-regression.md "tests/v015-visual.sh"
require docs/visual-regression.md "v0.16.0 Real-site Rendering & Performance"
require docs/visual-regression.md "real-site-visual.golden.json"
require docs/performance.md "v0.16.0 Real-site image / animation budget"
require docs/performance.md "RenderMetricsSnapshot"
require docs/visual-regression.md "v0.17.0 Browser-grade Differential"
require docs/visual-regression.md "2 CSS px"
require docs/performance.md "v0.17.0 Browser differential / frame budget"
require docs/performance.md "performance-gate.json"
require docs/wpt.md "## v0.17.0の選定範囲"
require examples/modern-web-compat/index.html "Tailwind v4 real-site"
require examples/modern-web-compat/index.html "/real-site/"
require examples/modern-web-compat/index.html "Growse v0.17.0"
require examples/browser-grade-compat/index.html "Browser-grade Compatibility"
require examples/browser-grade-compat/corpus.json '"release": "v0.17.0"'
require docs/details-design.md "歴史資料"
require docs/details-design.md "runtime-worker-design.md"

if grep -Eq "Growse v0\.[4-9]\.0の実装を基準|Growse v0\.1[0-6]\.0の実装を基準" docs/css-support.md; then
    echo "CSS対応表に古い実装基準が残っています" >&2
    exit 1
fi

if grep -Eq 'GROWSE_VERSION=v0\.1[3-6]\.0|growse:v0\.1[3-6]\.0' README.md; then
    echo "README.mdに古い現行install例が残っています" >&2
    exit 1
fi
if grep -Eq '\| 0\.1[3-6]\.x \| Yes \|' SECURITY.md; then
    echo "SECURITY.mdに古いsupport系列が残っています" >&2
    exit 1
fi
if grep -Eq 'Growse v0\.1[3-6]\.0の実装を基準|Process Sandboxではない|ECMAScript module、dynamic import、import map' docs/runtime-support.md; then
    echo "Runtime対応表にv0.13.0の非対応説明が残っています" >&2
    exit 1
fi
if grep -Fq 'Growse v0.13.0は' docs/devtools.md; then
    echo "DevTools設計に古い実装基準が残っています" >&2
    exit 1
fi

echo "v0.17.0文書同期検証成功: README, SECURITY.md, Runtime / Web API対応表, Runtime worker設計, DevTools, Performance / CSS / Visual / WPT文書, Browser-grade Showcase"
