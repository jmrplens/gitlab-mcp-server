#!/usr/bin/env bash
# Run Docker-backed model evaluation presets for one MCP tool surface.

set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/eval-surfaces-docker.sh <dynamic|meta> [preset]

Runs the Docker GitLab CE or Enterprise evaluation suite end-to-end for one tool surface:
1. Clean and start the Docker GitLab fixture stack.
2. Provision GitLab and register the CI runner.
3. Prepare evaluator fixtures.
4. Run the selected Docker preset, or all Docker presets by default.
5. Publish docs/testing/model-results.md and README.md summaries after full runs.

Preset values:
  docker-read
  docker-mutating-safe
  docker-destructive-safe
  docker-enterprise-read
  docker-enterprise-mutating-safe
  docker-enterprise-destructive-safe
  docker-capability-discovery
  docker-error-recovery

Environment overrides:
  EVAL_SURFACE_MODELS       Comma-separated provider:model list.
  EVAL_SURFACE_ENTERPRISE   Set to true to use GitLab EE plus Enterprise-only presets.
  EVAL_SURFACE_CASE_SET     Case set to run: ce, enterprise, or all. Defaults to ce for CE runtime and enterprise for Enterprise runtime.
  EVAL_SURFACE_FIXTURE_SMOKE Set true to prepare and smoke-test fixtures for selected presets without model calls.
  EVAL_SURFACE_PUBLISH_DOCS Set false to skip README/docs publication for full multi-preset runs.
  EVAL_SURFACE_OUT_ROOT     Artifact root (default: dist/evaluation/surfaces).
  EVAL_SURFACE_RUN_DIR      Exact artifact directory for this run.
  EVAL_SURFACE_TIMESTAMP    UTC-like timestamp used in names.
  EVAL_SURFACE_PRESET       Single Docker preset to run.
  EVAL_SURFACE_KEEP_DOCKER  Set to 1 to leave Docker GitLab running.
  EVAL_DOCKER_GITLAB_IMAGE  GitLab image override (defaults to EE in Enterprise mode).
  PRESET                    Make-friendly alias for EVAL_SURFACE_PRESET.
  ENTERPRISE                Make-friendly alias for EVAL_SURFACE_ENTERPRISE.
  GO_BIN                    Go binary to use (default: go).
  DOCKER_COMPOSE            Compose command (default: docker compose).
USAGE
}

bool_enabled() {
  case "$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|y|on) return 0 ;;
    *) return 1 ;;
  esac
}

dotenv_value() {
  local key="$1"
  local file=".env"
  [[ -f "$file" ]] || return 0
  KEY="$key" ENV_PATH="$file" python3 -c 'import os
key = os.environ.get("KEY", "")
path = os.environ.get("ENV_PATH", "")
try:
    with open(path, "r", encoding="utf-8") as handle:
        for raw in handle:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            name, value = line.split("=", 1)
            name = name.strip()
            if name.startswith("export "):
                name = name.split(None, 1)[1].strip()
            if name != key:
                continue
            print(value.strip().strip("\"").strip(chr(39)))
            break
except OSError:
    pass
' 2>/dev/null || true
}

activation_code_value() {
  if enterprise_license_file_has_value; then
    return 0
  fi
  local value="${GITLAB_ACTIVATION_CODE:-}"
  if [[ -z "$value" ]]; then
    value="$(dotenv_value GITLAB_ACTIVATION_CODE)"
  fi
  if [[ -z "$value" ]]; then
    value="$(dotenv_value ENTERPRISE_LICENSE)"
  fi
  if [[ "$value" =~ ^[[:alnum:]]{24}$ ]]; then
    printf '%s' "$value"
  fi
}

enterprise_license_file_has_value() {
  local file="${E2E_ENTERPRISE_LICENSE_FILE:-test/e2e/.enterprise-license}"
  [[ -f "$file" ]] || return 1
  python3 - "$file" <<'PY'
import pathlib
import sys

try:
    raise SystemExit(0 if pathlib.Path(sys.argv[1]).read_text(errors="replace").strip() else 1)
except OSError:
    raise SystemExit(1)
PY
}

surface="${1:-${SURFACE:-}}"
if [[ -z "$surface" || "$surface" == "-h" || "$surface" == "--help" ]]; then
  usage
  exit 0
fi

case "$surface" in
  dynamic|meta) ;;
  *)
    echo "ERROR: surface must be dynamic or meta (got: $surface)" >&2
    exit 1
    ;;
esac

timestamp="${EVAL_SURFACE_TIMESTAMP:-$(date -u +%Y%m%d-%H%M%S)}"
output_root="${EVAL_SURFACE_OUT_ROOT:-dist/evaluation/surfaces}"
gitlab_url="${EVAL_DOCKER_GITLAB_URL:-http://localhost:8929}"
compose_file="${EVAL_DOCKER_COMPOSE_FILE:-test/e2e/docker-compose.yml}"
go_bin="${GO_BIN:-go}"
default_models="${EVAL_SURFACE_DEFAULT_MODELS:-anthropic:claude-haiku-4-5-20251001,google:gemini-flash-latest,openai:gpt-5.4-nano,qwen:qwen3.6-flash}"
models="${EVAL_SURFACE_MODELS:-${EVAL_MODELS:-$default_models}}"
requested_preset="${2:-${EVAL_SURFACE_PRESET:-${PRESET:-}}}"
fixture_smoke=false
if bool_enabled "${EVAL_SURFACE_FIXTURE_SMOKE:-${FIXTURE_SMOKE:-}}"; then
  fixture_smoke=true
fi
enterprise=false
if bool_enabled "${EVAL_SURFACE_ENTERPRISE:-${ENTERPRISE:-}}"; then
  enterprise=true
fi
if [[ "$requested_preset" == docker-enterprise-* ]]; then
  enterprise=true
fi
case_set="${EVAL_SURFACE_CASE_SET:-${CASE_SET:-}}"
if [[ -z "$case_set" ]]; then
  if [[ -n "$requested_preset" && "$requested_preset" != docker-enterprise-* ]]; then
    case_set="ce"
  elif [[ "$enterprise" == "true" ]]; then
    case_set="enterprise"
  else
    case_set="ce"
  fi
fi
case "$case_set" in
  ce|enterprise|all) ;;
  *)
    echo "ERROR: EVAL_SURFACE_CASE_SET must be ce, enterprise, or all (got: $case_set)" >&2
    exit 1
    ;;
esac
if [[ "$case_set" == "enterprise" || "$case_set" == "all" ]]; then
  enterprise=true
fi
runtime_label="CE"
case_label="CE"
image_default="${GITLAB_IMAGE:-}"
if [[ "$enterprise" == "true" ]]; then
  runtime_label="Enterprise"
  image_default="${image_default:-gitlab/gitlab-ee:latest}"
fi
gitlab_image="${EVAL_DOCKER_GITLAB_IMAGE:-$image_default}"
case "$case_set" in
  ce)
    all_presets=(docker-read docker-mutating-safe docker-destructive-safe docker-capability-discovery docker-error-recovery)
    if [[ "$enterprise" == "true" ]]; then
      case_label="CE-on-Enterprise"
    fi
    ;;
  enterprise)
    all_presets=(docker-enterprise-read docker-enterprise-mutating-safe docker-enterprise-destructive-safe)
    case_label="Enterprise"
    ;;
  all)
    all_presets=(docker-read docker-mutating-safe docker-destructive-safe docker-capability-discovery docker-error-recovery docker-enterprise-read docker-enterprise-mutating-safe docker-enterprise-destructive-safe)
    case_label="CE+Enterprise-on-Enterprise"
    ;;
esac
presets=("${all_presets[@]}")
run_all_presets=1

if [[ -n "$requested_preset" ]]; then
  case "$requested_preset" in
    docker-read|docker-mutating-safe|docker-destructive-safe|docker-enterprise-read|docker-enterprise-mutating-safe|docker-enterprise-destructive-safe|docker-capability-discovery|docker-error-recovery)
      if [[ "$case_set" == "ce" && "$requested_preset" == docker-enterprise-* ]]; then
        echo "ERROR: ce case set does not accept Enterprise presets (got: $requested_preset)" >&2
        exit 1
      fi
      if [[ "$case_set" == "enterprise" && "$requested_preset" != docker-enterprise-* ]]; then
        echo "ERROR: enterprise case set only accepts docker-enterprise-* presets (got: $requested_preset)" >&2
        exit 1
      fi
      presets=("$requested_preset")
      run_all_presets=0
      ;;
    *)
      echo "ERROR: preset must be one of: ${all_presets[*]} (got: $requested_preset)" >&2
      exit 1
      ;;
  esac
fi

run_name="${timestamp}-${surface}-docker"
if [[ "$enterprise" == "true" && "$case_set" == "ce" ]]; then
  run_name="${timestamp}-${surface}-docker-enterprise-ce"
elif [[ "$enterprise" == "true" && "$case_set" == "all" ]]; then
  run_name="${timestamp}-${surface}-docker-enterprise-all"
fi
run_dir="${EVAL_SURFACE_RUN_DIR:-$output_root/$run_name}"
log_dir="$run_dir/logs"
fixtures="$run_dir/e2e-fixtures.json"

mkdir -p "$log_dir"

read -r -a compose_command <<< "${DOCKER_COMPOSE:-docker compose}"
compose=("${compose_command[@]}" -f "$compose_file")

compose_env=()
if [[ -n "$gitlab_image" ]]; then
  export GITLAB_IMAGE="$gitlab_image"
fi
if [[ "$enterprise" == "true" ]]; then
  activation_code="$(activation_code_value)"
  if [[ -n "$activation_code" ]]; then
    export GITLAB_ACTIVATION_CODE="$activation_code"
    printf 'Enterprise activation code: provided to GitLab EE container\n'
  elif enterprise_license_file_has_value; then
    printf 'Enterprise license cache: will install cached license during setup\n'
  fi
fi

reports=()
cleanup_started=0

run_logged() {
  local name="$1"
  shift
  local log="$log_dir/${name}.log"
  printf '==> %s\n' "$name"
  {
    printf '$'
    printf ' %q' "$@"
    printf '\n'
  } > "$log"
  "$@" >> "$log" 2>&1
}

cleanup() {
  if [[ "${EVAL_SURFACE_KEEP_DOCKER:-}" == "1" ]]; then
    printf '==> docker-cleanup skipped (EVAL_SURFACE_KEEP_DOCKER=1)\n'
    return
  fi
  if [[ "$cleanup_started" == "0" ]]; then
    return
  fi
  local log="$log_dir/docker-down-final.log"
  printf '==> docker-down-final\n'
  "${compose[@]}" down -v > "$log" 2>&1 || {
    local status=$?
    echo "WARN: final Docker cleanup failed; see $log" >&2
    return "$status"
  }
}
trap cleanup EXIT

retry_setup_gitlab() {
  local log="$log_dir/setup-gitlab.log"
  printf '==> setup-gitlab\n'
  {
    for attempt in 1 2 3; do
      if GITLAB_ENTERPRISE="$enterprise" ./test/e2e/scripts/setup-gitlab.sh "$gitlab_url"; then
        return 0
      fi
      if [[ "$attempt" == "3" ]]; then
        echo "ERROR: setup-gitlab.sh failed after 3 attempts"
        return 1
      fi
      echo "WARN: setup-gitlab.sh failed (attempt $attempt/3), retrying in 5s..."
      sleep 5
    done
  } > "$log" 2>&1
}

run_evaluator() {
  local name="$1"
  shift
  run_logged "$name" env "GITLAB_ENTERPRISE=$enterprise" "$go_bin" run ./cmd/eval_mcp_surfaces --docker-auto-start=false "$@"
}

run_fixture_smoke() {
  local name="$1"
  local preset="$2"
  local report="$3"
  run_evaluator "$name" \
    --tool-surface "$surface" \
    --edition "$(preset_edition_arg "$preset")" \
    --preset "$preset" \
    --backend gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --dry-run \
    --fixture-smoke \
    --fixtures "$fixtures" \
    --use-fixtures \
    --execute-tools \
    --skip-unavailable \
    --out "$report" \
    --terminal-log "$log_dir/${name}.terminal.log"
}

check_report_clean() {
  local name="$1"
  local report="$2"
  run_logged "$name" env "GITLAB_ENTERPRISE=$enterprise" "$go_bin" run ./cmd/eval_mcp_surfaces --docker-auto-start=false --check-report-clean "$report"
}

preset_edition_arg() {
  # When case_set is "all", the case registry mixes CE and Enterprise
  # cases (e.g. MS-ENT-DYN-9/-10 in docker-error-recovery and
  # docker-capability-discovery). Forcing --edition=ce would filter
  # those out. Use --edition=all so the per-preset filter
  # (e.g. error-recovery trait) decides which cases run.
  if [[ "$case_set" == "all" ]]; then
    printf '%s' all
    return
  fi
  case "$1" in
    docker-enterprise-*) printf '%s' enterprise ;;
    *) printf '%s' ce ;;
  esac
}

prepare_fixtures() {
  local name="$1"
  local preset="$2"
  run_evaluator "$name" \
    --tool-surface "$surface" \
    --edition "$(preset_edition_arg "$preset")" \
    --preset "$preset" \
    --backend gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --dry-run \
    --prepare-fixtures \
    --fixtures-only \
    --fixtures "$fixtures" \
    --out "$run_dir/${timestamp}-${surface}-${name}.md" \
    --terminal-log "$log_dir/${name}.terminal.log"
}

prepare_preset_fixtures() {
  local name="$1"
  local preset="$2"
  for attempt in 1 2; do
    if prepare_fixtures "$name" "$preset"; then
      return 0
    fi
    if [[ "$attempt" == "2" ]]; then
      break
    fi
    printf 'WARN: %s fixture preparation failed for %s (attempt %s/2), retrying...\n' "$name" "$preset" "$attempt" >&2
  done
  return 1
}

printf 'Evaluation artifacts: %s\n' "$run_dir"
printf 'Surface: %s\n' "$surface"
printf 'Runtime edition: %s\n' "$runtime_label"
printf 'Case set: %s\n' "$case_label"
if [[ -n "$gitlab_image" ]]; then
  printf 'GitLab image: %s\n' "$gitlab_image"
fi
if [[ "$run_all_presets" == "1" ]]; then
  printf 'Presets: %s\n' "${presets[*]}"
else
  printf 'Preset: %s\n' "${presets[0]}"
fi
if [[ "$fixture_smoke" == "true" ]]; then
  printf 'Mode: fixture smoke only\n'
fi
printf 'Models: %s\n' "$models"

run_logged docker-down-initial "${compose_env[@]}" "${compose[@]}" down -v
run_logged docker-up "${compose_env[@]}" "${compose[@]}" up -d
cleanup_started=1
run_logged wait-for-gitlab ./test/e2e/scripts/wait-for-gitlab.sh "$gitlab_url" 600
retry_setup_gitlab
run_logged register-runner ./test/e2e/scripts/register-runner.sh "$gitlab_url"

if ! prepare_preset_fixtures fixtures-prepare "${presets[0]}"; then
  echo "ERROR: initial fixture preparation failed for ${presets[0]}; see $log_dir/fixtures-prepare.log" >&2
  exit 1
fi

preset_status=0
status_file="$run_dir/full-run-status.txt"
if [[ "$run_all_presets" == "0" ]]; then
  status_file="$run_dir/preset-run-status.txt"
fi
: > "$status_file"
if [[ "$fixture_smoke" == "true" ]]; then
  status_file="$run_dir/fixture-smoke-status.txt"
  : > "$status_file"
  for preset in "${presets[@]}"; do
    if [[ "$enterprise" == "true" && "$run_all_presets" == "1" && "$preset" != "${presets[0]}" ]]; then
      if ! prepare_preset_fixtures "fixtures-prepare-${preset}" "$preset"; then
        printf '%s: fixture-prepare-failed\n' "$preset" | tee -a "$status_file"
        preset_status=1
        continue
      fi
    fi
    report="$run_dir/${timestamp}-${surface}-${preset}-fixture-smoke.md"
    printf '=== fixture-smoke %s ===\n' "$preset" | tee -a "$status_file"
    if run_fixture_smoke "fixture-smoke-${preset}" "$preset" "$report"; then
      if check_report_clean "report-clean-fixture-smoke-${preset}" "$report"; then
        printf '%s: process_ok report_clean\n' "$preset" | tee -a "$status_file"
      else
        printf '%s: process_ok report_failed\n' "$preset" | tee -a "$status_file"
        preset_status=1
      fi
    else
      printf '%s: process_failed report_unknown\n' "$preset" | tee -a "$status_file"
      preset_status=1
    fi
  done
  if [[ "$preset_status" -ne 0 ]]; then
    echo "ERROR: one or more fixture smoke presets failed. See $status_file" >&2
    exit "$preset_status"
  fi
  printf 'Fixture smoke complete: %s\n' "$run_dir"
  exit 0
fi
for preset in "${presets[@]}"; do
  if [[ "$enterprise" == "true" && "$run_all_presets" == "1" && "$preset" != "${presets[0]}" ]]; then
    if ! prepare_preset_fixtures "fixtures-prepare-${preset}" "$preset"; then
      printf '%s: fixture-prepare-failed\n' "$preset" | tee -a "$status_file"
      preset_status=1
      continue
    fi
  fi
  report="$run_dir/${timestamp}-${surface}-${preset}-all-models.md"
  reports+=("$report")
  printf '=== %s ===\n' "$preset" | tee -a "$status_file"
  if run_evaluator "$preset" \
    --tool-surface "$surface" \
    --edition "$(preset_edition_arg "$preset")" \
    --preset "$preset" \
    --models "$models" \
    --backend gitlab \
    --gitlab-env-file test/e2e/.env.docker \
    --fixtures "$fixtures" \
    --use-fixtures \
    --execute-tools \
    --skip-unavailable \
    --out "$report" \
    --terminal-log "$log_dir/${preset}.terminal.log"; then
    if check_report_clean "report-clean-${preset}" "$report"; then
      printf '%s: process_ok report_clean\n' "$preset" | tee -a "$status_file"
    else
      printf '%s: process_ok report_failed\n' "$preset" | tee -a "$status_file"
      preset_status=1
    fi
  else
    printf '%s: process_failed report_unknown\n' "$preset" | tee -a "$status_file"
    preset_status=1
  fi
done

if [[ "$preset_status" -ne 0 ]]; then
  echo "ERROR: one or more presets failed; docs were not published. See $status_file" >&2
  exit "$preset_status"
fi

if [[ "$run_all_presets" == "0" ]]; then
  printf 'Preset evaluation complete: %s\n' "$run_dir"
  printf 'Report: %s\n' "${reports[0]}"
  printf 'Docs publish skipped for single-preset run.\n'
  exit 0
fi

publish_docs="true"
if [[ "$enterprise" == "true" && "$case_set" != "enterprise" ]]; then
  publish_docs="false"
fi
if [[ -n "${EVAL_SURFACE_PUBLISH_DOCS:-}" ]]; then
  if bool_enabled "$EVAL_SURFACE_PUBLISH_DOCS"; then
    publish_docs="true"
  else
    publish_docs="false"
  fi
fi
if [[ "$publish_docs" != "true" ]]; then
  printf 'Evaluation complete: %s\n' "$run_dir"
  printf 'Docs publish skipped for %s case set on %s runtime.\n' "$case_label" "$runtime_label"
  exit 0
fi

publish_args=(
  --publish-docs
  --publish-label "Docker $case_label $surface $timestamp"
  --publish-mode replace-current
  --terminal-log "$log_dir/publish-docs.terminal.log"
)
for report in "${reports[@]}"; do
  publish_args+=(--publish-from "$report")
done
run_evaluator publish-docs "${publish_args[@]}"
run_logged format-md-tables "$go_bin" run ./cmd/format_md_tables/

printf 'Evaluation complete: %s\n' "$run_dir"
