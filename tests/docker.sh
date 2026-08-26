#!/usr/bin/env bash
set -euo pipefail

require_file_value() {
    local file=$1
    local value=$2
    if ! grep -Fq -- "$value" "$file"; then
        echo "${file}にDocker releaseの必須設定がありません: ${value}" >&2
        exit 1
    fi
}

require_pinned_base() {
    local image=$1
    if ! grep -Eq "^FROM ${image}:[^@[:space:]]+@sha256:[0-9a-f]{64}([[:space:]]+AS[[:space:]]+[[:alnum:]_.-]+)?$" Dockerfile; then
        echo "Dockerfileの${image}ベースイメージがタグ付きSHA-256 digestで固定されていません" >&2
        exit 1
    fi
}

require_pinned_base golang
require_pinned_base ubuntu
require_file_value Dockerfile "go build -trimpath"
require_file_value Dockerfile "https://deb.debian.org"
require_file_value Dockerfile "https://archive.ubuntu.com"
require_file_value Dockerfile "COPY --from=build /out/growse /usr/local/bin/growse"
require_file_value Dockerfile "rm -rf /usr/bin/pebble /var/lib/pebble"
require_file_value Dockerfile "USER growse"
require_file_value Dockerfile 'ENTRYPOINT ["growse"]'
require_file_value .dockerignore ".git"
require_file_value .dockerignore "dist"

ubuntu_ca_line=$(grep -nF "apt-get install --no-install-recommends -y ca-certificates" Dockerfile | cut -d: -f1)
ubuntu_https_line=$(grep -nF "s|http://archive.ubuntu.com|https://archive.ubuntu.com|g" Dockerfile | cut -d: -f1)
if [[ -z "$ubuntu_ca_line" || -z "$ubuntu_https_line" || "$ubuntu_ca_line" -ge "$ubuntu_https_line" ]]; then
    echo "Ubuntu自身のCA bundleをHTTPS切替前に導入していません" >&2
    exit 1
fi

workflow=.github/workflows/release.yml
require_file_value "$workflow" "needs: build"
require_file_value "$workflow" "docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e"
require_file_value "$workflow" "docker/login-action@dbcb813823bdd20940b903addbd779551569679f"
require_file_value "$workflow" "docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a"
require_file_value "$workflow" "push: true"
require_file_value "$workflow" "sbom: true"
require_file_value "$workflow" "provenance: mode=max"
require_file_value "$workflow" 'org.opencontainers.image.version=${{ env.RELEASE_TAG }}'
require_file_value "$workflow" 'image: ${{ env.IMAGE_NAME }}@${{ steps.build.outputs.digest }}'
require_file_value "$workflow" "severity-cutoff: high"
require_file_value "$workflow" "fail-build: true"
require_file_value "$workflow" "Promote verified image tags by digest"
require_file_value "$workflow" 'BUILD_DIGEST: ${{ steps.build.outputs.digest }}'
require_file_value "$workflow" '"$IMAGE_NAME@$BUILD_DIGEST"'
require_file_value "$workflow" "push-to-registry: true"

container_attest_job=$(sed -n '/^  container-attest:/,/^  publish:/p' "$workflow")
for value in \
    "Log in to GitHub Container Registry for attestation" \
    "docker/login-action@dbcb813823bdd20940b903addbd779551569679f" \
    "registry: ghcr.io" \
    'username: ${{ github.actor }}' \
    'password: ${{ github.token }}'; do
    if [[ "$container_attest_job" != *"$value"* ]]; then
        echo "${workflow}のcontainer-attest jobに必須設定がありません: ${value}" >&2
        exit 1
    fi
done

ci_workflow=.github/workflows/ci.yml
require_file_value "$ci_workflow" "Docker package (v0.10.0)"
require_file_value "$ci_workflow" "docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e"
require_file_value "$ci_workflow" "docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a"
require_file_value "$ci_workflow" "load: true"
require_file_value "$ci_workflow" "push: false"
require_file_value "$ci_workflow" "growse:v0.10.0"
require_file_value "$ci_workflow" "Verify unused Pebble runtime is absent"
require_file_value "$ci_workflow" "anchore/scan-action@e1165082ffb1fe366ebaf02d8526e7c4989ea9d2"
require_file_value "$ci_workflow" "severity-cutoff: high"
require_file_value "$ci_workflow" "fail-build: true"

echo "Docker検証成功: PR build, pinned base, digest scan, SBOM, provenance"
