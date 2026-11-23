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
// The differences held on purpose are the name a refusal on stderr starts
// with, this command's rather than the script's, and the words a decoder
// gives for what it could not read, Go's rather than Python's, named in
// decoderWords.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"io"
	"maps"
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

// decoderWords are the reasons a run reports in the words of Go's decoders
// that the script reported in Python's, by run: the rest of each line is
// the script's.
var decoderWords = map[string][][2]string{
	"a wheel's provenance that cannot be read": {
		{"illegal base64 data at input byte 3", "Only base64 data is allowed"},
	},
	"a nuget index that is not JSON": {
		{"invalid character '<' looking for beginning of value", "Expecting value: line 1 column 1 (char 0)"},
	},
	"a nuget package that is not a zip": {
		{"zip: not a valid zip file", "File is not a zip file"},
	},
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
	status, body := s.next(asked)
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req,
	}, nil
}

// next takes the URL's next outcome, the last one repeating: a body is
// answered with 200, and a URL with none is answered with 404.
func (s *scriptedRegistries) next(asked string) (status int, body []byte) {
	outcomes := s.answers[asked]
	if len(outcomes) == 0 {
		return http.StatusNotFound, nil
	}
	outcome := outcomes[0]
	if len(outcomes) > 1 {
		s.answers[asked] = outcomes[1:]
	}
	var encoded string
	if json.Unmarshal(outcome, &encoded) != nil {
		if err := json.Unmarshal(outcome, &status); err != nil {
			s.t.Errorf("recorded outcome for %s is neither a body nor a status: %s", asked, outcome)
		}
		return status, nil
	}
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		s.t.Errorf("recorded body for %s is not base64: %v", asked, err)
	}
	return http.StatusOK, body
}

// readVerifyRecord loads the record.
func readVerifyRecord(t *testing.T) verifyRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "verify_published.json"))
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	var record verifyRecord
	if err = json.Unmarshal(data, &record); err != nil {
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
			for _, words := range decoderWords[run.Name] {
				if !strings.Contains(stdout, words[0]) {
					t.Errorf("the declared decoder words %q are not in the output", words[0])
				}
				stdout = strings.ReplaceAll(stdout, words[0], words[1])
			}
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

// fakeClock is a clock only sleeping moves.
type fakeClock struct {
	now   time.Time
	slept []float64
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) clock() time.Time { return c.now }

func (c *fakeClock) sleep(d time.Duration) {
	c.slept = append(c.slept, d.Seconds())
	c.now = c.now.Add(d)
}

// TestVerifyPublished_NpmAndNugetWaitingTogether_KeepTheirOwnAllowances
// replays a run where npm and NuGet both wait, which no recorded run does:
// the lagging npm package of one recorded run and the NuGet pointer of
// another, together. npm spending its whole budget leaves NuGet's wait
// untouched, and NuGet's deadline still counts from the start of the run,
// so npm's time is part of it.
func TestVerifyPublished_NpmAndNugetWaitingTogether_KeepTheirOwnAllowances(t *testing.T) {
	record := readVerifyRecord(t)
	recorded := func(name string) verifyRecordRun {
		t.Helper()
		for _, run := range record.Runs {
			if run.Name == name {
				return run
			}
		}
		t.Fatalf("the record holds no run %q", name)
		return verifyRecordRun{}
	}
	together := func(nuget, deadline string) verifyRecordRun {
		run := recorded("an npm version served after a wait")
		run.Answers = maps.Clone(run.Answers)
		maps.Copy(run.Answers, recorded(nuget).Answers)
		// One 30s wait is npm's whole budget.
		run.Args = []string{"<checksums>", "--retry-budget", "30", "--retry-delay", "30", "--nuget-deadline", deadline, "--nuget-digests", "<digests>"}
		return run
	}
	t.Run("npm spending its budget leaves NuGet its wait", func(t *testing.T) {
		code, stdout, _, _, slept := replayVerify(t, record, together("the nuget pointer listed after a wait", "120"))
		if code != 0 {
			t.Errorf("exit %d, want 0: NuGet had no wait left after npm's\n%s", code, stdout)
		}
		if !slices.Equal(slept, []float64{30, 30}) {
			t.Errorf("slept %v, want npm's one wait and then NuGet's", slept)
		}
	})
	t.Run("the NuGet deadline counts npm's time", func(t *testing.T) {
		code, stdout, _, _, slept := replayVerify(t, record, together("the nuget pointer never listed", "60"))
		if code != 1 {
			t.Errorf("exit %d, want 1 for a pointer never listed\n%s", code, stdout)
		}
		// npm's 30s are the first half of NuGet's 60, so NuGet waits once,
		// not twice.
		if !slices.Equal(slept, []float64{30, 30}) {
			t.Errorf("slept %v, want npm's wait and one of NuGet's inside 60s from the start", slept)
		}
	})
}

// TestRetryBudget_Wait_StopsWhenTheAllowanceRunsOut holds the budget to an
// allowance of sleep: two 4s waits fit in 10s and a third does not, and a
// zero budget or a zero delay never waits.
func TestRetryBudget_Wait_StopsWhenTheAllowanceRunsOut(t *testing.T) {
	c := newFakeClock()
	budget := &retryBudget{remaining: 10, delay: 4, sleep: c.sleep}
	if waited := []bool{budget.wait(), budget.wait(), budget.wait()}; !slices.Equal(waited, []bool{true, true, false}) {
		t.Error("10s of budget allow exactly two 4s waits")
	}
	if !slices.Equal(c.slept, []float64{4, 4}) || budget.waits != 2 {
		t.Errorf("slept %v with %d waits, want [4 4] and 2", c.slept, budget.waits)
	}
	zero := (&retryBudget{remaining: 0, delay: 20, sleep: c.sleep}).wait()
	instant := (&retryBudget{remaining: 600, delay: 0, sleep: c.sleep}).wait()
	if zero || instant {
		t.Errorf("a zero budget waited (%t) or a zero delay waited (%t)", zero, instant)
	}
}

// TestRetryDeadline_Wait_CountsFromTheStart holds NuGet's allowance to an
// instant: a wait that would end past it is refused, time spent before the
// first wait counts, and a deadline long past has nothing left.
func TestRetryDeadline_Wait_CountsFromTheStart(t *testing.T) {
	deadline := func(c *fakeClock, secs, delay float64) *retryDeadline {
		return &retryDeadline{seconds: secs, delay: delay, end: c.now.Add(seconds(secs)), now: c.clock, sleep: c.sleep}
	}
	t.Run("a wait past the deadline", func(t *testing.T) {
		c := newFakeClock()
		d := deadline(c, 10, 4)
		if waited := []bool{d.wait(), d.wait(), d.wait()}; !slices.Equal(waited, []bool{true, true, false}) {
			t.Error("a third 4s wait would end at 12s, past the 10s deadline")
		}
		if d.remaining() != 2 || d.waits != 2 {
			t.Errorf("remaining %v after %d waits, want 2 after 2", d.remaining(), d.waits)
		}
	})
	t.Run("time before the first wait", func(t *testing.T) {
		c := newFakeClock()
		d := deadline(c, 100, 30)
		c.now = c.now.Add(80 * time.Second)
		if d.remaining() != 20 || d.wait() || len(c.slept) > 0 {
			t.Errorf("eighty seconds spent leave %v of a hundred and the wait was allowed: %v", d.remaining(), c.slept)
		}
	})
	t.Run("a deadline long past", func(t *testing.T) {
		c := newFakeClock()
		d := deadline(c, 10, 1)
		c.now = c.now.Add(50 * time.Second)
		if d.remaining() != 0 || d.wait() {
			t.Errorf("remaining %v, want 0 and no wait", d.remaining())
		}
	})
	t.Run("no deadline or no delay", func(t *testing.T) {
		c := newFakeClock()
		if deadline(c, 0, 20).wait() || deadline(c, 600, 0).wait() || len(c.slept) > 0 {
			t.Error("a zero deadline or a zero delay waited")
		}
	})
}

// TestParseVerifyArgs_DefaultsAndRefusals holds the command line: the stated
// defaults, positionals among the flags, and each argument it cannot use.
func TestParseVerifyArgs_DefaultsAndRefusals(t *testing.T) {
	o, err := parseVerifyArgs([]string{"--skip-npm", "1.2.3", "--skip-pypi", "dist", "--skip-nuget"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := verifyOptions{
		version: "1.2.3", checksums: "dist", skipNpm: true, skipPypi: true, skipNuget: true,
		retryBudget: 600, retryDelay: 20, deadline: 2400,
	}
	if o != want {
		t.Errorf("parsed %+v, want %+v", o, want)
	}
	refusals := map[string][]string{
		"--nuget-digests needs a file":    {"1.2.3", "dist", "--nuget-digests"},
		"--retry-budget needs a value":    {"1.2.3", "dist", "--retry-budget"},
		"--retry-delay needs a number":    {"1.2.3", "dist", "--retry-delay", "soon"},
		"unknown option --verbose":        {"1.2.3", "dist", "--verbose"},
		"takes the release version":       {"1.2.3"},
		"takes the release version and t": {"1.2.3", "dist", "extra"},
	}
	for message, args := range refusals {
		t.Run(message, func(t *testing.T) {
			if _, refusal := parseVerifyArgs(args); refusal == nil || !strings.Contains(refusal.Error(), message) {
				t.Errorf("parse %q: %v, want %q", args, refusal, message)
			}
		})
	}
}

// TestRunVerifyPublished_RefusesWhatItCannotRead holds the refusals that
// end a run before a registry is asked: a usage error is 2, and an
// unreadable checksums.txt or digests file is 1, each on stderr.
func TestRunVerifyPublished_RefusesWhatItCannotRead(t *testing.T) {
	dir := t.TempDir()
	sums := filepath.Join(dir, checksumsName)
	line := strings.Repeat("a", 64) + "  gitlab-mcp-server-linux-amd64\n"
	if err := os.WriteFile(sums, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"usage", []string{"1.2.3"}, 2, "takes the release version"},
		{"no checksums.txt", []string{"1.2.3", "missing"}, 1, "missing"},
		{"no digests file", []string{"1.2.3", dir, "--skip-npm", "--skip-pypi", "--nuget-digests", "gone"}, 1, "gone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runIn(dir, append([]string{"verify-published"}, tc.args...), &stdout, &stderr, os.Getenv)
			if code != tc.code || !strings.Contains(stderr.String(), tc.want) ||
				!strings.HasPrefix(stderr.String(), toolName+" verify-published: ") {
				t.Errorf("exit %d, stderr %q; want %d naming %q", code, stderr.String(), tc.code, tc.want)
			}
		})
	}
	t.Run("a directory holding checksums.txt", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runIn(dir, []string{"verify-published", "1.2.3", ".", "--skip-npm", "--skip-pypi", "--skip-nuget"}, &stdout, &stderr, os.Getenv)
		if code != 0 || !strings.Contains(stdout.String(), "against 1 entries in "+checksumsName) {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
	})
}

// TestAttestedDigests_ReadsTheLinesItWants holds the reader of the nuget
// job's output to sha256sum's format: a binary-mode star and a carriage
// return are read through, and a line naming no package, a short digest and
// a lone token are ignored.
func TestAttestedDigests_ReadsTheLinesItWants(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("B", 64)
	text := a + "  gitlab-mcp-server.1.0.0.nupkg\r\n" +
		b + " *gitlab-mcp-server.linux-x64.1.0.0.nupkg\n" +
		strings.Repeat("c", 64) + "  checksums.txt\n" +
		strings.Repeat("d", 63) + "  gitlab-mcp-server.osx-x64.1.0.0.nupkg\n" +
		"lonely-token\n\n"
	file := filepath.Join(t.TempDir(), "nupkg.sha256")
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := attestedDigests(file)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"gitlab-mcp-server.1.0.0.nupkg": a, "gitlab-mcp-server.linux-x64.1.0.0.nupkg": strings.ToLower(b)}
	if len(got) != len(want) || got["gitlab-mcp-server.1.0.0.nupkg"] != a || got["gitlab-mcp-server.linux-x64.1.0.0.nupkg"] != want["gitlab-mcp-server.linux-x64.1.0.0.nupkg"] {
		t.Errorf("read %v, want %v", got, want)
	}
}

// unsignedZip is a package built by archive/zip with the entries named.
func unsignedZip(t *testing.T, comment string, names ...string) []byte {
	t.Helper()
	var blob bytes.Buffer
	w := zip.NewWriter(&blob)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte("content of " + name)); err != nil {
			t.Fatal(err)
		}
	}
	if comment != "" {
		if err := w.SetComment(comment); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return blob.Bytes()
}

// signNupkg signs a package the way nuget.org does: a stored entry named
// name after the last local entry, its record after the last record, and
// the end record counted up, the archive comment kept.
func signNupkg(t *testing.T, unsigned []byte, name string) []byte {
	t.Helper()
	le := binary.LittleEndian
	eocd := bytes.LastIndex(unsigned, []byte("PK\x05\x06"))
	if eocd < 0 {
		t.Fatal("no end record to sign")
	}
	count := le.Uint16(unsigned[eocd+10:])
	size, offset := le.Uint32(unsigned[eocd+12:]), le.Uint32(unsigned[eocd+16:])
	const data = "signature"
	nameLen := uint16(len(name)) //#nosec G115 -- a short entry name a test chose
	sum := crc32.ChecksumIEEE([]byte(data))
	local := binary.LittleEndian.AppendUint32(nil, 0x04034b50)
	local = le.AppendUint16(local, 20)
	local = le.AppendUint16(local, 0)
	local = le.AppendUint16(local, 0)
	local = le.AppendUint32(local, 0)
	local = le.AppendUint32(local, sum)
	local = le.AppendUint32(local, uint32(len(data)))
	local = le.AppendUint32(local, uint32(len(data)))
	local = le.AppendUint16(local, nameLen)
	local = le.AppendUint16(local, 0)
	local = append(append(local, name...), data...)
	record := le.AppendUint32(nil, 0x02014b50)
	record = le.AppendUint16(record, 20)
	record = le.AppendUint16(record, 20)
	record = le.AppendUint16(record, 0)
	record = le.AppendUint16(record, 0)
	record = le.AppendUint32(record, 0)
	record = le.AppendUint32(record, sum)
	record = le.AppendUint32(record, uint32(len(data)))
	record = le.AppendUint32(record, uint32(len(data)))
	record = le.AppendUint16(record, nameLen)
	record = append(record, make([]byte, 12)...)
	record = le.AppendUint32(record, offset)
	record = append(record, name...)
	out := bytes.Clone(unsigned[:offset])
	out = append(out, local...)
	out = append(out, unsigned[offset:offset+size]...)
	out = append(out, record...)
	end := bytes.Clone(unsigned[eocd:])
	le.PutUint16(end[8:], count+1)
	le.PutUint16(end[10:], count+1)
	le.PutUint32(end[12:], size+46+uint32(nameLen))
	le.PutUint32(end[16:], offset+30+uint32(nameLen)+uint32(len(data)))
	return append(out, end...)
}

// TestNugetUnsigned_GivesBackThePackageThatWasSigned holds the unsigning to
// NuGet's definition: the package before signing comes back byte for byte,
// its comment kept, an unsigned package and one whose look-alike entry has
// another case come back unchanged, and each package that cannot be
// unsigned is refused with its reason.
func TestNugetUnsigned_GivesBackThePackageThatWasSigned(t *testing.T) {
	unsigned := unsignedZip(t, "", "gitlab-mcp-server.nuspec", "tools/any/linux-x64/gitlab-mcp-server")
	commented := unsignedZip(t, "kept", "README.md")
	for name, original := range map[string][]byte{"a package": unsigned, "a package with a comment": commented} {
		t.Run(name, func(t *testing.T) {
			signed := signNupkg(t, original, ".signature.p7s")
			if bytes.Equal(signed, original) {
				t.Fatal("signing changed nothing")
			}
			got, err := nugetUnsigned(signed)
			if err != nil || !bytes.Equal(got, original) {
				t.Errorf("unsigned %d bytes (%v), want the %d signed", len(got), err, len(original))
			}
		})
	}
	unchanged := map[string][]byte{
		"an unsigned package": unsigned,
		"a look-alike entry":  signNupkg(t, unsigned, ".SIGNATURE.P7S"),
	}
	for name, blob := range unchanged {
		t.Run(name, func(t *testing.T) {
			if got, err := nugetUnsigned(blob); err != nil || !bytes.Equal(got, blob) {
				t.Errorf("a package without the signature entry changed (%v)", err)
			}
		})
	}

	signed := signNupkg(t, unsigned, ".signature.p7s")
	le := binary.LittleEndian
	eocd := bytes.LastIndex(signed, []byte("PK\x05\x06"))
	damaged := bytes.Clone(signed)
	copy(damaged[le.Uint32(signed[eocd+16:]):], "XXXX")
	resized := bytes.Clone(signed)
	le.PutUint32(resized[eocd+12:], le.Uint32(signed[eocd+12:])+1)
	refusals := []struct {
		name, want string
		blob       []byte
	}{
		{"not a zip", "no end-of-central-directory record", []byte("not a package")},
		{"an end record cut short", "no end-of-central-directory record", signed[:len(signed)-4]},
		{"bytes after the end record", "bytes follow", append(bytes.Clone(signed), "junk"...)},
		{"a damaged central record", "no central directory record at offset", damaged},
		{"a directory of another size", "not the size its end record says", resized},
		{"two signatures", "more than one .signature.p7s", signNupkg(t, signed, ".signature.p7s")},
		{"a signature before another entry", "not the last local entry", unsignedZip(t, "", ".signature.p7s", "README.md")},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := nugetUnsigned(tc.blob); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("unsigning: %v, want %q", err, tc.want)
			}
		})
	}
}

// provenanceDocument is a PEP 740 provenance of one bundle.
func provenanceDocument(t *testing.T, publisher map[string]any, statements ...string) []byte {
	t.Helper()
	attestations := []any{}
	for _, statement := range statements {
		attestations = append(attestations, map[string]any{"envelope": map[string]any{"statement": statement}})
	}
	doc, err := json.Marshal(map[string]any{"version": 1, "attestation_bundles": []any{
		map[string]any{"publisher": publisher, "attestations": attestations},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// encodedStatement is an in-toto statement as base64 of its JSON.
func encodedStatement(t *testing.T, predicate string, subjects ...[2]string) string {
	t.Helper()
	list := []any{}
	for _, s := range subjects {
		list = append(list, map[string]any{"name": s[0], "digest": map[string]any{"sha256": s[1]}})
	}
	body, err := json.Marshal(map[string]any{"_type": inTotoStatement, "predicateType": predicate, "subject": list})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(body)
}

// TestCheckPypiProvenance_NamesEachDepartureOnce holds each way a
// provenance can depart from release.yml's to one finding naming it, read
// once with no wait, and the provenance it signed to a pass.
func TestCheckPypiProvenance_NamesEachDepartureOnce(t *testing.T) {
	const wheel = "jmrplens_gitlab_mcp_server-1.0.0-py3-none-win_amd64.whl"
	digest := strings.Repeat("e", 64)
	zero := strings.Repeat("0", 64)
	ours := func(changes map[string]any) map[string]any {
		publisher := map[string]any{"kind": "GitHub", "repository": "jmrplens/gitlab-mcp-server", "workflow": "release.yml", "environment": "release"}
		maps.Copy(publisher, changes)
		return publisher
	}
	good := encodedStatement(t, pypiPublishType, [2]string{wheel, digest})
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{"the one release.yml signed", provenanceDocument(t, ours(nil), good), ""},
		{"no provenance at all", nil, "PyPI serves no provenance for it"},
		{"a body that is not JSON", []byte("<html>"), "cannot be read"},
		{"another version", []byte(`{"version": 2, "attestation_bundles": []}`), "is not a version 1 provenance"},
		{"a bundle without a publisher", []byte(`{"version": 1, "attestation_bundles": [{"attestations": []}]}`), "names no publisher"},
		{"a statement that is not base64", provenanceDocument(t, ours(nil), "not base64!"), "cannot be read"},
		{"a statement that is a list", provenanceDocument(t, ours(nil), base64.StdEncoding.EncodeToString([]byte("[1]"))), "is not a JSON object"},
		{"another repository", provenanceDocument(t, ours(map[string]any{"repository": "someone/else"}), good), "names GitHub someone/else release.yml (environment release)"},
		{"no environment", provenanceDocument(t, ours(map[string]any{"environment": nil}), good), "release.yml (environment None)"},
		{"other bytes", provenanceDocument(t, ours(nil), encodedStatement(t, pypiPublishType, [2]string{wheel, zero})), wheel + " at sha256 " + zero},
		{"another predicate", provenanceDocument(t, ours(nil), encodedStatement(t, "https://slsa.dev/provenance/v1", [2]string{wheel, digest})), "https://slsa.dev/provenance/v1"},
		{"a second subject", provenanceDocument(t, ours(nil), encodedStatement(t, pypiPublishType, [2]string{wheel, digest}, [2]string{"other.whl", digest})), "other.whl"},
		{"no attestation", provenanceDocument(t, ours(nil)), "it states nothing"},
	}
	provenanceURL := pypiIntegrity + pypiDist + "/1.0.0/" + wheel + "/provenance"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string][]json.RawMessage{}
			if tc.body != nil {
				encoded, _ := json.Marshal(base64.StdEncoding.EncodeToString(tc.body))
				answers[provenanceURL] = []json.RawMessage{encoded}
			}
			registries := &scriptedRegistries{t: t, answers: answers}
			var out bytes.Buffer
			c := newFakeClock()
			v := &verifyPublished{
				version: "1.0.0", stdout: &out, client: &http.Client{Transport: registries},
				budget: &retryBudget{sleep: c.sleep},
			}
			v.checkPypiProvenance(map[string]string{wheel: digest})
			if len(registries.calls) != 1 || len(c.slept) > 0 {
				t.Errorf("asked %v and slept %v, want one look", registries.calls, c.slept)
			}
			switch {
			case tc.want == "" && len(v.problems) > 0:
				t.Errorf("the provenance release.yml signed failed: %v", v.problems)
			case tc.want != "" && (len(v.problems) != 1 || !strings.HasPrefix(v.problems[0], "pypi "+wheel+": ") || !strings.Contains(v.problems[0], tc.want)):
				t.Errorf("problems %q, want one naming %q", v.problems, tc.want)
			}
		})
	}
}

// servedBody is one recorded answer: a body.
func servedBody(t *testing.T, body []byte) []json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(base64.StdEncoding.EncodeToString(body))
	if err != nil {
		t.Fatal(err)
	}
	return []json.RawMessage{encoded}
}

// TestVerifyPublished_DownloadsItCannotRead_AreNamed holds what a download
// that arrives but cannot be read reports: npm metadata naming no tarball
// and a tarball that is not gzip, a wheel that is not a zip or carries no
// binary, a runtime package carrying none, and a package that cannot be
// unsigned or has no attested digest.
func TestVerifyPublished_DownloadsItCannotRead_AreNamed(t *testing.T) {
	newRun := func(answers map[string][]json.RawMessage) (*verifyPublished, *scriptedRegistries) {
		registries := &scriptedRegistries{t: t, answers: answers}
		return &verifyPublished{
			version: "1.0.0", stdout: &bytes.Buffer{}, client: &http.Client{Transport: registries},
			budget: &retryBudget{},
		}, registries
	}
	linux := releasePlatforms[0]
	meta := npmRegistry + strings.ReplaceAll(strings.ReplaceAll(npmPlatformName(linux), "@", "%40"), "/", "%2F") + "/1.0.0"
	tarball := "https://registry.npmjs.org/tarball.tgz"
	npmCases := map[string]struct {
		answers map[string][]json.RawMessage
		want    string
	}{
		"no tarball":  {map[string][]json.RawMessage{meta: servedBody(t, []byte(`{"dist": {}}`))}, "answers no dist.tarball"},
		"not gzip":    {map[string][]json.RawMessage{meta: servedBody(t, []byte(`{"dist": {"tarball": "`+tarball+`"}}`)), tarball: servedBody(t, []byte("plain"))}, "EOF"},
		"bad request": {map[string][]json.RawMessage{meta: {json.RawMessage("400")}}, "HTTP 400 Bad Request"},
	}
	for name, tc := range npmCases {
		t.Run("npm "+name, func(t *testing.T) {
			v, _ := newRun(tc.answers)
			if _, _, _, err := v.npmPackage(linux); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("npm package: %v, want %q", err, tc.want)
			}
		})
	}

	wheelURL := "https://files.pythonhosted.org/w.whl"
	wheelCases := map[string]struct {
		blob []byte
		want string
	}{
		"not a zip":  {[]byte("plain"), "not a valid zip file"},
		"no binary":  {unsignedZip(t, "", "README.md"), "ships no binary under"},
		"no license": {unsignedZip(t, "", pypiNorm+"-1.0.0.data/scripts/README"), "ships no binary under"},
	}
	for name, tc := range wheelCases {
		t.Run("wheel "+name, func(t *testing.T) {
			v, _ := newRun(map[string][]json.RawMessage{wheelURL: servedBody(t, tc.blob)})
			if _, _, _, err := v.wheelContents(wheelURL); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("wheel: %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("a runtime package without its binary", func(t *testing.T) {
		if _, err := nugetBinary(unsignedZip(t, "", "README.md"), "linux-x64", "url"); err == nil || !strings.Contains(err.Error(), "ships no gitlab-mcp-server binary") {
			t.Errorf("nuget binary: %v", err)
		}
		if _, err := nugetBinary([]byte("plain"), "linux-x64", "url"); err == nil {
			t.Error("a package that is not a zip was read")
		}
	})

	t.Run("attested packages that cannot be judged", func(t *testing.T) {
		v, _ := newRun(nil)
		v.checkAttestedNupkg("linux-x64", []byte("x"), "url", "a.nupkg", map[string]string{})
		v.checkAttestedNupkg("linux-x64", []byte("plain"), "url", "a.nupkg", map[string]string{"a.nupkg": "0"})
		if len(v.problems) != 2 || !strings.Contains(v.problems[0], "recorded no attested digest") ||
			!strings.Contains(v.problems[1], "cannot be unsigned the way NuGet defines it") {
			t.Errorf("problems %q", v.problems)
		}
	})

	t.Run("statements described the way the script did", func(t *testing.T) {
		got := describeStatement(map[string]any{"predicateType": true, "subject": []any{"loose", 1.5}})
		if got != "True of 'loose', 1.5" {
			t.Errorf("described %q", got)
		}
		if empty := describeStatement(map[string]any{}); empty != "None of no subject" {
			t.Errorf("described %q", empty)
		}
		if pyText(false) != "False" {
			t.Error("false is not spelled as Python spells it")
		}
	})
}
