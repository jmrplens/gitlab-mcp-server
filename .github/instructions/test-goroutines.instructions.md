---
description: "Testing.T usage rules for code that runs off the test goroutine: HTTP mock handlers, go statements, errgroup tasks, and MCP tool handlers"
applyTo: "**/*_test.go"
---

# Assertions off the test goroutine

`testing.T.FailNow` — and therefore `t.Fatal`/`t.Fatalf` — must be called from
the goroutine running the test. Anywhere else (an `http.HandlerFunc` literal,
a `go` statement, an `errgroup.Group.Go` task, an MCP tool handler registered
on an in-memory server) it only terminates *that* goroutine: the HTTP response
is truncated or never written, the client observes a transport error, and the
test continues against the wreckage. `go vet`'s `testinggoroutine` analyzer
does not catch handler literals; `cmd/audit_test_goroutines` does
(`make audit-test-goroutines` / `make check-test-goroutines`).

## The six rules

Every assertion inside a function literal that crosses a goroutine boundary
MUST follow all of these:

1. **Never `t.Fatal`/`t.Fatalf`/`t.FailNow` inside the literal.** Use
   `t.Errorf` (or record and assert later — see rule 5).
2. **Always `return` immediately after the `t.Errorf`, even when it is the
   last statement.** `t.Fatal` provided that exit for free; a later edit
   appending code below a bare `t.Errorf` silently reintroduces the bug (this
   exact mistake produced a nil-dereference panic in PR #270).
3. **Write a deterministic response before returning.** For HTTP handlers,
   `http.Error(w, "<what failed>", http.StatusInternalServerError)`; for MCP
   tool handlers, an error result. Without it the client silently receives a
   `200` with an empty body — a worse signal than the EOF it replaces.
4. **Nothing the failed check would have validated may be used afterwards.**
   If the check guarded a nil (`r.MultipartForm`, a decoded struct), the
   `return` must come before any use of it.
5. **Prefer recording over asserting.** Store observed values in locals (or a
   struct) inside the handler and assert on the test goroutine after the
   client call returns. This removes the problem instead of mitigating it.
6. **"Must not be called" guards become a recorded flag** — `var called
   atomic.Bool` (or `atomic.Int64` for counts) written in the handler and
   asserted afterwards. The `atomic` is mandatory: an unsynchronised variable
   written from the handler goroutine is a data race.

## Canonical shapes

```go
// Guard that must never fire (rule 6):
var hits atomic.Int64
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    hits.Add(1)
    http.Error(w, "unexpected call", http.StatusInternalServerError)
}))
// ... drive the client ...
if n := hits.Load(); n != 0 {
    t.Fatalf("API was reached %d time(s)", n)
}

// Validation inside a handler that still owes a response (rules 2–4):
if r.Header.Get("PRIVATE-TOKEN") == "" {
    t.Errorf("missing PRIVATE-TOKEN header")
    http.Error(w, "missing token", http.StatusInternalServerError)
    return
}

// Recording for later assertion (rule 5):
var gotBody []byte
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    gotBody, _ = io.ReadAll(r.Body)
    testutil.RespondJSON(w, http.StatusOK, `{}`)
}))
// ... drive the client, then assert on gotBody from the test goroutine.
```
