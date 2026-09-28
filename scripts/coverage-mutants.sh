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
# The second thing it decides is how long each mutant may run. gremlins has no
# setting for that: it multiplies --timeout-coefficient by the wall time of its
# own coverage run, `go test [-tags T] [-coverpkg P] -cover -coverprofile F
# ./<pkg>/...` from the module root. So the coefficient is derived here from a
# run of that same command under the same tags, timed by the clock, and held
# under a ceiling. Deriving it from anything else is how a package behind a
# build tag came to give each mutant a deadline of 95 hours. See issue 915.
#
# The third is what that deadline has to pay for. gremlins runs each
# mutant's tests in its own copy of the module, one per worker, and Go keys a
# compile on the package's directory unless -trimpath is set, so the first
# mutant on every worker used to recompile every package of this module its
# test imports inside that mutant's own deadline. Every go command here and in
# gremlins therefore runs under -trimpath, and the command gremlins runs per
# mutant is run once beforehand, so a worker's copy finds everything it needs
# already compiled. See issue 1029.
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
# A coefficient the caller fixes in GREMLINS_FLAGS, which gremlins reads after
# the one this script derives and so uses instead of it. It has no
# environment binding to read here: a flag beats gremlins' own variable, and
# the script always passes one.
fixed=""
fixed_given=""
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
        timeout-coefficient) fixed=$value fixed_given=yes ;;
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
# The deadline gremlins will apply is announced below, so a fixed coefficient
# has to be read as gremlins reads it. pflag parses an int with base 0, so a
# leading zero makes it octal, and gremlins reads 0 as its own default of 3
# (DefaultTimeoutCoefficient, internal/engine/executor.go, v0.6.0). A plain
# whole number without a leading zero is the one spelling both agree on, and
# anything else is refused before the suite runs rather than announced wrong.
if [ -n "$fixed_given" ]; then
  if ! [[ $fixed =~ ^(0|[1-9][0-9]*)$ ]]; then
    echo "gremlins: GREMLINS_FLAGS: --timeout-coefficient '$fixed' is not a whole number written without a leading zero, so the deadline gremlins would apply cannot be told; refusing to measure" >&2
    exit 1
  fi
  [ "$fixed" != 0 ] || fixed=3
fi
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

# Every go command from here on, and gremlins with every command it runs,
# inherits these two, so what the script measures and warms is what gremlins
# will find.
#
# -trimpath because gremlins copies the module into a directory per worker and
# runs each mutant's tests there, and without it cmd/go puts the package's
# directory into its compile key (buildActionID in cmd/go/internal/work/exec.go,
# Go 1.27.1). No compile done here was then of any use to a worker, and the
# first mutant on each one recompiled every package of this module its test
# imports inside its own deadline. That is what the 300 s default budget was
# sized to absorb, and a fixed coefficient passed through GREMLINS_FLAGS does
# not absorb it: under `--workers 2 --timeout-coefficient 20` six packages of
# issue 971 reported twenty-one mutants TIMED OUT, every one among the first a
# worker compiled. Under -trimpath the directory is left out of the key.
#
# -count=1 because gremlins multiplies the coefficient by the wall time of its
# own coverage run, which Go's test cache would otherwise answer instantly for
# a package whose files have not changed since it last ran. A go command
# ignores a flag in GOFLAGS that it does not know, so the same setting reaches
# go list and go mod download harmlessly.
export GOFLAGS="${GOFLAGS:-} -trimpath -count=1"
# -trimpath also drops the root the toolchain was installed at from the
# binaries it builds, and that root is where runtime.GOROOT, and go/build's
# Default.GOROOT after it, fall back to when GOROOT is not set. A test binary is
# started with the environment the go command was given, so without this a
# test that runs the go command out of cmd/internal/golist would exec "bin/go"
# and fail: cmd/gen_testing_docs, cmd/godoc_tool and cmd/internal/golist did.
# Setting the root the go command itself uses restores exactly the value an
# untrimmed build records.
GOROOT=$(go env GOROOT)
export GOROOT
root=$(go list -m -f '{{.Dir}}')
# go list prints native paths, which carry backslashes on Windows. Every path
# below is cut against the root and written into a pattern or a target, and a
# strip that expects a slash strips nothing on a backslash, so the root and
# the package directory are written with slashes once, here, which go and Git
# Bash both read on every platform.
root=${root//\\//}

# covered_by_coverpkg reports whether the -coverpkg patterns name the package
# whose import path it is given: 0 when they do, 1 when they resolve and miss
# it, and 2 when go list could not resolve them at all (an empty entry in the
# list, or one go reads as a flag), which is not the same answer and is not
# reported as one. They are resolved the way gremlins' coverage run resolves
# them: a comma-separated list, from the module root, under the same tags. A
# pattern that matches nothing lists nothing, which is a no.
covered_by_coverpkg() {
  local patterns listed
  IFS=, read -r -a patterns <<<"$coverpkg"
  listed=$(cd "$root" && go list -e ${tag_args[@]+"${tag_args[@]}"} -f '{{.ImportPath}}' "${patterns[@]}") || return 2
  grep -qxF -- "$1" <<<"$listed"
}

# The package is loaded under those tags, and -e keeps one go cannot load
# (every file behind a tag nobody passed, a path that is not there) from ending
# the script with go's own message, which does not say what to do about it.
#
# A package with no test file under its tags is refused unless the run is an
# integration run with a -coverpkg that names the package, and the package is
# one another package's test can link. Each mutant runs only this package's
# tests unless --integration is given, so without it no mutant of such a
# package can be killed, and unless a -coverpkg names it nothing covers its
# blocks and every mutant is reported NOT COVERED: a -coverpkg naming only
# other packages leaves it exactly as uncovered as none. Before this check its
# baseline passed having run nothing, printed no duration, and handed gremlins
# a coefficient of 3001. Under both, the module's other tests cover its blocks
# and gremlins runs them against each mutant, so it is measured, and the run
# says so.
#
# That holds only for an importable package measured where it is. No other
# package's test can link a package main, and a package staged below as
# <dir>.mutants-<name> is a copy nothing imports, whose files gremlins matches
# against the profile by their module-relative path, the copy's and never the
# original's. So either is refused whatever -i and -coverpkg say, since every
# mutant of it would be reported NOT COVERED.
listing=$(go list -e ${tag_args[@]+"${tag_args[@]}"} -f '{{.Name}}
{{.Dir}}
{{.ImportPath}}
{{len .TestGoFiles}} {{len .XTestGoFiles}}
{{with .Error}}{{.}}{{end}}' "$PKG")
{
  IFS= read -r pkgname
  IFS= read -r pkgdir
  IFS= read -r pkgpath
  read -r tests xtests
  listerr=$(cat)
} <<<"$listing"
pkgdir=${pkgdir//\\//}
# Staged when gremlins cannot resolve the package where it is (see below).
stage=""
case "$(basename "$pkgdir")" in
  *"$pkgname") ;;
  *) stage=1 ;;
esac
refusal=""
# Whether the refusal may suggest measuring through the module's other tests,
# which is no suggestion for a package they cannot link.
elsewhere=1
if [ -n "$listerr" ]; then
  refusal="go cannot load $PKG under build tags ${tags:-(none)}: $listerr"
elif [ "$tests $xtests" = "0 0" ]; then
  covered=1
  if [ "$pkgname" = main ] || [ -n "$stage" ]; then
    elsewhere=""
    refusal="$PKG has no test files under build tags ${tags:-(none)}, and it is a package main or one measured through a staged copy, which no other package's test can link, so no -coverpkg covers it and every mutant would be reported NOT COVERED; refusing to measure"
  elif [ -z "$integration" ] || [ -z "$coverpkg" ]; then
    refusal="$PKG has no test files under build tags ${tags:-(none)}, so no mutant of it could be killed: each mutant runs only this package's tests without --integration, and without a -coverpkg every one is reported NOT COVERED; refusing to measure"
  elif covered_by_coverpkg "$pkgpath"; then
    covered=0
  else
    covered=$?
  fi
  if [ -z "$refusal" ] && [ "$covered" = 2 ]; then
    refusal="go list could not resolve -coverpkg $coverpkg (its own message is above); refusing to measure"
  elif [ -z "$refusal" ] && [ "$covered" = 1 ]; then
    refusal="$PKG has no test files under build tags ${tags:-(none)}, and -coverpkg $coverpkg does not name $pkgpath, so nothing would cover its blocks and every mutant would be reported NOT COVERED; refusing to measure"
  elif [ -z "$refusal" ]; then
    echo "gremlins: $PKG has no test files under build tags ${tags:-(none)}; measuring it through the module's other tests, which --integration runs against each mutant and -coverpkg $coverpkg lets cover it"
  fi
fi
if [ -n "$refusal" ]; then
  echo "gremlins: $refusal" >&2
  hint="gremlins: a package behind a build tag is measured with GREMLINS_FLAGS='--tags <tag>', which reaches this baseline and gremlins alike; a tag set only in a .gremlins.yaml reaches gremlins and not the baseline."
  if [ -n "$elsewhere" ]; then
    hint="$hint One tested only from elsewhere in the module is measured with GREMLINS_FLAGS='-i --coverpkg <pattern>', a pattern that names it"
  fi
  echo "$hint" >&2
  exit 1
fi

# The staging directory is removed whatever happens. Left behind it is a second
# copy of the package inside the module, which every tree-wide `go build ./...`
# and every gate that loads ./... would then read as real. The baseline's
# output and coverage profile go with it.
#
# An interrupt ends the run rather than returning to it. The cleanup used to be
# the handler for INT and TERM as well, and a handler returns to the script:
# a signal during the baseline removed the copy and then went on to time it and
# start gremlins on a directory that was gone, and one during gremlins let the
# run exit 0. Exiting from the handler runs the EXIT trap, which cleans up.
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
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

target=$PKG
# A package whose directory ends in its name is one gremlins resolves on its
# own, and is run where it is.
if [ -n "$stage" ]; then
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
fi

# patterns_for sets the two patterns a run over the directory it is given
# names, both from the module root. scan is the run gremlins times, its
# coverage step, over the directory's subtree or over the whole module under
# --integration. each is the command gremlins runs against each mutant (see
# the run of it below): the package itself, since the default exclusion keeps
# every mutant in it, or the whole subtree when the caller's own exclusion may
# leave packages below it to be mutated, or the whole module under
# --integration.
#
# The root and the package directory were written with slashes when they were
# read, so the cut after the root works on Windows as well; a strip expecting a
# slash on a native path used to strip nothing there and leave a pattern naming
# a directory that does not exist, which go test fails and this script would
# then report as a failing suite.
patterns_for() {
  if [ -n "$integration" ] || [ "$1" = "$root" ]; then
    scan=./...
  else
    scan="./${1#"$root"/}/..."
  fi
  if [ -n "$integration" ] || [ -n "$excluded" ]; then
    each=$scan
  elif [ "$1" = "$root" ]; then
    each=.
  else
    each="./${1#"$root"/}"
  fi
}
# The patterns naming the package where it is, which is what a reader
# reproducing a failure runs: a staged copy is gone by the time they read it.
patterns_for "$pkgdir"
shown_scan=$scan
shown_each=$each
dir=$pkgdir
[ -z "$staged" ] || dir=$staged
patterns_for "$dir"
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
# Said after either refusal of a suite that fails here, since -trimpath is a
# cause the output above cannot name: a test that finds its files through its
# own compiled source path (runtime.Caller) is handed a module-relative path
# under it rather than a directory, and fails here while passing a plain run.
# The command it offers is the one the script ran, with the GOFLAGS, GOROOT and
# tags it ran under, over the package where it is rather than over a staged
# copy the EXIT trap removes before anyone reads this. Without GOROOT a test
# that runs the go command fails for another reason (see above).
trimpath_hint() {
  local flags=${GOFLAGS# }
  echo "gremlins: every go command here runs under -trimpath (issue 1029), which hands a test that locates files through runtime.Caller a module-relative path; GOROOT=$GOROOT GOFLAGS='$flags' go test${tags:+ -tags $tags} $1, run from the module root, shows whether that is the cause" >&2
}
if ! baseline; then
  cat "$out" >&2
  if [ -n "$staged" ]; then
    echo "gremlins: the staged copy of $PKG does not pass its own tests there, so the verdicts would be about the staging rather than the package; refusing to measure" >&2
    echo "gremlins: a test that reads its own directory or import path is the usual cause. Run it unstaged to see, and read issue 872 before trusting a clean figure from one." >&2
  else
    echo "gremlins: $PKG does not pass its own tests, or a package below it does not (the output above says which), so every mutant would read as killed; refusing to measure" >&2
  fi
  trimpath_hint "$shown_scan"
  exit 1
fi

# The command gremlins runs against each mutant, run once against the
# unmutated package: `go test [-tags T] -failfast <package>`, or `./...` from
# the root under --integration (getTestArgs in internal/engine/executor.go,
# v0.6.0; gremlins adds a -timeout, which decides nothing about a compile).
# gremlins names the package by its import path and runs it from the worker's
# copy of the target directory; the relative path from the root names the same
# package, and under -trimpath neither the working directory nor the copy's own
# location is part of any compile key, so this run leaves in the build cache
# every package the first mutant on each worker would otherwise compile. A
# caller who states their own --exclude-files may leave packages below the
# target to be mutated, each run as its own `go test <package>`, so the run
# covers the target's whole subtree then (patterns_for, above).
#
# The coverage run above does not: it compiles the packages its pattern or its
# -coverpkg names with coverage counters, which a mutant's plain `go test` does
# not share, so a package below the target that the target imports would still
# be compiled inside the first mutant's deadline. Measured on six packages of issue 971
# under `--workers 2 --timeout-coefficient 20`, -trimpath and this run together
# turned all twenty-one first-compile timeouts into kills with no other verdict
# moving.
#
# It is also a gate. gremlins reads a failing run of this command as a killed
# mutant, and it is not the command the coverage run is: a test that behaves
# differently when coverage is on passes one and fails the other.
if ! (cd "$root" && go test -count=1 ${tag_args[@]+"${tag_args[@]}"} -failfast "$each") >"$out" 2>&1; then
  cat "$out" >&2
  echo "gremlins: $PKG passes its tests under -cover and fails go test -failfast $each, the command gremlins runs against each mutant, so every mutant would read as killed; refusing to measure" >&2
  trimpath_hint "$shown_each"
  exit 1
fi

# The last run is timed by the clock, because `go test`'s own summary line
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
  echo "gremlins: $PKG passed its tests and then failed them on the timed run, so a mutant's verdict would depend on which way the suite fell; refusing to measure" >&2
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
# cache answers that instantly for an unchanged package, so the GOFLAGS it
# inherits carries -count=1 to make what it multiplies a real measurement. The
# ceiling's multiple is clamped at 6000 too, although no comparison below can
# tell 6000 from more, because older mawk builds (1.3.4 20200120, the awk
# Debian 12 installs) print an integer past 2^31 in exponent form (3.33333e+09
# for a ceiling of 1e9 s over a base of 0.3 s), which bash's -lt cannot
# compare. gawk, BWK awk, busybox awk and current mawk print the integer.
#
# A coefficient the caller fixed in GREMLINS_FLAGS is the one gremlins applies,
# so it is the one announced, and neither the budget nor the ceiling bounds it.
# Announcing the derived one beside it printed a deadline that was not in force,
# which is exactly the mode issue 1029's timeouts were read in.
coeff_args=()
if [ -n "$fixed_given" ]; then
  deadline=$(awk -v b="$base" -v c="$fixed" 'BEGIN{printf "%.1f", b * c}')
  echo "gremlins: $PKG coverage run takes ${base}s by the clock, and GREMLINS_FLAGS fixes -timeout-coefficient $fixed: about ${deadline}s per mutant, which neither the ${budget}s budget nor the ${ceiling}s ceiling bounds"
else
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
  coeff_args=(--timeout-coefficient "$coeff")
fi

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

go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0 \
  unleash --invert-logical --workers 4 ${coeff_args[@]+"${coeff_args[@]}"} ${gremlins_flags[@]+"${gremlins_flags[@]}"} "$target"
