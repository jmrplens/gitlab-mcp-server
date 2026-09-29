package paths

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// TestOrbitCheck_Classified_ADeclarationAnswersOnlyItsOwnDirection verifies a
// declaration is matched by package, output, path and direction together: the
// same path answered for a published field does not answer a key an answer
// carries, nor a kind the published field does not carry, nor one the
// client-go struct does not decode, and a declaration nothing matched is
// named, sorted, in the key a reader finds it by.
func TestOrbitCheck_Classified_ADeclarationAnswersOnlyItsOwnDirection(t *testing.T) {
	previous := declaredOrbitFields
	t.Cleanup(func() { declaredOrbitFields = previous })
	declaredOrbitFields = []orbitDeclaration{
		{Package: "p", Output: "O", Path: "a", Direction: orbitPublished, Category: "published-cat", Reason: "published-why"},
		{Package: "p", Output: "O", Path: "b", Direction: orbitSent, Category: "sent-cat", Reason: "sent-why"},
		{Package: "p", Output: "O", Path: "c[]", Direction: orbitKind, Category: "kind-cat", Reason: "kind-why"},
		{Package: "p", Output: "O", Path: "d", Direction: orbitDecoderKind, Category: "decoder-cat", Reason: "decoder-why"},
		{Package: "p", Output: "O", Path: "z", Direction: orbitSent, Category: "unused"},
		{Package: "p", Output: "O", Path: "y", Direction: orbitPublished, Category: "unused"},
		{Package: "p", Output: "O", Path: "x", Direction: orbitKind, Category: "unused"},
		{Package: "p", Output: "O", Path: "w", Direction: orbitDecoderKind, Category: "unused"},
	}
	check := OrbitCheck{
		Ran:               true,
		Unpublished:       []OrbitField{{Package: "p", Output: "O", Path: "a"}, {Package: "p", Output: "O", Path: "b"}, {Package: "q", Output: "O", Path: "a"}},
		Unsurfaced:        []OrbitField{{Package: "p", Output: "O", Path: "b"}, {Package: "p", Output: "X", Path: "b"}, {Package: "p", Output: "O", Path: "c[]"}},
		Mismatched:        []OrbitField{{Package: "p", Output: "O", Path: "c[]"}, {Package: "p", Output: "O", Path: "a"}, {Package: "p", Output: "O", Path: "d"}},
		DecoderMismatched: []OrbitField{{Package: "p", Output: "O", Path: "d"}, {Package: "p", Output: "O", Path: "c[]"}},
	}

	check = check.classified()

	// Each finding is read as its category and reason; a kind declaration
	// answers the depth it names, the elements of c and not c itself, and
	// nothing the other questions found at the same path.
	answers := func(found []OrbitField) []string {
		var out []string
		for _, finding := range found {
			out = append(out, finding.Category+"/"+finding.Reason)
		}
		return out
	}
	got := [][]string{answers(check.Unpublished), answers(check.Unsurfaced), answers(check.Mismatched), answers(check.DecoderMismatched)}
	want := [][]string{
		{"published-cat/published-why", "/", "/"},
		{"sent-cat/sent-why", "/", "/"},
		{"kind-cat/kind-why", "/", "/"},
		{"decoder-cat/decoder-why", "/"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %q, want %q", got, want)
	}
	if check.undeclared() != 7 {
		t.Errorf("undeclared() = %d, want the seven findings no declaration answered", check.undeclared())
	}
	if want := []string{"p.O w (decoder-kind)", "p.O x (kind)", "p.O y (published)", "p.O z (sent)"}; !slices.Equal(check.UnusedDeclarations, want) {
		t.Errorf("unused = %q, want %q", check.UnusedDeclarations, want)
	}
	stale := check.staleDeclarations()
	if len(stale) != 4 || !strings.HasPrefix(stale[0], "p.O w (decoder-kind) is declared against the Orbit response record and no finding matched it") {
		t.Errorf("staleDeclarations() = %q", stale)
	}
	nothing := OrbitCheck{UnusedDeclarations: []string{"left over"}}.classified()
	if nothing.Unpublished != nil || nothing.Unsurfaced != nil || nothing.Mismatched != nil || nothing.DecoderMismatched != nil ||
		len(nothing.UnusedDeclarations) != 8 {
		t.Errorf("classified() of nothing = %+v", nothing)
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
			if !slices.Contains([]orbitDirection{orbitPublished, orbitSent, orbitKind, orbitDecoderKind}, declaration.Direction) {
				t.Errorf("declaration answers direction %q, which no question asks", declaration.Direction)
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

// orbitRunTree prepares a tree whose only R-PATH input of interest is the
// Orbit record, with one recorded call of the status shape carrying a key no
// field publishes and a kind its field does not decode, and one of a type
// whose field the kinds question cannot judge.
func orbitRunTree(t *testing.T) string {
	t.Helper()
	root := orbitTree(t)
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
	stubOrbitRecord(t, orbitrecord.Document{Calls: []orbitrecord.Call{
		orbitCall("orbit.status", "raw", "StatusOutput", keys(orbitrecord.Root, "user", "user.available=boolean", "system", "system.status=number", "added")),
		orbitCall("orbit.clock", "raw", "ClockOutput", keys(orbitrecord.Root, "uptime=number")),
	}}, nil)
	return root
}

// orbitRunClientGo is the client-go package the run hands the decoder half:
// the struct the status output is filled from, whose status is still a
// string, and a type that decodes itself where the answers carry a key the
// output does not publish.
const orbitRunClientGo = `package gitlab

type Status struct {
	User   *StatusUser   ` + "`json:\"user,omitempty\"`" + `
	System *StatusSystem ` + "`json:\"system,omitempty\"`" + `
	Added  Opaque        ` + "`json:\"added\"`" + `
}

type StatusUser struct {
	Available bool ` + "`json:\"available\"`" + `
}

type StatusSystem struct {
	Status string ` + "`json:\"status,omitempty\"`" + `
}

type Opaque struct{}

func (o *Opaque) UnmarshalJSON(data []byte) error { return nil }
`

// TestRun_TheOrbitRecord_IsJudgedAndGatesNothing verifies the wiring into the
// report: the Orbit section and its counters are published, a kind a field
// does not carry and one a client-go struct does not decode count among the
// findings, a field whose type cannot be judged is counted apart in either
// half, and none of it fails the run.
func TestRun_TheOrbitRecord_IsJudgedAndGatesNothing(t *testing.T) {
	root := orbitRunTree(t)
	clientGo := t.TempDir()
	writeSourceFile(t, clientGo, "orbit.go", orbitRunClientGo)
	previous := readDecoders
	readDecoders = func(string) *decoderSource {
		return decoderSourceFrom(structs.Pairings{
			ClientGoDir: clientGo,
			Outputs:     []structs.OutputPairing{{Package: "orbitx", MCPType: "StatusOutput", SDKType: "Status"}},
		})
	}
	t.Cleanup(func() { readDecoders = previous })

	content, clean, err := Run(t.Context(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := decode(t, content)
	summary := report.Summary
	got := []int{
		summary.OrbitCalls, summary.OrbitCompared, summary.OrbitNested, summary.OrbitKindsCompared, summary.OrbitDecoderKindsCompared,
		summary.OrbitUnpublished, summary.OrbitUnsurfaced, summary.OrbitMismatched, summary.OrbitDecoderMismatched,
		summary.OrbitKindsUnjudged, summary.OrbitDecoderKindsUnjudged, summary.OrbitUndeclared, summary.TypedJudgedByOrbit,
	}
	if want := []int{2, 2, 4, 4, 4, 5, 1, 1, 1, 1, 1, 8, 0}; !clean || !slices.Equal(got, want) {
		t.Errorf("clean = %t, orbit counters = %v, want %v", clean, got, want)
	}
	orbit := report.Shapes.Orbit
	if !orbit.Ran || len(orbit.Unsurfaced) != 1 || len(orbit.Mismatched) != 1 || orbit.Mismatched[0].Path != "system.status" ||
		len(orbit.KindsUnjudged) != 1 || orbit.KindsUnjudged[0].GoType != "time.Duration" {
		t.Errorf("orbit = %+v", orbit)
	}
	if !orbit.DecodersRead || len(orbit.DecoderMismatched) != 1 || orbit.DecoderMismatched[0].Type != "gl.StatusSystem" ||
		len(orbit.DecoderKindsUnjudged) != 1 || orbit.DecoderKindsUnjudged[0].Path != "added" ||
		!slices.Equal(orbit.Decoders, []string{"orbitx.StatusOutput from gl.Status"}) || !slices.Equal(orbit.Undecoded, []string{"orbitx.ClockOutput"}) {
		t.Errorf("decoder half = %+v", orbit)
	}
}

// TestRun_AStaleOrbitDeclaration_FailsTheGate verifies the one way the Orbit
// section reaches the gate: a declaration that answers nothing is a stale
// declaration.
func TestRun_AStaleOrbitDeclaration_FailsTheGate(t *testing.T) {
	root := orbitRunTree(t)
	declaredOrbitFields = []orbitDeclaration{{Package: "internal/tools/orbitx", Output: "StatusOutput", Path: "gone", Direction: orbitPublished, Category: "c", Reason: "r"}}

	content, clean, err := Run(t.Context(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := decode(t, content)
	if clean || report.Summary.StaleDeclarations != 1 || !strings.HasPrefix(report.StaleDeclarations[0], "internal/tools/orbitx.StatusOutput gone (published) is declared") {
		t.Errorf("clean = %t, stale = %q", clean, report.StaleDeclarations)
	}
}
