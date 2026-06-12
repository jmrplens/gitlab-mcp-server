#!/usr/bin/env bash
# setup-orbit-fixtures.sh — create reproducible Orbit knowledge-graph
# fixtures in any GitLab.com group. Idempotent: re-running it on an
# already-provisioned namespace is a no-op.
#
# Usage:
#   GITLAB_COM_TOKEN=glpat-... \
#   ORBIT_FIXTURES_NAMESPACE=plens1 \
#   ./scripts/setup-orbit-fixtures.sh [--mirror-cli]
#
# Environment:
#   GITLAB_COM_TOKEN (required)        — personal access token with api+write_repository
#   ORBIT_FIXTURES_NAMESPACE (default: plens1)
#                                       — group path under which to create projects
#   ORBIT_FIXTURES_VISIBILITY (default: private)
#   ORBIT_FIXTURES_GITLAB_URL (default: https://gitlab.com)
#
# Flags:
#   --mirror-cli    additionally mirror gitlab-org/cli (the glab CLI) as
#                   plens1/glab-mirror. The mirror brings real-world data:
#                   real MRs, real CI, real code, real branches. Skipped
#                   on re-runs.
#   --skip-mirror   do not mirror gitlab-org/cli even if it does not
#                   exist. Used by CI to avoid the ~10 min mirror step.
#
# What it creates:
#   <namespace>/kg-fixtures
#     - Python package (acme.orders) with cross-module imports
#     - .gitlab-ci.yml (4 stages, 5 jobs, SAST, Secret Detection)
#     - 1 squash-merged MR (feature/restock-helper)
#     - 5 issues, 1 milestone, 7 labels
#   <namespace>/security-fixtures
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
      sed -n '2,40p' "$0"
      exit 0
      ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

# ─── Configuration ─────────────────────────────────────────────────────

: "${GITLAB_COM_TOKEN:?GITLAB_COM_TOKEN is required}"
GITLAB_URL="${ORBIT_FIXTURES_GITLAB_URL:-https://gitlab.com}"
NAMESPACE="${ORBIT_FIXTURES_NAMESPACE:-plens1}"
VISIBILITY="${ORBIT_FIXTURES_VISIBILITY:-private}"

API="$GITLAB_URL/api/v4"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." &>/dev/null && pwd)"
FIXTURE_BASE="$ROOT_DIR/test/fixtures/orbit"

H_AUTH=(-H "PRIVATE-TOKEN: $GITLAB_COM_TOKEN")
H_JSON=(-H "Content-Type: application/json")

# Resolve namespace id
echo ">>> resolving namespace '$NAMESPACE' on $GITLAB_URL"
# URL-encode the namespace so nested subgroups (e.g. team/platform)
# resolve correctly under /groups/<encoded-namespace>. Without this
# the request would be interpreted as /groups/team/platform (a
# 404) instead of /groups/team%2Fplatform.
NAMESPACE_ENC=$(printf %s "$NAMESPACE" | python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read(), safe=""))')
NAMESPACE_ID=$(curl -sS "${H_AUTH[@]}" "$API/groups/$NAMESPACE_ENC" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["id"])')
echo "    namespace id: $NAMESPACE_ID"

# ─── Helpers ───────────────────────────────────────────────────────────

# api_post PATH JSON  →  echoes the response body
api_post() {
  curl -sS -X POST "${H_AUTH[@]}" "${H_JSON[@]}" "$API$1" -d "$2"
}

# api_get PATH  →  echoes the response body
api_get() {
  curl -sS "${H_AUTH[@]}" "$API$1"
}

# expect_existing_or_new PATH JSON NEW_ID_BASH
#   Runs GET PATH; if a 200-ish response with a positive integer id is
#   returned, treats the resource as already created and echoes the
#   existing id. Otherwise POSTs the JSON body and echoes the new id.
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
  # Strip the query string before computing the POST path so that
  # ensure_resource "/projects/$KG_ID/milestones?title=foo" POSTs to
  # /projects/$KG_ID/milestones rather than to /projects/$KG_ID.
  local post_path="${get_path%%\?*}"
  api_post "$(dirname "$post_path")" "$post_json" \
    | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("id",""))'
}

# ─── Project creation (pushes local source tree via git) ───────────────

# create_or_skip_project NAME PATH FIXTURE_DIR DESCRIPTION [MIRROR_FROM]
#   - If the project does not exist, creates it.
#   - If it exists, prints "project exists" to stderr and echoes the
#     existing id to stdout (so callers can capture it with $()).
#   - Otherwise, creates the project, clones it locally, copies the
#     fixture content, commits, and pushes.
create_or_skip_project() {
  local name="$1"
  local path="$2"
  local fixture_dir="$3"
  local description="$4"
  local mirror_from="${5:-}"  # remote URL to push as origin instead of default

  local existing_id
  existing_id=$(api_get "/projects/$NAMESPACE%2F$path" \
    | python3 -c 'import sys,json
try:
    print(json.load(sys.stdin).get("id",""))
except Exception: pass' 2>/dev/null)

  if [ -n "$existing_id" ] && [ "$existing_id" -gt 0 ] 2>/dev/null; then
    echo "    ✓ project exists: $NAMESPACE/$path (id=$existing_id)" >&2
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
echo "idempotent and skips already-created resources."
