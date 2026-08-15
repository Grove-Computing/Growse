#!/usr/bin/env bash
set -euo pipefail

skill_root=".agents/skills/growse-release-train"
skill_file="${skill_root}/SKILL.md"
metadata_file="${skill_root}/agents/openai.yaml"
template_file="${skill_root}/assets/release-scope-template.md"
status_script="${skill_root}/scripts/release-status.sh"

for file in "$skill_file" "$metadata_file" "$template_file" "$status_script"; do
    if [[ ! -s "$file" ]]; then
        echo "Growse Release Train Skillの必須Fileがありません: $file" >&2
        exit 1
    fi
done

require() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にSkillの必須設定がありません: ${value}" >&2
        exit 1
    fi
}

require "$skill_file" "name: growse-release-train"
require "$skill_file" "release/vX.Y.Z"
require "$skill_file" 'scripts/release-status.sh vX.Y.Z'
require "$skill_file" '全大項目PRのbaseを必ず`release/vX.Y.Z`にする'
require "$skill_file" 'Testが成功した後だけ、対象checkboxを`[x]`へ更新する'
require "$metadata_file" 'display_name: "Growse Release Train"'
require "$metadata_file" 'Use $growse-release-train'
require "$template_file" "## 11. 完了条件"

if grep -RqE 'TODO|\[TODO' "$skill_root"; then
    echo "Growse Release Train Skillに未解決のTODOがあります" >&2
    exit 1
fi

if [[ ! -x "$status_script" ]]; then
    echo "release-status.shに実行権限がありません" >&2
    exit 1
fi
bash -n "$status_script"

status=$($status_script v0.9.0)
for value in \
    "release_branch=release/v0.9.0" \
    "checkbox_total=54" \
    "checkbox_completed=54" \
    "checkbox_remaining=0" \
    "- WebGo Scheduler" \
    "- 品質とリリース"
do
    if ! grep -Fq -- "$value" <<<"$status"; then
        echo "release-status.shの出力に必須値がありません: $value" >&2
        exit 1
    fi
done

if "$status_script" 0.9.0 >/dev/null 2>&1; then
    echo "release-status.shがv prefixのないversionを受理しました" >&2
    exit 1
fi

echo "Growse Release Train Skill検証成功: metadata, scope template, release status"
