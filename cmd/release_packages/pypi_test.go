// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// pypi_test.go holds what the pypi subcommand writes into the wheel's
// modules to the project's header, which no gate reads there.
package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// TestPypiFileHeader_IsTheProjectHeaderInPythonComments verifies that the
// header the wheel's two modules open with is the one cmd/internal/fileheader
// states, in # comments and followed by a blank line, so changing the holder
// or the year there changes what the next wheel publishes.
func TestPypiFileHeader_IsTheProjectHeaderInPythonComments(t *testing.T) {
	want := "# " + fileheader.CopyrightStatement + "\n# " + fileheader.LicenseStatement + "\n\n"
	if pypiFileHeader != want {
		t.Errorf("pypiFileHeader = %q, want %q", pypiFileHeader, want)
	}
}
