// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// zero_settings_test.go drives a stdio server whose environment sets duration
// variables to zero, and checks that the server starts and serves rather than
// refusing a zero the flag of the same name accepts (issue 958, and items 1
// and 3 of issue 1171).
package stdioe2e

import (
	"testing"
)

// TestZeroSettings_AZeroDurationInTheEnvironment_IsServed pins the stdio
// reader's zero: a duration variable set to zero turns its setting off, as the
// flag of the same name does, and a zero written as a Go duration means what
// the bare digit does. GITLAB_MCP_SESSION_TIMEOUT=0 used to refuse a stdio
// start, and so did 0s in every one of them. GITLAB_MCP_ACTION_TIMEOUT and
// GITLAB_MCP_SESSION_REVALIDATE_INTERVAL are the two stdio itself applies, so
// the first's 0s also leaves the call it serves without a deadline; the rest
// configure settings only HTTP mode has, which stdio no longer reads at all
// since issue 1171 and names at startup instead (http_only_flags_test.go).
func TestZeroSettings_AZeroDurationInTheEnvironment_IsServed(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"GITLAB_MCP_SESSION_TIMEOUT", "0"},
		{"GITLAB_MCP_SESSION_TIMEOUT", "0s"},
		{"GITLAB_MCP_POOL_IDLE_TIMEOUT", "0s"},
		{"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "0s"},
		{"GITLAB_MCP_ACTION_TIMEOUT", "0s"},
		{"GITLAB_MCP_DRAIN_DELAY", "0s"},
		{"GITLAB_MCP_AUTH_FAILURE_WINDOW", "0s"},
		{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW", "0s"},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			env[tc.name] = tc.value
			s := startSession(t, env)
			if got := s.call(t, currentUserCall(1)); !served(got) {
				t.Fatalf("a stdio server with %s=%s did not serve: %v\nstderr: %s", tc.name, tc.value, got, s.stderrText())
			}
		})
	}
}
