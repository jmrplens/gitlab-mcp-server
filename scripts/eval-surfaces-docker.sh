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

Environment overrides:
  EVAL_SURFACE_MODELS       Comma-separated provider:model list.
  EVAL_SURFACE_ENTERPRISE   Set to true to use GitLab EE plus Enterprise presets.
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
run_dir="${EVAL_SURFACE_RUN_DIR:-$output_root/${timestamp}-${surface}-docker}"
log_dir="$run_dir/logs"
fixtures="$run_dir/e2e-fixtures.json"
gitlab_url="${EVAL_DOCKER_GITLAB_URL:-http://localhost:8929}"
compose_file="${EVAL_DOCKER_COMPOSE_FILE:-test/e2e/docker-compose.yml}"
go_bin="${GO_BIN:-go}"
models="${EVAL_SURFACE_MODELS:-${EVAL_MODELS:-anthropic:claude-haiku-4-5-20251001,google:gemini-3.1-flash-lite-preview,openai:gpt-5.4-nano,qwen:qwen3.6-flash}}"
requested_preset="${2:-${EVAL_SURFACE_PRESET:-${PRESET:-}}}"
enterprise=false
if bool_enabled "${EVAL_SURFACE_ENTERPRISE:-${ENTERPRISE:-}}"; then
  enterprise=true
fi
if [[ "$requested_preset" == docker-enterprise-* ]]; then
  enterprise=true
fi
edition_label="CE"
image_default="${GITLAB_IMAGE:-}"
if [[ "$enterprise" == "true" ]]; then
  edition_label="Enterprise"
  image_default="${image_default:-gitlab/gitlab-ee:latest}"
fi
gitlab_image="${EVAL_DOCKER_GITLAB_IMAGE:-$image_default}"
if [[ "$enterprise" == "true" ]]; then
  all_presets=(docker-enterprise-read docker-enterprise-mutating-safe docker-enterprise-destructive-safe docker-capability-discovery)
else
  all_presets=(docker-read docker-mutating-safe docker-destructive-safe docker-capability-discovery)
fi
presets=("${all_presets[@]}")
run_all_presets=1

if [[ -n "$requested_preset" ]]; then
  case "$requested_preset" in
    docker-read|docker-mutating-safe|docker-destructive-safe|docker-enterprise-read|docker-enterprise-mutating-safe|docker-enterprise-destructive-safe|docker-capability-discovery)
      presets=("$requested_preset")
      run_all_presets=0
      ;;
    *)
      echo "ERROR: preset must be one of: ${all_presets[*]} (got: $requested_preset)" >&2
      exit 1
      ;;
  esac
fi

mkdir -p "$log_dir"

read -r -a compose_command <<< "${DOCKER_COMPOSE:-docker compose}"
compose=("${compose_command[@]}" -f "$compose_file")

compose_env=()
if [[ -n "$gitlab_image" ]]; then
  compose_env=(env "GITLAB_IMAGE=$gitlab_image")
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
  run_logged "$name" env "GITLAB_ENTERPRISE=$enterprise" "$go_bin" run ./cmd/eval_mcp_surfaces "$@"
}

prepare_fixtures() {
  local name="$1"
  local preset="$2"
  run_evaluator "$name" \
    --tool-surface "$surface" \
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

printf 'Evaluation artifacts: %s\n' "$run_dir"
printf 'Surface: %s\n' "$surface"
printf 'Edition: %s\n' "$edition_label"
if [[ -n "$gitlab_image" ]]; then
  printf 'GitLab image: %s\n' "$gitlab_image"
fi
if [[ "$run_all_presets" == "1" ]]; then
  printf 'Presets: %s\n' "${presets[*]}"
else
  printf 'Preset: %s\n' "${presets[0]}"
fi
printf 'Models: %s\n' "$models"

run_logged docker-down-initial "${compose_env[@]}" "${compose[@]}" down -v
run_logged docker-up "${compose_env[@]}" "${compose[@]}" up -d
cleanup_started=1
run_logged wait-for-gitlab ./test/e2e/scripts/wait-for-gitlab.sh "$gitlab_url" 600
retry_setup_gitlab
run_logged register-runner ./test/e2e/scripts/register-runner.sh "$gitlab_url"

prepare_fixtures fixtures-prepare "${presets[0]}"

preset_status=0
status_file="$run_dir/full-run-status.txt"
if [[ "$run_all_presets" == "0" ]]; then
  status_file="$run_dir/preset-run-status.txt"
fi
: > "$status_file"
for preset in "${presets[@]}"; do
  if [[ "$enterprise" == "true" && "$run_all_presets" == "1" && "$preset" != "${presets[0]}" ]]; then
    prepare_fixtures "fixtures-prepare-${preset}" "$preset"
  fi
  report="$run_dir/${timestamp}-${surface}-${preset}-all-models.md"
  reports+=("$report")
  printf '=== %s ===\n' "$preset" | tee -a "$status_file"
  if run_evaluator "$preset" \
    --tool-surface "$surface" \
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
    printf '%s: ok\n' "$preset" | tee -a "$status_file"
  else
    printf '%s: failed\n' "$preset" | tee -a "$status_file"
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

publish_args=(
  --publish-docs
  --publish-label "Docker $edition_label $surface $timestamp"
  --publish-mode replace-current
  --terminal-log "$log_dir/publish-docs.terminal.log"
)
for report in "${reports[@]}"; do
  publish_args+=(--publish-from "$report")
done
run_evaluator publish-docs "${publish_args[@]}"
run_logged format-md-tables "$go_bin" run ./cmd/format_md_tables/

printf 'Evaluation complete: %s\n' "$run_dir"
