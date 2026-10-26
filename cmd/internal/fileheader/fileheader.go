// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package fileheader

// CopyrightStatement is the copyright statement every source file carries,
// without the comment marker its language puts in front of it.
const CopyrightStatement = "Copyright (c) 2026 José M. Requena Plens"

// LicenseStatement is the license statement every source file carries, in
// the form SPDX defines for naming a file's license in a comment near its
// top, without the comment marker.
const LicenseStatement = "SPDX-License-Identifier: MIT"

// Go is how a Go source file opens: the two statements as line comments, then
// the blank line that keeps them apart from what follows. Without it a header
// directly above the package clause would be read as the package's doc
// comment, and one directly above a "Code generated" line would join that
// comment group.
const Go = "// " + CopyrightStatement + "\n// " + LicenseStatement + "\n\n"
