package main

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
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
// It deliberately does the least it can. Anything that parses as a JSON object
// carrying "jsonrpc":"2.0" is passed through untouched, whatever else is wrong
// with it, because deciding what a valid message means is the SDK's job and a
// filter that second-guessed it would be a second, divergent implementation of
// the protocol.
func resilientStdio(in io.Reader, out io.Writer) (io.ReadCloser, io.WriteCloser) {
	shared := &lockedWriter{w: out}
	return &sanitizedInput{in: bufio.NewReader(in), out: shared}, shared
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
	in  *bufio.Reader
	out io.Writer

	// pending holds the remainder of the line currently being handed over.
	// Lines are passed through whole, so the SDK's own framing is unchanged.
	pending []byte
}

// Read implements io.Reader, yielding only lines the SDK can parse.
//
// bufio.Reader.ReadString is used rather than a Scanner because a Scanner caps
// a token at its buffer size and a legitimate request can be large — a package
// publish carries its file inline as base64. Silently truncating one of those
// would turn a working call into the exact failure this is here to prevent.
func (s *sanitizedInput) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		line, err := s.in.ReadString('\n')
		if line != "" {
			if refusal, ok := refuseUnreadable(line); ok {
				if refusal != nil {
					_, _ = s.out.Write(refusal)
				}
			} else {
				s.pending = []byte(line)
			}
		}
		if err != nil {
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

// Close is a no-op: stdin is not ours to close.
func (s *sanitizedInput) Close() error { return nil }

// refuseUnreadable reports whether a line must be kept from the SDK, and what
// to answer its sender.
//
// A nil refusal with ok true means drop it silently: a blank line is framing,
// not a message, and answering one would be noise on a stream that is otherwise
// well behaved.
func refuseUnreadable(line string) (refusal []byte, refuse bool) {
	trimmed := trimSpace(line)
	if trimmed == "" {
		return nil, true
	}

	var probe struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		// Not JSON at all. The id is unknowable, so it is null, which is what
		// JSON-RPC 2.0 prescribes for a parse error.
		return errorLine(nil, -32700, "Parse error"), true
	}
	if probe.JSONRPC != "2.0" {
		// Structurally JSON and not a JSON-RPC message. The id, if it carried
		// one, is worth echoing so the sender can match the refusal.
		return errorLine(probe.ID, -32600, "Invalid Request"), true
	}
	return nil, false
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
