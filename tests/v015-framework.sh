#!/usr/bin/env bash
set -euo pipefail

go test \
  ./internal/css \
  ./internal/devtools \
  ./internal/dom \
  ./internal/layout \
  ./internal/paint \
  ./internal/style \
  ./internal/runtime/javascript \
  ./internal/browser

go test ./internal/integration
go test ./examples/modern-web-compat

echo "v0.15.0互換検証成功: Unit, Integration, Framework fixture"
