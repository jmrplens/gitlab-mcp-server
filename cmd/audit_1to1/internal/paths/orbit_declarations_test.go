package paths

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// TestClassifyOrbitFindings_ADeclarationAnswersOnlyItsOwnDirection verifies a
// declaration is matched by package, output, path and direction together: the
// same path answered for a published field does not answer a key an answer
// carries, and a declaration nothing matched is named, sorted, in the key a
// reader finds it by.
func TestClassifyOrbitFindings_ADeclarationAnswersOnlyItsOwnDirection(t *testing.T) {
	previous := declaredOrbitFields
	t.Cleanup(func() { declaredOrbitFields = previous })
	declaredOrbitFields = []orbitDeclaration{
		{Package: "p", Output: "O", Path: "a", Category: "published-cat", Reason: "published-why"},
		{Package: "p", Output: "O", Path: "b", Sent: true, Category: "sent-cat", Reason: "sent-why"},
		{Package: "p", Output: "O", Path: "z", Sent: true, Category: "unused"},
		{Package: "p", Output: "O", Path: "y", Category: "unused"},
	}
	unpublished := []OrbitField{{Package: "p", Output: "O", Path: "a"}, {Package: "p", Output: "O", Path: "b"}, {Package: "q", Output: "O", Path: "a"}}
	unsurfaced := []OrbitField{{Package: "p", Output: "O", Path: "b"}, {Package: "p", Output: "X", Path: "b"}}

	gotUnpublished, gotUnsurfaced, unused := classifyOrbitFindings(unpublished, unsurfaced)

	if gotUnpublished[0].Category != "published-cat" || gotUnpublished[0].Reason != "published-why" || gotUnpublished[1].declared() || gotUnpublished[2].declared() {
		t.Errorf("unpublished = %+v", gotUnpublished)
	}
	if gotUnsurfaced[0].Category != "sent-cat" || gotUnsurfaced[0].Reason != "sent-why" || gotUnsurfaced[1].declared() {
		t.Errorf("unsurfaced = %+v", gotUnsurfaced)
	}
	if want := []string{"p.O y (published)", "p.O z (sent)"}; !slices.Equal(unused, want) {
		t.Errorf("unused = %q, want %q", unused, want)
	}
	stale := OrbitCheck{Ran: true, UnusedDeclarations: unused}.staleDeclarations()
	if len(stale) != 2 || !strings.HasPrefix(stale[0], "p.O y (published) is declared against the Orbit response record and no finding matched it") {
		t.Errorf("staleDeclarations() = %q", stale)
	}
	if none, noneSent, noneUnused := classifyOrbitFindings(nil, nil); none != nil || noneSent != nil || len(noneUnused) != 4 {
		t.Errorf("classify of nothing = %v, %v, %v", none, noneSent, noneUnused)
	}
}

// TestDeclaredOrbitFields_EachCarriesItsEvidence verifies the real table's
// entries are about the Orbit package and each says what it is and why, which
// is the bar every declaration table here holds.
func TestDeclaredOrbitFields_EachCarriesItsEvidence(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range declaredOrbitFields {
		t.Run(declaration.key(), func(t *testing.T) {
			if declaration.Package != orbitPkg || declaration.Output == "" || declaration.Path == "" {
				t.Errorf("declaration names %q %q %q", declaration.Package, declaration.Output, declaration.Path)
			}
			if declaration.Category == "" || len(declaration.Reason) < 80 {
				t.Errorf("declaration carries category %q and a %d-character reason", declaration.Category, len(declaration.Reason))
			}
			if seen[declaration.key()] {
				t.Errorf("declared twice")
			}
			seen[declaration.key()] = true
		})
	}
}

// TestRun_TheOrbitRecord_IsJudgedAndAStaleDeclarationFailsTheGate verifies the
// wiring into the report: the Orbit section and its counters are published,
// the findings themselves gate nothing, and a declaration that answers nothing
// is a stale declaration, which does.
func TestRun_TheOrbitRecord_IsJudgedAndAStaleDeclarationFailsTheGate(t *testing.T) {
	root := orbitTree(t)
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
	stubOrbitRecord(t, orbitrecord.Document{Calls: []orbitrecord.Call{
		orbitCall("orbit.status", "raw", "StatusOutput", keys(orbitrecord.Root, "user", "user.available", "system", "system.status", "added")),
	}}, nil)

	content, clean, err := Run(t.Context(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := decode(t, content)
	summary := report.Summary
	if !clean || summary.OrbitCalls != 1 || summary.OrbitCompared != 1 || summary.OrbitNested != 4 ||
		summary.OrbitUnpublished != 5 || summary.OrbitUnsurfaced != 1 || summary.OrbitUndeclared != 6 || summary.TypedJudgedByOrbit != 0 {
		t.Errorf("clean = %t, summary = %+v", clean, summary)
	}
	if !report.Shapes.Orbit.Ran || len(report.Shapes.Orbit.Unsurfaced) != 1 {
		t.Errorf("orbit = %+v", report.Shapes.Orbit)
	}

	declaredOrbitFields = []orbitDeclaration{{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "gone", Category: "c", Reason: "r"}}
	content, clean, err = Run(t.Context(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	report = decode(t, content)
	if clean || report.Summary.StaleDeclarations != 1 || !strings.HasPrefix(report.StaleDeclarations[0], "internal/tools/orbitx.StatusOutput gone (published) is declared") {
		t.Errorf("clean = %t, stale = %q", clean, report.StaleDeclarations)
	}
}
