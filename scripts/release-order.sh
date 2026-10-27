#!/usr/bin/env bash
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

# release-order.sh decides, for the tag a release is cut from, whether it may
# be released at all and whether it is the newest release, which is what the
# release workflow's preflight asks before anything publishes (issue 1323).
#
# Usage: release-order.sh <tag>   (the tags the repository holds on stdin)
#
# stdin is the tag list as `git ls-remote --tags --refs <remote> 'v*'` prints
# it, or one tag name per line; the last field of each line is read and a
# refs/tags/ prefix dropped. The tag being released does not have to be in it:
# a rehearsal names the version VERSION carries before that tag exists.
#
# It prints three lines for $GITHUB_OUTPUT:
#
#   stable=true          the tag is a plain vX.Y.Z; anything else is refused
#   newest=true|false    no stable tag in the list is higher than it
#   npm_dist_tag=...     latest for the newest release, release-X.Y otherwise
#
# Only those lines go to stdout, so the workflow can append it to
# $GITHUB_OUTPUT whole; everything else goes to stderr. A prerelease or
# malformed tag exits 1 with an ::error:: line and prints no outputs, so
# preflight fails and nothing downstream of it runs. Before this,
# a prerelease tag pushed the image as :latest and published the GitHub
# release, and only then did the npm job refuse it, which left the release
# half published. A stable tag lower than one already released is a backport:
# it publishes under its own version everywhere, and leaves every pointer to
# the newest release (the image's :latest, the GitHub release marked Latest,
# npm's latest dist-tag, the Homebrew formula, main's manifests and the MCP
# Registry entry) where it is. Versions are compared with sort -V, so v3.10.0
# is above v3.9.0, and prerelease tags already in the list are not compared at
# all.
set -euo pipefail

TAG="${1:?Usage: $0 <tag>   (the tags the repository holds on stdin)}"
STABLE_TAG='^v[0-9]+\.[0-9]+\.[0-9]+$'

if ! [[ "$TAG" =~ $STABLE_TAG ]]; then
  echo "::error::${TAG} is not a release version: a release tag is vMAJOR.MINOR.PATCH, and a prerelease or build suffix is refused before anything publishes, because :latest, the GitHub release and the manifests on main would move to it" >&2
  exit 1
fi

highest="$TAG"
while read -r line || [ -n "$line" ]; do
  name="${line##*[[:space:]]}"
  name="${name#refs/tags/}"
  if [[ "$name" =~ $STABLE_TAG ]]; then
    highest="$(printf '%s\n%s\n' "$highest" "$name" | sort -V | tail -n 1)"
  fi
done

echo "stable=true"
if [ "$highest" = "$TAG" ]; then
  echo "newest=true"
  echo "npm_dist_tag=latest"
else
  minor="${TAG#v}"
  minor="${minor%.*}"
  echo "newest=false"
  echo "npm_dist_tag=release-${minor}"
  echo "${TAG} is a backport: ${highest} is newer, so the pointers to the newest release stay on it" >&2
fi
