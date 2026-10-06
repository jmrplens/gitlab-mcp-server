// Command measure_conditions plans and reports the condition coverage
// measurement that .github/workflows/conditions.yml runs on demand (issue
// 1210).
//
// The condition gate is gobco, run through scripts/coverage-conditions.sh,
// which is the recipe behind `make coverage-conditions`. It runs on Linux
// before a commit, and on the Windows and macOS legs of CI only over the files
// those systems alone build. A figure for a whole package on Windows or macOS
// had no way to be taken except by hand on a machine of that system, so the
// figures quoted for internal/toolutil in pull request 1206 came from a
// Windows virtual machine and nobody could repeat them on macOS at all. The
// workflow runs the same script on each runner a dispatch names, and this
// command is the two ends of it that are not that script:
//
//	measure_conditions plan -packages <list> -systems <list> -gate <gate>
//	measure_conditions summarize -plan <json> -records <dir> -gate <gate> -commit <sha>
//
// plan validates what the dispatch asked for and prints the matrix the
// workflow fans out over, as one `matrix=<json>` line for $GITHUB_OUTPUT. An
// empty package list means every package carrying a GOOS- or GOARCH-constrained
// file, found the way cmd/audit_dead_consts finds them: the files the go
// command builds for each package are listed under every platform the release
// builds (.goreleaser.yml's goos crossed with its goarch), and a package whose
// files differ between two of them is one. A file behind a tag alone, such as
// the race detector's or the e2e suite's, builds on no release target without
// that tag and so differs on none of them, which keeps it out: the script is
// run without tags here, and could not measure such a package anyway.
//
// summarize reads the record each run left beside gobco's raw output, joins it
// to the plan, and writes the job summary: one row per package and runner with
// the figure gobco printed, and one row per condition gobco names as not
// evaluated both ways, with its file and line. Every figure is the one the
// script printed on that runner. Nothing is inferred from another platform,
// and nothing is computed that the script did not count.
//
// It is a measurement, not a gate, and no figure fails it. The script's exit
// status is read for one thing only, whether a measurement was taken: 0 when
// it was and the gate held nothing, 3 when it was and the gate (GOBCO_GATE)
// held at least one condition, and anything else when the script could not
// measure at all. A run in the third case has already failed its own job with
// the reason, and is reported here as not measured with the lines the script
// said it in. summarize itself exits 1 only when a record cannot be read or
// disagrees with itself, since then the table would be a guess, and 2 on a
// usage error.
package main
