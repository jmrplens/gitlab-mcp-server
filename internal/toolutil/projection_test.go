package toolutil

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProjectOutput_PrunesNoiseKeys(t *testing.T) {
	input := map[string]any{
		"id":          123,
		"name":        "my-awesome-project",
		"avatar_url":  "https://gitlab.example.com/uploads/-/system/project/avatar/123/avatar.png",
		"gravatar_id": "d41d8cd98f00b204e9800998ecf8427e",
		"_links": map[string]any{
			"self":     "https://gitlab.example.com/api/v4/projects/123",
			"issues":   "https://gitlab.example.com/api/v4/projects/123/issues",
			"repo_url": "https://gitlab.example.com/group/my-awesome-project.git",
		},
		"empty_string": "",
		"nil_field":    nil,
		"owner": map[string]any{
			"id":         45,
			"username":   "alice",
			"avatar_url": "https://gitlab.example.com/uploads/-/system/user/avatar/45/avatar.png",
		},
	}

	result := ProjectOutput(input)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if _, exists := m["_links"]; exists {
		t.Errorf("expected _links to be pruned")
	}
	if _, exists := m["avatar_url"]; exists {
		t.Errorf("expected avatar_url to be pruned")
	}
	if _, exists := m["gravatar_id"]; exists {
		t.Errorf("expected gravatar_id to be pruned")
	}
	if _, exists := m["empty_string"]; exists {
		t.Errorf("expected empty_string to be pruned")
	}
	if _, exists := m["nil_field"]; exists {
		t.Errorf("expected nil_field to be pruned")
	}

	// Verify preserved functional data
	if m["id"] != 123 {
		t.Errorf("expected id 123, got %v", m["id"])
	}
	if m["name"] != "my-awesome-project" {
		t.Errorf("expected name 'my-awesome-project', got %v", m["name"])
	}

	owner, ok := m["owner"].(map[string]any)
	if !ok {
		t.Fatalf("expected owner map[string]any, got %T", m["owner"])
	}
	if owner["username"] != "alice" {
		t.Errorf("expected owner username 'alice', got %v", owner["username"])
	}
	if _, exists := owner["avatar_url"]; exists {
		t.Errorf("expected owner avatar_url to be pruned")
	}
}

func TestProjectOutput_SlicePreservation(t *testing.T) {
	input := []any{
		map[string]any{
			"id":         1,
			"avatar_url": "http://example.com/avatar1.png",
			"title":      "Issue 1",
		},
		map[string]any{
			"id":         2,
			"avatar_url": "http://example.com/avatar2.png",
			"title":      "Issue 2",
		},
	}

	result := ProjectOutput(input)
	slice, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(slice) != 2 {
		t.Fatalf("expected 2 items, got %d", len(slice))
	}
	item0 := slice[0].(map[string]any)
	if item0["title"] != "Issue 1" {
		t.Errorf("expected title 'Issue 1', got %v", item0["title"])
	}
	if _, exists := item0["avatar_url"]; exists {
		t.Errorf("expected avatar_url to be pruned from slice item")
	}
}

func TestProjectOutput_EmptyCollections(t *testing.T) {
	emptyList := []any{}
	resList := ProjectOutput(emptyList)
	if s, ok := resList.([]any); !ok || len(s) != 0 {
		t.Errorf("expected empty []any, got %v", resList)
	}

	emptyMap := map[string]any{}
	resMap := ProjectOutput(emptyMap)
	if m, ok := resMap.(map[string]any); !ok || len(m) != 0 {
		t.Errorf("expected empty map[string]any, got %v", resMap)
	}

	if ProjectOutput(nil) != nil {
		t.Errorf("expected nil for nil input")
	}
}

func TestProjectOutput_CompressionBenchmark(t *testing.T) {
	// Sample payload simulating GitLab project API response with extensive links and metadata
	sampleProject := map[string]any{
		"id":                  42,
		"description":         "Enterprise Core Repository",
		"name":                "core-platform",
		"path":                "core-platform",
		"default_branch":      "main",
		"visibility":          "private",
		"avatar_url":          "https://gitlab.example.com/uploads/project/avatar/42/logo.png",
		"gravatar_id":         "abcdef1234567890",
		"web_url":             "https://gitlab.example.com/group/core-platform",
		"node_id":             "gid://gitlab/Project/42",
		"site_admin":          false,
		"starred_url":         "https://gitlab.example.com/api/v4/projects/42/star",
		"subscriptions_url":   "https://gitlab.example.com/api/v4/projects/42/subscriptions",
		"organizations_url":   "https://gitlab.example.com/api/v4/projects/42/orgs",
		"repos_url":           "https://gitlab.example.com/api/v4/projects/42/repos",
		"events_url":          "https://gitlab.example.com/api/v4/projects/42/events",
		"received_events_url": "https://gitlab.example.com/api/v4/projects/42/received_events",
		"gists_url":           "https://gitlab.example.com/api/v4/projects/42/gists",
		"following_url":       "https://gitlab.example.com/api/v4/projects/42/following",
		"followers_url":       "https://gitlab.example.com/api/v4/projects/42/followers",
		"_links": map[string]any{
			"self":           "https://gitlab.example.com/api/v4/projects/42",
			"issues":         "https://gitlab.example.com/api/v4/projects/42/issues",
			"merge_requests": "https://gitlab.example.com/api/v4/projects/42/merge_requests",
			"repo_branches":  "https://gitlab.example.com/api/v4/projects/42/repository/branches",
			"labels":         "https://gitlab.example.com/api/v4/projects/42/labels",
			"events":         "https://gitlab.example.com/api/v4/projects/42/events",
			"members":        "https://gitlab.example.com/api/v4/projects/42/members",
		},
		"empty_description": "",
		"null_status":       nil,
		"creator": map[string]any{
			"id":         1,
			"username":   "devops-admin",
			"name":       "DevOps Administrator",
			"avatar_url": "https://gitlab.example.com/uploads/user/avatar/1/avatar.png",
			"web_url":    "https://gitlab.example.com/devops-admin",
		},
	}

	rawBytes, _ := json.Marshal(sampleProject)
	projected := ProjectOutput(sampleProject)
	projBytes, _ := json.Marshal(projected)

	rawLen := len(rawBytes)
	projLen := len(projBytes)
	savedPct := float64(rawLen-projLen) / float64(rawLen) * 100.0

	t.Logf("Raw payload size:       %d bytes", rawLen)
	t.Logf("Projected payload size: %d bytes", projLen)
	t.Logf("Payload compression:    %.1f%% reduction", savedPct)

	if savedPct < 50.0 {
		t.Errorf("expected at least 50%% reduction, got %.1f%%", savedPct)
	}
}

func TestDefaultFormatResult_WithAndWithoutLeanEnv(t *testing.T) {
	sample := map[string]any{
		"id":         100,
		"avatar_url": "https://gitlab.example.com/avatar.png",
		"name":       "demo",
	}

	// 1. Without lean mode (default: false)
	t.Setenv("GITLAB_MCP_LEAN_OUTPUT", "false")
	resUnprojected := defaultFormatResult(sample)
	textUnprojected := resUnprojected.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(textUnprojected, "avatar_url") {
		t.Errorf("expected avatar_url when lean mode is disabled")
	}

	// 2. With lean mode
	t.Setenv("GITLAB_MCP_LEAN_OUTPUT", "true")
	resProjected := defaultFormatResult(sample)
	textProjected := resProjected.Content[0].(*mcp.TextContent).Text
	if strings.Contains(textProjected, "avatar_url") {
		t.Errorf("did not expect avatar_url when lean mode is enabled")
	}
	if !strings.Contains(textProjected, `"id":100`) || !strings.Contains(textProjected, `"name":"demo"`) {
		t.Errorf("expected functional fields preserved, got %s", textProjected)
	}
}

func TestProjectOutput_StructAndNil(t *testing.T) {
	if got := ProjectOutput(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}

	type DemoStruct struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Empty     string `json:"empty"`
	}

	st := DemoStruct{
		ID:        42,
		Name:      "struct-test",
		AvatarURL: "https://example.com/avatar.png",
	}

	result := ProjectOutput(st)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if _, exists := m["avatar_url"]; exists {
		t.Errorf("expected avatar_url to be pruned")
	}
	if _, exists := m["empty"]; exists {
		t.Errorf("expected empty to be pruned")
	}
	if m["name"] != "struct-test" {
		t.Errorf("expected name 'struct-test', got %v", m["name"])
	}
}

func TestProjectOutput_PreservesHtmlUrl(t *testing.T) {
	input := map[string]any{
		"id":       100,
		"html_url": "https://gitlab.example.com/project/-/blob/main/LICENSE",
	}
	result := ProjectOutput(input)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["html_url"] != "https://gitlab.example.com/project/-/blob/main/LICENSE" {
		t.Errorf("expected html_url to be preserved, got %v", m["html_url"])
	}
}

func TestProjectOutput_PreservesLargeIntegerPrecision(t *testing.T) {
	type BigIntStruct struct {
		BigID int64  `json:"big_id"`
		Name  string `json:"name"`
	}
	// 9007199254740993 is 2^53 + 1, which loses precision in float64 (rounds to 9007199254740992)
	const expectedBigID int64 = 9007199254740993
	st := BigIntStruct{
		BigID: expectedBigID,
		Name:  "precision-test",
	}

	result := ProjectOutput(st)
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal projected result: %v", err)
	}

	jsonStr := string(b)
	if !strings.Contains(jsonStr, `"big_id":9007199254740993`) {
		t.Errorf("expected exact big_id 9007199254740993, got %s", jsonStr)
	}
}

func TestProjectOutput_PreservesEmptyContentKeys(t *testing.T) {
	input := map[string]any{
		"key":         "DEPLOY_TOKEN",
		"value":       "",
		"file_name":   "empty.txt",
		"content":     "",
		"diff":        "",
		"description": "",
		"empty_field": "",
	}

	result := ProjectOutput(input)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if val, exists := m["value"]; !exists || val != "" {
		t.Errorf("expected 'value' to be preserved as empty string, got %v (exists: %v)", val, exists)
	}
	if val, exists := m["content"]; !exists || val != "" {
		t.Errorf("expected 'content' to be preserved as empty string, got %v (exists: %v)", val, exists)
	}
	if val, exists := m["diff"]; !exists || val != "" {
		t.Errorf("expected 'diff' to be preserved as empty string, got %v (exists: %v)", val, exists)
	}
	if _, exists := m["description"]; exists {
		t.Errorf("expected 'description' with empty string to be pruned")
	}
	if _, exists := m["empty_field"]; exists {
		t.Errorf("expected 'empty_field' with empty string to be pruned")
	}
}
