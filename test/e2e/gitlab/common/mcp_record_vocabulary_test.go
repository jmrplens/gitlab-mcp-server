//go:build e2e

// mcp_record_vocabulary_test.go pins the words a run writes into its call
// record, because a reader joins on them and nothing else.
//
// Every call this suite makes is written to a shard as text: which surface and
// mode the session was, what the call was for, and what it expected. The
// coverage command reads those shards back and decides from those same strings
// whether an action is asserted, swept, refused or unservable. The join is by
// string equality, so renaming one — a mode, a purpose, a failure class — does
// not fail a build or a test. It makes the two halves stop matching, and the
// report then says an action was never exercised when it was, which is the one
// kind of wrong a coverage report must never be.
//
// This is the same shape as the server card's end-reason test, and for the same
// reason: a vocabulary shared across a boundary needs one place that fails when
// half of it moves.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestRecordVocabulary_IsWhatAReaderJoinsOn holds each value of the record's
// vocabulary to the word it is written as.
//
// The literals are written out rather than derived, deliberately: deriving them
// from the constants would compare each value with itself and pass through any
// rename, which is exactly the change this exists to catch.
func TestRecordVocabulary_IsWhatAReaderJoinsOn(t *testing.T) {
	for family, words := range map[string]map[string]string{
		"surfaces": {
			"dynamic":    harness.SurfaceDynamic.String(),
			"meta":       harness.SurfaceMeta.String(),
			"individual": harness.SurfaceIndividual.String(),
		},
		"modes": {
			"default":   harness.ModeDefault.String(),
			"read-only": harness.ModeReadOnly.String(),
			"safe":      harness.ModeSafe.String(),
		},
		"capability surfaces": {
			"full":    harness.CapabilitiesFull.String(),
			"minimal": harness.CapabilitiesMinimal.String(),
		},
		"elicitation policies": {
			"none":        harness.ElicitationNone.String(),
			"auto-accept": harness.ElicitationAutoAccept.String(),
			"scripted":    harness.ElicitationScripted.String(),
		},
		"transports": {
			"stdio": harness.TransportStdio.String(),
			"http":  harness.TransportHTTP.String(),
		},
		// The purpose decides whether a call counts as coverage at all: a
		// fixture built or torn down is worth less than one a test asserted on,
		// and the reader tells them apart by these words alone.
		"purposes": {
			"test":    harness.PurposeTest.String(),
			"cleanup": harness.PurposeCleanup.String(),
			"sweep":   harness.PurposeSweep.String(),
			"raw":     harness.PurposeRaw.String(),
		},
		"expectations": {
			"ok":  string(harness.ExpectationOK),
			"any": string(harness.ExpectationAny),
		},
		// Four of these are the server's own refusal vocabulary, so a change
		// there is a change to what a client reads, and the record is where the
		// suite notices.
		"failure classes": {
			"tool_error":         harness.FailureToolError.String(),
			"unknown_action":     harness.FailureUnknownAction.String(),
			"invalid_params":     harness.FailureInvalidParams.String(),
			"needs_confirmation": harness.FailureNeedsConfirmation.String(),
			"safe_mode":          harness.FailureSafeMode.String(),
			"not_found":          harness.FailureNotFound.String(),
			"forbidden":          harness.FailureForbidden.String(),
			"protocol_error":     harness.FailureProtocolError.String(),
			"withheld":           harness.FailureWithheld.String(),
		},
	} {
		t.Run(family, func(t *testing.T) {
			for want, got := range words {
				if got != want {
					t.Errorf("a value of %s is recorded as %q, want %q", family, got, want)
				}
			}
		})
	}
}

// TestRecordVocabulary_ASkippedRunSaysWhatItWanted checks the other half of the
// record: the words that explain why a test did not run.
//
// A skipped scenario is the one outcome a reader cannot investigate from the
// record alone, so the reason has to read as a sentence. Both of these reach a
// message: a Need names itself when a run does not provide it, and a
// Requirement names the runtime a scenario wanted. Printed as the struct and
// the integer they are underneath, an omission would read as noise and the
// report would hide the difference between a test that was skipped for a
// missing runner and one skipped for an unlicensed instance.
func TestRecordVocabulary_ASkippedRunSaysWhatItWanted(t *testing.T) {
	t.Run("needs", func(t *testing.T) {
		for want, got := range map[string]string{
			"admin":            harness.NeedAdmin.String(),
			"runner":           harness.NeedRunner.String(),
			"external network": harness.NeedExternalNetwork.String(),
		} {
			if got != want {
				t.Errorf("a need reads as %q in a skip message, want %q", got, want)
			}
		}
	})

	t.Run("runtime requirements", func(t *testing.T) {
		for want, got := range map[string]string{
			"any runtime":                    harness.Any.String(),
			"free (no license)":              harness.Free.String(),
			"licensed (Premium or Ultimate)": harness.Licensed.String(),
		} {
			if got != want {
				t.Errorf("a runtime requirement reads as %q, want %q", got, want)
			}
		}
	})
}

// TestRecordVocabulary_AnActionIsRecordedByItsCanonicalID checks the join key
// itself.
//
// An action ID is the one value a reader looks an action up by, in the record
// and in the catalog alike, so it has to be written exactly as the catalog
// spells it. Formatted through anything that decorated it — a surface prefix, a
// tool name — the coverage report would find no action by that name and call
// every one of them unexercised.
func TestRecordVocabulary_AnActionIsRecordedByItsCanonicalID(t *testing.T) {
	if got := issueListAction.String(); got != "issue.list" {
		t.Errorf("an action ID is recorded as %q, want the canonical %q", got, "issue.list")
	}
}
