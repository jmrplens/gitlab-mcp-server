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
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/gitlab-mcp-server ./cmd/server

# --- Runtime stage ---
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /out/gitlab-mcp-server /usr/local/bin/gitlab-mcp-server

USER appuser

EXPOSE 8080

ENTRYPOINT ["gitlab-mcp-server"]
CMD ["--http", "--http-addr", "0.0.0.0:8080"]
