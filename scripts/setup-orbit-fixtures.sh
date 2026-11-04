#!/usr/bin/env bash
# Copyright (c) 2026 José M. Requena Plens
# SPDX-License-Identifier: MIT

# setup-orbit-fixtures.sh — create reproducible Orbit knowledge-graph
# fixtures in any GitLab.com group. Idempotent: re-running it on an
# already-provisioned namespace reconciles it: missing pieces are created,
# each project is set to ORBIT_FIXTURES_VISIBILITY, and the rest left alone.
#
# Usage:
#   GITLAB_COM_TOKEN=glpat-... \
#   ORBIT_FIXTURES_NAMESPACE=jmrp-oss/orbit-fixtures \
#   ./scripts/setup-orbit-fixtures.sh [--mirror-cli]
#
# Environment:
#   GITLAB_COM_TOKEN (required)        — personal access token with api+write_repository
#   ORBIT_FIXTURES_NAMESPACE (default: jmrp-oss/orbit-fixtures)
#                                       full path of the group or subgroup under which to create projects
#   ORBIT_FIXTURES_VISIBILITY (default: public)
#                                       every project in jmrp-oss is public; set private for a private group
#   ORBIT_FIXTURES_GITLAB_URL (default: https://gitlab.com)
#
# Flags:
#   --mirror-cli    additionally mirror gitlab-org/cli (the glab CLI) as
#                   <namespace>/glab-mirror. The mirror brings real data:
#                   real MRs, real CI, real code, real branches. An existing
#                   mirror is reconciled, not pushed again.
#   --skip-mirror   do not mirror gitlab-org/cli even if it does not
#                   exist. Used by CI to avoid the ~10 min mirror step.
#
# What it creates:
#   <namespace>/kg-fixtures
#     - the repository's MIT LICENSE
#     - Python package (acme.orders) with cross-module imports
#     - .gitlab-ci.yml (4 stages, 5 jobs, SAST, Secret Detection)
#     - 1 squash-merged MR (feature/restock-helper)
#     - 5 issues, 1 milestone, 7 labels
#   <namespace>/security-fixtures
#     - the repository's MIT LICENSE
#     - Intentionally vulnerable code (AWS keys, SQLi, weak hash, eval)
#     - SAST, Secret-Detection, Dependency-Scanning, Container-Scanning
#     - 5 vulnerabilities emitted by the analyzers
#     - 1 smoke test
#   <namespace>/glab-mirror (only with --mirror-cli)
#     - Full mirror of gitlab-org/cli for realistic cross-entity queries
#
# Idempotency: each project, branch, label, milestone, issue, and MR is
# checked for existence via the API before being created. Re-runs are safe.

set -euo pipefail

# ─── Argument parsing ──────────────────────────────────────────────────

MIRROR_CLI=false
SKIP_MIRROR=false
for arg in "$@"; do
  case "$arg" in
    --mirror-cli) MIRROR_CLI=true ;;
    --skip-mirror) SKIP_MIRROR=true ;;
    -h|--help)
      # The usage comment, from its first line to the blank line that ends
      # it, found by its text so that no line above it shifts what is shown.
      sed -n '/^# setup-orbit-fixtures\.sh /,/^$/{/./p;}' "$0"
      exit 0
      ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

# ─── Configuration ─────────────────────────────────────────────────────

: "${GITLAB_COM_TOKEN:?GITLAB_COM_TOKEN is required}"
GITLAB_URL="${ORBIT_FIXTURES_GITLAB_URL:-https://gitlab.com}"
NAMESPACE="${ORBIT_FIXTURES_NAMESPACE:-jmrp-oss/orbit-fixtures}"
# Public by default because the fixtures live in jmrp-oss, a GitLab for Open
# Source program namespace whose every project has to be public and carry an
# OSI license: a project re-provisioned private there would break the rule.
VISIBILITY="${ORBIT_FIXTURES_VISIBILITY:-public}"

# #1533: jmrp-oss allows exactly one private project, and these fixtures are
# public. Refuse a non-public visibility up front: the script now sets the
# visibility of projects it did not create, so a typo would flip both fixtures.
# GitLab resolves paths case-insensitively, so the guard compares lowercased,
# or JMRP-OSS/... would reach the same projects and skip the check.
case "$(printf '%s' "$NAMESPACE" | tr '[:upper:]' '[:lower:]')" in
  jmrp-oss | jmrp-oss/*)
    if [ "$VISIBILITY" != "public" ]; then
      echo "error: ORBIT_FIXTURES_VISIBILITY=$VISIBILITY is not allowed under $NAMESPACE:" >&2
      echo "       jmrp-oss allows one private project, and these fixtures must be public." >&2
      exit 2
    fi
    ;;
esac

API="$GITLAB_URL/api/v4"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." &>/dev/null && pwd)"
FIXTURE_BASE="$ROOT_DIR/test/fixtures/orbit"

H_AUTH=(-H "PRIVATE-TOKEN: $GITLAB_COM_TOKEN")
H_JSON=(-H "Content-Type: application/json")

# ─── Helpers ───────────────────────────────────────────────────────────

# Every request has a deadline: a connection GitLab accepts but never answers
# would otherwise block the run with no way to tell it apart from a slow one.
CURL_TIMEOUTS=(--connect-timeout 10 --max-time 120)

# api_post PATH JSON  ->  echoes the response body
api_post() {
  curl -sS "${CURL_TIMEOUTS[@]}" -X POST "${H_AUTH[@]}" "${H_JSON[@]}" "$API$1" -d "$2"
}

# api_get PATH  ->  echoes the response body
api_get() {
  curl -sS "${CURL_TIMEOUTS[@]}" "${H_AUTH[@]}" "$API$1"
}

# api_request METHOD PATH [JSON]  ->  sets API_STATUS and API_BODY
#   The helpers above run in a command substitution, so a status set there
#   would be lost; this one runs in the caller's shell. curl exits 0 for an
#   HTTP error response, and a write GitLab refused must not be reported as
#   applied, so the status is checked before anything is printed as done.
api_request() {
  local method="$1" path="$2" body="${3:-}" response
  if [ -n "$body" ]; then
    response=$(curl -sS "${CURL_TIMEOUTS[@]}" -X "$method" "${H_AUTH[@]}" "${H_JSON[@]}" "$API$path" -d "$body" -w '\n%{http_code}')
  else
    response=$(curl -sS "${CURL_TIMEOUTS[@]}" -X "$method" "${H_AUTH[@]}" "$API$path" -w '\n%{http_code}')
  fi
  API_STATUS=${response##*$'\n'}
  API_BODY=${response%$'\n'*}
}

# api_ok  ->  whether the last api_request was answered with a 2xx
#   A string match: a curl that never ran or was killed (an argument over the
#   kernel's limit, an OOM kill) leaves API_STATUS empty or not a number, and
#   [ "" -lt 200 ] is an error, which an if reads as false.
api_ok() {
  case "$API_STATUS" in
    2[0-9][0-9]) return 0 ;;
    *) return 1 ;;
  esac
}

# ensure_resource GET_PATH JSON
#   Runs GET GET_PATH; if a 200-ish response with a positive integer id is
#   returned, treats the resource as already created and echoes the
#   existing id. Otherwise POSTs the JSON body to the same collection and
#   echoes the new id, or prints GitLab's answer and exits when the answer
#   carries no id, since a caller that went on would report a resource
#   that was never created.
ensure_resource() {
  local get_path="$1"
  local post_json="$2"
  local id=$(api_get "$get_path" | python3 -c 'import sys,json
try:
    d = json.load(sys.stdin)
    if isinstance(d, dict) and d.get("id"): print(d["id"])
    elif isinstance(d, list) and d and d[0].get("id"): print(d[0]["id"])
except Exception: pass' 2>/dev/null)
  if [ -n "$id" ] && [ "$id" -gt 0 ] 2>/dev/null; then
    echo "$id"
    return
  fi
  # The query string is what finds an existing resource; the collection in
  # front of it is where a new one is created, so
  # ensure_resource "/projects/$KG_ID/milestones?title=foo" POSTs to
  # /projects/$KG_ID/milestones.
  local post_path="${get_path%%\?*}"
  local resp new_id
  resp=$(api_post "$post_path" "$post_json")
  new_id=$(printf '%s' "$resp" | python3 -c 'import sys,json
try:
    d = json.load(sys.stdin)
    if isinstance(d, dict) and d.get("id"): print(d["id"])
except Exception: pass' 2>/dev/null)
  if [ -z "$new_id" ]; then
    echo "    ✗ POST $post_path did not create anything: $resp" >&2
    exit 1
  fi
  echo "$new_id"
}

# ─── Namespace ─────────────────────────────────────────────────────────

echo ">>> resolving namespace '$NAMESPACE' on $GITLAB_URL"
# URL-encode the namespace so nested subgroups (e.g. team/platform)
# resolve correctly under /groups/<encoded-namespace>. Without this
# the request would be interpreted as /groups/team/platform (a
# 404) instead of /groups/team%2Fplatform.
NAMESPACE_ENC=$(printf %s "$NAMESPACE" | python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read(), safe=""))')
# Read through api_get like every other API read here, never as curl piped
# straight into python3: OpenSSF Scorecard's Pinned-Dependencies check reads
# a download utility piped into an interpreter as a script fetched and run,
# whatever the interpreter does with it, and the answer here is the group's
# JSON, which no version, checksum or digest could pin.
NAMESPACE_ID=$(api_get "/groups/$NAMESPACE_ENC" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')
echo "    namespace id: $NAMESPACE_ID"

# ─── Project creation (pushes local source tree via git) ───────────────

# reconcile_existing_project ID FIXTURE_DIR
#   Brings a fixture project that already exists into line (#1533): creates
#   by API the fixture files the project lacks, then sets the visibility, since
#   jmrp-oss requires an OSI license on every project and a run that stops
#   halfway must not have made public a project still without one. A re-run
#   used to leave an existing project untouched, so one provisioned before its
#   LICENSE existed, a private one, and one whose first push failed all stayed
#   broken while the run reported success.
reconcile_existing_project() {
  local project_id="$1"
  local fixture_dir="$2"

  # A read that failed must not become an empty visibility, which would look
  # like a mismatch and PUT public: that would expose a private project whose
  # state was simply unreadable. Stop instead.
  api_request GET "/projects/$project_id"
  if [ "$API_STATUS" != "200" ]; then
    echo "    ✗ cannot read project $project_id (HTTP $API_STATUS); not reconciling" >&2
    return 1
  fi
  local current_visibility
  current_visibility=$(printf '%s' "$API_BODY" | python3 -c 'import sys,json
try:
    print(json.load(sys.stdin).get("visibility",""))
except Exception: pass' 2>/dev/null)
  case "$current_visibility" in
    public | internal | private) ;;
    *)
      echo "    ✗ project $project_id has no readable visibility; not reconciling" >&2
      return 1
      ;;
  esac
  # A mirror has no fixture files to compare, and its tree is the upstream
  # project's: thousands of entries read for nothing.
  if [ -n "$fixture_dir" ]; then
    add_missing_fixture_files "$project_id" "$fixture_dir" || return 1
  fi
  if [ "$current_visibility" != "$VISIBILITY" ]; then
    # The body is built by json.dumps, as the commit's is below, so a configured
    # value carrying JSON syntax stays one string and adds no field.
    local visibility_body
    visibility_body=$(VISIBILITY="$VISIBILITY" python3 -c 'import json, os; print(json.dumps({"visibility": os.environ["VISIBILITY"]}))')
    api_request PUT "/projects/$project_id" "$visibility_body"
    if ! api_ok; then
      echo "    ✗ could not set visibility (HTTP $API_STATUS): $API_BODY" >&2
      return 1
    fi
    echo "    → visibility $current_visibility → $VISIBILITY" >&2
  fi
}

# add_missing_fixture_files ID FIXTURE_DIR
#   Creates in one commit the files git tracks in the fixture that branch main
#   of the project lacks.
add_missing_fixture_files() {
  local project_id="$1"
  local fixture_dir="$2"

  # The repository tree lists the paths the project already holds. Every page
  # is read: one page of 100 is not the whole tree, and a fixture path past
  # the first page would be created again on every run. A route that cannot be
  # read stops reconciliation, never guessed at.
  local page=1 pages="" body count payload
  while :; do
    api_request GET "/projects/$project_id/repository/tree?ref=main&recursive=true&per_page=100&page=$page"
    if [ "$API_STATUS" != "200" ]; then
      echo "    ✗ cannot list the project's files (HTTP $API_STATUS); not reconciling" >&2
      return 1
    fi
    body="$API_BODY"
    count=$(printf '%s' "$body" | python3 -c 'import sys,json
try:
    answer = json.load(sys.stdin)
    print(len(answer) if isinstance(answer, list) else "ERR")
except Exception:
    print("ERR")')
    if [ "$count" = "ERR" ]; then
      echo "    ✗ the project's file list is unreadable; not reconciling" >&2
      return 1
    fi
    [ "$count" -eq 0 ] && break
    pages+="$body"$'\n'
    [ "$count" -lt 100 ] && break
    page=$((page + 1))
  done
  # The files git tracks, never whatever else the disk holds: these projects are
  # public, and a local .env or __pycache__ must not reach them. The answer goes
  # in on stdin, never in the environment, where a variable over the kernel's
  # 128 KiB per-string limit stops python3 from starting. Any failure stops the
  # run, so that no payload can only mean the comparison ran.
  #
  # One commit, and one request: .gitlab-ci.yml is among the fixture files, so a
  # commit per file would start a pipeline per file, each on a partial tree, and
  # a stop halfway would leave part of a fixture behind. A project holding
  # GitLab's README alone is one whose first push failed, the push that removes
  # that README, so there it is replaced. The LICENSE stays first in the list,
  # though nothing lands before anything else in one commit.
  payload=$(printf '%s' "$pages" | FIXTURE_DIR="$fixture_dir" python3 -c 'import base64,json,os,subprocess,sys
present = set()
for line in sys.stdin:
    if line.strip():
        present.update(e["path"] for e in json.loads(line) if e.get("type") != "tree")
fixture_dir = os.environ["FIXTURE_DIR"]
listed = subprocess.run(["git", "ls-files", "-z"], cwd=fixture_dir, check=True, stdout=subprocess.PIPE).stdout
tracked = [rel for rel in listed.decode().split("\0") if rel]
replaced = ["README.md"] if present == {"README.md"} else []
actions = []
for rel in sorted(tracked, key=lambda rel: (rel != "LICENSE", rel)):
    if rel in replaced or rel not in present:
        with open(os.path.join(fixture_dir, rel), "rb") as f:
            content = base64.b64encode(f.read()).decode()
        actions.append({"action": "update" if rel in replaced else "create", "file_path": rel, "encoding": "base64", "content": content})
if actions:
    print(json.dumps({"branch": "main", "commit_message": "Add the fixture files this project lacks", "actions": actions}))') || {
    echo "    ✗ cannot compare the project's files with the fixture; not reconciling" >&2
    return 1
  }
  if [ -z "$payload" ]; then
    echo "    ✓ project files are in line" >&2
    return 0
  fi
  api_request POST "/projects/$project_id/repository/commits" "$payload"
  if ! api_ok; then
    echo "    ✗ could not add the missing fixture files (HTTP $API_STATUS): $API_BODY" >&2
    return 1
  fi
  printf '%s' "$payload" | python3 -c 'import json,sys
for action in json.load(sys.stdin)["actions"]:
    print("    → %s %s" % ("replaced" if action["action"] == "update" else "added", action["file_path"]), file=sys.stderr)'
}

# create_or_skip_project NAME PATH FIXTURE_DIR DESCRIPTION [MIRROR_FROM]
#   - If the project exists, reconciles it (visibility and missing fixture
#     files) and echoes the existing id to stdout (so callers can capture it
#     with $()).
#   - Otherwise, creates the project, clones it locally, copies the
#     fixture content, commits, and pushes.
create_or_skip_project() {
  local name="$1"
  local path="$2"
  local fixture_dir="$3"
  local description="$4"
  local mirror_from="${5:-}"  # remote URL to push as origin instead of default

  # The namespace is encoded as the group lookup above encodes it: under a
  # subgroup a raw slash would send /projects/jmrp-oss/orbit-fixtures%2F...,
  # which GitLab answers 404, and the script would then try to create a
  # project that already exists and stop when GitLab refuses it.
  local existing_id
  existing_id=$(api_get "/projects/$NAMESPACE_ENC%2F$path" \
    | python3 -c 'import sys,json
try:
    print(json.load(sys.stdin).get("id",""))
except Exception: pass' 2>/dev/null)

  if [ -n "$existing_id" ] && [ "$existing_id" -gt 0 ] 2>/dev/null; then
    echo "    ✓ project exists: $NAMESPACE/$path (id=$existing_id)" >&2
    # Command substitutions run without errexit unless inherit_errexit is on,
    # so a failed reconciliation has to be returned explicitly.
    reconcile_existing_project "$existing_id" "$fixture_dir" || return 1
    echo "$existing_id"
    return 0
  fi

  echo "    → creating project $NAMESPACE/$path" >&2
  local create_resp
  create_resp=$(api_post "/projects" "$(cat <<JSON
{
  "name": "$name",
  "path": "$path",
  "namespace_id": $NAMESPACE_ID,
  "description": "$description",
  "visibility": "$VISIBILITY",
  "initialize_with_readme": true,
  "default_branch": "main",
  "issues_enabled": true,
  "merge_requests_enabled": true,
  "wiki_enabled": false
}
JSON
)")
  local new_id
  new_id=$(echo "$create_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("id",""))')
  if [ -z "$new_id" ] || [ "$new_id" = "None" ]; then
    echo "    ✗ failed to create project: $create_resp" >&2
    return 1
  fi
  echo "    ✓ created project id=$new_id" >&2

  # Push fixture content
  #
  # We use `git -c http.extraHeader=PRIVATE-TOKEN: …` to keep the
  # token out of any URL that git might echo on a failed push. The
  # `-c` flag is per-invocation, so it does NOT propagate to the
  # cloned/mirrored repo's git config. After the clone, we
  # therefore set the same http.extraHeader on the local repo so
  # the subsequent `git push` (which uses the persisted `origin`
  # remote, not the URL with embedded creds) also authenticates.
  # The credential lives in `.git/config` for the lifetime of
  # the work dir, which is removed immediately after the push.
  if [ -n "$mirror_from" ]; then
    echo "    → mirroring from $mirror_from (this may take several minutes)" >&2
    git clone --quiet --mirror "$mirror_from" "$ROOT_DIR/.tmp-orbit-mirror-$path" 2>&1 | tail -3 >&2
    (cd "$ROOT_DIR/.tmp-orbit-mirror-$path" && \
      git config --local http.extraHeader "PRIVATE-TOKEN: $GITLAB_COM_TOKEN" && \
      git push --quiet --force "${GITLAB_URL}/$NAMESPACE/$path.git" 'refs/heads/*:refs/heads/*' 'refs/tags/*:refs/tags/*' 2>&1 | tail -3) >&2 || true
    rm -rf "$ROOT_DIR/.tmp-orbit-mirror-$path"
  else
    local work="$ROOT_DIR/.tmp-orbit-push-$path"
    rm -rf "$work"
    git -c "http.extraHeader=PRIVATE-TOKEN: $GITLAB_COM_TOKEN" \
      clone --quiet "${GITLAB_URL}/$NAMESPACE/$path.git" "$work" 2>&1 | tail -3 >&2
    # Persist the same http.extraHeader on the local repo so the
    # subsequent `git push -u origin main` authenticates. Without
    # this, the persisted `origin` remote (clean URL, no creds) has
    # no credentials and the push returns HTTP Basic: Access
    # denied. See: https://gitlab.com/help/topics/git/troubleshooting_git.md
    #
    # Note: only http.extraHeader is persisted. The user.email /
    # user.name identity for the bot is passed inline to `git
    # commit -c user.email=… -c user.name=…` below; per-command
    # `-c` flags do not write to `.git/config`, so the surrounding
    # main repo's git config (or any other cwd the script is
    # invoked from) is not contaminated by the bot identity.
    (cd "$work" && \
      git config --local http.extraHeader "PRIVATE-TOKEN: $GITLAB_COM_TOKEN")
    # Remove the auto-generated README so the local one wins on push
    rm -f "$work/README.md"
    rsync -a --exclude='.git/' "$fixture_dir/" "$work/"
    (
      cd "$work"
      git -c user.email="orbit-fixtures@local" \
          -c user.name="Orbit Fixtures Bot" \
        add -A
      git -c user.email="orbit-fixtures@local" \
          -c user.name="Orbit Fixtures Bot" \
        commit -m "Initial fixture content" --quiet
      git push --quiet -u origin main 2>&1 | tail -3 >&2
    )
    rm -rf "$work"
  fi
  echo "$new_id"
}

# ─── Label creation ────────────────────────────────────────────────────

ensure_label() {
  local project_id="$1" name="$2" color="$3" description="$4"
  local existing=$(api_get "/projects/$project_id/labels?search=$(printf %s "$name" | python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read()))')" \
    | python3 -c 'import sys,json
try:
    arr = json.load(sys.stdin)
    if isinstance(arr, list):
        for L in arr:
            if L.get("name") == "'"$name"'": print(L["id"]); break
except Exception: pass' 2>/dev/null)
  if [ -n "$existing" ] && [ "$existing" -gt 0 ] 2>/dev/null; then
    echo "$existing" >/dev/null
    return
  fi
  api_post "/projects/$project_id/labels" "$(cat <<JSON
{"name":"$name","color":"$color","description":"$description"}
JSON
)" >/dev/null
}

# ─── Fixture 1: kg-fixtures ──────────────────────────────────────────

echo ""
echo ">>> fixture: $NAMESPACE/kg-fixtures"
KG_ID=$(create_or_skip_project \
  "kg-fixtures" \
  "kg-fixtures" \
  "$FIXTURE_BASE/kg-fixtures" \
  "Knowledge Graph fixture project: source code with imports, CI pipelines, issues, milestones, merge requests. Designed to exercise every entity type in the Orbit ontology.")
if [ -z "$KG_ID" ]; then echo "aborting"; exit 1; fi

echo "    → labels"
for label_data in \
  "bug|#d73a4a" \
  "enhancement|#a2eeef" \
  "documentation|#0075ca" \
  "good first issue|#7057ff" \
  "ci|#fbca04" \
  "security|#b60205" \
  "fixture|#0e8a16"
do
  IFS='|' read -r name color <<< "$label_data"
  ensure_label "$KG_ID" "$name" "$color" "Fixture label for $name testing."
done

echo "    → milestone"
MS_ID=$(ensure_resource "/projects/$KG_ID/milestones?title=$(printf %s "v0.2 — KG coverage milestone" | python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read()))')" "$(cat <<'JSON'
{
  "title": "v0.2 — KG coverage milestone",
  "description": "Track the fixtures that populate each Orbit entity type. Closes when the namespace reports coverage for ci, plan-Milestone, and security domains.",
  "due_date": "2026-07-01"
}
JSON
)")
echo "    ✓ milestone id=$MS_ID"

echo "    → issues"
for issue_data in \
  '1|Add reserve_stock seed helper|enhancement,good first issue' \
  '2|Index additional entity types in graph_status|fixture' \
  '3|Document the Orbit query DSL|documentation,fixture' \
  '4|Trigger security scanning for security domain|security,ci' \
  '5|Tighten ruff rules in CI|ci,enhancement'
do
  IFS='|' read -r num title labels <<< "$issue_data"
  # ensure_resource with the ?search=<title> query makes the loop
  # idempotent: re-running the script won't keep appending duplicate
  # issues with the same title.
  issue_title_enc=$(printf %s "$title" | python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read()))')
  ensure_resource "/projects/$KG_ID/issues?search=$issue_title_enc" "$(cat <<JSON
{
  "title": "$title",
  "description": "Fixture issue for Knowledge Graph coverage. Used by gitlab-mcp-server Orbit tools to exercise the plan and ci domains.",
  "labels": "$labels"
}
JSON
)" >/dev/null
done

echo "    → environments"
for env_data in \
  'staging|https://staging.example.com' \
  'production|https://example.com'
do
  IFS='|' read -r env_name ext_url <<< "$env_data"
  # ensure_resource with the ?name=<env> query makes the loop
  # idempotent: re-running the script won't keep appending duplicate
  # environments with the same name.
  ensure_resource "/projects/$KG_ID/environments?name=$env_name" "$(cat <<JSON
{"name":"$env_name","external_url":"$ext_url"}
JSON
)" >/dev/null
done

echo "    → squash-merged MR (feature/restock-helper → main)"
# Idempotency: only create the branch and MR if MR !1 does not exist
existing_mr=$(api_get "/projects/$KG_ID/merge_requests?source_branch=feature/restock-helper&state=all" \
  | python3 -c 'import sys,json
try:
    arr = json.load(sys.stdin)
    if isinstance(arr, list) and arr:
        print(arr[0]["iid"])
except Exception: pass' 2>/dev/null)
if [ -z "$existing_mr" ]; then
  work="$ROOT_DIR/.tmp-orbit-mr-$KG_ID"
  rm -rf "$work"
  git -c "http.extraHeader=PRIVATE-TOKEN: $GITLAB_COM_TOKEN" \
    clone --quiet "${GITLAB_URL}/$NAMESPACE/kg-fixtures.git" "$work" 2>&1 | tail -2
  (
    cd "$work"
    git config user.email "orbit-fixtures@local"
    git config user.name  "Orbit Fixtures Bot"
    git checkout -q -b feature/restock-helper
    # Insert a seed_stock helper (deterministic diff)
    python3 - <<'PY'
from pathlib import Path
p = Path("src/acme/orders/fulfillment.py")
src = p.read_text()
seed = '''

def seed_stock(sku: str, quantity: int) -> None:
    """Add `quantity` units of `sku` to the in-memory stock register."""
    _STOCK[sku] = _STOCK.get(sku, 0) + quantity

'''
needle = "def reserve_stock"
idx = src.find(needle)
assert idx > 0
p.write_text(src[:idx] + seed + src[idx:])
PY
    git add -A
    git commit -m "feat(fulfillment): add seed_stock helper for fixture test runs" --quiet
    git push --quiet -u origin feature/restock-helper 2>&1 | tail -2
  )
  new_mr=$(api_post "/projects/$KG_ID/merge_requests" "$(cat <<'JSON'
{
  "source_branch": "feature/restock-helper",
  "target_branch": "main",
  "title": "Add seed_stock helper for fixture test runs",
  "description": "Adds a `seed_stock(sku, quantity)` helper to the fulfillment module so test runs can populate the in-memory stock register without mocking.",
  "remove_source_branch": true,
  "squash": true
}
JSON
)" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("iid",""))')
  echo "    ✓ MR !$new_mr opened"
  api_post "/projects/$KG_ID/merge_requests/$new_mr/merge" '{"squash":true,"should_remove_source_branch":true,"merge_commit_message":"Add seed_stock helper for fixture test runs"}' >/dev/null
  echo "    ✓ MR !$new_mr merged"
  rm -rf "$work"
else
  echo "    ✓ MR with source=feature/restock-helper already exists (!$existing_mr)"
fi

# ─── Fixture 2: security-fixtures ─────────────────────────────────────

echo ""
echo ">>> fixture: $NAMESPACE/security-fixtures"
SEC_ID=$(create_or_skip_project \
  "security-fixtures" \
  "security-fixtures" \
  "$FIXTURE_BASE/security-fixtures" \
  "Security scan fixture: intentional vulnerable patterns so SAST, Secret Detection, and Dependency Scanning report findings.")
if [ -z "$SEC_ID" ]; then echo "aborting"; exit 1; fi

# ─── Optional: mirror gitlab-org/cli ───────────────────────────────────

if [ "$MIRROR_CLI" = true ] && [ "$SKIP_MIRROR" = false ]; then
  echo ""
  echo ">>> mirror: $NAMESPACE/glab-mirror (from gitlab-org/cli)"
  MIRROR_ID=$(create_or_skip_project \
    "glab-mirror" \
    "glab-mirror" \
    "" \
    "Mirror of gitlab-org/cli (the glab CLI). Brings real-world data: real MRs, real CI, real code, real branches. Used as a stable, large-scale fixture for Orbit knowledge graph queries." \
    "https://gitlab.com/gitlab-org/cli.git")
  if [ -n "$MIRROR_ID" ]; then
    echo "    ✓ glab-mirror id=$MIRROR_ID"
  fi
fi

# ─── Summary ──────────────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo " Done. Fixtures provisioned under $NAMESPACE on $GITLAB_URL"
echo "═══════════════════════════════════════════════════════════════"
echo "  $NAMESPACE/kg-fixtures        (id=$KG_ID) — code, CI, milestone, MR"
echo "  $NAMESPACE/security-fixtures  (id=$SEC_ID) — SAST/secret-detection findings"
if [ "$MIRROR_CLI" = true ] && [ -n "${MIRROR_ID:-}" ]; then
  echo "  $NAMESPACE/glab-mirror       (id=$MIRROR_ID) — glab CLI mirror"
fi
echo ""
echo "The Orbit knowledge graph indexer will pick up the new content"
echo "within a few minutes. Re-run this script at any time — it is"
echo "idempotent: an existing project is brought into line, and any other"
echo "existing resource is left as it is."
