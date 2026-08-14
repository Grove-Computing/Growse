#!/usr/bin/env bash
set -euo pipefail

require() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にv0.7.0文書の必須記述がありません: ${value}" >&2
        exit 1
    fi
}

require README.md "examples/dashboard"
require README.md "examples/animation"
require README.md "GROWSE_VERSION=v0.7.0"
require README.md "growse:v0.7.0"
require README.md "CSS Transition"
require SECURITY.md "| 0.7.x | Yes |"
require SECURITY.md "Page全体で4096件"
require SECURITY.md "最大深度8"
require docs/css-support.md "Growse v0.7.0"
require docs/css-support.md '`grid`、`inline-grid`'
require docs/css-support.md '`transform`、`transform-origin`'
require docs/css-support.md '`transition-*`、`transition`'
require docs/css-support.md '`animation-*`、`animation`'
require docs/css-support.md '`prefers-reduced-motion`'
require docs/css-support.md "Subgrid、masonry"
require docs/wpt.md "## v0.7.0の選定範囲"
require docs/wpt.md "CSS Transitions Level 2"

if grep -Fq "Growse v0.6.0の実装を基準" docs/css-support.md; then
    echo "CSS対応表に古いv0.6.0基準が残っています" >&2
    exit 1
fi

echo "v0.7.0文書同期検証成功: README, SECURITY.md, CSS対応表, WPT文書"
