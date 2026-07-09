# syntax=docker/dockerfile:1
# Multi-stage build for the Kapibara control-plane: build the React UI, embed it
# into the Go binary, and ship a minimal runtime image.

# 1) Build the web UI → emits ../pkg/webui/dist (the go:embed source).
FROM node:22-alpine AS ui
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2) Compile the Go binary with the freshly built UI embedded.
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/pkg/webui/dist ./pkg/webui/dist
ARG VERSION=docker
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/orcinustools/kapibara/pkg/version.Version=${VERSION} -X github.com/orcinustools/kapibara/pkg/version.GitCommit=${COMMIT}" \
    -o /kapibara ./cmd/kapibara

# 3) Fetch the railpack binary (used to generate build plans for in-cluster,
#    Docker-less Git builds). Keep RAILPACK_VERSION in sync with the frontend
#    image referenced by KAPIBARA_RAILPACK_FRONTEND.
FROM alpine:3.20 AS tools
ARG TARGETARCH=amd64
ARG RAILPACK_VERSION=0.30.0
RUN apk add --no-cache curl tar
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) RARCH=x86_64-unknown-linux-musl ;; \
      arm64) RARCH=arm64-unknown-linux-musl ;; \
      *)     RARCH=x86_64-unknown-linux-musl ;; \
    esac; \
    curl -sSL "https://github.com/railwayapp/railpack/releases/download/v${RAILPACK_VERSION}/railpack-v${RAILPACK_VERSION}-${RARCH}.tar.gz" -o /tmp/rp.tgz; \
    tar -xzf /tmp/rp.tgz -C /usr/local/bin railpack; \
    chmod +x /usr/local/bin/railpack; \
    /usr/local/bin/railpack --version

# 4) Minimal runtime. Bundles git (clone), railpack (plan) and buildctl (build)
#    so the control-plane can build from Git in-cluster without a Docker daemon.
#    Uses a glibc base (not alpine/musl): railpack fetches a glibc `mise` at
#    build time, which fails to exec under musl.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 10001 -m -s /usr/sbin/nologin kapibara
COPY --from=build /kapibara /usr/local/bin/kapibara
COPY --from=tools /usr/local/bin/railpack /usr/local/bin/railpack
COPY --from=moby/buildkit:latest /usr/bin/buildctl /usr/local/bin/buildctl
# railpack/mise write their cache under $HOME and /tmp at build time.
ENV HOME=/tmp
USER kapibara
EXPOSE 9000
ENTRYPOINT ["kapibara"]
CMD ["serve"]
