#!/usr/bin/env bash
# fetch-release-assets.sh — download published release assets and verify them
# before anything repackages them.
#
# Usage:
#   scripts/fetch-release-assets.sh [--checksums-only] <version> <dest-dir> [asset-pattern ...]
#
#   --checksums-only   fetch and verify only checksums.txt and its signature
#   <version>          release version without the leading v (e.g. 2.7.6)
#   <dest-dir>         directory the assets are downloaded into (created)
#   [asset-pattern]    gh release download --pattern globs; default: every
#                      gitlab-mcp-server-* binary
#
# Requires GH_TOKEN with read access to the repository's releases, and cosign
# on PATH. REPO defaults to $GITHUB_REPOSITORY.
#
# REHEARSAL_ARCHIVE, when set, names a tar of the GoReleaser job's own build
# and replaces the download: a release rehearsal has no release to fetch and
# signs nothing, so the archive is unpacked into <dest-dir> and checked against
# the checksums.txt it carries. That is the same integrity check a real
# release gets, minus the signature a rehearsal deliberately does not mint.
#
# Why this exists. The npm and PyPI distributions used to be assembled inside
# the same job that produced the binaries, from a plain `cp` of GoReleaser's
# output, with the signed checksums.txt sitting unread beside them. Anything
# that perturbed dist/ between GoReleaser and the packagers — a stale file from
# a re-run, a worm in a transitive dependency of some other step — shipped to
# two immutable registries. Packaging now happens in a job that produces no
# artifacts of its own and has to fetch them, so "what we package" and "what we
# published" are the same bytes by construction.
#
# The honest limit: checksums.txt.sigstore.json is a keyless signature minted
# from this workflow's own OIDC identity, so code running inside a compromised
# release job could re-sign a tampered manifest under exactly the identity this
# check demands. It defeats staleness, accidental corruption and an
# opportunistic payload that does not know this repository; it does not defeat
# an adversary who already owns the release job. What defeats that is the
# post-release verification that runs outside the release window, and the
# separation of permissions across jobs that this script exists to enable.
set -euo pipefail

CHECKSUMS_ONLY=0
if [ "${1:-}" = "--checksums-only" ]; then
  CHECKSUMS_ONLY=1
  shift
fi

VERSION="${1:?Usage: $0 [--checksums-only] <version> <dest-dir> [asset-pattern ...]}"
DEST="${2:?Usage: $0 [--checksums-only] <version> <dest-dir> [asset-pattern ...]}"
shift 2
PATTERNS=("$@")
if [ ${#PATTERNS[@]} -eq 0 ] && [ "$CHECKSUMS_ONLY" -eq 0 ]; then
  PATTERNS=("gitlab-mcp-server-*")
fi

REPO="${REPO:-${GITHUB_REPOSITORY:-jmrplens/gitlab-mcp-server}}"
TAG="v${VERSION}"
OIDC_ISSUER="https://token.actions.githubusercontent.com"
SIGNER_IDENTITY="https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${TAG}"
ARCHIVE="${REHEARSAL_ARCHIVE:-}"

mkdir -p "$DEST"

if [ -n "$ARCHIVE" ]; then
  echo "Rehearsal: unpacking ${ARCHIVE} into ${DEST} in place of the ${TAG} release assets"
  if [ "$CHECKSUMS_ONLY" -eq 1 ]; then
    tar -xf "$ARCHIVE" -C "$DEST" checksums.txt
    echo "checksums.txt unpacked; no assets requested"
    exit 0
  fi
  tar -xf "$ARCHIVE" -C "$DEST"
else
  echo "Downloading ${TAG} assets from ${REPO} into ${DEST}"
  args=(--repo "$REPO" --dir "$DEST" --clobber)
  for pattern in "${PATTERNS[@]}"; do
    args+=(--pattern "$pattern")
  done
  # checksums.txt and its bundle are never optional: they are what makes the
  # rest verifiable.
  args+=(--pattern "checksums.txt" --pattern "checksums.txt.sigstore.json")
  gh release download "$TAG" "${args[@]}"

  for required in checksums.txt checksums.txt.sigstore.json; do
    if [ ! -f "$DEST/$required" ]; then
      echo "ERROR: ${TAG} published no $required — refusing to package unverifiable assets" >&2
      exit 1
    fi
  done

  echo "Verifying checksums.txt against its Sigstore bundle"
  cosign verify-blob \
    --bundle "$DEST/checksums.txt.sigstore.json" \
    --certificate-identity "$SIGNER_IDENTITY" \
    --certificate-oidc-issuer "$OIDC_ISSUER" \
    "$DEST/checksums.txt"

  if [ "$CHECKSUMS_ONLY" -eq 1 ]; then
    echo "checksums.txt verified; no assets requested"
    exit 0
  fi
fi

if [ ! -f "$DEST/checksums.txt" ]; then
  echo "ERROR: no checksums.txt in ${DEST} — nothing here can be verified" >&2
  exit 1
fi

# The exit status of --ignore-missing is not trusted either way. On an empty
# intersection it is 1 with "no file was verified", and the empty intersection
# is a normal case: the registry and manifest jobs fetch the bundle alone, and
# the bundle is deliberately absent from checksums.txt. Read as a failure, that
# status stopped both jobs before the bundle's own verification below could
# run, which the first rehearsal to reach them found. So a FAILED line fails
# here, and what was actually checked is counted, with the bundle joining the
# count on its own evidence.
echo "Verifying assets against checksums.txt"
(
  cd "$DEST"
  sha256sum --check --ignore-missing checksums.txt > sha256-check.log 2>&1 || true
  cat sha256-check.log
)
if grep -q ": FAILED" "$DEST/sha256-check.log"; then
  echo "ERROR: an asset does not match checksums.txt" >&2
  rm -f "$DEST/sha256-check.log"
  exit 1
fi
verified=$(grep -c ": OK$" "$DEST/sha256-check.log" || true)
rm -f "$DEST/sha256-check.log"
echo "Verified ${verified} asset(s) against checksums.txt"

# The .mcpb is built outside GoReleaser, so it is absent from checksums.txt and
# is the one asset a job can legitimately fetch on its own. Its integrity comes
# from the build-provenance attestation instead, and it counts towards the
# "something was actually verified" floor below. A rehearsal attests nothing,
# and the bundle it unpacked was built by a job of the same run.
if [ -f "$DEST/gitlab-mcp-server.mcpb" ]; then
  if [ -n "$ARCHIVE" ]; then
    echo "Rehearsal: the .mcpb came from this run's own build; its attestation is minted at release"
  else
    echo "Verifying the .mcpb build-provenance attestation"
    gh attestation verify "$DEST/gitlab-mcp-server.mcpb" --repo "$REPO"
  fi
  verified=$((verified + 1))
fi

if [ "$verified" -eq 0 ]; then
  echo "ERROR: nothing here could be verified — checksums.txt matched no file and no bundle was found" >&2
  exit 1
fi
