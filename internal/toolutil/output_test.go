// output_test.go contains unit tests for MCP tool output helpers.
package toolutil

import (
	"testing"
)

// TestSuccessResult verifies that [SuccessResult] returns a non-nil result
// with IsError=false for non-empty markdown, and nil for empty input.
func TestSuccessResult(t *testing.T) {
	t.Run("non-empty markdown", func(t *testing.T) {
		result := SuccessResult("## Title\nSome content")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Content) != 1 {
			t.Fatalf("expected 1 content item, got %d", len(result.Content))
		}
		if result.IsError {
			t.Error("expected IsError=false")
		}
	})

	t.Run("empty markdown returns nil", func(t *testing.T) {
		result := SuccessResult("")
		if result != nil {
			t.Error("expected nil for empty markdown")
		}
	})
}

// TestErrorResult verifies that [ErrorResult] returns a non-nil result
// with IsError=true and a single content entry.
func TestErrorResult(t *testing.T) {
	result := ErrorResult("something went wrong")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
}
