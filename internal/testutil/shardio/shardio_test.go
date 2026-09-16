package shardio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture record every test here drives the mechanism with.
//
// It is a record of its own rather than either real one, because what these
// tests are about is the mechanism and nothing else: a record package's own
// tests should fail for something that package decided, and a fixture is what
// keeps this suite from failing when one of them adds a line type. It carries
// the three shapes the mechanism has to tell apart: a line that cannot fail, a
// line whose float can be NaN and so cannot be encoded, and a line holding raw
// JSON somebody else wrote.
const (
	fixtureSchema  = 1
	fixtureDirEnv  = "GITLAB_MCP_TEST_SHARDIO_FIXTURE_DIR"
	fixturePrefix  = "fixture-"
	fixtureExt     = ".jsonl"
	fixtureCapHint = "; cap what it carries"
	fixtureRawHint = "; the text belongs in a raw field"

	typeNote    = "note"
	typeMeasure = "measure"
	typeCarrier = "carrier"
)

// fixtureRecord is the envelope the fixture lines are written in.
type fixtureRecord struct {
	Schema  int      `json:"schema"`
	Type    string   `json:"type"`
	Note    *note    `json:"note,omitempty"`
	Measure *measure `json:"measure,omitempty"`
	Carrier *carrier `json:"carrier,omitempty"`
}

// fixtureLine is one thing a fixture shard can hold.
type fixtureLine interface {
	record() fixtureRecord
}

// note is a line nothing about it can make unwritable.
type note struct {
	Text string `json:"text"`
}

// record wraps the note in its envelope.
func (n *note) record() fixtureRecord {
	return fixtureRecord{Schema: fixtureSchema, Type: typeNote, Note: n}
}

// measure carries a float, which is the one ordinary Go value encoding/json
// refuses: NaN and the infinities have no JSON spelling.
type measure struct {
	Value float64 `json:"value"`
}

// record wraps the measure in its envelope.
func (m *measure) record() fixtureRecord {
	return fixtureRecord{Schema: fixtureSchema, Type: typeMeasure, Measure: m}
}

// carrier holds JSON this package did not build, which is what [Spec.Check]
// exists for.
type carrier struct {
	Raw json.RawMessage `json:"raw,omitempty"`
}

// record wraps the carrier in its envelope.
func (c *carrier) record() fixtureRecord {
	return fixtureRecord{Schema: fixtureSchema, Type: typeCarrier, Carrier: c}
}

// fixturePayloads answers, per line type, whether the record carries the
// payload that type names.
var fixturePayloads = map[string]func(fixtureRecord) bool{
	typeNote:    func(r fixtureRecord) bool { return r.Note != nil },
	typeMeasure: func(r fixtureRecord) bool { return r.Measure != nil },
	typeCarrier: func(r fixtureRecord) bool { return r.Carrier != nil },
}

// validateFixture is the fixture record's own reader-side check.
func validateFixture(r fixtureRecord) error {
	return ValidateEnvelope(r, r.Schema, fixtureSchema, r.Type, fixturePayloads)
}

// plainSpec is the shape of a record carrying nothing the writer must inspect
// before encoding: no Check and no hints, which is the arm every branch those
// two fields open has to be taken with as well.
func plainSpec() Spec[fixtureRecord, fixtureLine] {
	return Spec[fixtureRecord, fixtureLine]{
		DirEnv:   fixtureDirEnv,
		Prefix:   fixturePrefix,
		Ext:      fixtureExt,
		Noun:     "a fixture",
		TypeOf:   func(r fixtureRecord) string { return r.Type },
		Envelope: func(l fixtureLine) fixtureRecord { return l.record() },
		Validate: validateFixture,
	}
}

// checkedSpec is the shape of a record filled from somebody else's output: a
// pre-encode check on its raw field, and a hint on each refusal that is about
// one line.
func checkedSpec() Spec[fixtureRecord, fixtureLine] {
	spec := plainSpec()
	spec.Check = func(r fixtureRecord) string {
		if r.Carrier != nil && len(r.Carrier.Raw) != 0 && !json.Valid(r.Carrier.Raw) {
			return "the carried value"
		}
		return ""
	}
	spec.CheckHint = fixtureRawHint
	spec.CapHint = fixtureCapHint
	return spec
}

// newFixture returns a mechanism over spec, released when the test ends.
//
// Each test gets a registry of its own, so a stubbed shard constructor and a
// writer left open belong to that test and to nothing else. The temporary
// directory is made by the caller before this is called, so its removal is
// registered first and therefore runs after the release: a shard left open is a
// directory Windows refuses to remove.
func newFixture(t *testing.T, spec Spec[fixtureRecord, fixtureLine]) *Shards[fixtureRecord, fixtureLine] {
	t.Helper()

	shards := New(spec)
	t.Cleanup(shards.Release)
	return shards
}

// TestIsShard_MatchesThePrefixAndTheExtensionOfTheSpec verifies that a name is
// a shard only when both halves the spec names are there, and that the pattern
// the writer creates a file with is built from the same two.
//
// The two are asked apart on purpose: the pattern goes to os.CreateTemp and the
// predicate to a directory walk, and a record whose prefix changed in one and
// not the other would write shards its own reader ignores.
func TestIsShard_MatchesThePrefixAndTheExtensionOfTheSpec(t *testing.T) {
	shards := New(plainSpec())

	if got, want := shards.pattern(), fixturePrefix+"*"+fixtureExt; got != want {
		t.Errorf("pattern() = %q, want %q", got, want)
	}

	cases := []struct {
		name  string
		file  string
		shard bool
	}{
		{name: "both halves", file: "fixture-1234.jsonl", shard: true},
		{name: "nothing between them", file: "fixture-.jsonl", shard: true},
		{name: "another prefix", file: "calls-1234.jsonl", shard: false},
		{name: "another extension", file: "fixture-1234.txt", shard: false},
		{name: "neither", file: "report.json", shard: false},
		{name: "empty", file: "", shard: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := shards.IsShard(testCase.file); got != testCase.shard {
				t.Errorf("IsShard(%q) = %t, want %t", testCase.file, got, testCase.shard)
			}
		})
	}
}

// TestNew_KeepsOneRegistryPerRecord verifies that two records naming one
// directory get a writer each, and write a shard each.
//
// A registry keyed on the directory alone would hand one record the other's
// writer, with the other's seen set and the other's open file, and the first
// symptom would be a shard holding lines of two kinds. It is also what keeps
// one record's Release from closing another's file, which the Windows reasoning
// on Release depends on.
func TestNew_KeepsOneRegistryPerRecord(t *testing.T) {
	dir := t.TempDir()
	first := newFixture(t, plainSpec())
	second := New(Spec[fixtureRecord, fixtureLine]{
		DirEnv:   "GITLAB_MCP_TEST_SHARDIO_OTHER_DIR",
		Prefix:   "other-",
		Ext:      fixtureExt,
		Noun:     "another fixture",
		TypeOf:   func(r fixtureRecord) string { return r.Type },
		Envelope: func(l fixtureLine) fixtureRecord { return l.record() },
		Validate: validateFixture,
	})
	t.Cleanup(second.Release)

	reporter := &recordingReporter{}
	one, other := first.OpenDir(dir), second.OpenDir(dir)
	if one == nil || other == nil {
		t.Fatal("OpenDir returned no writer for one of the two records")
	}
	if one == other {
		t.Fatal("the two records were handed one writer for the directory they share")
	}
	one.Write(reporter, &note{Text: "first"})
	other.Write(reporter, &note{Text: "second"})

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("files in the shared directory = %d, want one shard per record: %v", len(entries), entries)
	}
	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			if first.IsShard(entry.Name()) == second.IsShard(entry.Name()) {
				t.Errorf("%q is a shard of both records or of neither, want exactly one", entry.Name())
			}
		})
	}
}

// TestCreateShard_ReportsADirectoryItCannotMake verifies that the shard
// constructor fails when the directory cannot be created.
//
// It is called directly because every writer test stubs it, so this is the only
// place the real one's failure branch is reached.
func TestCreateShard_ReportsADirectoryItCannotMake(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	file, err := createShard(filepath.Join(blocked, "shards"), fixturePrefix+"*"+fixtureExt)

	if err == nil {
		_ = file.Close()
		t.Error("createShard succeeded under a regular file, want an error")
	}
}

// TestCreateShard_NamesTheFileAfterThePattern verifies that the real
// constructor makes the directory and names the file so the reader's predicate
// recognizes it.
func TestCreateShard_NamesTheFileAfterThePattern(t *testing.T) {
	shards := New(plainSpec())
	dir := filepath.Join(t.TempDir(), "made", "here")

	file, err := createShard(dir, shards.pattern())
	if err != nil {
		t.Fatalf("createShard error = %v", err)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("Close error = %v", closeErr)
	}

	name := filepath.Base(file.Name())
	if !shards.IsShard(name) {
		t.Errorf("createShard named %q, which IsShard does not recognize", name)
	}
	matched, matchErr := filepath.Match(shards.pattern(), name)
	if matchErr != nil {
		t.Fatalf("Match error = %v", matchErr)
	}
	if !matched {
		t.Errorf("createShard named %q, which does not match the pattern %q", name, shards.pattern())
	}
}

// probeLine returns the encoded length of one fixture line, so a test can pad a
// line to exactly the length the reader's scanner accepts.
func probeLine(t *testing.T, line fixtureLine) int {
	t.Helper()

	encoded, err := json.Marshal(line.record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	return len(encoded)
}

// lineOfLength returns a note line whose encoded form is exactly length bytes.
func lineOfLength(t *testing.T, length int) *note {
	t.Helper()

	// Every x escapes to one byte, so the padding is exact.
	padding := length - probeLine(t, &note{Text: ""})
	if padding < 0 {
		t.Fatalf("a note line is already %d bytes, want at most %d", probeLine(t, &note{Text: ""}), length)
	}
	line := &note{Text: strings.Repeat("x", padding)}
	if got := probeLine(t, line); got != length {
		t.Fatalf("the padded line is %d bytes, want %d", got, length)
	}
	return line
}

// wantMessage fails unless the reporter said exactly one thing and it carries
// every fragment named.
func wantMessage(t *testing.T, reporter *recordingReporter, fragments ...string) {
	t.Helper()

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	for _, fragment := range fragments {
		if !strings.Contains(reporter.joined(), fragment) {
			t.Errorf("report = %q, want it to carry %q", reporter.joined(), fragment)
		}
	}
}

// wantNoMessage fails unless the reporter said nothing.
func wantNoMessage(t *testing.T, reporter *recordingReporter) {
	t.Helper()

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
}
