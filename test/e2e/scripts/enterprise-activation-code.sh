#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

REPO_ROOT="$REPO_ROOT" python3 - <<'PY'
import os
import pathlib
import re

activation_pattern = re.compile(r"^[A-Za-z0-9]{24}$")
repo_root = pathlib.Path(os.environ["REPO_ROOT"])


def clean(value: str) -> str:
    return value.strip().strip('"').strip("'")


def dotenv_values() -> dict[str, str]:
    env_path = repo_root / ".env"
    values: dict[str, str] = {}
    if not env_path.exists():
        return values
    for raw in env_path.read_text(errors="replace").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        if key.startswith("export "):
            key = key.split(None, 1)[1].strip()
        values[key] = clean(value)
    return values


def cached_license_present() -> bool:
    configured = os.environ.get("E2E_ENTERPRISE_LICENSE_FILE", "")
    path = pathlib.Path(configured) if configured else repo_root / "test/e2e/.enterprise-license"
    if not path.is_absolute():
        path = repo_root / path
    try:
        return bool(path.read_text(errors="replace").strip())
    except OSError:
        return False


if cached_license_present():
    raise SystemExit(0)


value = clean(os.environ.get("GITLAB_ACTIVATION_CODE", ""))
if not activation_pattern.fullmatch(value):
    value = clean(os.environ.get("ENTERPRISE_LICENSE", ""))
if not activation_pattern.fullmatch(value):
    values = dotenv_values()
    value = values.get("GITLAB_ACTIVATION_CODE", "")
    if not activation_pattern.fullmatch(value):
        value = values.get("ENTERPRISE_LICENSE", "")

if activation_pattern.fullmatch(value):
    print(value)
PY
