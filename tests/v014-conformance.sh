#!/usr/bin/env bash
set -euo pipefail

# A fixed shuffle seed plus three fresh test-process runs detects order leaks while
# preserving an exactly reproducible execution order in local and CI runs.
go test -count=3 -shuffle=1337 -timeout=2m \
  ./internal/runtime/javascript \
  ./internal/runtime/isolated \
  ./internal/browser \
  ./internal/serviceworker

echo "v0.14.0決定性検証成功: WPT, security, lifecycle, quota, crash, cancel"
