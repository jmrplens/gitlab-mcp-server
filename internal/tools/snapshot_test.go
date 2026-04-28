// snapshot_test.go verifies that MCP tool definitions have not changed
// unexpectedly. It serializes all tool metadata to deterministic JSON and
// compares against golden files. Set UPDATE_TOOLSNAPS=true to regenerate.

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolSnapshot captures the fields we care about for snapshot comparison.
type toolSnapshot struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	InputSchema  json.RawMessage      `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage      `json:"outputSchema,omitempty"`
	Annotations  *mcp.ToolAnnotations `json:"annotations,omitempty"`
}

// TestToolSnapshots_Individual compares individual-mode tool definitions
// against the golden file testdata/tools_individual.json.
func TestToolSnapshots_Individual(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	session := newMCPSession(t, handler, true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	snapshots := buildSnapshots(t, result.Tools)
	goldenPath := filepath.Join("testdata", "tools_individual.json")
	compareOrUpdate(t, goldenPath, snapshots)
}

// TestToolSnapshots_Meta compares meta-tool definitions against the
// golden file testdata/tools_meta.json.
func TestToolSnapshots_Meta(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	session := newMetaMCPSession(t, handler, true)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	snapshots := buildSnapshots(t, result.Tools)
	goldenPath := filepath.Join("testdata", "tools_meta.json")
	compareOrUpdate(t, goldenPath, snapshots)
}

// buildSnapshots extracts snapshot data from MCP tool definitions,
// sorted alphabetically by name for deterministic output.
func buildSnapshots(t *testing.T, tools []*mcp.Tool) []toolSnapshot {
	t.Helper()
	snaps := make([]toolSnapshot, 0, len(tools))
	for _, tool := range tools {
		s := toolSnapshot{
			Name:        tool.Name,
			Description: tool.Description,
			Annotations: tool.Annotations,
		}
		if tool.InputSchema != nil {
			raw, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("marshal InputSchema for %s: %v", tool.Name, err)
			}
			s.InputSchema = raw
		}
		if tool.OutputSchema != nil {
			raw, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatalf("marshal OutputSchema for %s: %v", tool.Name, err)
			}
			s.OutputSchema = raw
		}
		snaps = append(snaps, s)
	}
	slices.SortFunc(snaps, func(a, b toolSnapshot) int {
		return strings.Compare(a.Name, b.Name)
	})
	return snaps
}

// compareOrUpdate either updates the golden file or compares current
// output against it, reporting a clear diff on mismatch.
func compareOrUpdate(t *testing.T, goldenPath string, current []toolSnapshot) {
	t.Helper()

	got, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		t.Fatalf("marshal current snapshots: %v", err)
	}

	if os.Getenv("UPDATE_TOOLSNAPS") == "true" {
		if mkdirErr := os.MkdirAll(filepath.Dir(goldenPath), 0o755); mkdirErr != nil {
			t.Fatalf("create testdata dir: %v", mkdirErr)
		}
		if writeErr := os.WriteFile(goldenPath, got, 0o644); writeErr != nil {
			t.Fatalf("write golden file: %v", writeErr)
		}
		t.Logf("Updated golden file: %s (%d tools)", goldenPath, len(current))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v\nRun with UPDATE_TOOLSNAPS=true to generate", goldenPath, err)
	}

	if string(got) == string(want) {
		return
	}

	// Parse both for structured diff
	var wantSnaps []toolSnapshot
	if unmarshalErr := json.Unmarshal(want, &wantSnaps); unmarshalErr != nil {
		t.Fatalf("parse golden file: %v", unmarshalErr)
	}

	reportDiff(t, goldenPath, wantSnaps, current)
}

// reportDiff produces a human-readable diff showing which tools were
// added, removed, or changed between the golden and current snapshots.
func reportDiff(t *testing.T, goldenPath string, want, got []toolSnapshot) {
	t.Helper()

	wantMap := make(map[string]toolSnapshot, len(want))
	for _, s := range want {
		wantMap[s.Name] = s
	}
	gotMap := make(map[string]toolSnapshot, len(got))
	for _, s := range got {
		gotMap[s.Name] = s
	}

	var diffs []string

	// Removed tools
	for name := range wantMap {
		if _, ok := gotMap[name]; !ok {
			diffs = append(diffs, "REMOVED tool: "+name)
		}
	}

	// Added tools
	for name := range gotMap {
		if _, ok := wantMap[name]; !ok {
			diffs = append(diffs, "ADDED tool: "+name)
		}
	}

	// Changed tools
	for name, wSnap := range wantMap {
		gSnap, ok := gotMap[name]
		if !ok {
			continue
		}
		if wSnap.Description != gSnap.Description {
			diffs = append(diffs, "CHANGED "+name+" description:\n  old: "+wSnap.Description+"\n  new: "+gSnap.Description)
		}
		if string(wSnap.InputSchema) != string(gSnap.InputSchema) {
			diffs = append(diffs, "CHANGED "+name+" inputSchema:\n  old: "+string(wSnap.InputSchema)+"\n  new: "+string(gSnap.InputSchema))
		}
		if string(wSnap.OutputSchema) != string(gSnap.OutputSchema) {
			diffs = append(diffs, "CHANGED "+name+" outputSchema:\n  old: "+string(wSnap.OutputSchema)+"\n  new: "+string(gSnap.OutputSchema))
		}
		wAnn, err := json.Marshal(wSnap.Annotations)
		if err != nil {
			t.Fatalf("marshal want annotations for %s: %v", name, err)
		}
		gAnn, err := json.Marshal(gSnap.Annotations)
		if err != nil {
			t.Fatalf("marshal got annotations for %s: %v", name, err)
		}
		if string(wAnn) != string(gAnn) {
			diffs = append(diffs, "CHANGED "+name+" annotations:\n  old: "+string(wAnn)+"\n  new: "+string(gAnn))
		}
	}

	slices.Sort(diffs)

	t.Errorf("Tool snapshots changed (%s). Found %d difference(s):\n%s\n\nRun with UPDATE_TOOLSNAPS=true to update golden files.",
		goldenPath, len(diffs), strings.Join(diffs, "\n"))
}
