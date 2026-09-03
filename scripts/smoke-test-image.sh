#!/usr/bin/env bash
# smoke-test-image.sh — start the container image on every platform it claims
# to support and check it prints the expected version.
#
# Usage:
#   scripts/smoke-test-image.sh <expected-version> <image>=<platform> [...]
#
# Example:
#   scripts/smoke-test-image.sh 2.7.6 \
#     gitlab-mcp-server:smoke-amd64=linux/amd64 \
#     gitlab-mcp-server:smoke-arm64=linux/arm64
#
# Non-native platforms need QEMU binfmt registered (docker/setup-qemu-action in
# CI, `docker run --privileged tonistiigi/binfmt --install all` locally).
#
# Why: the published linux/arm64 image of 2.7.5 could not exec at all. Go picks
# a position-independent executable's ELF interpreter by stat-ing the *build*
# host, so the cross-compiled binary asked for /lib/ld-linux-aarch64.so.1 while
# the Alpine runtime ships only /lib/ld-musl-aarch64.so.1. Every gate in the
# pipeline passed: the e2e suite runs in-process, CI built only the runner's
# native platform, and the release built both and started neither. An image
# nothing has ever executed on the platform it advertises is not a tested
# artefact, and no static Dockerfile check would have caught it — only running
# the thing does.
set -euo pipefail

VERSION="${1:?Usage: $0 <expected-version> <image>=<platform> [...]}"
shift
if [ "$#" -eq 0 ]; then
  echo "ERROR: name at least one <image>=<platform> pair" >&2
  exit 1
fi

failures=0
for pair in "$@"; do
  image="${pair%%=*}"
  platform="${pair#*=}"
  if [ "$image" = "$pair" ] || [ -z "$platform" ]; then
    echo "ERROR: '$pair' is not <image>=<platform>" >&2
    exit 1
  fi

  echo "==> ${image} on ${platform}"
  if ! output=$(docker run --rm --platform "$platform" "$image" --version 2>&1); then
    echo "FAIL: ${image} (${platform}) did not start:" >&2
    printf '%s\n' "$output" >&2
    failures=$((failures + 1))
    continue
  fi

  if ! printf '%s' "$output" | grep -q "^gitlab-mcp-server ${VERSION}\b"; then
    echo "FAIL: ${image} (${platform}) started but printed:" >&2
    printf '%s\n' "$output" >&2
    echo "      expected a line beginning 'gitlab-mcp-server ${VERSION}'" >&2
    failures=$((failures + 1))
    continue
  fi

  printf '    %s\n' "$output"
done

if [ "$failures" -ne 0 ]; then
  echo "${failures} platform(s) failed the smoke test" >&2
  exit 1
fi
echo "All platforms started and reported version ${VERSION}"
