// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package baseline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// File is the committed baseline, relative to the repository root.
const File = "docs/development/audit-1to1-baseline.json"

// SchemaVersion is the version of the file's format, and of the comparison
// report's. A baseline of another version is refused rather than compared,
// since its keys may be spelled another way.
const SchemaVersion = 1

// shownDifferences is how many differences of each kind a failure names; the
// report holds them all.
const shownDifferences = 5

// Baseline is every report-only finding of the tree, as keys by stream.
type Baseline struct {
	SchemaVersion int                 `json:"schema_version"`
	Streams       map[string][]string `json:"streams"`
}

// Marshal returns the baseline as indented JSON with a trailing newline, the
// streams in name order, which is the file -write-baseline writes.
func (b Baseline) Marshal() []byte {
	return append(cmdutil.Must(json.MarshalIndent(b, "", "  ")), '\n')
}

// Read reads the baseline at path, refusing a field the format does not
// have and another version of it.
func Read(path string) (Baseline, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- the committed baseline, read and never written
	if err != nil {
		return Baseline{}, fmt.Errorf("read the baseline: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var baseline Baseline
	if err = decoder.Decode(&baseline); err != nil {
		return Baseline{}, fmt.Errorf("decode the baseline %s: %w", path, err)
	}
	if baseline.SchemaVersion != SchemaVersion {
		return Baseline{}, fmt.Errorf("the baseline %s is of schema version %d and this command reads %d: make gen-audit-1to1-baseline rewrites it",
			path, baseline.SchemaVersion, SchemaVersion)
	}
	return baseline, nil
}

// Difference is one key the baseline and the tree disagree on.
type Difference struct {
	Stream string `json:"stream"`
	Key    string `json:"key"`
}

// Comparison is what -check-baseline reports.
type Comparison struct {
	SchemaVersion int `json:"schema_version"`
	// Baseline names the committed file the tree was compared with.
	Baseline string            `json:"baseline"`
	Summary  ComparisonSummary `json:"summary"`
	// New are the findings of the tree the baseline does not hold, and Stale
	// the keys of the baseline no finding of the tree answers.
	New   []Difference `json:"new"`
	Stale []Difference `json:"stale"`
}

// ComparisonSummary counts what the comparison held.
type ComparisonSummary struct {
	// Streams and Keys are what the tree holds, and New and Stale the
	// differences of each kind.
	Streams int `json:"streams"`
	Keys    int `json:"keys"`
	New     int `json:"new"`
	Stale   int `json:"stale"`
}

// Compare holds the tree's findings to the committed baseline, stream by
// stream, a stream either side lacks being an empty one there.
func Compare(committed, current Baseline) Comparison {
	comparison := Comparison{SchemaVersion: SchemaVersion, Baseline: File, New: []Difference{}, Stale: []Difference{}}
	names := map[string]bool{}
	for _, streams := range []map[string][]string{committed.Streams, current.Streams} {
		for name := range streams {
			names[name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		held, found := committed.Streams[name], current.Streams[name]
		comparison.New = append(comparison.New, absent(name, found, held)...)
		comparison.Stale = append(comparison.Stale, absent(name, held, found)...)
		comparison.Summary.Keys += len(found)
	}
	comparison.Summary.Streams = len(current.Streams)
	comparison.Summary.New = len(comparison.New)
	comparison.Summary.Stale = len(comparison.Stale)
	return comparison
}

// absent returns the keys of from that to does not hold, as differences of
// stream, in from's order.
func absent(stream string, from, to []string) []Difference {
	held := make(map[string]bool, len(to))
	for _, key := range to {
		held[key] = true
	}
	var out []Difference
	for _, key := range from {
		if !held[key] {
			out = append(out, Difference{Stream: stream, Key: key})
		}
	}
	return out
}

// Clean reports whether the tree holds exactly the baseline's findings.
func (c Comparison) Clean() bool {
	return len(c.New) == 0 && len(c.Stale) == 0
}

// Marshal returns the comparison as indented JSON with a trailing newline.
func (c Comparison) Marshal() []byte {
	return append(cmdutil.Must(json.MarshalIndent(c, "", "  ")), '\n')
}

// Failure is the line a failing check ends on: how many differences of each
// kind there are, the first of them with their stream, and what to run when
// the change means them.
func (c Comparison) Failure() string {
	var parts []string
	if len(c.New) > 0 {
		parts = append(parts, fmt.Sprintf("%d findings the baseline does not hold (%s)", len(c.New), named(c.New)))
	}
	if len(c.Stale) > 0 {
		parts = append(parts, fmt.Sprintf("%d keys of the baseline no finding answers (%s)", len(c.Stale), named(c.Stale)))
	}
	return "audit_1to1: " + strings.Join(parts, " and ") +
		"; when the change means them, make gen-audit-1to1-baseline records the tree (see report)"
}

// named spells the first differences as "stream: key", and how many more the
// report holds.
func named(differences []Difference) string {
	shown := make([]string, 0, shownDifferences)
	for _, difference := range differences[:min(len(differences), shownDifferences)] {
		shown = append(shown, difference.Stream+": "+difference.Key)
	}
	text := strings.Join(shown, "; ")
	if more := len(differences) - len(shown); more > 0 {
		text += fmt.Sprintf("; and %d more", more)
	}
	return text
}
