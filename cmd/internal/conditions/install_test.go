// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// TestGobco_IsTheModuleAtTheRelease holds the two spellings of the pin to each
// other, so a bump moves both.
func TestGobco_IsTheModuleAtTheRelease(t *testing.T) {
	t.Parallel()
	if want := "github.com/rillig/gobco@" + Release; Gobco != want {
		t.Errorf("Gobco = %q, want %q", Gobco, want)
	}
}

// TestInstall_ThePause_IsTheOneAsked verifies what is waited between two
// attempts: ten seconds unless the environment says another number, and never
// after the last.
func TestInstall_ThePause_IsTheOneAsked(t *testing.T) {
	rec := readRecord(t)
	for _, tc := range []struct {
		name string
		env  []string
		want time.Duration
	}{{"unset", nil, 10 * time.Second}, {"set", []string{"GOBCO_INSTALL_PAUSE=3"}, 3 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			b.environ = append(b.environ, tc.env...)
			b.installFailures = 3
			var out, errs bytes.Buffer
			if status := Install(t.Context(), b.tools(), b.environ, &out, &errs); status != NotMeasured {
				t.Errorf("status = %d, want 1", status)
			}
			if want := []time.Duration{tc.want, tc.want}; !slices.Equal(b.pauses, want) {
				t.Errorf("paused %v, want %v", b.pauses, want)
			}
			if want := "trying again in " + tc.want.String(); !strings.Contains(errs.String(), want) {
				t.Errorf("stderr = %q, want %q in it", errs.String(), want)
			}
		})
	}
}

// TestInstall_AGoCommandThatCannotStart_IsAnAttemptThatFailed verifies that a
// go install that could not be started is said and tried again like one that
// failed.
func TestInstall_AGoCommandThatCannotStart_IsAnAttemptThatFailed(t *testing.T) {
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	b.plant = func(call recipe.Call) (bool, int, error) { return true, 0, errors.New("planted") }
	var out, errs bytes.Buffer
	if status := Install(t.Context(), b.tools(), b.environ, &out, &errs); status != NotMeasured {
		t.Errorf("status = %d, want 1", status)
	}
	if got := strings.Count(errs.String(), "gobco: planted\n"); got != 3 || !strings.HasSuffix(errs.String(), "failed 3 times; giving up\n") {
		t.Errorf("stderr = %q, want the reason three times and the last verdict", errs.String())
	}
}

// TestInstall_AnInterrupt_EndsItWithTheSignalsStatus verifies that a signal
// during the pause ends the install with the status a shell gives it, having
// tried no more.
func TestInstall_AnInterrupt_EndsItWithTheSignalsStatus(t *testing.T) {
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	b.installFailures = 3
	ctx, interrupt := interruptible(t)
	tools := b.tools()
	paused := 0
	tools.Pause = func(ctx context.Context, _ time.Duration) {
		paused++
		interrupt(os.Interrupt)
		<-ctx.Done()
	}
	var out, errs bytes.Buffer
	if status := Install(ctx, tools, b.environ, &out, &errs); status != 130 || b.installs != 1 || paused != 1 {
		t.Errorf("status = %d after %d attempt(s) and %d pause(s), want 130 after one of each", status, b.installs, paused)
	}
}

// TestPinnedOnPath_AGobcoThatDoesNotAnswer_IsNotThePinnedOne verifies that a
// gobco that exits non-zero when asked for its version, or that cannot be
// started, is not run, and that none is asked once the run is ending.
func TestPinnedOnPath_AGobcoThatDoesNotAnswer_IsNotThePinnedOne(t *testing.T) {
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	b.planted["gobin"] = "1.3.4"
	if pinned, _ := pinnedOnPath(t.Context(), b.tools(), b.environ); pinned == "" {
		t.Fatal("the planted gobco was not found")
	}
	b.plant = answering("-version", "1.3.4\n", 1, nil)
	if pinned, found := pinnedOnPath(t.Context(), b.tools(), b.environ); pinned != "" || !strings.HasSuffix(found, ", which reports no version") {
		t.Errorf("pinnedOnPath() = %q, %q; want none", pinned, found)
	}
	ctx, interrupt := interruptible(t)
	interrupt(os.Interrupt)
	asked := false
	b.plant = func(recipe.Call) (bool, int, error) { asked = true; return false, 0, nil }
	if pinned, _ := pinnedOnPath(ctx, b.tools(), b.environ); pinned != "" || asked {
		t.Errorf("pinnedOnPath() under an interrupt = %q, asked %t; want none asked", pinned, asked)
	}
}
