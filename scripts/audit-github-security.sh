#!/usr/bin/env bash
set -euo pipefail

repository=${GITHUB_REPOSITORY:-Grove-Computing/Growse}
organization=${repository%%/*}
failures=0

pass() {
    printf 'PASS  %s\n' "$1"
}

fail() {
    printf 'FAIL  %s\n' "$1" >&2
    failures=$((failures + 1))
}

expect_equal() {
    local description=$1
    local actual=$2
    local expected=$3
    if [[ "$actual" == "$expected" ]]; then
        pass "$description"
    else
        fail "$description (actual: ${actual:-<empty>}, expected: $expected)"
    fi
}

immutable=$(gh api -H 'X-GitHub-Api-Version: 2026-03-10' "repos/$repository/immutable-releases" --jq .enabled 2>/dev/null || printf false)
expect_equal "Immutable Releases is enabled" "$immutable" true

two_factor=$(gh api "orgs/$organization" --jq .two_factor_requirement_enabled)
expect_equal "Organization requires 2FA" "$two_factor" true

wait_timer=$(gh api "repos/$repository/environments/release" --jq '.protection_rules[] | select(.type == "wait_timer") | .wait_timer' 2>/dev/null || true)
expect_equal "release Environment wait timer is 30 minutes" "$wait_timer" 30

admin_bypass=$(gh api "repos/$repository/environments/release" --jq .can_admins_bypass 2>/dev/null || true)
expect_equal "release Environment administrator bypass is disabled" "$admin_bypass" false

tag_policy=$(gh api "repos/$repository/environments/release/deployment-branch-policies" --jq '.branch_policies[] | select(.name == "v*" and .type == "tag") | .name' 2>/dev/null || true)
expect_equal "release Environment accepts only v* tags" "$tag_policy" 'v*'

main_ruleset_id=$(gh api "repos/$repository/rulesets" --jq '.[] | select(.name == "main protection" and .target == "branch") | .id')
if [[ -z "$main_ruleset_id" ]]; then
    fail "main protection Ruleset exists"
else
    main_bypass_count=$(gh api "repos/$repository/rulesets/$main_ruleset_id" --jq '.bypass_actors | length')
    expect_equal "main Ruleset has no bypass actor" "$main_bypass_count" 0
    unicode_check=$(gh api "repos/$repository/rulesets/$main_ruleset_id" --jq '[.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[].context] | any(. == "unicode-security")')
    expect_equal "unicode-security is a required main check" "$unicode_check" true
fi

tag_ruleset_id=$(gh api "repos/$repository/rulesets" --jq '.[] | select(.name == "signed immutable release tags" and .target == "tag") | .id')
if [[ -z "$tag_ruleset_id" ]]; then
    fail "signed immutable release tags Ruleset exists"
else
    tag_bypass_count=$(gh api "repos/$repository/rulesets/$tag_ruleset_id" --jq '.bypass_actors | length')
    expect_equal "release tag Ruleset has no bypass actor" "$tag_bypass_count" 0
fi

printf '\nMANUAL  Confirm passkey or hardware security key and offline recovery codes.\n'
printf 'MANUAL  Restrict classic PAT access and set fine-grained PAT maximum lifetime to 90 days.\n'
printf 'MANUAL  Review OAuth Apps, GitHub Apps, and the SSH key inventory in GitHub settings.\n'

if [[ "$failures" -ne 0 ]]; then
    printf '\nGitHub security audit failed with %d finding(s).\n' "$failures" >&2
    exit 1
fi

printf '\nGitHub security audit passed.\n'
