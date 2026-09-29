//go:build stdioe2e

// retired_env_test.go drives a stdio server whose environment still carries a
// variable name 3.1.0 stopped reading, against the real binary. The rule is
// what ignoring the name would cost: the three that withhold part of what the
// server serves (GITLAB_READ_ONLY, GITLAB_SAFE_MODE and EXCLUDE_TOOLS) refuse
// the start, and every other retired name is named at WARN while the server
// serves.
package stdioe2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// retiredEnvLine is the line internal/config writes for a retired name, and
// retiredRefusalSuffix what cmd/server adds to it when it refuses the start.
// Both are spelled here because this module cannot import either package.
func retiredEnvLine(retired, renamed string) string {
	return retired + " is no longer read (removed in 3.1.0): rename it to " + renamed
}

const retiredRefusalSuffix = ", and this deployment will not be started under a capability it did not ask for"

// TestRetiredEnvNames_AWithholdingSetting_RefusesToStart pins the three
// retired names a warning is not enough for. A stdio server that stopped
// reading one in silence would serve writes, or an action the operator
// removed, and the stdio servers most likely to still carry one are those an
// MCP client starts with nobody reading their stderr.
//
// EXCLUDE_TOOLS is the case this file was written for. It is the one bare
// name of the three, and until it joined the other two a retired exclusion
// was only named and the excluded actions served, while the --exclude-tools
// flag beside it refused a stdio start (issue 1045).
//
// Each name is given twice, once in the environment and once in the file
// GITLAB_MCP_ENV_FILE names, since a long-lived deployment is at least as
// likely to keep one there: a file the server loads configures it exactly as
// much as the environment does, so it is refused the same.
func TestRetiredEnvNames_AWithholdingSetting_RefusesToStart(t *testing.T) {
	cases := []struct{ retired, value, renamed string }{
		{retired: "GITLAB_READ_ONLY", value: "true", renamed: "GITLAB_MCP_READ_ONLY"},
		{retired: "GITLAB_SAFE_MODE", value: "true", renamed: "GITLAB_MCP_SAFE_MODE"},
		{retired: "EXCLUDE_TOOLS", value: "project.delete", renamed: "GITLAB_MCP_EXCLUDE_TOOLS"},
	}
	for _, tc := range cases {
		t.Run(tc.retired, func(t *testing.T) {
			line := retiredEnvLine(tc.retired, tc.renamed) + retiredRefusalSuffix

			t.Run("in the environment", func(t *testing.T) {
				env := baseEnv(startFakeGitLab(t).URL)
				env[tc.retired] = tc.value
				awaitRefusal(t, startSession(t, env), line)
			})
			t.Run("in the env file", func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "deployment.env")
				if err := os.WriteFile(path, []byte(tc.retired+"="+tc.value+"\n"), 0o600); err != nil {
					t.Fatalf("writing the env file: %v", err)
				}
				env := baseEnv(startFakeGitLab(t).URL)
				env["GITLAB_MCP_ENV_FILE"] = path
				awaitRefusal(t, startSession(t, env), line)
			})
		})
	}
}

// TestRetiredEnvNames_AnyOtherRetiredName_IsNamedAndServed is the other half
// of the split: a retired name that configures something other than what the
// server withholds costs a setting when ignored, which is worth a line and not
// an outage. The server names it at WARN with the spelling to use, and serves.
//
// The call goes through gitlab_execute_action, which the individual surface
// does not register, so its being served is also the retired value having
// been ignored rather than read.
func TestRetiredEnvNames_AnyOtherRetiredName_IsNamedAndServed(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("a stdio server carrying a retired TOOL_SURFACE did not serve: %v", got)
	}
	warned := awaitLogRecord(t, s, retiredEnvLine("TOOL_SURFACE", "GITLAB_MCP_TOOL_SURFACE"), 10*time.Second)
	if level, _ := warned["level"].(string); level != "WARN" {
		t.Errorf("the retired TOOL_SURFACE is named at %s, want WARN", level)
	}
}
