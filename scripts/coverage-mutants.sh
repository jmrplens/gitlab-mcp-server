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
set -euo pipefail

PKG=${1:?usage: coverage-mutants.sh <package> [budget] [floor]}
BUDGET=${2:-30}
FLOOR=${3:-10}

# The budget is a knob and the floor is what it may not go under: below a few
# seconds the per-mutant budget falls under the fixed cost of starting
# `go test` and every mutant is reported TIMED OUT having never run. The floor
# is applied out loud, or the printed budget is a second lie on top of that.
budget=$(awk -v want="$BUDGET" -v floor="$FLOOR" 'BEGIN{print (want+0 < floor+0) ? floor : want}')
if [ "$budget" != "$BUDGET" ]; then
  echo "gremlins: MUTANT_BUDGET=${BUDGET}s is under the ${FLOOR}s floor and would report untested mutants as timeouts; using ${budget}s"
fi

pkgname=$(go list -f '{{.Name}}' "$PKG")
pkgdir=$(go list -f '{{.Dir}}' "$PKG")
root=$(go list -m -f '{{.Dir}}')

# The staging directory is removed whatever happens. Left behind it is a second
# copy of the package inside the module, which every tree-wide `go build ./...`
# and every gate that loads ./... would then read as real.
staged=""
cleanup() {
  if [ -n "$staged" ] && [ -d "$staged" ]; then
    rm -rf "$staged"
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

baseline=$(go test -count=1 "$target" 2>&1) || {
  printf '%s\n' "$baseline" >&2
  if [ -n "$staged" ]; then
    echo "gremlins: the staged copy of $PKG does not pass its own tests there, so the verdicts would be about the staging rather than the package; refusing to measure" >&2
    echo "gremlins: a test that reads its own directory or import path is the usual cause. Run it unstaged to see, and read issue 872 before trusting a clean figure from one." >&2
  else
    echo "gremlins: $PKG does not pass its own tests, so every mutant would read as killed; refusing to measure" >&2
  fi
  exit 1
}

# The whole duration, minutes included. `go test` prints a summary over a
# minute as 1m2.345s, and a pattern that reads the seconds off the end takes
# that for 2.345: the coefficient is then derived from a package twenty-six
# times faster than the one being measured, and every mutant gets a timeout
# that much larger than intended. A line with no duration on it, which is what
# a cached result prints, leaves base empty and falls back below.
base=$(printf '%s\n' "$baseline" | tail -1 | awk '{ d = $NF }
  END {
    if (d !~ /^([0-9]+m)?[0-9]+(\.[0-9]+)?s$/) exit 0
    sub(/s$/, "", d); minutes = 0
    if (match(d, /^[0-9]+m/)) { minutes = substr(d, 1, RLENGTH - 1) + 0; d = substr(d, RLENGTH + 1) }
    printf "%.3f", minutes * 60 + d + 0
  }')
[ -n "$base" ] || base=0.010

# The coefficient is applied to gremlins' OWN coverage run rather than to the
# baseline above, and Go's test cache answers that instantly for an unchanged
# package, so GOFLAGS carries -count=1 to make what it multiplies a real
# measurement. `go build` ignores a flag it does not know, so the same setting
# is harmless for the compile around each mutant.
coeff=$(awk -v b="$base" -v f="$budget" 'BEGIN{c=int(f/b)+1; if(c<8)c=8; if(c>6000)c=6000; print c}')
echo "gremlins: $PKG tests take ${base}s, so -timeout-coefficient $coeff for a ~${budget}s budget"

# GREMLINS_FLAGS is split into words on purpose and then quoted, because its
# documented use carries a regular expression, and a caller passing `.*` to
# --exclude-files would otherwise have it expanded against the working
# directory before gremlins ever saw it.
read -r -a gremlins_flags <<<"${GREMLINS_FLAGS:-}"

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
# A caller who states their own --exclude-files is left alone: they are asking
# for a different measurement, and two rules for one flag is how one of them
# silently wins.
if [[ " ${gremlins_flags[*]-} " != *" --exclude-files"* && " ${gremlins_flags[*]-} " != *" -E"* ]]; then
  gremlins_flags+=(--exclude-files=/)
fi

GOFLAGS="${GOFLAGS:-} -count=1" go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0 \
  unleash --invert-logical --workers 4 --timeout-coefficient "$coeff" "${gremlins_flags[@]}" "$target"
