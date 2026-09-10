#!/usr/bin/env bash
# Make the referrers fallback tag say what the referrers it lists are.
#
# A registry without the OCI referrers API (ghcr.io answers 404 on
# /v2/<name>/referrers/<digest>) leaves the referrers list to the client: an
# image index at the tag sha256-<hex> of the subject, maintained by
# read-modify-write. The distribution spec says the descriptor a client
# appends there MUST carry the pushed manifest's artifactType (its config
# mediaType when it has none) and MUST copy all of its annotations. Every
# go-containerregistry release before v0.22.1 wrote less than that: the
# artifactType of the config until v0.21.6 and no annotations until v0.22.1,
# and cosign carries go-containerregistry, so a cosign signature landed in the
# list as "application/vnd.oci.empty.v1+json" with nothing else, which is a
# descriptor no consumer that filters the list can match. The manifest behind
# it was always right; only the list entry was wrong.
#
# This reads every entry back through the manifest it names and rewrites the
# tag when an entry is short of what its manifest says. It changes no
# manifest, no blob and no digest a consumer resolves: a subject's referrers
# are found by digest from these entries, and cosign verify reads all of them
# without looking at these fields at all. A registry that serves the referrers
# API has no such tag, and the fetch reports it as absent.
#
# Usage: referrers-fallback-repair.sh <repository> <subject-digest>...
# ORAS_FLAGS adds flags to every oras call (a test registry passes --plain-http).
# DRY_RUN=1 reports what would be rewritten and pushes nothing.

set -euo pipefail

REPO="${1:?Usage: $0 <repository> <subject-digest>...}"
shift
[ "$#" -gt 0 ] || { echo "Usage: $0 <repository> <subject-digest>..." >&2; exit 2; }
read -r -a ORAS <<<"${ORAS_FLAGS:-}"
oras() { command oras "$@" ${ORAS[@]+"${ORAS[@]}"}; }

INDEX_TYPE=application/vnd.oci.image.index.v1+json
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

for digest in "$@"; do
  if ! [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
    echo "::error::${digest} is not a sha256 digest" >&2
    exit 2
  fi
  tag="sha256-${digest#sha256:}"
  work="${scratch}/${tag}"
  mkdir -p "$work"

  if ! oras manifest fetch "${REPO}:${tag}" >"${work}/index.json" 2>"${work}/fetch.err"; then
    if grep -q -i -E "not found|MANIFEST_UNKNOWN|NAME_UNKNOWN" "${work}/fetch.err"; then
      echo "${tag}: no fallback tag (the registry serves the referrers API, or nothing refers to this digest)"
      continue
    fi
    cat "${work}/fetch.err" >&2
    exit 1
  fi

  media_type="$(jq -r '.mediaType // ""' "${work}/index.json")"
  if [ "$media_type" != "$INDEX_TYPE" ]; then
    echo "::error::${tag} is ${media_type:-untyped}, not an image index; leaving it alone" >&2
    exit 1
  fi

  count="$(jq '.manifests | length' "${work}/index.json")"
  cp "${work}/index.json" "${work}/new.json"
  for ((i = 0; i < count; i++)); do
    entry="$(jq -r ".manifests[$i].digest" "${work}/index.json")"
    # An entry whose manifest is gone says nothing about itself; leave it as it is.
    if ! oras manifest fetch "${REPO}@${entry}" >"${work}/m.json" 2>"${work}/entry.err"; then
      echo "::warning::${tag}: entry ${entry} could not be read, its descriptor is left untouched: $(tr '\n' ' ' <"${work}/entry.err")"
      continue
    fi
    # artifactType: the manifest's own, else its config mediaType. annotations:
    # the manifest's, all of them. Nothing else on the entry is touched.
    jq --slurpfile m "${work}/m.json" \
      ".manifests[$i] |= (
         . + (if (\$m[0].artifactType // \"\") != \"\" then {artifactType: \$m[0].artifactType}
              elif (\$m[0].config.mediaType // \"\") != \"\" then {artifactType: \$m[0].config.mediaType}
              else {} end)
           + (if (\$m[0].annotations // {}) != {} then {annotations: \$m[0].annotations} else {} end))" \
      "${work}/new.json" >"${work}/tmp.json"
    mv "${work}/tmp.json" "${work}/new.json"
  done

  if cmp -s <(jq -S . "${work}/index.json") <(jq -S . "${work}/new.json"); then
    echo "${tag}: ${count} descriptor(s) already say what their manifests say"
    continue
  fi

  if [ -n "${DRY_RUN:-}" ]; then
    echo "${tag}: would rewrite ${count} descriptor(s) (DRY_RUN set, nothing pushed)"
  else
    jq -c . "${work}/new.json" >"${work}/push.json"
    oras manifest push --media-type "$INDEX_TYPE" "${REPO}:${tag}" "${work}/push.json" >/dev/null
    echo "${tag}: rewrote ${count} descriptor(s)"
  fi
  jq -r '.manifests[] | "  \(.digest[0:19]) artifactType=\(.artifactType // "-") annotations=\(.annotations // {} | keys | join(","))"' "${work}/new.json"
done
