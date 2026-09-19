#!/usr/bin/env bash
set -euo pipefail

artifact_dir=${GROWSE_REAL_SITE_ARTIFACT_DIR:-${TMPDIR:-/tmp}/growse-v020-real-site-visual}
export GROWSE_REAL_SITE_ARTIFACT_DIR="$artifact_dir"

go test ./examples/real-site-compat -run 'TestRealSiteCorpusProducesGrowseReferenceDiffAndRegionArtifacts|TestDiffMetricDetectsStructuralChange' -count=1

for fixture in one-mb-club wikipedia-ja github-profile cv-btxx t0-vc schemescape kidlat; do
  for viewport in desktop narrow; do
    test -s "$artifact_dir/${fixture}-${viewport}-growse.png"
    test -s "$artifact_dir/${fixture}-${viewport}-reference.png"
    test -s "$artifact_dir/${fixture}-${viewport}-diff.png"
    test -s "$artifact_dir/${fixture}-${viewport}-metrics.json"
  done
done

echo "v0.20.0実サイトvisual evidence成功: 7 fixtures × desktop/narrow × Growse/reference/diff/metrics"
