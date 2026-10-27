#!/usr/bin/env python3
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

"""Tests scripts/publish-pypi.sh against a stand-in python3, curl, dirname and venv.

Release.yml's pypi job published through pypa/gh-action-pypi-publish, whose
generated container action runs an image pypa names by a registry tag, inside
a job holding the id-token: write PyPI's trusted publishing exchanges (issue
1243). The job now runs this script with --trusted-publishing instead, so the
script is what holds the properties the action held, and these cases hold it
to each of them:

  * the tools are installed from .github/requirements/pypi-publish.txt in
    --require-hashes mode, from wheels only, and twine check --strict runs
    with them before anything is uploaded;
  * the upload token is minted the way PyPI documents trusted publishing by
    hand: the audience pypi.org publishes, the job's OIDC token for it, and a
    POST of that token to pypi.org's mint-token endpoint;
  * every wheel is uploaded with a PEP 740 publish attestation, signed first,
    so a signing failure mints nothing;
  * twine uploads with --skip-existing, so a re-run of the job lands on the
    files PyPI already holds without failing;
  * each credential reaches the one process that needs it: the minted token
    twine alone, the job's token request credential the identity request and
    the signer alone, and none of them is ever a command-line argument; the
    names that carry them are dropped before the script's first child starts;
  * every request is made over https alone, within a time limit, and fails on
    an HTTP error with the answer still written out;
  * a rehearsal (--dry-run) installs and checks, and stops before the mint:
    nothing is signed and nothing is uploaded;
  * a refusal anywhere on the way to the upload stops the run with PyPI's own
    reasons, before twine is started.

They run a copy of the script in a scratch tree holding the requirements file
and a wheelhouse. A stand-in python3 first on PATH makes the venv out of
stand-ins and hands every other call to the real interpreter, a stand-in curl
answers the three requests as GitHub and PyPI do, and a stand-in dirname
answers the script's lookup of its own directory, which is the first child
the script starts. Every stand-in records its arguments, its environment and,
for curl, what it read on its standard input, which is what the cases judge.

Run with:

    python3 -m unittest discover -s scripts -p 'publish_pypi_sh_test.py'
"""

import ast
import base64
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
SCRIPT = os.path.join(ROOT, "scripts", "publish-pypi.sh")
REQUIREMENTS = os.path.join(ROOT, ".github", "requirements", "pypi-publish.txt")
VERSION = "9.8.7"
DIST = "jmrplens_gitlab_mcp_server"
TAGS = [
    "manylinux_2_17_x86_64.manylinux2014_x86_64",
    "manylinux_2_17_aarch64.manylinux2014_aarch64",
    "macosx_11_0_x86_64",
    "macosx_11_0_arm64",
    "win_amd64",
    "win_arm64",
]

AUDIENCE_URL = "https://pypi.org/_/oidc/audience"
MINT_URL = "https://pypi.org/_/oidc/mint-token"
UPLOAD_URL = "https://upload.pypi.org/legacy/"
IDENTITY = "https://token.actions.example/idtoken"
REQUEST_URL = IDENTITY + "?api-version=2.0"
REQUEST_TOKEN = "request-credential-of-the-job"
# The job's OIDC token has the shape of the JWT it stands for, a header, a
# payload and a signature, each base64url without padding and joined by dots,
# which is what publish-pypi.sh holds the token it is handed to. It is built
# here rather than written out, so the source carries no literal a secret
# scanner reads as a token.
OIDC_TOKEN = ".".join(
    base64.urlsafe_b64encode(part).rstrip(b"=").decode("ascii")
    for part in (b'{"alg":"RS256"}', b"oidc-for-pypi", b"signature")
)
MINTED = "pypi-minted-upload-token"
STORED = "pypi-stored-api-token"
SECRETS = (REQUEST_TOKEN, OIDC_TOKEN, MINTED, STORED)

# One program for every stand-in, which reads its role off the name it was
# run by. The fake credentials are written into it rather than handed to it
# through the environment, so the environment each stand-in records carries
# only what the script passed on.
STANDIN = r'''#!{python} -I
import json
import os
import shutil
import sys

REAL = {python!r}
ROLE = os.path.basename(sys.argv[0])
ARGS = sys.argv[1:]
IDENTITY = {identity!r}
REQUEST_TOKEN = {request_token!r}
OIDC_TOKEN = {oidc_token!r}
MINTED = {minted!r}


def reads_stdin():
    """Whether curl reads its standard input, as --config - and @- make it."""
    if ROLE != "curl":
        return False
    return "@-" in ARGS or any(a == "--config" and ARGS[i + 1:i + 2] == ["-"] for i, a in enumerate(ARGS))


STDIN = sys.stdin.read() if reads_stdin() else ""
with open(os.environ["PYPI_CALLS"], "a", encoding="utf-8") as log:
    log.write(json.dumps({{"tool": ROLE, "argv": ARGS, "env": dict(os.environ), "stdin": STDIN}}) + "\n")


def answer(body, status):
    sys.stdout.write(body)
    sys.exit(22 if status >= 400 and "--fail-with-body" in ARGS else 0)


if ROLE == "dirname":
    # GNU dirname's answer for the one path the script hands it.
    print(os.path.dirname(ARGS[-1].rstrip("/") or "/") or ".")
    sys.exit(0)

if ROLE == "python3":
    if ARGS[:2] == ["-m", "venv"]:
        os.makedirs(os.path.join(ARGS[2], "bin"))
        shutil.copy(__file__, os.path.join(ARGS[2], "bin", "python"))
        sys.exit(0)
    os.execv(REAL, [REAL] + ARGS)

if ROLE == "python":
    command = ARGS[:3]
    if command == ["-m", "pip", "install"]:
        sys.exit(0)
    if command == ["-m", "twine", "check"]:
        sys.exit(0)
    if command == ["-m", "pypi_attestations", "sign"]:
        if os.environ.get("STANDIN_SIGN_EXIT"):
            sys.exit(int(os.environ["STANDIN_SIGN_EXIT"]))
        if os.environ.get("STANDIN_SIGN_WRITES") == "nothing":
            sys.exit(0)
        for path in ARGS[3:]:
            with open(path + ".publish.attestation", "w", encoding="utf-8") as fh:
                fh.write("{{}}")
        sys.exit(0)
    if command == ["-m", "twine", "upload"]:
        sys.exit(0)
    sys.exit("stand-in python: unexpected " + " ".join(ARGS))

if ROLE == "curl":
    url = next((a for a in ARGS if a.startswith("https://")), "")
    if url == "https://pypi.org/_/oidc/audience":
        answer(os.environ.get("STANDIN_AUDIENCE_BODY", '{{"audience": "pypi"}}'),
               int(os.environ.get("STANDIN_AUDIENCE_STATUS", "200")))
    if url.split("?", 1)[0] == IDENTITY:
        if 'header = "Authorization: bearer ' + REQUEST_TOKEN + '"' not in STDIN:
            answer('{{"message": "Bad credentials"}}', 401)
        answer(os.environ.get("STANDIN_OIDC_BODY", json.dumps({{"count": 1, "value": OIDC_TOKEN}})), 200)
    if url == "https://pypi.org/_/oidc/mint-token":
        try:
            body = json.loads(STDIN)
        except ValueError:
            body = None
        if body != {{"token": OIDC_TOKEN}}:
            answer('{{"message": "Token request failed", "errors": [{{"code": "invalid-payload", '
                   '"description": "the request carries no token"}}]}}', 422)
        answer(os.environ.get("STANDIN_MINT_BODY", json.dumps({{"success": True, "token": MINTED}})),
               int(os.environ.get("STANDIN_MINT_STATUS", "200")))
    sys.stderr.write("curl: (6) Could not resolve host\n")
    sys.exit(6)

sys.exit("stand-in: unknown role " + ROLE)
'''

BUILD = """import argparse
import os

parser = argparse.ArgumentParser()
parser.add_argument("--binaries", required=True)
parser.add_argument("--version", required=True)
parser.add_argument("--out", required=True)
args = parser.parse_args()
os.makedirs(args.out, exist_ok=True)
for tag in {tags!r}:
    with open(os.path.join(args.out, "{dist}-" + args.version + "-py3-none-" + tag + ".whl"), "wb") as fh:
        fh.write(b"a wheel")
"""

VALIDATE = """import os
import sys

sys.exit(int(os.environ.get("STANDIN_VALIDATE_EXIT", "0")))
"""

# What the job's environment may carry that the script reads, cleared from
# every run so that a case passes on a runner whose own job sets one.
INHERITED = ("GITHUB_ACTIONS", "ACTIONS_ID_TOKEN_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_TOKEN", "PYPI_TOKEN",
             "TWINE_USERNAME", "TWINE_PASSWORD", "TWINE_REPOSITORY_URL")


class PublishPypiTest(unittest.TestCase):
    """Runs a copy of the real script against stand-ins that record every call."""

    def setUp(self):
        if shutil.which("bash") is None:
            # GitHub sets CI on every step; there a missing tool is a failure.
            if os.environ.get("CI"):
                self.fail("the script needs bash, and CI must run these cases rather than skip them")
            self.skipTest("the script needs bash")
        self.work = tempfile.mkdtemp(prefix="publish-pypi-")
        self.addCleanup(shutil.rmtree, self.work, True)
        os.makedirs(os.path.join(self.work, "scripts"))
        shutil.copy(SCRIPT, os.path.join(self.work, "scripts", "publish-pypi.sh"))
        with open(os.path.join(self.work, "scripts", "build_pypi.py"), "w", encoding="utf-8") as fh:
            fh.write(BUILD.format(tags=TAGS, dist=DIST))
        with open(os.path.join(self.work, "scripts", "validate_pypi.py"), "w", encoding="utf-8") as fh:
            fh.write(VALIDATE)
        self.requirements = os.path.join(self.work, ".github", "requirements", "pypi-publish.txt")
        os.makedirs(os.path.dirname(self.requirements))
        shutil.copy(REQUIREMENTS, self.requirements)
        self.wheelhouse = os.path.join(self.work, "pypi", "dist")
        bin_dir = os.path.join(self.work, "bin")
        os.makedirs(bin_dir)
        program = STANDIN.format(python=sys.executable, identity=IDENTITY, request_token=REQUEST_TOKEN,
                                 oidc_token=OIDC_TOKEN, minted=MINTED)
        for role in ("python3", "curl", "dirname"):
            path = os.path.join(bin_dir, role)
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(program)
            os.chmod(path, 0o755)
        temporary = os.path.join(self.work, "tmp")
        os.makedirs(temporary)
        self.calls_log = os.path.join(self.work, "calls.jsonl")
        self.env = {name: value for name, value in os.environ.items() if name not in INHERITED}
        self.env.update(PATH=bin_dir + os.pathsep + os.environ.get("PATH", ""), PYPI_CALLS=self.calls_log,
                        TMPDIR=temporary)

    def write_wheels(self, version=VERSION, extra=()):
        os.makedirs(self.wheelhouse, exist_ok=True)
        for name in [f"{DIST}-{version}-py3-none-{tag}.whl" for tag in TAGS] + list(extra):
            with open(os.path.join(self.wheelhouse, name), "wb") as fh:
                fh.write(b"a wheel")

    def wheels(self):
        return sorted(os.path.join(self.wheelhouse, f"{DIST}-{VERSION}-py3-none-{tag}.whl") for tag in TAGS)

    def start_over(self):
        """Clears the record and the wheelhouse, for the next case of a table."""
        if os.path.exists(self.calls_log):
            os.remove(self.calls_log)
        shutil.rmtree(self.wheelhouse, ignore_errors=True)

    def trusted(self, **extra):
        """The environment of a job holding id-token: write, with extra over it."""
        env = {"ACTIONS_ID_TOKEN_REQUEST_URL": REQUEST_URL, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": REQUEST_TOKEN}
        env.update(extra)
        return env

    def run_script(self, *args, env=None):
        return subprocess.run(
            ["bash", os.path.join(self.work, "scripts", "publish-pypi.sh"), "dist", VERSION, *args],
            cwd=self.work, env=dict(self.env, **(env or {})), stdin=subprocess.DEVNULL,
            capture_output=True, text=True, timeout=120, check=False,
        )

    def recorded(self):
        """Every call a stand-in recorded, in the order the script made them."""
        if not os.path.exists(self.calls_log):
            return []
        with open(self.calls_log, encoding="utf-8") as fh:
            return [json.loads(line) for line in fh]

    def calls(self):
        """The recorded calls but the script's lookup of its own directory."""
        return [call for call in self.recorded() if call["tool"] != "dirname"]

    @staticmethod
    def label(call):
        """One short label for a call: the tool, and what it was asked to do."""
        argv = call["argv"]
        if call["tool"] == "curl":
            return "curl " + next(a for a in argv if a.startswith("https://")).split("?")[0]
        if call["tool"] == "python":
            return " ".join(argv[1:3])
        if call["tool"] == "dirname":
            return "dirname"
        if argv[:1] == ["-c"]:
            return "python3 -c"
        return "python3 " + (" ".join(argv[:2]) if argv[:1] == ["-m"] else os.path.basename(argv[0]))

    def steps(self):
        """Each call but the interpreter's own JSON reads, as one short label."""
        return [self.label(call) for call in self.non_json_calls()]

    def call(self, label):
        found = [call for call in self.non_json_calls() if self.label(call) == label]
        self.assertEqual(len(found), 1, f"{label} ran {len(found)} times: {self.steps()}")
        return found[0]

    def non_json_calls(self):
        return [call for call in self.calls() if self.label(call) != "python3 -c"]

    def holders(self, secret):
        """Every recorded call whose environment carries secret, by label.

        The interpreter's JSON reads and the script's lookup of its own
        directory count too: a child of either kind inherits whatever the
        script still exports when it starts.
        """
        return [self.label(call) for call in self.recorded() if secret in json.dumps(call["env"])]

    def readers(self, secret):
        """The URLs of the curl calls that read secret on their standard input."""
        return [next(a for a in call["argv"] if a.startswith("https://")) for call in self.calls()
                if secret in call["stdin"]]

    def test_trusted_publishing_attests_each_wheel_then_uploads_with_a_token_minted_from_the_jobs_identity(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.steps(), [
            "python3 -m venv",
            "pip install",
            "twine check",
            "pypi_attestations sign",
            "curl " + AUDIENCE_URL,
            "curl https://token.actions.example/idtoken",
            "curl " + MINT_URL,
            "twine upload",
        ])

        install = self.call("pip install")["argv"]
        self.assertIn("--require-hashes", install)
        self.assertIn(["--only-binary", ":all:"], [install[i:i + 2] for i in range(len(install))])
        self.assertEqual(install[install.index("-r") + 1], self.requirements)
        self.assertEqual(self.call("twine check")["argv"][3:], ["--strict"] + self.wheels())
        self.assertEqual(sorted(self.call("pypi_attestations sign")["argv"][3:]), self.wheels())

        identity = self.call("curl https://token.actions.example/idtoken")
        self.assertIn(REQUEST_URL + "&audience=pypi", identity["argv"], "the audience is the one pypi.org publishes")
        mint = self.call("curl " + MINT_URL)
        self.assertEqual(json.loads(mint["stdin"]), {"token": OIDC_TOKEN})

        upload = self.call("twine upload")
        argv = upload["argv"][3:]
        for flag in ("--non-interactive", "--skip-existing", "--attestations"):
            self.assertIn(flag, argv)
        self.assertEqual(argv[argv.index("--repository-url") + 1], UPLOAD_URL)
        files = sorted(a for a in argv if a.startswith(self.wheelhouse))
        self.assertEqual(files, sorted(self.wheels() + [w + ".publish.attestation" for w in self.wheels()]))
        self.assertEqual(upload["env"].get("TWINE_USERNAME"), "__token__")
        self.assertEqual(upload["env"].get("TWINE_PASSWORD"), MINTED)
        self.assertIn(f"published version {VERSION}", result.stdout)

    def test_each_credential_reaches_the_one_process_that_needs_it_and_no_command_line(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        for call in self.recorded():
            for secret in SECRETS:
                with self.subTest(tool=call["tool"], argv=call["argv"][:3], secret=secret):
                    self.assertNotIn(secret, " ".join(call["argv"]), "a command line is readable by every process")
        self.assertEqual(self.holders(MINTED), ["twine upload"])
        self.assertEqual(self.holders(REQUEST_TOKEN), ["pypi_attestations sign"])
        self.assertEqual(self.holders(OIDC_TOKEN), [])
        signer = self.call("pypi_attestations sign")["env"]
        self.assertEqual(signer.get("ACTIONS_ID_TOKEN_REQUEST_URL"), REQUEST_URL)
        self.assertEqual(signer.get("ACTIONS_ID_TOKEN_REQUEST_TOKEN"), REQUEST_TOKEN)
        self.assertEqual(self.readers(REQUEST_TOKEN), [REQUEST_URL + "&audience=pypi"])
        self.assertEqual(self.readers(OIDC_TOKEN), [MINT_URL])

    def test_the_names_carrying_a_credential_are_dropped_before_the_first_child_starts(self):
        cases = [
            ("trusted publishing", ("--no-assemble", "--trusted-publishing"), self.trusted()),
            ("a stored token", ("--no-assemble",), {"PYPI_TOKEN": STORED}),
        ]
        for name, args, env in cases:
            with self.subTest(name):
                self.start_over()
                self.write_wheels()
                result = self.run_script(*args, env=env)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                first = self.recorded()[0]
                self.assertEqual(self.label(first), "dirname", "the script's first child finds its own directory")
                # The names alone, so that a failure prints no environment.
                inherited = [name for name in ("PYPI_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_URL",
                                               "ACTIONS_ID_TOKEN_REQUEST_TOKEN") if name in first["env"]]
                self.assertEqual(inherited, [], "the script's first child inherits them")

    def test_every_request_is_https_alone_bounded_in_time_and_fails_on_an_http_error(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        requests = [call for call in self.calls() if call["tool"] == "curl"]
        self.assertEqual(len(requests), 3, self.steps())
        for call in requests:
            with self.subTest(self.label(call)):
                argv = call["argv"]
                pairs = [argv[i:i + 2] for i in range(len(argv))]
                self.assertIn(["--proto", "=https"], pairs, "no other protocol is spoken")
                self.assertIn(["--max-time", "30"], pairs, "a request that hangs ends")
                self.assertIn("--fail-with-body", argv, "an HTTP error fails, with PyPI's reasons still written out")
                self.assertEqual({"-L", "--location"} & set(argv), set(), "no redirect is followed")

    def test_the_audience_is_the_query_of_a_request_url_that_carries_none(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing",
                                 env=self.trusted(ACTIONS_ID_TOKEN_REQUEST_URL=IDENTITY))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(IDENTITY + "?audience=pypi", self.call("curl " + IDENTITY)["argv"])
        self.assertEqual(self.call("pypi_attestations sign")["env"].get("ACTIONS_ID_TOKEN_REQUEST_URL"), IDENTITY)

    def test_on_github_actions_the_job_log_masks_both_tokens(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted(GITHUB_ACTIONS="true"))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("::add-mask::" + OIDC_TOKEN, result.stdout.splitlines())
        self.assertIn("::add-mask::" + MINTED, result.stdout.splitlines())

    def test_off_github_actions_no_token_is_printed(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        for secret in SECRETS:
            self.assertNotIn(secret, result.stdout + result.stderr)

    def test_a_refusal_on_the_way_to_the_upload_stops_the_run_before_twine_starts(self):
        refusal = json.dumps({"message": "Token request failed", "errors": [
            {"code": "invalid-publisher", "description": "valid token, but no corresponding publisher"}]})
        identity, mint = "curl " + IDENTITY, "curl " + MINT_URL
        # Each case with what the script must say, and the first step it must
        # not reach.
        cases = [
            ("the audience cannot be read", {"STANDIN_AUDIENCE_STATUS": "503"}, None, identity),
            ("the audience answer names none", {"STANDIN_AUDIENCE_BODY": '{"message": "gone"}'},
             "carries no audience", identity),
            ("the audience is not a plain word", {"STANDIN_AUDIENCE_BODY": '{"audience": "pypi&audience=x"}'},
             "is not an audience", identity),
            ("the job's token answer carries none", {"STANDIN_OIDC_BODY": '{"count": 0}'}, "carries no value", mint),
            ("the job's token is not a JWT", {"STANDIN_OIDC_BODY": '{"count": 1, "value": "a\\"b"}'},
             "is not a JWT", mint),
            ("pypi refuses the mint", {"STANDIN_MINT_STATUS": "422", "STANDIN_MINT_BODY": refusal},
             "invalid-publisher: valid token, but no corresponding publisher", "twine upload"),
            ("the mint answer carries no token", {"STANDIN_MINT_BODY": '{"success": true}'}, "carries no token",
             "twine upload"),
        ]
        for name, answers, said, unreached in cases:
            with self.subTest(name):
                self.start_over()
                self.write_wheels()
                result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted(**answers))
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertNotIn(unreached, self.steps())
                self.assertNotIn("twine upload", self.steps())
                if said is not None:
                    self.assertIn(said, result.stderr)

    def test_a_signing_failure_mints_nothing(self):
        cases = [
            ("the signer fails", {"STANDIN_SIGN_EXIT": "1"}, None),
            ("the signer writes no attestation", {"STANDIN_SIGN_WRITES": "nothing"}, "pypi-attestations wrote no"),
        ]
        for name, answers, said in cases:
            with self.subTest(name):
                self.start_over()
                self.write_wheels()
                result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted(**answers))
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual([step for step in self.steps() if step.startswith("curl")], [])
                self.assertNotIn("twine upload", self.steps())
                if said is not None:
                    self.assertIn(said, result.stderr)

    def test_a_rehearsal_installs_and_checks_and_stops_before_the_mint(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", "--dry-run", env=self.trusted())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.steps(), ["python3 -m venv", "pip install", "twine check"])
        self.assertIn("nothing was minted, signed or uploaded", result.stdout)
        self.assertEqual(sorted(os.listdir(self.wheelhouse)), sorted(os.path.basename(w) for w in self.wheels()))

    def test_trusted_publishing_without_the_jobs_identity_is_refused_before_anything_runs(self):
        self.write_wheels()
        for env in ({}, {"ACTIONS_ID_TOKEN_REQUEST_URL": REQUEST_URL},
                    {"ACTIONS_ID_TOKEN_REQUEST_TOKEN": REQUEST_TOKEN}):
            with self.subTest(sorted(env)):
                result = self.run_script("--no-assemble", "--trusted-publishing", "--dry-run", env=env)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn("id-token: write", result.stderr)
                self.assertEqual(self.calls(), [])

    def test_trusted_publishing_refuses_a_stored_token_beside_it(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", "--trusted-publishing", env=self.trusted(PYPI_TOKEN=STORED))
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn("PYPI_TOKEN is set", result.stderr)
        self.assertEqual(self.calls(), [])

    def test_a_stored_token_uploads_without_attestations_and_reaches_twine_alone(self):
        self.write_wheels()
        result = self.run_script("--no-assemble", env={"PYPI_TOKEN": STORED})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.steps(), ["python3 -m venv", "pip install", "twine check", "twine upload"])
        upload = self.call("twine upload")
        argv = upload["argv"][3:]
        self.assertNotIn("--attestations", argv)
        self.assertIn("--skip-existing", argv)
        self.assertEqual(argv[argv.index("--repository-url") + 1], UPLOAD_URL)
        self.assertEqual(sorted(a for a in argv if a.startswith(self.wheelhouse)), self.wheels())
        self.assertEqual(upload["env"].get("TWINE_PASSWORD"), STORED)
        self.assertEqual(self.holders(STORED), ["twine upload"])
        self.assertNotIn(STORED, " ".join(" ".join(call["argv"]) for call in self.calls()))

    def test_without_a_token_the_upload_is_refused_before_anything_runs(self):
        self.write_wheels()
        result = self.run_script("--no-assemble")
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("PYPI_TOKEN is not set", result.stderr)
        self.assertEqual(self.calls(), [])

    def test_without_no_assemble_the_wheelhouse_is_built_and_validated_first(self):
        result = self.run_script("--dry-run")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.steps(), ["python3 build_pypi.py", "python3 validate_pypi.py", "python3 -m venv",
                                        "pip install", "twine check"])
        build, validate = self.non_json_calls()[:2]
        self.assertEqual(build["argv"][1:], ["--binaries", "dist", "--version", VERSION, "--out", self.wheelhouse])
        self.assertEqual(validate["argv"][1:], ["--wheels", self.wheelhouse, "--version", VERSION])

    def test_a_wheelhouse_that_fails_validation_installs_nothing(self):
        result = self.run_script("--dry-run", env={"STANDIN_VALIDATE_EXIT": "1"})
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("python3 -m venv", self.steps())

    def test_no_assemble_refuses_a_wheelhouse_holding_anything_but_wheels_of_the_version(self):
        cases = [
            ("no wheelhouse", None, "does not exist"),
            ("an empty wheelhouse", [], "holds no wheel"),
            ("a wheel of another version", [f"{DIST}-9.8.6-py3-none-win_amd64.whl"], f"{DIST}-9.8.6-py3-none"),
            ("an attestation left behind", [f"{DIST}-{VERSION}-py3-none-win_amd64.whl.publish.attestation"],
             "publish.attestation"),
            ("a hidden file", [".partial"], ".partial"),
            ("a wheel of a tag build_pypi.py does not make", [f"{DIST}-{VERSION}-py3-none-linux_riscv64.whl"],
             "linux_riscv64"),
        ]
        for name, extra, said in cases:
            with self.subTest(name):
                self.start_over()
                if extra == []:
                    os.makedirs(self.wheelhouse)
                elif extra is not None:
                    self.write_wheels(extra=extra)
                result = self.run_script("--no-assemble", "--dry-run")
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn(said, result.stderr)
                self.assertEqual(self.calls(), [])

    def test_no_assemble_refuses_a_wheelhouse_missing_any_of_the_six_wheels(self):
        # build_pypi.py stops at the first release binary it cannot find and
        # leaves the wheels it built before it, with nothing else beside them.
        for tag in TAGS:
            with self.subTest(tag):
                self.start_over()
                self.write_wheels()
                missing = f"{DIST}-{VERSION}-py3-none-{tag}.whl"
                os.remove(os.path.join(self.wheelhouse, missing))
                result = self.run_script("--no-assemble", env={"PYPI_TOKEN": STORED})
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn(f"holds no wheel {missing} as a regular file", result.stderr)
                self.assertEqual(self.calls(), [])

    def test_no_assemble_refuses_a_wheel_that_is_not_a_regular_file(self):
        def directory(wheel):
            os.remove(wheel)
            os.makedirs(wheel)

        def symlink(wheel):
            elsewhere = os.path.join(self.work, "elsewhere.whl")
            os.replace(wheel, elsewhere)
            os.symlink(elsewhere, wheel)

        for name, replace in [("a directory", directory), ("a symbolic link", symlink)]:
            with self.subTest(name):
                self.start_over()
                self.write_wheels()
                wheel = self.wheels()[0]
                replace(wheel)
                result = self.run_script("--no-assemble", "--dry-run")
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn(f"holds no wheel {os.path.basename(wheel)} as a regular file", result.stderr)
                self.assertEqual(self.calls(), [])

    def test_the_platform_tags_are_the_six_validate_pypi_requires(self):
        with open(os.path.join(ROOT, "scripts", "validate_pypi.py"), encoding="utf-8") as fh:
            tree = ast.parse(fh.read())
        required = next(ast.literal_eval(node.value) for node in tree.body if isinstance(node, ast.Assign)
                        and any(getattr(target, "id", None) == "EXPECTED_TAGS" for target in node.targets))
        self.assertEqual(sorted(TAGS), sorted(required))

    def test_an_unknown_option_is_refused(self):
        result = self.run_script("--skip-existing")
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn("unknown option: --skip-existing", result.stderr)
        self.assertEqual(self.calls(), [])

    def test_shellcheck_finds_nothing(self):
        if shutil.which("shellcheck") is None:
            if os.environ.get("CI"):
                self.fail("shellcheck is missing, and CI must run this case rather than skip it")
            self.skipTest("shellcheck is not installed")
        result = subprocess.run(["shellcheck", SCRIPT], capture_output=True, text=True, timeout=120, check=False)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
