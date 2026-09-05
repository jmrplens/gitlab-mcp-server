// Command audit_test_subtests finds case loops in test functions that assert
// without opening a subtest, and can rewrite the mechanical ones.
//
// The project's Go guidelines ask for table-driven tests whose cases run
// under t.Run: a failing case then names itself, a t.Fatal inside one case
// stops that case rather than the whole table, and `go test -run` can select
// a single case. The shape that violates the rule is a range over a table
// (a slice or map literal, inline or bound to a local variable) whose body
// calls t.Error, t.Errorf, t.Fatal, t.Fatalf, t.Fail or t.FailNow, or hands
// t to a helper, with no t.Run anywhere inside the loop body.
//
// A loop that walks a sequence of dependent steps rather than independent
// cases is not a table. Mark it with a comment on the line before the loop
// or at the end of its first line:
//
//	// sequential: each step depends on the state the previous one left
//	for _, step := range []step{...} {
//
// and the auditor records it as declared-sequential instead of reporting it.
// A loop inside a synctest.Test bubble is skipped too: the testing package
// panics on t.Run called inside a bubble, so the rule cannot apply there and
// the site is counted separately.
//
// -fix rewrites the sites whose subtest name is unambiguous: a []string
// table names each case after its element, a struct table after a string
// field called name, desc, description, label, title or id, and a
// map[string]... table after its key. The body is wrapped in
// t.Run(name, func(t *testing.T) { ... }) and a bare continue becomes
// return; bodies that break, goto, or use a table without such a name are
// left for a hand rewrite and stay in the report.
//
// Usage:
//
//	go run ./cmd/audit_test_subtests [-json out.json] [-check] [-fix] [dirs...]
//
// With no directories the module's test files under ./cmd, ./internal and
// ./test are scanned. -check exits non-zero when any site remains, so the
// tool gates CI.
package main
