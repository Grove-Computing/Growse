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

require_file_value Dockerfile "FROM golang:1.26-bookworm AS build"
require_file_value Dockerfile "FROM ubuntu:24.04"
require_file_value Dockerfile "go build -trimpath"
require_file_value Dockerfile "COPY --from=build /out/growse /usr/local/bin/growse"
require_file_value Dockerfile 'ENTRYPOINT ["growse"]'
require_file_value .dockerignore ".git"
require_file_value .dockerignore "dist"

workflow=.github/workflows/release.yml
require_file_value "$workflow" "needs: build"
require_file_value "$workflow" 'org.opencontainers.image.version=$RELEASE_TAG'
require_file_value "$workflow" 'docker push "$IMAGE_NAME:$RELEASE_TAG"'
require_file_value "$workflow" 'docker push "$IMAGE_NAME:latest"'

echo "Docker release検証成功: multi-stage image, v0.6 release tag, latest tag"
