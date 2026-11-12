// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// detect_timeout_test.go holds, on the real binary, the bound issue 1592 put
// on what a stdio start asks GitLab about its token, its user, its tier and a
// fine-grained token's grant. Against an instance that accepts the connection
// and never answers, a tools/list sent at start waited 133 seconds on 3.1.0:
// the connectivity check gave up after its own ten seconds, and every SDK
// request after it waited out its response-header timeout and its retries.
// The start now gives those questions ten seconds together, as an HTTP pool
// entry's build is given, and then serves the catalog.
//
// The instance is a [netfixture.SilentEndpoint], which accepts every
// connection and never writes a byte. An address that hangs up at once, as
// [netfixture.DeadEndpoint] does, fails each request immediately, so a case
// built on it passes with no bound at all.
package stdioe2e

import (
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// detectWithin is how long a tools/list sent at start may wait against an
// instance that never answers: the connectivity check's ten seconds, the
// detection bound's ten, and room for the process to start and register its
// catalog. Without the bound the listing waited more than two minutes.
const detectWithin = 35 * time.Second

// TestStartup_AnInstanceThatNeverAnswers_ServesTheCatalogWithinTheBound
// verifies that a tools/list sent to a server whose instance accepts the
// connection and never answers is answered with the catalog within the
// connectivity check and the detection bound together, and that the start
// says it is degraded, which is what it is.
func TestStartup_AnInstanceThatNeverAnswers_ServesTheCatalogWithinTheBound(t *testing.T) {
	silent := netfixture.SilentEndpoint(t)
	s := startSession(t, baseEnv(silent))

	started := time.Now()
	s.send(t, request(1, "tools/list", ""))
	var listing map[string]any
	for listing == nil {
		got := s.readMessage(t, detectWithin)
		if _, notification := got["method"]; notification && got["id"] == nil {
			continue
		}
		listing = got
	}
	waited := time.Since(started)

	if _, listed := listing["result"]; !listed {
		t.Fatalf("tools/list against an instance that never answers = %v, want the catalog", listing)
	}
	if waited > detectWithin {
		t.Errorf("tools/list was answered after %v, want it within %v", waited, detectWithin)
	}
	if !strings.Contains(s.stderrText(), "degraded mode") {
		t.Errorf("the start against an instance that never answers did not say it is degraded\nstderr: %s", s.stderrText())
	}
}
