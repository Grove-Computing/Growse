FROM golang:1.26-bookworm AS build

RUN apt-get update \
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

FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install --no-install-recommends -y \
        ca-certificates \
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
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/growse /usr/local/bin/growse

ENTRYPOINT ["growse"]
