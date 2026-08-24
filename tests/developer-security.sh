#!/usr/bin/env bash
set -euo pipefail

workflow=.github/workflows/unicode-security.yml
release_workflow=.github/workflows/release.yml
policy=.security/security-policy.json
documentation=docs/developer-security.md

require_file_value() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にDeveloper Supply Chain Securityの必須設定がありません: ${value}" >&2
        exit 1
    fi
}

for value in \
    "pull_request:" \
    "permissions:" \
    "contents: read" \
    "Checkout trusted scanner" \
    "Build trusted scanner" \
    "Checkout candidate without credentials" \
    "persist-credentials: false" \
    "Scan complete candidate tree with trusted policy" \
    "Scan pull request diff with trusted policy" \
    'github.event.pull_request.base.sha' \
    'github.event.pull_request.head.sha' \
    'name: unicode-security'
do
    require_file_value "$workflow" "$value"
done

if grep -Eq 'pull_request_target:|secrets\.|(contents|actions|checks|pull-requests|issues|packages): write' "$workflow"; then
    echo "trusted scanner workflowはSecretや書き込み権限を使用できません" >&2
    exit 1
fi

for value in \
    "Revalidate release source" \
    "go run ./cmd/securityscan" \
    "Revalidate Go dependencies" \
    "Release cool-down" \
    "environment: release" \
    'echo "Release commit: $GITHUB_SHA"' \
    "cat dist/*.sha256"
do
    require_file_value "$release_workflow" "$value"
done

for value in \
    '"binary_allowlist"' \
    '"sha256"' \
    '"unicode_allowlist"' \
    '"editor_extensions"' \
    '"publisher"' \
    '"version"'
do
    require_file_value "$policy" "$value"
done

for value in \
    "commit署名" \
    "tag署名" \
    "code --list-extensions --show-versions" \
    "Networkから隔離" \
    "Organization Audit log" \
    "gh attestation verify" \
    "同じPR"
do
    require_file_value "$documentation" "$value"
done

for value in \
    "Immutable Releases is enabled" \
    "Organization requires 2FA" \
    "administrator bypass is disabled" \
    "unicode-security is a required main check" \
    "fine-grained PAT maximum lifetime to 90 days"
do
    require_file_value scripts/audit-github-security.sh "$value"
done

go run ./cmd/securityscan

echo "Developer Supply Chain Security検証成功: trusted scanner, release gate, policy, runbook"
