#!/usr/bin/env bash
set -euo pipefail

release_workflow=.github/workflows/release.yml
dependency_workflow=.github/workflows/dependency-review.yml
dependabot=.github/dependabot.yml

require_file_value() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にSupply Chain Securityの必須設定がありません: ${value}" >&2
        exit 1
    fi
}

for value in \
    "permissions:" \
    "contents: read" \
    "actions/dependency-review-action@a1d282b36b6f3519aa1f3fc636f609c47dddb294" \
    "fail-on-severity: high"
do
    require_file_value "$dependency_workflow" "$value"
done

if grep -Eq 'pull_request_target|secrets\.|(contents|pull-requests|issues|packages): write' "$dependency_workflow"; then
    echo "Dependency Reviewはfork PRでSecretや書き込み権限を使用できません" >&2
    exit 1
fi

require_file_value "$dependabot" "package-ecosystem: gomod"
require_file_value "$dependabot" "package-ecosystem: github-actions"
require_file_value "$dependabot" "package-ecosystem: docker"

if [[ $(grep -Fc "id-token: write" "$release_workflow") -ne 2 ]]; then
    echo "id-token: writeはArchive/Containerのattestation jobだけに設定してください" >&2
    exit 1
fi

for value in \
    "name: Attest archive provenance" \
    "name: Attest SBOM provenance" \
    "name: Attest archive SBOM" \
    'subject-digest: ${{ needs.docker.outputs.digest }}' \
    'IMAGE_NAME: ${{ needs.docker.outputs.image }}' \
    'IMAGE_DIGEST: ${{ needs.docker.outputs.digest }}' \
    'gh attestation verify "oci://$IMAGE_NAME@$IMAGE_DIGEST"' \
    "Verify OCI SBOM and provenance by digest" \
    "Scan published image by digest"
do
    require_file_value "$release_workflow" "$value"
done

scan_line=$(grep -n "Scan published image by digest" "$release_workflow" | cut -d: -f1)
promote_line=$(grep -n "Promote verified image tags by digest" "$release_workflow" | cut -d: -f1)
if [[ -z "$scan_line" || -z "$promote_line" || "$scan_line" -ge "$promote_line" ]]; then
    echo "Release tagはHigh/Critical scanに成功したdigestだけへ付与してください" >&2
    exit 1
fi

if grep -Eq 'continue-on-error:[[:space:]]*true|only-fixed:[[:space:]]*true|ignore-unfixed|\.grype\.yaml' "$release_workflow"; then
    echo "High/Critical vulnerabilityの恒久的な除外設定は禁止です" >&2
    exit 1
fi

while read -r _ image _; do
    if [[ ! "$image" =~ @sha256:[0-9a-f]{64}$ ]]; then
        echo "Docker base imageはdigestで固定してください: $image" >&2
        exit 1
    fi
done < <(grep '^FROM ' Dockerfile)

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
valid_sbom="$tmp_dir/valid.spdx.json"
invalid_sbom="$tmp_dir/invalid.spdx.json"
printf '%s\n' '{"spdxVersion":"SPDX-2.3","packages":[{"name":"github.com/Grove-Computing/Growse"}]}' > "$valid_sbom"
printf '%s\n' '{"spdxVersion":"SPDX-2.3","packages":[]}' > "$invalid_sbom"

scripts/validate-sbom.sh "$valid_sbom"
if scripts/validate-sbom.sh "$invalid_sbom" >/dev/null 2>&1; then
    echo "Growse Go moduleを含まないSBOMを受理しました" >&2
    exit 1
fi

for value in \
    "gh attestation verify" \
    "docker buildx imagetools inspect" \
    "Supply Chain Securityの例外"
do
    require_file_value SECURITY.md "$value"
done

echo "Supply Chain Security検証成功: dependency review, SBOM, provenance, permissions"
