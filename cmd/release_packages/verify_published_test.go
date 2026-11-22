// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// verify_published_test.go holds the verify-published subcommand to what
// scripts/verify_published_packages.py did.
//
// testdata/verify_published.json is the record. Each run stood in scripted
// registries for urlopen (a list of answers per URL, each a body or an HTTP
// status, consumed in order with the last repeating, and 404 for a URL with
// none), a clock that only sleeping moves, a checksums.txt and, for most
// runs, the digests the nuget job attested, then ran the script's main() at
// 1c26b3b04, the revision of main this port started from. The record keeps
// the answers every run starts from once, and for each run the answers it
// changed, the URLs it removed, its arguments, exit status, stdout, stderr,
// every URL it asked in order and every sleep it took.
//
// Each run is repeated here with the same answers served by a RoundTripper
// at the same URLs, so the requests are compared as the script made them.
// The one difference held on purpose is the name a refusal on stderr starts
// with, this command's rather than the script's.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// verifyRecord is testdata/verify_published.json.
type verifyRecord struct {
	RecordedFrom string                       `json:"recorded_from"`
	Version      string                       `json:"version"`
	Answers      map[string][]json.RawMessage `json:"answers"`
	Runs         []verifyRecordRun            `json:"runs"`
}

// verifyRecordRun is one recorded run of the script.
type verifyRecordRun struct {
	Name         string                       `json:"name"`
	Args         []string                     `json:"args"`
	Checksums    string                       `json:"checksums"`
	NugetDigests *string                      `json:"nuget_digests"`
	Answers      map[string][]json.RawMessage `json:"answers"`
	Removed      []string                     `json:"removed"`
	Exit         int                          `json:"exit"`
	Stdout       string                       `json:"stdout"`
	Stderr       string                       `json:"stderr"`
	Requests     []string                     `json:"requests"`
	Slept        []float64                    `json:"slept"`
}

// scriptedRegistries answers each URL with its recorded outcomes in order,
// the last one repeating, and records every URL asked.
type scriptedRegistries struct {
	t       *testing.T
	answers map[string][]json.RawMessage
	calls   []string
}

func (s *scriptedRegistries) RoundTrip(req *http.Request) (*http.Response, error) {
	asked := req.URL.String()
	s.calls = append(s.calls, asked)
	status, body := http.StatusNotFound, []byte(nil)
	if outcomes := s.answers[asked]; len(outcomes) > 0 {
		outcome := outcomes[0]
		if len(outcomes) > 1 {
			s.answers[asked] = outcomes[1:]
		}
		var encoded string
		if err := json.Unmarshal(outcome, &encoded); err == nil {
			decoded, decodeErr := base64.StdEncoding.DecodeString(encoded)
			if decodeErr != nil {
				s.t.Errorf("recorded body for %s is not base64: %v", asked, decodeErr)
			}
			status, body = http.StatusOK, decoded
		} else if err := json.Unmarshal(outcome, &status); err != nil {
			s.t.Errorf("recorded outcome for %s is neither a body nor a status: %s", asked, outcome)
		}
	}
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req,
	}, nil
}

// readVerifyRecord loads the record.
func readVerifyRecord(t *testing.T) verifyRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "verify_published.json"))
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	var record verifyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode the record: %v", err)
	}
	if len(record.Runs) == 0 {
		t.Fatal("the record holds no runs")
	}
	return record
}

// replayVerify runs verify-published as one recorded run did and returns
// its exit code, stdout and stderr with the scratch paths put back as the
// record's placeholders, the URLs it asked and the sleeps it took.
func replayVerify(t *testing.T, record verifyRecord, run verifyRecordRun) (code int, stdout, stderr string, calls []string, slept []float64) {
	t.Helper()
	dir := t.TempDir()
	sums := filepath.Join(dir, "checksums.txt")
	digests := filepath.Join(dir, "nupkg.sha256")
	if err := os.WriteFile(sums, []byte(run.Checksums), 0o600); err != nil {
		t.Fatal(err)
	}
	if run.NugetDigests != nil {
		if err := os.WriteFile(digests, []byte(*run.NugetDigests), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{record.Version}
	for _, arg := range run.Args {
		switch arg {
		case "<checksums>":
			arg = sums
		case "<digests>":
			arg = digests
		}
		args = append(args, arg)
	}
	options, err := parseVerifyArgs(args)
	if err != nil {
		t.Fatalf("parse %q: %v", args, err)
	}

	answers := map[string][]json.RawMessage{}
	for url, outcomes := range record.Answers {
		answers[url] = slices.Clone(outcomes)
	}
	for url, outcomes := range run.Answers {
		answers[url] = slices.Clone(outcomes)
	}
	for _, url := range run.Removed {
		delete(answers, url)
	}
	registries := &scriptedRegistries{t: t, answers: answers}
	now := time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	sleep := func(d time.Duration) {
		slept = append(slept, d.Seconds())
		now = now.Add(d)
	}

	var out, errOut bytes.Buffer
	code = verifyRun(dir, options, &out, &errOut, &http.Client{Transport: registries}, clock, sleep)
	placeholders := strings.NewReplacer(sums, "<checksums>", digests, "<digests>")
	return code, placeholders.Replace(out.String()), placeholders.Replace(errOut.String()), registries.calls, slept
}

// TestVerifyPublished_RecordedRuns_MatchTheScript replays every recorded run
// and holds the exit code, stdout, stderr, the URLs asked in order and the
// sleeps taken to the script's.
func TestVerifyPublished_RecordedRuns_MatchTheScript(t *testing.T) {
	record := readVerifyRecord(t)
	for _, run := range record.Runs {
		t.Run(run.Name, func(t *testing.T) {
			code, stdout, stderr, calls, slept := replayVerify(t, record, run)
			if code != run.Exit {
				t.Errorf("exit %d, the script exited %d", code, run.Exit)
			}
			if stdout != run.Stdout {
				t.Errorf("stdout differs from the script's:\n--- got\n%s\n--- want\n%s", stdout, run.Stdout)
			}
			// The one difference held on purpose: a refusal names this
			// command.
			wantErr := strings.ReplaceAll(run.Stderr, "verify_published_packages: ", toolName+" verify-published: ")
			if stderr != wantErr {
				t.Errorf("stderr %q, want %q", stderr, wantErr)
			}
			if !slices.Equal(calls, run.Requests) {
				t.Errorf("asked %d URL(s), the script asked %d:\n--- got\n%s\n--- want\n%s",
					len(calls), len(run.Requests), strings.Join(calls, "\n"), strings.Join(run.Requests, "\n"))
			}
			if !slices.Equal(slept, run.Slept) && (len(slept) > 0 || len(run.Slept) > 0) {
				t.Errorf("slept %v, the script slept %v", slept, run.Slept)
			}
		})
	}
}
