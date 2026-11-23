// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// nuget_test.go covers what the record of the builders does not reach in
// the nuget subcommand: the escaping of the .mcp/server.json printer.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestWritePythonString_EscapesAsEnsureASCIIDoes holds the printer to what
// Python's json.dumps wrote for a string holding every kind of character:
// the quote and the backslash, the five control characters named by letter,
// another control character, DEL, a character of the first plane past ASCII
// and one beyond it, as a surrogate pair, with the slash left as it is.
func TestWritePythonString_EscapesAsEnsureASCIIDoes(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writePythonString(&out, "q\"b\\ n\n r\r t\t b\b f\f c\x01 d\x7f \xc3\xa9 \xe2\x80\x94 \U0001F600 /")
	// json.dumps of the same string, from Python 3.14.
	// Each % stands for a backslash and a u, written apart so the escape
	// stays an escape in this source.
	want := strings.ReplaceAll(`"q\"b\\ n\n r\r t\t b\b f\f c%0001 d%007f %00e9 %2014 %d83d%de00 /"`, "%", `\`+"u")
	if out.String() != want {
		t.Errorf("writePythonString:\n got %s\nwant %s", out.String(), want)
	}
}
