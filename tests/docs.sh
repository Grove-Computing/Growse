#!/usr/bin/env bash
set -euo pipefail

require() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にv0.6.0文書の必須記述がありません: ${value}" >&2
        exit 1
    fi
}

require README.md "examples/dashboard"
require README.md "GROWSE_VERSION=v0.6.0"
require README.md "growse:v0.6.0"
require SECURITY.md "| 0.6.x | Yes |"
require SECURITY.md "最大深度8"
require docs/css-support.md "Growse v0.6.0"
require docs/css-support.md '`grid`、`inline-grid`'
require docs/css-support.md '`transform`、`transform-origin`'
require docs/css-support.md "Subgrid、masonry"

if grep -Fq "Growse v0.5.0の実装" docs/css-support.md; then
    echo "CSS対応表に古いv0.5.0基準が残っています" >&2
    exit 1
fi

echo "v0.6.0文書同期検証成功: README, SECURITY.md, CSS対応表"
