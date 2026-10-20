//go:build e2e

package tagged

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPlanted_BehindThe_e2e_Tag_Reported is read only by a load that sets e2e.
func TestPlanted_BehindThe_e2e_Tag_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_ = url
}
