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
		ID json.RawMessage `json:"id"`
	}
	limited := http.MaxBytesReader(nil, r.Body, maxIDProbeBytes)
	if err := json.NewDecoder(limited).Decode(&probe); err != nil {
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
