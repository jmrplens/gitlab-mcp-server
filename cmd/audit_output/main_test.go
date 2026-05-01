package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

var errNoop = errors.New("noop")

func TestShouldSkipRouteOutputSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{name: "sampling meta-tool", toolName: "gitlab_analyze", want: true},
		{name: "package meta-tool", toolName: "gitlab_package", want: false},
		{name: "runner meta-tool", toolName: "gitlab_runner", want: false},
		{name: "empty tool name", toolName: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldSkipRouteOutputSchema(tt.toolName, "action", toolutil.ActionRoute{}); got != tt.want {
				t.Fatalf("shouldSkipRouteOutputSchema(%q) = %v, want %v", tt.toolName, got, tt.want)
			}
		})
	}
}

func TestCollectRouteOutputSchemaFindings(t *testing.T) {
	t.Parallel()

	noop := func(context.Context, map[string]any) (any, error) { return nil, errNoop }
	routes := map[string]toolutil.ActionMap{
		"gitlab_analyze": {
			"issue_summary": {Handler: noop},
		},
		"gitlab_package": {
			"missing": {Handler: noop},
			"valid": {
				Handler:      noop,
				OutputSchema: toolutil.SchemaForRoute[toolutil.VoidOutput](),
			},
		},
	}

	got := collectRouteOutputSchemaFindings(routes)
	if len(got) != 1 {
		t.Fatalf("collectRouteOutputSchemaFindings returned %d findings, want 1: %#v", len(got), got)
	}
	if got[0].tool != "gitlab_package" {
		t.Fatalf("finding tool = %q, want gitlab_package", got[0].tool)
	}
	if got[0].category != "route-output-schema" {
		t.Fatalf("finding category = %q, want route-output-schema", got[0].category)
	}
}
