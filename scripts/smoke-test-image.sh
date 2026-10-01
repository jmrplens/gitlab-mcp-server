#!/usr/bin/env bash
# smoke-test-image.sh — start the container image on every platform it claims
# to support and check it prints the expected version, and that it carries the
# licence and the third-party notices generated for its own binary.
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
#
# The licence and THIRD_PARTY_NOTICES sit in /usr/share/licenses/gitlab-mcp-server.
# The licence must be this repository's, and the notices, which the image's
# builder generates from the binary it just built, must name the platform the
# image was built for: notices left over from another build, or a step that
# stopped writing them, would otherwise ship without anything noticing.
set -euo pipefail

VERSION="${1:?Usage: $0 <expected-version> <image>=<platform> [...]}"
shift
if [ "$#" -eq 0 ]; then
  echo "ERROR: name at least one <image>=<platform> pair" >&2
  exit 1
fi

LICENSES_DIR=/usr/share/licenses/gitlab-mcp-server
REPO_LICENSE="$(cd "$(dirname "$0")/.." && pwd)/LICENSE"
NOTICES_HEADER="Third-party notices for gitlab-mcp-server"

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

  if ! licence=$(docker run --rm --platform "$platform" --entrypoint /bin/cat "$image" "$LICENSES_DIR/LICENSE" 2>&1); then
    echo "FAIL: ${image} (${platform}) carries no ${LICENSES_DIR}/LICENSE:" >&2
    printf '%s\n' "$licence" >&2
    failures=$((failures + 1))
    continue
  fi
  if [ "$licence" != "$(cat "$REPO_LICENSE")" ]; then
    echo "FAIL: ${image} (${platform}) carries a ${LICENSES_DIR}/LICENSE that is not this repository's" >&2
    failures=$((failures + 1))
    continue
  fi

  if ! notices=$(docker run --rm --platform "$platform" --entrypoint /bin/cat "$image" "$LICENSES_DIR/THIRD_PARTY_NOTICES" 2>&1); then
    echo "FAIL: ${image} (${platform}) carries no ${LICENSES_DIR}/THIRD_PARTY_NOTICES:" >&2
    printf '%s\n' "$notices" >&2
    failures=$((failures + 1))
    continue
  fi
  # Here-strings rather than pipes: the notices run to hundreds of kilobytes,
  # and a printf into a grep -q or a head that stops reading early dies of
  # SIGPIPE, which pipefail would report as the check failing.
  if [ "$(head -n 1 <<< "$notices")" != "$NOTICES_HEADER" ] ||
    ! grep -qxF "Builds:    ${platform}" <<< "$notices"; then
    echo "FAIL: ${image} (${platform}) carries THIRD_PARTY_NOTICES that are not the generator's for ${platform}:" >&2
    head -n 5 <<< "$notices" >&2
    failures=$((failures + 1))
    continue
  fi
  echo "    licence and third-party notices for ${platform} present"
done

if [ "$failures" -ne 0 ]; then
  echo "${failures} platform(s) failed the smoke test" >&2
  exit 1
fi
echo "All platforms started, reported version ${VERSION} and carry their licence and notices"
