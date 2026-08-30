package main

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// maxIDProbeBytes bounds how much of a request body is read to recover its id.
//
// The id is a top-level member and conventionally the second one, so this is
// generous rather than tight. It exists because the probe runs on requests that
// are being refused, including unauthenticated ones: without a bound, a caller
// with no credential could make the server buffer a body of any size.
const maxIDProbeBytes = 1 << 20

// requestIDFromBody recovers the JSON-RPC id of a request that is about to be
// refused, so the error response can correlate with it.
//
// "Error responses MUST include the same ID as the request they correspond to
// (except in error cases where the ID could not be read due a malformed
// request)." The exception is narrow, and none of the rejections this serves
// fall under it: an unsupported protocol version, an untrusted Origin, a wrong
// Host and a missing credential are all decided from headers, on a body that is
// perfectly well formed and whose id is right there.
//
// It returns nil when there is no id to be had: a GET, a notification, a body
// that is not a JSON-RPC request, or one larger than the probe reads. Callers
// omit the member entirely in that case rather than sending null: under
// 2026-07-28 a RequestId is a string or an integer, so null is not a legal
// value, and the schema marks the member optional precisely so it can be left
// out.
//
// Reading the body is safe only because every caller is refusing the request
// and returning; nothing downstream will read it again. Do not call this on a
// path that forwards.
func requestIDFromBody(r *http.Request) json.RawMessage {
	if r == nil || r.Body == nil || r.Method != http.MethodPost {
		return nil
	}

	var probe struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxIDProbeBytes))
	if err := decoder.Decode(&probe); err != nil {
		return nil
	}
	// Decode stops at the end of the first JSON value, so a body like
	// `{"id":1} trailing` decodes without complaint. That body is not a
	// request, and an id lifted out of it is not the id of anything: require
	// the value to be the whole of what was sent.
	if decoder.More() {
		return nil
	}
	// The same rule the stdio filter applies in [refuseUnreadable]: an object
	// that does not announce itself as JSON-RPC 2.0 is not a message, so there
	// is no request to correlate a refusal with. Stopping here rather than also
	// requiring "method" is deliberate. A body carrying jsonrpc and an id but
	// no method is a malformed request, and the specification's own exception
	// covers exactly that case; refusing to echo would be defensible too, but
	// it would refuse an id the sender can still match.
	if probe.JSONRPC != "2.0" {
		return nil
	}
	if !isRequestID(probe.ID) {
		return nil
	}
	return probe.ID
}

// isRequestID reports whether a raw JSON value can stand as a JSON-RPC id.
//
// A string or a number is echoed back as it arrived, which is what correlation
// means: the client matches on the value it sent, not on our reading of it.
// Everything else (null, an object, an array, an absent member) leaves the
// response with no id.
func isRequestID(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	switch {
	case trimmed[0] == '"':
		var s string
		return json.Unmarshal(trimmed, &s) == nil
	case trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9'):
		var n json.Number
		return json.Unmarshal(trimmed, &n) == nil
	}
	return false
}
