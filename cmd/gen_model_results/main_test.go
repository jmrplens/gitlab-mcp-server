package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// newRoot builds a repository root holding the two pages with their markers,
// and nothing else.
//
// The pages are assembled from the block table rather than pasted in, so a
// marker renamed in one place cannot leave a fixture behind that still carries
// the old one and passes.
func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range pagePaths() {
		var page bytes.Buffer
		page.WriteString("# " + path + "\n\nProse a person wrote.\n")
		for _, one := range blocks {
			if one.Path != path {
				continue
			}
			page.WriteString("\n" + one.Start + "\n" + one.End + "\n")
		}
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("make the page directory: %v", err)
		}
		if err := os.WriteFile(full, page.Bytes(), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

// commit writes a record holding the given rows into a root.
func commit(t *testing.T, root string, rows []row) {
	t.Helper()
	if err := writeRecord(root, document{Rows: rows}); err != nil {
		t.Fatalf("write the record: %v", err)
	}
}

// drive runs the command against a root and hands back its status and what it
// said, so a test asserts on the sentences a maintainer reads.
func drive(t *testing.T, root string, opts options) (status int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	status = run(root, opts, &out, &errs)
	return status, out.String(), errs.String()
}

// TestRun_NoCommittedRecord_PassesWithANote is the state this step leaves the
// repository in and the one it must not fail on: the record is written by a
// paid run, so a gate that failed for its absence would make every push red
// over a file nobody can write without spending money.
func TestRun_NoCommittedRecord_PassesWithANote(t *testing.T) {
	root := newRoot(t)
	if status, _, _ := drive(t, root, options{render: true}); status != exitOK {
		t.Fatalf("the render exited %d", status)
	}

	status, stdout, stderr := drive(t, root, options{check: true})
	if status != exitOK {
		t.Fatalf("the gate exited %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "nothing is published yet") {
		t.Errorf("the gate said %q, want a note naming the missing record", stdout)
	}
}

// TestRun_TheEmptyBlocks_AreGeneratedAndGated is what makes the withdrawal
// sentences generated rather than hand-written: a block edited by hand between
// the markers is stale, and the gate says so.
func TestRun_TheEmptyBlocks_AreGeneratedAndGated(t *testing.T) {
	root := newRoot(t)
	if status, _, stderr := drive(t, root, options{render: true}); status != exitOK {
		t.Fatalf("the render exited %d: %s", status, stderr)
	}
	page := filepath.Join(root, readmeRelPath)
	body, err := os.ReadFile(page) //#nosec G304 -- a path this test just built
	if err != nil {
		t.Fatalf("read the rendered page: %v", err)
	}
	if !strings.Contains(string(body), "is readable at commit") {
		t.Fatalf("the rendered README carries no withdrawal sentence:\n%s", body)
	}

	edited := strings.Replace(string(body), "Withdrawn.", "Actually, here are some numbers.", 1)
	//#nosec G703 -- a path this test just built under its own temporary directory
	if writeErr := os.WriteFile(page, []byte(edited), 0o600); writeErr != nil {
		t.Fatalf("edit the page: %v", writeErr)
	}
	status, _, stderr := drive(t, root, options{check: true})
	if status != exitFindings {
		t.Fatalf("a hand-edited block exited %d, want a finding", status)
	}
	if !strings.Contains(stderr, readmeRelPath) {
		t.Errorf("the finding %q does not name the file", stderr)
	}
}

// TestRun_ARecordAndItsPages_Agree drives the whole publishing path: a record
// with a row in it, the pages drawn from it, and a gate that then holds.
func TestRun_ARecordAndItsPages_Agree(t *testing.T) {
	root := newRoot(t)
	one := publishOne(t, publishableShard())
	one.Key.Tier = "ultimate"
	commit(t, root, []row{one})

	if status, _, stderr := drive(t, root, options{render: true}); status != exitOK {
		t.Fatalf("the render exited %d: %s", status, stderr)
	}
	page, err := os.ReadFile(filepath.Join(root, pageRelPath)) //#nosec G304 -- a path this test just built
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	if !strings.Contains(string(page), fixtureModel) {
		t.Errorf("the published page does not name the model it was drawn from:\n%s", page)
	}

	defer stubGit(t, func(string, ...string) (bool, error) { return false, nil })()
	status, stdout, stderr := drive(t, root, options{check: true})
	if status != exitOK {
		t.Fatalf("the gate exited %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "1 row(s), 1 cross-vendor table(s)") {
		t.Errorf("the gate summarized the record as %q", stdout)
	}
}

// TestRun_ARecordThePagesDoNotMatch_IsAFinding is the freshness half of the
// gate: the pages are a rendering of the record, and a rendering that has
// stopped matching is a page claiming something the record does not say.
func TestRun_ARecordThePagesDoNotMatch_IsAFinding(t *testing.T) {
	root := newRoot(t)
	if status, _, _ := drive(t, root, options{render: true}); status != exitOK {
		t.Fatal("the first render failed")
	}
	commit(t, root, []row{publishOne(t, publishableShard())})

	defer stubGit(t, func(string, ...string) (bool, error) { return false, nil })()
	status, _, stderr := drive(t, root, options{check: true})
	if status != exitFindings {
		t.Fatalf("a record the pages do not carry exited %d, want a finding", status)
	}
	if !strings.Contains(stderr, regenerate) {
		t.Errorf("the finding %q does not say how to fix it", stderr)
	}
}

// TestRun_ARowNoBlockPublishes_IsAFinding keeps a measurement from being
// scored, committed and shown to nobody. The individual surface has no block
// yet, and a record holding one of its rows has to say so.
func TestRun_ARowNoBlockPublishes_IsAFinding(t *testing.T) {
	root := newRoot(t)
	one := publishOne(t, publishableShard())
	one.Key.Surface = "individual"
	one.Key.SliceSize = 128
	one.Provenance.Surface = "individual"
	commit(t, root, []row{one})
	if status, _, stderr := drive(t, root, options{render: true}); status != exitOK {
		t.Fatalf("the render exited %d: %s", status, stderr)
	}

	defer stubGit(t, func(string, ...string) (bool, error) { return false, nil })()
	status, _, stderr := drive(t, root, options{check: true})
	if status != exitFindings {
		t.Fatalf("an unpublished row exited %d, want a finding", status)
	}
	if !strings.Contains(stderr, "no managed block publishes") {
		t.Errorf("the finding %q does not say what is wrong", stderr)
	}
}

// TestRun_FoldingAFakeRun_PublishesNothingAndSaysWhy is the verification the
// rebuild plan asks for by name: the fake proves the pipe and its every row is
// refused, row by row, naming the fake.
func TestRun_FoldingAFakeRun_PublishesNothingAndSaysWhy(t *testing.T) {
	root := newRoot(t)
	records := publishableShard()
	records[0].Run.Providers[0].Name = fakeProvider
	records[0].Run.Providers[0].Spec = "fake:perfect"
	records[1].Attempt.Model = "fake:perfect"
	records[4].Session.ToolSchemaDigests = map[string]string{"fake:perfect": fixtureTools}

	status, stdout, stderr := drive(t, root, options{shards: writeShard(t, records)})
	if status != exitOK {
		t.Fatalf("folding a fake run exited %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "[fake-provider]") || !strings.Contains(stdout, "fake:perfect") {
		t.Errorf("the fold said %q, want the row refused by name", stdout)
	}
	if !strings.Contains(stdout, "0 published") {
		t.Errorf("the fold said %q, want nothing published", stdout)
	}

	body, err := os.ReadFile(filepath.Join(root, recordRelPath)) //#nosec G304 -- a path this test just built
	if err != nil {
		t.Fatalf("the record was not written at all: %v", err)
	}
	if strings.Contains(string(body), "fake") {
		t.Errorf("the record carries the fake's row:\n%s", body)
	}
}

// TestRun_FoldingARealRun_WritesTheRecordAndRedrawsThePages is the same path
// with a row that survives: the record gains it and both pages are drawn again
// from the record rather than from the run.
func TestRun_FoldingARealRun_WritesTheRecordAndRedrawsThePages(t *testing.T) {
	root := newRoot(t)
	status, stdout, stderr := drive(t, root, options{shards: writeShard(t, publishableShard()), render: true})
	if status != exitOK {
		t.Fatalf("the fold exited %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "1 published") {
		t.Errorf("the fold said %q, want one row published", stdout)
	}
	page, err := os.ReadFile(filepath.Join(root, pageRelPath)) //#nosec G304 -- a path this test just built
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	if !strings.Contains(string(page), fixtureModel) {
		t.Error("the page was not redrawn from the record the fold just wrote")
	}
}

// TestRun_FoldingTheSameRunTwice_RefusesTheSecond is the collision rule end to
// end: a re-fold does not replace the figures already published, and says which
// row it would have replaced.
func TestRun_FoldingTheSameRunTwice_RefusesTheSecond(t *testing.T) {
	root := newRoot(t)
	dir := writeShard(t, publishableShard())
	if status, _, stderr := drive(t, root, options{shards: dir}); status != exitOK {
		t.Fatalf("the first fold exited %d: %s", status, stderr)
	}
	status, stdout, stderr := drive(t, root, options{shards: dir})
	if status != exitOK {
		t.Fatalf("the second fold exited %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "[duplicate-row]") || !strings.Contains(stdout, recordRelPath) {
		t.Errorf("the second fold said %q, want the row refused by name", stdout)
	}
}

// TestRun_ARefoldOfARunAlreadyPublished_ReplacesItsRowByName is the only path
// by which a corrected scoring rule reaches a row that is already published.
//
// The record holds scored columns, a redraw scores nothing, and a plain second
// fold is refused as a duplicate, so without this a corrected rule could reach
// a published row only by hand-editing the JSON. The re-fold drops what those
// shards publish, says which row it dropped, and folds them again. Here the
// second shard carries different token numbers, which stands in for a scoring
// change: the row is replaced rather than duplicated, and the figures are the
// second fold's.
func TestRun_ARefoldOfAWholeRun_ReplacesEveryCaseItMeasured(t *testing.T) {
	root := newRoot(t)
	if status, _, stderr := drive(t, root, options{shards: writeShard(t, publishableShard())}); status != exitOK {
		t.Fatalf("the first fold exited %d: %s", status, stderr)
	}

	rescored := publishableShard()
	rescored[2].Turn.Usage.Input = 4242
	status, stdout, stderr := drive(t, root, options{shards: writeShard(t, rescored), refold: true})
	if status != exitOK {
		t.Fatalf("the re-fold exited %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "merged into the published row") || !strings.Contains(stdout, fixtureModel) {
		t.Errorf("the re-fold said %q, want the row it merged into named", stdout)
	}
	if !strings.Contains(stdout, "1 case(s) replaced ("+fixtureCase+")") {
		t.Errorf("the re-fold said %q, want the case it replaced named", stdout)
	}
	if !strings.Contains(stdout, "1 published") {
		t.Errorf("the re-fold said %q, want the row folded in again", stdout)
	}

	doc, committed, err := readRecord(root)
	if err != nil || !committed {
		t.Fatalf("read the record back: %v", err)
	}
	if len(doc.Rows) != 1 {
		t.Fatalf("the record holds %d rows, want the one row replaced rather than added to", len(doc.Rows))
	}
	if doc.Rows[0].Tokens.Input != 4242 {
		t.Errorf("the record kept %d input tokens, want the re-folded run's own", doc.Rows[0].Tokens.Input)
	}
}

// TestRun_ARefold_TouchesOnlyTheRowsThoseShardsPublish is what keeps the
// re-fold from reaching past what it was given: a row those shards do not name
// is left exactly as it stood, whatever else the fold does.
//
// It is about a row under a *different* key. The row under the *same* key is
// the one a re-run of a single case produces, and what happens to that one is
// TestRun_ARefoldOfOneCase_KeepsTheCasesItDidNotMeasure: this test read as the
// guard against that case for a while and never was.
func TestRun_ARefold_TouchesOnlyTheRowsThoseShardsPublish(t *testing.T) {
	root := newRoot(t)
	other := publishOne(t, publishableShard())
	other.Key.Model = "openai:another-model"
	other.Key.ToolSchemaDigest = "tools-other"
	commit(t, root, []row{other})

	if status, _, stderr := drive(t, root, options{shards: writeShard(t, publishableShard()), refold: true}); status != exitOK {
		t.Fatalf("the re-fold exited %d: %s", status, stderr)
	}
	doc, _, err := readRecord(root)
	if err != nil {
		t.Fatalf("read the record back: %v", err)
	}
	if len(doc.Rows) != 2 {
		t.Fatalf("the record holds %d rows, want the untouched one kept beside the re-folded one", len(doc.Rows))
	}
	for _, one := range doc.Rows {
		if one.Key.Model == other.Key.Model && one.Key.ToolSchemaDigest != "tools-other" {
			t.Errorf("the row nothing re-folded came back as %+v", one.Key)
		}
	}
}

// TestRun_UsageErrors covers the four ways this command cannot be asked to do
// something: a gate that also writes, a re-fold with nothing to re-fold from,
// no flag at all, and a directory holding no shard.
func TestRun_UsageErrors(t *testing.T) {
	root := newRoot(t)

	status, _, stderr := drive(t, root, options{check: true, shards: t.TempDir()})
	if status != exitUsage {
		t.Errorf("-check with -shards exited %d, want a usage error", status)
	}
	if !strings.Contains(stderr, "writes nothing") {
		t.Errorf("the message %q does not say why", stderr)
	}

	status, _, stderr = drive(t, root, options{check: true, refold: true})
	if status != exitUsage || !strings.Contains(stderr, "writes nothing") {
		t.Errorf("-check with -refold exited %d saying %q, want a usage error", status, stderr)
	}

	status, _, stderr = drive(t, root, options{refold: true, render: true})
	if status != exitUsage {
		t.Errorf("-refold with no -shards exited %d, want a usage error", status)
	}
	if !strings.Contains(stderr, "-shards") {
		t.Errorf("the message %q does not say what is missing", stderr)
	}

	status, _, stderr = drive(t, root, options{})
	if status != exitUsage {
		t.Errorf("no flag at all exited %d, want a usage error", status)
	}
	if !strings.Contains(stderr, "nothing to do") {
		t.Errorf("the message %q does not say what to pass", stderr)
	}

	status, _, stderr = drive(t, root, options{shards: t.TempDir()})
	if status != exitUsage {
		t.Errorf("an empty shard directory exited %d, want a usage error", status)
	}
	if !strings.Contains(stderr, "no modeleval-") {
		t.Errorf("the message %q does not name what was missing", stderr)
	}
}

// TestRun_ARecordOfAnotherSchemaVersion_IsAFindingAndNotACrash keeps the gate
// answering when the record ahead of it was written by another version of this
// command.
func TestRun_ARecordOfAnotherSchemaVersion_IsAFindingAndNotACrash(t *testing.T) {
	root := newRoot(t)
	if err := os.WriteFile(filepath.Join(root, recordRelPath), []byte(`{"schema_version":99}`), 0o600); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	if status, _, stderr := drive(t, root, options{check: true}); status != exitFindings {
		t.Fatalf("exited %d (%s), want a finding", status, stderr)
	}
	if status, _, _ := drive(t, root, options{render: true}); status != exitUsage {
		t.Errorf("the render exited %d, want it to refuse a record it cannot read", status)
	}
}

// TestRun_APageWithoutItsMarkers_IsReportedRatherThanRewritten holds the one
// failure a generator must never paper over: a page whose markers have been
// removed cannot be written to, and guessing where the block went would put a
// table in the middle of somebody's prose.
func TestRun_APageWithoutItsMarkers_IsReportedRatherThanRewritten(t *testing.T) {
	root := newRoot(t)
	if err := os.WriteFile(filepath.Join(root, readmeRelPath), []byte("# No markers here\n"), 0o600); err != nil {
		t.Fatalf("write the page: %v", err)
	}
	status, _, stderr := drive(t, root, options{render: true})
	if status != exitUsage {
		t.Fatalf("exited %d, want the missing marker reported", status)
	}
	if !strings.Contains(stderr, "marker") {
		t.Errorf("the message %q does not name what was missing", stderr)
	}
}

// TestRun_APageThatCannotBeRead_IsReported covers the other end of the same
// read: a page that is not there at all is a tree this command cannot work in,
// and saying which file is the whole of the fix.
func TestRun_APageThatCannotBeRead_IsReported(t *testing.T) {
	root := newRoot(t)
	if err := os.Remove(filepath.Join(root, pageRelPath)); err != nil {
		t.Fatalf("remove the page: %v", err)
	}
	if status, _, stderr := drive(t, root, options{render: true}); status != exitUsage || !strings.Contains(stderr, pageRelPath) {
		t.Errorf("exited %d saying %q, want the missing page named", status, stderr)
	}
	if status, _, stderr := drive(t, root, options{check: true}); status != exitFindings || !strings.Contains(stderr, pageRelPath) {
		t.Errorf("the gate exited %d saying %q, want the missing page named", status, stderr)
	}
}

// TestRun_ACommittedRowTheRulesNowRefuse_IsAFinding is the half of the gate
// that is not a comparison of bytes: a row published when a rule did not exist,
// or before the corpus moved under it, is refused on review rather than left
// standing beside figures that no longer describe anything.
func TestRun_ACommittedRowTheRulesNowRefuse_IsAFinding(t *testing.T) {
	root := newRoot(t)
	moved := publishOne(t, publishableShard())
	moved.Key.CorpusDigest = "a corpus that has moved"
	moved.Provenance.CorpusDigest = moved.Key.CorpusDigest
	commit(t, root, []row{moved})
	if status, _, stderr := drive(t, root, options{render: true}); status != exitOK {
		t.Fatalf("the render exited %d: %s", status, stderr)
	}

	defer stubGit(t, func(string, ...string) (bool, error) { return false, nil })()
	status, _, stderr := drive(t, root, options{check: true})
	if status != exitFindings {
		t.Fatalf("a row the rules now refuse exited %d, want a finding", status)
	}
	if !strings.Contains(stderr, "stale-corpus") {
		t.Errorf("the finding %q does not name the rule", stderr)
	}
}

// TestRun_ARowMeasuredOffThisHistory_IsANoteAndNotAFinding is the other side of
// that: what a revision says is reported and never gated, because a shallow
// clone cannot answer the question at all.
func TestRun_ARowMeasuredOffThisHistory_IsANoteAndNotAFinding(t *testing.T) {
	root := newRoot(t)
	commit(t, root, []row{publishOne(t, publishableShard())})
	if status, _, stderr := drive(t, root, options{render: true}); status != exitOK {
		t.Fatalf("the render exited %d: %s", status, stderr)
	}

	defer stubGit(t, func(_ string, args ...string) (bool, error) { return args[0] == "cat-file", nil })()
	status, stdout, stderr := drive(t, root, options{check: true})
	if status != exitOK {
		t.Fatalf("a revision note exited %d (%s), want it reported and not gated", status, stderr)
	}
	if !strings.Contains(stdout, "not an ancestor of HEAD") {
		t.Errorf("the gate said %q, want the note", stdout)
	}
}

// TestRun_ShardsThatCannotBeScored_AreReportedRatherThanPublished covers the
// two ways a fold stops: a shard whose lines cannot be joined into attempts,
// and an attempt naming a case the corpus does not have.
func TestRun_ShardsThatCannotBeScored_AreReportedRatherThanPublished(t *testing.T) {
	root := newRoot(t)

	noRun := publishableShard()[1:]
	status, _, stderr := drive(t, root, options{shards: writeShard(t, noRun)})
	if status != exitUsage || !strings.Contains(stderr, "run line") {
		t.Errorf("a shard with no run line exited %d saying %q", status, stderr)
	}

	unknown := publishableShard()
	unknown[1].Attempt.Case = "MT-nobody-wrote-this"
	status, _, stderr = drive(t, root, options{shards: writeShard(t, unknown)})
	if status != exitUsage || !strings.Contains(stderr, "MT-nobody-wrote-this") {
		t.Errorf("an unknown case exited %d saying %q", status, stderr)
	}

	// A re-fold reads the same shards to learn which rows it replaces, so it
	// stops on the same shard for the same reason, before anything is dropped.
	status, _, stderr = drive(t, root, options{shards: writeShard(t, noRun), refold: true})
	if status != exitUsage || !strings.Contains(stderr, "run line") {
		t.Errorf("a re-fold of a shard with no run line exited %d saying %q", status, stderr)
	}
}

// TestRun_ARecordThatCannotBeRead_IsNeverMistakenForAnEmptyOne is the
// distinction the whole no-record-yet path rests on: a record that is not there
// is nothing published, and a record that cannot be read is a tree this command
// must refuse to write over.
func TestRun_ARecordThatCannotBeRead_IsNeverMistakenForAnEmptyOne(t *testing.T) {
	root := newRoot(t)
	if err := os.MkdirAll(filepath.Join(root, recordRelPath), 0o750); err != nil {
		t.Fatalf("make the record a directory: %v", err)
	}
	if status, _, stderr := drive(t, root, options{check: true}); status != exitFindings || !strings.Contains(stderr, recordRelPath) {
		t.Errorf("the gate exited %d saying %q, want the unreadable record named", status, stderr)
	}
	status, _, stderr := drive(t, root, options{shards: writeShard(t, publishableShard())})
	if status != exitUsage || !strings.Contains(stderr, recordRelPath) {
		t.Errorf("a fold over an unreadable record exited %d saying %q", status, stderr)
	}
}

// stubGit replaces the revision probe and hands back its restorer, so the three
// answers below are each reachable from a test, which none of them is from a
// checkout in one state.
func stubGit(t *testing.T, answer func(dir string, args ...string) (bool, error)) func() {
	t.Helper()
	previous := gitProbe
	gitProbe = answer
	return func() { gitProbe = previous }
}

// TestRevisionNotes_OnlySpeakAboutARevisionGitKnowsAndCannotReach is the note
// that reports and never fails. A shallow CI clone knows no revision but HEAD,
// so a note that fired whenever git could not answer would fire on every push
// and inform nobody.
func TestRevisionNotes_OnlySpeakAboutARevisionGitKnowsAndCannotReach(t *testing.T) {
	one := publishOne(t, publishableShard())
	doc := document{Rows: []row{one, one}}

	cases := []struct {
		name   string
		answer func(string, ...string) (bool, error)
		want   int
	}{
		{"git cannot be run", func(string, ...string) (bool, error) { return false, errors.New("no git here") }, 0},
		{"the revision is unknown", func(string, ...string) (bool, error) { return false, nil }, 0},
		{"the revision is an ancestor", func(string, ...string) (bool, error) { return true, nil }, 0},
		{"known and not an ancestor", func(_ string, args ...string) (bool, error) {
			return args[0] == "cat-file", nil
		}, 1},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			defer stubGit(t, one.answer)()
			if got := revisionNotes(t.TempDir(), doc); len(got) != one.want {
				t.Errorf("got %d note(s) %v, want %d", len(got), got, one.want)
			}
		})
	}
}

// TestRevisionNotes_ARevisionThatIsNotOne_IsNeverHandedToGit is what keeps a
// value read out of a committed file from reaching a process as an argument.
func TestRevisionNotes_ARevisionThatIsNotOne_IsNeverHandedToGit(t *testing.T) {
	one := publishOne(t, publishableShard())
	one.Provenance.Commit = "--upload-pack=touch /tmp/pwned"

	asked := false
	defer stubGit(t, func(string, ...string) (bool, error) {
		asked = true
		return true, nil
	})()
	if got := revisionNotes(t.TempDir(), document{Rows: []row{one}}); len(got) != 0 {
		t.Errorf("got %v, want nothing said about a revision that is not one", got)
	}
	if asked {
		t.Error("a value that is not a revision was handed to git")
	}
}

// TestAskGit_SeparatesAnAnswerOfNoFromGitNotBeingThere is the distinction both
// notes above rest on. It is driven against a stand-in on the path rather than
// against the real git, so the three outcomes are the test's own to decide and
// a machine without git still runs them.
func TestAskGit_SeparatesAnAnswerOfNoFromGitNotBeingThere(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("the stand-in is a shell script, which Windows does not run as a program")
	}
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"git answers yes", 0, true},
		{"git answers no", 1, false},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Setenv("PATH", standInGit(t, one.status))
			answer, err := askGit(t.TempDir(), "cat-file", "-e", fixtureCommit+"^{commit}")
			if err != nil {
				t.Fatalf("an exit status was an error rather than an answer: %v", err)
			}
			if answer != one.want {
				t.Errorf("answer = %v, want %v", answer, one.want)
			}
		})
	}

	t.Run("git is not there at all", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if _, err := askGit(t.TempDir(), "cat-file", "-e", fixtureCommit); err == nil {
			t.Error("git being absent was reported as an answer rather than as a silence")
		}
	})

	// The third way this can go, and the one a lookup cannot rule out: a file
	// named git that resolves and then cannot be run. It must read as the
	// silence an absent git is and never as an answer of no, which is what
	// decides whether a note is withheld or a row is called off this history.
	t.Run("git resolves and cannot be run", func(t *testing.T) {
		dir := t.TempDir()
		//#nosec G302,G306 -- a file this test puts on its own PATH, which has to carry the executable bit to be resolved at all
		if err := os.WriteFile(filepath.Join(dir, "git"), []byte("not a program\n"), 0o700); err != nil {
			t.Fatalf("write the stand-in: %v", err)
		}
		t.Setenv("PATH", dir)
		if _, err := askGit(t.TempDir(), "cat-file", "-e", fixtureCommit); err == nil {
			t.Error("a git that cannot be executed was reported as an answer rather than as a silence")
		}
	})
}

// standInGit writes a program named git that exits with the given status, and
// returns the directory to put on the path.
func standInGit(t *testing.T, status int) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nexit " + strconv.Itoa(status) + "\n"
	//#nosec G302,G306 -- a program this test runs, which has to be executable
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write the stand-in: %v", err)
	}
	return dir
}

// runtimeIsWindows says whether the stand-in above can be run at all.
func runtimeIsWindows() bool { return runtime.GOOS == "windows" }

// TestRun_ARefoldOfOneCase_KeepsTheCasesItDidNotMeasure is the case
// MODELEVAL_CASES exists for, and the one the re-fold got wrong.
//
// A row is identified by its key, and the set of cases behind it is in no part
// of that key. So a re-run of one corrected case produces shards whose key is
// the row's, and the two paths a maintainer has both fail: a plain fold is
// refused as a duplicate row, and a re-fold drops the row and replaces it with
// one covering a single case. The second is the dangerous one, because it is
// what the documentation tells you to reach for, it reports success, and what
// it discards is the rest of a paid run.
//
// TestRun_ARefold_DropsOnlyTheRowsThoseShardsPublish reads as the guard against
// exactly this and is not: it proves a row under a *different* key survives.
// The loss happens under the same key, which is the only shape a re-run of one
// case can have.
func TestRun_ARefoldOfOneCase_KeepsTheCasesItDidNotMeasure(t *testing.T) {
	root := newRoot(t)
	if status, _, stderr := drive(t, root, options{shards: writeShard(t, twoCaseShard())}); status != exitOK {
		t.Fatalf("the first fold exited %d: %s", status, stderr)
	}

	doc, _, err := readRecord(root)
	if err != nil {
		t.Fatalf("read the record back: %v", err)
	}
	if len(doc.Rows) != 1 || doc.Rows[0].Counts.Attempts != 2 {
		t.Fatalf("the first fold published %d row(s) of %d attempt(s), want one row of two",
			len(doc.Rows), doc.Rows[0].Counts.Attempts)
	}

	// One case is re-run because it was corrected, which is what
	// MODELEVAL_CASES=MT-003 leaves in the shard directory.
	status, stdout, stderr := drive(t, root, options{shards: writeShard(t, oneCaseRerunShard()), refold: true})
	if status != exitOK {
		t.Fatalf("the re-fold of one case exited %d: %s\n%s", status, stderr, stdout)
	}

	doc, _, err = readRecord(root)
	if err != nil {
		t.Fatalf("read the record back: %v", err)
	}
	if len(doc.Rows) != 1 {
		t.Fatalf("the record holds %d rows, want the one row updated in place", len(doc.Rows))
	}
	if got := doc.Rows[0].Counts.Attempts; got != 2 {
		t.Errorf("the row now counts %d attempt(s), want 2: re-running one case discarded the measurement of every case it did not re-run", got)
	}
}

// TestRun_AMergedRow_SaysWhichRunMeasuredEachCase is the honesty half of the
// merge.
//
// The moment a row can be assembled from two runs, its provenance block stops
// being one run's: the commit and date at the top are the last contributor's,
// and a case measured three weeks earlier sits in the same figures. Saying
// nothing about that would be the same fold this record was rebuilt to stop, so
// every case carries the run, the day and the tree that measured it, and a
// reader asking when a case was last put to the model can answer it.
func TestRun_AMergedRow_SaysWhichRunMeasuredEachCase(t *testing.T) {
	root := newRoot(t)
	if status, _, stderr := drive(t, root, options{shards: writeShard(t, twoCaseShard())}); status != exitOK {
		t.Fatalf("the first fold exited %d: %s", status, stderr)
	}

	// The re-run of one case happened later, on a different tree.
	rerun := oneCaseRerunShard()
	rerun[0].Run.RunID = "run-rerun"
	rerun[0].Run.Commit = "2222222222222222222222222222222222222222"
	rerun[0].Run.StartedAt = rerun[0].Run.StartedAt.AddDate(0, 0, 21)

	if status, _, stderr := drive(t, root, options{shards: writeShard(t, rerun), refold: true}); status != exitOK {
		t.Fatalf("the re-fold exited %d: %s", status, stderr)
	}

	doc, _, err := readRecord(root)
	if err != nil {
		t.Fatalf("read the record back: %v", err)
	}
	if len(doc.Rows) != 1 {
		t.Fatalf("the record holds %d rows, want one", len(doc.Rows))
	}
	cases := doc.Rows[0].Cases
	if len(cases) != 2 {
		t.Fatalf("the merged row holds %d case(s), want both", len(cases))
	}

	held, reran := cases[fixtureCase], cases["MT-003"]
	if held.Run != "run-fixture" {
		t.Errorf("%s says it was measured by %q, want the run that is still behind it", fixtureCase, held.Run)
	}
	if reran.Run != "run-rerun" {
		t.Errorf("MT-003 says it was measured by %q, want the re-run", reran.Run)
	}
	if held.Commit == reran.Commit {
		t.Errorf("both cases name the tree %q, so the row cannot say one of them was re-measured", held.Commit)
	}
	if held.Date == reran.Date {
		t.Errorf("both cases name the day %q, so the row cannot say when each was last put to the model", held.Date)
	}
}
