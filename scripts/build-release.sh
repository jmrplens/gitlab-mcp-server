#!/usr/bin/env bash
# build-release.sh — Cross-compile release binaries for all platforms.
# Usage: ./scripts/build-release.sh
# Called by: make release (on Linux / macOS)

set -euo pipefail

BINARY_NAME="gitlab-mcp-server"
CMD_PATH="./cmd/server"
VERSION="$(tr -d '[:space:]' < VERSION)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"

LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}"
OUT_DIR="dist"

# Build targets: GOOS GOARCH extension
TARGETS=(
    "linux   amd64  "
    "linux   arm64  "
    "windows amd64  .exe"
    "windows arm64  .exe"
    "darwin  amd64  "
    "darwin  arm64  "
)

echo "=== Building release v${VERSION} (commit ${COMMIT}) ==="
echo "Output directory: ${OUT_DIR}"
echo ""

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

export CGO_ENABLED=0
failed=0
total=${#TARGETS[@]}

for target in "${TARGETS[@]}"; do
    # shellcheck disable=SC2086
    set -- $target
    goos="$1"
    goarch="$2"
    ext="${3:-}"

    out_file="${BINARY_NAME}-${goos}-${goarch}${ext}"
    out_path="${OUT_DIR}/${out_file}"

    printf "  Building %-45s" "${out_file} ..."
    if GOOS="${goos}" GOARCH="${goarch}" go build -ldflags="${LDFLAGS}" -o "${out_path}" "${CMD_PATH}" 2>&1; then
        size=$(du -h "${out_path}" | cut -f1)
        echo " OK (${size})"
    else
        echo " FAILED"
        failed=$((failed + 1))
    fi
done

# Generate SHA256 checksums
echo ""
echo "=== Generating checksums ==="
checksum_file="${OUT_DIR}/checksums.txt"

cd "${OUT_DIR}"
if command -v sha256sum &>/dev/null; then
    sha256sum gitlab-mcp-server-* > checksums.txt
elif command -v shasum &>/dev/null; then
    shasum -a 256 gitlab-mcp-server-* > checksums.txt
else
    echo "ERROR: Neither sha256sum nor shasum found" >&2
    exit 1
fi
cd - >/dev/null

echo "Checksums written to ${checksum_file}"

echo ""

# Summary
ok=$((total - failed))
echo "=== Release build complete ==="
echo "  Version : v${VERSION}"
echo "  Commit  : ${COMMIT}"
echo "  Binaries: ${ok}/${total} succeeded"
echo "  Output  : ${OUT_DIR}/"
echo ""

if [ "${failed}" -gt 0 ]; then
    echo "${failed} build(s) failed!" >&2
    exit 1
fi

cat "${checksum_file}"
