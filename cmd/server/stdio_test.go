package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// TestResilientStdio_PassesValidMessagesThroughUnchanged is the property that
// matters most, because the filter sits on the protocol's critical path.
//
// Everything else it does is a refinement; getting this wrong breaks every
// call. A valid message must arrive at the SDK byte for byte, including one
// large enough to be interesting — a package publish carries its file inline as
// base64, and a filter that truncated one would turn a working call into
// exactly the failure it exists to prevent.
func TestResilientStdio_PassesValidMessagesThroughUnchanged(t *testing.T) {
	big := strings.Repeat("QUJDREVG", 200_000) // ~1.6 MB of base64
	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"x","arguments":{"content_base64":"` + big + `"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		// Padded and re-indented by a client that formats its output.
		`  {"jsonrpc":"2.0","id":3,"method":"ping"}  `,
	}

	in := strings.NewReader(strings.Join(messages, "\n") + "\n")
	var out bytes.Buffer
	reader, _ := resilientStdio(in, &out)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the filtered stream: %v", err)
	}
	if want := strings.Join(messages, "\n") + "\n"; string(got) != want {
		if len(got) != len(want) {
			t.Fatalf("the filtered stream is %d bytes, want %d: a valid message was altered or truncated", len(got), len(want))
		}
		t.Fatal("the filtered stream differs from the input, though the lengths match")
	}
	if out.Len() != 0 {
		t.Errorf("valid input produced output on the response stream: %q", out.String())
	}
}

// TestResilientStdio_AnswersUnreadableInput pins what each shape of unreadable
// line is answered with, and that none of them reaches the SDK.
func TestResilientStdio_AnswersUnreadableInput(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantCode int
		wantID   string
	}{
		{
			name:     "not JSON",
			line:     `{not json`,
			wantCode: -32700,
			wantID:   "null",
		},
		{
			name:     "truncated mid-key",
			line:     `{"jsonrpc":"2.0","id":1,"meth`,
			wantCode: -32700,
			wantID:   "null",
		},
		{
			// Well-formed JSON that is not a Request object. -32700 would be
			// wrong here: the server parsed it fine, it is the shape that is
			// unusable, and a client told "parse error" looks for a syntax
			// problem it does not have.
			name:     "a JSON array is valid JSON and an invalid request",
			line:     `["jsonrpc","2.0"]`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "a bare JSON string is the same case",
			line:     `"hello"`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "a bare number is the same case",
			line:     `42`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "JSON with no version tag",
			line:     `{"hello":"world"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name: "JSON with the wrong version tag keeps its id",
			line: `{"jsonrpc":"1.0","id":7,"method":"tools/list"}`,
			// The sender gets its id back so it can match the refusal to the
			// request it sent.
			wantCode: -32600,
			wantID:   "7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			reader, _ := resilientStdio(strings.NewReader(tt.line+"\n"), &out)

			forwarded, err := io.ReadAll(reader)
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("reading: %v", err)
			}
			if len(forwarded) != 0 {
				t.Errorf("unreadable input reached the SDK: %q", forwarded)
			}

			var answer struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Error   struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if unmarshalErr := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &answer); unmarshalErr != nil {
				t.Fatalf("the refusal is not JSON: %q (%v)", out.String(), unmarshalErr)
			}
			if answer.JSONRPC != "2.0" {
				t.Errorf("refusal is not JSON-RPC 2.0: %q", out.String())
			}
			if answer.Error.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", answer.Error.Code, tt.wantCode)
			}
			if string(answer.ID) != tt.wantID {
				t.Errorf("id = %s, want %s", answer.ID, tt.wantID)
			}
			if !strings.HasSuffix(out.String(), "\n") {
				t.Error("the refusal is not newline-terminated, so it runs into the next message")
			}
		})
	}
}

// TestResilientStdio_DropsBlankLinesSilently checks that framing is not
// mistaken for content.
//
// A trailing newline, or a client that separates messages with a blank line, is
// not sending anything; answering it would put noise on a stream that is
// behaving correctly.
func TestResilientStdio_DropsBlankLinesSilently(t *testing.T) {
	var out bytes.Buffer
	const valid = `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	reader, _ := resilientStdio(strings.NewReader("\n\n   \n"+valid+"\n\n"), &out)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if strings.TrimSpace(string(got)) != valid {
		t.Errorf("forwarded %q, want just the one real message", got)
	}
	if out.Len() != 0 {
		t.Errorf("blank lines were answered: %q", out.String())
	}
}

// TestResilientStdio_RecoversAndKeepsReading is the whole point: a bad line
// must not end the stream.
func TestResilientStdio_RecoversAndKeepsReading(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{not json`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"hello":"world"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	reader, _ := resilientStdio(strings.NewReader(input), &out)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	for _, want := range []string{`"id":1`, `"id":2`, `"id":3`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(string(got), want) {
				t.Errorf("message %s did not survive the bad lines around it:\n%s", want, got)
			}
		})
	}
	if strings.Contains(string(got), "not json") || strings.Contains(string(got), "hello") {
		t.Errorf("a bad line was forwarded:\n%s", got)
	}
	if refusals := strings.Count(strings.TrimSpace(out.String()), "\n") + 1; refusals != 2 {
		t.Errorf("wrote %d refusals, want 2", refusals)
	}
}

// TestLockedWriter_SerializesConcurrentWrites pins the reason the writer is
// shared rather than each side holding its own.
//
// The filter and the SDK both write to stdout. A refusal interleaved into the
// middle of a response would corrupt the very stream the filter exists to
// protect, which would be a worse failure than the one it prevents.
func TestLockedWriter_SerializesConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	w := &lockedWriter{w: &buf}

	const writers, each = 8, 50
	lines := []string{
		strings.Repeat("a", 4096) + "\n",
		strings.Repeat("b", 4096) + "\n",
	}

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(which int) {
			defer wg.Done()
			for range each {
				_, _ = w.Write([]byte(lines[which%len(lines)]))
			}
		}(i)
	}
	wg.Wait()

	for line := range strings.SplitSeq(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if len(line) != 4096 {
			t.Fatalf("a line is %d bytes, want 4096: writes interleaved", len(line))
		}
		if strings.Count(line, string(line[0])) != len(line) {
			t.Fatal("a line mixes content from two writers")
		}
	}
}

// TestScalarID_ObjectsAndArraysAreNotEchoed pins the response's own validity:
// JSON-RPC allows a string, number or null id, so a refusal echoing an object
// id would itself be an invalid response.
func TestScalarID_ObjectsAndArraysAreNotEchoed(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]string{
		`7`:        `7`,
		`"seven"`:  `"seven"`,
		`null`:     `null`,
		`{"a":1}`:  ``,
		`[1,2]`:    ``,
		` {"a":1}`: ``,
	} {
		t.Run(raw, func(t *testing.T) {
			got := string(scalarID(json.RawMessage(raw)))
			if got != want {
				t.Errorf("scalarID(%s) = %q, want %q", raw, got, want)
			}
		})
	}
}
