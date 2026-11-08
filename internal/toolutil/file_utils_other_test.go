// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build !unix

// file_utils_other_test.go covers the half of file_utils_other.go that has an
// answer to check, on the platforms that build it.
package toolutil

import "testing"

// TestRefuseWritableArchive_AnyMode_IsAccepted verifies an import archive is
// never refused for its permission bits where Go synthesizes them from the
// read-only attribute: 0666, which a Unix half refuses, is what every file
// Windows lets anybody write reports, and what one only its owner may write
// reports too.
func TestRefuseWritableArchive_AnyMode_IsAccepted(t *testing.T) {
	t.Parallel()
	const canonical = `C:\imports\project-export.tar.gz`
	if got, err := refuseWritableArchive(canonical, modeOnlyInfo{mode: 0o666}); got != canonical || err != nil {
		t.Errorf("refuseWritableArchive(0666) = %q, %v; want %q accepted", got, err, canonical)
	}
}
