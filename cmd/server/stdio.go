// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// resilientStdio builds the reader and writer the stdio transport runs on,
// with unreadable input answered rather than fatal.
//
// The SDK's read loop (internal/jsonrpc2/conn.go, readIncoming) breaks on any
// error from its reader, so a message that fails to parse is handled exactly
// like a closed pipe: the session ends, and on stdio that is the process. The
// client is left with EOF on a stream it can still write to, and nothing is
// written explaining why.
//
// JSON-RPC 2.0 defines error codes for both shapes of this, and the framing
// here is one message per line, so the next line is an independent message and
// resynchronizing costs nothing. This filter does that on the SDK's behalf: a
// line it would choke on is answered and dropped, and the SDK never sees it.
//
// It deliberately does the least it can. A line holding one JSON value is put
// to the SDK's own decoder, jsonrpc.DecodeMessage, which is the function the
// read loop decodes a single message with: a frame it accepts is passed
// through, whatever else is wrong with it, and a frame it rejects is answered
// here, because that rejection is what would end the session. Deciding what a
// valid message means stays the SDK's job, and a filter that wrote down its own
// rules would be a second, divergent implementation of the protocol. The one
// thing a passed line loses is the whitespace after its message, which the
// read loop refuses too (see [frameFor]).
func resilientStdio(in io.Reader, out io.Writer) (*sanitizedInput, io.WriteCloser) {
	return resilientStdioWith(in, out, stdioLimitsFromEnv())
}

// resilientStdioWith is [resilientStdio] with the inbound ceilings named
// explicitly, so a test can pin behavior at a cap it can reach cheaply.
func resilientStdioWith(in io.Reader, out io.Writer, limits stdioLimits) (*sanitizedInput, io.WriteCloser) {
	shared := &lockedWriter{w: out}
	return &sanitizedInput{in: bufio.NewReader(in), out: shared, limits: limits}, shared
}

// stdioLimits bound one inbound line: how long it may be, and how deeply its
// JSON may nest.
type stdioLimits struct {
	// maxLineBytes is the longest line that will be assembled. A longer one
	// is dropped and answered rather than accumulated.
	maxLineBytes int
	// maxDepth is the deepest JSON nesting a line may contain. Zero disables
	// the check.
	maxDepth int
}

// defaultMaxStdioLineBytes is the line ceiling when nothing configures one.
//
// Deliberately above the largest legitimate message rather than below it: a
// package publish carries its file inline as base64, and this matches the
// SDK's own default for an HTTP request body (4 MiB) rather than undercutting
// it, so the two transports refuse the same messages.
const defaultMaxStdioLineBytes = 4 << 20

// stdioMaxLineBytesEnv overrides the line ceiling for a deployment whose
// client genuinely sends larger messages — the inline-base64 case above.
const stdioMaxLineBytesEnv = "GITLAB_MCP_STDIO_MAX_LINE_BYTES"

// stdioMaxLineBytesSetting is the same variable without its prefix, the name
// [config.TrimmedGetenv] reads it under (issue 957); the warning names the
// variable in full, as an operator exports it.
const stdioMaxLineBytesSetting = "STDIO_MAX_LINE_BYTES"

// stdioLimitsFromEnv reads the configured line ceiling, keeping the default
// when the value is missing, unparseable or not positive.
//
// A bad value warns rather than refusing startup: stdio configuration arrives
// through the environment of whatever launched the process, and failing to
// start over one mistyped number would take the client down with it.
func stdioLimitsFromEnv() stdioLimits {
	limits := stdioLimits{maxLineBytes: defaultMaxStdioLineBytes, maxDepth: maxInboundJSONDepth}
	raw := config.TrimmedGetenv(stdioMaxLineBytesSetting)
	if raw == "" {
		return limits
	}
	size, err := strconv.Atoi(raw)
	if err != nil || size <= 0 {
		slog.Warn("stdio line limit could not be parsed; using the default",
			"variable", stdioMaxLineBytesEnv, "value", raw, "default", defaultMaxStdioLineBytes)
		return limits
	}
	limits.maxLineBytes = size
	return limits
}

// lockedWriter serializes writes from the SDK and from the input filter.
//
// Both write to the same stdout, and a refusal interleaved into the middle of a
// response would corrupt the very stream this exists to protect.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// Close is a no-op: stdout is not ours to close.
func (l *lockedWriter) Close() error { return nil }

// sanitizedInput presents stdin to the SDK with unparseable lines removed.
type sanitizedInput struct {
	in     *bufio.Reader
	out    io.Writer
	limits stdioLimits

	// pending holds the remainder of the line currently being handed over.
	// Lines are passed through whole, so the SDK's own framing is unchanged.
	pending []byte

	// closed records that the client closed its end of the pipe.
	//
	// This is the only place that fact is observable. The SDK reports it back
	// as "server is closing: EOF", but the EOF in there is formatted with %v
	// rather than %w and the sentinel it wraps is in an internal package, so
	// neither half survives errors.Is and the caller cannot tell a client
	// hanging up from the transport breaking. Recording it at the read is
	// exact, and cheaper than matching on the message text.
	closed atomic.Bool
}

// clientClosed reports whether stdin reached end of file.
//
// The stdio binding calls that the primary graceful-shutdown signal, so the
// answer is what separates an ordinary stop from a failure.
func (s *sanitizedInput) clientClosed() bool { return s.closed.Load() }

// Read implements io.Reader, yielding only lines the SDK can parse.
//
// A line is assembled rather than scanned with a bufio.Scanner because a
// Scanner caps a token at its buffer size and silently truncates past it,
// which would turn a legitimate large request — a package publish carries its
// file inline as base64 — into the exact failure this is here to prevent. The
// assembly is bounded all the same: a line past the ceiling is dropped up to
// the next newline and answered, which is the resynchronisation this file
// already performs for unparseable lines. Without that bound there was no
// ceiling at all, and a peer that never sent a newline grew the process by the
// size of what it wrote.
func (s *sanitizedInput) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		line, oversize, err := s.readLine()
		// Ifs rather than a tagless switch, here and in readLine, whose case
		// expressions carry no statement counter for the mutation gate to see.
		if oversize {
			_, _ = s.out.Write(errorLine(nil, -32600, fmt.Sprintf(
				"Invalid Request: message exceeds the %d byte limit; send a smaller message or raise %s",
				s.maxLineBytes(), stdioMaxLineBytesEnv,
			)))
		} else if line != "" {
			s.accept(line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.closed.Store(true)
			}
			if len(s.pending) == 0 {
				return 0, err
			}
			break
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

// accept hands a line to the SDK, or answers it here when it is one the SDK
// must not be given.
func (s *sanitizedInput) accept(line string) {
	refusal, refused := refuseUnreadable(line, s.limits.maxDepth)
	if !refused {
		s.pending = frameFor(line)
		return
	}
	if refusal != nil {
		_, _ = s.out.Write(refusal)
	}
}

// frameFor returns a line the filter passes as the SDK is handed it: with the
// whitespace after its message removed, and the newline that ended it kept.
//
// The SDK's read loop reads one JSON value and then the byte after it, and ends
// the session unless that byte is a line break, so a message followed by a
// space or a tab on its line ended the session though the line is valid JSON
// carrying a valid message. Whitespace before the message is skipped by the
// same read and stays.
func frameFor(line string) []byte {
	frame := []byte(strings.TrimRight(line, " \t\r\n"))
	if strings.HasSuffix(line, "\n") {
		frame = append(frame, '\n')
	}
	return frame
}

// maxLineBytes is the configured ceiling, or the default for a value built
// without one.
func (s *sanitizedInput) maxLineBytes() int {
	if s.limits.maxLineBytes > 0 {
		return s.limits.maxLineBytes
	}
	return defaultMaxStdioLineBytes
}

// readLine assembles the next line, reporting separately that it was longer
// than the ceiling and has been discarded.
//
// Discarding continues to the newline rather than stopping at the ceiling, so
// the remainder of an over-long message is not read as a message of its own —
// which would turn one refusal into a stream of them, and could hand the SDK a
// fragment that happens to parse.
func (s *sanitizedInput) readLine() (line string, oversize bool, err error) {
	limit := s.maxLineBytes()
	var assembled []byte
	for {
		chunk, readErr := s.in.ReadSlice('\n')
		// ReadSlice yields the delimiter with the line it terminates. The
		// ceiling is on the message, not on the framing around it, so the
		// newline is not charged to it: counting it refused a message of
		// exactly the ceiling, which is a byte narrower than the SDK's HTTP
		// body cap this is meant to match. ReadSlice returns the delimiter
		// whenever it returns no error, so the chunk is never empty here.
		counted := len(chunk)
		if readErr == nil {
			counted--
		}
		// Once over, keep reading to the newline and keep nothing.
		if !oversize {
			if len(assembled)+counted > limit {
				oversize = true
				assembled = nil
			} else {
				assembled = append(assembled, chunk...)
			}
		}
		if readErr == nil {
			return string(assembled), oversize, nil
		}
		// A full buffer is not an error, it is how ReadSlice says "more of
		// this line follows".
		if errors.Is(readErr, bufio.ErrBufferFull) {
			continue
		}
		return string(assembled), oversize, readErr
	}
}

// Close is a no-op: stdin is not ours to close.
func (s *sanitizedInput) Close() error { return nil }

// refuseUnreadable reports whether a line must be kept from the SDK, and what
// to answer its sender.
//
// A nil refusal with ok true means drop it silently: a blank line is framing,
// not a message, and answering one would be noise on a stream that is otherwise
// well behaved.
//
// # The checks before the decoder are load-bearing
//
// The depth check comes first so that nothing decodes a hostile line. go-sdk
// caps its own decode at 1000 levels from v1.8.0 on; before that it had no cap
// and decoded in time quadratic in the depth, and the standard library's parse
// that stood here, with its cap of 10000, was the only bound on this
// transport, about two CPU seconds a message. maxDepth makes the bound
// deliberate and puts it where a legitimate message never reaches it.
//
// json.Valid then holds the line to being exactly one JSON value, which the
// SDK's decoder does not: it reads the first value of what it is handed and
// ignores the rest, while the read loop ends the session on anything after a
// value but a line break. Without it a message followed by more text would
// pass the decoder here and still end the session.
func refuseUnreadable(line string, maxDepth int) (refusal []byte, refuse bool) {
	trimmed := trimSpace(line)
	if trimmed == "" {
		return nil, true
	}

	if maxDepth > 0 && toolutil.ExceedsJSONDepth([]byte(trimmed), maxDepth) {
		// The id is not knowable without the parse this is refusing to
		// perform, so it is null, as JSON-RPC 2.0 prescribes when it cannot
		// be read.
		return errorLine(nil, -32600, fmt.Sprintf(
			"Invalid Request: message nests JSON deeper than %d levels", maxDepth,
		)), true
	}

	frame := []byte(trimmed)
	if !json.Valid(frame) {
		// Not JSON, or more than one value. The id is unknowable, so it is
		// null, which is what JSON-RPC 2.0 prescribes for a parse error.
		return errorLine(nil, -32700, "Parse error"), true
	}
	if _, err := jsonrpc.DecodeMessage(frame); err != nil {
		// A frame the SDK's own decoder rejects ends the session as surely
		// as one that is not JSON, since the read loop breaks on any error
		// from that decode: JSON that is not an object, an object without
		// "jsonrpc":"2.0" (the SDK matches the key's case, so a key spelled
		// in another case does not carry it), an id that is a boolean, an
		// object or an array, a method that is not a string, an object
		// carrying nothing but the version tag. Asking that function rather
		// than repeating its rules keeps the SDK the one judge of what a
		// valid message is, and covers what a later SDK rejects with no
		// change here. It is valid JSON, so the answer is -32600 rather than
		// -32700: telling a client it sent no JSON sends it looking for a
		// syntax problem it does not have.
		return errorLine(refusalID(frame), -32600, "Invalid Request"), true
	}
	return nil, false
}

// refusalID returns the id a refusal of frame carries: the sender's own when
// frame is a request whose id the SDK can hold, and nothing otherwise.
//
// A frame with no method member is a response, and its id is one this server
// chose for a request of its own, so answering with it would fail that request
// with a refusal of something else. The members are read by their exact names,
// as the SDK reads them.
func refusalID(frame []byte) json.RawMessage {
	var members map[string]json.RawMessage
	if json.Unmarshal(frame, &members) != nil {
		return nil
	}
	if _, isRequest := members["method"]; !isRequest {
		return nil
	}
	return scalarID(members["id"])
}

// scalarID returns the id if JSON-RPC may echo it, or nothing.
//
// JSON-RPC allows a string, a number or null, and the SDK's own rule for what
// an id can be (jsonrpc.MakeID over the value encoding/json decodes) decides
// it: an object, an array or a boolean is dropped, and so is a number no
// float64 holds, which the SDK cannot read back. The answer then carries null.
func scalarID(id json.RawMessage) json.RawMessage {
	var value any
	if json.Unmarshal(id, &value) != nil {
		return nil
	}
	if _, err := jsonrpc.MakeID(value); err != nil {
		return nil
	}
	return id
}

// errorLine builds one newline-terminated JSON-RPC error response.
func errorLine(id json.RawMessage, code int, message string) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	})
	if err != nil {
		// Unreachable: every field here marshals. Staying silent is better
		// than writing a half-formed line onto the protocol stream.
		return nil
	}
	return append(body, '\n')
}

// trimSpace trims ASCII whitespace, which is all a JSON line can be padded
// with.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
