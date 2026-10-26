// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package fileheader holds the two lines every source file of this repository
// opens with: the copyright statement, naming the maintainer LICENSE names as
// the holder, and the SPDX identifier of the license LICENSE grants.
//
// They live here, rather than in the gate that holds the tree to them, because
// a generated file carries them too and its generator is what writes them:
// cmd/gen_action_catalog_manifest, cmd/gen_action_grants and cmd/gen_brand
// each open the Go file they emit with [Go], and cmd/audit_file_headers judges
// every file, generated or written by hand, against the same constants. A
// header spelled once for the generators and once for the gate would drift,
// and the gate would then fail on a file the generator had just rewritten.
//
// The year is the one LICENSE carries, and it is the same in every file: the
// lines are a notice for the project, not a record of when each file was
// written. Changing the holder or the year is a change to these constants, to
// LICENSE, and to every file the fix mode of cmd/audit_file_headers rewrites
// from them.
package fileheader
