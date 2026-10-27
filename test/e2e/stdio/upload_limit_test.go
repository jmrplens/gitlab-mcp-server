// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// upload_limit_test.go drives a stdio server whose upload limit it cannot
// use, and checks that the start is refused, through the variable and
// through the flag that writes it, and one whose limit is the ceiling or
// blank, which serves (issue 1170, register row RQB-007).
package stdioe2e

import (
	"strings"
	"testing"
	"time"
)

// TestUploadLimit_AMalformedOrOversizedValue_RefusesToStart pins stdio's
// refusal of an upload limit it cannot use, which HTTP mode now makes too
// (the HTTP module holds that half). A value that does not parse, one above
// the 1 TB ceiling and the flag that writes the variable all refuse at ERROR,
// naming the variable, before anything is written on stdout. A size whose
// bytes do not fit in 64 bits used to wrap: 17179869185GB started a server
// under a 1 GiB limit nobody wrote.
func TestUploadLimit_AMalformedOrOversizedValue_RefusesToStart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		args  []string
		says  string
	}{
		{
			name: "a value that does not parse", value: "the-size-of-a-cat",
			says: `invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value: invalid size "the-size-of-a-cat"`,
		},
		{
			name: "a size above the ceiling", value: "1025GB",
			says: "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB",
		},
		{
			name: "a size no byte count holds", value: "17179869185GB",
			says: `invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value: size "17179869185GB" is a byte count that does not fit in 64 bits`,
		},
		{
			name: "the flag, which writes the variable", args: []string{"--upload-max-file-size=1025GB"},
			says: "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			if tc.value != "" {
				env["GITLAB_MCP_UPLOAD_MAX_FILE_SIZE"] = tc.value
			}
			s := startSessionWithArgs(t, env, tc.args...)
			code, exited := s.waitExit(t, 20*time.Second)
			if !exited {
				t.Fatalf("a stdio server with an upload limit it cannot use started instead of refusing\nstderr: %s", s.stderrText())
			}
			if code == 0 {
				t.Errorf("the refusal exited 0\nstderr: %s", s.stderrText())
			}
			refused := awaitLogRecord(t, s, "server exited with error", 5*time.Second)
			if level, _ := refused["level"].(string); level != "ERROR" {
				t.Errorf("the refusal is at %s, want ERROR", level)
			}
			if said, _ := refused["error"].(string); !strings.Contains(said, tc.says) {
				t.Errorf("the refusal says %q, want it to carry %q", said, tc.says)
			}
			if logs := s.stderrText(); strings.Contains(logs, stdioStartedLine) {
				t.Errorf("the refused run started the server:\n%s", logs)
			}
			if b, err := s.stdout.ReadByte(); err == nil {
				t.Errorf("the refused run wrote %q on stdout, which only JSON-RPC may use", b)
			}
		})
	}
}

// TestUploadLimit_TheCeilingOrABlankValue_IsServed is the other half. The
// ceiling is a limit an operator may set, so a stdio server given exactly
// 1 TB starts and serves rather than refusing a value equal to the maximum.
// A blank value is no value, as it is to HTTP mode's overlay, so the server
// starts under the default: stdio used to refuse it as a size that did not
// parse, where the same exported line started an HTTP server.
func TestUploadLimit_TheCeilingOrABlankValue_IsServed(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{name: "the ceiling itself", value: "1024GB"},
		{name: "a blank value", value: "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			env["GITLAB_MCP_UPLOAD_MAX_FILE_SIZE"] = tc.value
			s := startSession(t, env)
			if got := s.call(t, currentUserCall(1)); !served(got) {
				t.Fatalf("a stdio server whose upload limit is %q did not serve: %v\nstderr: %s", tc.value, got, s.stderrText())
			}
			s.terminate(t, 10*time.Second)
		})
	}
}
