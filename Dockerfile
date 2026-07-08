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

# 3) Minimal runtime.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 kapibara
COPY --from=build /kapibara /usr/local/bin/kapibara
USER kapibara
EXPOSE 9000
ENTRYPOINT ["kapibara"]
CMD ["serve"]
