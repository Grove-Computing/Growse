#!/usr/bin/env bash
set -euo pipefail

go test \
  ./internal/css \
  ./internal/style \
  ./internal/layout \
  ./internal/paint \
  ./internal/browser \
  ./internal/ui

go test ./internal/integration
go test ./examples/modern-web-compat

echo "v0.16.0実サイト互換検証成功: Unit, Integration, Tailwind/SvelteKit fixture"
