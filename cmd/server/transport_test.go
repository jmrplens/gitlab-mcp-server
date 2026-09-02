package main

import (
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
			if _, err := os.Stat("/dev/zero"); err != nil {
				t.Skipf("/dev/zero is not available: %v", err)
			}
			previousStdin, previousDevNull := stdinStat, devNullStat
			t.Cleanup(func() { stdinStat, devNullStat = previousStdin, previousDevNull })
			if tc.breakStdin {
				stdinStat = func() (os.FileInfo, error) { return nil, os.ErrPermission }
			} else {
				stdinStat = func() (os.FileInfo, error) { return os.Stat("/dev/zero") }
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

// TestResolveTransport_SelectorPrecedence verifies that --http keeps exactly
// the meaning it always had, that --transport wins when both are given, and
// that an unknown value is refused rather than guessed at.
//
// The precedence matters more than it looks: --http is in every existing
// deployment, so a change in what it means would be a silent breaking change
// for all of them. An empty --transport therefore defers to it completely.
func TestResolveTransport_SelectorPrecedence(t *testing.T) {
	withStdin(t, os.DevNull)

	for _, tc := range []struct {
		name      string
		transport string
		useHTTP   bool
		httpSet   bool
		wantHTTP  bool
		wantErr   bool
	}{
		{name: "unset defers to --http false", transport: "", useHTTP: false, wantHTTP: false},
		{name: "unset defers to --http true", transport: "", useHTTP: true, httpSet: true, wantHTTP: true},
		{name: "stdio is obeyed", transport: "stdio", wantHTTP: false},
		{name: "http is obeyed", transport: "http", wantHTTP: true},
		{name: "stdio overrides an explicit --http", transport: "stdio", useHTTP: true, httpSet: true, wantHTTP: false},
		{name: "http overrides an explicit --http=false", transport: "http", useHTTP: false, httpSet: true, wantHTTP: true},
		{name: "auto infers, here from the null device", transport: "auto", wantHTTP: true},
		{name: "case and spacing are forgiven", transport: "  AUTO ", wantHTTP: true},
		{name: "an unknown selector is refused", transport: "tcp", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotHTTP, err := resolveTransport(tc.transport, tc.useHTTP, tc.httpSet)
			if tc.wantErr {
				if err == nil {
					t.Fatal("resolveTransport() error = nil, want an error naming the accepted values")
				}
				for _, want := range []string{transportStdio, transportHTTP, transportAuto} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTransport() error = %v", err)
			}
			if gotHTTP != tc.wantHTTP {
				t.Errorf("resolveTransport(%q, %v) = %v, want %v", tc.transport, tc.useHTTP, gotHTTP, tc.wantHTTP)
			}
		})
	}
}
