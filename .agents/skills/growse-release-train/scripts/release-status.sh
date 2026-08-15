#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "usage: $0 vX.Y.Z" >&2
    exit 2
}

if [[ $# -ne 1 || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    usage
fi

tag=$1
release_branch="release/${tag}"

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
    echo "Git Repository内で実行してください" >&2
    exit 2
}
cd "$root"

scope_document="docs/${tag}.md"
if [[ ! -f "$scope_document" ]]; then
    echo "scope documentがありません: $scope_document" >&2
    exit 1
fi

current_branch=$(git branch --show-current)
dirty_files=$(git status --porcelain | wc -l | tr -d ' ')
total=$(grep -Ec '^- \[[ xX]\] ' "$scope_document" || true)
completed=$(grep -Ec '^- \[[xX]\] ' "$scope_document" || true)
remaining=$((total - completed))

local_release=no
if git show-ref --verify --quiet "refs/heads/${release_branch}"; then
    local_release=yes
fi

tracked_release=no
if git show-ref --verify --quiet "refs/remotes/origin/${release_branch}"; then
    tracked_release=yes
fi

echo "release_tag=${tag}"
echo "release_branch=${release_branch}"
echo "current_branch=${current_branch}"
echo "scope_document=${scope_document}"
echo "dirty_files=${dirty_files}"
echo "local_release_branch=${local_release}"
echo "origin_tracking_branch=${tracked_release}"
echo "checkbox_total=${total}"
echo "checkbox_completed=${completed}"
echo "checkbox_remaining=${remaining}"

echo "major_items:"
awk '
    /^## .*完了条件/ { in_completion = 1; next }
    in_completion && /^## / { exit }
    in_completion && /^### / { sub(/^### /, ""); print "- " $0 }
' "$scope_document"

if ((remaining > 0)); then
    echo "remaining_items:"
    grep -E '^- \[ \] ' "$scope_document" || true
fi
