// schema_middleware_test.go contains unit tests for the receiving middleware
// that visits the tools of the first successful tools/list response, which the
// schema lockdown and the pagination schema rewrite both register.
package toolutil

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listedTools is the answer a handler under the middleware gives to a
// tools/list it served.
func listedTools() *mcp.ListToolsResult {
	return &mcp.ListToolsResult{
		Tools: []*mcp.Tool{{Name: "gitlab_issue_list"}, {Name: "gitlab_project_get"}},
	}
}

// answering returns a handler that gives the prepared answers in order, so a
// test can say what the handler under the middleware returned on each call.
func answering(answers ...mcp.Result) mcp.MethodHandler {
	return func(context.Context, string, mcp.Request) (mcp.Result, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	}
}

// TestFirstToolsListMiddleware_SuccessfulListing_IsVisitedOnce verifies that
// the tools of a served listing are visited, that the listing is returned
// untouched, and that a second listing is not visited again. The guard matters
// because the visit mutates the shared input-schema maps the tools carry, so a
// second concurrent listing must find them stable rather than rewrite them.
func TestFirstToolsListMiddleware_SuccessfulListing_IsVisitedOnce(t *testing.T) {
	listed := listedTools()
	var visits [][]*mcp.Tool
	handler := firstToolsListMiddleware(func(tools []*mcp.Tool) {
		visits = append(visits, tools)
	})(answering(listed, listed))

	for range 2 {
		result, err := handler(context.Background(), "tools/list", nil)
		if err != nil {
			t.Fatalf("handler() error = %v, want nil", err)
		}
		if result != mcp.Result(listed) {
			t.Errorf("handler() = %v, want the listing the handler under it returned", result)
		}
	}

	if len(visits) != 1 {
		t.Fatalf("visited %d times, want exactly one visit over the first listing", len(visits))
	}
	if len(visits[0]) != len(listed.Tools) {
		t.Errorf("visited %d tools, want %d", len(visits[0]), len(listed.Tools))
	}
}

// TestFirstToolsListMiddleware_FailedListing_KeepsItsError verifies that a
// tools/list the handler under the middleware could not serve comes back as the
// failure it was. Swallowing it would answer the client with an empty listing
// and no error, which reads as a server that registered no tools.
func TestFirstToolsListMiddleware_FailedListing_KeepsItsError(t *testing.T) {
	failure := errors.New("the catalog could not be built")
	visited := false
	handler := firstToolsListMiddleware(func([]*mcp.Tool) { visited = true })(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, failure
		},
	)

	result, err := handler(context.Background(), "tools/list", nil)
	if !errors.Is(err, failure) {
		t.Errorf("handler() error = %v, want %v", err, failure)
	}
	if result != nil {
		t.Errorf("handler() = %v, want nil beside the error", result)
	}
	if visited {
		t.Error("visited the tools of a listing that failed")
	}
}

// TestFirstToolsListMiddleware_OtherMethod_PassesThrough verifies that every
// method other than tools/list is returned as it arrived and visits nothing.
func TestFirstToolsListMiddleware_OtherMethod_PassesThrough(t *testing.T) {
	called := &mcp.CallToolResult{}
	visited := false
	handler := firstToolsListMiddleware(func([]*mcp.Tool) { visited = true })(answering(called))

	result, err := handler(context.Background(), "tools/call", nil)
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if result != mcp.Result(called) {
		t.Errorf("handler() = %v, want the result the handler under it returned", result)
	}
	if visited {
		t.Error("visited the tools of a call that was not a listing")
	}
}

// TestFirstToolsListMiddleware_NilListing_DoesNotSpendTheVisit verifies that a
// listing that is a nil pointer is left alone rather than visited.
//
// It satisfies the type assertion, so reading the tools off it dereferences
// nothing; and visiting it at all would spend the one visit the sync.Once
// allows on a response with no tools in it, leaving the real listing unvisited
// and its schemas silently unrewritten.
func TestFirstToolsListMiddleware_NilListing_DoesNotSpendTheVisit(t *testing.T) {
	listed := listedTools()
	var missing *mcp.ListToolsResult
	var visits [][]*mcp.Tool
	handler := firstToolsListMiddleware(func(tools []*mcp.Tool) {
		visits = append(visits, tools)
	})(answering(missing, listed))

	for range 2 {
		if _, err := handler(context.Background(), "tools/list", nil); err != nil {
			t.Fatalf("handler() error = %v, want nil", err)
		}
	}

	if len(visits) != 1 {
		t.Fatalf("visited %d times, want the one listing that had tools", len(visits))
	}
	if len(visits[0]) != len(listed.Tools) {
		t.Errorf("visited %d tools, want the %d of the listing that was not nil", len(visits[0]), len(listed.Tools))
	}
}

// TestOnFirstToolsList_NilServer verifies that registering the visit on no
// server at all is a no-op rather than a panic, which is what lets the two
// callers hand over whatever server they were given without checking it first.
func TestOnFirstToolsList_NilServer(t *testing.T) {
	visited := false
	onFirstToolsList(nil, func([]*mcp.Tool) { visited = true })
	if visited {
		t.Error("visited the tools of a server that does not exist")
	}
}
