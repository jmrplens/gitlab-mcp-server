#!/usr/bin/env bash
# docker-images.sh: which image a Docker run tests, and whether it is the one
# its tag names today.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_docker_images_test.py,
# which drives these functions with a stand-in docker and curl answering what
# a real daemon and registry answered. It defines functions and runs nothing,
# so sourcing it changes no state.
#
# The reason it exists is that `docker compose up` pulls an image only when it
# is missing. A host kept testing the GitLab release it first pulled, five
# weeks old by the time anyone looked, and every scenario gated on a newer
# release skipped while the run stayed green. Pulling on every run would fix
# that at the cost of gigabytes a run for nothing, so a run asks the registry
# which image the tag names now, without pulling, and pulls only when that is
# not the image it holds.

# image_freshness says how a local image stands against the registry: it is
# given the digest the registry's tag names now and the local image's
# RepoDigests, one per line, and prints one word.
#
#   current     the local image is the one the tag names now
#   stale       the tag names another image, so the registry has a newer one
#   unrecorded  the local image carries no registry digest at all, since it
#               was built or loaded here, so there is nothing to compare
#
# A digest is the image's content address, so a match under any repository
# name is the same image: a pull through a mirror records the digest the
# registry does.
image_freshness() {
    local digest="$1" repo_digests="$2" recorded=""
    while IFS= read -r line; do
        [ -n "${line}" ] || continue
        recorded=yes
        if [ "${line##*@}" = "${digest}" ]; then
            echo current
            return 0
        fi
    done <<<"${repo_digests}"
    if [ -n "${recorded}" ]; then
        echo stale
    else
        echo unrecorded
    fi
}

# registry_digest prints the digest the registry's tag names now, without
# pulling anything: `docker buildx imagetools inspect` reads the manifest from
# the registry and answers Docker Hub anonymously. It fails when the registry
# could not be asked or answered something that is not a digest.
registry_digest() {
    local reference="$1" digest
    digest="$(docker buildx imagetools inspect --format '{{.Manifest.Digest}}' "${reference}" 2>/dev/null)" || return 1
    if [[ ! ${digest} =~ ^sha256:[0-9a-f]{64}$ ]]; then
        return 1
    fi
    printf '%s\n' "${digest}"
}

# refresh_image brings one local image up to the one its tag names now, and
# pulls only when that is another image. It never fails the run: a registry it
# cannot reach, or a pull that fails, keeps the local image and says so.
#
#   - no local image: nothing is done, and compose pulls it as it always has;
#   - a reference pinned by digest: nothing is done, since a digest never moves
#     and compose pulls it when it is missing;
#   - the local image is the current one: nothing is done;
#   - the registry has another: it is pulled, and the image it replaced is
#     removed when no tag and no container still holds it.
refresh_image() {
    local reference="$1" local_id repo_digests digest new_id
    case "${reference}" in
        *@sha256:*)
            echo "    ${reference}: pinned by digest, which never moves; nothing to ask the registry"
            return 0
            ;;
    esac
    if ! local_id="$(docker image inspect --format '{{.Id}}' "${reference}" 2>/dev/null)"; then
        echo "    ${reference}: no local image; compose pulls it"
        return 0
    fi
    repo_digests="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "${reference}" 2>/dev/null)" || repo_digests=""
    if ! digest="$(registry_digest "${reference}")"; then
        echo "WARN: ${reference}: docker buildx imagetools inspect could not ask the registry which image its tag names now; keeping the local image ${local_id}" >&2
        return 0
    fi
    case "$(image_freshness "${digest}" "${repo_digests}")" in
        current)
            echo "    ${reference}: the local image is the registry's current one (${digest}); no pull"
            return 0
            ;;
        unrecorded)
            echo "WARN: ${reference}: the local image ${local_id} carries no registry digest, so nothing says whether it is current; keeping it" >&2
            return 0
            ;;
    esac
    echo "    ${reference}: the registry has a newer image (${digest}); pulling it"
    if ! docker pull "${reference}"; then
        echo "WARN: ${reference}: the pull failed; keeping the local image ${local_id}" >&2
        return 0
    fi
    new_id="$(docker image inspect --format '{{.Id}}' "${reference}" 2>/dev/null)" || new_id=""
    if [ "${new_id}" = "${local_id}" ]; then
        echo "    ${reference}: the pull brought no new image (${local_id}); nothing to remove"
        return 0
    fi
    remove_replaced_image "${reference}" "${local_id}"
}

# remove_replaced_image removes the image a pull replaced, by its ID, unless a
# tag still names it or a container, running or stopped, was created from it.
# Removing either would take something from whoever holds it, so the image is
# kept and the reason printed.
remove_replaced_image() {
    local reference="$1" old_id="$2" tags users
    tags="$(docker image inspect --format '{{range .RepoTags}}{{println .}}{{end}}' "${old_id}" 2>/dev/null)" || tags=""
    tags="$(printf '%s' "${tags}" | tr '\n' ' ')"
    tags="${tags% }"
    if [ -n "${tags}" ]; then
        echo "    ${reference}: kept the image it replaced, ${old_id}, which is still tagged ${tags}"
        return 0
    fi
    users="$(docker ps -a -q --filter "ancestor=${old_id}" 2>/dev/null)" || users=""
    users="$(printf '%s' "${users}" | tr '\n' ' ')"
    users="${users% }"
    if [ -n "${users}" ]; then
        echo "    ${reference}: kept the image it replaced, ${old_id}, which container ${users} still uses"
        return 0
    fi
    if docker image rm "${old_id}" >/dev/null; then
        echo "    ${reference}: removed the image it replaced, ${old_id}"
    else
        echo "WARN: ${reference}: could not remove the image it replaced, ${old_id}" >&2
    fi
}

# tested_gitlab_version prints the GitLab release the instance at the URL
# reports, read with the token the setup script wrote into the env file, as
# "<version> (revision <revision>)". It is what the run tested, whatever the
# image's tag says, and it prints "unknown" with the reason rather than fail
# the run.
tested_gitlab_version() {
    local url="$1" env_file="$2" token answer
    # shellcheck source=/dev/null
    token="$(set -a && . "${env_file}" >/dev/null 2>&1 && printf '%s' "${GITLAB_TOKEN:-}")" || token=""
    if [ -z "${token}" ]; then
        echo "unknown (${env_file} holds no GITLAB_TOKEN to ask with)"
        return 0
    fi
    if ! answer="$(curl -sf --max-time 30 -H "PRIVATE-TOKEN: ${token}" "${url%/}/api/v4/version")"; then
        echo "unknown (GET ${url%/}/api/v4/version did not answer)"
        return 0
    fi
    printf '%s' "${answer}" | python3 -c '
import json, sys
try:
    reported = json.load(sys.stdin)
except ValueError:
    reported = None
if not isinstance(reported, dict) or not reported.get("version"):
    print("unknown (GET /api/v4/version answered no version)")
else:
    print("%s (revision %s)" % (reported["version"], reported.get("revision") or "unknown"))
'
}
