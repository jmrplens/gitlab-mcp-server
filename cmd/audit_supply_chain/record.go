package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// actionRecordPath is where the record of what each pinned action's commit
// holds is committed, relative to the repository root.
const actionRecordPath = "docs/development/pinned-actions.json"

// actionRecordSchema is the version of the record's shape this command reads
// and writes.
const actionRecordSchema = 1

// recordCommand is the command a finding about the record tells its reader to
// run.
const recordCommand = "go run ./cmd/audit_supply_chain -record"

// actionRecordAbout is the sentence the record carries at its top, since JSON
// has no comments and a reader opening the file should not have to find this
// command to learn what it is.
const actionRecordAbout = "What each action the workflows pin holds at its commit: the metadata file of " +
	"every action a step reaches, and the files a credentialed job's steps read from an action's own " +
	"directory. Written by " + recordCommand + ", which reads them from raw.githubusercontent.com; " +
	"read offline by make check-supply-chain, which judges a composite action's steps from it and " +
	"refuses a pin it holds no entry for. Every file is kept as its lines, in ASCII."

// maxRecordedFile bounds one file -record fetches. An action's metadata and
// the scripts beside it are a few kilobytes; anything near this is not one,
// and is refused rather than committed.
const maxRecordedFile = 1 << 20

// recordFetchTimeout bounds one request -record makes.
const recordFetchTimeout = 30 * time.Second

// actionRecord is the committed record: for each pinned reference, in the
// spelling a uses: line carries it, what that commit holds.
type actionRecord struct {
	Actions map[string]recordedAction `json:"actions"`
	Schema  int                       `json:"schema"`
}

// recordedAction is what one pinned action's commit holds: which metadata file
// the runner reads, empty when the commit has neither, that file's lines, and
// the files of the action's directory a credentialed job's steps read. The
// fields are in the order a reader of the file wants them.
type recordedAction struct {
	Metadata string                  `json:"metadata"`
	Lines    []string                `json:"lines,omitempty"`
	Files    map[string]recordedFile `json:"files,omitempty"`
}

// recordedFile is one file of an action's directory: its lines, or the fact
// that the commit does not have it.
type recordedFile struct {
	Absent bool     `json:"absent,omitempty"`
	Lines  []string `json:"lines,omitempty"`
}

// writtenRecord is the record as the file carries it, with the schema and the
// sentence that says what it is ahead of what it holds.
type writtenRecord struct {
	Schema  int                       `json:"schema"`
	About   string                    `json:"about"`
	Actions map[string]recordedAction `json:"actions"`
}

// text is the metadata file's text, as it was fetched.
func (r recordedAction) text() string {
	return strings.Join(r.Lines, "\n")
}

// text is the file's text, as it was fetched.
func (f recordedFile) text() string {
	return strings.Join(f.Lines, "\n")
}

// textLines splits a fetched text into the lines the record keeps, so that
// joining them gives the text back byte for byte, a final newline included.
func textLines(text string) []string {
	return strings.Split(text, "\n")
}

// loadActionRecord reads the committed record under root. A tree without one
// records nothing, so every pin in it is reported as unjudged; a record that
// cannot be read, or is of another schema, stops the audit.
func loadActionRecord(root string) (actionRecord, error) {
	empty := actionRecord{Schema: actionRecordSchema, Actions: map[string]recordedAction{}}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(actionRecordPath))) //#nosec G304 -- the audited repository's own record.
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return actionRecord{}, fmt.Errorf("read %s: %w", actionRecordPath, err)
	}
	var written writtenRecord
	if err = json.Unmarshal(data, &written); err != nil {
		return actionRecord{}, fmt.Errorf("parse %s: %w", actionRecordPath, err)
	}
	if written.Schema != actionRecordSchema {
		return actionRecord{}, fmt.Errorf("%s: schema %d, want %d", actionRecordPath, written.Schema, actionRecordSchema)
	}
	if written.Actions != nil {
		empty.Actions = written.Actions
	}
	return empty, nil
}

// renderActionRecord writes the record in the form it is committed in:
// indented JSON, keys sorted, ending in one newline, and in ASCII, every other
// character spelled as a JSON escape. A third party's file is copied into this
// repository verbatim, and an em dash or an emoji in it would otherwise trip
// the text gates this repository holds its own files to.
func renderActionRecord(record actionRecord) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	// The record holds strings, booleans, an integer, maps keyed by strings
	// and slices only, which encoding/json always encodes, and a buffer
	// takes every write.
	cmdutil.MustDo(encoder.Encode(writtenRecord{About: actionRecordAbout, Schema: record.Schema, Actions: record.Actions}))
	return asciiJSON(buffer.String())
}

// asciiJSON spells every character of an encoded JSON document outside ASCII
// as the escape JSON defines for it, a surrogate pair beyond the basic plane.
// Only a string can hold one, so the document means what it meant.
func asciiJSON(document string) []byte {
	var builder strings.Builder
	for _, character := range document {
		if character < 0x80 {
			builder.WriteRune(character)
			continue
		}
		for _, unit := range utf16.Encode([]rune{character}) {
			fmt.Fprintf(&builder, `\u%04x`, unit)
		}
	}
	return []byte(builder.String())
}

// fetchFunc reads one file of a repository at a commit: its text, or false
// when the commit does not have it. Any other failure is an error.
type fetchFunc func(repository, sha, filePath string) (body string, found bool, err error)

// rawFetcher reads a commit's files from base, which serves every file of a
// public repository at a commit at base/owner/repo/sha/path with no token.
func rawFetcher(client *http.Client, base string) fetchFunc {
	return func(repository, sha, filePath string) (string, bool, error) {
		address := base + "/" + repository + "/" + sha + "/" + filePath
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, http.NoBody)
		if err != nil {
			return "", false, fmt.Errorf("fetch %s: %w", address, err)
		}
		response, err := client.Do(request)
		if err != nil {
			return "", false, fmt.Errorf("fetch %s: %w", address, err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode == http.StatusNotFound {
			return "", false, nil
		}
		if response.StatusCode != http.StatusOK {
			return "", false, fmt.Errorf("fetch %s: %s", address, response.Status)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maxRecordedFile+1))
		if err != nil {
			return "", false, fmt.Errorf("read %s: %w", address, err)
		}
		if len(body) > maxRecordedFile {
			return "", false, fmt.Errorf("fetch %s: larger than %d bytes, which no action's own file is", address, maxRecordedFile)
		}
		return string(body), true, nil
	}
}

// metadataNames are the files the runner reads an action's metadata from, in
// the order it looks for them.
var metadataNames = []string{"action.yml", "action.yaml"}

// actionStore answers what a pinned action's commit holds, from the record
// and, while -record runs, from the network.
//
// Offline, it remembers what was asked of it, so that what the record holds
// and nothing asked for can be reported as stale. Recording, it starts from
// nothing and records each answer it fetches, so the record it leaves behind
// is exactly what the audit read; err keeps the first fetch that failed, since
// a record built on a guess must not be written.
type actionStore struct {
	record      actionRecord
	fetch       fetchFunc
	err         error
	readActions map[string]bool
	readFiles   map[string]bool
}

// newOfflineStore answers from a committed record and never fetches.
func newOfflineStore(record actionRecord) *actionStore {
	return &actionStore{record: record, readActions: map[string]bool{}, readFiles: map[string]bool{}}
}

// newRecordingStore answers by fetching, and records each answer.
func newRecordingStore(fetch fetchFunc) *actionStore {
	store := newOfflineStore(actionRecord{Schema: actionRecordSchema, Actions: map[string]recordedAction{}})
	store.fetch = fetch
	return store
}

// action returns what the record holds for a pinned action, fetching it first
// while recording, and false when the record holds nothing for it.
func (s *actionStore) action(ref actionReference) (recordedAction, bool) {
	key := ref.key()
	s.readActions[key] = true
	recorded, ok := s.record.Actions[key]
	if ok || s.fetch == nil {
		return recorded, ok
	}
	for _, name := range metadataNames {
		body, found, err := s.fetch(ref.repository, ref.sha, path.Join(ref.directory, name))
		if err != nil {
			s.failed(err)
			return recordedAction{}, false
		}
		if found {
			recorded = recordedAction{Metadata: name, Lines: textLines(body)}
			break
		}
	}
	s.record.Actions[key] = recorded
	return recorded, true
}

// file returns a file of a pinned action's directory: its text and true when
// the commit has it, and whether the record answers for it at all, fetching it
// first while recording.
func (s *actionStore) file(key, filePath string) (body string, readable, recorded bool) {
	s.readFiles[key+"/"+filePath] = true
	action, known := s.record.Actions[key]
	if !known {
		return "", false, false
	}
	if file, ok := action.Files[filePath]; ok || s.fetch == nil {
		return file.text(), ok && !file.Absent, ok
	}
	ref, _ := parseActionReference(key)
	text, found, err := s.fetch(ref.repository, ref.sha, path.Join(ref.directory, filePath))
	if err != nil {
		s.failed(err)
		return "", false, false
	}
	entry := recordedFile{Absent: true}
	if found {
		entry = recordedFile{Lines: textLines(text)}
	}
	if action.Files == nil {
		action.Files = map[string]recordedFile{}
	}
	action.Files[filePath] = entry
	s.record.Actions[key] = action
	return text, found, true
}

// failed keeps the first fetch that failed.
func (s *actionStore) failed(err error) {
	if s.err == nil {
		s.err = err
	}
}

// staleProblems names what the record holds and nothing asked for: an action
// no step reaches, or a file of an action no step read, in a stable order.
func (s *actionStore) staleProblems() []string {
	keys := make([]string, 0, len(s.record.Actions))
	for key := range s.record.Actions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	for _, key := range keys {
		if !s.readActions[key] {
			problems = append(problems, fmt.Sprintf("%s: %s is recorded and no workflow reaches it; run %s",
				actionRecordPath, key, recordCommand))
			continue
		}
		files := make([]string, 0, len(s.record.Actions[key].Files))
		for filePath := range s.record.Actions[key].Files {
			files = append(files, filePath)
		}
		sort.Strings(files)
		for _, filePath := range files {
			if !s.readFiles[key+"/"+filePath] {
				problems = append(problems, fmt.Sprintf("%s: %s/%s is recorded and nothing reads it; run %s",
					actionRecordPath, key, filePath, recordCommand))
			}
		}
	}
	return problems
}
