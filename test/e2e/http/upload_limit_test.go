// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// upload_limit_test.go drives the real binary in HTTP mode with an upload
// limit it cannot use, and checks that the start is refused as a stdio start
// refuses it, and with one that is the ceiling or blank, which serves as it
// does on stdio (issue 1170, register row RQB-007).
package httpe2e

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// TestUploadLimit_AMalformedOrOversizedValue_RefusesToStartOverHTTP pins the
// refusal on the transport a hosted deployment uses, through the variable and
// through the flag, which writes the variable. A value that did not parse, or
// was not positive, used to start a server under the 2GB default, and one
// above the 1 TB ceiling under the ceiling, each with a WARN line easy to miss
// in a deployment's logs, where stdio refused all three. Each now exits
// non-zero before the listener opens, naming the variable.
func TestUploadLimit_AMalformedOrOversizedValue_RefusesToStartOverHTTP(t *testing.T) {
	base := []string{"--http", "--http-addr=127.0.0.1:0", "--gitlab-url=https://gitlab.example.com"}
	// The output is the JSON log, where the quotes around a value the error
	// quotes are escaped, so each refusal is matched up to the value.
	for _, tc := range []struct {
		name  string
		env   map[string]string
		flags []string
		says  string
	}{
		{
			name: "a value that does not parse",
			env:  map[string]string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE": "the-size-of-a-cat"},
			says: "invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value: invalid size ",
		},
		{
			name: "a size of nothing",
			env:  map[string]string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE": "0"},
			says: "invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value: size must be positive",
		},
		{
			name: "a size above the ceiling",
			env:  map[string]string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE": "1025GB"},
			says: "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB",
		},
		{
			name: "a size no byte count holds",
			env:  map[string]string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE": "17179869185GB"},
			says: "does not fit in 64 bits",
		},
		{
			name:  "the flag, which writes the variable",
			flags: []string{"--upload-max-file-size=1025GB"},
			says:  "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runServerWithEnvExpectingExit(t, serverBinary(t), tc.env, append(slices.Clone(base), tc.flags...)...)
			if err == nil {
				t.Fatalf("the server started and exited cleanly; output:\n%s", out)
			}
			if !strings.Contains(out, tc.says) {
				t.Errorf("the start was not refused with %q; output:\n%s", tc.says, out)
			}
			if strings.Contains(out, "listening on tcp") {
				t.Errorf("the server listened under an upload limit it refuses; output:\n%s", out)
			}
		})
	}
}

// TestUploadLimit_TheCeilingOrABlankValue_IsServedOverHTTP is the other half.
// The ceiling is a limit an operator may set, so a server given exactly 1 TB
// starts and serves rather than refusing a value equal to the maximum. A
// blank value is no value, so the server starts under the default, as a stdio
// server now does too (the stdio module holds that half). startServer fails
// the test on a server that does not come up healthy.
func TestUploadLimit_TheCeilingOrABlankValue_IsServedOverHTTP(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{name: "the ceiling itself", value: "1024GB"},
		{name: "a blank value", value: "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := startServer(t, map[string]string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE": tc.value})
			if got := srv.do(t, request{method: http.MethodGet, path: "/health"}); got.status != http.StatusOK {
				t.Errorf("/health answered %d, want 200 from a server whose upload limit is %q", got.status, tc.value)
			}
		})
	}
}
