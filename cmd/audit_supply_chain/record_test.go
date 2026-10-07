// Tests for the committed record of what each pinned action's commit holds,
// the store the audit reads it through, and the fetcher -record fills it with.
package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// attestKey and the fixture keys below are pinned references the record is
// keyed by, in the spelling a workflow's uses: line carries them.
const (
	attestKey   = "actions/attest@" + testSHA
	composedKey = "example/composite@" + testSHA
	nestedKey   = "example/tools/setup@" + testSHA
)

// TestLoadActionRecord_MissingFileOrActions_IsEmpty verifies that a tree
// without a record, and a record that names no actions, are read as one that
// records nothing, so every pinned action in it is reported as unjudged rather
// than the audit refusing to run or reading a nil map.
//
// A missing record is the state of every fixture and of a checkout that
// deleted the file; refusing would hide the pins behind one error, while an
// empty record names each of them.
func TestLoadActionRecord_MissingFileOrActions_IsEmpty(t *testing.T) {
	t.Parallel()

	bare := t.TempDir()
	writeFile(t, bare, actionRecordPath, `{"schema": 1}`)
	roots := map[string]string{"no record": t.TempDir(), "a record naming no actions": bare}

	for name, root := range roots {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			record, err := loadActionRecord(root)
			if err != nil {
				t.Fatalf("loadActionRecord: %v", err)
			}
			if record.Schema != actionRecordSchema || len(record.Actions) != 0 || record.Actions == nil {
				t.Errorf("loadActionRecord() = %#v, want an empty record of schema %d with a map to fill", record, actionRecordSchema)
			}
		})
	}
}

// TestLoadActionRecord_Unreadable_IsAnError verifies that a record the audit
// cannot read, or reads as another schema, stops the audit instead of being
// read as one that records nothing.
func TestLoadActionRecord_Unreadable_IsAnError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "not JSON", body: "{", want: "parse " + actionRecordPath},
		{name: "another schema", body: `{"schema": 2, "actions": {}}`, want: "schema 2, want 1"},
		{name: "a directory", body: "", want: "read " + actionRecordPath},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if testCase.body == "" {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(actionRecordPath)), 0o750); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
			} else {
				writeFile(t, root, actionRecordPath, testCase.body)
			}
			if _, err := loadActionRecord(root); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("loadActionRecord() error = %v, want one mentioning %q", err, testCase.want)
			}
		})
	}
}

// TestRenderActionRecord_Text_RoundTripsAsASCII verifies the record's written
// form: every file as the lines it holds, so a refresh reads as a line diff,
// written in ASCII, so a third party's em dash or emoji cannot trip the text
// gates this repository holds its own files to, and read back byte for byte.
func TestRenderActionRecord_Text_RoundTripsAsASCII(t *testing.T) {
	t.Parallel()

	original := actionRecord{Schema: actionRecordSchema, Actions: map[string]recordedAction{
		composedKey: {
			Metadata: "action.yml",
			Lines:    textLines("name: \u2014 \U0001F3C3 <&>\nruns:\n  using: composite\n"),
			Files: map[string]recordedFile{
				"setup.sh":   {Lines: textLines("echo ok")},
				"missing.sh": {Absent: true},
				"edge.sh":    {Lines: textLines("\u007f\u0080")},
			},
		},
		attestKey: {Metadata: ""},
	}}

	rendered := renderActionRecord(original)
	for index, character := range rendered {
		if character > 0x7f {
			t.Fatalf("renderActionRecord() byte %d is %#x, want ASCII only:\n%s", index, character, rendered)
		}
	}
	for _, want := range []string{`"name: \u2014 \ud83c\udfc3 <&>"`, `"absent": true`, "\n  \"actions\": {", "\"\x7f\\u0080\""} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(string(rendered), want) {
				t.Errorf("renderActionRecord() lacks %q:\n%s", want, rendered)
			}
		})
	}
	if !strings.HasSuffix(string(rendered), "}\n") {
		t.Errorf("renderActionRecord() does not end in one newline:\n%s", rendered)
	}

	root := t.TempDir()
	writeFile(t, root, actionRecordPath, string(rendered))
	loaded, err := loadActionRecord(root)
	if err != nil {
		t.Fatalf("loadActionRecord: %v", err)
	}
	if got := loaded.Actions[composedKey].text(); got != "name: \u2014 \U0001F3C3 <&>\nruns:\n  using: composite\n" {
		t.Errorf("text() after the round trip = %q", got)
	}
	if got := loaded.Actions[composedKey].Files["setup.sh"].text(); got != "echo ok" {
		t.Errorf("file text() after the round trip = %q, want %q", got, "echo ok")
	}
	if !loaded.Actions[composedKey].Files["missing.sh"].Absent || loaded.Actions[attestKey].Metadata != "" {
		t.Errorf("loaded = %#v, want the absent file and the metadata-less action kept", loaded)
	}
	if string(renderActionRecord(loaded)) != string(rendered) {
		t.Errorf("rendering what was loaded differs from what was written")
	}
}

// TestRawFetcher_Response_DecidesTheAnswer verifies how -record reads one file
// of a pinned commit: a 200 is its text, a 404 is a file the commit does not
// have, and anything else, a request that cannot be made or a body that is too
// large or cut short, is an error that stops the recording rather than an
// entry written as absent.
func TestRawFetcher_Response_DecidesTheAnswer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/actions/attest/" + testSHA + "/action.yml":
			_, _ = io.WriteString(writer, "runs:\n  using: node24\n")
		case "/actions/attest/" + testSHA + "/huge.sh":
			_, _ = writer.Write(make([]byte, maxRecordedFile+1))
		case "/actions/attest/" + testSHA + "/exact.sh":
			_, _ = writer.Write(make([]byte, maxRecordedFile))
		case "/actions/attest/" + testSHA + "/broken.sh":
			writer.WriteHeader(http.StatusBadGateway)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	fetch := rawFetcher(server.Client(), server.URL)

	body, found, err := fetch("actions/attest", testSHA, "action.yml")
	if err != nil || !found || body != "runs:\n  using: node24\n" {
		t.Errorf("fetch(action.yml) = %q, %v, %v, want the file", body, found, err)
	}
	if body, found, err = fetch("actions/attest", testSHA, "action.yaml"); err != nil || found || body != "" {
		t.Errorf("fetch(action.yaml) = %q, %v, %v, want a file the commit does not have", body, found, err)
	}
	if body, found, err = fetch("actions/attest", testSHA, "exact.sh"); err != nil || !found || len(body) != maxRecordedFile {
		t.Errorf("fetch(exact.sh) = %d bytes, %v, %v, want a file of exactly the bound accepted", len(body), found, err)
	}
	for _, file := range []string{"huge.sh", "broken.sh"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			if _, refused, fetchErr := fetch("actions/attest", testSHA, file); fetchErr == nil || refused {
				t.Errorf("fetch(%s) = %v, %v, want an error", file, refused, fetchErr)
			}
		})
	}
}

// TestRawFetcher_RequestThatFails_IsAnError verifies the three failures no
// server answer describes: a base that is not a URL, a transport that cannot
// reach the host, and a body that breaks off while it is read. Each is an
// error naming its cause, never a file the commit does not have.
func TestRawFetcher_RequestThatFails_IsAnError(t *testing.T) {
	t.Parallel()

	failing := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no route")
	})}
	truncated := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(failingReader{})}, nil
	})}
	cases := []struct {
		name   string
		client *http.Client
		base   string
		want   string
	}{
		{name: "a base that is not a URL", client: http.DefaultClient, base: "http://\x7f", want: "invalid control character"},
		{name: "a transport that fails", client: failing, base: "http://example.test", want: "no route"},
		{name: "a body cut short", client: truncated, base: "http://example.test", want: "cut short"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, found, err := rawFetcher(testCase.client, testCase.base)("a/b", testSHA, "action.yml")
			if err == nil || found || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("fetch() = %v, %v, want an error mentioning %q", found, err, testCase.want)
			}
		})
	}
}

// TestActionStore_Offline_AnswersFromTheRecordAndKnowsWhatWentUnread verifies
// the store the gate reads: an action or file the record holds is answered, one
// it does not is reported as unrecorded, and whatever the record holds that no
// reference asked for is stale.
func TestActionStore_Offline_AnswersFromTheRecordAndKnowsWhatWentUnread(t *testing.T) {
	t.Parallel()

	store := newOfflineStore(actionRecord{Schema: actionRecordSchema, Actions: map[string]recordedAction{
		composedKey: {Metadata: "action.yml", Lines: textLines("runs: {}"), Files: map[string]recordedFile{
			"read.sh":   {Lines: textLines("echo read")},
			"absent.sh": {Absent: true},
			"unread.sh": {Lines: textLines("echo unread")},
		}},
		attestKey: {Metadata: "action.yml", Lines: textLines("runs: {}")},
	}})

	composed, _ := parseActionReference(composedKey)
	if recorded, ok := store.action(composed); !ok || recorded.text() != "runs: {}" {
		t.Errorf("action(%s) = %#v, %v, want the recorded action", composedKey, recorded, ok)
	}
	unknown, _ := parseActionReference("example/unknown@" + testSHA)
	if _, ok := store.action(unknown); ok {
		t.Errorf("action(unknown) found an action the record does not hold")
	}
	if body, readable, recorded := store.file(composedKey, "read.sh"); body != "echo read" || !readable || !recorded {
		t.Errorf("file(read.sh) = %q, %v, %v, want the recorded text", body, readable, recorded)
	}
	if _, readable, recorded := store.file(composedKey, "absent.sh"); readable || !recorded {
		t.Errorf("file(absent.sh) = %v, %v, want a recorded absence", readable, recorded)
	}
	if _, _, recorded := store.file(composedKey, "never.sh"); recorded {
		t.Error("file(never.sh) is recorded, want a file the record does not hold")
	}
	if _, _, recorded := store.file("example/unknown@"+testSHA, "x.sh"); recorded {
		t.Error("file of an unrecorded action is recorded")
	}

	want := []string{
		actionRecordPath + ": " + attestKey + " is recorded and no workflow reaches it; run " + recordCommand,
		actionRecordPath + ": " + composedKey + "/unread.sh is recorded and nothing reads it; run " + recordCommand,
	}
	if got := store.staleProblems(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("staleProblems() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestActionStore_Recording_FetchesWhatIsAskedAndRecordsIt verifies the store
// -record fills: an action is looked for as action.yml, then action.yaml, under
// its directory; a commit with neither is recorded as having no metadata; a
// file is fetched from the action's directory and recorded, a missing one as
// absent; and nothing is fetched twice.
func TestActionStore_Recording_FetchesWhatIsAskedAndRecordsIt(t *testing.T) {
	t.Parallel()

	served := map[string]string{
		"example/tools/" + testSHA + "/setup/action.yaml": "runs:\n  using: composite\n",
		"example/tools/" + testSHA + "/setup/run.sh":      "echo run\n",
		"actions/attest/" + testSHA + "/action.yml":       "runs:\n  using: node24\n",
	}
	var asked []string
	store := newRecordingStore(func(repository, sha, filePath string) (string, bool, error) {
		address := repository + "/" + sha + "/" + filePath
		asked = append(asked, address)
		body, found := served[address]
		return body, found, nil
	})

	nested, _ := parseActionReference(nestedKey)
	if recorded, ok := store.action(nested); !ok || recorded.Metadata != "action.yaml" || recorded.text() != "runs:\n  using: composite\n" {
		t.Errorf("action(%s) = %#v, %v, want action.yaml", nestedKey, recorded, ok)
	}
	if body, readable, recorded := store.file(nestedKey, "run.sh"); body != "echo run\n" || !readable || !recorded {
		t.Errorf("file(run.sh) = %q, %v, %v, want the fetched text", body, readable, recorded)
	}
	if _, readable, recorded := store.file(nestedKey, "gone.sh"); readable || !recorded {
		t.Errorf("file(gone.sh) = %v, %v, want a recorded absence", readable, recorded)
	}
	attest, _ := parseActionReference(attestKey)
	if recorded, ok := store.action(attest); !ok || recorded.Metadata != "action.yml" {
		t.Errorf("action(%s) = %#v, %v, want action.yml", attestKey, recorded, ok)
	}
	bare, _ := parseActionReference("example/bare@" + testSHA)
	if recorded, ok := store.action(bare); !ok || recorded.Metadata != "" {
		t.Errorf("action(bare) = %#v, %v, want an action recorded with no metadata", recorded, ok)
	}
	if store.err != nil || len(store.staleProblems()) != 0 {
		t.Errorf("store.err = %v, staleProblems() = %v, want neither in a record built from what was asked", store.err, store.staleProblems())
	}
	wantAsked := []string{
		"example/tools/" + testSHA + "/setup/action.yml",
		"example/tools/" + testSHA + "/setup/action.yaml",
		"example/tools/" + testSHA + "/setup/run.sh",
		"example/tools/" + testSHA + "/setup/gone.sh",
		"actions/attest/" + testSHA + "/action.yml",
		"example/bare/" + testSHA + "/action.yml",
		"example/bare/" + testSHA + "/action.yaml",
	}
	if strings.Join(asked, "\n") != strings.Join(wantAsked, "\n") {
		t.Errorf("fetched =\n%s\nwant\n%s", strings.Join(asked, "\n"), strings.Join(wantAsked, "\n"))
	}
	if _, again := store.action(nested); !again || len(asked) != len(wantAsked) {
		t.Errorf("asking for %s again fetched it again", nestedKey)
	}
}

// TestActionStore_Recording_Failure_KeepsTheFirstError verifies that a fetch
// that fails records nothing, stops the search for the metadata file at the
// failure, and that the store keeps the first error while it goes on
// answering, so the run that sees it can refuse to write a record built on a
// guess and say which fetch broke it.
func TestActionStore_Recording_Failure_KeepsTheFirstError(t *testing.T) {
	t.Parallel()

	var asked []string
	store := newRecordingStore(func(_, _, filePath string) (string, bool, error) {
		asked = append(asked, filePath)
		if strings.Contains(filePath, "fails") {
			return "", false, errors.New("network down " + filePath)
		}
		return "runs:\n  using: composite\n", true, nil
	})

	nested, _ := parseActionReference(nestedKey)
	if _, ok := store.action(nested); !ok {
		t.Fatalf("action(%s) not recorded", nestedKey)
	}
	if _, _, recorded := store.file(nestedKey, "fails-first.sh"); recorded {
		t.Error("a file whose fetch failed is recorded")
	}
	if _, _, recorded := store.file(nestedKey, "fails-second.sh"); recorded {
		t.Error("a second failed fetch is recorded")
	}
	if store.err == nil || !strings.Contains(store.err.Error(), "fails-first.sh") {
		t.Errorf("store.err = %v, want the first failure kept", store.err)
	}

	broken, _ := parseActionReference("example/fails@" + testSHA)
	failing := newRecordingStore(func(_, _, filePath string) (string, bool, error) {
		asked = append(asked, filePath)
		return "", false, errors.New("network down " + filePath)
	})
	if _, ok := failing.action(broken); ok {
		t.Error("an action whose fetch failed is recorded")
	}
	if failing.err == nil || !strings.Contains(failing.err.Error(), "action.yml") {
		t.Errorf("failing.err = %v, want the first failure, on action.yml", failing.err)
	}
	want := []string{"setup/action.yml", "setup/fails-first.sh", "setup/fails-second.sh", "action.yml"}
	if strings.Join(asked, " ") != strings.Join(want, " ") {
		t.Errorf("fetched %v, want %v: the search for metadata stops at a failure", asked, want)
	}
}

// roundTripFunc adapts a function to http.RoundTripper, so a test can hand the
// fetcher a transport that fails the way a network does.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// failingReader is a response body whose first read fails, which is what a
// connection that drops mid-body looks like to the reader.
type failingReader struct{}

// Read implements io.Reader.
func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("cut short")
}

// writeRecord writes record into a fixture root as the committed record.
func writeRecord(t *testing.T, root string, record actionRecord) {
	t.Helper()

	writeFile(t, root, actionRecordPath, string(renderActionRecord(record)))
}

// recordOf builds a record from each action's metadata text, keyed by the
// pinned reference, with every action's metadata named action.yml.
func recordOf(actions map[string]string) actionRecord {
	record := actionRecord{Schema: actionRecordSchema, Actions: map[string]recordedAction{}}
	for key, text := range actions {
		record.Actions[key] = recordedAction{Metadata: "action.yml", Lines: textLines(text)}
	}
	return record
}
