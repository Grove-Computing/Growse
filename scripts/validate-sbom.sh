#!/usr/bin/env bash
set -euo pipefail

sbom=${1:-}

if [[ -z "$sbom" || ! -s "$sbom" ]]; then
    echo "SBOM fileが見つからないか空です: ${sbom:-<未指定>}" >&2
    exit 1
fi

if [[ "$sbom" != /* ]]; then
    sbom="$(pwd)/$sbom"
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"
go run ./cmd/sbomcheck "$sbom"
