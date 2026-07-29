#!/usr/bin/env bash
# Sign a container image with cosign, retrying transient OIDC failures.
#
# Keyless signing resolves its identity token from the ambient GitHub OIDC
# endpoint. That endpoint intermittently answers with a plain-text error body
# where cosign expects JSON, which surfaces as:
#
#   fetching ambient OIDC credentials: invalid character 'u' looking for
#   beginning of value
#
# The image itself is already pushed by then, so failing the step leaves a
# published but unsigned image and fails the release. The condition clears on
# its own, so retry before giving up.
#
# Usage: cosign-sign-retry.sh <image-ref-with-digest>

set -euo pipefail

IMAGE_REF="${1:?Usage: $0 <image-ref-with-digest>}"
ATTEMPTS="${COSIGN_SIGN_ATTEMPTS:-3}"

for attempt in $(seq 1 "$ATTEMPTS"); do
  if cosign sign --yes "$IMAGE_REF"; then
    exit 0
  fi

  if [[ "$attempt" -eq "$ATTEMPTS" ]]; then
    echo "cosign sign failed for ${IMAGE_REF} after ${ATTEMPTS} attempts" >&2
    exit 1
  fi

  delay=$((attempt * 10))
  echo "cosign sign failed for ${IMAGE_REF} (attempt ${attempt}/${ATTEMPTS}); retrying in ${delay}s" >&2
  sleep "$delay"
done
