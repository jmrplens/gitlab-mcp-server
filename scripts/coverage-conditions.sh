#!/usr/bin/env bash
# Run gobco over one package, over exactly the files the go command builds.
#
# gobco reads a package by handing every file whose name ends in .go to
# go/parser.ParseDir and type-checking all of them together (instrument and
# resolveTypes in instrumenter.go, v1.3.4, the latest release; its default
# branch at 7a09995 does the same). Neither step consults a build constraint,
# a //go:build line and a _windows.go name alike, so a package that declares a
# function twice, once per platform, stops with a redeclaration panic before
# anything is measured:
#
#   panic: probe_linux.go:5:6: peerStdinIsNull redeclared in this block
#
# Three packages here are in that shape: cmd/server (peerStdinIsNull,
# bindUnixSocket and isConnRefused, each declared once for Linux or Unix and
# once for the rest), internal/toolutil (syncDirectory, openLeafNoFollow and
# createNewLeafNoFollow, each in a unix and an other file) and
# test/e2e/internal/harness (serverBuildArgs, in server_race.go and
# server_norace.go). None had had a condition measured (issue 1017).
#
# So a package whose directory holds a .go file the go command does not build
# is staged: the module is copied into a temporary directory, every such file
# is removed from the package's copy, and gobco is run on the package there.
# gobco then parses one build context, the one its own `go test` compiles, and
# every figure it prints is about the files that build on this platform under
# the tags given. The files left out are named, because this run does not
# measure their conditions: only a run on a platform that builds them could,
# and nothing runs gobco on Windows or macOS today, so theirs stay open under
# issue 1017.
#
# The copy is of the module rather than of the package, because anything else
# changes what the tests see. gobco bridges an external test package to the
# instrumented one through the import path those tests already name, so a copy
# of the package under another path hands them the original, uninstrumented
# package and the build fails (internal/toolutil: undefined:
# toolutil.GobcoCover). Tests also read files by paths relative to their own
# directory (docs/, the root's server.json, their testdata) and some check the
# directory's own name (the harness's TestPackageName_...). In a copy of the
# module every import path, relative path and directory name is the
# original's. It leaves out .git, .claude/worktrees (complete worktrees of this
# repository), node_modules and dist, which no package's test reads and which
# gobco would otherwise copy again into its own work directory, and it lives
# outside the tree, so a run killed before its trap leaves a copy in the
# temporary directory rather than a second copy of every package in the tree.
#
# The files to remove are the complement of what go list says it builds
# (GoFiles, CgoFiles, TestGoFiles, XTestGoFiles), not its IgnoredGoFiles:
# ParseDir also reads a file whose name starts with a dot or an underscore,
# which the go command never builds and IgnoredGoFiles never lists. go list
# is asked under the same tags the gobco run passes to its `go test`, since
# without them every file of a package behind a tag reads as left out.
#
# gobco decides which parsed files to instrument with a go/build.Context
# holding only GOOS and GOARCH (shouldBuild). That context reads a constraint
# naming nothing but operating systems, architectures and `unix` exactly as the
# go command does, and rejects one naming anything else it is not given: a tag,
# a release (go1.22), cgo, the compiler. A file the go command builds under
# such a constraint would be compiled and its conditions left out of the
# report, which then reads as complete. So the copy blanks the constraint lines
# of those files, and only of those, line for line, so every position gobco
# reports is the original's and the file is instrumented like the rest. A
# package holding such a file is staged for that reason alone.
#
# A package with neither kind of file, a constraint on the platform alone
# included (a _unix_test.go behind //go:build !windows with no Windows
# counterpart), is run where it is, exactly as the recipe did before, so the
# figures of every package it could already measure do not move.
#
# Blanking has a cost the run cannot see: a test that reads its own files'
# headers finds them blank. The e2e harness checks that every one of its
# sources carries exactly //go:build e2e, so under TAGS=e2e its run fails
# there, and a failed staged run names the blanked files as the likely cause.
# That package waits for gobco to honour build tags (issue 1017).
#
# A report that measured no condition at all, `Condition coverage: 0/0`, is
# refused rather than passed on. It is what gobco prints for a package whose
# test binary never wrote its counts (no test ran) and for one whose every
# file it declined to instrument, and both read like a package with nothing
# left to test.
set -euo pipefail

# The files a run leaves out are listed in the order a glob returns them,
# which follows the locale's collation; C is the one every machine sorts the
# same way.
export LC_ALL=C

readonly GOBCO="github.com/rillig/gobco@v1.3.4"

PKG=${1:?usage: coverage-conditions.sh <package> [tags]}
TAGS=${2:-}
tag_args=()
gobco_args=()
if [ -n "$TAGS" ]; then
  tag_args=(-tags "$TAGS")
  # gobco hands every -test option to its `go test` after the package, where
  # the go command still reads a build flag.
  gobco_args=("-test=-tags=$TAGS")
fi

# The package under the build context every go command here shares with the
# `go test` gobco runs. -e keeps a package go cannot load (a path that is not
# there, a directory every file of which a constraint leaves out) from ending
# the script with go's own message, which does not say what the run needed.
listing=$(go list -e ${tag_args[@]+"${tag_args[@]}"} -f '{{.Dir}}
{{with .Module}}{{.Dir}}{{end}}
{{len .TestGoFiles}} {{len .XTestGoFiles}}
{{join .GoFiles " "}} {{join .CgoFiles " "}} {{join .TestGoFiles " "}} {{join .XTestGoFiles " "}}
{{with .Error}}{{.}}{{end}}' "$PKG")
{
  IFS= read -r pkgdir
  IFS= read -r moddir
  read -r tests xtests
  read -r -a built
  listerr=$(cat)
} <<<"$listing"
if [ -n "$listerr" ]; then
  echo "gobco: go cannot load $PKG under build tags ${TAGS:-(none)}: $listerr; refusing to measure" >&2
  echo "gobco: a package behind a build tag is measured with TAGS=<tag>, which reaches go list and gobco's go test alike" >&2
  exit 1
fi
# go list prints native paths, which carry backslashes on Windows, and the
# package directory is cut against the module root below, so both are written
# with slashes once, here, which go and Git Bash both read.
pkgdir=${pkgdir//\\//}
moddir=${moddir//\\//}
{
  read -r goos
  read -r goarch
} < <(go env GOOS GOARCH)
context="$goos/$goarch under build tags ${TAGS:-(none)}"

# The names a constraint may use and still be read by gobco's context exactly
# as the go command reads it: every operating system and architecture the
# toolchain knows, and `unix`, which go/build derives from GOOS the same way.
platform_names=" unix $(go tool dist list | tr '/' '\n' | sort -u | tr '\n' ' ')"

# beyond_platform reports whether a file's header, the lines above its package
# clause, carries a build constraint in either spelling that names anything
# outside those, which is what gobco's own context would misjudge.
beyond_platform() {
  local name
  while IFS= read -r name; do
    case "$platform_names" in
      *" $name "*) ;;
      *) return 0 ;;
    esac
  done < <(awk '/^package[ \t]/ { exit }
    /^\/\/go:build[ \t]/ || /^\/\/ [+]build[ \t]/ {
      sub(/^\/\/(go:build| [+]build)[ \t]+/, "")
      n = split($0, names, /[^A-Za-z0-9_.]+/)
      for (i = 1; i <= n; i++) if (names[i] != "") print names[i]
    }' "$1")
  return 1
}

# built_here reports whether the go command builds the named file of the
# package under this build context.
built_here() {
  local name
  for name in ${built[@]+"${built[@]}"}; do
    if [ "$name" = "$1" ]; then
      return 0
    fi
  done
  return 1
}

# Every file gobco would parse: each regular file of the directory whose name
# ends in .go, a leading dot or underscore included, since ParseDir skips none
# of them and the go command skips all of them.
left_out=()
left_out_tests=""
shopt -s nullglob dotglob
for path in "$pkgdir"/*.go; do
  name=${path##*/}
  if [ -f "$path" ] && ! built_here "$name"; then
    left_out+=("$name")
    case "$name" in
      *_test.go) left_out_tests=yes ;;
    esac
  fi
done
shopt -u nullglob dotglob
constrained=()
for name in ${built[@]+"${built[@]}"}; do
  if beyond_platform "$pkgdir/$name"; then
    constrained+=("$name")
  fi
done

if [ "$tests $xtests" = "0 0" ] && [ -n "$left_out_tests" ]; then
  echo "gobco: $PKG keeps no test file on $context: its tests are among the files the go command leaves out here (${left_out[*]}), so no test would run and gobco would measure nothing; refusing to measure" >&2
  echo "gobco: a package behind a build tag is measured with TAGS=<tag>, which reaches go list and gobco's go test alike" >&2
  exit 1
fi

# The staged module and the captured report are removed whatever happens. An
# interrupt ends the run rather than returning to it: a handler that only
# cleaned up would let the script carry on against a copy it had just removed.
workdir=""
report=""
cleanup() {
  if [ -n "$workdir" ] && [ -d "$workdir" ]; then
    rm -rf "$workdir"
  fi
  if [ -n "$report" ]; then
    rm -f "$report"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
report=$(mktemp)

target=$pkgdir
staged=""
if [ "${#left_out[@]}" -gt 0 ] || [ "${#constrained[@]}" -gt 0 ]; then
  staged=yes
  if [ "${#left_out[@]}" -gt 0 ]; then
    echo "gobco: $PKG holds ${#left_out[@]} .go file(s) the go command does not build on $context (${left_out[*]}), which gobco would parse with the rest (issue 1017); measuring it in a staged copy of the module without them. The conditions of the files left out are not measured by this run: only a run on a platform that builds them could measure them"
    if [ "${#constrained[@]}" -gt 0 ]; then
      echo "gobco: the copy blanks the build constraint lines of ${constrained[*]}, line for line, since each names more than the platform and gobco's own narrower build context would decline to instrument it"
    fi
  else
    echo "gobco: $PKG builds ${constrained[*]} under a build constraint naming more than the platform, which gobco's own narrower build context would decline to instrument; measuring it in a staged copy of the module with those lines blanked, line for line, so gobco instruments every file the go command builds here"
  fi
  workdir=$(mktemp -d)
  module="$workdir/$(basename "$moddir")"
  mkdir "$module"
  (cd "$moddir" && tar -cf - --exclude=./.git --exclude=./.claude/worktrees --exclude=node_modules --exclude=./dist .) |
    (cd "$module" && tar -xf -)
  target=$module
  if [ "$pkgdir" != "$moddir" ]; then
    target="$module/${pkgdir#"$moddir"/}"
  fi
  for name in ${left_out[@]+"${left_out[@]}"}; do
    rm -f "$target/$name"
  done
  for name in ${constrained[@]+"${constrained[@]}"}; do
    awk '!body && /^package[ \t]/ { body = 1 } !body && (/^\/\/go:build[ \t]/ || /^\/\/ [+]build[ \t]/) { print ""; next } { print }' \
      "$target/$name" >"$target/$name.blanked"
    mv "$target/$name.blanked" "$target/$name"
  done
fi

# gobco's report is passed through as it is written and kept, so the figure
# it ends on can be judged. gobco prints its figures even when its go test
# fails, so a failure is said out loud rather than left to an exit status a
# reader of the report may not see, in one line a caller can pick out
# (scripts/check-spec-conditions.sh reports it as the reason). A staged run
# then names what the staging itself changed, most likely first.
(cd "$target" && go run "$GOBCO" ${gobco_args[@]+"${gobco_args[@]}"}) | tee "$report" || {
  status=$?
  if [ -z "$staged" ]; then
    echo "gobco: gobco exited $status on $PKG, so any figure above is not a measurement: its go test failed, or it could not instrument the package" >&2
    exit "$status"
  fi
  echo "gobco: gobco exited $status on the staged copy of $PKG, so any figure above is not a measurement: its go test failed there, or it could not instrument the copy" >&2
  if [ "${#constrained[@]}" -gt 0 ]; then
    echo "gobco: the copy blanked the build constraint lines of ${constrained[*]}, so a test that reads its own files' headers finds them blank there and fails; that is the likeliest cause (the e2e harness's source checks fail this way), and a package whose tests do that cannot be measured this way until gobco honours build tags (issue 1017)" >&2
  fi
  echo "gobco: a test that reads something the copy leaves out (.git, .claude/worktrees, node_modules, dist) fails there too, and the package's own tests run where it is are what to compare against" >&2
  exit "$status"
}

if ! grep -qE '^Condition coverage: [0-9]+/[1-9][0-9]*$' "$report"; then
  echo "gobco: the report above measured no condition of $PKG (Condition coverage: 0/0, or no figure at all), which is what gobco prints when no test wrote its counts or when it declined to instrument every file; refusing to pass it on as a measurement" >&2
  exit 1
fi
