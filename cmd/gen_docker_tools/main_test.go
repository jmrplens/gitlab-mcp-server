package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestRun_DefaultMetaToolsOutputsSortedJSON verifies the generator can
// introspect the base meta-tool surface and emit deterministic Docker JSON.
func TestRun_DefaultMetaToolsOutputsSortedJSON(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	tools := decodeDockerTools(t, stdout.Bytes())
	if len(tools) == 0 {
		t.Fatal("run() emitted no tools")
	}
	if !slices.IsSortedFunc(tools, func(a, b dockerTool) int {
		return strings.Compare(a.Name, b.Name)
	}) {
		t.Fatalf("run() emitted tools out of order: %#v", tools)
	}
	if !hasDockerTool(tools, "gitlab_project") {
		t.Fatalf("run() emitted tools = %#v, want gitlab_project", tools)
	}
}

// TestRun_ModeFlags verifies enterprise and individual modes register their
// distinct tool surfaces instead of silently falling back to the base meta mode.
func TestRun_ModeFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantTool string
	}{
		{name: "enterprise", args: []string{"--enterprise"}, wantTool: "gitlab_geo"},
		{name: "individual", args: []string{"--individual"}, wantTool: "gitlab_project_list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run(tt.args, &stdout); err != nil {
				t.Fatalf("run() error = %v", err)
			}
			tools := decodeDockerTools(t, stdout.Bytes())
			if !hasDockerTool(tools, tt.wantTool) {
				t.Fatalf("run(%v) emitted %d tools, want %s", tt.args, len(tools), tt.wantTool)
			}
		})
	}
}

// TestRun_InvalidFlagReturnsError verifies flag parsing errors are returned to
// main without writing a misleading partial JSON payload.
func TestRun_InvalidFlagReturnsError(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--unknown"}, &stdout)
	if err == nil {
		t.Fatal("run() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "parse flags") {
		t.Fatalf("run() error = %v, want parse flags context", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

// TestRun_EncodeErrorIsReturned verifies that the JSON encode error path is
// returned to the caller when stdout rejects writes.
//
// The test swaps stdout for a writer that always errors so the encoder's
// Write call fails, ensuring the encode-error branch is covered.
func TestRun_EncodeErrorIsReturned(t *testing.T) {
	err := run(nil, errWriter{})
	if err == nil {
		t.Fatal("run() error = nil, want encode error")
	}
	if !strings.Contains(err.Error(), "encode") {
		t.Fatalf("run() error = %v, want encode context", err)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("encode write failed")
}

// TestSchemaArgs_SortsTopLevelPropertiesAndNormalizesTypes verifies Docker
// argument generation is deterministic and preserves property descriptions.
func TestSchemaArgs_SortsTopLevelPropertiesAndNormalizesTypes(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"zeta":  map[string]any{"type": []any{"integer", "null"}, "description": "last"},
			"alpha": map[string]any{"type": "string", "description": "first"},
			"beta":  map[string]any{"description": "fallback"},
		},
	}

	got := schemaArgs(schema)
	want := []dockerArg{
		{Name: "alpha", Type: "string", Desc: "first"},
		{Name: "beta", Type: "string", Desc: "fallback"},
		{Name: "zeta", Type: "integer", Desc: "last"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schemaArgs() = %#v, want %#v", got, want)
	}
}

// TestSchemaArgs_InvalidSchemasReturnNil verifies malformed or unmarshalable
// schema values fail closed instead of producing misleading arguments.
func TestSchemaArgs_InvalidSchemasReturnNil(t *testing.T) {
	tests := []struct {
		name   string
		schema any
	}{
		{name: "nil", schema: nil},
		{name: "marshal error", schema: func() {}},
		{name: "wrong properties shape", schema: map[string]any{"properties": "invalid"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaArgs(tt.schema); got != nil {
				t.Fatalf("schemaArgs() = %#v, want nil", got)
			}
		})
	}
}

// TestTypeString_Scenarios verifies JSON Schema type normalization handles the
// supported scalar and union shapes used by MCP input schemas.
func TestTypeString_Scenarios(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "string", in: "boolean", want: "boolean"},
		{name: "union", in: []any{"number", "null"}, want: "number"},
		{name: "empty union", in: []any{}, want: "string"},
		{name: "non string union", in: []any{42}, want: "string"},
		{name: "unknown", in: map[string]any{}, want: "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typeString(tt.in); got != tt.want {
				t.Fatalf("typeString(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func hasDockerTool(tools []dockerTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func decodeDockerTools(t *testing.T, data []byte) []dockerTool {
	t.Helper()
	var tools []dockerTool
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatalf("Unmarshal() error = %v, output = %q", err, string(data))
	}
	return tools
}
