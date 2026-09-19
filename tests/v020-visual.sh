#!/usr/bin/env bash
set -euo pipefail

artifact_dir="${GROWSE_VISUAL_ARTIFACT_DIR:-}"
if [[ -n "$artifact_dir" ]]; then
  mkdir -p "$artifact_dir"
fi

go test ./internal/paint -run 'Test(DashboardVisualRegression|V020TextVisualEvidence)$' -count=1
go test ./internal/ui -run '^TestTextShaperInitialization$' -count=1

if [[ -n "$artifact_dir" ]]; then
  test -s "$artifact_dir/dashboard.png"
  test -s "$artifact_dir/text-visual-evidence.png"
fi

echo "v0.20.0 Visual Evidence成功: raster PNG, text region, shaper initialization"
