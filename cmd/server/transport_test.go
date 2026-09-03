package main

import (
	"bytes"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStdin points the transport inference at the named file for one test.
func withStdin(t *testing.T, path string) {
	t.Helper()
	previous := stdinStat
	t.Cleanup(func() { stdinStat = previous })
	stdinStat = func() (os.FileInfo, error) { return os.Stat(path) }
}

// TestTransportStatSeams_ByDefault_ExamineTheRealProcess verifies that the two
// replaceable stats read this process's own standard input and the real null
// device.
//
// It is the one property every other test in this file cannot see: they all
// replace those seams, so a version that read the wrong stream, or a hardcoded
// path instead of the platform's null device, would satisfy all of them while
// inferring the transport from something that is not stdin.
//
// The stdin half asserts by moving os.Stdin and checking the answer follows,
// rather than by comparing against os.Stdin.Stat(). Comparing would be
// circular in the case that matters: a test binary's own stdin is usually the
// null device, so a seam wrongly hardcoded to the null device would agree with
// it and the test would pass on the one machine shape it exists to rule out.
func TestTransportStatSeams_ByDefault_ExamineTheRealProcess(t *testing.T) {
	standIn := filepath.Join(t.TempDir(), "stdin")
	file, err := os.Create(standIn)
	if err != nil {
		t.Fatalf("creating a file to stand in for stdin: %v", err)
	}
	previousStdin := os.Stdin
	t.Cleanup(func() {
		os.Stdin = previousStdin
		_ = file.Close()
	})
	os.Stdin = file

	wantStdin, err := os.Stat(standIn)
	if err != nil {
		t.Fatalf("examining the stand-in: %v", err)
	}
	gotStdin, err := stdinStat()
	if err != nil {
		t.Fatalf("stdinStat() error = %v", err)
	}
	if !os.SameFile(gotStdin, wantStdin) {
		t.Errorf("stdinStat() describes %q, want the file standing in for this process's stdin", gotStdin.Name())
	}

	wantNull, err := os.Stat(os.DevNull)
	if err != nil {
		t.Fatalf("%s could not be examined: %v", os.DevNull, err)
	}
	gotNull, err := devNullStat()
	if err != nil {
		t.Fatalf("devNullStat() error = %v", err)
	}
	if !os.SameFile(gotNull, wantNull) {
		t.Errorf("devNullStat() describes %q, want %s", gotNull.Name(), os.DevNull)
	}
}

// TestInferTransport_StdinShape_DecidesTheTransport verifies that HTTP is
// chosen for exactly one shape of file descriptor 0, the null device, and
// stdio for every other.
//
// The distinction the inference rests on is not "is this a terminal", which
// cannot separate the cases that matter, since a terminal and the null device
// are both character devices. It is whether anybody handed this process a
// stdin at all: a container started without -i, and Compose without
// stdin_open, connect the null device, while the `docker run -i` every MCP
// client configuration uses connects a pipe.
func TestInferTransport_StdinShape_DecidesTheTransport(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(regular, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	fifo := filepath.Join(dir, "pipe")
	hasFIFO := mkfifoForTest(t, fifo)

	for _, tc := range []struct {
		name     string
		path     string
		skip     bool
		wantHTTP bool
		wantWhy  string
	}{
		{
			name:     "the null device means nobody is speaking to this process",
			path:     os.DevNull,
			wantHTTP: true,
			wantWhy:  os.DevNull,
		},
		{
			name:     "a regular file is a shell redirect replaying a session",
			path:     regular,
			wantHTTP: false,
			wantWhy:  "regular file",
		},
		{
			name:     "a pipe is a client",
			path:     fifo,
			skip:     !hasFIFO,
			wantHTTP: false,
			wantWhy:  "pipe",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("this platform has no FIFO to point stdin at")
			}
			withStdin(t, tc.path)
			gotHTTP, why := inferTransport()
			if gotHTTP != tc.wantHTTP {
				t.Errorf("inferTransport() http = %v, want %v (reason: %s)", gotHTTP, tc.wantHTTP, why)
			}
			if !strings.Contains(why, tc.wantWhy) {
				t.Errorf("reason = %q, want it to mention %q", why, tc.wantWhy)
			}
		})
	}
}

// TestInferTransport_CharacterDeviceThatIsNotNull_ChoosesStdio verifies the
// case a terminal takes, which no test can open directly but which shares its
// shape: a character device that is not the null device.
//
// This is the case the naive "is it a character device" check gets wrong, and
// getting it wrong means a person running the binary in a terminal silently
// gets an HTTP listener.
func TestInferTransport_CharacterDeviceThatIsNotNull_ChoosesStdio(t *testing.T) {
	const other = "/dev/zero"
	if _, err := os.Stat(other); err != nil {
		t.Skipf("%s is not available: %v", other, err)
	}
	withStdin(t, other)

	gotHTTP, why := inferTransport()
	if gotHTTP {
		t.Errorf("a character device that is not %s chose HTTP; a terminal has this shape (reason: %s)", os.DevNull, why)
	}
}

// TestInferTransport_UnreadableInputs_ChooseStdio verifies that neither an
// unstattable stdin nor an unstattable null device stops the process, and that
// both fall to stdio.
//
// Stdio is the safer fallback: a stdio server that should have been HTTP fails
// visibly within seconds, while an HTTP listener that should have been stdio
// is a client hanging with no output, which is the defect this whole mechanism
// exists to close.
func TestInferTransport_UnreadableInputs_ChooseStdio(t *testing.T) {
	// A directory, and deliberately not a regular file: the null device is
	// only stat'ed for a stdin that is neither a pipe nor a regular file, so a
	// regular file here would answer before reaching the branch the second
	// case exists to cover. A directory is the one such shape that needs no
	// device node, so both cases run on every platform rather than being
	// skipped wherever /dev/zero is absent.
	neitherPipeNorFile := t.TempDir()

	for _, tc := range []struct {
		name         string
		breakStdin   bool
		breakDevNull bool
		wantWhy      string
	}{
		{
			name:       "stdin cannot be examined",
			breakStdin: true,
			wantWhy:    "stdin could not be examined",
		},
		{
			name:         "the null device cannot be examined",
			breakDevNull: true,
			wantWhy:      "null device could not be examined",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousStdin, previousDevNull := stdinStat, devNullStat
			t.Cleanup(func() { stdinStat, devNullStat = previousStdin, previousDevNull })
			if tc.breakStdin {
				stdinStat = func() (os.FileInfo, error) { return nil, os.ErrPermission }
			} else {
				stdinStat = func() (os.FileInfo, error) { return os.Stat(neitherPipeNorFile) }
			}
			if tc.breakDevNull {
				devNullStat = func() (os.FileInfo, error) { return nil, os.ErrNotExist }
			}

			gotHTTP, why := inferTransport()
			if gotHTTP {
				t.Errorf("an unreadable input chose HTTP; want stdio (reason: %s)", why)
			}
			if !strings.Contains(why, tc.wantWhy) {
				t.Errorf("reason = %q, want it to mention %q", why, tc.wantWhy)
			}
		})
	}
}

// assertRefusalNamesEverySelector checks that a refused --transport value is
// answered with an error listing what would have been accepted.
//
// Naming them is the whole value of refusing rather than guessing: an operator
// who mistyped one selector learns the set from the message instead of the
// documentation.
func assertRefusalNamesEverySelector(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("resolveTransport() error = nil, want an error naming the accepted values")
	}
	for _, want := range []string{transportStdio, transportHTTP, transportAuto} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestResolveTransport_SelectorPrecedence verifies that --http keeps exactly
// the meaning it always had, that --transport wins when both are given and
// says so, and that an unknown value is refused rather than guessed at.
//
// The precedence matters more than it looks: --http is in every existing
// deployment, so a change in what it means would be a silent breaking change
// for all of them. An empty --transport therefore defers to it completely.
//
// The override is asserted alongside the outcome because overriding silently
// is the failure mode worth catching: an operator who passed both selectors
// and got the transport the other one names has no way to find out why.
func TestResolveTransport_SelectorPrecedence(t *testing.T) {
	withStdin(t, os.DevNull)

	for _, tc := range []struct {
		name          string
		transport     string
		useHTTP       bool
		httpSet       bool
		wantHTTP      bool
		wantInference bool
		wantOverride  bool
		wantErr       bool
	}{
		{name: "unset defers to --http false", transport: "", useHTTP: false, wantHTTP: false},
		{name: "unset defers to --http true", transport: "", useHTTP: true, httpSet: true, wantHTTP: true},
		{name: "stdio is obeyed", transport: "stdio", wantHTTP: false},
		{name: "http is obeyed", transport: "http", wantHTTP: true},
		{name: "stdio overrides an explicit --http", transport: "stdio", useHTTP: true, httpSet: true, wantHTTP: false, wantOverride: true},
		{name: "http overrides an explicit --http=false", transport: "http", useHTTP: false, httpSet: true, wantHTTP: true, wantOverride: true},
		{name: "stdio says nothing about an --http nobody passed", transport: "stdio", useHTTP: false, wantHTTP: false},
		{name: "auto infers, here from the null device", transport: "auto", wantHTTP: true, wantInference: true},
		{name: "auto ignores an explicit --http and says so", transport: "auto", useHTTP: false, httpSet: true, wantHTTP: true, wantInference: true, wantOverride: true},
		{name: "case and spacing are forgiven", transport: "  AUTO ", wantHTTP: true, wantInference: true},
		{name: "an unknown selector is refused", transport: "tcp", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := resolveTransport(tc.transport, tc.useHTTP, tc.httpSet)
			if tc.wantErr {
				assertRefusalNamesEverySelector(t, err)
				return
			}
			if err != nil {
				t.Fatalf("resolveTransport() error = %v", err)
			}
			if decision.HTTP != tc.wantHTTP {
				t.Errorf("resolveTransport(%q, %v) = %v, want %v", tc.transport, tc.useHTTP, decision.HTTP, tc.wantHTTP)
			}
			// Only auto has an observation to report; a stated transport that
			// claimed to have inferred one would be lying to the operator.
			if (decision.Inference != "") != tc.wantInference {
				t.Errorf("resolveTransport(%q) reported inference %q, want reported = %v", tc.transport, decision.Inference, tc.wantInference)
			}
			if (decision.Override != "") != tc.wantOverride {
				t.Errorf("resolveTransport(%q, http=%v, set=%v) reported override %q, want reported = %v",
					tc.transport, tc.useHTTP, tc.httpSet, decision.Override, tc.wantOverride)
			}
		})
	}
}

// TestTransportDecision_Explain_SaysWhatItDecidedAndWhy verifies that a
// decision reports the override it applied and the observation it rested on,
// and that a decision with neither says nothing at all.
//
// The explanation is held back rather than logged where the choice is made,
// and that ordering is the reason it is data first: the transport has to be
// settled before the environment files carrying LOG_LEVEL have been read, so
// logging it there put one plain-text line onto a stream that is otherwise
// JSON records, in exactly the configuration the container image ships.
func TestTransportDecision_Explain_SaysWhatItDecidedAndWhy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		decision  transportDecision
		want      []string
		wantQuiet bool
	}{
		{
			name:     "an override is reported to the operator who caused it",
			decision: transportDecision{Override: "--transport=stdio overrides --http"},
			want:     []string{"--transport=stdio overrides --http"},
		},
		{
			name:     "an inferred HTTP transport names what it read",
			decision: transportDecision{HTTP: true, Inference: "stdin is " + os.DevNull},
			want:     []string{"transport inferred from stdin", `"transport":"http"`, "stdin is " + os.DevNull},
		},
		{
			name:     "an inferred stdio transport names what it read",
			decision: transportDecision{HTTP: false, Inference: "stdin is a pipe"},
			want:     []string{"transport inferred from stdin", `"transport":"stdio"`, "stdin is a pipe"},
		},
		{
			name:     "auto over an explicit --http reports both",
			decision: transportDecision{HTTP: true, Override: "--transport=auto ignores --http", Inference: "stdin is " + os.DevNull},
			want:     []string{"--transport=auto ignores --http", "transport inferred from stdin"},
		},
		{
			name:      "a stated transport has nothing to explain",
			decision:  transportDecision{HTTP: true},
			wantQuiet: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			t.Cleanup(func() { slog.SetDefault(previous) })
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))

			tc.decision.explain()

			got := logged.String()
			if tc.wantQuiet && strings.TrimSpace(got) != "" {
				t.Errorf("a decision with nothing to explain logged %q", got)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("the explanation does not carry %q: %q", want, got)
				}
			}
		})
	}
}

// TestFlagWasSet_DistinguishesAPassedFlagFromItsDefault verifies that
// flagWasSet answers on what the operator typed rather than on the value that
// ended up in the variable.
//
// That distinction is the whole reason it asks the flag set: --http=false and
// an absent --http hold the same value and mean different things.
// --transport=http reports that it overrode the first and stays silent about
// the second, so a check comparing values would announce an override to
// everyone who never passed the flag it claims to have overridden.
func TestFlagWasSet_DistinguishesAPassedFlagFromItsDefault(t *testing.T) {
	withFreshFlagSet(t)
	flag.Bool("http", false, "")
	flag.String("transport", "", "")
	if err := flag.CommandLine.Parse([]string{"-http=false"}); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	for _, tc := range []struct {
		name string
		flag string
		want bool
	}{
		{name: "a flag passed with its own default value was still passed", flag: "http", want: true},
		{name: "a flag nobody typed was not", flag: "transport", want: false},
		{name: "a flag that does not exist was not", flag: "no-such-flag", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := flagWasSet(tc.flag); got != tc.want {
				t.Errorf("flagWasSet(%q) = %v, want %v", tc.flag, got, tc.want)
			}
		})
	}
}
