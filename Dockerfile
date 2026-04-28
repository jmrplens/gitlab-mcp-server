# syntax=docker/dockerfile:1@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769

# --- Build stage ---
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine3.23 AS builder

# hadolint ignore=DL3018
RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go mod download

COPY . .

ARG VERSION=""
ARG COMMIT=""
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-trimpath -buildmode=pie \
	-ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
	-o /out/gitlab-mcp-server ./cmd/server

# --- Runtime stage ---
FROM alpine:3.23.4

# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata && \
	addgroup -S -g 10001 appgroup && \
	adduser -S -u 10001 -G appgroup -h /home/appuser appuser

COPY --from=builder /out/gitlab-mcp-server /usr/local/bin/gitlab-mcp-server

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD ["wget", "-q", "--spider", "-O", "/dev/null", "http://localhost:8080/health"]

ARG VERSION=""
ARG COMMIT=""
ARG BUILD_DATE=""
LABEL org.opencontainers.image.title="gitlab-mcp-server" \
	org.opencontainers.image.description="MCP server exposing GitLab REST API v4 and GraphQL operations as Model Context Protocol tools" \
	org.opencontainers.image.source="https://github.com/jmrplens/gitlab-mcp-server" \
	org.opencontainers.image.documentation="https://github.com/jmrplens/gitlab-mcp-server/tree/main/docs" \
	org.opencontainers.image.url="https://github.com/jmrplens/gitlab-mcp-server" \
	org.opencontainers.image.version="${VERSION}" \
	org.opencontainers.image.revision="${COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.licenses="MIT" \
	org.opencontainers.image.authors="jmrplens" \
	org.opencontainers.image.vendor="jmrplens" \
	io.modelcontextprotocol.server.name="io.github.jmrplens/gitlab-mcp-server"

ENTRYPOINT ["gitlab-mcp-server"]
CMD ["--http", "--http-addr", "0.0.0.0:8080"]
