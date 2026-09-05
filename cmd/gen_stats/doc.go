// Command gen_stats auto-generates the repository statistics section in
// README.md. It classifies every tracked Go file, counts
// dependencies, and replaces the content between the
// <!-- START STATS --> / <!-- END STATS --> markers.
//
// Every figure is a pure function of the tracked file set. Nothing is derived
// from git history: a commit count changes with the very commit that would
// refresh it, so the section could never be both committed and current, and a
// CI shallow clone would compute a different number anyway. Files come from the
// git index rather than a directory walk for the same reason — see
// [listTrackedGoFiles]. That is what makes `--check` usable as a gate.
//
// Usage:
//
//	go run ./cmd/gen_stats/
//
// With --check it verifies the section is current without writing.
package main
