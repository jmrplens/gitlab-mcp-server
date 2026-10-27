// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package declared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPlanted_ExcusedByADeclaration_Excused reads the address after the close
// only as text, which is the one thing a declaration may excuse.
func TestPlanted_ExcusedByADeclaration_Excused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if !strings.HasPrefix(url, "http://") {
		t.Errorf("url = %q", url)
	}
}
