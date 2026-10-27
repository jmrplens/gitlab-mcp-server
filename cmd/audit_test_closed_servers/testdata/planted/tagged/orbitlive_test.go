// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build orbitlive

package tagged

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPlanted_BehindThe_orbitlive_Tag_Reported is read only by a load that sets orbitlive.
func TestPlanted_BehindThe_orbitlive_Tag_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_ = url
}
