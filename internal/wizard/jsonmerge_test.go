// jsonmerge_test.go contains unit tests for the deep JSON merge function.

package wizard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStripJSONC validates the JSONC-to-JSON conversion, covering single-line
// comments, block comments, trailing commas, BOM prefix, and strings containing
// comment-like characters that should be preserved.
func TestStripJSONC(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // expected to be valid JSON after stripping
	}{
		{
			name:  "single-line comments removed",
			input: "{\n // comment\n \"a\": 1\n}",
			want:  `{"a":1}`,
		},
		{
			name:  "block comment removed",
			input: "{\n /* block\n comment */\n \"a\": 1\n}",
			want:  `{"a":1}`,
		},
		{
			name:  "trailing comma in object",
			input: `{"a": 1, "b": 2,}`,
			want:  `{"a":1,"b":2}`,
		},
		{
			name:  "trailing comma in array",
			input: `{"a": [1, 2,]}`,
			want:  `{"a":[1,2]}`,
		},
		{
			name:  "BOM prefix stripped",
			input: "\xEF\xBB\xBF{\"a\": 1}",
			want:  `{"a":1}`,
		},
		{
			name:  "slashes inside strings preserved",
			input: `{"url": "https://example.com/path"}`,
			want:  `{"url":"https://example.com/path"}`,
		},
		{
			name:  "double-slash inside string preserved",
			input: `{"note": "use // for comments"}`,
			want:  `{"note":"use // for comments"}`,
		},
		{
			name:  "escaped quotes in string",
			input: `{"val": "say \"hello\""}`,
			want:  `{"val":"say \"hello\""}`,
		},
		{
			name:  "mixed comments and trailing commas",
			input: "{\n // first\n \"a\": 1, /* inline */ \"b\": 2,\n}",
			want:  `{"a":1,"b":2}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripJSONC([]byte(tt.input))

			// Verify the output is valid JSON
			var gotParsed, wantParsed any
			if err := json.Unmarshal(got, &gotParsed); err != nil {
				t.Fatalf("stripJSONC output is not valid JSON: %v\ngot: %s", err, got)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantParsed); err != nil {
				t.Fatalf("test want is not valid JSON: %v", err)
			}

			// Compare by re-marshaling to canonical form
			gotJSON, _ := json.Marshal(gotParsed)
			wantJSON, _ := json.Marshal(wantParsed)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestReadJSONFile_MalformedJSON verifies that readJSONFile returns a
// descriptive error when the file contains invalid JSON.
func TestReadJSONFile_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json!}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readJSONFile(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// TestReadJSONFile_NotExists verifies that readJSONFile returns an
// os.IsNotExist error when the file doesn't exist.
func TestReadJSONFile_NotExists(t *testing.T) {
	_, err := readJSONFile(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

// TestReadJSONFile_ValidFile verifies readJSONFile reads and parses a clean
// JSON file correctly.
func TestReadJSONFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.json")
	content := `{"key": "value", "nested": {"inner": 42}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("key = %v, want %q", result["key"], "value")
	}
}

// TestWriteJSONFile_CreatesParentDirs verifies that writeJSONFile creates
// intermediate directories as needed.
func TestWriteJSONFile_CreatesParentDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	path := filepath.Join(dir, "config.json")

	data := map[string]any{"server": "gitlab"}
	if err := writeJSONFile(path, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was written and is valid JSON
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	var result map[string]any
	if err = json.Unmarshal(content, &result); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}
	if result["server"] != "gitlab" {
		t.Errorf("server = %v, want %q", result["server"], "gitlab")
	}
}

// TestWriteJSONFile_Roundtrip verifies that writing then reading a JSON file
// preserves the data structure.
func TestWriteJSONFile_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roundtrip.json")

	original := map[string]any{
		"servers": map[string]any{
			"gitlab": map[string]any{
				"command": "/bin/test",
			},
		},
	}
	if err := writeJSONFile(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	servers, ok := result["servers"].(map[string]any)
	if !ok {
		t.Fatal("missing 'servers' key")
	}
	gitlab, ok := servers["gitlab"].(map[string]any)
	if !ok {
		t.Fatal("missing 'gitlab' entry")
	}
	if gitlab["command"] != "/bin/test" {
		t.Errorf("command = %v, want %q", gitlab["command"], "/bin/test")
	}
}

// TestWriteJSONFile_MkdirAllFailure verifies writeJSONFile returns an error
// when the parent directory cannot be created (path collision with a file).
func TestWriteJSONFile_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file that will block MkdirAll from creating a subdirectory
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	targetPath := filepath.Join(blocker, "subdir", "config.json")
	err := writeJSONFile(targetPath, map[string]any{"a": 1})
	if err == nil {
		t.Fatal("expected error when parent path is a file, got nil")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("error = %v, want to contain 'creating directory'", err)
	}
}

// TestMergeServerEntry_InvalidExistingJSON verifies that MergeServerEntry
// returns an error when the existing config file has unparseable content.
func TestMergeServerEntry_InvalidExistingJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(path, []byte("NOT JSON AT ALL"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"command": "/bin/test"}
	err := MergeServerEntry(path, "servers", "gitlab", entry)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestMergeServerEntry_RootKeyNotObject verifies that MergeServerEntry
// replaces a non-object root key value with a proper map.
func TestMergeServerEntry_RootKeyNotObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Root key exists but is a string, not a map
	if err := os.WriteFile(path, []byte(`{"servers": "not-a-map"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"command": "/bin/test"}
	if err := MergeServerEntry(path, "servers", "gitlab", entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	servers, ok := result["servers"].(map[string]any)
	if !ok {
		t.Fatal("servers should be a map after merge")
	}
	if _, ok = servers["gitlab"]; !ok {
		t.Error("gitlab entry not added")
	}
}

// TestWriteJSONFile_MarshalError verifies writeJSONFile returns an error
// when the data contains an unmarshable value (channel).
func TestWriteJSONFile_MarshalError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	ch := make(chan int)
	data := map[string]any{"bad": ch}

	err := writeJSONFile(path, data)
	if err == nil {
		t.Fatal("expected error for unmarshable value, got nil")
	}
	if !strings.Contains(err.Error(), "marshaling JSON") {
		t.Errorf("error = %v, want to contain 'marshaling JSON'", err)
	}
}

// TestWriteJSONFile_WriteFileFails verifies writeJSONFile returns an error
// when the file cannot be written (read-only directory).
func TestWriteJSONFile_WriteFileFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping: test requires non-root")
	}
	tmpDir := t.TempDir()
	readOnly := filepath.Join(tmpDir, "locked")
	if err := os.Mkdir(readOnly, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })

	path := filepath.Join(readOnly, "test.json")
	data := map[string]any{"key": "value"}

	err := writeJSONFile(path, data)
	if err == nil {
		t.Fatal("expected error when write fails, got nil")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Errorf("error = %v, want to contain 'writing'", err)
	}
}
