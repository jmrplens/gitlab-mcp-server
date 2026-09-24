package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// fixerCatalog is the small oracle the fixer is driven against: two IDs and
// the individual tool each projects, one of them deliberately not derivable
// from its ID.
//
// gitlab_fetch_demo is what makes these tests about the real problem. If every
// tool name were gitlab_<domain>_<action> the fixer would be a formula and
// would need no catalog at all; the individual name is declared per action, so
// the catalog is the only thing that can answer.
func fixerCatalog() *actionids.IDs {
	return actionids.NewWithTools(
		[]string{"demo.get", "demo.list"},
		nil,
		map[string]string{"gitlab_fetch_demo": "demo.get", "gitlab_demo_list": "demo.list"},
	)
}

// stagePackage writes one package of files under a temporary root and returns
// the root, so a test states the source it is about and nothing else.
func stagePackage(t *testing.T, pkg string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(pkg))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// readStaged reads one staged file back.
func readStaged(t *testing.T, root, pkg, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pkg), name)) //#nosec G304 -- a path this test just built
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// runtimeIsWindows is the one platform question these tests ask, for the link
// a test plants and Windows does not grant the privilege to make.
func runtimeIsWindows() bool { return runtime.GOOS == "windows" }

// fixerSite is one folded hint of the staged package.
func fixerSite(value string) site {
	return site{Package: "demo", File: "demo/demo.go", Line: 1, Kind: kindErrorHint, Value: value, Resolved: true}
}

// TestFixHints_RewritesTheHintAndLeavesTheDescription is the rule the whole
// pass rests on, and the one a formula would get wrong.
//
// The same tool name is correct in an individual tool's Description ("See
// also: gitlab_fetch_demo") and wrong in a hint, and the two sit in the same
// file. What separates them is that the hint's text is one the walk folded and
// the description's is not, so the description is not reachable by this pass
// however often it spells the same tool.
func TestFixHints_RewritesTheHintAndLeavesTheDescription(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\n" +
			"const notFound = \"" + hint + "\"\n\n" +
			"const description = \"Get one demo. See also: gitlab_fetch_demo.\"\n",
	})

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}

	body := readStaged(t, root, "demo", "demo.go")
	if !strings.Contains(body, "verify demo_id with demo.get first") {
		t.Errorf("the hint was not rewritten:\n%s", body)
	}
	if !strings.Contains(body, "See also: gitlab_fetch_demo.") {
		t.Errorf("the description moved, and the tool name is correct there:\n%s", body)
	}
	if report.Files != 1 || len(report.Fixes) != 1 {
		t.Fatalf("report = %+v, want one file and one name", report)
	}
	fix := report.Fixes[0]
	if fix.Tool != "gitlab_fetch_demo" || fix.ID != "demo.get" || fix.File != "demo/demo.go" || fix.Line != 3 {
		t.Errorf("fix = %+v, want the tool, the ID it projects and where it was", fix)
	}
}

// TestFixHints_LeavesALiteralThatIsOnlyAName holds the second rule, which was
// found by running without it.
//
// A tool name is also written as a literal that is nothing but the name, and
// such a literal is contained in every hint that mentions that tool. The first
// run over this tree therefore rewrote 48 lines of
// internal/tools/projects/action_specs.go and renamed the tools. A hint reads
// as a sentence and a name never does, so a literal with no space in it is not
// one this pass has anything to say about.
func TestFixHints_LeavesALiteralThatIsOnlyAName(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst toolDemoGet = \"gitlab_fetch_demo\"\n\nconst notFound = \"" + hint + "\"\n",
	})

	if _, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false); err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}

	body := readStaged(t, root, "demo", "demo.go")
	if !strings.Contains(body, "const toolDemoGet = \"gitlab_fetch_demo\"") {
		t.Errorf("the tool name constant was rewritten, which renames the tool:\n%s", body)
	}
	if !strings.Contains(body, "verify demo_id with demo.get first") {
		t.Errorf("the hint beside it was not rewritten:\n%s", body)
	}
}

// TestFixHints_TestFilesMoveOnlyWhenAsked holds both halves of the test
// rewrite: a test that pins a hint has to move with it, or the pass leaves the
// suite red for a reason that is not a defect, and it moves only under
// -fix-hints-tests, so a reviewer can read the production half on its own.
func TestFixHints_TestFilesMoveOnlyWhenAsked(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	files := map[string]string{
		"demo.go":      "package demo\n\nconst notFound = \"" + hint + "\"\n",
		"demo_test.go": "package demo\n\nconst wantHint = \"" + hint + "\"\n",
	}

	t.Run("left alone by default", func(t *testing.T) {
		root := stagePackage(t, "demo", files)
		if _, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false); err != nil {
			t.Fatalf("fixHints() error = %v", err)
		}
		if body := readStaged(t, root, "demo", "demo_test.go"); !strings.Contains(body, "gitlab_fetch_demo") {
			t.Errorf("the test moved without being asked:\n%s", body)
		}
	})

	t.Run("moved when asked", func(t *testing.T) {
		root := stagePackage(t, "demo", files)
		report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), true)
		if err != nil {
			t.Fatalf("fixHints() error = %v", err)
		}
		if body := readStaged(t, root, "demo", "demo_test.go"); !strings.Contains(body, "verify demo_id with demo.get first") {
			t.Errorf("the assertion did not move with the hint:\n%s", body)
		}
		if report.Files != 2 {
			t.Errorf("files = %d, want the production file and its test", report.Files)
		}
	})
}

// TestFixHints_ATestLiteralHoldingNoHint_IsLeft holds what keeps the test
// pass from being a rename: a test literal that spells a tool and, rewritten,
// would hold no hint the walk folded is an assertion about something else,
// and a rewrite there would have no fix behind it.
func TestFixHints_ATestLiteralHoldingNoHint_IsLeft(t *testing.T) {
	const hint = "verify demo_id first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go":      "package demo\n\nconst notFound = \"" + hint + "\"\n",
		"demo_test.go": "package demo\n\nconst wantHint = \"verify demo_id with gitlab_fetch_demo first\"\n",
	})

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), true)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	if report.Files != 0 || len(report.Fixes) != 0 {
		t.Fatalf("report = %+v, want nothing rewritten", report)
	}
	if body := readStaged(t, root, "demo", "demo_test.go"); !strings.Contains(body, "gitlab_fetch_demo") {
		t.Errorf("a test moved although no hint did:\n%s", body)
	}
}

// TestFixHints_ATestPinningARenderedHint_MovesAfterProductionAlreadyDid holds
// the shape a formatter's test is written in, and the order the tree was
// fixed in.
//
// A test pins the bullet a hint renders as ("- " + hint + "\n"), or the card
// around it, and that literal contains the hint rather than being contained in
// it, which is the only thing the production rule admits. Read as it will be
// written, it contains the hint exactly when the assertion is about that
// sentence. The production text here is already canonical, as it is once a
// first run has moved it, and the test still moves, while a literal that
// names the same tool beside no hint (an individual tool's description) stays.
func TestFixHints_ATestPinningARenderedHint_MovesAfterProductionAlreadyDid(t *testing.T) {
	const hint = "verify demo_id with demo.get first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst notFound = \"" + hint + "\"\n",
		"demo_test.go": "package demo\n\n" +
			"const wantBullet = \"\\n---\\n- verify demo_id with gitlab_fetch_demo first\\n\"\n\n" +
			"const wantDescription = \"Get one demo. See also: gitlab_fetch_demo.\"\n\n" +
			"const wantName = \"gitlab_fetch_demo\"\n",
	})

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), true)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	body := readStaged(t, root, "demo", "demo_test.go")
	if !strings.Contains(body, `"\n---\n- verify demo_id with demo.get first\n"`) {
		t.Errorf("the rendered bullet did not move:\n%s", body)
	}
	for _, kept := range []string{"See also: gitlab_fetch_demo.", `wantName = "gitlab_fetch_demo"`} {
		t.Run(kept, func(t *testing.T) {
			if !strings.Contains(body, kept) {
				t.Errorf("a literal holding no hint moved:\n%s", body)
			}
		})
	}
	if report.Files != 1 || len(report.Fixes) != 1 {
		t.Errorf("report = %+v, want the one test file and the one name", report)
	}
}

// TestFixHints_ATestPinningARenderedHint_MovesInTheRunThatMovesProduction
// holds the other order: the production text still spells the tool, and one
// run moves it and the test that pins it.
//
// The values the fixer judges a test literal against are the ones the walk
// folded before anything was rewritten, so they still spell the old name. A
// test literal read with its tool names rewritten no longer contains a value
// that spells the old one, and was left pinning a sentence the same run had
// just moved; the value is read with its tool names rewritten too.
func TestFixHints_ATestPinningARenderedHint_MovesInTheRunThatMovesProduction(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go":      "package demo\n\nconst notFound = \"" + hint + "\"\n",
		"demo_test.go": "package demo\n\nconst wantBullet = \"\\n---\\n- verify demo_id with gitlab_fetch_demo first\\n\"\n",
	})

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), true)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	for file, want := range map[string]string{
		"demo.go":      `"verify demo_id with demo.get first"`,
		"demo_test.go": `"\n---\n- verify demo_id with demo.get first\n"`,
	} {
		t.Run(file, func(t *testing.T) {
			if body := readStaged(t, root, "demo", file); !strings.Contains(body, want) {
				t.Errorf("%s did not move:\n%s", file, body)
			}
		})
	}
	if report.Files != 2 || len(report.Fixes) != 2 {
		t.Errorf("report = %+v, want the production file and its test, one name each", report)
	}
}

// TestFixHints_AOneWordHint_AdmitsNoTestLiteral holds the second half of the
// sentence test for the pinning rule: a folded value with no space in it is a
// name rather than a hint, and a literal containing it is not one pinning it.
func TestFixHints_AOneWordHint_AdmitsNoTestLiteral(t *testing.T) {
	root := stagePackage(t, "demo", map[string]string{
		"demo.go":      "package demo\n\nconst tag = \"demo.get\"\n",
		"demo_test.go": "package demo\n\nconst want = \"use gitlab_fetch_demo for demo.get\"\n",
	})
	report, err := fixHints(root, []site{fixerSite("demo.get")}, fixerCatalog(), true)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	if len(report.Fixes) != 0 {
		t.Errorf("fixes = %+v, want nothing moved by a one-word value", report.Fixes)
	}
}

// TestPinsAHint_TheHintMustReadAsASentence holds the pinning rule on its own:
// a literal holding a whole sentence the walk folded pins it, and one holding
// a one-word value pins nothing, since that value is contained in every
// literal that spells it.
func TestPinsAHint_TheHintMustReadAsASentence(t *testing.T) {
	if pinsAHint("use demo.get", []string{"demo.get"}) {
		t.Error("a literal holding a one-word value was read as pinning a hint")
	}
	if !pinsAHint("- read it with demo.get\n", []string{"read it with demo.get"}) {
		t.Error("a bullet holding the whole hint was not read as pinning it")
	}
}

// TestFixHints_EveryKindOfServedProse_IsRewritten holds that the fixer takes
// its values from every kind the served-prose rule judges: an error's message,
// a format kept verbatim, a next step and a Usage line, each rewritten in the
// literal it was written as, while the individual tool's Description in the
// same file keeps the name that is right there.
func TestFixHints_EveryKindOfServedProse_IsRewritten(t *testing.T) {
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\n" +
			"var errMissing = errors.New(\"project_id is required. Use gitlab_demo_list to find it\")\n\n" +
			"func wrap(id int) error { return fmt.Errorf(\"demo %q: read it with gitlab_fetch_demo\", id) }\n\n" +
			"func end(c *Card) { c.End(\"Use `gitlab_demo_list` to see the others\") }\n\n" +
			"var usage = \"Use after gitlab_demo_list returns an id.\"\n\n" +
			"var description = \"Get one demo. See also: gitlab_demo_list.\"\n",
	})
	sites := []site{
		{Package: "demo", File: "demo/demo.go", Line: 3, Kind: kindMessage, Value: "project_id is required. Use gitlab_demo_list to find it", Resolved: true},
		{Package: "demo", File: "demo/demo.go", Line: 5, Kind: kindMessage, Value: "demo %q: read it with gitlab_fetch_demo", Resolved: true},
		{Package: "demo", File: "demo/demo.go", Line: 7, Kind: kindNextStep, Value: "Use `gitlab_demo_list` to see the others", Resolved: true},
		{Package: "demo", File: "demo/demo.go", Line: 9, Kind: kindUsage, Value: "Use after gitlab_demo_list returns an id.", Resolved: true},
	}

	report, err := fixHints(root, sites, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	body := readStaged(t, root, "demo", "demo.go")
	for _, want := range []string{
		`"project_id is required. Use demo.list to find it"`,
		`"demo %q: read it with demo.get"`,
		"\"Use `demo.list` to see the others\"",
		`"Use after demo.list returns an id."`,
		`"Get one demo. See also: gitlab_demo_list."`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("rewritten file is missing %s:\n%s", want, body)
			}
		})
	}
	if len(report.Fixes) != 4 {
		t.Errorf("fixes = %+v, want the four sentences and not the description", report.Fixes)
	}
}

// TestRenameTools_OnlyNamesTheCatalogResolves_AreRewritten holds the text a
// test literal is judged as: every tool the individual surface registers
// becomes its action's ID, and a name the catalog cannot answer for stays as
// written, since a guess there would pin a sentence nobody wrote.
func TestRenameTools_OnlyNamesTheCatalogResolves_AreRewritten(t *testing.T) {
	if got, want := renameTools("use gitlab_demo, then gitlab_fetch_demo", fixerCatalog()), "use gitlab_demo, then demo.get"; got != want {
		t.Errorf("renameTools() = %q, want %q", got, want)
	}
}

// TestFixHints_DeclaredTokens_AreNeitherRewrittenNorReported holds that the
// fixer reads the rule's declarations: a template family spelled like a tool
// and a tool the package's own surface registers are correct where they are,
// so they are neither rewritten nor sent to the list a reader works through
// by hand.
func TestFixHints_DeclaredTokens_AreNeitherRewrittenNorReported(t *testing.T) {
	const hint = "call gitlab_execute_action with a gitlab_ci_ymls template"
	const pkg = "internal/tools/dynamic"
	root := stagePackage(t, pkg, map[string]string{
		"register.go": "package dynamic\n\nconst refusal = \"" + hint + "\"\n",
	})
	at := site{Package: pkg, File: pkg + "/register.go", Line: 1, Kind: kindMessage, Value: hint, Resolved: true}

	report, err := fixHints(root, []site{at}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	if len(report.Fixes) != 0 || len(report.Unresolved) != 0 {
		t.Errorf("report = %+v, want the declared tokens neither moved nor listed", report)
	}
}

// TestFeedsFixer_ServedProseAndUsage_ButNotASchemaTag holds which sites the
// fixer takes its values from: every kind of served prose, the Usage line the
// served-prose rule judges too, and not a schema description, whose tag is one
// literal the fixer never rewrites, nor a published ID.
func TestFeedsFixer_ServedProseAndUsage_ButNotASchemaTag(t *testing.T) {
	for kind, want := range map[string]bool{
		kindErrorHint:         true,
		kindHintField:         true,
		kindMessage:           true,
		kindNextStep:          true,
		kindParamGuidance:     true,
		kindUsage:             true,
		kindSchemaDescription: false,
		kindDescription:       false,
		kindRelated:           false,
		kindHint:              false,
		kindAssertion:         false,
	} {
		t.Run(kind, func(t *testing.T) {
			if got := feedsFixer(kind); got != want {
				t.Errorf("feedsFixer(%q) = %t, want %t", kind, got, want)
			}
		})
	}
}

// TestFixHints_AToolTheCatalogDoesNotPublish_IsReportedNotGuessed holds what
// the fixer does when it has no answer.
//
// The meta tool names are the live case: a hint saying "use gitlab_demo action
// 'list'" names something the individual surface does not register, so the
// catalog resolves it to nothing. Writing a guess into served prose is worse
// than leaving the finding, so the name stays and the report names it.
func TestFixHints_AToolTheCatalogDoesNotPublish_IsReportedNotGuessed(t *testing.T) {
	const hint = "use gitlab_demo action 'list' to find one"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst notFound = \"" + hint + "\"\n",
	})

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	if len(report.Fixes) != 0 || report.Files != 0 {
		t.Fatalf("report = %+v, want nothing rewritten", report)
	}
	if len(report.Unresolved) != 1 || report.Unresolved[0].Tool != "gitlab_demo" {
		t.Fatalf("unresolved = %+v, want the name the catalog could not answer for", report.Unresolved)
	}
	if body := readStaged(t, root, "demo", "demo.go"); !strings.Contains(body, "gitlab_demo action") {
		t.Errorf("the prose was rewritten with a guess:\n%s", body)
	}
}

// TestFixHints_SitesThatAreNotFoldedHints_SelectNoPackage holds the filter the
// walk's own record is read through, one term at a time.
//
// Each case fails exactly one term and satisfies the other three, which is
// what makes every term decide something a caller can see. A case failing two
// at once would let either of them be dropped without anything noticing, which
// is how this filter came to have four mutants no test could kill: it used to
// be applied twice, and the second application hid the first.
func TestFixHints_SitesThatAreNotFoldedHints_SelectNoPackage(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	for _, testCase := range []struct {
		name string
		at   site
	}{
		{name: "another kind", at: site{Package: "demo", File: "demo/demo.go", Kind: kindRelated, Value: hint, Resolved: true}},
		{name: "not folded", at: site{Package: "demo", File: "demo/demo.go", Kind: kindErrorHint, Value: hint}},
		{name: "no package", at: site{File: "demo/demo.go", Kind: kindErrorHint, Value: hint, Resolved: true}},
		{name: "no text", at: site{Package: "demo", File: "demo/demo.go", Kind: kindErrorHint, Resolved: true}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := stagePackage(t, "demo", map[string]string{
				"demo.go": "package demo\n\nconst notFound = \"" + hint + "\"\n",
			})
			report, err := fixHints(root, []site{testCase.at}, fixerCatalog(), false)
			if err != nil {
				t.Fatalf("fixHints() error = %v", err)
			}
			if report.Files != 0 {
				t.Errorf("report = %+v, want the package left alone", report)
			}
		})
	}
}

// TestFixHints_ASourceItCannotRead_IsReportedRatherThanPassedOver holds the
// two failures of a rewriting pass, and both matter for the same reason: a
// rewrite that skipped what it could not read would report a clean run over a
// tree it had not finished.
func TestFixHints_ASourceItCannotRead_IsReportedRatherThanPassedOver(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"

	t.Run("a package directory that is not there", func(t *testing.T) {
		root := t.TempDir()
		_, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
		if err == nil {
			t.Fatal("fixHints() error = nil, want the missing directory reported")
		}
		if !strings.Contains(err.Error(), "demo") {
			t.Errorf("error = %v, want it to name the package it could not read", err)
		}
	})

	t.Run("a file that does not parse", func(t *testing.T) {
		root := stagePackage(t, "demo", map[string]string{"demo.go": "package demo\n\nthis is not Go\n"})
		_, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
		if err == nil {
			t.Fatal("fixHints() error = nil, want the unparseable file reported")
		}
		if !strings.Contains(err.Error(), "parse") {
			t.Errorf("error = %v, want it to say what it could not do", err)
		}
	})
}

// TestFixHints_SeveralNamesInOneFile_AreAllRewrittenAndTheFileStaysValid
// holds the byte arithmetic. Each rewrite is applied at the literal's own
// offsets, so two names in one file have to be written from one pass over the
// source rather than spliced one after another into a buffer whose offsets the
// previous splice already moved.
func TestFixHints_SeveralNamesInOneFile_AreAllRewrittenAndTheFileStaysValid(t *testing.T) {
	const first = "verify demo_id with gitlab_fetch_demo first"
	const second = "list them with gitlab_demo_list, then read one with gitlab_fetch_demo"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst (\n\tnotFound = \"" + first + "\"\n\tempty    = \"" + second + "\"\n)\n",
	})

	report, err := fixHints(root, []site{fixerSite(first), fixerSite(second)}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	if len(report.Fixes) != 3 {
		t.Fatalf("fixes = %+v, want all three names", report.Fixes)
	}
	body := readStaged(t, root, "demo", "demo.go")
	for _, want := range []string{
		"\tnotFound = \"verify demo_id with demo.get first\"\n",
		"\tempty    = \"list them with demo.list, then read one with demo.get\"\n",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("rewritten file is missing %q:\n%s", want, body)
			}
		})
	}
	if strings.Contains(body, "gitlab_") {
		t.Errorf("a name was left behind:\n%s", body)
	}
}

// TestWriteHintFixReport_SaysWhatMovedPerPackageAndWhatWasLeft holds the
// report a reviewer reads before the diff. Per package rather than per site,
// because 785 lines is a list a reader scrolls past, and the unresolved names
// are listed in full because there are few of them and each is a decision
// somebody has to make.
func TestWriteHintFixReport_SaysWhatMovedPerPackageAndWhatWasLeft(t *testing.T) {
	var out bytes.Buffer
	writeHintFixReport(&out, hintFixReport{
		Files: 2,
		Fixes: []hintFix{
			{File: "internal/tools/zeta/zeta.go", Line: 4, Tool: "gitlab_fetch_demo", ID: "demo.get"},
			{File: "internal/tools/alpha/alpha.go", Line: 9, Tool: "gitlab_demo_list", ID: "demo.list"},
			{File: "internal/tools/alpha/markdown.go", Line: 2, Tool: "gitlab_demo_list", ID: "demo.list"},
		},
		Unresolved: []hintFix{{File: "internal/tools/alpha/alpha.go", Line: 12, Tool: "gitlab_demo"}},
	})

	report := out.String()
	for _, want := range []string{
		"rewrote 3 tool name(s) in 2 file(s)",
		"1 tool name(s) left alone: the catalog publishes no such tool",
		"internal/tools/alpha/alpha.go:12 gitlab_demo",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(report, want) {
				t.Errorf("report = %q, want it to contain %q", report, want)
			}
		})
	}
	// The count is read out of its own line rather than matched with the
	// padding around it: the padding is a column width, and a test that pinned
	// it would fail on a longer package name instead of on a wrong count.
	counts := map[string]string{}
	for line := range strings.SplitSeq(report, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && strings.HasPrefix(fields[0], "internal/tools/") {
			counts[fields[0]] = fields[1]
		}
	}
	for pkg, want := range map[string]string{"internal/tools/alpha": "2", "internal/tools/zeta": "1"} {
		t.Run(pkg, func(t *testing.T) {
			if counts[pkg] != want {
				t.Errorf("%s moved %q name(s), want %q; report:\n%s", pkg, counts[pkg], want, report)
			}
		})
	}
	if strings.Index(report, "internal/tools/alpha ") > strings.Index(report, "internal/tools/zeta ") {
		t.Errorf("report = %q, want the packages in order so two runs read the same", report)
	}
}

// TestWriteHintFixReport_NothingLeftBehind_SaysNothingAboutIt holds the quiet
// half: a run with an answer for every name prints no list of exceptions,
// which is what makes the list worth reading when there is one.
func TestWriteHintFixReport_NothingLeftBehind_SaysNothingAboutIt(t *testing.T) {
	var out bytes.Buffer
	writeHintFixReport(&out, hintFixReport{Files: 1, Fixes: []hintFix{{File: "a/a.go", Tool: "t", ID: "demo.get"}}})

	if report := out.String(); strings.Contains(report, "left alone") {
		t.Errorf("report = %q, want no exception list when there are none", report)
	}
}

// TestFixHints_APackageWhoseHintsAreAllEmpty_IsSkipped holds the guard between
// the two readings of the walk's record: the package is selected because it
// folded a hint, and then the hint turns out to carry no text. Nothing can be
// looked for, so nothing is opened.
func TestFixHints_APackageWhoseHintsAreAllEmpty_IsSkipped(t *testing.T) {
	root := t.TempDir()
	report, err := fixHints(root, []site{{Package: "demo", File: "demo/demo.go", Kind: kindErrorHint, Resolved: true}}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v, want the package passed over rather than read", err)
	}
	if report.Files != 0 {
		t.Errorf("report = %+v, want nothing touched", report)
	}
}

// TestFixHints_OneSiteOfThePackageIsNotAHint_LeavesTheRestReadable holds the
// filter inside the value list: a package is selected by one folded hint and
// its other sites are read past, rather than folded into the text this pass
// looks for.
func TestFixHints_OneSiteOfThePackageIsNotAHint_LeavesTheRestReadable(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst notFound = \"" + hint + "\"\n",
	})

	report, err := fixHints(root, []site{
		fixerSite(hint),
		{Package: "demo", File: "demo/specs.go", Kind: kindRelated, Value: "demo.list", Resolved: true},
		{Package: "demo", File: "demo/specs.go", Kind: kindErrorHint, Expr: "buildHint(x)"},
	}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}
	if len(report.Fixes) != 1 {
		t.Errorf("fixes = %+v, want only the folded hint's name", report.Fixes)
	}
}

// TestFixHints_WhatIsNotGoSourceIsNotOpened holds what the directory walk
// passes over: a subdirectory, and a file that is not Go. Both would fail to
// parse, so admitting either would turn an ordinary package layout into a
// refusal.
func TestFixHints_WhatIsNotGoSourceIsNotOpened(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go":   "package demo\n\nconst notFound = \"" + hint + "\"\n",
		"README.md": "# not Go\n",
	})
	if err := os.MkdirAll(filepath.Join(root, "demo", "testdata.go"), 0o750); err != nil {
		t.Fatalf("plant the directory: %v", err)
	}

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v, want the walk to pass over what is not Go source", err)
	}
	if report.Files != 1 {
		t.Errorf("report = %+v, want only the one Go file rewritten", report)
	}
}

// TestFixHints_AFileItCannotRead_IsReported holds the read failure with the
// arrangement this repository already uses for one: a link to nothing, which a
// directory listing reports as an ordinary entry and an open cannot follow.
func TestFixHints_AFileItCannotRead_IsReported(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("making a link needs a privilege Windows does not grant by default")
	}
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst notFound = \"" + hint + "\"\n",
	})
	if err := os.Symlink("nowhere", filepath.Join(root, "demo", "aaa.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
	if err == nil {
		t.Fatal("fixHints() error = nil, want the unreadable file reported")
	}
	if !strings.Contains(err.Error(), "read") || !strings.Contains(err.Error(), "aaa.go") {
		t.Errorf("error = %v, want it to say what it could not do and to which file", err)
	}
}

// TestFixHints_AFileItCannotWrite_IsReportedRatherThanAbsorbed holds the one
// failure this pass must never pass over.
//
// It is reached through the writer seam, because by the time it can happen the
// file has been listed, read and parsed: what remains is outside the run's
// control, and no fixture can produce it. A rewrite that carried on would
// report a clean run over a tree it had half finished, and the report is what
// a reader trusts instead of the diff.
func TestFixHints_AFileItCannotWrite_IsReportedRatherThanAbsorbed(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go": "package demo\n\nconst notFound = \"" + hint + "\"\n",
	})
	restore := writeSource
	writeSource = func(string, []byte, os.FileMode) error { return errors.New("read-only filesystem") }
	defer func() { writeSource = restore }()

	_, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
	if err == nil {
		t.Fatal("fixHints() error = nil, want the failed write reported")
	}
	if !strings.Contains(err.Error(), "write") || !strings.Contains(err.Error(), "read-only filesystem") {
		t.Errorf("error = %v, want it to name the step and carry what the filesystem said", err)
	}
	if body := readStaged(t, root, "demo", "demo.go"); !strings.Contains(body, "gitlab_fetch_demo") {
		t.Errorf("the file changed although the write failed:\n%s", body)
	}
}

// TestFixHints_ATestFileThatDoesNotParse_IsReported holds the second walk's
// own failure: the production pass moved something, so the test files are
// opened, and one of them is not Go.
func TestFixHints_ATestFileThatDoesNotParse_IsReported(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	root := stagePackage(t, "demo", map[string]string{
		"demo.go":      "package demo\n\nconst notFound = \"" + hint + "\"\n",
		"demo_test.go": "package demo\n\nthis is not Go\n",
	})

	_, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), true)
	if err == nil {
		t.Fatal("fixHints() error = nil, want the unparseable test file reported")
	}
	if !strings.Contains(err.Error(), "demo_test.go") {
		t.Errorf("error = %v, want it to name the file", err)
	}
}

// TestFixHints_LiteralsThatCarryNoProse_AreLeftWhereTheyAre holds the two
// shapes the walk over a file declines before it ever looks for a tool name: a
// literal that is not a string at all, and a string with nothing in it but
// space. Neither can carry a hint, and the pass says so by moving nothing.
func TestFixHints_LiteralsThatCarryNoProse_AreLeftWhereTheyAre(t *testing.T) {
	const hint = "verify demo_id with gitlab_fetch_demo first"
	const source = "package demo\n\n" +
		"const retries = 3\n\n" +
		"const padding = \"   \"\n\n" +
		"const notFound = \"" + hint + "\"\n"
	root := stagePackage(t, "demo", map[string]string{"demo.go": source})

	report, err := fixHints(root, []site{fixerSite(hint)}, fixerCatalog(), false)
	if err != nil {
		t.Fatalf("fixHints() error = %v", err)
	}

	body := readStaged(t, root, "demo", "demo.go")
	if !strings.Contains(body, "const retries = 3") {
		t.Errorf("the number literal moved:\n%s", body)
	}
	if !strings.Contains(body, "const padding = \"   \"") {
		t.Errorf("the blank literal moved:\n%s", body)
	}
	if len(report.Fixes) != 1 {
		t.Errorf("fixes = %+v, want only the hint", report.Fixes)
	}
}
