// Package main tests the goroutine-boundary assertion auditor against a
// fixture file covering every boundary kind and both site categories.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureSource exercises: category A and B t.Fatal sites in an HTTP handler,
// a compliant t.Errorf (return follows), a violating t.Errorf (no return), a
// go-statement abort, an errgroup-style .Go abort, a handler struct field,
// and — as negatives — aborts on the test goroutine plus a helper literal
// that never leaves it.
const fixtureSource = `package fixture

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFixture(t *testing.T) {
	_ = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("category B: handler still writes below") // fatal, B
		}
		w.WriteHeader(http.StatusOK)
	}))

	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("category A: tail position") // fatal, A
	})

	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			t.Errorf("compliant: returns after") // no finding
			http.Error(w, "bad", http.StatusInternalServerError)
			return
		}
		t.Errorf("violating: nothing returns after this") // errorf_no_return
	})

	go func() {
		t.Fatal("go statement abort") // fatal, A
	}()

	var g interface{ Go(func() error) }
	g.Go(func() error {
		t.FailNow() // fatal, B (return follows in source order)
		return nil
	})

	_ = struct{ ElicitationHandler func() }{
		ElicitationHandler: func() {
			t.Fatal("handler field abort") // fatal, A
		},
	}

	// Negatives: test-goroutine aborts must not be flagged.
	t.Fatal("on the test goroutine")
}

func helperOnTestGoroutine(t *testing.T) {
	check := func() {
		t.Fatal("plain literal, never crosses a goroutine boundary") // not flagged
	}
	check()
}
`

// TestScan_FixtureClassification verifies the scanner finds exactly the
// planted sites, classifies A/B correctly, honors the compliant
// errorf-then-return shape, and ignores test-goroutine aborts.
func TestScan_FixtureClassification(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture_test.go"), []byte(fixtureSource), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	report, err := scan([]string{dir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if got, want := report.Summary.FatalSites, 5; got != want {
		t.Errorf("fatal sites = %d, want %d: %+v", got, want, report.Fatal)
	}
	if got, want := report.Summary.CategoryA, 3; got != want {
		t.Errorf("category A = %d, want %d", got, want)
	}
	if got, want := report.Summary.CategoryB, 2; got != want {
		t.Errorf("category B = %d, want %d", got, want)
	}
	if got, want := report.Summary.ErrorfNoReturn, 1; got != want {
		t.Errorf("errorf_no_return = %d, want %d: %+v", got, want, report.ErrorfNoReturn)
	}

	boundaries := map[string]bool{}
	for _, f := range report.Fatal {
		boundaries[f.Boundary] = true
	}
	for _, want := range []string{"http.HandlerFunc", "go statement", ".Go(...)", "handler field ElicitationHandler"} {
		if !boundaries[want] {
			t.Errorf("missing boundary kind %q in %v", want, boundaries)
		}
	}
}

// TestScan_CleanFileHasNoFindings verifies a file whose handlers follow the
// contract produces an empty report.
func TestScan_CleanFileHasNoFindings(t *testing.T) {
	clean := `package fixture

import (
	"net/http"
	"testing"
)

func TestClean(t *testing.T) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("wrong method")
			http.Error(w, "wrong method", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	t.Fatal("test goroutine aborts stay legal")
}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clean_test.go"), []byte(clean), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	report, err := scan([]string{dir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.Summary.FatalSites != 0 || report.Summary.ErrorfNoReturn != 0 {
		t.Fatalf("clean file produced findings: %+v", report.Summary)
	}
}
