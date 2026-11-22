// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// stamp_test.go holds the stamp subcommand to what
// scripts/update-server-json-sha.sh did, and homebrew_test.go holds the
// homebrew subcommand to scripts/update-homebrew-tap.sh.
//
// testdata/release_stamp.json is the record of both, taken at the tip of the
// layer below this one with jq 1.6. Each stamp run laid out a scratch tree
// holding the six manifests the stamp writes (frozen in the record as
// inputs), changed as the run says (the edited manifests are recorded, not
// the edit), a dist/ with a checksums.txt and stand-in .mcpb bundles, ran the
// script there with RELEASE_PACKAGES naming a build of this command for the
// npm step, and kept the exit status, stdout, stderr and every manifest the
// run changed, as the lines that differ from the input or as the whole text.
//
// The one difference held on purpose is the note the npm step prints when
// the launcher's package.json is absent: the script also skipped the step
// when RELEASE_PACKAGES was unset, and this command is what the step called,
// so its note names the file alone.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// stampRecord is testdata/release_stamp.json.
type stampRecord struct {
	RecordedFrom string                  `json:"recorded_from"`
	Version      string                  `json:"version"`
	Inputs       map[string]string       `json:"inputs"`
	Outputs      map[string]recordedText `json:"outputs"`
	License      string                  `json:"license"`
	GitEnv       map[string]string       `json:"git_env"`
	Stamps       []stampRun              `json:"stamps"`
	Taps         []tapRun                `json:"taps"`
}

// recordedText is a file's text: whole, or as the lines that differ from
// the input named by From, by index.
type recordedText struct {
	From  string            `json:"from"`
	Lines map[string]string `json:"lines"`
	Text  *string           `json:"text"`
}

// stampRun is one recorded run of update-server-json-sha.sh.
type stampRun struct {
	Name    string                  `json:"name"`
	Args    []string                `json:"args"`
	Files   map[string]string       `json:"files"`
	Remove  []string                `json:"remove"`
	Edited  map[string]recordedText `json:"edited"`
	Exit    int                     `json:"exit"`
	Stdout  string                  `json:"stdout"`
	Stderr  string                  `json:"stderr"`
	Changed map[string]string       `json:"changed"`
}

// readStampRecord loads the record.
func readStampRecord(t *testing.T) stampRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "release_stamp.json"))
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	var record stampRecord
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode the record: %v", err)
	}
	if len(record.Stamps) == 0 || len(record.Taps) == 0 {
		t.Fatal("the record holds no runs")
	}
	return record
}

// resolve is a recorded text as the file's content: whole, or the input it
// names with the recorded lines replaced.
func (r stampRecord) resolve(t *testing.T, text recordedText, path string) string {
	t.Helper()
	if text.Text != nil {
		return *text.Text
	}
	from := text.From
	if from == "" {
		from = path
	}
	lines := strings.Split(r.Inputs[from], "\n")
	for index, line := range text.Lines {
		i, err := strconv.Atoi(index)
		if err != nil || i >= len(lines) {
			t.Fatalf("recorded line %q of %s is outside the input", index, from)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// writeTree writes files under root, making their directories.
func writeTree(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for name, data := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// layOutStampTree builds the scratch tree one recorded run started from and
// returns it with the manifests it holds.
func layOutStampTree(t *testing.T, record stampRecord, run stampRun) (string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	manifests := map[string]string{}
	for path, text := range record.Inputs {
		manifests[path] = text
	}
	for _, path := range run.Remove {
		delete(manifests, path)
	}
	for path, text := range run.Edited {
		manifests[path] = record.resolve(t, text, path)
	}
	files := map[string][]byte{}
	for path, text := range manifests {
		files[path] = []byte(text)
	}
	for path, content := range run.Files {
		if strings.HasSuffix(path, ".txt") {
			files[path] = []byte(content)
			continue
		}
		data, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			t.Fatalf("recorded file %s is not base64: %v", path, err)
		}
		files[path] = data
	}
	writeTree(t, root, files)
	return root, manifests
}

// TestStamp_RecordedRuns_MatchTheScript replays every recorded stamp run and
// holds the exit code, stdout, stderr and every manifest to the script's,
// and checks that nothing else under the manifests moved.
func TestStamp_RecordedRuns_MatchTheScript(t *testing.T) {
	record := readStampRecord(t)
	for _, run := range record.Stamps {
		t.Run(run.Name, func(t *testing.T) {
			root, before := layOutStampTree(t, record, run)
			var stdout, stderr bytes.Buffer
			code := runIn(root, append([]string{"stamp"}, run.Args...), &stdout, &stderr, os.Getenv)
			if code != run.Exit {
				t.Errorf("exit %d, the script exited %d (stderr %q)", code, run.Exit, stderr.String())
			}
			// The one difference held on purpose: the npm note names the
			// file alone.
			wantOut := strings.ReplaceAll(run.Stdout, " not found or RELEASE_PACKAGES not set, skipping", " not found, skipping")
			if stdout.String() != wantOut {
				t.Errorf("stdout differs from the script's:\n--- got\n%s\n--- want\n%s", stdout.String(), wantOut)
			}
			if stderr.String() != run.Stderr {
				t.Errorf("stderr differs from the script's:\n--- got\n%s\n--- want\n%s", stderr.String(), run.Stderr)
			}
			paths := make([]string, 0, len(before))
			for path := range before {
				paths = append(paths, path)
			}
			sort.Strings(paths)
			for _, path := range paths {
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
				if err != nil {
					t.Errorf("%s: %v", path, err)
					continue
				}
				want := before[path]
				if hash, changed := run.Changed[path]; changed {
					want = record.resolve(t, record.Outputs[hash], path)
				}
				if string(got) != want {
					t.Errorf("%s differs from what the script left:\n--- got\n%s\n--- want\n%s", path, got, want)
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "tmp.") {
					t.Errorf("the run left %s behind", entry.Name())
				}
			}
		})
	}
}

// TestStamp_SecondRunWithTheSameInputs_WritesTheSameFiles holds the stamp to
// what commit-manifests relies on: run again over its own result with the
// same arguments, it writes the same bytes.
func TestStamp_SecondRunWithTheSameInputs_WritesTheSameFiles(t *testing.T) {
	record := readStampRecord(t)
	run := record.Stamps[0]
	root, before := layOutStampTree(t, record, run)
	snapshot := func() map[string]string {
		files := map[string]string{}
		for path := range before {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				t.Fatal(err)
			}
			files[path] = string(data)
		}
		return files
	}
	var out bytes.Buffer
	if code := runIn(root, append([]string{"stamp"}, run.Args...), &out, &out, os.Getenv); code != 0 {
		t.Fatalf("first run exited %d: %s", code, out.String())
	}
	first := snapshot()
	if code := runIn(root, append([]string{"stamp"}, run.Args...), &out, &out, os.Getenv); code != 0 {
		t.Fatalf("second run exited %d: %s", code, out.String())
	}
	for path, text := range snapshot() {
		if text != first[path] {
			t.Errorf("%s moved on the second run", path)
		}
	}
}
