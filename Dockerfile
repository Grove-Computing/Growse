FROM golang:1.27-bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466 AS build

RUN sed -i 's|http://deb.debian.org|https://deb.debian.org|g; s|http://security.debian.org|https://security.debian.org|g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update \
    && apt-get install --no-install-recommends -y \
        gcc \
        libegl1-mesa-dev \
        libffi-dev \
        libgles2-mesa-dev \
        libvulkan-dev \
        libwayland-dev \
        libx11-dev \
        libx11-xcb-dev \
        libxcursor-dev \
        libxkbcommon-x11-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/growse ./cmd/growse

FROM ubuntu:26.04@sha256:678c6550cc43645e08669028bc177f50be4e7c5b8cca677067b1914d4afc7a03

# Ubuntuのベースイメージに含まれるPebbleはGrowseでは使用しない。
# 不要なGoバイナリをランタイムへ残さず、脆弱性の影響範囲から外す。
RUN apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates \
    && sed -i 's|http://archive.ubuntu.com|https://archive.ubuntu.com|g; s|http://security.ubuntu.com|https://security.ubuntu.com|g' /etc/apt/sources.list.d/ubuntu.sources \
    && apt-get update \
    && apt-get install --no-install-recommends -y \
        libegl1 \
        libffi8 \
        libgles2 \
        libvulkan1 \
        libwayland-client0 \
        libwayland-cursor0 \
        libwayland-egl1 \
        libx11-6 \
        libx11-xcb1 \
        libxcursor1 \
        libxkbcommon-x11-0 \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /usr/bin/pebble /var/lib/pebble \
    && groupadd --system growse \
    && useradd --system --gid growse --create-home --home-dir /home/growse growse

COPY --from=build /out/growse /usr/local/bin/growse

USER growse
ENTRYPOINT ["growse"]
