// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// revalidate_interval_test.go drives a stdio server holding a fine-grained
// token, whose grant and instance version it re-reads on a timer so a session
// follows an instance upgraded under it, and checks that the timer follows
// GITLAB_MCP_SESSION_REVALIDATE_INTERVAL as the HTTP pool's revalidation does
// (issue 1171, Q16 of its triage). Stdio used to parse the variable and then
// re-read at the fifteen-minute default whatever it said.
package stdioe2e

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// startPhaseBSession starts a stdio server whose fine-grained token may read
// its grant, against an instance reporting the release the table records, with
// the revalidation interval set to interval, and returns once the start has
// read the grant: a tools/list is held until the catalog is ready, which is
// after the grant is judged.
func startPhaseBSession(t *testing.T, interval string) (*session, *fakeGitLab) {
	t.Helper()

	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	fake.version = actiongrants.Table().Version
	fake.grantScopes = workItemCreateGrant
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_SESSION_REVALIDATE_INTERVAL"] = interval
	s := startSession(t, env)

	assertListed(t, s, 1)
	if reads := fake.grantReads.Load(); reads < 1 {
		t.Fatalf("the start read the grant %d times, want it read before serving", reads)
	}
	return s, fake
}

// assertListed holds a tools/list with the given id to being answered with a
// result.
func assertListed(t *testing.T, s *session, id int) {
	t.Helper()
	if got := s.call(t, request(id, "tools/list", "")); got["result"] == nil {
		t.Fatalf("tools/list was not answered with a result: %v\nstderr: %s", got, s.stderrText())
	}
}

// TestRevalidateInterval_OnStdio_ReReadsTheGrantAtIt pins the setting reaching
// the stdio timer: an interval of a fifth of a second has the grant read round
// after round within seconds, where the fifteen-minute default it used to run
// at whatever the variable said reads it once in the test's lifetime.
func TestRevalidateInterval_OnStdio_ReReadsTheGrantAtIt(t *testing.T) {
	_, fake := startPhaseBSession(t, "200ms")

	started := fake.grantReads.Load()
	deadline := time.Now().Add(10 * time.Second)
	for fake.grantReads.Load() < started+3 {
		if time.Now().After(deadline) {
			t.Fatalf("the grant was read %d times in the 10s after the start, want at least 3 more at an interval of 200ms",
				fake.grantReads.Load()-started)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestRevalidateInterval_OnStdio_ZeroStopsTheReReads pins what zero means to
// the stdio timer since it reads the setting: no re-read at all, as zero stops
// the pool's revalidation, and a process that goes on serving. time.NewTicker
// refuses an interval of zero by panicking, which a stdio server handing it
// the setting unguarded would have done as the catalog became ready, killing
// the process; and the window waited is fifteen rounds of the sibling case's
// interval.
func TestRevalidateInterval_OnStdio_ZeroStopsTheReReads(t *testing.T) {
	s, fake := startPhaseBSession(t, "0")

	started := fake.grantReads.Load()
	time.Sleep(3 * time.Second)
	assertListed(t, s, 2)
	if reads := fake.grantReads.Load() - started; reads != 0 {
		t.Errorf("the grant was read %d more times after the start with GITLAB_MCP_SESSION_REVALIDATE_INTERVAL=0, want none", reads)
	}
}
