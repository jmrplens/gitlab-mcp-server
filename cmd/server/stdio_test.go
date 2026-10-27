// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
		// Padded and re-indented by a client that formats its output. The
		// whitespace after the message is not handed on, since the SDK's read
		// loop ends the session on anything after a value but a line break;
		// the message itself is.
		`  {"jsonrpc":"2.0","id":3,"method":"ping"}  `,
	}

	in := strings.NewReader(strings.Join(messages, "\n") + "\n")
	var out bytes.Buffer
	reader, _ := resilientStdio(in, &out)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the filtered stream: %v", err)
	}
	framed := make([]string, len(messages))
	for i, message := range messages {
		framed[i] = strings.TrimRight(message, " ")
	}
	if want := strings.Join(framed, "\n") + "\n"; string(got) != want {
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
		{
			// With no method it is no request, and its id may be one this
			// server gave a request of its own.
			name:     "JSON with the wrong version tag and no method answers null",
			line:     `{"jsonrpc":"1.0","id":5}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			// A boolean is no id JSON-RPC allows, so it is not echoed: a
			// refusal carrying it would itself be an invalid response.
			name:     "JSON with the wrong version tag and a boolean id answers null",
			line:     `{"jsonrpc":"1.0","id":true}`,
			wantCode: -32600,
			wantID:   "null",
		},
		// The frames below carry "jsonrpc":"2.0" and are still ones the SDK's
		// decoder rejects, which ends a stdio session as surely as a line that
		// is not JSON: its read loop breaks on any error from the decode.
		{
			name:     "a boolean id",
			line:     `{"jsonrpc":"2.0","id":true,"method":"ping"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "an object id",
			line:     `{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "an array id",
			line:     `{"jsonrpc":"2.0","id":[1],"method":"ping"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "a method that is not a string keeps its id",
			line:     `{"jsonrpc":"2.0","id":1,"method":5}`,
			wantCode: -32600,
			wantID:   "1",
		},
		{
			// Neither a request, having no method, nor a response, having no
			// id to answer.
			name:     "an object carrying only the version tag",
			line:     `{"jsonrpc":"2.0"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			// encoding/json matches keys without regard to case and the SDK's
			// decoder does not, so this reads as version 2.0 to the probe and
			// as no version at all to the SDK.
			name:     "the version key spelled in another case keeps its id",
			line:     `{"JSONRPC":"2.0","id":5,"method":"ping"}`,
			wantCode: -32600,
			wantID:   "5",
		},
		// A frame with no method is a response, and its id is one this
		// server chose for a request of its own. Echoing it would answer
		// that request with this refusal and fail a call that has nothing to
		// do with the bad frame, so the refusal carries null.
		{
			name:     "a response whose error is not an object answers null",
			line:     `{"jsonrpc":"2.0","id":3,"error":"oops"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			name:     "a response whose error code is not a number answers null",
			line:     `{"jsonrpc":"2.0","id":3,"result":{},"error":{"code":"x","message":"m"}}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			// The SDK's decoder refuses this frame for its id, a number no
			// float64 holds, so that id is not one to answer with either.
			name:     "an id no number can hold answers null",
			line:     `{"jsonrpc":"2.0","id":1e400,"method":"ping"}`,
			wantCode: -32600,
			wantID:   "null",
		},
		{
			// JSON on the line after the message is no message of its own:
			// the SDK's read loop ends the session on whatever follows a
			// value other than a line break.
			name:     "a message followed by more text on its line",
			line:     `{"jsonrpc":"2.0","id":4,"method":"ping"} trailing`,
			wantCode: -32700,
			wantID:   "null",
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

// TestResilientStdio_PassesWhatTheSDKDecoderAccepts pins the other half of
// asking the SDK's decoder: a frame it accepts is the SDK's to answer, however
// odd, so the filter hands it over byte for byte and says nothing. Each of
// these survives the SDK's read and is answered or ignored by the SDK itself:
// params of the wrong type are answered -32602, a string id is answered, a
// null id is a notification, and an id with no method is a response nobody is
// waiting for. The last repeats the version key in another case, which
// encoding/json reads as the later value and the SDK's decoder, matching case,
// does not read at all: the filter's own reading of a frame is never what
// decides whether it passes.
func TestResilientStdio_PassesWhatTheSDKDecoderAccepts(t *testing.T) {
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":"x"}`,
		`{"jsonrpc":"2.0","id":"abc","method":"ping"}`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1}`,
		`{"jsonrpc":"2.0","JSONRPC":"1.0","id":1,"method":"ping"}`,
	} {
		t.Run(line, func(t *testing.T) {
			var out bytes.Buffer
			reader, _ := resilientStdio(strings.NewReader(line+"\n"), &out)

			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			if string(got) != line+"\n" {
				t.Errorf("forwarded %q, want the line unchanged", got)
			}
			if out.Len() != 0 {
				t.Errorf("a frame the SDK decodes was answered by the filter: %q", out.String())
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
// JSON-RPC allows a string, number or null id, so a refusal echoing an object,
// array or boolean id would itself be an invalid response. A number no float64
// holds is no id the SDK can read back either, and an absent id is nothing to
// echo.
func TestScalarID_ObjectsAndArraysAreNotEchoed(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]string{
		`7`:        `7`,
		`1.5`:      `1.5`,
		`"seven"`:  `"seven"`,
		`null`:     `null`,
		`{"a":1}`:  ``,
		`[1,2]`:    ``,
		` {"a":1}`: ``,
		`true`:     ``,
		`false`:    ``,
		` true`:    ``,
		`1e400`:    ``,
		``:         ``,
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

// TestSanitizedInput_AFinalLineWithoutANewlineIsHeldToTheSameCap covers the
// one line whose length carries no delimiter to discount. Only a newline is
// framing, so a final line the client never terminated is charged every byte
// it has: at the cap it is delivered, one past it is refused, exactly as the
// terminated lines above are.
func TestSanitizedInput_AFinalLineWithoutANewlineIsHeldToTheSameCap(t *testing.T) {
	for _, tc := range []struct {
		name        string
		line        string
		wantRefused bool
	}{
		{"at_the_cap", `{"jsonrpc":"2.0","id":22,"method":"ping"}`, false},
		{"one_over_the_cap", `{"jsonrpc":"2.0","id":222,"method":"ping"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			reader, _ := resilientStdioWith(strings.NewReader(tc.line), &out,
				stdioLimits{maxLineBytes: 41, maxDepth: maxInboundJSONDepth})

			forwarded, err := io.ReadAll(reader)
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("reading: %v", err)
			}
			if delivered := string(forwarded) == tc.line; delivered == tc.wantRefused {
				t.Errorf("a final %d-byte line under a 41-byte cap was delivered = %v, want %v", len(tc.line), delivered, !tc.wantRefused)
			}
			if tc.wantRefused {
				assertJSONRPCRefusal(t, out.Bytes(), -32600, "41")
			}
		})
	}
}

// TestRefuseUnreadable_ZeroDepthDisablesTheCheck pins what the zero value of
// maxDepth means, as its field says: no depth check at all, rather than a
// ceiling of nothing that every object exceeds.
func TestRefuseUnreadable_ZeroDepthDisablesTheCheck(t *testing.T) {
	if refusal, refused := refuseUnreadable(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"a":[1]}}`, 0); refused {
		t.Errorf("a depth of zero refused an ordinary message with %q", refusal)
	}
}

// TestRefuseUnreadable_ABlankLineIsDroppedSilently pins the contract the
// function's comment states for framing: a blank line is dropped with no
// answer, whatever whitespace it is made of, the empty line included. Read
// happens never to pass the empty one, and the function must not depend on
// that, since a line is blank to a client for the same reason either way.
func TestRefuseUnreadable_ABlankLineIsDroppedSilently(t *testing.T) {
	for _, line := range []string{"", " ", "\t\r\n ", "\r"} {
		t.Run(strconv.Quote(line), func(t *testing.T) {
			refusal, refused := refuseUnreadable(line, maxInboundJSONDepth)
			if !refused || refusal != nil {
				t.Errorf("refuseUnreadable(%q) = (%q, %v), want it dropped with no answer", line, refusal, refused)
			}
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
	// The refusal says how to get past it, which a client whose legitimate
	// message is simply large has no other way to learn.
	assertJSONRPCRefusal(t, out.Bytes(), -32600, "GITLAB_MCP_STDIO_MAX_LINE_BYTES")
}

// TestRefuseUnreadable_RefusesOverNestedLines verifies that a line whose JSON
// nests past the ceiling is refused before it is parsed.
//
// Until go-sdk v1.8.0 capped its own decode at 1000 levels, stdio's only
// protection was the standard library's nesting cap of 10000, reached only by
// the parse this function performs first: below it, a tools/call nested 9000
// deep cost 1.57 seconds of CPU in the SDK's decode. The explicit ceiling is
// what makes the bound deliberate, and it also covers params._meta, which the
// SDK decodes into a map for every method before any middleware could refuse
// it.
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

// TestStdioLimitsFromEnv_ReadsTheDocumentedVariable sets the line cap under
// the name the configuration reference and the refusal message give an
// operator, spelled out rather than through the constant the reader uses, so
// a renamed constant cannot take the documented name with it unnoticed.
func TestStdioLimitsFromEnv_ReadsTheDocumentedVariable(t *testing.T) {
	t.Setenv("GITLAB_MCP_STDIO_MAX_LINE_BYTES", "65536")

	if got := stdioLimitsFromEnv().maxLineBytes; got != 65536 {
		t.Errorf("maxLineBytes = %d with GITLAB_MCP_STDIO_MAX_LINE_BYTES=65536, want 65536", got)
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

// TestSanitizedInput_MaxLineBytes_DefaultsWhenUnconfigured pins the ceiling a
// reader built without limits enforces: the default, not zero, since a zero
// ceiling would refuse every line.
func TestSanitizedInput_MaxLineBytes_DefaultsWhenUnconfigured(t *testing.T) {
	t.Parallel()
	reader, _ := resilientStdioWith(strings.NewReader(""), &bytes.Buffer{}, stdioLimits{})
	if got := reader.maxLineBytes(); got != defaultMaxStdioLineBytes {
		t.Errorf("maxLineBytes() = %d with no limit configured, want the default %d", got, defaultMaxStdioLineBytes)
	}
	configured, _ := resilientStdioWith(strings.NewReader(""), &bytes.Buffer{}, stdioLimits{maxLineBytes: 512})
	if got := configured.maxLineBytes(); got != 512 {
		t.Errorf("maxLineBytes() = %d, want the configured 512", got)
	}
}

// TestErrorLine_AnIDThatIsNotJSON_WritesNothing covers the one way the answer
// can fail to marshal: a raw id that is not valid JSON, which the encoder
// refuses rather than copying through. Every caller passes an id scalarID
// has already vetted, so the case is a contract check on the only fallback:
// silence, since a half-formed line on the protocol stream would be worse
// than no answer.
func TestErrorLine_AnIDThatIsNotJSON_WritesNothing(t *testing.T) {
	t.Parallel()
	if got := errorLine(json.RawMessage("{"), -32600, "refused"); got != nil {
		t.Errorf("errorLine with an unmarshalable id wrote %q, want nothing", got)
	}
	if got := errorLine(json.RawMessage(`7`), -32600, "refused"); !strings.HasSuffix(string(got), "\n") || !strings.Contains(string(got), `"id":7`) {
		t.Errorf("errorLine with a valid id wrote %q, want a newline-terminated answer carrying the id", got)
	}
}

// TestResilientStdio_WhatItPassesTheSDKReads hands the filtered stream to the
// SDK's own stdio connection, which is the judge that matters: an error from
// its read ends the session, whatever the filter made of the line.
//
// A message with whitespace after it on its line is valid JSON and a valid
// message, and it still ended the session, because the read loop takes the
// byte after the value and accepts nothing there but a line break. The filter
// hands such a line on without that whitespace.
func TestResilientStdio_WhatItPassesTheSDKReads(t *testing.T) {
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`  {"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}  `,
		"{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\t\r",
		`{"jsonrpc":"2.0","JSONRPC":"1.0","id":1,"method":"ping"}`,
	} {
		t.Run(strconv.Quote(line), func(t *testing.T) {
			var out bytes.Buffer
			reader, _ := resilientStdio(strings.NewReader(line+"\n"), &out)

			msgs, err := readThroughSDK(t, reader)
			if err != nil {
				t.Fatalf("the filter passed %q and the SDK's reader refused it, which ends a stdio session: %v", line, err)
			}
			if len(msgs) != 1 {
				t.Fatalf("the SDK read %d messages from %q, want the one ping", len(msgs), line)
			}
			if req, ok := msgs[0].(*jsonrpc.Request); !ok || req.Method != "ping" {
				t.Errorf("the SDK read %#v, want the ping", msgs[0])
			}
			if out.Len() != 0 {
				t.Errorf("a message the SDK reads was answered by the filter: %q", out.String())
			}
		})
	}
}

// FuzzRefuseUnreadable holds the stdio filter to the reader behind it, for any
// one line, in both directions.
//
// What the filter passes, the SDK's stdio connection reads as one message and
// nothing else, since an error there ends the session. What it drops, it
// answers with one JSON-RPC error whose id is null or the sender's own, and
// never the id of a response, which is the id of a request this server sent.
// And it drops no line that connection would have read as one message, with
// two exceptions that are the filter's own: a line nested past its depth
// ceiling, and a JSON-RPC batch, which it refuses whatever revision was
// negotiated because it cannot see which one was, and the SDK ends the session
// on a batch from 2025-06-18 on. A line holding several values is no frame of
// this transport, and the SDK's own check for one is not dependable: it looks
// at the byte after a value only when its decoder already holds it, so the
// next value can be read as a message of its own or end the session one read
// later.
func FuzzRefuseUnreadable(f *testing.F) {
	for _, seed := range []string{
		``,
		`   `,
		`{not json`,
		`{"jsonrpc":"2.0","id":1,"meth`,
		`["jsonrpc","2.0"]`,
		`"hello"`,
		`42`,
		`null`,
		`{"hello":"world"}`,
		`{"jsonrpc":"1.0","id":7,"method":"tools/list"}`,
		`{"jsonrpc":"1.0","id":true}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`  {"jsonrpc":"2.0","id":1,"method":"ping"}  `,
		"{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\t\r",
		`{"jsonrpc":"2.0","id":1,"method":"ping"} trailing`,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		// Found by this fuzzer: the SDK reads the object as a message and
		// ends the session on the 0 at its next read.
		`{"jsonrpc":"2.0","id":0,"":{},"00000000000":"","00000000000":""}0`,
		`{"jsonrpc":"2.0","id":true,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`,
		`{"jsonrpc":"2.0","id":[1],"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1e400,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"method":5}`,
		`{"jsonrpc":"2.0"}`,
		`{"JSONRPC":"2.0","id":5,"method":"ping"}`,
		`{"jsonrpc":"2.0","JSONRPC":"1.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":"x"}`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1}`,
		`{"jsonrpc":"2.0","id":3,"error":"oops"}`,
		`{"jsonrpc":"2.0","id":3,"result":{},"error":{"code":"x","message":"m"}}`,
		`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`,
		`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"_meta":` + strings.Repeat("[", 120) + strings.Repeat("]", 120) + `}}`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, line string) {
		if strings.ContainsRune(line, '\n') {
			t.Skip("one line at a time: the newline is the framing")
		}
		var out bytes.Buffer
		reader, _ := resilientStdioWith(strings.NewReader(line+"\n"), &out, stdioLimits{maxDepth: maxInboundJSONDepth})

		msgs, err := readThroughSDK(t, reader)
		if err != nil {
			t.Fatalf("the filter passed %q and the SDK's reader refused it, which ends a stdio session: %v", line, err)
		}
		if len(msgs) > 1 {
			t.Fatalf("the filter passed %q, which the SDK reads as %d messages", line, len(msgs))
		}
		if len(msgs) == 1 {
			if out.Len() != 0 {
				t.Errorf("the filter both passed %q and answered it: %q", line, out.String())
			}
			return
		}

		frame := []byte(trimSpace(line))
		if len(frame) == 0 {
			if out.Len() != 0 {
				t.Errorf("a blank line was answered: %q", out.String())
			}
			return
		}
		assertAnswersWithoutTakingAResponseID(t, frame, out.Bytes())

		if frame[0] == '[' || toolutil.ExceedsJSONDepth(frame, maxInboundJSONDepth) {
			return
		}
		if raw, rawErr := readThroughSDK(t, io.NopCloser(strings.NewReader(line+"\n"))); rawErr == nil && len(raw) == 1 {
			t.Errorf("the filter refused %q, which the SDK reads as the one message %#v", line, raw[0])
		}
	})
}

// readThroughSDK reads everything in holds through the SDK's own stdio
// connection, the code a stdio session's read loop runs, and returns the
// messages it read and the error it stopped on, nil when it reached the end of
// the input. Any other error is what ends a session.
func readThroughSDK(t *testing.T, in io.ReadCloser) ([]jsonrpc.Message, error) {
	t.Helper()
	conn, err := (&mcp.IOTransport{Reader: in, Writer: &lockedWriter{w: io.Discard}}).Connect(t.Context())
	if err != nil {
		t.Fatalf("connecting the SDK's stdio transport: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var msgs []jsonrpc.Message
	for {
		msg, readErr := conn.Read(t.Context())
		if errors.Is(readErr, io.EOF) {
			return msgs, nil
		}
		if readErr != nil {
			return msgs, readErr
		}
		msgs = append(msgs, msg)
	}
}

// assertAnswersWithoutTakingAResponseID checks that refusal is one JSON-RPC
// error answering frame: version 2.0, a parse or invalid request code, and an
// id that is null unless frame is a request, in which case the SDK itself can
// read the refusal back, which no id it cannot hold would let it do.
func assertAnswersWithoutTakingAResponseID(t *testing.T, frame, refusal []byte) {
	t.Helper()
	if bytes.Count(refusal, []byte("\n")) != 1 || !bytes.HasSuffix(refusal, []byte("\n")) {
		t.Fatalf("the refusal of %q is not one newline-terminated line: %q", frame, refusal)
	}
	var answer struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(refusal, &answer); err != nil {
		t.Fatalf("the refusal of %q is not JSON: %q (%v)", frame, refusal, err)
	}
	if answer.JSONRPC != "2.0" || (answer.Error.Code != -32700 && answer.Error.Code != -32600) {
		t.Errorf("the refusal of %q is not a JSON-RPC parse or invalid request error: %q", frame, refusal)
	}
	if string(answer.ID) == "null" {
		return
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(frame, &members); err != nil {
		t.Fatalf("the refusal of %q, which is no JSON object, carries the id %s", frame, answer.ID)
	}
	if _, isRequest := members["method"]; !isRequest {
		t.Errorf("the refusal of %q, which is no request, carries the id %s of a request of this server's", frame, answer.ID)
	}
	if _, err := jsonrpc.DecodeMessage(refusal); err != nil {
		t.Errorf("the refusal of %q carries an id the SDK cannot read back: %q (%v)", frame, refusal, err)
	}
}
