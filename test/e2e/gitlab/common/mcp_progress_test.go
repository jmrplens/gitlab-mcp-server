//go:build e2e

// mcp_progress_test.go drives the progress capability end to end: a call that
// carries a progress token, and the notifications the server sends back while
// it runs.
//
// Nothing else in the suite asks for one. The handlers' own trackers are unit
// tested in process, which proves a tracker fires and nothing about whether a
// notification reaches a client: the token has to survive _meta, the
// dispatcher, the handler and the transport, and every one of those is only
// exercised by the real binary. A capability the server declares and never
// demonstrably delivers is the same hole the elicitation recorder had.
//
// The action is a generic package publish, which reports as it reads the
// content it is about to send. It is deliberately a byte-counted action rather
// than a step-based one: the step flows are the interactive wizards, which need
// an elicitation policy and are covered by their own scenarios.

package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// progressPayloadBytes is the published content's size.
//
// Comfortably past the reporter's 64 KB floor, so the read is reported while
// it is still going rather than only at its end: a payload under the floor
// produces exactly one notification, on EOF, which would pass an assertion
// that some note arrived while proving nothing about progress being reported
// as work happens.
const progressPayloadBytes = 300 * 1024

// TestProgress_PackagePublish_ReportsAsItReadsTheContent publishes a package
// with a progress token and holds the notifications to what the specification
// requires of them.
//
// Three properties, and the first is the one a client breaks on: progress must
// strictly increase. The others are that the notes describe the work rather
// than being empty, and that the series ends at the total it counts towards,
// so a client's bar reaches its end instead of stopping short.
func TestProgress_PackagePublish_ReportsAsItReadsTheContent(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	project := fixture.NewProject(e, fixture.WithNamePrefix("progress"))
	content := strings.Repeat("progress fixture payload\n", progressPayloadBytes/25)

	published, notes := harness.WithProgress[packages.PublishOutput](s, actionPackagePublish, map[string]any{
		"project_id":      project.IDParam(),
		"package_name":    "progress-fixture",
		"package_version": "1.0.0",
		"file_name":       "payload.txt",
		"content_base64":  base64.StdEncoding.EncodeToString([]byte(content)),
	})
	if published.PackageFileID == 0 {
		t.Fatalf("publish answered %+v, want a package file ID", published)
	}

	if len(notes) == 0 {
		t.Fatal("a call carrying a progress token received no notification: the capability is declared and undelivered")
	}

	previous := -1.0
	for i, note := range notes {
		if note.Progress <= previous {
			t.Errorf("note %d reports progress %v after %v, which the specification forbids: it must strictly increase",
				i, note.Progress, previous)
		}
		previous = note.Progress
		if note.Message == "" {
			t.Errorf("note %d carries no message, so a client has nothing to show for it", i)
		}
	}

	last := notes[len(notes)-1]
	if last.Total <= 0 {
		t.Errorf("the last note reports total %v, want the size the progress counts towards", last.Total)
	} else if last.Progress != last.Total {
		t.Errorf("the series ends at %v of %v, want it to reach the total so a client's bar completes",
			last.Progress, last.Total)
	}
}

// TestProgress_WithoutAToken_TheCallStillSucceeds checks the other half of the
// contract: progress is an enhancement, and a client that asks for none must be
// served identically.
//
// It is worth asserting rather than assuming, because the tracker is built from
// the request and a handler that dereferenced it unconditionally would fail
// only for the clients that ask for nothing — which is most of them, and none
// of the tests above.
func TestProgress_WithoutAToken_TheCallStillSucceeds(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	project := fixture.NewProject(e, fixture.WithNamePrefix("noprogress"))

	published := harness.Do[packages.PublishOutput](s, actionPackagePublish, map[string]any{
		"project_id":      project.IDParam(),
		"package_name":    "no-progress-fixture",
		"package_version": "1.0.0",
		"file_name":       "payload.txt",
		"content_base64":  base64.StdEncoding.EncodeToString([]byte("a short payload")),
	})
	if published.PackageFileID == 0 {
		t.Fatalf("publish answered %+v, want a package file ID", published)
	}
}
