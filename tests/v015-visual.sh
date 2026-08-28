#!/usr/bin/env bash
set -euo pipefail

go test ./examples/modern-web-compat -run TestFrameworkFixtureVisualRegression -count=1
go test ./internal/ui -run TestDevToolsPanelsVisualRegression -count=1

echo "v0.15.0 Visual Regression成功: SSR, hydration, interaction, responsive, fallback, DevTools"
