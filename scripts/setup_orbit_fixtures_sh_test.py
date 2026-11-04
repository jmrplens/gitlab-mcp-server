#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests what scripts/setup-orbit-fixtures.sh asks GitLab for.

The script provisions the Orbit live test fixtures on GitLab.com, and every
resource but a project and a label goes through ensure_resource: a GET on the
collection with a query that finds an existing one, and a POST to the same
collection when it finds none. The POST used to go to the parent of the
collection, so the milestone, the five issues and the two environments of
kg-fixtures were never created, and the script said it had made them, because
a POST answered without an id was printed as created. Both halves are tested
here: what each POST is sent to, and that a POST answered without an id stops
the script instead of passing. So is a namespace nested in a subgroup, the
shape of the default jmrp-oss/orbit-fixtures: the project lookup sent the
namespace's own slash unencoded, which GitLab answers 404, so the script tried
to create a project that already existed and stopped when GitLab refused it.
So is --help, which prints the script's usage comment and asks GitLab for
nothing.

Each case runs the real script with a stand-in curl first on PATH. It logs
every request as its method and path, answers each from a table of routes
keyed by method and path (of the query string only page= counts), and answers
anything else as GitLab answers a route it does not have. The stand-in is a shell
script rather than Python because the script under test already starts
python3 several times per resource, and on a loaded host each start costs
seconds.

Run with:

    python3 -m unittest discover -s scripts -p 'setup_orbit_fixtures_sh_test.py'
"""

import base64
import json
import os
import shutil
import subprocess
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "setup-orbit-fixtures.sh")
# An absolute path: on Windows, CreateProcess searches System32 before PATH and
# would pick the WSL launcher called bash.exe, which runs no distro here.
BASH = shutil.which("bash") or "bash"
API = "/api/v4"
KG_ID = 10
SEC_ID = 20

# The routes live one file each under CURL_ROUTES, named by route_file.
CURL = r"""#!/usr/bin/env bash
method=GET url= want_status=0 body= page= query=
while [ $# -gt 0 ]; do
  case "$1" in
    -X) method=$2; shift 2 ;;
    -H) shift 2 ;;
    -d) body=$2; shift 2 ;;
    -w) want_status=1; shift 2 ;;
    --connect-timeout|--max-time) shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
path=${url#*://}
path=/${path#*/}
case "$path" in
  *\?*) query=${path#*\?}; path=${path%%\?*} ;;
esac
case "$query" in
  *page=*) page=${query##*page=}; page=${page%%&*} ;;
esac
printf '%s %s\n' "$method" "$path" >> "$CURL_LOG"
[ -n "$body" ] && printf '%s %s\t%s\n' "$method" "$path" "$body" >> "$CURL_BODY_LOG"
[ -n "$query" ] && printf '%s %s\n' "$path" "$query" >> "$CURL_LOG.query"
key="$method $path"
key=${key// /_}
key=${key//\//!}
[ -f "$CURL_ROUTES/$key.dead" ] && exit 126
status=404
answer="$CURL_ROUTES/$key"
[ -n "$page" ] && [ -f "$CURL_ROUTES/$key@$page" ] && answer="$CURL_ROUTES/$key@$page"
if [ -f "$answer" ]; then
  cat "$answer"
  status=200
  [ -f "$answer.status" ] && status=$(cat "$answer.status")
else
  printf '%s' '{"message":"404 Not Found"}'
fi
[ "$want_status" = 1 ] && printf '\n%s' "$status"
exit 0
"""


def route_file(key):
    """The file name the stand-in curl reads the answer to a route from."""
    return key.replace(" ", "_").replace("/", "!")


def _fixture_files(project):
    """The files git tracks in a project's fixture directory, relative and POSIX-slash."""
    base = os.path.join(ROOT, "test", "fixtures", "orbit", project)
    listed = subprocess.run(["git", "ls-files", "-z"], cwd=base, check=True, stdout=subprocess.PIPE).stdout
    return sorted(rel for rel in listed.decode().split("\0") if rel)


def existing_fixtures(namespace="plens1"):
    """The routes of a namespace whose two projects and seven labels exist and nothing else.

    GitLab finds a group or a project by its full path with every slash in it
    percent-encoded, a nested namespace's own slashes included, so those are
    the only paths the routes answer. Each project is public and holds every
    fixture file, so a re-run finds it in line and writes nothing (#1533).
    """
    encoded = namespace.replace("/", "%2F")
    labels = [
        {"id": n, "name": name}
        for n, name in enumerate(
            ["bug", "enhancement", "documentation", "good first issue", "ci", "security", "fixture"], 1
        )
    ]
    return {
        "GET " + API + "/groups/" + encoded: {"id": 1},
        "GET " + API + "/projects/" + encoded + "%2Fkg-fixtures": {"id": KG_ID},
        "GET " + API + "/projects/" + encoded + "%2Fsecurity-fixtures": {"id": SEC_ID},
        "GET " + API + "/projects/%d" % KG_ID: {"id": KG_ID, "visibility": "public"},
        "GET " + API + "/projects/%d" % SEC_ID: {"id": SEC_ID, "visibility": "public"},
        "GET " + API + "/projects/%d/repository/tree" % KG_ID:
            [{"path": p} for p in _fixture_files("kg-fixtures")],
        "GET " + API + "/projects/%d/repository/tree" % SEC_ID:
            [{"path": p} for p in _fixture_files("security-fixtures")],
        "GET " + API + "/projects/%d/labels" % KG_ID: labels,
        "GET " + API + "/projects/%d/milestones" % KG_ID: [],
        "GET " + API + "/projects/%d/issues" % KG_ID: [],
        "GET " + API + "/projects/%d/environments" % KG_ID: [],
        "GET " + API + "/projects/%d/merge_requests" % KG_ID: [{"iid": 1}],
        "POST " + API + "/projects/%d/milestones" % KG_ID: {"id": 501},
        "POST " + API + "/projects/%d/issues" % KG_ID: {"id": 601},
        "POST " + API + "/projects/%d/environments" % KG_ID: {"id": 701},
    }


class SetupOrbitFixturesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)
        bindir = os.path.join(self.tmp, "bin")
        os.mkdir(bindir)
        curl = os.path.join(bindir, "curl")
        with open(curl, "w") as f:
            f.write(CURL)
        os.chmod(curl, 0o755)
        self.log = os.path.join(self.tmp, "curl.log")
        self.body_log = os.path.join(self.tmp, "curl-body.log")
        self.routes = os.path.join(self.tmp, "routes")
        self.env = dict(os.environ)
        self.env.update(
            {
                "PATH": bindir + os.pathsep + os.environ["PATH"],
                "CURL_LOG": self.log,
                "CURL_BODY_LOG": self.body_log,
                "CURL_ROUTES": self.routes,
                "GITLAB_COM_TOKEN": "stand-in-token",
                "ORBIT_FIXTURES_NAMESPACE": "plens1",
                "ORBIT_FIXTURES_GITLAB_URL": "https://gitlab.example.test",
            }
        )

    def run_script(self, routes, flag="--skip-mirror"):
        os.mkdir(self.routes)
        for key, answer in routes.items():
            status, body = answer if isinstance(answer, tuple) else (200, answer)
            with open(os.path.join(self.routes, route_file(key)), "w") as f:
                # A string goes out as it is, for a body that is not JSON.
                if isinstance(body, str):
                    f.write(body)
                else:
                    json.dump(body, f)
            if status is None:
                open(os.path.join(self.routes, route_file(key) + ".dead"), "w").close()
            if status != 200:
                with open(os.path.join(self.routes, route_file(key) + ".status"), "w") as f:
                    f.write(str(status))
        return subprocess.run(
            [BASH, SCRIPT, flag], env=self.env, capture_output=True, text=True, timeout=600
        )

    def committed_actions(self):
        """The file actions of the commits the script created, in order."""
        actions = []
        for row in self.request_bodies("POST"):
            if row["path"].endswith("/repository/commits"):
                actions.extend(json.loads(row["body"])["actions"])
        return actions

    def request_bodies(self, method):
        """(path, body) for every request the script sent with method and a body."""
        if not os.path.exists(self.body_log):
            return []
        rows = []
        with open(self.body_log) as f:
            for line in f:
                head, _, body = line.rstrip(chr(10)).partition(chr(9))
                if head.startswith(method + " "):
                    rows.append({"path": head.split(" ", 1)[1], "body": body})
        return rows

    def requests(self, method):
        """The paths of every request the script sent with method, in order."""
        if not os.path.exists(self.log):
            return []
        with open(self.log) as f:
            rows = [line.rstrip("\n").split(" ", 1) for line in f]
        return [{"method": m, "path": p} for m, p in rows if m == method]

    def test_each_missing_resource_is_posted_to_its_own_collection(self):
        result = self.run_script(existing_fixtures())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        posted = [r["path"] for r in self.requests("POST")]
        self.assertEqual(posted.count(API + "/projects/%d/milestones" % KG_ID), 1, posted)
        self.assertEqual(posted.count(API + "/projects/%d/issues" % KG_ID), 5, posted)
        self.assertEqual(posted.count(API + "/projects/%d/environments" % KG_ID), 2, posted)
        self.assertNotIn(API + "/projects/%d" % KG_ID, posted)
        self.assertIn("milestone id=501", result.stdout)

    def test_existing_resources_are_not_posted_again(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/milestones" % KG_ID] = [{"id": 500}]
        routes["GET " + API + "/projects/%d/issues" % KG_ID] = [{"id": 600}]
        routes["GET " + API + "/projects/%d/environments" % KG_ID] = [{"id": 700}]
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.requests("POST"), [])
        self.assertIn("milestone id=500", result.stdout)

    def test_a_post_answered_without_an_id_stops_the_script(self):
        routes = existing_fixtures()
        routes["POST " + API + "/projects/%d/milestones" % KG_ID] = {"message": "403 Forbidden"}
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("POST /projects/%d/milestones" % KG_ID, result.stderr)
        self.assertIn("403 Forbidden", result.stderr)
        self.assertNotIn("milestone id=", result.stdout)
        posted = [r["path"] for r in self.requests("POST")]
        self.assertNotIn(API + "/projects/%d/issues" % KG_ID, posted)

    def test_a_nested_namespace_finds_its_existing_projects(self):
        namespace = "jmrp-oss/orbit-fixtures"
        self.env["ORBIT_FIXTURES_NAMESPACE"] = namespace
        routes = existing_fixtures(namespace)
        routes["GET " + API + "/projects/%d/milestones" % KG_ID] = [{"id": 500}]
        routes["GET " + API + "/projects/%d/issues" % KG_ID] = [{"id": 600}]
        routes["GET " + API + "/projects/%d/environments" % KG_ID] = [{"id": 700}]
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        looked_up = [r["path"] for r in self.requests("GET")]
        self.assertIn(API + "/groups/jmrp-oss%2Forbit-fixtures", looked_up)
        self.assertIn(API + "/projects/jmrp-oss%2Forbit-fixtures%2Fkg-fixtures", looked_up)
        self.assertIn(API + "/projects/jmrp-oss%2Forbit-fixtures%2Fsecurity-fixtures", looked_up)
        self.assertEqual(self.requests("POST"), [])
        self.assertIn("project exists: jmrp-oss/orbit-fixtures/kg-fixtures (id=%d)" % KG_ID, result.stderr)

    def test_help_prints_the_whole_usage_comment(self):
        result = subprocess.run([BASH, SCRIPT, "--help"], env=self.env, capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        lines = result.stdout.splitlines()
        self.assertTrue(lines[0].startswith("# setup-orbit-fixtures.sh "), lines[:3])
        self.assertTrue(lines[-1].endswith("Re-runs are safe."), lines[-3:])
        self.assertTrue(all(line.startswith("#") for line in lines), lines)
        self.assertNotIn("Copyright", result.stdout)
        self.assertNotIn("SPDX-License-Identifier", result.stdout)
        self.assertEqual(self.requests("GET"), [])


    def test_an_existing_project_without_the_license_gets_it_first(self):
        routes = existing_fixtures()
        files = [{"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"]
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = files
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([a["file_path"] for a in self.committed_actions()], ["LICENSE"])
        self.assertIn("added LICENSE", result.stderr)

    def test_a_failed_first_push_is_repaired_in_one_commit(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [{"path": "README.md"}]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        commits = [json.loads(r["body"]) for r in self.request_bodies("POST") if r["path"].endswith("/repository/commits")]
        self.assertEqual(len(commits), 1, "a commit per file would start a pipeline per file")
        self.assertEqual(commits[0]["branch"], "main")
        actions = commits[0]["actions"]
        self.assertEqual(sorted(a["file_path"] for a in actions), _fixture_files("kg-fixtures"))
        self.assertEqual(actions[0]["file_path"], "LICENSE")
        kinds = {a["file_path"]: a["action"] for a in actions}
        self.assertEqual(kinds.pop("README.md"), "update")
        self.assertEqual(set(kinds.values()), {"create"})
        base = os.path.join(ROOT, "test", "fixtures", "orbit", "kg-fixtures")
        for action in actions:
            self.assertEqual(action["encoding"], "base64")
            with open(os.path.join(base, action["file_path"]), "rb") as f:
                self.assertEqual(base64.b64decode(action["content"]), f.read(), action["file_path"])
        self.assertIn("replaced README.md", result.stderr)

    def test_a_readme_beside_other_files_is_left_alone(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"
        ]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([a["file_path"] for a in self.committed_actions()], ["LICENSE"])

    def test_a_file_git_does_not_track_is_not_created(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"
        ]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        # Not ".env": a developer's own would be overwritten and then deleted.
        planted = os.path.join(ROOT, "test", "fixtures", "orbit", "kg-fixtures", ".env.setup-orbit-fixtures-test")
        with open(planted, "w") as f:
            f.write("SECRET=stand-in\n")
        self.addCleanup(os.remove, planted)
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([a["file_path"] for a in self.committed_actions()], ["LICENSE"])

    def test_a_file_list_git_cannot_give_stops_reconciliation(self):
        # Names no repository, so git answers as it does outside a checkout.
        self.env["GIT_DIR"] = os.path.join(self.tmp, "no-such-repository")
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"
        ]
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("cannot compare", result.stderr)
        self.assertNotIn("project files are in line", result.stderr)

    def test_a_tracked_file_missing_from_the_disk_is_not_created_empty(self):
        # An index of this test's own, so that git lists a file the disk does not hold.
        index = os.path.join(self.tmp, "index")
        subprocess.run(
            ["git", "update-index", "--add", "--cacheinfo",
             "100644,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,test/fixtures/orbit/kg-fixtures/no-such-file"],
            cwd=ROOT, env=dict(os.environ, GIT_INDEX_FILE=index), check=True,
        )
        self.env["GIT_INDEX_FILE"] = index
        routes = existing_fixtures()
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("cannot compare", result.stderr)
        self.assertEqual(self.committed_actions(), [])

    def test_a_tree_answered_with_no_list_stops_reconciliation(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = {"message": "not a list"}
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("file list is unreadable", result.stderr)
        self.assertEqual(self.requests("POST"), [])

    def test_a_tree_answered_with_no_json_stops_reconciliation(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = "<html>not json</html>"
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("file list is unreadable", result.stderr)
        self.assertEqual(self.requests("POST"), [])

    def test_an_existing_mirror_takes_the_visibility_and_no_file_claim(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/plens1%2Fglab-mirror"] = {"id": 30}
        routes["GET " + API + "/projects/30"] = {"id": 30, "visibility": "private"}
        routes["PUT " + API + "/projects/30"] = {"id": 30, "visibility": "public"}
        result = self.run_script(routes, "--mirror-cli")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([r["path"] for r in self.requests("PUT")], [API + "/projects/30"])
        read = [r["path"] for r in self.requests("GET")]
        self.assertNotIn(API + "/projects/30/repository/tree", read)
        # Said of kg-fixtures and security-fixtures, never of the mirror.
        self.assertEqual(result.stderr.count("project files are in line"), 2, result.stderr)

    def test_a_write_curl_wrote_no_status_for_stops_reconciliation(self):
        # A status of None makes the stand-in exit as a curl that never started
        # does, with an argument over the kernel's limit: no output at all.
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"
        ]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = (None, {})
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("added LICENSE", result.stderr)

    def test_a_visibility_update_curl_wrote_no_status_for_stops_reconciliation(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "private"}
        routes["PUT " + API + "/projects/%d" % KG_ID] = (None, {})
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("visibility private → public", result.stderr)

    def test_the_tree_is_read_from_the_branch_written_to(self):
        result = self.run_script(existing_fixtures())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        with open(self.log + ".query") as f:
            trees = [line for line in f if "/repository/tree " in line]
        self.assertTrue(trees)
        for line in trees:
            self.assertIn("ref=main", line)

    def test_a_directory_does_not_stand_in_for_a_file(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"
        ] + [{"path": "LICENSE", "type": "tree"}]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([a["file_path"] for a in self.committed_actions()], ["LICENSE"])

    def test_a_refused_file_creation_leaves_a_private_project_private(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "private"}
        routes["PUT " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "public"}
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"
        ]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = (400, {"message": "bad request"})
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.requests("PUT"), [])
        self.assertNotIn("visibility private → public", result.stderr)

    def test_an_existing_private_project_is_made_public(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "private"}
        routes["PUT " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "public"}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        updated = [r["path"] for r in self.requests("PUT")]
        self.assertEqual(updated, [API + "/projects/%d" % KG_ID])
        self.assertIn("visibility private → public", result.stderr)
        payload = json.loads(self.request_bodies("PUT")[-1]["body"])
        self.assertEqual(payload, {"visibility": "public"})

    def test_reads_every_tree_page(self):
        routes = existing_fixtures()
        all_files = [{"path": p} for p in _fixture_files("kg-fixtures")]
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": "filler/%d" % i} for i in range(100)
        ]
        routes["GET " + API + "/projects/%d/repository/tree@2" % KG_ID] = all_files
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.committed_actions(), [], "files on the second page must count as present")

    def test_the_license_is_created_before_another_missing_file(self):
        routes = existing_fixtures()
        files = _fixture_files("kg-fixtures")
        others = [p for p in files if p != "LICENSE"]
        missing = ["LICENSE", others[0]]
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = [
            {"path": p} for p in files if p not in missing
        ]
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = {"id": "c0"}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([a["file_path"] for a in self.committed_actions()], missing)

    def test_unreadable_project_details_stop_reconciliation(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = (500, {"message": "boom"})
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("cannot read project", result.stderr)
        self.assertEqual(self.requests("PUT"), [], "no visibility may be written from an unreadable project")

    def test_a_project_without_a_visibility_is_not_made_public(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = {"id": KG_ID}
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("no readable visibility", result.stderr)
        self.assertEqual(self.requests("PUT"), [])

    def test_a_refused_visibility_update_stops_reconciliation(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "private"}
        routes["PUT " + API + "/projects/%d" % KG_ID] = (403, {"message": "forbidden"})
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("could not set visibility", result.stderr)
        self.assertNotIn("visibility private → public", result.stderr)

    def test_an_unreadable_tree_stops_reconciliation(self):
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = (500, {"message": "boom"})
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("cannot list the project's files", result.stderr)
        self.assertEqual(self.committed_actions(), [])

    def test_a_refused_file_creation_stops_reconciliation(self):
        routes = existing_fixtures()
        files = [{"path": p} for p in _fixture_files("kg-fixtures") if p != "LICENSE"]
        routes["GET " + API + "/projects/%d/repository/tree" % KG_ID] = files
        routes["POST " + API + "/projects/%d/repository/commits" % KG_ID] = (
            400,
            {"message": "bad request"},
        )
        result = self.run_script(routes)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("could not add the missing fixture files", result.stderr)
        self.assertNotIn("added LICENSE", result.stderr)

    def test_a_private_visibility_is_refused_under_jmrp_oss(self):
        self.env["ORBIT_FIXTURES_NAMESPACE"] = "jmrp-oss/orbit-fixtures"
        self.env["ORBIT_FIXTURES_VISIBILITY"] = "private"
        result = self.run_script(existing_fixtures("jmrp-oss/orbit-fixtures"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ORBIT_FIXTURES_VISIBILITY=private is not allowed", result.stderr)
        self.assertFalse(os.path.exists(self.log), "the refusal must come before any API call")

    def test_a_private_visibility_is_refused_under_an_uppercase_jmrp_oss(self):
        # GitLab resolves group and project paths case-insensitively, so a
        # differently cased namespace reaches the same projects and must not
        # slip past the public-only guard.
        self.env["ORBIT_FIXTURES_NAMESPACE"] = "JMRP-OSS/Orbit-Fixtures"
        self.env["ORBIT_FIXTURES_VISIBILITY"] = "private"
        result = self.run_script(existing_fixtures("jmrp-oss/orbit-fixtures"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ORBIT_FIXTURES_VISIBILITY=private is not allowed", result.stderr)
        self.assertFalse(os.path.exists(self.log), "the refusal must come before any API call")

    def test_the_visibility_is_json_encoded_in_the_update_body(self):
        # A configured visibility carrying JSON syntax must stay one string in
        # the PUT body and add no field of its own.
        visibility = 'private", "issues_access_level": "disabled'
        self.env["ORBIT_FIXTURES_VISIBILITY"] = visibility
        routes = existing_fixtures()
        routes["GET " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": "public"}
        routes["PUT " + API + "/projects/%d" % KG_ID] = {"id": KG_ID, "visibility": visibility}
        routes["PUT " + API + "/projects/%d" % SEC_ID] = {"id": SEC_ID, "visibility": visibility}
        result = self.run_script(routes)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        updated = [r["path"] for r in self.requests("PUT")]
        self.assertEqual(
            updated,
            [API + "/projects/%d" % KG_ID, API + "/projects/%d" % SEC_ID],
        )
        for row in self.request_bodies("PUT"):
            self.assertEqual(json.loads(row["body"]), {"visibility": visibility})


if __name__ == "__main__":
    unittest.main()
