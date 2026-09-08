package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apiexposes"
)

// fixedClock is the day a generated record is asserted against.
func fixedClock() time.Time { return time.Date(2026, 9, 8, 11, 0, 0, 0, time.UTC) }

// commitA and commitB are the commits the fixture archives name.
const (
	commitA = "0123456789abcdef0123456789abcdef01234567"
	commitB = "89abcdef0123456789abcdef0123456789abcdef"
)

// refusedDial is a transport nothing can answer through.
type refusedDial struct{}

func (refusedDial) RoundTrip(request *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("dial tcp %s: connect: connection refused", request.URL.Host)
}

// brokenBody is a transport whose answer cannot be read to the end.
type brokenBody struct{}

func (brokenBody) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(failingReader{})}, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// runCommand runs the command with both streams captured.
func runCommand(t *testing.T, cfg genRun) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(cfg, &out, &errOut)
	return status, out.String(), errOut.String()
}

// entitySource writes n entities in one file, enough to clear the floor a
// whole tree has to reach when n is large, and one field each so the file
// is one the reader accepts.
func entitySource(n int) string {
	var b strings.Builder
	b.WriteString("module API\n  module Entities\n")
	for i := range n {
		fmt.Fprintf(&b, "    class E%d < Grape::Entity\n      expose :id\n    end\n", i)
	}
	b.WriteString("  end\nend\n")
	return b.String()
}

// featureTable writes a table of n premium symbols.
func featureTable(n int) string {
	var b strings.Builder
	b.WriteString("module GitlabSubscriptions\n  class Features\n    PREMIUM_FEATURES = %i[\n")
	for i := range n {
		fmt.Fprintf(&b, "      feature_%d\n", i)
	}
	b.WriteString("    ].freeze\n  end\nend\n")
	return b.String()
}

// tarball builds a subtree archive the way GitLab serves one: the pax global
// header git archive opens with, a root directory naming the commit, the
// files below it, plus a directory entry and a file that is not Ruby, which
// the reader passes over. An empty root is an archive with no entries at all.
func tarball(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	if root != "" {
		global := &tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": commitA}}
		if err := archive.WriteHeader(global); err != nil {
			t.Fatalf("write the archive: %v", err)
		}
		if err := archive.WriteHeader(&tar.Header{Name: root + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
			t.Fatalf("write the archive: %v", err)
		}
		files["README.md"] = "not ruby"
	}
	for path, content := range files {
		header := &tar.Header{Name: root + "/" + path, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatalf("write the archive: %v", err)
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			t.Fatalf("write the archive: %v", err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close the archive: %v", err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatalf("close the archive: %v", err)
	}
	return buf.Bytes()
}

// shortArchive builds an archive whose one file announces more bytes than
// follow, so its header reads and its content does not.
func shortArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	root := "gitlab-master-" + commitA + "-lib-api-entities"
	if err := archive.WriteHeader(&tar.Header{Name: root + "/lib/api/entities/cut.rb", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4096}); err != nil {
		t.Fatalf("write the archive: %v", err)
	}
	if _, err := archive.Write([]byte("module API\n")); err != nil {
		t.Fatalf("write the archive: %v", err)
	}
	// Close would refuse the short write, and what the buffer already holds
	// is the shape wanted: a header and less content than it promises.
	if err := zipped.Close(); err != nil {
		t.Fatalf("close the archive: %v", err)
	}
	return buf.Bytes()
}

// gitlabAnswers maps each fetched path to its body: the three archives by
// the subtree they were asked for, the table by its raw path.
type gitlabAnswers struct {
	archives map[string][]byte
	table    []byte
	status   int
}

// serving returns a server answering the way gitlab.com does for the four
// fetches the generator makes.
func serving(t *testing.T, answers gitlabAnswers) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if answers.status != 0 {
			w.WriteHeader(answers.status)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/archive.tar.gz") {
			body, ok := answers.archives[r.URL.Query().Get("path")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(body)
			return
		}
		if strings.HasSuffix(r.URL.Path, featuresPath) && answers.table != nil {
			_, _ = w.Write(answers.table)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server
}

// wholeTree is a set of answers that clears every floor: one archive with
// enough entities, two small ones, and a table with enough features.
func wholeTree(t *testing.T) gitlabAnswers {
	t.Helper()
	return gitlabAnswers{
		archives: map[string][]byte{
			"lib/api/entities":       tarball(t, "gitlab-master-"+commitA+"-lib-api-entities", map[string]string{"lib/api/entities/all.rb": entitySource(apiexposes.MinimumEntities + 1)}),
			"ee/lib/api/entities":    tarball(t, "gitlab-master-"+commitA+"-ee-lib-api-entities", map[string]string{"ee/lib/api/entities/epic.rb": "module API\n  module Entities\n    class Epic < Grape::Entity\n      expose :id, if: ->(e, _) { e.feature_available?(:feature_1) }\n    end\n  end\nend\n"}),
			"ee/lib/ee/api/entities": tarball(t, "gitlab-master-"+commitA+"-ee-lib-ee-api-entities", map[string]string{"ee/lib/ee/api/entities/e0.rb": "module EE\n  module API\n    module Entities\n      module E0\n        prepended do\n          expose :licensed\n        end\n      end\n    end\n  end\nend\n"}),
		},
		table: []byte(featureTable(apiexposes.MinimumFeatures + 1)),
	}
}

// generating runs a generate against a server, into a fresh directory.
func generating(t *testing.T, answers gitlabAnswers, client *http.Client) (int, string, string, string) {
	t.Helper()
	server := serving(t, answers)
	dir := t.TempDir()
	if client == nil {
		client = server.Client()
	}
	status, out, errOut := runCommand(t, genRun{ref: "master", dir: dir, client: client, base: server.URL, now: fixedClock})
	return status, out, errOut, dir
}

// TestRun_Generate_FetchesEverySourceAndWritesTheRecord verifies the network
// half end to end: the three subtrees and the table are fetched at one ref,
// the commit the archives name is recorded with the day and a digest, the
// prepend lands on its entity with its edition, the licensed condition is
// resolved to its tier, and the summary says what was written.
func TestRun_Generate_FetchesEverySourceAndWritesTheRecord(t *testing.T) {
	status, out, errOut, dir := generating(t, wholeTree(t), nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s", status, errOut)
	}
	doc, err := apiexposes.Read(dir)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if doc.Source.Ref != "master" || doc.Source.Commit != commitA || doc.Source.RetrievedAt != "2026-09-08" || len(doc.Source.SHA256) != 64 {
		t.Errorf("Source = %+v, want the ref, the archives' commit, the fixed day and a digest", doc.Source)
	}
	if doc.Source.Files != 3 || doc.Source.Entities != apiexposes.MinimumEntities+2 || doc.Source.Features != apiexposes.MinimumFeatures+1 {
		t.Errorf("Source counts = %+v", doc.Source)
	}
	e0 := doc.Entities["APIEntitiesE0"]
	if len(e0.Fields) != 2 || e0.Fields[1].Name != "licensed" || e0.Fields[1].Edition != "ee" {
		t.Errorf("E0 = %+v, want the prepended field after its own", e0)
	}
	if epic := doc.Entities["APIEntitiesEpic"]; epic.Edition != "ee" || epic.Fields[0].Tier != apiexposes.TierPremium {
		t.Errorf("Epic = %+v, want an Enterprise entity with a premium field", epic)
	}
	if !strings.Contains(out, "wrote gitlab-api-exposes.json; "+strconv.Itoa(apiexposes.MinimumEntities+2)+" entities and "+strconv.Itoa(apiexposes.MinimumFeatures+1)+" licensed features from gitlab-org/gitlab at master (01234567), retrieved 2026-09-08") {
		t.Errorf("run() stdout = %q, want the summary", out)
	}
}

// TestRun_Generate_RefusesWhatItCannotTrust verifies every way the network
// half stops before writing: archives that resolved the ref to different
// commits, an archive that is not one, an archive naming no commit, an empty
// one, an answer that is not 200, a connection that fails, a body that cannot
// be read, a source the reader refuses, a tree too small to be GitLab's, and a
// record that cannot be written.
func TestRun_Generate_RefusesWhatItCannotTrust(t *testing.T) {
	mismatched := wholeTree(t)
	mismatched.archives["ee/lib/api/entities"] = tarball(t, "gitlab-master-"+commitB+"-ee-lib-api-entities", map[string]string{"ee/lib/api/entities/e.rb": entitySource(1)})
	notGzip := wholeTree(t)
	notGzip.archives["lib/api/entities"] = []byte("not an archive")
	noCommit := wholeTree(t)
	noCommit.archives["lib/api/entities"] = tarball(t, "gitlab-master-lib-api-entities", map[string]string{"lib/api/entities/all.rb": entitySource(1)})
	empty := wholeTree(t)
	empty.archives["lib/api/entities"] = tarball(t, "", map[string]string{})
	truncated := wholeTree(t)
	truncated.archives["lib/api/entities"] = truncated.archives["lib/api/entities"][:len(truncated.archives["lib/api/entities"])-40]
	short := wholeTree(t)
	short.archives["lib/api/entities"] = shortArchive(t)
	unreadable := wholeTree(t)
	unreadable.archives["lib/api/entities"] = tarball(t, "gitlab-master-"+commitA+"-lib-api-entities", map[string]string{"lib/api/entities/bad.rb": "module API\n  module Entities\nend\nend\nend\n"})
	small := wholeTree(t)
	small.archives["lib/api/entities"] = tarball(t, "gitlab-master-"+commitA+"-lib-api-entities", map[string]string{"lib/api/entities/few.rb": entitySource(3)})
	noTable := wholeTree(t)
	noTable.table = nil

	cases := []struct {
		name    string
		answers gitlabAnswers
		client  *http.Client
		want    string
	}{
		{name: "archives naming different commits", answers: mismatched, want: "ee/lib/api/entities resolved master to " + commitB + " where an earlier archive resolved it to " + commitA + ": the ref moved between downloads, run again"},
		{name: "an archive that is not gzip", answers: notGzip, want: "lib/api/entities: the archive is not gzip"},
		{name: "an archive naming no commit", answers: noCommit, want: `names no commit`},
		{name: "an empty archive", answers: empty, want: "the archive is empty"},
		{name: "an archive cut short", answers: truncated, want: "read the archive"},
		{name: "a file cut short inside the archive", answers: short, want: "read lib/api/entities/cut.rb: unexpected EOF"},
		{name: "an answer that is not 200", answers: gitlabAnswers{status: http.StatusInternalServerError}, want: "500 Internal Server Error"},
		{name: "a connection that fails", answers: wholeTree(t), client: &http.Client{Transport: refusedDial{}}, want: "connection refused"},
		{name: "a body that cannot be read", answers: wholeTree(t), client: &http.Client{Transport: brokenBody{}}, want: "connection reset"},
		{name: "a source the reader refuses", answers: unreadable, want: "bad.rb:5: end with nothing open"},
		{name: "a tree too small to be GitLab's", answers: small, want: "refusing to replace the record with a truncated read"},
		{name: "a table that is not there", answers: noTable, want: "features.rb: 404 Not Found"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, _, errOut, dir := generating(t, testCase.answers, testCase.client)

			if status != 1 {
				t.Errorf("run() = %d, want 1", status)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("run() stderr = %q, want it to say %q", errOut, testCase.want)
			}
			if _, err := os.Stat(filepath.Join(dir, apiexposes.FileName)); err == nil {
				t.Error("a record was written on a refused run")
			}
		})
	}
}

// TestRun_Generate_RequestItCannotBuildOrRecordItCannotWrite_IsReported
// verifies the two failures around the fetch itself: a base URL that makes
// no request, and a record directory that is a file.
func TestRun_Generate_RequestItCannotBuildOrRecordItCannotWrite_IsReported(t *testing.T) {
	t.Run("a base URL no request can be built for", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{ref: "master", dir: t.TempDir(), client: http.DefaultClient, base: "http://[bad", now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "build the request for") {
			t.Errorf("run() = %d, stderr %q, want the request failure", status, errOut)
		}
	})
	t.Run("a record that cannot be written", func(t *testing.T) {
		server := serving(t, wholeTree(t))
		occupied := t.TempDir()
		if err := os.Mkdir(filepath.Join(occupied, apiexposes.FileName), 0o750); err != nil {
			t.Fatalf("prepare: %v", err)
		}

		status, _, errOut := runCommand(t, genRun{ref: "master", dir: occupied, client: server.Client(), base: server.URL, now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "write ") {
			t.Errorf("run() = %d, stderr %q, want the write failure", status, errOut)
		}
	})
}

// checkout writes a local checkout holding the three directories and the
// table.
func checkout(t *testing.T, withTable bool) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range entityDirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			t.Fatalf("prepare: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "lib", "api", "entities", "all.rb"), []byte(entitySource(apiexposes.MinimumEntities+1)), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib", "api", "entities", "notes.txt"), []byte("not ruby"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if withTable {
		if err := os.MkdirAll(filepath.Join(root, "ee", "app", "models", "gitlab_subscriptions"), 0o750); err != nil {
			t.Fatalf("prepare: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(featuresPath)), []byte(featureTable(apiexposes.MinimumFeatures+1)), 0o600); err != nil {
			t.Fatalf("prepare: %v", err)
		}
	}
	return root
}

// TestRun_Source_ReadsALocalCheckoutWithoutTheNetwork verifies the -source
// half: the same directories read off disk, recorded as a local checkout
// rather than a commit, with the failures a checkout can have.
func TestRun_Source_ReadsALocalCheckoutWithoutTheNetwork(t *testing.T) {
	t.Run("a whole checkout", func(t *testing.T) {
		dir := t.TempDir()
		status, out, errOut := runCommand(t, genRun{ref: "master", dir: dir, source: checkout(t, true), client: &http.Client{Transport: refusedDial{}}, now: fixedClock})

		if status != 0 {
			t.Fatalf("run() = %d, want 0; stderr:\n%s", status, errOut)
		}
		doc, err := apiexposes.Read(dir)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if doc.Source.Commit != "local checkout" || doc.Source.SHA256 != "" || doc.Source.Files != 1 {
			t.Errorf("Source = %+v, want a local checkout with one Ruby file", doc.Source)
		}
		if !strings.Contains(out, "wrote gitlab-api-exposes.json") {
			t.Errorf("run() stdout = %q", out)
		}
	})
	t.Run("a checkout without the table", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{ref: "master", dir: t.TempDir(), source: checkout(t, false), now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "read the feature table") {
			t.Errorf("run() = %d, stderr %q, want the table failure", status, errOut)
		}
	})
	t.Run("a checkout missing a directory", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{ref: "master", dir: t.TempDir(), source: t.TempDir(), now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "read lib/api/entities") {
			t.Errorf("run() = %d, stderr %q, want the directory failure", status, errOut)
		}
	})
	t.Run("a checkout that is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("prepare: %v", err)
		}

		status, _, errOut := runCommand(t, genRun{ref: "master", dir: t.TempDir(), source: file, now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "read the checkout") {
			t.Errorf("run() = %d, stderr %q, want the checkout failure", status, errOut)
		}
	})
	t.Run("a Ruby file that cannot be read", func(t *testing.T) {
		root := checkout(t, true)
		if err := os.Symlink(filepath.Join(root, "nowhere.rb"), filepath.Join(root, "lib", "api", "entities", "dangling.rb")); err != nil {
			t.Skipf("this platform will not make a symlink here: %v", err)
		}

		status, _, errOut := runCommand(t, genRun{ref: "master", dir: t.TempDir(), source: root, now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "read lib/api/entities: ") {
			t.Errorf("run() = %d, stderr %q, want the file failure", status, errOut)
		}
	})
}

// usableRecord is a committed record every check accepts on the fixed day.
func usableRecord() apiexposes.Document {
	entities := make(map[string]apiexposes.Entity, apiexposes.MinimumEntities+1)
	for i := range apiexposes.MinimumEntities + 1 {
		entities[fmt.Sprintf("APIEntitiesE%d", i)] = apiexposes.Entity{File: "lib/api/entities/all.rb", Line: i + 3, Fields: []apiexposes.Field{{Name: "id", Line: i + 4}}}
	}
	features := make(map[string]string, apiexposes.MinimumFeatures+1)
	for i := range apiexposes.MinimumFeatures + 1 {
		features[fmt.Sprintf("feature_%d", i)] = apiexposes.TierPremium
	}
	return apiexposes.Document{
		Source:   apiexposes.Source{Ref: "master", Commit: commitA, RetrievedAt: "2026-09-01", SHA256: "abc", Files: 1, Entities: len(entities), Features: len(features)},
		Entities: entities,
		Features: features,
	}
}

// committing writes a record where -check and -report read it.
func committing(t *testing.T, doc apiexposes.Document) string {
	t.Helper()
	dir := t.TempDir()
	if err := apiexposes.Write(dir, doc); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	return dir
}

// TestRun_Check_JudgesTheCommittedRecord verifies the CI half: a usable record
// passes and names its provenance, and each way a record fails to be usable is
// named, with the instruction to refresh it.
func TestRun_Check_JudgesTheCommittedRecord(t *testing.T) {
	t.Run("a usable record", func(t *testing.T) {
		status, out, errOut := runCommand(t, genRun{dir: committing(t, usableRecord()), check: true, now: fixedClock})

		if status != 0 {
			t.Fatalf("run() = %d, want 0; stderr:\n%s", status, errOut)
		}
		if !strings.Contains(out, "the record is usable; 401 entities and 201 licensed features from gitlab-org/gitlab at master (01234567), retrieved 2026-09-01") {
			t.Errorf("run() stdout = %q", out)
		}
	})

	tooFewEntities := usableRecord()
	tooFewEntities.Entities = map[string]apiexposes.Entity{"APIEntitiesE0": tooFewEntities.Entities["APIEntitiesE0"]}
	tooFewFeatures := usableRecord()
	tooFewFeatures.Features = map[string]string{"one": apiexposes.TierPremium}
	unprovenanced := usableRecord()
	unprovenanced.Source.Commit = ""
	notADate := usableRecord()
	notADate.Source.RetrievedAt = "yesterday"
	future := usableRecord()
	future.Source.RetrievedAt = "2030-01-01"
	old := usableRecord()
	old.Source.RetrievedAt = "2025-01-01"

	cases := []struct {
		name string
		doc  apiexposes.Document
		want string
	}{
		{name: "too few entities", doc: tooFewEntities, want: "the record carries 1 entities and GitLab declares more than 400"},
		{name: "too few features", doc: tooFewFeatures, want: "the record carries 1 licensed features and GitLab's table lists more than 200"},
		{name: "no provenance", doc: unprovenanced, want: "does not say which ref and commit it came from"},
		{name: "a retrieval day that is not a date", doc: notADate, want: `was taken on "yesterday", which is not a date`},
		{name: "a retrieval day in the future", doc: future, want: "was taken on 2030-01-01, which has not happened yet"},
		{name: "a record older than the window", doc: old, want: "the record is 615 days old and the window is 180"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, _, errOut := runCommand(t, genRun{dir: committing(t, testCase.doc), check: true, now: fixedClock})

			if status != 1 {
				t.Errorf("run() = %d, want 1", status)
			}
			if !strings.Contains(errOut, testCase.want) || !strings.Contains(errOut, "refresh it with `make gen-api-exposes`") {
				t.Errorf("run() stderr = %q, want it to say %q and how to refresh", errOut, testCase.want)
			}
		})
	}

	t.Run("no record", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{dir: t.TempDir(), check: true, now: fixedClock})

		if status != 1 || !strings.Contains(errOut, "read ") {
			t.Errorf("run() = %d, stderr %q, want the read failure", status, errOut)
		}
	})
	t.Run("a usable record judged by the wall clock", func(t *testing.T) {
		fresh := usableRecord()
		fresh.Source.RetrievedAt = time.Now().UTC().Format(time.DateOnly)

		status, out, errOut := runCommand(t, genRun{dir: committing(t, fresh), check: true})

		if status != 0 || !strings.Contains(out, "the record is usable") {
			t.Errorf("run() = %d, stdout %q, stderr %q", status, out, errOut)
		}
	})
}

// TestRun_Report_PrintsAnEntityAsAReviewerReadsIt verifies the reading half:
// the parent's fields first, every note a field carries in one bracket,
// nested fields indented under theirs, the entity accepted in its Ruby
// spelling, and the two ways a report cannot be made.
func TestRun_Report_PrintsAnEntityAsAReviewerReadsIt(t *testing.T) {
	doc := usableRecord()
	doc.Entities["APIEntitiesParent"] = apiexposes.Entity{File: "lib/api/entities/parent.rb", Line: 3, Fields: []apiexposes.Field{{Name: "id", Line: 4}}}
	doc.Entities["APIEntitiesChild"] = apiexposes.Entity{File: "lib/api/entities/child.rb", Line: 3, Parent: "APIEntitiesParent", Edition: "ee", Fields: []apiexposes.Field{
		{Name: "gated", Line: 4, If: "->(c, _) { c.feature_available?(:feature_1) }", Features: []string{"feature_1"}, Tier: apiexposes.TierPremium},
		{Name: "prepended", Line: 9, Edition: "ee", Unless: "->(_, _) { off? }"},
		{Name: "links", Line: 5, Using: "APIEntitiesParent", Nested: []apiexposes.Field{{Name: "self", Line: 6}}},
	}}
	dir := committing(t, doc)

	t.Run("an entity in its Ruby spelling", func(t *testing.T) {
		status, out, errOut := runCommand(t, genRun{dir: dir, report: "API::Entities::Child"})

		if status != 0 {
			t.Fatalf("run() = %d, want 0; stderr:\n%s", status, errOut)
		}
		want := "APIEntitiesChild (lib/api/entities/child.rb:3, inherits APIEntitiesParent, ee only): 4 field(s)\n" +
			"  id\n" +
			"  gated  [premium; if ->(c, _) { c.feature_available?(:feature_1) }]\n" +
			"  prepended  [ee; unless ->(_, _) { off? }]\n" +
			"  links  [as APIEntitiesParent]\n" +
			"    self\n"
		if out != want {
			t.Errorf("run() stdout =\n%s\nwant\n%s", out, want)
		}
	})
	t.Run("an entity the record does not hold", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{dir: dir, report: "APIEntitiesNope"})

		if status != 1 || !strings.Contains(errOut, "APIEntitiesNope is not an entity the record holds (403 entities; names look like APIEntitiesProject)") {
			t.Errorf("run() = %d, stderr %q", status, errOut)
		}
	})
	t.Run("no record to report from", func(t *testing.T) {
		status, _, errOut := runCommand(t, genRun{dir: t.TempDir(), report: "APIEntitiesChild"})

		if status != 1 || !strings.Contains(errOut, "read ") {
			t.Errorf("run() = %d, stderr %q, want the read failure", status, errOut)
		}
	})
}
