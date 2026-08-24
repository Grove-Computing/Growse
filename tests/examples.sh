#!/usr/bin/env bash
set -euo pipefail

examples=(
  animation
  counter
  css3-core
  dashboard
  data-app
  devtools
  flexbox
  persistent-app
  todo
)

for example in "${examples[@]}"; do
  directory="examples/${example}"
  if [[ ! -s "${directory}/index.html" || ! -s "${directory}/style.css" ]]; then
    echo "v0.8.0 Demoの必須Assetがありません: ${directory}" >&2
    exit 1
  fi
done

go test \
  ./examples/animation \
  ./examples/counter \
  ./examples/css3-core \
  ./examples/dashboard \
  ./examples/data-app \
  ./examples/devtools \
  ./examples/flexbox \
  ./examples/persistent-app \
  ./examples/todo

echo "v0.12.0 Demo回帰検証成功: ${examples[*]}"
