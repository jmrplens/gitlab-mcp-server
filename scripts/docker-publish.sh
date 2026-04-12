#!/usr/bin/env bash
# docker-publish.sh — Build and push Docker image to GitHub Container Registry.
# Usage: ./scripts/docker-publish.sh [registry_url]
#
# Arguments:
#   registry_url  (optional) Full registry URL. Default: reads from DOCKER_REGISTRY
#                 env var or uses the GitLab project path from VERSION + go.mod.
#
# Environment variables:
#   DOCKER_REGISTRY   Override the container registry URL
#   GITHUB_USER       GitHub username for registry login (or CI_REGISTRY_USER in CI)
#   GITHUB_TOKEN      GitHub token for registry login (or CI_REGISTRY_PASSWORD in CI)

set -euo pipefail

# Load .env if present (supports both project root and scripts/ invocation)
ENV_FILE="${BASH_SOURCE[0]%/*}/../.env"
if [ -f "${ENV_FILE}" ]; then
    set -a
    # shellcheck source=/dev/null
    . "${ENV_FILE}"
    set +a
fi

VERSION="$(tr -d '[:space:]' < VERSION)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
IMAGE_NAME="gitlab-mcp-server"

# Determine registry URL
if [ -n "${1:-}" ]; then
    REGISTRY="$1"
elif [ -n "${DOCKER_REGISTRY:-}" ]; then
    REGISTRY="${DOCKER_REGISTRY}"
else
    echo "Error: No registry URL provided."
    echo "Usage: $0 <registry_url>"
    echo "   or: DOCKER_REGISTRY=registry.example.com/group/project $0"
    exit 1
fi

echo "=== Docker Publish v${VERSION} (commit ${COMMIT}) ==="
echo "Registry: ${REGISTRY}"
echo ""

# Login to registry
REGISTRY_HOST="${REGISTRY%%/*}"
USER="${GITHUB_USER:-${CI_REGISTRY_USER:-}}"
PASS="${GITHUB_TOKEN:-${CI_REGISTRY_PASSWORD:-}}"

if [ -n "${USER}" ] && [ -n "${PASS}" ]; then
    echo "Logging in to ${REGISTRY_HOST}..."
    echo "${PASS}" | docker login "${REGISTRY_HOST}" -u "${USER}" --password-stdin
    echo ""
fi

# Build image
echo "Building image..."
DOCKER_BUILDKIT=1 docker build \
    --build-arg VERSION="${VERSION}" \
    --build-arg COMMIT="${COMMIT}" \
    --secret id=update_token,env=GITHUB_UPDATE_TOKEN \
    -t "${REGISTRY}:${VERSION}" \
    -t "${REGISTRY}:latest" \
    .

echo ""

# Push images
echo "Pushing ${REGISTRY}:${VERSION}..."
docker push "${REGISTRY}:${VERSION}"

echo "Pushing ${REGISTRY}:latest..."
docker push "${REGISTRY}:latest"

echo ""
echo "=== Done ==="
echo "  ${REGISTRY}:${VERSION}"
echo "  ${REGISTRY}:latest"
