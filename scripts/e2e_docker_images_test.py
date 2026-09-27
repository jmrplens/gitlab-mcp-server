#!/usr/bin/env python3
"""Tests which image a Docker e2e run tests, and when it pulls a newer one.

test/e2e/scripts/run-docker-e2e.sh used to leave the images to `docker
compose up`, which pulls an image only when it is missing, so a host kept
testing the GitLab release it first pulled: five weeks old by the time issue
1014 looked, and every scenario gated on a newer release skipped while the run
stayed green. The run now asks the registry which image each tag names today,
without pulling, and pulls only when that is not the image it holds, removing
the one it replaced. The functions that decide it live in
test/e2e/scripts/docker-images.sh so that these tests can drive them with a
stand-in docker and curl, without a daemon and without a registry.

The stand-in answers what the real ones answered on 2026-09-27, recorded on
the host whose runs the issue describes: `docker image inspect` of the cached
gitlab/gitlab-ce:latest and gitlab/gitlab-ee:latest, and `docker buildx
imagetools inspect --format '{{.Manifest.Digest}}'` of the same tags against
Docker Hub, anonymously. Both cached images were behind their tags.

Run with:

    python3 -m unittest discover -s scripts -p 'e2e_docker_images_test.py'
"""

import os
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "test", "e2e", "scripts", "docker-images.sh")
RUNNER = os.path.join(ROOT, "test", "e2e", "scripts", "run-docker-e2e.sh")
COMPOSE = os.path.join(ROOT, "test", "e2e", "docker-compose.yml")

# What the daemon and the registry answered on 2026-09-27. The CE image the
# host held, its RepoDigests line (inspect ends every line with a newline and
# the listing with one more), and the digest the tag names now. imagetools
# prints the digest with no newline after it.
CE_REFERENCE = "gitlab/gitlab-ce:latest"
CE_LOCAL_ID = "sha256:2558871d07c99ad1a3c00f5d486e6e39820ac1796738d8f82f0d987886c30bca"
CE_REPO_DIGESTS = "gitlab/gitlab-ce@sha256:f7e453ff51d1910235365085fe836e4589716d26b44d99a8aa3e2c41377f034f\n\n"
CE_REGISTRY_DIGEST = "sha256:9b33b45b9f42d176bada85ee5ecb81ddab7e506c435f44cd582206e284b2809c"
EE_REFERENCE = "gitlab/gitlab-ee:latest"
EE_LOCAL_ID = "sha256:b0f0ead69bceb941c50e33492804c1baa35c03f55a282ac90020de75b561dff4"
EE_REPO_DIGESTS = "gitlab/gitlab-ee@sha256:b5167605564d64acf896be614791092d1409ac3f78214a9e4394ebb24a0be1b5\n\n"
EE_REGISTRY_DIGEST = "sha256:803e5887cd561ed49ce84c92756838d39e9bcef5c061d4399f2e1057a335cae2"
# An image ID a pull of the new CE digest would leave the tag on. Invented,
# since recording it would have meant pulling.
CE_NEW_ID = "sha256:" + "4" * 64

# The variables the stand-ins read, cleared before every case so the
# developer's own environment is never part of one.
STUB_KEYS = (
    "LOCAL_ID",
    "NEW_ID",
    "REPO_DIGESTS",
    "REMOTE_DIGEST",
    "REGISTRY_FAILS",
    "PULL_FAILS",
    "OLD_TAGS",
    "USERS",
    "RM_FAILS",
    "CURL_ANSWER",
    "CURL_FAILS",
)

# The stand-in docker: every call is appended to $STATE/calls, one per line,
# and each answer is read from the environment the case sets. A successful
# pull leaves $STATE/pulled behind, after which the tag names NEW_ID. The
# driver sources the script under the options run-docker-e2e.sh runs with,
# runs the command it is handed, and prints its exit status last.
DRIVER = r"""
set -euo pipefail
. "$1"
shift
docker() {
    printf '%s\n' "docker $*" >> "${STATE}/calls"
    case "$1 $2" in
        "image inspect")
            case "$4" in
                '{{.Id}}')
                    if [ -f "${STATE}/pulled" ]; then
                        printf '%s\n' "${NEW_ID}"
                    elif [ -n "${LOCAL_ID}" ]; then
                        printf '%s\n' "${LOCAL_ID}"
                    else
                        echo "Error response from daemon: No such image: $5" >&2
                        return 1
                    fi
                    ;;
                '{{range .RepoDigests}}{{println .}}{{end}}') printf '%s' "${REPO_DIGESTS}" ;;
                '{{range .RepoTags}}{{println .}}{{end}}') printf '%s' "${OLD_TAGS}" ;;
                *) echo "stand-in: unexpected format $4" >&2; return 99 ;;
            esac
            ;;
        "buildx imagetools")
            if [ -n "${REGISTRY_FAILS}" ]; then
                echo "ERROR: failed to do request: dial tcp: lookup registry-1.docker.io: no such host" >&2
                return 1
            fi
            printf '%s' "${REMOTE_DIGEST}"
            ;;
        "ps -a") printf '%s' "${USERS}" ;;
        "image rm") [ -z "${RM_FAILS}" ] || return 1 ;;
        pull\ *)
            [ -z "${PULL_FAILS}" ] || return 1
            : > "${STATE}/pulled"
            ;;
        *) echo "stand-in: unexpected docker $*" >&2; return 99 ;;
    esac
}
curl() {
    printf '%s\n' "curl $*" >> "${STATE}/calls"
    [ -z "${CURL_FAILS}" ] || return 22
    printf '%s' "${CURL_ANSWER}"
}
status=0
"$@" || status=$?
echo "status=${status}"
"""


def drive(*command, **stub):
    """Runs one function of the script against the stand-ins.

    Returns its stdout without the status line, its stderr, its exit status
    and the calls it made, one string each.
    """
    with tempfile.TemporaryDirectory() as state:
        env = {key: value for key, value in os.environ.items() if key not in STUB_KEYS}
        env.update({key: "" for key in STUB_KEYS})
        env["STATE"] = state
        env.update(stub)
        result = subprocess.run(
            ["bash", "-c", DRIVER, "driver", SCRIPT, *command],
            env=env,
            capture_output=True,
            text=True,
            check=True,
        )
        calls_path = os.path.join(state, "calls")
        calls = []
        if os.path.exists(calls_path):
            with open(calls_path, encoding="utf-8") as handle:
                calls = handle.read().splitlines()
    lines = result.stdout.splitlines()
    status = lines.pop()
    return {
        "stdout": "\n".join(lines),
        "stderr": result.stderr,
        "status": status,
        "calls": calls,
    }


def pulls(calls):
    return [call for call in calls if call.startswith("docker pull ")]


def removals(calls):
    return [call for call in calls if call.startswith("docker image rm ")]


class ImageFreshnessTest(unittest.TestCase):
    def freshness(self, digest, repo_digests):
        got = drive("image_freshness", digest, repo_digests)
        self.assertEqual(got["status"], "status=0")
        return got["stdout"]

    def test_the_recorded_ce_image_is_stale(self):
        self.assertEqual(self.freshness(CE_REGISTRY_DIGEST, CE_REPO_DIGESTS), "stale")

    def test_an_image_whose_digest_the_tag_names_is_current(self):
        self.assertEqual(self.freshness(CE_REGISTRY_DIGEST, "gitlab/gitlab-ce@" + CE_REGISTRY_DIGEST + "\n\n"), "current")

    def test_a_digest_recorded_under_another_repository_is_the_same_image(self):
        # A pull through a mirror records the content address the registry
        # serves under the mirror's name; the digest decides, not the name.
        repo_digests = "mirror.example/gitlab/gitlab-ce@sha256:" + "1" * 64 + "\nregistry.example/gitlab-ce@" + CE_REGISTRY_DIGEST + "\n"
        self.assertEqual(self.freshness(CE_REGISTRY_DIGEST, repo_digests), "current")

    def test_a_digest_that_only_begins_like_the_current_one_is_another_image(self):
        self.assertEqual(self.freshness(CE_REGISTRY_DIGEST, "gitlab/gitlab-ce@" + CE_REGISTRY_DIGEST + "0\n"), "stale")

    def test_an_image_with_no_registry_digest_is_unrecorded(self):
        for repo_digests in ("", "\n", "\n\n"):
            with self.subTest(repo_digests=repo_digests):
                self.assertEqual(self.freshness(CE_REGISTRY_DIGEST, repo_digests), "unrecorded")


class RegistryDigestTest(unittest.TestCase):
    def test_the_recorded_answer_is_the_digest(self):
        got = drive("registry_digest", CE_REFERENCE, REMOTE_DIGEST=CE_REGISTRY_DIGEST)
        self.assertEqual((got["stdout"], got["status"]), (CE_REGISTRY_DIGEST, "status=0"))
        self.assertEqual(
            got["calls"],
            [f"docker buildx imagetools inspect --format {{{{.Manifest.Digest}}}} {CE_REFERENCE}"],
        )

    def test_an_answer_that_is_not_a_digest_fails(self):
        for answer in ("", "sha256:abc", "sha512:" + "0" * 64, CE_REGISTRY_DIGEST + "\nsha256:" + "0" * 64, "sha256:" + "A" * 64):
            with self.subTest(answer=answer):
                got = drive("registry_digest", CE_REFERENCE, REMOTE_DIGEST=answer)
                self.assertEqual((got["stdout"], got["status"]), ("", "status=1"))

    def test_a_registry_that_cannot_be_reached_fails_quietly(self):
        got = drive("registry_digest", CE_REFERENCE, REGISTRY_FAILS="yes")
        self.assertEqual((got["stdout"], got["stderr"], got["status"]), ("", "", "status=1"))


class RefreshImageTest(unittest.TestCase):
    def refresh(self, reference=CE_REFERENCE, **stub):
        got = drive("refresh_image", reference, **stub)
        # Whatever happened, the run goes on: a refresh never fails it.
        self.assertEqual(got["status"], "status=0", got)
        return got

    def test_a_missing_image_is_left_to_compose(self):
        got = self.refresh()
        self.assertEqual(got["stdout"], f"    {CE_REFERENCE}: no local image; compose pulls it")
        self.assertEqual(got["calls"], [f"docker image inspect --format {{{{.Id}}}} {CE_REFERENCE}"])

    def test_a_reference_pinned_by_digest_asks_nothing(self):
        reference = "gitlab/gitlab-ce@" + CE_REGISTRY_DIGEST
        got = self.refresh(reference, LOCAL_ID=CE_LOCAL_ID)
        self.assertEqual(got["stdout"], f"    {reference}: pinned by digest, which never moves; nothing to ask the registry")
        self.assertEqual(got["calls"], [])

    def test_the_current_image_is_not_pulled(self):
        got = self.refresh(
            EE_REFERENCE,
            LOCAL_ID=EE_LOCAL_ID,
            REPO_DIGESTS="gitlab/gitlab-ee@" + EE_REGISTRY_DIGEST + "\n\n",
            REMOTE_DIGEST=EE_REGISTRY_DIGEST,
        )
        self.assertEqual(got["stdout"], f"    {EE_REFERENCE}: the local image is the registry's current one ({EE_REGISTRY_DIGEST}); no pull")
        self.assertEqual(pulls(got["calls"]), [])
        self.assertEqual(removals(got["calls"]), [])

    def test_the_recorded_stale_images_are_pulled_and_the_old_ones_removed(self):
        for reference, local_id, repo_digests, digest in (
            (CE_REFERENCE, CE_LOCAL_ID, CE_REPO_DIGESTS, CE_REGISTRY_DIGEST),
            (EE_REFERENCE, EE_LOCAL_ID, EE_REPO_DIGESTS, EE_REGISTRY_DIGEST),
        ):
            with self.subTest(reference=reference):
                got = self.refresh(reference, LOCAL_ID=local_id, NEW_ID=CE_NEW_ID, REPO_DIGESTS=repo_digests, REMOTE_DIGEST=digest)
                self.assertEqual(
                    got["stdout"].splitlines(),
                    [
                        f"    {reference}: the registry has a newer image ({digest}); pulling it",
                        f"    {reference}: removed the image it replaced, {local_id}",
                    ],
                )
                self.assertEqual(pulls(got["calls"]), [f"docker pull {reference}"])
                # By the ID it had, never by the tag, which names the new
                # image by now.
                self.assertEqual(removals(got["calls"]), [f"docker image rm {local_id}"])
                self.assertIn(f"docker ps -a -q --filter ancestor={local_id}", got["calls"])
                self.assertEqual(got["stderr"], "")

    def test_the_replaced_image_is_kept_while_a_tag_still_names_it(self):
        got = self.refresh(
            LOCAL_ID=CE_LOCAL_ID,
            NEW_ID=CE_NEW_ID,
            REPO_DIGESTS=CE_REPO_DIGESTS,
            REMOTE_DIGEST=CE_REGISTRY_DIGEST,
            OLD_TAGS="gitlab/gitlab-ce:19.3.0-ce.0\nlocal/gitlab:pinned\n",
        )
        self.assertEqual(
            got["stdout"].splitlines()[-1],
            f"    {CE_REFERENCE}: kept the image it replaced, {CE_LOCAL_ID}, which is still tagged gitlab/gitlab-ce:19.3.0-ce.0 local/gitlab:pinned",
        )
        self.assertEqual(removals(got["calls"]), [])

    def test_the_replaced_image_is_kept_while_a_container_uses_it(self):
        got = self.refresh(
            LOCAL_ID=CE_LOCAL_ID,
            NEW_ID=CE_NEW_ID,
            REPO_DIGESTS=CE_REPO_DIGESTS,
            REMOTE_DIGEST=CE_REGISTRY_DIGEST,
            USERS="16eee3bda8c3\na7310a3bd9ba\n",
        )
        self.assertEqual(
            got["stdout"].splitlines()[-1],
            f"    {CE_REFERENCE}: kept the image it replaced, {CE_LOCAL_ID}, which container 16eee3bda8c3 a7310a3bd9ba still uses",
        )
        self.assertEqual(removals(got["calls"]), [])

    def test_a_removal_that_fails_warns(self):
        got = self.refresh(
            LOCAL_ID=CE_LOCAL_ID,
            NEW_ID=CE_NEW_ID,
            REPO_DIGESTS=CE_REPO_DIGESTS,
            REMOTE_DIGEST=CE_REGISTRY_DIGEST,
            RM_FAILS="yes",
        )
        self.assertEqual(got["stderr"], f"WARN: {CE_REFERENCE}: could not remove the image it replaced, {CE_LOCAL_ID}\n")

    def test_a_pull_that_brings_the_same_image_removes_nothing(self):
        # The index a tag names can move while the platform's image stays the
        # same, when only another platform was rebuilt.
        got = self.refresh(LOCAL_ID=CE_LOCAL_ID, NEW_ID=CE_LOCAL_ID, REPO_DIGESTS=CE_REPO_DIGESTS, REMOTE_DIGEST=CE_REGISTRY_DIGEST)
        self.assertEqual(got["stdout"].splitlines()[-1], f"    {CE_REFERENCE}: the pull brought no new image ({CE_LOCAL_ID}); nothing to remove")
        self.assertEqual(removals(got["calls"]), [])

    def test_a_failed_pull_keeps_the_local_image(self):
        got = self.refresh(LOCAL_ID=CE_LOCAL_ID, REPO_DIGESTS=CE_REPO_DIGESTS, REMOTE_DIGEST=CE_REGISTRY_DIGEST, PULL_FAILS="yes")
        self.assertEqual(got["stderr"], f"WARN: {CE_REFERENCE}: the pull failed; keeping the local image {CE_LOCAL_ID}\n")
        self.assertEqual(removals(got["calls"]), [])

    def test_an_unreachable_registry_keeps_the_local_image_and_warns(self):
        got = self.refresh(LOCAL_ID=CE_LOCAL_ID, REPO_DIGESTS=CE_REPO_DIGESTS, REGISTRY_FAILS="yes")
        self.assertEqual(
            got["stderr"],
            f"WARN: {CE_REFERENCE}: docker buildx imagetools inspect could not ask the registry which image its tag names now; "
            f"keeping the local image {CE_LOCAL_ID}\n",
        )
        self.assertEqual(got["stdout"], "")
        self.assertEqual(pulls(got["calls"]), [])

    def test_an_answer_that_is_not_a_digest_keeps_the_local_image_and_warns(self):
        got = self.refresh(LOCAL_ID=CE_LOCAL_ID, REPO_DIGESTS=CE_REPO_DIGESTS, REMOTE_DIGEST="<html>rate limited</html>")
        self.assertIn("could not ask the registry", got["stderr"])
        self.assertEqual(pulls(got["calls"]), [])

    def test_an_image_with_no_registry_digest_is_kept_and_warns(self):
        got = self.refresh(LOCAL_ID=CE_LOCAL_ID, REPO_DIGESTS="\n", REMOTE_DIGEST=CE_REGISTRY_DIGEST)
        self.assertEqual(
            got["stderr"],
            f"WARN: {CE_REFERENCE}: the local image {CE_LOCAL_ID} carries no registry digest, so nothing says whether it is current; keeping it\n",
        )
        self.assertEqual(pulls(got["calls"]), [])


class TestedGitLabVersionTest(unittest.TestCase):
    def version(self, env_text, url="http://localhost:8929/", **stub):
        with tempfile.NamedTemporaryFile("w", suffix=".env", delete=False) as handle:
            handle.write(env_text)
            env_file = handle.name
        try:
            got = drive("tested_gitlab_version", url, env_file, **stub)
        finally:
            os.unlink(env_file)
        self.assertEqual(got["status"], "status=0", got)
        return got

    def test_the_version_and_revision_the_instance_reports(self):
        got = self.version(
            "GITLAB_URL=http://localhost:8929\nGITLAB_TOKEN=glpat-e2e\n",
            CURL_ANSWER='{"version":"19.4.1-ee","revision":"6a0b1f2c3d4","kas":{"enabled":false},"enterprise":true}',
        )
        self.assertEqual(got["stdout"], "19.4.1-ee (revision 6a0b1f2c3d4)")
        # The trailing slash of the URL is not doubled.
        self.assertEqual(
            got["calls"],
            ["curl -sf --max-time 30 -H PRIVATE-TOKEN: glpat-e2e http://localhost:8929/api/v4/version"],
        )

    def test_a_missing_revision_is_said_to_be_unknown(self):
        got = self.version("GITLAB_TOKEN=glpat-e2e\n", CURL_ANSWER='{"version":"19.4.1"}')
        self.assertEqual(got["stdout"], "19.4.1 (revision unknown)")

    def test_an_instance_that_does_not_answer_is_unknown(self):
        got = self.version("GITLAB_TOKEN=glpat-e2e\n", CURL_FAILS="yes")
        self.assertEqual(got["stdout"], "unknown (GET http://localhost:8929/api/v4/version did not answer)")

    def test_an_answer_with_no_version_is_unknown(self):
        for answer in ("not json", "[]", '{"version":""}', "{}"):
            with self.subTest(answer=answer):
                got = self.version("GITLAB_TOKEN=glpat-e2e\n", CURL_ANSWER=answer)
                self.assertEqual(got["stdout"], "unknown (GET /api/v4/version answered no version)")

    def test_an_env_file_with_no_token_asks_nothing(self):
        got = self.version("GITLAB_URL=http://localhost:8929\n")
        self.assertTrue(got["stdout"].startswith("unknown (") and "holds no GITLAB_TOKEN" in got["stdout"], got["stdout"])
        self.assertEqual(got["calls"], [])

    def test_an_env_file_that_is_not_there_asks_nothing(self):
        got = drive("tested_gitlab_version", "http://localhost:8929", "/nonexistent/.env.docker")
        self.assertEqual(got["status"], "status=0")
        self.assertEqual(got["stdout"], "unknown (/nonexistent/.env.docker holds no GITLAB_TOKEN to ask with)")
        self.assertEqual(got["calls"], [])


class RunnerWiringTest(unittest.TestCase):
    """The lifecycle script refreshes both images before compose starts
    anything, and after the previous stack is down, since a container of that
    stack would otherwise hold the image a pull replaces."""

    def test_both_images_are_refreshed_between_the_cleanup_and_the_first_up(self):
        with open(RUNNER, encoding="utf-8") as handle:
            text = handle.read()
        cleanup = text.index('"${DOWN[@]}" 2>/dev/null || true')
        gitlab = text.index('refresh_image "${GITLAB_IMAGE}"')
        runner = text.index('refresh_image "${GITLAB_RUNNER_IMAGE}"')
        first_up = text.index(" up -d")
        self.assertLess(cleanup, gitlab)
        self.assertLess(gitlab, first_up)
        self.assertLess(runner, first_up)
        self.assertIn('. "${SCRIPT_DIR}/docker-images.sh"', text)

    def test_the_compose_file_takes_the_runner_image_the_script_checked(self):
        with open(COMPOSE, encoding="utf-8") as handle:
            compose = handle.read()
        with open(RUNNER, encoding="utf-8") as handle:
            runner = handle.read()
        self.assertIn("image: ${GITLAB_RUNNER_IMAGE:-gitlab/gitlab-runner:latest}", compose)
        self.assertIn('export GITLAB_RUNNER_IMAGE="${GITLAB_RUNNER_IMAGE:-gitlab/gitlab-runner:latest}"', runner)


if __name__ == "__main__":
    unittest.main()
