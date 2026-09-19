#!/usr/bin/env bash
# Refuse an em dash (U+2014) in the text a pull request lands in main, and
# refuse a review bot's generated block in its description.
#
# Two surfaces, two subcommands, because each becomes permanent in a different
# way and only one of them can still be cleaned by a later commit:
#
#   diff         The lines this branch ADDS, judged net over the branch rather
#                than per commit, so a branch that introduces an em dash and
#                removes it again at its tip passes. A whole-tree check is not
#                an option: the tree already carries some five thousand of them
#                in prose written before the rule, so it would be red on its
#                first run and switched off on its second.
#
#   description  The pull request title and body. This repository squash
#                merges, so the description is the commit message that lands on
#                main, and no later commit can edit it out of the history. The
#                description is read from the API at the moment this runs
#                rather than from the workflow event payload: the payload is a
#                snapshot taken when the run was queued, and a review bot that
#                reinjects its generated block while the run is in flight does
#                not appear in it.
#
# The character is built from its bytes below rather than written out, so this
# script does not carry the thing it refuses.

set -euo pipefail

EM_DASH=$(printf '\xe2\x80\x94')

# Paths whose added lines are not judged, each with its reason. A generated
# record that quotes GitLab's own prose is not this repository's writing:
# docs/development/gitlab-api-live.json holds the parameter descriptions a
# booted GitLab reported, one of which already contains an em dash, so a
# re-take of that record must not fail this gate. A declaration that no longer
# names a file in the tree is a finding of its own, on the terms every
# declaration table in this repository is held to.
# Paths whose added lines are not this branch's prose. The API record is
# machine-written from a booted GitLab. The rest are generated documents that
# copy package doc comments verbatim, so the roughly two thousand em dashes
# already in the tree reach them the moment a branch regenerates one:
# docs/development/testing/testing.md carries the one in
# internal/gatewaycompat/doc.go today. Failing a branch for regenerating an
# artifact it did not write is how a gate teaches people to route around it.
# README.md is deliberately not here, being mostly hand-written; an em dash
# reaching its generated block would be a finding about the generator.
EXCLUDED_PATHS=(
  "docs/development/gitlab-api-live.json"
  "docs/development/testing/testing.md"
  "docs/development/testing/e2e-coverage.md"
  "llms.txt"
  "llms-full.txt"
)

# Markers a review bot writes around the block it injects into a pull request
# description. The HTML comments are the vendor-neutral half and the ones that
# matter: a bot re-finds its own block by them, which is exactly why the block
# comes back after an author deletes it. The heading names CodeRabbit, which is
# the bot this repository runs, because "Summary by <something>" is a heading a
# human might reasonably write.
INJECTED_BLOCK_PATTERNS=(
  '<!--[[:space:]]*(this is an[[:space:]]+)?auto-generated comment'
  '<!--[[:space:]]*end of auto-generated comment'
  '<!--[[:space:]]*(walkthrough|description|summary|changed_files)_(start|end)[[:space:]]*-->'
  '^#{1,6}[[:space:]]*summary by coderabbit'
)

usage() {
  cat <<'USAGE'
Usage:
  scripts/check-em-dash.sh diff [BASE_REF]
  scripts/check-em-dash.sh description

diff         Fail when a line this branch adds carries an em dash (U+2014).
             BASE_REF defaults to $EM_DASH_BASE, then to origin/main. The
             comparison is the three-dot form, so only what this branch added
             on top of their merge base is judged.

description  Fail when the pull request title or body carries an em dash, or
             when the body carries a generated block a review bot injected.
             The pull request is $PR_NUMBER, or the one open for the current
             branch. Set PR_TITLE_FILE and PR_BODY_FILE to judge text from
             disk instead, which is how the gate is rehearsed without a pull
             request.
USAGE
}

# GitHub renders an annotation against the file and line it names, so a finding
# lands on the diff the author is looking at and not only in a log.
annotate() {
  [[ -n "${GITHUB_ACTIONS:-}" ]] || return 0
  printf '::error file=%s,line=%s::%s\n' "$1" "$2" "$3"
}

# A declaration that excuses nothing is stale, and a stale one is how a real
# finding gets excused later by accident.
check_declarations() {
  local path stale=0
  for path in "${EXCLUDED_PATHS[@]}"; do
    if [[ ! -e "$path" ]]; then
      echo "ERROR: the exclusion list names $path, which is not in the tree." >&2
      stale=1
    fi
  done
  if [[ "$stale" -ne 0 ]]; then
    echo "Remove the entry from EXCLUDED_PATHS in $0, or restore the file." >&2
    return 1
  fi
}

cmd_diff() {
  local base="${1:-${EM_DASH_BASE:-origin/main}}"

  check_declarations

  if ! git rev-parse --verify --quiet "$base^{commit}" > /dev/null; then
    echo "ERROR: cannot resolve the base commit '$base'." >&2
    echo "Fetch it (git fetch origin main), or name another: make check-em-dash EM_DASH_BASE=<ref>." >&2
    return 1
  fi

  local pathspecs=(":(top)") path
  for path in "${EXCLUDED_PATHS[@]}"; do
    pathspecs+=(":(top,exclude)$path")
  done

  # Explicit prefixes and no external diff driver: a developer with
  # diff.noprefix, diff.mnemonicPrefix or a textconv filter configured would
  # otherwise hand this parser a shape it cannot read, and a gate that reads
  # nothing passes everything.
  local findings
  findings=$(
    git -c core.quotePath=false diff --no-color --no-ext-diff --no-textconv \
      -U0 --src-prefix=a/ --dst-prefix=b/ "$base...HEAD" -- "${pathspecs[@]}" \
      | LC_ALL=C EM_DASH="$EM_DASH" awk '
        BEGIN { OFS = ":"; dash = ENVIRON["EM_DASH"]; path = ""; lineno = 0; inhunk = 0 }
        # A file header only ever precedes the first hunk of its file, so the
        # hunk state is what tells it from an added line whose own text starts
        # with "++ ". Without that, such a line reads as a header, sets path to
        # the empty string, and every added line after it in the file is
        # skipped: the gate would fail open and say nothing, which is the one
        # way for a gate to be worse than absent.
        /^diff --git / { path = ""; inhunk = 0; next }
        !inhunk && /^\+\+\+ / {
          path = ($0 == "+++ /dev/null") ? "" : substr($0, 7)
          next
        }
        /^@@ / {
          match($0, /\+[0-9]+/)
          lineno = substr($0, RSTART + 1, RLENGTH - 1) + 0
          inhunk = 1
          next
        }
        /^\+/ {
          if (path != "" && index($0, dash) > 0) { print path, lineno, substr($0, 2) }
          lineno++
          next
        }
        /^-/ { next }
        /^ / { lineno++; next }
      '
  )

  if [[ -z "$findings" ]]; then
    echo "OK: no line added between $base and HEAD carries an em dash."
    return 0
  fi

  local file line text count=0
  while IFS=: read -r file line text; do
    printf '%s:%s: %s\n' "$file" "$line" "$text"
    annotate "$file" "$line" "this line adds an em dash (U+2014); rewrite it"
    count=$((count + 1))
  done <<< "$findings"

  cat >&2 <<EOF

FAIL: $count added line(s) carry an em dash (U+2014).

Nothing written here uses one: not code, not a comment, not documentation, not
a commit message. Rewrite each line above. A comma, a colon, a full stop or a
pair of parentheses says the same thing, and the sentence usually reads better
cut in two.

Only the lines this branch adds are judged, so prose that was already there is
not yours to fix. Re-check with:

    make check-em-dash
EOF
  return 1
}

# Prefix every hit with the field it came from, so a multi-line body reads as a
# list of locations rather than one blob under one label.
label_hits() {
  awk -v label="$1" '{ print label ":" $0 }'
}

# Where the description came from is printed on every run, because a gate that
# silently judged the wrong text is worse than one that did not run at all.
read_description() {
  if [[ -n "${PR_BODY_FILE:-}" || -n "${PR_TITLE_FILE:-}" ]]; then
    PR_SOURCE="files on disk"
    PR_TITLE=""
    PR_BODY=""
    [[ -z "${PR_TITLE_FILE:-}" ]] || PR_TITLE=$(cat "$PR_TITLE_FILE")
    [[ -z "${PR_BODY_FILE:-}" ]] || PR_BODY=$(cat "$PR_BODY_FILE")
    return 0
  fi

  if ! command -v gh > /dev/null 2>&1; then
    echo "ERROR: gh is needed to read the pull request description." >&2
    echo "Install it, or set PR_TITLE_FILE and PR_BODY_FILE to judge text from disk." >&2
    return 1
  fi

  local repo="${GITHUB_REPOSITORY:-}"
  [[ -n "$repo" ]] || repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)

  local number="${PR_NUMBER:-}"
  if [[ -z "$number" ]]; then
    number=$(gh pr view --json number --jq .number 2> /dev/null || echo "")
  fi
  if [[ -z "$number" ]]; then
    echo "No pull request is open for this branch, so there is no description to judge." >&2
    echo "Name one with PR_NUMBER=<n>, or rehearse with PR_BODY_FILE=<path>." >&2
    return 2
  fi

  # One request, answered now rather than read out of the event payload: see
  # the header for why the difference matters.
  local payload
  payload=$(gh api "repos/$repo/pulls/$number")
  PR_SOURCE="$repo#$number, read from the API just now"
  PR_TITLE=$(jq -r '.title // ""' <<< "$payload")
  PR_BODY=$(jq -r '.body // ""' <<< "$payload")
}

cmd_description() {
  local PR_SOURCE PR_TITLE PR_BODY status=0
  read_description || {
    status=$?
    # Nothing to judge is not a failure: the diff half still gates, and a
    # branch with no pull request yet has no description to land in main.
    [[ "$status" -eq 2 ]] && return 0
    return "$status"
  }

  echo "Judging the pull request title and body ($PR_SOURCE)."

  local em_dashed=0 injected=0 hits pattern

  if [[ "$PR_TITLE" == *"$EM_DASH"* ]]; then
    printf 'title: %s\n' "$PR_TITLE"
    em_dashed=1
  fi

  hits=$(printf '%s\n' "$PR_BODY" | { grep -nF -- "$EM_DASH" || true; } | label_hits body)
  if [[ -n "$hits" ]]; then
    printf '%s\n' "$hits"
    em_dashed=1
  fi

  if [[ "$em_dashed" -ne 0 ]]; then
    cat >&2 <<'EOF'

FAIL: the pull request title or body carries an em dash (U+2014).

Edit the pull request description itself, not a commit. This repository squash
merges, so the description becomes the commit message on main and no later
commit can take it back out:

    gh pr edit <number>
EOF
  fi

  # One pattern per marker, but the findings are reported in the body's own
  # order: an opening marker read after its closing one is a puzzle rather than
  # a location.
  local markers=""
  for pattern in "${INJECTED_BLOCK_PATTERNS[@]}"; do
    hits=$(printf '%s\n' "$PR_BODY" | { grep -nEi -- "$pattern" || true; } | label_hits body)
    [[ -z "$hits" ]] || markers+="$hits"$'\n'
  done
  if [[ -n "$markers" ]]; then
    printf '%s' "$markers" | sort -u -t: -k2,2n
    injected=1
  fi

  if [[ "$injected" -ne 0 ]]; then
    cat >&2 <<'EOF'

FAIL: the pull request description carries a block a review bot generated.

Delete the block named above and everything down to its closing marker, then
edit the description:

    gh pr edit <number>

The bot reinjects the block after an edit, which is why this step reads the
description live instead of trusting the workflow event payload, and why it
runs at the end of the pipeline rather than at the start. Strip it before the
merge: a squash merge copies the description into main's history, and editing
the pull request afterwards does not reach the commit.
EOF
  fi

  if [[ "$em_dashed" -ne 0 || "$injected" -ne 0 ]]; then
    echo >&2
    echo "Re-check with: make check-pr-description" >&2
    return 1
  fi

  echo "OK: the title and body carry no em dash and no injected block."
}

main() {
  local subcommand="${1:-}"
  shift || true
  case "$subcommand" in
    diff) cmd_diff "$@" ;;
    description) cmd_description "$@" ;;
    -h | --help | help) usage ;;
    *)
      usage >&2
      return 2
      ;;
  esac
}

main "$@"
