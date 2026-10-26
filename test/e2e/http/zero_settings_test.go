//go:build httpe2e

// zero_settings_test.go drives the real binary with a limit set to zero, and
// checks that zero means what issue 958 decided it means for that limit: off,
// and nothing else switched off with it, or a refusal at startup where neither
// no limit nor a limit of nothing is a deployment an operator could want.
//
// Each case is one place where the flag and the variable of a setting, or two
// settings that share a window, used to disagree about zero. A unit test
// reassembles one reader at a time, and what an operator meets is the binary
// reading both.
package httpe2e

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// zeroSessionTimeoutWarning is the line a stateful deployment whose session
// timeout is zero writes at startup, spelled here because this module cannot
// import cmd/server.
const zeroSessionTimeoutWarning = "--session-timeout=0 (or GITLAB_MCP_SESSION_TIMEOUT=0)"

// TestZeroSettings_ASessionTimeoutOfZeroInTheEnvironment_TurnsItOff pins the
// variable meaning what the flag means by zero (issue 958, and item 1 of issue
// 1171): a stateful session no client deletes then never expires, and the
// server says what that costs at startup in the words it uses for the flag.
// GITLAB_MCP_SESSION_TIMEOUT=0 used to refuse the start, so a deployment
// configured through its environment could not choose the setting the flag
// offered. 0s means the same as 0 (item 3).
func TestZeroSettings_ASessionTimeoutOfZeroInTheEnvironment_TurnsItOff(t *testing.T) {
	for _, value := range []string{"0", "0s"} {
		t.Run(value, func(t *testing.T) {
			srv := startServer(t, map[string]string{"GITLAB_MCP_SESSION_TIMEOUT": value}, "--stateless=false")
			if logs := srv.logs(); !strings.Contains(logs, zeroSessionTimeoutWarning) {
				t.Errorf("GITLAB_MCP_SESSION_TIMEOUT=%s started without the warning %q:\n%s", value, zeroSessionTimeoutWarning, logs)
			}
		})
	}
}

// TestZeroSettings_AZeroDurationWrittenAsADuration_StartsTheServer pins item 3
// of issue 1171: every duration variable whose zero turns its setting off
// takes 0s, as the flag of the same name does. Each used to refuse the start
// on anything but the bare digit.
func TestZeroSettings_AZeroDurationWrittenAsADuration_StartsTheServer(t *testing.T) {
	for _, name := range []string{
		"GITLAB_MCP_POOL_IDLE_TIMEOUT", "GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "GITLAB_MCP_ACTION_TIMEOUT",
		"GITLAB_MCP_DRAIN_DELAY", "GITLAB_MCP_AUTH_FAILURE_WINDOW", "GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW",
	} {
		t.Run(name, func(t *testing.T) {
			srv := startServer(t, map[string]string{name: "0s"})
			if got := srv.do(t, request{method: http.MethodGet, path: "/health"}); got.status != http.StatusOK {
				t.Errorf("%s=0s: /health = %d, want 200", name, got.status)
			}
		})
	}
}

// TestZeroSettings_AFailureWindowOfZero_LeavesTheDistinctCredentialBudgetOn
// pins INV-015's clause that switching one limit off switches no other (issue
// 958). The distinct-credential budget's escalation steps by the failure
// window, so a window of zero, which switches the failure budget off, used to
// switch the distinct-credential budget off with it: a stream of distinct
// refused credentials was then never blocked. It now steps by the default
// window, a minute, which the first block's Retry-After announces.
func TestZeroSettings_AFailureWindowOfZero_LeavesTheDistinctCredentialBudgetOn(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--auth-failure-window=0",
		"--auth-distinct-token-limit=2",
	)

	var blocked response
	for i := range 6 {
		got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-distinct-" + strconv.Itoa(i)}))
		if got.status == http.StatusTooManyRequests {
			blocked = got
			break
		}
	}
	if blocked.status != http.StatusTooManyRequests {
		t.Fatal("with --auth-failure-window=0 a stream of distinct credentials was never blocked")
	}
	if after := retryAfterOf(t, blocked); after < 55 || after > 60 {
		t.Errorf("first Retry-After = %ds, want the default one-minute step", after)
	}
}

// TestZeroSettings_ANonPositiveOAuthCacheTTL_RefusesToStart pins the zero
// issue 958 keeps refused, and D7 of the 2026-09 documentation audit (issue
// 529): --oauth-cache-ttl=0 and a negative value used to start a server that
// replaced them with the default, 15 minutes no page described, while the
// variable refused the same zero. The flag now refuses what its variable
// refuses, since a TTL of zero would expire every token as it is verified.
//
// In legacy mode too, where no OAuth token is verified: the variable refuses
// its zero in either mode, so the flag does, and the two spellings of the
// setting start the same deployments.
func TestZeroSettings_ANonPositiveOAuthCacheTTL_RefusesToStart(t *testing.T) {
	modes := map[string][]string{
		"oauth":  {"--auth-mode=oauth", "--public-url=" + publicURL},
		"legacy": nil,
	}
	for mode, modeFlags := range modes {
		for _, value := range []string{"0", "0s", "-5m"} {
			t.Run(mode+"/flag/"+value, func(t *testing.T) {
				args := append([]string{
					"--http", "--http-addr=127.0.0.1:0",
					"--gitlab-url=https://gitlab.example.com",
					"--oauth-cache-ttl=" + value,
				}, modeFlags...)
				out, err := runServerExpectingExit(t, serverBinary(t), args...)
				if err == nil {
					t.Fatalf("the server started and exited cleanly with --oauth-cache-ttl=%s; output:\n%s", value, out)
				}
				if !strings.Contains(out, "--oauth-cache-ttl must be positive") {
					t.Errorf("--oauth-cache-ttl=%s did not refuse the start naming the flag; output:\n%s", value, out)
				}
			})
		}
		t.Run(mode+"/variable/0", func(t *testing.T) {
			args := append([]string{
				"--http", "--http-addr=127.0.0.1:0",
				"--gitlab-url=https://gitlab.example.com",
			}, modeFlags...)
			out, err := runServerWithEnvExpectingExit(t, serverBinary(t),
				map[string]string{"GITLAB_MCP_OAUTH_CACHE_TTL": "0"}, args...)
			if err == nil {
				t.Fatalf("the server started and exited cleanly with GITLAB_MCP_OAUTH_CACHE_TTL=0; output:\n%s", out)
			}
			if !strings.Contains(out, "invalid OAUTH_CACHE_TTL value") {
				t.Errorf("GITLAB_MCP_OAUTH_CACHE_TTL=0 did not refuse the start naming the variable; output:\n%s", out)
			}
		})
	}
}
