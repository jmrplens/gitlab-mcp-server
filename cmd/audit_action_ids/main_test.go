package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// auditedPattern is a small, real package to run the whole command over. The
// command's own source publishes no action IDs, so the run is a complete one
// that finds nothing, which is exactly the shape a clean run has to take.
const auditedPattern = "./cmd/audit_action_ids/..."

// TestRun_WholeCommand_ReportsAndWritesTheWorkList drives run end to end and
// holds the two things it owes a caller: a report on stdout and the work list
// on disk for the layer that fixes the cross-links.
func TestRun_WholeCommand_ReportsAndWritesTheWorkList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "action-ids.json")
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, jsonPath: path}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), toolName+":") {
		t.Errorf("stdout = %q, want the summary line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+path) {
		t.Errorf("stdout = %q, want the work list named", stdout.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("work list: %v", err)
	}
}

// TestRun_NoWorkListPath_WritesNothing holds that an empty -json is a report
// and nothing else, which is how the audit is run over one package without
// overwriting the tree's work list.
func TestRun_NoWorkListPath_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "wrote ") {
		t.Errorf("stdout = %q, want no work list written", stdout.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("temporary directory holds %d entries (err %v), want none", len(entries), err)
	}
}

// TestRun_UnloadableSource_ExitsOne holds that a run that could not be made is
// a failure rather than a clean report. The audit does not fail on a finding,
// so this is the only thing that can send it home with a 1, and a silent
// success here would be a report over source nobody loaded.
func TestRun_UnloadableSource_ExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{"./cmd/audit_action_ids/nothing/..."}}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
}

// TestRun_UnwritableWorkList_ExitsOne holds that a work list that could not be
// written fails the run, since the layer downstream reads that file and an
// absent one would read as no work to do.
func TestRun_UnwritableWorkList_ExitsOne(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, jsonPath: filepath.Join(blocker, "action-ids.json")}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
}

// TestRun_Check_DeadCrossLink_FailsAndNamesIt drives the gate over a fixture
// that publishes an ID the catalog does not have, which is the whole point of
// the flip: the command reported this and now refuses it. A gate only ever
// exercised on a clean tree is one nobody has watched fail.
func TestRun_Check_DeadCrossLink_FailsAndNamesIt(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

// Spec publishes a cross-link to an action the catalog does not have.
var Spec = toolutil.ActionSpecOptions{RelatedActions: []string{"project.no_such_action"}}
`),
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: root, patterns: fixturePatterns, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run with -check = %d over a dead cross-link, want 1", code)
	}
	if !strings.Contains(stdout.String(), "project.no_such_action") {
		t.Errorf("stdout = %q, want the dead ID named", stdout.String())
	}
	if !strings.Contains(stderr.String(), "ERROR:") {
		t.Errorf("stderr = %q, want the gate's own failure line", stderr.String())
	}
}

// TestRun_Check_FailureLine_CountsEachRefusalUnderItsOwnName drives the gate
// over a fixture that trips three of its four refusals by different amounts,
// and holds the one stderr line a CI log is read by. The line is one Fprintf
// over four counters, and a fixture with one dead ID and nothing else, which
// is what the gate was first watched fail on, cannot tell "2 resolve to no
// action, 1 names an alias, 3 not folded" from the same numbers in another
// order. A prose alias is what fills the second slot, since a Usage line may
// spell one and only the two declared spellings are excused.
func TestRun_Check_FailureLine_CountsEachRefusalUnderItsOwnName(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Spec publishes two dead cross-links and a Usage line naming a registered
// alias, deploy_key.create, which resolves to access.deploy_key_add.
var Spec = toolutil.ActionSpecOptions{
	RelatedActions: []string{"project.no_such_action", "issue.no_such_action"},
	Usage:          "Chain deploy_key.create after this.",
}

// Unfolded publishes three lists nothing can fold.
var Unfolded = toolutil.ActionSpecOptions{
	RelatedActions: []string{os.Getenv("A"), os.Getenv("B"), os.Getenv("C")},
}
`),
	}
	var stdout, stderr bytes.Buffer

	// The fixture package alone, since the counts are the whole assertion and
	// toolutil publishes sites of its own that would move them.
	code := run(auditConfig{dir: root, patterns: []string{"./" + fixtureDir + "/..."}, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run with -check = %d, want 1; stdout %q", code, stdout.String())
	}
	const want = "\nERROR: 2 published ID(s) resolve to no action, 1 name a registered alias rather than a catalog ID, 3 site(s) could not be folded, 0 declaration(s) excuse nothing\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if !strings.Contains(stdout.String(), `usage "deploy_key.create" alias of access.deploy_key_add`) {
		t.Errorf("stdout = %q, want the prose alias named with what it resolves to", stdout.String())
	}
}

// TestRun_Check_CleanPackage_Passes holds the other side of the switch: with
// nothing to refuse, -check is silent and exits 0, so the flag cannot be one
// that fails on everything.
func TestRun_Check_CleanPackage_Passes(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, check: true}, &stdout, &stderr); code != 0 {
		t.Fatalf("run with -check = %d over a package publishing no IDs, stderr %q", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "ERROR:") {
		t.Errorf("stderr = %q, want nothing from a clean run", stderr.String())
	}
}

// TestPatternsOrDefault_NoArguments_AuditTheWholeTree holds what a bare run
// covers: every package that publishes an action ID.
func TestPatternsOrDefault_NoArguments_AuditTheWholeTree(t *testing.T) {
	if got := patternsOrDefault(nil); !slices.Equal(got, defaultPatterns) {
		t.Errorf("patternsOrDefault(nil) = %v, want %v", got, defaultPatterns)
	}
	if got := patternsOrDefault([]string{auditedPattern}); !slices.Equal(got, []string{auditedPattern}) {
		t.Errorf("patternsOrDefault named a pattern of its own: %v", got)
	}
	if !slices.Contains(defaultPatterns, "./internal/tools/...") {
		t.Errorf("defaultPatterns = %v, want the tree that publishes action IDs", defaultPatterns)
	}
}
