#!/usr/bin/env bash
# Run gremlins over one package and make its verdicts mean what they say.
#
# gremlins decides which package to run the tests in by walking up from the
# mutated file's directory until a path component ends with the package
# clause's name, and falls back to the module path when none does
# (internal/engine/engine.go, func pkgName, v0.6.0). For `package main` in a
# directory that is not called main, nothing matches: it runs `go test` on the
# module path, this module's root holds no Go files, that exits 1, and exit 1
# is how gremlins recognises a killed mutant. Every covered mutant of every
# command under cmd/ is then reported KILLED without its tests ever running.
#
# Measured on cmd/audit_action_ids: the ordinary run says 134 killed, 0 lived,
# in 3.5 seconds, against a suite that takes 9. The same tree measured through
# a copy of the package in a directory whose name ends in "main" says 101
# killed, 33 lived, 2 not covered, in 10m49s. The coverage half is honest
# either way, since NOT COVERED comes from a real `go test -cover` over the
# real package; it is the kill verdict that is fabricated, so `Lived 0` from an
# unstaged run of a command says nothing at all.
#
# So this stages such a package into a directory inside the module whose name
# ends in "main", runs gremlins there, and removes it afterwards. The staged
# copy is a real package of this module, so the resolution lands on it and the
# tests that decide each verdict are the package's own. A package whose
# directory already ends with its package name needs none of this and is run
# where it is.
#
# What this does NOT do is paper over a staged run that cannot work. If the
# copy does not pass its own tests where it was staged, the run stops and says
# so rather than falling back to the unstaged run, because that run is the one
# that reports a clean package without measuring it. See issue 872.
#
# The other thing it decides is how long each mutant may run. gremlins has no
# setting for that: it multiplies --timeout-coefficient by the wall time of its
# own coverage run, `go test [-tags T] [-coverpkg P] -cover -coverprofile F
# ./<pkg>/...` from the module root. So the coefficient is derived here from a
# run of that same command under the same tags, timed by the clock, and held
# under a ceiling. Deriving it from anything else is how a package behind a
# build tag came to give each mutant a deadline of 95 hours. See issue 915.
set -euo pipefail

# bash writes the figure `time` reports with the locale's decimal separator,
# so under es_ES the baseline reads 0,940. The positive-number check below
# refuses that, so every run under a comma-decimal locale would stop. Without
# that check awk would read 0,940 as 0: the coefficient and the ceiling's cap
# would both hit the 6000 clamp, so MUTANT_DEADLINE_MAX would bound nothing,
# and gawk would fail on the division outright. C is the one locale every tool
# here reads the same way.
export LC_ALL=C

PKG=${1:?usage: coverage-mutants.sh <package> [budget] [floor] [ceiling]}
BUDGET=${2:-300}
FLOOR=${3:-10}
CEILING=${4:-3600}

# The three knobs are seconds, written as a plain number. awk reads the leading
# digits of anything else and drops the rest, so a ceiling written 2h would be
# read as 2, raised to the floor, and give every mutant about ten seconds, with
# a notice about the floor as the only sign of it.
for knob in "MUTANT_BUDGET=$BUDGET" "MUTANT_BUDGET_FLOOR=$FLOOR" "MUTANT_DEADLINE_MAX=$CEILING"; do
  if ! [[ ${knob#*=} =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "gremlins: ${knob} is not a number of seconds; MUTANT_BUDGET, MUTANT_BUDGET_FLOOR and MUTANT_DEADLINE_MAX (the script's second, third and fourth arguments) are all written as seconds, such as 300 or 3600" >&2
    exit 1
  fi
done

# The budget is a knob and the floor is what it may not go under: below a few
# seconds the per-mutant budget falls under the fixed cost of starting
# `go test` and every mutant is reported TIMED OUT having never run. The floor
# is applied out loud, or the printed budget is a second lie on top of that.
budget=$(awk -v want="$BUDGET" -v floor="$FLOOR" 'BEGIN{print (want+0 < floor+0) ? floor : want}')
if [ "$budget" != "$BUDGET" ]; then
  echo "gremlins: MUTANT_BUDGET=${BUDGET}s is under the ${FLOOR}s floor and would report untested mutants as timeouts; using ${budget}s"
fi

# The ceiling holds each mutant's deadline to about its value, so a mutant that
# makes the tests hang is reported TIMED OUT within about that long rather than
# holding a worker for as long as the coefficient allows. About, because the
# cap is derived from this script's baseline while gremlins multiplies the
# coefficient by its own coverage run: a run of that command a few percent
# slower than this one gives a deadline a few percent over the ceiling. It
# answers to the same floor as the budget, for the same reason: under it every
# mutant times out unrun.
ceiling=$(awk -v want="$CEILING" -v floor="$FLOOR" 'BEGIN{print (want+0 < floor+0) ? floor : want}')
if [ "$ceiling" != "$CEILING" ]; then
  echo "gremlins: MUTANT_DEADLINE_MAX=${CEILING}s is under the ${FLOOR}s floor and would report untested mutants as timeouts; using ${ceiling}s"
fi

# GREMLINS_FLAGS is split into words on purpose and then quoted, because its
# documented use carries a regular expression, and a caller passing `.*` to
# --exclude-files would otherwise have it expanded against the working
# directory before gremlins ever saw it.
read -r -a gremlins_flags <<<"${GREMLINS_FLAGS:-}"

# The baseline has to be a run of the command gremlins times, so it has to see
# what gremlins sees: the build tags, a -coverpkg, and whether --integration
# widens the run to the whole module. The tags and -coverpkg come from a flag
# or from gremlins' own environment binding, and a flag wins, so the
# environment is read first and the flags over it in order, the last of a
# repeated one winning as it does in pflag. They are read the way pflag reads
# them, shorthand clusters such as -dte2e included, because the measured
# failure was a tag that reached gremlins and not the baseline. A word this
# cannot read could be hiding a -t, so it is refused rather than guessed at. A
# tag set only in a .gremlins.yaml is out of reach, and the test-file check
# below stops such a run only where it would find no test file at all.
tags=${GREMLINS_UNLEASH_TAGS:-}
coverpkg=${GREMLINS_UNLEASH_COVERPKG:-}
excluded=${GREMLINS_UNLEASH_EXCLUDE_FILES:+yes}

# --integration is read from the flag alone, the one source this script can
# read that gremlins honors. gremlins binds GREMLINS_UNLEASH_INTEGRATION like
# the rest, but takes the value back with a bool type assertion
# (configuration.Get[bool], v0.6.0), and viper hands an environment value over
# as the string it was, so the assertion fails and gremlins runs one subtree
# whatever the variable says. Reading it here would time the whole module
# against a run of one package. A .gremlins.yaml can set it, since a YAML bool
# does pass that assertion, and that reaches gremlins and not this baseline,
# so an integration run is asked for with -i in GREMLINS_FLAGS.
integration=false
unreadable() {
  echo "gremlins: GREMLINS_FLAGS: cannot read $1 the way gremlins would, so the baseline could run under other build tags than gremlins does; refusing to measure" >&2
  exit 1
}
i=0
while [ "$i" -lt "${#gremlins_flags[@]}" ]; do
  word=${gremlins_flags[$i]}
  i=$((i + 1))
  case "$word" in
    --)
      break
      ;;
    --*)
      name=${word#--}
      value=""
      inline=""
      case "$name" in
        *=*) value=${name#*=}; name=${name%%=*}; inline=yes ;;
      esac
      # gremlins installs a pflag normalize function (cmd/unleash.go,
      # setFlagsOnCmd) that reads `_` and `.` in a flag name as `-`.
      name=${name//[._]/-}
      case "$name" in
        tags | coverpkg | exclude-files | output-statuses | diff | output | threshold-efficacy | threshold-mcover | workers | test-cpu | timeout-coefficient | config)
          if [ -z "$inline" ]; then
            [ "$i" -lt "${#gremlins_flags[@]}" ] || unreadable "$word"
            value=${gremlins_flags[$i]}
            i=$((i + 1))
          fi
          ;;
        *)
          [ -n "$inline" ] || value=true
          ;;
      esac
      case "$name" in
        tags) tags=$value ;;
        coverpkg) coverpkg=$value ;;
        integration) integration=$value ;;
        exclude-files) excluded=yes ;;
      esac
      ;;
    -?*)
      # gremlins' shorthands: d, i, the root command's persistent -s
      # (--silent) and cobra's -h are switches, the rest take a value, written
      # after `=`, joined to the letter, or as the next word.
      cluster=${word#-}
      while [ -n "$cluster" ]; do
        letter=${cluster:0:1}
        cluster=${cluster:1}
        value=true
        case "$letter" in
          d | i | s | h)
            if [ "${#cluster}" -gt 1 ] && [ "${cluster:0:1}" = "=" ]; then
              value=${cluster:1}
              cluster=""
            fi
            ;;
          S | t | D | o | E)
            if [ "${#cluster}" -gt 1 ] && [ "${cluster:0:1}" = "=" ]; then
              value=${cluster:1}
            elif [ -n "$cluster" ]; then
              value=$cluster
            elif [ "$i" -lt "${#gremlins_flags[@]}" ]; then
              value=${gremlins_flags[$i]}
              i=$((i + 1))
            else
              unreadable "$word"
            fi
            cluster=""
            ;;
          *)
            unreadable "$word"
            ;;
        esac
        case "$letter" in
          t) tags=$value ;;
          i) integration=$value ;;
          E) excluded=yes ;;
        esac
      done
      ;;
  esac
done
case "$integration" in
  1 | t | T | TRUE | true | True) integration=yes ;;
  *) integration="" ;;
esac
# Said once the flags are read, so the notice can say what decided the run.
if [ -n "${GREMLINS_UNLEASH_INTEGRATION:-}" ]; then
  if [ -n "$integration" ]; then
    echo "gremlins: GREMLINS_UNLEASH_INTEGRATION is set and gremlins v0.6.0 never reads it; this is an integration run because GREMLINS_FLAGS asks for one"
  else
    echo "gremlins: GREMLINS_UNLEASH_INTEGRATION is set, and gremlins v0.6.0 never reads it, so this is not an integration run; GREMLINS_FLAGS=-i makes one"
  fi
fi
tag_args=()
[ -z "$tags" ] || tag_args=(-tags "$tags")
cover_args=()
[ -z "$coverpkg" ] || cover_args=(-coverpkg "$coverpkg")

# The package is loaded under those tags, and -e keeps one go cannot load
# (every file behind a tag nobody passed, a path that is not there) from ending
# the script with go's own message, which does not say what to do about it.
#
# A package with no test file under its tags is refused unless the run is an
# integration run with a -coverpkg. Each mutant runs only this package's tests
# unless --integration is given, so without it no mutant of such a package can
# be killed, and without a -coverpkg nothing covers its blocks and every mutant
# is reported NOT COVERED. Before this check its baseline passed having run
# nothing, printed no duration, and handed gremlins a coefficient of 3001.
# Under both, the module's other tests cover its blocks and gremlins runs them
# against each mutant, so it is measured, and the run says so.
listing=$(go list -e ${tag_args[@]+"${tag_args[@]}"} -f '{{.Name}}
{{.Dir}}
{{len .TestGoFiles}} {{len .XTestGoFiles}}
{{with .Error}}{{.}}{{end}}' "$PKG")
{
  IFS= read -r pkgname
  IFS= read -r pkgdir
  read -r tests xtests
  listerr=$(cat)
} <<<"$listing"
refusal=""
if [ -n "$listerr" ]; then
  refusal="go cannot load $PKG under build tags ${tags:-(none)}: $listerr"
elif [ "$tests $xtests" = "0 0" ]; then
  if [ -n "$integration" ] && [ -n "$coverpkg" ]; then
    echo "gremlins: $PKG has no test files under build tags ${tags:-(none)}; measuring it through the module's other tests, which --integration runs against each mutant and -coverpkg $coverpkg lets cover it"
  else
    refusal="$PKG has no test files under build tags ${tags:-(none)}, so no mutant of it could be killed: each mutant runs only this package's tests without --integration, and without a -coverpkg every one is reported NOT COVERED; refusing to measure"
  fi
fi
if [ -n "$refusal" ]; then
  echo "gremlins: $refusal" >&2
  echo "gremlins: a package behind a build tag is measured with GREMLINS_FLAGS='--tags <tag>', which reaches this baseline and gremlins alike; a tag set only in a .gremlins.yaml reaches gremlins and not the baseline. One tested only from elsewhere in the module is measured with GREMLINS_FLAGS='-i --coverpkg <pattern>'" >&2
  exit 1
fi
root=$(go list -m -f '{{.Dir}}')

# The staging directory is removed whatever happens. Left behind it is a second
# copy of the package inside the module, which every tree-wide `go build ./...`
# and every gate that loads ./... would then read as real. The baseline's
# output and coverage profile go with it.
staged=""
out=""
profile=""
cleanup() {
  if [ -n "$staged" ] && [ -d "$staged" ]; then
    rm -rf "$staged"
  fi
  if [ -n "$out" ]; then
    rm -f "$out"
  fi
  if [ -n "$profile" ]; then
    rm -f "$profile"
  fi
}
trap cleanup EXIT INT TERM

target=$PKG
case "$(basename "$pkgdir")" in
  *"$pkgname")
    : # gremlins resolves this one on its own.
    ;;
  *)
    # The copy is a sibling of the original rather than somewhere tidy like
    # dist/, because Go's internal rule is about the path: a copy of a cmd/
    # command staged under dist/ cannot import cmd/internal/... and fails at
    # setup. Beside it, every import the package already makes is still legal.
    # ".mutants-" is in the name so the path cannot be one somebody meant to
    # keep, and an existing one is refused rather than removed: this script
    # deletes what it creates and nothing else, and a leftover means a previous
    # run was killed hard enough to skip its own trap, which a person should
    # see rather than have quietly overwritten.
    staged="$(dirname "$pkgdir")/$(basename "$pkgdir").mutants-${pkgname}"
    if [ -e "$staged" ]; then
      echo "gremlins: ${staged#"$root"/} is already there, which means a previous staged run did not clean up after itself; remove it and try again" >&2
      staged=""
      exit 1
    fi
    echo "gremlins: $PKG is package $pkgname in a directory that does not end in \"$pkgname\", which gremlins cannot resolve (issue 872); measuring a staged copy at ${staged#"$root"/}"
    cp -R "$pkgdir" "$staged"
    target="./${staged#"$root"/}"
    ;;
esac

# The run gremlins times: its coverage step, from the module root, over the
# target's subtree, or over the whole module under --integration.
dir=$pkgdir
[ -z "$staged" ] || dir=$staged
if [ -n "$integration" ] || [ "$dir" = "$root" ]; then
  scan=./...
else
  # go list prints native paths, which carry backslashes on Windows, and a
  # strip that expects a slash after the root strips nothing there and leaves a
  # pattern naming a directory that does not exist, which go test fails and
  # this script would then report as a failing suite. The path is cut after
  # the root whatever separator follows it and written with slashes, which go
  # reads on every platform.
  rel=${dir#"$root"}
  rel=${rel#[\\/]}
  scan="./${rel//\\//}/..."
fi
out=$(mktemp)
profile=$(mktemp)
baseline() {
  (cd "$root" && go test -count=1 ${tag_args[@]+"${tag_args[@]}"} ${cover_args[@]+"${cover_args[@]}"} \
    -cover -coverprofile "$profile" "$scan") >"$out" 2>&1
}

# The first run is the gate, and it is not timed. A package whose suite fails
# is refused rather than measured, because gremlins reads a failing test run as
# a killed mutant, so every mutant of it would read as killed. It also warms
# the build cache: gremlins downloads modules outside its own timer and times a
# coverage run whose compile the run before it already paid for, so the run
# timed here has to find the cache the way gremlins' will. Timing a first
# build instead makes the base too long on a package just edited, and the
# per-mutant deadline too short to run a mutant in.
(cd "$root" && go mod download)
if ! baseline; then
  cat "$out" >&2
  if [ -n "$staged" ]; then
    echo "gremlins: the staged copy of $PKG does not pass its own tests there, so the verdicts would be about the staging rather than the package; refusing to measure" >&2
    echo "gremlins: a test that reads its own directory or import path is the usual cause. Run it unstaged to see, and read issue 872 before trusting a clean figure from one." >&2
  else
    echo "gremlins: $PKG does not pass its own tests, or a package below it does not (the output above says which), so every mutant would read as killed; refusing to measure" >&2
  fi
  exit 1
fi

# The second run is timed by the clock, because `go test`'s own summary line
# says nothing reliable about it: it reports the test binary's run without the
# build gremlins' figure includes, a -cover run ends in a coverage figure
# rather than a duration, and a package with no tests under the tags given
# prints `[no test files]` and exits 0. Reading that line, and falling back to
# a guess of 0.010s when it carried no duration, is what turned 114 seconds of
# harness tests into a 95-hour deadline for every mutant.
TIMEFORMAT=%3R
status=0
base=$({ time baseline; } 2>&1) || status=$?
if [ "$status" != 0 ]; then
  cat "$out" >&2
  echo "gremlins: $PKG passed its tests and then failed them on the timed second run, so a mutant's verdict would depend on which way the suite fell; refusing to measure" >&2
  exit 1
fi
if ! awk -v b="$base" 'BEGIN{exit !(b ~ /^[0-9]+\.[0-9]+$/ && b + 0 > 0)}'; then
  echo "gremlins: the timed baseline reads '$base', which is not a positive number of seconds; refusing to derive a deadline from it" >&2
  exit 1
fi

# The coefficient is the budget's multiple of the base, never below 8 so a slow
# package still gets a real multiple of its own runtime and never above 6000,
# and then no larger than the ceiling's multiple. A ceiling that does not hold
# two runs would time out every mutant, so it is refused rather than applied.
# gremlins multiplies its OWN coverage run rather than this one, and Go's test
# cache answers that instantly for an unchanged package, so GOFLAGS carries
# -count=1 below to make what it multiplies a real measurement. The ceiling's
# multiple is clamped at 6000 too, although no comparison below can tell 6000
# from more, because awk prints an integer past 2^31 in exponent form
# (3.33333e+09 for a ceiling of 1e9 s over a base of 0.3 s), which bash's -lt
# cannot compare.
read -r coeff cap <<<"$(awk -v b="$base" -v f="$budget" -v m="$ceiling" 'BEGIN{
  c = int(f / b) + 1; if (c < 8) c = 8; if (c > 6000) c = 6000
  k = int(m / b); if (k > 6000) k = 6000
  print c, k
}')"
if [ "$cap" -lt 2 ]; then
  echo "gremlins: $PKG's coverage run takes ${base}s by the clock, and a ${ceiling}s ceiling does not hold two of them, so every mutant would be reported TIMED OUT; raise MUTANT_DEADLINE_MAX (the fourth argument) to measure it" >&2
  exit 1
fi
if [ "$coeff" -gt "$cap" ]; then
  if [ "$cap" -lt 8 ]; then
    echo "gremlins: the ${ceiling}s ceiling holds the coefficient at $cap, under the floor of 8 a slow package is otherwise given, so a mutant that is only slow under four workers may be reported TIMED OUT"
  else
    echo "gremlins: the ${ceiling}s ceiling holds the coefficient at $cap rather than $coeff"
  fi
  coeff=$cap
fi
deadline=$(awk -v b="$base" -v c="$coeff" 'BEGIN{printf "%.1f", b * c}')
echo "gremlins: $PKG coverage run takes ${base}s by the clock, so -timeout-coefficient $coeff: about ${deadline}s per mutant (budget ${budget}s, ceiling ${ceiling}s)"

# PKG names ONE package, which is what this target's usage line says and what
# the sweep's per-package figures claim. gremlins does not read it that way: it
# walks the directory it is given, so a package with anything under it is
# measured together with its whole subtree. Measured on ./cmd/audit_1to1: 789
# runnable mutants, of which 27 are the package's own and 762 belong to its
# seven sub-packages, each of which the sweep also measures on its own. It
# reaches fixtures too, so the planted trees under a command's testdata were
# being mutated as though they were source.
#
# --exclude-files takes a regexp over the path RELATIVE TO THE TARGET, which is
# the one fact this rests on and it was measured rather than assumed: on that
# same package `^internal/` and `/` both leave 24, while the module-relative
# `^cmd/audit_1to1/internal/` leaves all 789 and so matches nothing. A path
# with a separator in it is therefore exactly a file below the package, and `/`
# excludes all of them and nothing else. A leaf package has none, so passing it
# there changes no figure.
#
# A caller who states their own exclusion, as a flag in any spelling or through
# GREMLINS_UNLEASH_EXCLUDE_FILES, is left alone: they are asking for a
# different measurement, and two rules for one flag is how one of them silently
# wins. A flag passed here would override their variable without a word.
if [ -z "$excluded" ]; then
  gremlins_flags+=(--exclude-files=/)
fi

GOFLAGS="${GOFLAGS:-} -count=1" go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0 \
  unleash --invert-logical --workers 4 --timeout-coefficient "$coeff" ${gremlins_flags[@]+"${gremlins_flags[@]}"} "$target"
