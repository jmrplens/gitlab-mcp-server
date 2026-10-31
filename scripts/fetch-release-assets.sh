#!/usr/bin/env bash
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

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
#                      gitlab-mcp-server-<os>-* binary and its SBOM, and
#                      THIRD_PARTY_NOTICES, which every package carries beside
#                      the binaries and which the default therefore requires;
#                      it leaves out the Claude Desktop bundles
#                      (gitlab-mcp-server-<os>.mcpb and gitlab-mcp-server.mcpb)
#
# Every fetch but --checksums-only also takes gitlab-mcp-server.intoto.jsonl,
# the release's SLSA provenance (one Sigstore bundle per line), which every
# release after 3.4.0 publishes and the bundles are verified against.
#
# Requires GH_TOKEN with read access to the repository's releases, and cosign
# on PATH. REPO defaults to $GITHUB_REPOSITORY.
#
# REHEARSAL_ARCHIVE, when set, names a tar of the GoReleaser job's own build
# and replaces the download: a release rehearsal has no release to fetch and
# signs nothing, so the archive is unpacked into <dest-dir> and checked against
# the checksums.txt it carries, and each bundle against the unsigned
# provenance statements it carries (jq on PATH). That is the same integrity
# check a real release gets, minus the signatures a rehearsal deliberately
# does not mint.
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
# The license, notice and patent texts of what the binaries link, generated
# at release time (cmd/gen_third_party_notices) and listed in checksums.txt
# like every binary.
NOTICES="THIRD_PARTY_NOTICES"
REQUIRE_NOTICES=0
if [ ${#PATTERNS[@]} -eq 0 ] && [ "$CHECKSUMS_ONLY" -eq 0 ]; then
  # One glob per operating system rather than gitlab-mcp-server-*, which also
  # matches the per-OS Claude Desktop bundles (gitlab-mcp-server-linux.mcpb):
  # the jobs that take the default package the binaries and have no use for
  # some 75 MB of bundles, nor for the attestation round trips below. They
  # package the notices with the binaries, so a release short of them stops
  # here rather than in each packager.
  PATTERNS=("gitlab-mcp-server-darwin-*" "gitlab-mcp-server-linux-*" "gitlab-mcp-server-windows-*" "$NOTICES")
  REQUIRE_NOTICES=1
fi

REPO="${REPO:-${GITHUB_REPOSITORY:-jmrplens/gitlab-mcp-server}}"
TAG="v${VERSION}"
OIDC_ISSUER="https://token.actions.githubusercontent.com"
SIGNER_IDENTITY="https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${TAG}"
ARCHIVE="${REHEARSAL_ARCHIVE:-}"
# The release's SLSA provenance: the build-provenance attestations of every
# file in checksums.txt and of every bundle, as the release job attached them
# while the release was a draft. 3.4.0 is the last release without it, so a
# later one that lacks it is refused wherever a bundle is verified, since the
# bundles have no other signature.
PROVENANCE="gitlab-mcp-server.intoto.jsonl"
LAST_WITHOUT_PROVENANCE="3.4.0"
PROVENANCE_REQUIRED=0
if [ "$(printf '%s\n%s\n' "$LAST_WITHOUT_PROVENANCE" "$VERSION" | sort -V | tail -n 1)" != "$LAST_WITHOUT_PROVENANCE" ]; then
  PROVENANCE_REQUIRED=1
fi

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
  # rest verifiable. The provenance is fetched beside them whenever assets
  # are, and only the bundles depend on it, below.
  args+=(--pattern "checksums.txt" --pattern "checksums.txt.sigstore.json")
  if [ "$CHECKSUMS_ONLY" -eq 0 ]; then
    args+=(--pattern "$PROVENANCE")
  fi
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
# is a normal case: the registry and manifest jobs fetch the bundles alone, and
# the bundles are deliberately absent from checksums.txt. Read as a failure,
# that status stopped both jobs before the bundles' own verification below
# could run, which the first rehearsal to reach them found. So a FAILED line
# fails here, and what was actually checked is counted, with each bundle
# joining the count on its own evidence.
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
# gh downloads whatever matches and says nothing about a pattern that matched
# nothing, so the notices are required by name: present, and verified.
if [ "$REQUIRE_NOTICES" -eq 1 ] && ! grep -qxF "${NOTICES}: OK" "$DEST/sha256-check.log"; then
  echo "ERROR: ${TAG} has no ${NOTICES} listed in checksums.txt, and every package carries it beside the binaries" >&2
  rm -f "$DEST/sha256-check.log"
  exit 1
fi
verified=$(grep -c ": OK$" "$DEST/sha256-check.log" || true)
rm -f "$DEST/sha256-check.log"
echo "Verified ${verified} asset(s) against checksums.txt"

# The .mcpb bundles are built outside GoReleaser, so they are absent from
# checksums.txt and are the one kind of asset a job can legitimately fetch on
# its own. Their integrity comes from the build-provenance attestation instead,
# and each counts towards the "something was actually verified" floor below:
# every bundle in <dest-dir> is checked, the per-OS ones a registry job fetches
# and the universal one alike, so a bundle nobody attested fails here whichever
# pattern brought it.
#
# The attestation is read from the release's own provenance, with --bundle,
# which asks GitHub's API nothing and needs no permission beyond downloading
# the release, and it is held to the same signer as checksums.txt above: the
# release workflow at this release's tag. --repo alone accepts an attestation any
# workflow of the repository minted, at any ref, and these bundles are the ones
# whose hashes go into server.json. A release up to 3.4.0 published no
# provenance, so its bundles are looked up in GitHub's attestation store.
#
# A rehearsal attests nothing: the bundles it unpacked were built by a job of
# the same run, and the provenance it carries is the release's statements
# unsigned, so each bundle is held to being one of their subjects, by name and
# SHA-256, and to nothing more.
provenance_names() {
  # Every subject of the provenance's statements, as "<sha256>  <name>", a
  # line in either form: a Sigstore bundle or a bare in-toto statement.
  jq -r '(if has("dsseEnvelope") then .dsseEnvelope.payload | @base64d | fromjson else . end)
    | .subject[] | "\(.digest.sha256)  \(.name)"' "$1"
}
for bundle in "$DEST"/*.mcpb; do
  [ -f "$bundle" ] || continue
  name=$(basename "$bundle")
  if [ -f "$DEST/$PROVENANCE" ] && [ -n "$ARCHIVE" ]; then
    entry="$(sha256sum "$bundle" | cut -d' ' -f1)  ${name}"
    # grep reads to the end, without -q, so jq never writes into a closed pipe.
    if ! provenance_names "$DEST/$PROVENANCE" | grep -xF "$entry" > /dev/null; then
      echo "ERROR: ${name} is not a subject of the rehearsal's ${PROVENANCE} at its SHA-256" >&2
      exit 1
    fi
    echo "Rehearsal: ${name} is a subject of this run's ${PROVENANCE}, which is signed only at release"
  elif [ -f "$DEST/$PROVENANCE" ]; then
    echo "Verifying ${name} against ${PROVENANCE}"
    gh attestation verify "$bundle" --bundle "$DEST/$PROVENANCE" --repo "$REPO" --cert-identity "$SIGNER_IDENTITY"
  elif [ "$PROVENANCE_REQUIRED" -eq 1 ]; then
    echo "ERROR: ${TAG} carries no ${PROVENANCE}, which every release after ${LAST_WITHOUT_PROVENANCE} publishes, so ${name} has nothing to be verified against" >&2
    exit 1
  elif [ -n "$ARCHIVE" ]; then
    echo "Rehearsal: ${name} came from this run's own build; its attestation is minted at release"
  else
    echo "Verifying the build-provenance attestation of ${name} in GitHub's attestation store"
    gh attestation verify "$bundle" --repo "$REPO" --cert-identity "$SIGNER_IDENTITY"
  fi
  verified=$((verified + 1))
done

if [ "$verified" -eq 0 ]; then
  echo "ERROR: nothing here could be verified — checksums.txt matched no file and no bundle was found" >&2
  exit 1
fi
