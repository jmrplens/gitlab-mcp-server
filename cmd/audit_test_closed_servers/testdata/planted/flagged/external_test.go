// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flagged_test

import (
	"testing"

	"example.com/planted/flagged"
)

// use stands for anything that sends a request to an address.
func use(string) {}

// TestPlanted_ExternalTestPackage_Reported sits in the external test package,
// which is a variant of its own, and gets its server from the package under
// test.
func TestPlanted_ExternalTestPackage_Reported(t *testing.T) {
	srv := flagged.NewServer()
	url := srv.URL
	srv.Close()
	use(url)
}
