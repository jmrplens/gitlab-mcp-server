package tags

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestRegisterTools_ConfirmDeclined covers the ConfirmAction early-return
// branches in tag delete and unprotect handlers when the user declines.
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
		{"gitlab_tag_delete", map[string]any{"project_id": "42", "tag_name": "v1"}},
		{"gitlab_tag_unprotect", map[string]any{"project_id": "42", "tag_name": "v*"}},
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

// TestRegisterTools_GetNotFound covers the NotFoundResult branch in the
// gitlab_tag_get handler when the API returns 404.
func TestRegisterTools_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_tag_get",
		Arguments: map[string]any{"project_id": "42", "tag_name": "nonexistent"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestTagCreate_AlreadyExists covers the "already exists" hint branch in Create.
func TestTagCreate_AlreadyExists(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"Tag already exists"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", TagName: "v1", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for already-existing tag")
	}
}

// TestTagCreate_InvalidRef_Register covers the "is invalid" hint branch in Create
// via the register handler path.
func TestTagCreate_InvalidRef_Register(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Target is invalid"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", TagName: "v1", Ref: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
}

// TestTagList_PagePerPage covers the page/per_page optional branches in List.
func TestTagList_PagePerPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := List(context.Background(), client, ListInput{ProjectID: "1", PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestProtectTag_Conflict covers the 409 Conflict hint branch in ProtectTag.
func TestProtectTag_Conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"already exists"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{ProjectID: "1", TagName: "v*"})
	if err == nil {
		t.Fatal("expected error for conflict")
	}
}

// TestRegisterTools_ErrorPaths covers the error branches in RegisterTools
// closures for non-destructive tools against a 500 server.
func TestRegisterTools_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_tag_list", map[string]any{"project_id": "1"}},
		{"gitlab_tag_get", map[string]any{"project_id": "1", "tag_name": "v1"}},
		{"gitlab_tag_create", map[string]any{"project_id": "1", "tag_name": "v1", "ref": "main"}},
		{"gitlab_tag_get_signature", map[string]any{"project_id": "1", "tag_name": "v1"}},
		{"gitlab_tag_list_protected", map[string]any{"project_id": "1"}},
		{"gitlab_tag_get_protected", map[string]any{"project_id": "1", "tag_name": "v*"}},
		{"gitlab_tag_protect", map[string]any{"project_id": "1", "tag_name": "v*"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.name, err)
			}
			if !result.IsError {
				t.Errorf("CallTool(%s) expected IsError=true for 500 response", tt.name)
			}
		})
	}
}
