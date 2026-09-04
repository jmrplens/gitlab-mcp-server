// run_test.go covers the parts of a scenario run that can be decided without
// starting a server: what the record is allowed to claim about processor time
// when a sample is missing or goes backwards.
package main

import (
	"strings"
	"testing"
	"time"
)

// TestCPUFigures_PublishesOnlyWhatWasMeasured verifies the load figures are
// omitted, with a note, whenever the difference behind them is not meaningful.
//
// sampleCPU used to report an unanswered sample as zero, which is
// indistinguishable from an idle process. The caller subtracted one sample
// from the other, so an unanswered second sample against an answered first one
// produced negative seconds and a negative percentage, and both were written
// into the committed record and drawn on the published charts.
func TestCPUFigures_PublishesOnlyWhatWasMeasured(t *testing.T) {
	const wall = 2 * time.Second
	cases := []struct {
		name             string
		startup, total   cpuSample
		loadWall         time.Duration
		wantLoadSeconds  float64
		wantLoadPercent  float64
		wantStartup      float64
		wantTotalSeconds float64
		wantNote         string
	}{
		{
			name:             "both answered",
			startup:          cpuSample{seconds: 1, ok: true},
			total:            cpuSample{seconds: 3, ok: true},
			loadWall:         wall,
			wantStartup:      1,
			wantTotalSeconds: 3,
			wantLoadSeconds:  2,
			wantLoadPercent:  100,
		},
		{
			name:        "second sample unanswered",
			startup:     cpuSample{seconds: 1.2, ok: true},
			total:       cpuSample{ok: false},
			loadWall:    wall,
			wantStartup: 1.2,
			wantNote:    "did not answer",
		},
		{
			name:             "first sample unanswered",
			startup:          cpuSample{ok: false},
			total:            cpuSample{seconds: 3, ok: true},
			loadWall:         wall,
			wantTotalSeconds: 3,
			wantNote:         "did not answer",
		},
		{
			name:             "consumed time fell between samples",
			startup:          cpuSample{seconds: 5, ok: true},
			total:            cpuSample{seconds: 4, ok: true},
			loadWall:         wall,
			wantStartup:      5,
			wantTotalSeconds: 4,
			wantNote:         "fell between samples",
		},
		{
			name:             "no load window to divide by",
			startup:          cpuSample{seconds: 1, ok: true},
			total:            cpuSample{seconds: 3, ok: true},
			loadWall:         0,
			wantStartup:      1,
			wantTotalSeconds: 3,
			wantLoadSeconds:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, notes := cpuFigures(tc.startup, tc.total, tc.loadWall)

			if cpu.StartupSeconds != tc.wantStartup {
				t.Errorf("StartupSeconds = %v, want %v", cpu.StartupSeconds, tc.wantStartup)
			}
			if cpu.TotalSeconds != tc.wantTotalSeconds {
				t.Errorf("TotalSeconds = %v, want %v", cpu.TotalSeconds, tc.wantTotalSeconds)
			}
			if cpu.LoadSeconds != tc.wantLoadSeconds {
				t.Errorf("LoadSeconds = %v, want %v", cpu.LoadSeconds, tc.wantLoadSeconds)
			}
			if cpu.LoadPercent != tc.wantLoadPercent {
				t.Errorf("LoadPercent = %v, want %v", cpu.LoadPercent, tc.wantLoadPercent)
			}
			// Nothing this function publishes may be negative: that is the
			// defect it exists to prevent, whatever the inputs were.
			if cpu.StartupSeconds < 0 || cpu.TotalSeconds < 0 || cpu.LoadSeconds < 0 || cpu.LoadPercent < 0 {
				t.Errorf("published a negative figure: %+v", cpu)
			}

			switch {
			case tc.wantNote == "" && len(notes) != 0:
				t.Errorf("notes = %v, want none", notes)
			case tc.wantNote != "" && len(notes) == 0:
				t.Errorf("notes are empty, want one saying %q", tc.wantNote)
			case tc.wantNote != "" && !strings.Contains(notes[0], tc.wantNote):
				t.Errorf("note = %q, want it to say %q", notes[0], tc.wantNote)
			}
		})
	}
}
