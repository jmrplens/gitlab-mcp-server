//go:build e2e

// served_test.go covers the check that runs once when a session starts.
//
// Two halves. The comparison itself is pure and is tested with lists: what it
// reports, what it ignores, and what it says when the two sides differ. The
// expectation it compares against is built from the real assemblers, so the
// assertions about it are about the rules registration applies after a catalog
// is built rather than about a fixture.

package harness

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// TestCheckServedTools_TheSameSet_Passes checks the ordinary case: a session
// serving what the assemblers say it should is accepted.
func TestCheckServedTools_TheSameSet_Passes(t *testing.T) {
	expected := surfaceExpectation{
		tools:      []string{"gitlab_issue", "gitlab_project"},
		standalone: []string{"gitlab_discover_project"},
	}

	err := checkServedTools(SurfaceMeta, []string{"gitlab_discover_project", "gitlab_issue", "gitlab_project"}, expected)
	if err != nil {
		t.Fatalf("checkServedTools() = %v, want nil", err)
	}
}

// TestCheckServedTools_Differences_AreReportedBothWays checks that the report
// names what is missing and what is extra.
//
// Both halves matter and they mean different things: a tool the assemblers
// expect and the server did not register is a tool no test can reach, and one
// the server registered that the assemblers do not know about is a tool
// nothing in the catalog accounts for.
func TestCheckServedTools_Differences_AreReportedBothWays(t *testing.T) {
	expected := surfaceExpectation{tools: []string{"gitlab_issue", "gitlab_admin"}}

	err := checkServedTools(SurfaceMeta, []string{"gitlab_issue", "gitlab_surprise"}, expected)

	if err == nil {
		t.Fatal("checkServedTools() accepted two different sets")
	}
	for _, want := range []string{"gitlab_admin", "gitlab_surprise", "expected and not served", "served and not expected"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the report does not mention %q:\n%v", want, err)
			}
		})
	}
}

// TestCheckServedTools_StandaloneNames_AreIgnoredOnBothSides checks that the
// tools registered outside the catalog are left out of the comparison whether
// they were served, expected, or both.
//
// Both sides, because the individual surface projects the standalone groups
// into tools of its own as well as registering them separately: removing them
// from one side alone would make the filtering itself the difference.
func TestCheckServedTools_StandaloneNames_AreIgnoredOnBothSides(t *testing.T) {
	expected := surfaceExpectation{
		tools:      []string{"gitlab_issue", "gitlab_discover_project"},
		standalone: []string{"gitlab_discover_project", "gitlab_interactive_issue_create"},
	}

	err := checkServedTools(SurfaceIndividual, []string{"gitlab_issue", "gitlab_interactive_issue_create"}, expected)
	if err != nil {
		t.Fatalf("checkServedTools() = %v, want the standalone names ignored on both sides", err)
	}
}

// TestCheckServedTools_ManyDifferences_AreTruncated checks that a session
// differing wholesale reports a count rather than a thousand names.
func TestCheckServedTools_ManyDifferences_AreTruncated(t *testing.T) {
	expected := surfaceExpectation{}
	served := make([]string, 0, missingDiffLimit*3)
	for i := range missingDiffLimit * 3 {
		served = append(served, "gitlab_tool_"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}

	err := checkServedTools(SurfaceIndividual, served, expected)

	if err == nil {
		t.Fatal("checkServedTools() accepted a session serving tools nothing expected")
	}
	if !strings.Contains(err.Error(), "more)") {
		t.Errorf("a difference of %d names was not truncated:\n%v", len(served), err)
	}
}

// TestDifference_ReportsOnlyWhatIsMissingAndDeduplicates pins the set helper
// the report is built on.
func TestDifference_ReportsOnlyWhatIsMissingAndDeduplicates(t *testing.T) {
	cases := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{name: "nothing missing", a: []string{"one", "two"}, b: []string{"two", "one"}, want: nil},
		{name: "one missing", a: []string{"one", "two"}, b: []string{"one"}, want: []string{"two"}},
		{name: "duplicates collapse", a: []string{"two", "two"}, b: []string{"one"}, want: []string{"two"}},
		{name: "empty b", a: []string{"one"}, b: nil, want: []string{"one"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := difference(testCase.a, testCase.b); !slices.Equal(got, testCase.want) {
				t.Errorf("difference(%v, %v) = %v, want %v", testCase.a, testCase.b, got, testCase.want)
			}
		})
	}
}

// TestExpectedSurface_EachSurface_NamesWhatItRegisters checks the three
// assembler calls against what each surface actually registers.
//
// The dynamic surface registers two tools whatever the catalog holds, the meta
// surface one per group, and the individual surface one per action that
// declares a name. Getting any of those wrong would make every session of that
// surface abort with a difference that is the harness's own.
func TestExpectedSurface_EachSurface_NamesWhatItRegisters(t *testing.T) {
	inst := stubInstance(t)

	cases := []struct {
		name    string
		surface Surface
		check   func(*testing.T, surfaceExpectation)
	}{
		{
			name:    "dynamic",
			surface: SurfaceDynamic,
			check: func(t *testing.T, expected surfaceExpectation) {
				t.Helper()
				if len(expected.tools) != 2 {
					t.Errorf("the dynamic surface expects %v, want the two find and execute tools", expected.tools)
				}
				if _, found := expected.actions["issue.list"]; !found {
					t.Error("the dynamic catalog does not carry issue.list")
				}
			},
		},
		{
			name:    "meta",
			surface: SurfaceMeta,
			check: func(t *testing.T, expected surfaceExpectation) {
				t.Helper()
				if !slices.Contains(expected.tools, "gitlab_issue") {
					t.Errorf("the meta surface does not expect gitlab_issue: %v", expected.tools)
				}
				if slices.Contains(expected.tools, "gitlab_issue_list") {
					t.Error("the meta surface expects an individual tool name")
				}
			},
		},
		{
			name:    "individual",
			surface: SurfaceIndividual,
			check: func(t *testing.T, expected surfaceExpectation) {
				t.Helper()
				if !slices.Contains(expected.tools, "gitlab_issue_list") {
					t.Error("the individual surface does not expect gitlab_issue_list")
				}
				if _, found := expected.actions["server.health_check"]; found {
					t.Error("the individual surface expects server.health_check, which declares no individual tool")
				}
				if _, found := expected.actions["repository.file_history"]; found {
					t.Error("the individual surface expects repository.file_history, whose tool name repository.commit_list owns")
				}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := ServerConfig{Surface: testCase.surface}.normalized()
			expected, err := expectedSurface(inst, cfg.Surface, serverConfigFor(inst, cfg, inst.credential()))
			if err != nil {
				t.Fatalf("expectedSurface(%s): %v", testCase.surface, err)
			}
			testCase.check(t, expected)
		})
	}
}

// TestIndividualRegistrations_ReadOnlyMode_KeepsOnlyTheReads checks the step
// the individual surface takes after its catalog is built.
//
// Read-only mode is applied there by removing registered tools whose
// annotation says they write, rather than by filtering the catalog as the
// other two surfaces do. An expectation that skipped it would name every write
// tool the server removed, and every read-only session would abort.
func TestIndividualRegistrations_ReadOnlyMode_KeepsOnlyTheReads(t *testing.T) {
	inst := stubInstance(t)
	serverCfg := serverConfigFor(inst, ServerConfig{Surface: SurfaceIndividual, Mode: ModeReadOnly}.normalized(), inst.credential())
	catalog, _, err := gitlabtools.SharedIndividualCatalog(inst.client, serverCfg)
	if err != nil {
		t.Fatalf("assembling the individual catalog: %v", err)
	}

	all, _ := individualRegistrations(catalog, false)
	reads, readActions := individualRegistrations(catalog, true)

	if len(reads) >= len(all) {
		t.Fatalf("read-only mode kept %d of %d individual tools, want fewer", len(reads), len(all))
	}
	if !slices.Contains(reads, "gitlab_issue_list") {
		t.Error("read-only mode removed gitlab_issue_list, which reads")
	}
	if slices.Contains(reads, "gitlab_project_delete") {
		t.Error("read-only mode kept gitlab_project_delete, which writes")
	}
	if _, found := readActions["project.delete"]; found {
		t.Error("the read-only action set carries project.delete")
	}
}

// TestServerConfigFor_ReadOnlyCredential_NarrowsTheSurface checks that the
// expectation is built through the same narrowing the binary applies.
//
// A token that cannot write is served a read-only surface whatever the
// deployment asked for. An expectation built without that call would name
// every write tool, and every session using such a credential would abort with
// a difference that is not the server's.
func TestServerConfigFor_ReadOnlyCredential_NarrowsTheSurface(t *testing.T) {
	inst := stubInstance(t)

	cases := []struct {
		name         string
		scopes       []string
		wantReadOnly bool
	}{
		{name: "read_api only", scopes: []string{"read_api"}, wantReadOnly: true},
		{name: "api", scopes: []string{"api"}, wantReadOnly: false},
		{name: "undetected", scopes: nil, wantReadOnly: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			serverCfg := serverConfigFor(inst, ServerConfig{}.normalized(), credentialFacts{scopes: testCase.scopes, tier: inst.facts.Tier})
			if serverCfg.ReadOnly != testCase.wantReadOnly {
				t.Errorf("ReadOnly = %t, want %t for scopes %v", serverCfg.ReadOnly, testCase.wantReadOnly, testCase.scopes)
			}
			if testCase.wantReadOnly && !serverCfg.ReadOnlyFromTokenScope {
				t.Error("the narrowing did not record that the credential caused it")
			}
		})
	}
}

// TestServerConfigFor_CredentialTier_DecidesTheCatalog checks that the tier
// the expectation is built at is the credential's and not the run's: a token
// that could read no license is served the Free catalog on a licensed
// instance, and the expectation for it must name the Free groups only.
func TestServerConfigFor_CredentialTier_DecidesTheCatalog(t *testing.T) {
	inst := stubInstance(t)
	inst.facts.Tier = edition.Ultimate

	licensed := serverConfigFor(inst, ServerConfig{Surface: SurfaceMeta}.normalized(), inst.credential())
	unlicensed := serverConfigFor(inst, ServerConfig{Surface: SurfaceMeta}.normalized(), credentialFacts{tier: edition.Free})

	if licensed.Tier != edition.Ultimate || unlicensed.Tier != edition.Free {
		t.Fatalf("tiers = %s and %s, want the credential's: ultimate and free", licensed.Tier, unlicensed.Tier)
	}
	withLicense, err := expectedSurface(inst, SurfaceMeta, licensed)
	if err != nil {
		t.Fatalf("expectedSurface(ultimate): %v", err)
	}
	withoutLicense, err := expectedSurface(inst, SurfaceMeta, unlicensed)
	if err != nil {
		t.Fatalf("expectedSurface(free): %v", err)
	}
	if len(withoutLicense.tools) >= len(withLicense.tools) {
		t.Errorf("the Free expectation names %d groups and the Ultimate one %d, and Free is a subset",
			len(withoutLicense.tools), len(withLicense.tools))
	}
	if slices.Contains(withoutLicense.tools, "gitlab_vulnerability") {
		t.Error("the Free expectation names gitlab_vulnerability, a licensed group")
	}
}

// TestServerConfigFor_CarriesTheShapeTheBinaryBuilt checks the mapping from a
// harness configuration to the server configuration the assemblers take.
func TestServerConfigFor_CarriesTheShapeTheBinaryBuilt(t *testing.T) {
	inst := stubInstance(t)

	serverCfg := serverConfigFor(inst, ServerConfig{
		Surface:      SurfaceMeta,
		Mode:         ModeSafe,
		Capabilities: CapabilitiesMinimal,
		ExcludeTools: []string{"gitlab_issue"},
	}.normalized(), credentialFacts{scopes: []string{"api"}, tier: inst.facts.Tier})

	cases := []struct {
		name string
		got  any
		want any
	}{
		{name: "surface", got: serverCfg.ToolSurface, want: config.ToolSurfaceMeta},
		{name: "capabilities", got: serverCfg.CapabilitySurface, want: config.CapabilitySurfaceMinimal},
		{name: "safe mode", got: serverCfg.SafeMode, want: true},
		{name: "read-only", got: serverCfg.ReadOnly, want: false},
		{name: "tier", got: serverCfg.Tier, want: inst.facts.Tier},
		{name: "instance", got: serverCfg.GitLabURL, want: inst.facts.URL},
		{name: "exclusions", got: strings.Join(serverCfg.ExcludeTools, ","), want: "gitlab_issue"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

// TestStandaloneToolNames_AreTheOnesRegisteredOutsideTheCatalog pins the list
// the comparison ignores, so a tool moving into the catalog is noticed here
// rather than as an unexplained difference in every session.
func TestStandaloneToolNames_AreTheOnesRegisteredOutsideTheCatalog(t *testing.T) {
	client := gitlabclient.NewUnboundClient("https://gitlab.invalid")

	names := standaloneToolNames(client)

	if len(names) == 0 {
		t.Fatal("no standalone tools were named, and the binary registers several")
	}
	if !slices.Contains(names, "gitlab_discover_project") {
		t.Errorf("gitlab_discover_project is not among the standalone tools: %v", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("the standalone names are not sorted: %v", names)
	}
}
