package main

import (
	"slices"
	"strings"
	"testing"
)

// TestFoldCall_DomainPrefixHelper_IsFolded is the case this folding exists
// for. internal/tools/runnercontrollertokens writes every cross-link as
// canonicalID(actionNameTokenGet), where canonicalID puts a domain constant in
// front of a name; a scan over literals sees a bare action name with no domain
// and reports five phantoms that are not there.
func TestFoldCall_DomainPrefixHelper_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const (
	domain    = "demo"
	nameGet   = "get"
	nameList  = "list"
)

func canonicalID(name string) string { return domain + "." + name }

func qualified(name string) string { return canonicalID(name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID(nameGet), qualified(nameList)}}
}
`)

	want := []string{"demo.get", "demo.list"}
	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, want) {
		t.Errorf("related values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none: one helper calls the other", got)
	}
}

// TestFoldCall_BranchingHelper_IsNotFolded holds the one limit on folding. A
// helper that returns different strings on different paths has no single
// value, and picking one of them would be a guess reported as a fact.
func TestFoldCall_BranchingHelper_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func branching(project bool) string {
	if project {
		return "demo.project_get"
	}
	return "demo.get"
}

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{branching(true)}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a branching helper", got)
	}
	got := unresolvedExprs(sites)
	if len(got) != 1 || !strings.Contains(got[0], "branching") {
		t.Errorf("unresolved = %v, want the branching call named", got)
	}
}

// TestFoldCall_NonConstantArgument_IsNotFolded holds that a helper called with
// a value only known at run time is reported rather than folded to the prefix
// it would have carried.
func TestFoldCall_NonConstantArgument_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const domain = "demo"

func canonicalID(name string) string { return domain + "." + name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID(os.Getenv("DEMO"))}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_NonConcatenatingExpression_IsNotFolded holds that only the
// shapes a concatenation can take are folded here. Anything else is left to
// the type checker, which already answered no.
func TestFoldCall_NonConcatenatingExpression_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const domain = "demo"

func canonicalID(name string) string { return strings.Join([]string{domain, name}, ".") }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID("get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a join", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_ParenthesizedConcatenation_IsFolded holds the one shape the
// parser leaves in the way of an otherwise constant expression.
func TestFoldCall_ParenthesizedConcatenation_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func canonicalID(name string) string { return (domain + ".") + (name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID("get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("related values = %v, want demo.get", got)
	}
}
