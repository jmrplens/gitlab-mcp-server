#!/usr/bin/env python3
"""Audit the release supply chain's configuration invariants.

Five properties, each of which was false at some point and each of which is
invisible to every other gate in this repository:

1.  Every ``uses:`` in ``.github/workflows`` is pinned to a 40-character commit
    SHA. A mutable tag is resolved by the runner at job start, so a hijacked
    ``v7`` is consumed with no pull request, no cooldown and no review.
2.  A job holding ``contents: write`` or ``id-token: write`` runs no code
    resolved at run time. That means: no ``npx``, no ``@latest``, no
    ``curl … | sh``, no unhashed ``pip install`` — in its own ``run:`` blocks or
    in any ``scripts/`` file those blocks invoke; ``actions/checkout`` leaves no
    credential in ``.git/config``; and a tool the job downloads
    (GoReleaser, syft) is pinned to an exact version, because SHA-pinning the
    action that fetches a binary does not pin the binary.
3.  Dependabot states its cooldown instead of inheriting a platform default
    that GitHub can change under us.
4.  ``SECURITY.md`` names the major version the repository actually ships.
5.  Both installers verify the release's Sigstore bundle, not only a
    ``checksums.txt`` fetched from the same mutable release.

Usage:
    python3 scripts/audit_supply_chain.py [--root <dir>]

Exits non-zero and prints one line per violation. Requires PyYAML, which the
GitHub-hosted runners already carry.
"""

import argparse
import os
import re
import sys

try:
    import yaml
except ImportError:  # pragma: no cover - exercised only on a host without PyYAML
    sys.exit("audit_supply_chain: PyYAML is required (pip install pyyaml)")

# A pinned reference is owner/repo[/subpath]@<40 hex>, optionally followed by a
# comment naming the human-readable version Dependabot keeps current.
PINNED_USES = re.compile(r"^[^@\s]+@[0-9a-f]{40}$")
USES_LINE = re.compile(r"^\s*(?:-\s*)?uses:\s*(\S+)")

# Permissions that make a job worth attacking: one mints the repository's
# signing and publishing identity, the other can rewrite the repository.
CREDENTIALED_PERMISSIONS = ("contents", "id-token")

# Code resolved at run time. Each pattern names something whose bytes are not
# fixed by anything committed here.
UNLOCKED_CODE = [
    (re.compile(r"\bnpx\b"), "npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)"),
    (re.compile(r"@latest\b"), "@latest is whatever the registry serves at that moment"),
    (re.compile(r"curl[^\n|]*\|\s*(?:ba)?sh\b"), "piping a download into a shell runs unreviewed code"),
    (re.compile(r"\bpip\s+install\b(?![^\n]*--require-hashes)"), "pip install without --require-hashes resolves at run time"),
]

# Ecosystems where an explicit cooldown is meaningful. docker-compose is
# exempt: its images are :latest test fixtures that open no pull request.
COOLDOWN_ECOSYSTEMS = ("gomod", "npm", "github-actions", "docker")
MIN_COOLDOWN_DAYS = 3

# The installers must reach for a signature, not only a sibling checksum file.
SIGNATURE_TOOLS = ("cosign verify-blob", "gh attestation verify", "Invoke-Signature")


def load_workflows(root):
    """Return [(path, text, parsed_or_None)] for every workflow file."""
    wf_dir = os.path.join(root, ".github", "workflows")
    out = []
    for name in sorted(os.listdir(wf_dir)):
        if not name.endswith((".yml", ".yaml")):
            continue
        path = os.path.join(wf_dir, name)
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
        out.append((os.path.join(".github", "workflows", name), text, yaml.safe_load(text)))
    return out


def check_pinned_uses(path, text):
    """Every action reference is a commit SHA, not a tag."""
    problems = []
    for lineno, line in enumerate(text.splitlines(), start=1):
        m = USES_LINE.match(line)
        if not m:
            continue
        ref = m.group(1).strip().strip("\"'")
        if ref.startswith("./") or ref.startswith("docker://"):
            continue
        if not PINNED_USES.match(ref):
            problems.append(f"{path}:{lineno}: uses: {ref} is not pinned to a 40-character commit SHA")
    return problems


def job_permissions(doc, job):
    """Effective permissions for a job: its own if present, else the workflow's."""
    perms = job.get("permissions")
    if perms is None:
        perms = doc.get("permissions")
    if perms is None:
        # GitHub's own default is a read-only token; treat it as uncredentialed.
        return {}
    if isinstance(perms, str):
        # "write-all" / "read-all" shorthand.
        return {"contents": "write", "id-token": "write"} if perms == "write-all" else {}
    return perms


def is_credentialed(doc, job):
    perms = job_permissions(doc, job)
    return any(perms.get(k) == "write" for k in CREDENTIALED_PERMISSIONS)


ENV_EXPRESSION = re.compile(r"\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}")


def resolve_env(value, doc):
    """Substitute ${{ env.NAME }} from the workflow's top-level env block.

    Version pins are declared once in `env:` and referenced from the steps that
    download the tool, so the checker has to see through that one indirection
    or it reads every pin as unpinned.
    """
    env = doc.get("env") or {}
    return ENV_EXPRESSION.sub(lambda m: str(env.get(m.group(1), m.group(0))), str(value))


def strip_comments(text):
    """Drop whole-line comments so a rationale *about* npx is not read as npx."""
    return "\n".join(line for line in text.splitlines() if not line.lstrip().startswith("#"))


def step_run_text(step):
    return strip_comments(step.get("run") or "")


def referenced_scripts(run_text):
    """scripts/<file> paths a run block invokes, so their contents are audited too."""
    return sorted(set(re.findall(r"scripts/[A-Za-z0-9_.-]+\.(?:sh|mjs|py|ps1)", run_text)))


def check_credentialed_job(path, job_id, job, root, doc=None):
    """A job holding a write credential may only run code this repository pins."""
    doc = doc or {}
    problems = []
    steps = job.get("steps") or []
    seen_scripts = set()
    for index, step in enumerate(steps):
        uses = (step.get("uses") or "").split("@")[0]
        with_ = step.get("with") or {}
        where = f"{path}: job {job_id}: step {index}"

        if uses == "actions/checkout":
            if with_.get("persist-credentials") not in (False, "false"):
                problems.append(
                    f"{where}: actions/checkout must set persist-credentials: false — "
                    "the job's write-capable token would otherwise sit in .git/config "
                    "while the rest of the steps run"
                )
        if uses == "goreleaser/goreleaser-action":
            version = resolve_env(with_.get("version", ""), doc)
            if not re.fullmatch(r"v\d+\.\d+\.\d+", version):
                problems.append(
                    f"{where}: goreleaser-action version {version!r} is not an exact vX.Y.Z — "
                    "pinning the action does not pin the binary it downloads"
                )
        if uses.startswith("anchore/sbom-action"):
            problems.append(
                f"{where}: anchore/sbom-action must not run in a credentialed job. Even SHA-pinned, "
                "on Linux it downloads raw.githubusercontent.com/anchore/syft/main/install.sh and "
                "runs it with sh, so the code executing here comes from a branch head. syft-version "
                "only selects the tarball that mutable script fetches. Download the release tarball "
                "directly, pinned by SHA256 and verified with cosign, as the Install syft step does"
            )

        run_text = step_run_text(step)
        for pattern, why in UNLOCKED_CODE:
            if pattern.search(run_text):
                problems.append(f"{where}: run block matches {pattern.pattern!r}: {why}")
        for rel in referenced_scripts(run_text):
            if rel in seen_scripts:
                continue
            seen_scripts.add(rel)
            script_path = os.path.join(root, rel)
            if not os.path.isfile(script_path):
                continue
            with open(script_path, encoding="utf-8") as fh:
                body = strip_comments(fh.read())
            for pattern, why in UNLOCKED_CODE:
                if pattern.search(body):
                    problems.append(f"{path}: job {job_id}: {rel} matches {pattern.pattern!r}: {why}")
    return problems


def check_workflow_jobs(path, doc, root):
    problems = []
    for job_id, job in (doc.get("jobs") or {}).items():
        if not isinstance(job, dict) or not is_credentialed(doc, job):
            continue
        problems.extend(check_credentialed_job(path, job_id, job, root, doc))
    return problems


def check_dependabot(doc):
    """Every cooldown-capable ecosystem states a window of its own."""
    problems = []
    for entry in doc.get("updates") or []:
        ecosystem = entry.get("package-ecosystem")
        if ecosystem not in COOLDOWN_ECOSYSTEMS:
            continue
        directory = entry.get("directory", "?")
        cooldown = entry.get("cooldown")
        label = f".github/dependabot.yml: {ecosystem} ({directory})"
        if not isinstance(cooldown, dict):
            problems.append(f"{label}: no cooldown — the release window is whatever GitHub defaults to today")
            continue
        days = cooldown.get("default-days")
        if not isinstance(days, int) or days < MIN_COOLDOWN_DAYS:
            problems.append(f"{label}: cooldown.default-days is {days!r}, want an integer >= {MIN_COOLDOWN_DAYS}")
        if ecosystem in ("docker", "docker-compose"):
            extra = sorted(k for k in cooldown if k != "default-days")
            if extra:
                problems.append(
                    f"{label}: cooldown carries SemVer keys {extra} — Dependabot rejects them for this "
                    "ecosystem and a rejected configuration stops its updates entirely"
                )
    return problems


def check_security_policy(version, security_md):
    """SECURITY.md's supported-versions table names the major we actually ship."""
    problems = []
    major = version.strip().split(".")[0]
    rows = [line for line in security_md.splitlines() if line.startswith("|")]
    table = "\n".join(rows)
    if not rows:
        return ["SECURITY.md: no supported-versions table found"]
    if f"`{major}.x`" not in table:
        problems.append(
            f"SECURITY.md: the supported-versions table never names `{major}.x`, "
            f"but VERSION says {version.strip()}"
        )
    for row in rows:
        m = re.search(r"`(\d+)\.x`", row)
        if not m or m.group(1) == major:
            continue
        if ":white_check_mark:" in row:
            problems.append(
                f"SECURITY.md: `{m.group(1)}.x` is still marked supported while the shipping major is {major}"
            )
    return problems


def check_installers(install_sh, install_ps1):
    """Both installers reach for the release signature, not only its checksums."""
    problems = []
    for name, body in (("scripts/install.sh", install_sh), ("scripts/install.ps1", install_ps1)):
        if not any(tool in body for tool in SIGNATURE_TOOLS):
            problems.append(
                f"{name}: verifies no signature — checksums.txt comes from the same mutable release "
                "as the binary, so a consistent replacement of both files is accepted"
            )
        if "checksums.txt.sigstore.json" not in body:
            problems.append(f"{name}: never fetches checksums.txt.sigstore.json, which every release publishes")
    return problems


def audit(root):
    problems = []
    for path, text, doc in load_workflows(root):
        problems.extend(check_pinned_uses(path, text))
        if isinstance(doc, dict):
            problems.extend(check_workflow_jobs(path, doc, root))

    with open(os.path.join(root, ".github", "dependabot.yml"), encoding="utf-8") as fh:
        problems.extend(check_dependabot(yaml.safe_load(fh.read())))

    with open(os.path.join(root, "VERSION"), encoding="utf-8") as fh:
        version = fh.read()
    with open(os.path.join(root, "SECURITY.md"), encoding="utf-8") as fh:
        problems.extend(check_security_policy(version, fh.read()))

    with open(os.path.join(root, "scripts", "install.sh"), encoding="utf-8") as fh:
        install_sh = fh.read()
    with open(os.path.join(root, "scripts", "install.ps1"), encoding="utf-8") as fh:
        install_ps1 = fh.read()
    problems.extend(check_installers(install_sh, install_ps1))
    return problems


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--root",
        default=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."),
        help="repository root (default: the parent of scripts/)",
    )
    args = parser.parse_args()

    problems = audit(os.path.abspath(args.root))
    if problems:
        sys.stdout.write("supply-chain audit FAILED ({} problems):\n".format(len(problems)))
        for problem in problems:
            sys.stdout.write("  x {}\n".format(problem))
        sys.exit(1)
    sys.stdout.write("supply-chain audit passed: pinned actions, locked release jobs, "
                     "stated cooldowns, current security policy, signature-verifying installers\n")


if __name__ == "__main__":
    main()
