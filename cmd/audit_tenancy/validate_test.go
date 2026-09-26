package main

import (
	"errors"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// TestCheckValidate_TheRegistersOwnRefusalsAreFindings: each line of a
// validator's joined error is one finding, and a register both validators
// accept yields none.
func TestCheckValidate_TheRegistersOwnRefusalsAreFindings(t *testing.T) {
	report := fixture{files: map[string]string{"site/site.go": siteHeader}, validate: errFixture}.run(t)
	assertFindings(t, report, "G11",
		"tenancy.Validate: ROW-001: rule (INV-000): first",
		"tenancy.Validate: ROW-002: rule (INV-000): second",
	)
	clean := fixture{files: map[string]string{"site/site.go": siteHeader}}.run(t)
	assertFindings(t, clean, "G11")
}

// TestCheckValidate_TheFailureTableIsValidatedToo: a failure table the
// register's validator refuses is a finding under its own name.
func TestCheckValidate_TheFailureTableIsValidatedToo(t *testing.T) {
	cfg := fixture{files: map[string]string{"site/site.go": siteHeader}}.config(t)
	cfg.register.validateFailures = func([]tenancy.Failure) error { return errors.New("resolve/k: charged-attributable: refused") }
	report, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	assertFindings(t, report, "G11", "tenancy.ValidateFailures: resolve/k: charged-attributable: refused")
}

// TestViolations_SkipsEmptyLines: a joined error's blank lines are no
// findings.
func TestViolations_SkipsEmptyLines(t *testing.T) {
	if got := violations("v", errors.New("a\n\nb\n")); len(got) != 2 {
		t.Fatalf("violations = %v, want two", got)
	}
	if violations("v", nil) != nil {
		t.Fatal("a nil error produced findings")
	}
}
