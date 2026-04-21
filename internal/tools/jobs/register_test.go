package jobs

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_ConfirmDeclined covers the ConfirmAction early-return
// branches in erase, delete_artifacts, and delete_project_artifacts handlers
// when the user declines the destructive action confirmation.
func TestRegisterTools_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_job_erase", map[string]any{"project_id": "42", "job_id": 1}},
		{"gitlab_job_delete_artifacts", map[string]any{"project_id": "42", "job_id": 1}},
		{"gitlab_job_delete_project_artifacts", map[string]any{"project_id": "42"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

// TestToOutput_OptionalFields verifies that ToOutput correctly formats
// optional pointer fields (ArtifactsExpireAt, User, Runner, ErasedAt, Commit)
// that are nil by default but populated in some API responses.
func TestToOutput_OptionalFields(t *testing.T) {
	now := time.Now()
	job := &gl.Job{
		ID:                1,
		Name:              "test-job",
		Stage:             "test",
		Status:            "success",
		Ref:               "main",
		Pipeline:          gl.JobPipeline{ID: 10},
		ArtifactsExpireAt: &now,
		User:              &gl.User{Username: "admin"},
		Runner:            gl.JobRunner{ID: 5},
		ErasedAt:          &now,
		Commit: &gl.Commit{
			ID: "abc123def456",
		},
	}
	out := ToOutput(job)

	if out.ArtifactsExpireAt == "" {
		t.Error("expected ArtifactsExpireAt to be set")
	}
	if out.UserUsername != "admin" {
		t.Errorf("UserUsername = %q, want %q", out.UserUsername, "admin")
	}
	if out.RunnerID != 5 {
		t.Errorf("RunnerID = %d, want 5", out.RunnerID)
	}
	if out.ErasedAt == "" {
		t.Error("expected ErasedAt to be set")
	}
	if out.CommitSHA != "abc123def456" {
		t.Errorf("CommitSHA = %q, want %q", out.CommitSHA, "abc123def456")
	}
}

// TestBridgeToOutput_OptionalFields verifies that BridgeToOutput correctly
// formats optional pointer fields (User, DownstreamPipeline) when populated.
func TestBridgeToOutput_OptionalFields(t *testing.T) {
	now := time.Now()
	bridge := &gl.Bridge{
		ID:     2,
		Name:   "trigger",
		Stage:  "deploy",
		Status: "success",
		Ref:    "main",
		User:   &gl.User{Username: "deployer"},
		DownstreamPipeline: &gl.PipelineInfo{
			ID: 99,
		},
		CreatedAt:  &now,
		StartedAt:  &now,
		FinishedAt: &now,
	}
	out := BridgeToOutput(bridge)

	if out.UserUsername != "deployer" {
		t.Errorf("UserUsername = %q, want %q", out.UserUsername, "deployer")
	}
	if out.DownstreamPipeline != 99 {
		t.Errorf("DownstreamPipeline = %d, want 99", out.DownstreamPipeline)
	}
	if out.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
	if out.StartedAt == "" {
		t.Error("expected StartedAt to be set")
	}
	if out.FinishedAt == "" {
		t.Error("expected FinishedAt to be set")
	}
}

// TestTrace_Truncation covers the truncation branch in Trace when the
// trace log exceeds maxTraceBytes (100KB).
func TestTrace_Truncation(t *testing.T) {
	bigContent := strings.Repeat("A", maxTraceBytes+500)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(bigContent))
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("expected Truncated=true for oversized trace")
	}
	if len(out.Trace) != maxTraceBytes {
		t.Errorf("trace length = %d, want %d", len(out.Trace), maxTraceBytes)
	}
}

// TestReadArtifactContent_Truncation covers the truncation branch in
// readArtifactContent when content exceeds maxArtifactBytes (1MB).
func TestReadArtifactContent_Truncation(t *testing.T) {
	bigData := bytes.Repeat([]byte("X"), maxArtifactBytes+500)
	reader := bytes.NewReader(bigData)

	out, err := readArtifactContent(reader, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("expected Truncated=true for oversized artifact")
	}
	if out.Size != maxArtifactBytes {
		t.Errorf("Size = %d, want %d", out.Size, maxArtifactBytes)
	}
}

// TestReadSingleArtifactContent_Truncation covers the truncation branch in
// readSingleArtifactContent when content exceeds maxArtifactBytes (1MB).
func TestReadSingleArtifactContent_Truncation(t *testing.T) {
	bigData := bytes.Repeat([]byte("Y"), maxArtifactBytes+500)
	reader := bytes.NewReader(bigData)

	out, err := readSingleArtifactContent(reader, 1, "report.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("expected Truncated=true for oversized artifact")
	}
	if out.Size != maxArtifactBytes {
		t.Errorf("Size = %d, want %d", out.Size, maxArtifactBytes)
	}
}

// TestReadArtifactContent_ReadError covers the read error branch in
// readArtifactContent when the reader returns an unexpected error.
func TestReadArtifactContent_ReadError(t *testing.T) {
	_, err := readArtifactContent(&errorReader{}, 1)
	if err == nil {
		t.Fatal("expected error for failing reader, got nil")
	}
}

// TestReadSingleArtifactContent_ReadError covers the read error branch in
// readSingleArtifactContent when the reader returns an unexpected error.
func TestReadSingleArtifactContent_ReadError(t *testing.T) {
	_, err := readSingleArtifactContent(&errorReader{}, 1, "file.txt")
	if err == nil {
		t.Fatal("expected error for failing reader, got nil")
	}
}

// TestFormatOutputMarkdown_OptionalBranches covers the optional formatting
// branches in FormatOutputMarkdown (CommitSHA, Coverage, FailureReason, etc.).
func TestFormatOutputMarkdown_OptionalBranches(t *testing.T) {
	out := Output{
		ID:             1,
		Name:           "build",
		Stage:          "build",
		Status:         "failed",
		Ref:            "main",
		CommitSHA:      "abc123def456789",
		Duration:       45.5,
		QueuedDuration: 2.5,
		FailureReason:  "script_failure",
		Coverage:       85.5,
		UserUsername:   "admin",
		CreatedAt:      "2024-01-01T00:00:00Z",
		WebURL:         "https://gitlab.com/job/1",
	}
	md := FormatOutputMarkdown(out)

	checks := []string{
		"abc123def456", // truncated commit SHA
		"45.5s",
		"2.5s",
		"script_failure",
		"85.5%",
		"admin",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

// errorReader is a test helper that always returns an error on Read.
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
