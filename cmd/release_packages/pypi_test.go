// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// pypi_test.go holds what the pypi subcommand writes into the wheel's
// modules to the project's header, which no gate reads there.
package main

import (
	"bytes"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// TestPypiMetadata_AReadmeWithCRLF_IsDescribedWithLF verifies that the
// description embeds the README with its line endings read as Python's text
// mode read them, every CRLF and lone CR a LF, so a checkout that wrote the
// README with CRLF publishes the same METADATA and RECORD.
func TestPypiMetadata_AReadmeWithCRLF_IsDescribedWithLF(t *testing.T) {
	lf := pypiMetadata("9.8.7", []byte("# Title\n\nA line.\nAnother.\n"))
	if got := pypiMetadata("9.8.7", []byte("# Title\r\n\r\nA line.\rAnother.\r\n")); !bytes.Equal(got, lf) {
		t.Errorf("METADATA over a CRLF README differs from the LF one:\n%q\n%q", got, lf)
	}
}

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
