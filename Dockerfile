# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

# --- Build stage ---
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

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
# -buildmode=pie makes the binary dynamically linked, and Go picks its
# PT_INTERP by stat-ing the *build* host: it uses the glibc path unless the
# musl loader for that architecture exists locally (cmd/link/internal/ld/elf.go).
# The builder runs on $BUILDPLATFORM, so the native architecture got Alpine's
# musl loader and every cross-compiled one got a glibc loader the runtime image
# does not ship — the published linux/arm64 image died at exec with
# "Could not open '/lib/ld-linux-aarch64.so.1'". Name the interpreter
# explicitly instead of letting the linker guess from the wrong filesystem, and
# refuse to build an architecture whose musl name we have not spelled out.
#
# No -trimpath here, unlike the GoReleaser build. With it Go leaves -ldflags
# out of the binary's build information, and .git is not in the build context,
# so the main module's version is (devel) and nothing in the binary says which
# release it is: the image SBOM listed the server with no version, which no
# advisory against this module could ever be matched to. Without it the build
# information records -X main.version, which syft (the version release.yml
# pins) reads as the module's version and writes into a versioned purl. What
# it costs is the paths of this stage, /src for our packages, /go/pkg/mod for
# the dependencies and /usr/local/go for the standard library, none of which
# says anything about the host that ran the build.
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	set -eu; \
	case "${TARGETARCH}" in \
	amd64) MUSL_ARCH=x86_64 ;; \
	arm64) MUSL_ARCH=aarch64 ;; \
	*) echo "unsupported TARGETARCH=${TARGETARCH}: no musl loader name for it" >&2; exit 1 ;; \
	esac; \
	CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-buildmode=pie \
	-ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -I /lib/ld-musl-${MUSL_ARCH}.so.1" \
	-o /out/gitlab-mcp-server ./cmd/server; \
	grep -a -q "/lib/ld-musl-${MUSL_ARCH}.so.1" /out/gitlab-mcp-server || \
	{ echo "built binary does not request the musl loader for ${TARGETARCH}" >&2; exit 1; }

# --- Runtime stage ---
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata && \
	addgroup -S -g 10001 appgroup && \
	adduser -S -u 10001 -G appgroup -h /home/appuser appuser

COPY --from=builder /out/gitlab-mcp-server /usr/local/bin/gitlab-mcp-server

USER appuser

EXPOSE 8080

# --probe reads the listener off the running server's own flags, so the check
# follows --http-addr to another port or a unix socket and speaks TLS when
# --tls-cert is set. A wget of http://localhost:8080/health was right for the
# default command only and marked every other listener unhealthy while it
# served. An instance running stdio (a client's `docker run -i`) has nothing to
# probe and is reported healthy while it runs.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD ["gitlab-mcp-server", "--probe"]

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

# --transport auto, not --http, so one image serves both uses without the
# caller overriding the command.
#
# Any argument after the image name replaces CMD wholesale, so while this said
# --http, an MCP client running `docker run -i <image>` got an HTTP listener
# and waited at initialize forever. The documented cure was --http=false in
# every client configuration, a flag that had been copied into three dozen
# files to work around this line and is in none of them now.
#
# auto reads the transport off file descriptor 0: `docker run` without -i, and
# Compose without stdin_open, connect /dev/null, which nobody speaks JSON-RPC
# down, so that means HTTP. `docker run -i` connects a pipe, which is a client.
# --http and --http=false both still work and still win, for a deployment that
# would rather say it outright.
CMD ["--transport", "auto", "--http-addr", "0.0.0.0:8080"]
