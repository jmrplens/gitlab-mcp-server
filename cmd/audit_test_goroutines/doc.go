// Command audit_test_goroutines detects testing.T abort calls made from
// goroutines other than the one running the test.
//
// The testing package documents that FailNow — and therefore t.Fatal and
// t.Fatalf — must be called from the test goroutine. Inside an HTTP mock
// handler, a go statement, an errgroup task, or an MCP tool handler, the call
// instead terminates only that goroutine: the response is truncated or never
// written, the client observes a transport error, and the test continues
// against the wreckage. go vet's testinggoroutine analyzer flags bare
// `go func() { t.Fatal() }()` but cannot see that a function literal passed
// to http.HandlerFunc or mcp.AddTool crosses a goroutine boundary, which is
// how these sites accumulate unnoticed.
//
// The tool reports every t.Fatal/t.Fatalf/t.FailNow call inside such a
// literal, classifies each site as category A (the abort is in tail position:
// nothing else would have run) or category B (the handler still had work to
// do, so the response is observably truncated), and separately reports
// t.Error/t.Errorf calls whose enclosing block does not return afterwards —
// the conversion contract requires an explicit return so a later edit cannot
// silently reintroduce dead code paths (the defect class behind PR #270's
// nil-dereference panic).
//
// Usage:
//
//	go run ./cmd/audit_test_goroutines [-json out.json] [-check] [dirs...]
//
// With no directories the module's test files under ./cmd, ./internal, and
// ./test are scanned. -check exits non-zero when any finding exists, so the
// tool can gate CI once the sweep lands.
package main
