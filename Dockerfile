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
RUN CGO_ENABLED=0 go build \
	-trimpath -buildmode=pie \
	-ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
	-o /out/gitlab-mcp-server ./cmd/server

# --- Runtime stage ---
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata && \
	addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /out/gitlab-mcp-server /usr/local/bin/gitlab-mcp-server

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]

ENTRYPOINT ["gitlab-mcp-server"]
CMD ["--http", "--http-addr", "0.0.0.0:8080"]
