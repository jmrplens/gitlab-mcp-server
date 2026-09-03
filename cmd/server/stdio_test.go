package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
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
			t.Parallel()
			got := string(scalarID(json.RawMessage(raw)))
			if got != want {
				t.Errorf("scalarID(%s) = %q, want %q", raw, got, want)
			}
		})
	}
}

// TestResilientStdio_AFinalLineWithoutANewline_IsStillDelivered covers the read
// that ends with data and an error at once.
//
// A client that writes its last message and closes the pipe without a trailing
// newline is ordinary — several MCP clients do exactly that on shutdown — and
// the reader is handed both the line and io.EOF in the same call. Returning the
// error there would drop a complete, valid request on the floor and end the
// session; the data has to be delivered first, with the EOF surfacing on the
// next read.
func TestResilientStdio_AFinalLineWithoutANewline_IsStillDelivered(t *testing.T) {
	t.Parallel()

	const message = `{"jsonrpc":"2.0","id":9,"method":"ping"}`
	var out bytes.Buffer
	reader, _ := resilientStdio(strings.NewReader(message), &out)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the filtered stream: %v", err)
	}
	if string(got) != message {
		t.Errorf("filtered stream = %q, want the unterminated final message delivered verbatim", got)
	}
	if !reader.clientClosed() {
		t.Error("the reader did not observe the closed input, so the exit status would report a failure")
	}
	if out.Len() != 0 {
		t.Errorf("a valid message produced output on the response stream: %q", out.String())
	}
}

// endlessNonNewline yields a byte that is never a newline, so a line of any
// length can be fed to the reader without allocating it.
type endlessNonNewline struct{}

func (endlessNonNewline) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// TestSanitizedInput_OverlongLineIsRefusedAndTheStreamResynchronises verifies
// that a line longer than the cap is dropped, answered, and followed by
// service as usual.
//
// The accumulator had no ceiling at all: bytes with no newline in them were
// held until one arrived, so writing 1 GiB with no newline took the process
// from 204 MiB resident to 1190 MiB and it was still growing. stdio is also
// the one transport with no body cap of any kind, since MaxRequestBodyBytes is
// a streamable-HTTP setting. Resynchronising rather than dying is the
// behavior this file already implements for unparseable lines, and it is what
// keeps a client's next request served.
func TestSanitizedInput_OverlongLineIsRefusedAndTheStreamResynchronises(t *testing.T) {
	const valid = `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	for _, tc := range []struct {
		name        string
		cap         int
		line        string
		wantRefused bool
	}{
		// Every cap admits the 40 bytes of the trailing well-formed line, so
		// what a row varies is whether the line under test fits. The cap is on
		// the message, not on the framing: a line of exactly cap bytes is
		// within it, which is what the middle two rows pin.
		{"under_the_cap", 512, `{"jsonrpc":"2.0","id":2,"method":"ping"}`, false},
		{"at_the_cap", 41, `{"jsonrpc":"2.0","id":22,"method":"ping"}`, false},
		{"one_over_the_cap", 41, `{"jsonrpc":"2.0","id":222,"method":"ping"}`, true},
		{"far_over_the_cap", 64, strings.Repeat("x", 200_000), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			reader, _ := resilientStdioWith(
				strings.NewReader(tc.line+"\n"+valid+"\n"), &out,
				stdioLimits{maxLineBytes: tc.cap, maxDepth: maxInboundJSONDepth},
			)

			forwarded, err := io.ReadAll(reader)
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("reading: %v", err)
			}
			if !strings.Contains(string(forwarded), valid) {
				t.Errorf("the well-formed line after the refusal never reached the SDK: %q", forwarded)
			}
			if !tc.wantRefused {
				if out.Len() != 0 {
					t.Errorf("a line within the cap was answered with %q", out.String())
				}
				return
			}
			if strings.Contains(string(forwarded), tc.line) {
				t.Errorf("the over-long line reached the SDK: %d bytes forwarded", len(forwarded))
			}
			assertJSONRPCRefusal(t, out.Bytes(), -32600, strconv.Itoa(tc.cap))
		})
	}
}

// TestSanitizedInput_UnterminatedLineIsBounded verifies that a flood with no
// newline in it is refused at the cap instead of being accumulated.
//
// The reader yields non-newline bytes without ever allocating the stream, so a
// failure here is a hang or an out-of-memory rather than a wrong value: the
// point of the cap is that the process does not grow with the input.
func TestSanitizedInput_UnterminatedLineIsBounded(t *testing.T) {
	var out bytes.Buffer
	reader, _ := resilientStdioWith(
		io.LimitReader(endlessNonNewline{}, 8<<20), &out,
		stdioLimits{maxLineBytes: 4096, maxDepth: maxInboundJSONDepth},
	)

	forwarded, err := io.ReadAll(reader)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("reading: %v", err)
	}
	if len(forwarded) != 0 {
		t.Errorf("%d bytes of an unterminated flood reached the SDK", len(forwarded))
	}
	assertJSONRPCRefusal(t, out.Bytes(), -32600, "4096")
}

// TestRefuseUnreadable_RefusesOverNestedLines verifies that a line whose JSON
// nests past the ceiling is refused before it is parsed.
//
// stdio's accidental protection is the standard library's nesting cap of
// 10000, reached only by the pre-parse this function performs: below it, a
// tools/call nested 9000 deep still cost 1.57 seconds of CPU in the SDK's
// decode. The explicit ceiling is what makes the bound deliberate, and it also
// covers params._meta, which the SDK decodes into a map for every method
// before any middleware could refuse it.
func TestRefuseUnreadable_RefusesOverNestedLines(t *testing.T) {
	for _, tc := range []struct {
		name        string
		depth       int
		wantRefused bool
	}{
		{"ordinary_message", 4, false},
		{"at_the_ceiling", maxInboundJSONDepth, false},
		{"one_past_the_ceiling", maxInboundJSONDepth + 1, true},
		{"the_amplification_payload", 9000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"_meta":` +
				strings.Repeat("[", tc.depth-2) + strings.Repeat("]", tc.depth-2) + `}}`
			refusal, refused := refuseUnreadable(line, maxInboundJSONDepth)
			if refused != tc.wantRefused {
				t.Fatalf("refuseUnreadable at depth %d refused = %v, want %v", tc.depth, refused, tc.wantRefused)
			}
			if !tc.wantRefused {
				return
			}
			assertJSONRPCRefusal(t, refusal, -32600, strconv.Itoa(maxInboundJSONDepth))
		})
	}
}

// TestStdioLimitsFromEnv verifies that the line cap is configurable, that a
// value that is not a positive number leaves the default in place, and that
// the default is above the largest legitimate message rather than below it.
func TestStdioLimitsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  int
	}{
		{"unset", "", defaultMaxStdioLineBytes},
		{"explicit", "65536", 65536},
		{"padded", "  65536  ", 65536},
		{"zero_keeps_the_default", "0", defaultMaxStdioLineBytes},
		{"negative_keeps_the_default", "-1", defaultMaxStdioLineBytes},
		{"garbage_keeps_the_default", "plenty", defaultMaxStdioLineBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value != "" {
				t.Setenv(stdioMaxLineBytesEnv, tc.value)
			}
			got := stdioLimitsFromEnv()
			if got.maxLineBytes != tc.want {
				t.Errorf("maxLineBytes = %d, want %d", got.maxLineBytes, tc.want)
			}
			if got.maxDepth != maxInboundJSONDepth {
				t.Errorf("maxDepth = %d, want %d", got.maxDepth, maxInboundJSONDepth)
			}
		})
	}
}

// assertJSONRPCRefusal checks that raw is one newline-terminated JSON-RPC
// error carrying the given code, and that its message names detail — the
// limit the caller passed, so a refusal tells its sender what to change.
func assertJSONRPCRefusal(t *testing.T, raw []byte, wantCode int, detail string) {
	t.Helper()
	if len(raw) == 0 {
		t.Fatal("no refusal was written")
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Error("the refusal is not newline-terminated, so it runs into the next message")
	}
	var answer struct {
		JSONRPC string `json:"jsonrpc"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &answer); err != nil {
		t.Fatalf("the refusal is not JSON: %q (%v)", raw, err)
	}
	if answer.JSONRPC != "2.0" {
		t.Errorf("refusal is not JSON-RPC 2.0: %q", raw)
	}
	if answer.Error.Code != wantCode {
		t.Errorf("code = %d, want %d", answer.Error.Code, wantCode)
	}
	if !strings.Contains(answer.Error.Message, detail) {
		t.Errorf("message = %q, want it to name %s", answer.Error.Message, detail)
	}
}
