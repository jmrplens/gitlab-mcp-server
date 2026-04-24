// bulkimports_test.go contains unit tests for the bulk import migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package bulkimports

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStartMigration verifies the behavior of start migration.
func TestStartMigration(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/bulk_imports" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 42,
			"status": "created",
			"source_type": "gitlab",
			"source_url": "https://source.gitlab.com",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"has_failures": false
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := StartMigration(t.Context(), client, StartMigrationInput{
		URL:         "https://source.gitlab.com",
		AccessToken: "glpat-test",
		Entities: []EntityInput{
			{
				SourceType:           "group_entity",
				SourceFullPath:       "source-group",
				DestinationSlug:      "dest-group",
				DestinationNamespace: "dest-ns",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.Status != "created" {
		t.Errorf("Status = %q, want created", out.Status)
	}
	if out.HasFailures {
		t.Error("HasFailures = true, want false")
	}
}

// TestStartMigration_Error verifies that StartMigration handles the error scenario correctly.
func TestStartMigration_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := StartMigration(t.Context(), client, StartMigrationInput{
		URL:         "https://source.gitlab.com",
		AccessToken: "bad",
		Entities:    []EntityInput{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatStartMigrationMarkdown verifies the behavior of format start migration markdown.
func TestFormatStartMigrationMarkdown(t *testing.T) {
	out := MigrationOutput{
		ID:          1,
		Status:      "created",
		SourceType:  "gitlab",
		SourceURL:   "https://src.example.com",
		CreatedAt:   "2026-01-01",
		UpdatedAt:   "2026-01-01",
		HasFailures: false,
	}
	md := FormatStartMigrationMarkdown(out)
	if !strings.Contains(md, "Bulk Import") {
		t.Error("missing title")
	}
	if !strings.Contains(md, "created") {
		t.Error("missing status")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// StartMigration — with optional fields
// ---------------------------------------------------------------------------.

// TestStartMigration_WithOptionalFields verifies the behavior of start migration with optional fields.
func TestStartMigration_WithOptionalFields(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/bulk_imports" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 100,
				"status": "created",
				"source_type": "gitlab",
				"source_url": "https://source.gitlab.com",
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-01T00:00:00Z",
				"has_failures": false
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	migrateProjects := true
	migrateMemberships := false
	out, err := StartMigration(t.Context(), client, StartMigrationInput{
		URL:         "https://source.gitlab.com",
		AccessToken: "glpat-test",
		Entities: []EntityInput{
			{
				SourceType:           "group_entity",
				SourceFullPath:       "source-group",
				DestinationSlug:      "dest-group",
				DestinationNamespace: "dest-ns",
				MigrateProjects:      &migrateProjects,
				MigrateMemberships:   &migrateMemberships,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("ID = %d, want 100", out.ID)
	}
}

// ---------------------------------------------------------------------------
// FormatStartMigrationMarkdown — with failures
// ---------------------------------------------------------------------------.

// TestFormatStartMigrationMarkdown_WithFailures verifies the behavior of format start migration markdown with failures.
func TestFormatStartMigrationMarkdown_WithFailures(t *testing.T) {
	out := MigrationOutput{
		ID:          2,
		Status:      "failed",
		SourceType:  "gitlab",
		SourceURL:   "https://src|pipe.example.com",
		CreatedAt:   "2026-06-01",
		UpdatedAt:   "2026-06-02",
		HasFailures: true,
	}
	md := FormatStartMigrationMarkdown(out)
	if !strings.Contains(md, "failed") {
		t.Error("missing status")
	}
	if !strings.Contains(md, "true") {
		t.Error("missing has_failures=true")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip verifies the behavior of m c p round trip.
func TestMCPRound_Trip(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/bulk_imports", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 42,
			"status": "created",
			"source_type": "gitlab",
			"source_url": "https://source.gitlab.com",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"has_failures": false
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_start_bulk_import",
		Arguments: map[string]any{
			"url":          "https://source.gitlab.com",
			"access_token": "glpat-test",
			"entities": []any{
				map[string]any{
					"source_type":           "group_entity",
					"source_full_path":      "source-group",
					"destination_slug":      "dest-group",
					"destination_namespace": "dest-ns",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool returned IsError=true")
	}
}

// TestMCPRound_TripAPIError verifies the register handler returns an error
// when the StartMigration API call fails.
func TestMCPRound_TripAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_start_bulk_import",
		Arguments: map[string]any{
			"url":          "https://source.gitlab.com",
			"access_token": "glpat-test",
			"entities": []any{
				map[string]any{
					"source_type":           "group_entity",
					"source_full_path":      "source-group",
					"destination_slug":      "dest-group",
					"destination_namespace": "dest-ns",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result from API failure")
	}
}
