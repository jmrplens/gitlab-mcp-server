//go:build httpe2e

// negative_flags_test.go drives the real binary with a duration flag set below
// zero, and checks that the start is refused as the variable of the same
// setting refuses it (issue 1171, Q16 of its triage). Zero already says off to
// each of them, so a minus sign is a typo rather than a setting.
package httpe2e

import (
	"strings"
	"testing"
)

// negativeDurationSettings are the three duration settings whose flag let a
// negative value through: each flag with the variable that already refused
// one, and the words each refusal names it by.
var negativeDurationSettings = []struct {
	flag, variable, flagRefusal, variableRefusal string
}{
	{
		"--session-timeout", "GITLAB_MCP_SESSION_TIMEOUT",
		"--session-timeout must not be negative", "invalid SESSION_TIMEOUT value",
	},
	{
		"--revalidate-interval", "GITLAB_MCP_SESSION_REVALIDATE_INTERVAL",
		"--revalidate-interval must not be negative", "invalid SESSION_REVALIDATE_INTERVAL value",
	},
	{
		"--pool-idle-timeout", "GITLAB_MCP_POOL_IDLE_TIMEOUT",
		"--pool-idle-timeout must not be negative", "invalid POOL_IDLE_TIMEOUT value",
	},
}

// TestNegativeFlags_ADurationBelowZero_RefusesToStart pins the flag refusing
// what its variable refuses. A negative --session-timeout,
// --revalidate-interval or --pool-idle-timeout used to start a server that
// read it as off: sessions that never expired, without the warning a zero
// writes, credentials never re-checked, entries never reclaimed. Each now
// exits non-zero before listening, naming the flag, and the variable is held
// beside it, so the two spellings of a setting start the same deployments.
func TestNegativeFlags_ADurationBelowZero_RefusesToStart(t *testing.T) {
	base := []string{"--http", "--http-addr=127.0.0.1:0", "--gitlab-url=https://gitlab.example.com", "--stateless=false"}
	for _, setting := range negativeDurationSettings {
		t.Run(setting.flag, func(t *testing.T) {
			out, err := runServerExpectingExit(t, serverBinary(t), append(base, setting.flag+"=-1m")...)
			if err == nil {
				t.Fatalf("the server started and exited cleanly with %s=-1m; output:\n%s", setting.flag, out)
			}
			if !strings.Contains(out, setting.flagRefusal) {
				t.Errorf("%s=-1m did not refuse the start naming the flag; output:\n%s", setting.flag, out)
			}
		})
		t.Run(setting.variable, func(t *testing.T) {
			out, err := runServerWithEnvExpectingExit(t, serverBinary(t), map[string]string{setting.variable: "-1m"}, base...)
			if err == nil {
				t.Fatalf("the server started and exited cleanly with %s=-1m; output:\n%s", setting.variable, out)
			}
			if !strings.Contains(out, setting.variableRefusal) {
				t.Errorf("%s=-1m did not refuse the start naming the variable; output:\n%s", setting.variable, out)
			}
		})
	}
}
