//go:build neverset

package hidden

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPlanted_HiddenByAConstraint_Unjudged would be a finding if any load read
// it.
func TestPlanted_HiddenByAConstraint_Unjudged(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_ = srv.URL
}
