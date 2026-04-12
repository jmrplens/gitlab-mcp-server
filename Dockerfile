# syntax=docker/dockerfile:1@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769

# --- Build stage ---
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=""
ARG COMMIT=""
# GITHUB_UPDATE_TOKEN is passed via BuildKit secret mount to avoid leaking in image layers.
# Build with: docker build --secret id=update_token,env=GITHUB_UPDATE_TOKEN ...
RUN --mount=type=secret,id=update_token \
    GITHUB_UPDATE_TOKEN="$(cat /run/secrets/update_token 2>/dev/null || echo '')" && \
    CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.defaultAutoUpdateToken=${GITHUB_UPDATE_TOKEN}" \
    -o /out/gitlab-mcp-server ./cmd/server

# --- Runtime stage ---
FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /out/gitlab-mcp-server /usr/local/bin/gitlab-mcp-server

USER appuser

EXPOSE 8080

ENTRYPOINT ["gitlab-mcp-server"]
CMD ["--http", "--http-addr", "0.0.0.0:8080"]
