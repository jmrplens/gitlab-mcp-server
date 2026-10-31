// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package releaseassets reads what a release publishes: the checksum file
// GoReleaser writes beside the assets, and the binaries it lists, identified
// by their format, the operating system they run on and the architecture of
// each image they hold.
//
// # Why one package
//
// Every Go command of this repository that reads checksums.txt or identifies
// a release binary does it here, so that the rules are written once: what a
// line of the checksum file has to be, and what makes a file a linux, darwin
// or windows binary of amd64 or arm64. cmd/reproduce_release reads the
// checksum file and the universal darwin binary through it, and the release
// checks that replace the scripts of issue 1114 are built on it. The two
// installers keep their own parse, since they run where no binary of ours
// exists yet, and so does scripts/fetch-release-assets.sh, the step that
// verifies what its job then runs.
//
// The package imports the standard library and nothing else, because the
// commands built on it run in the credentialed release job, where rule 2 of
// cmd/audit_supply_chain allows a Go command only the standard library and
// this module.
//
// # Checksums
//
// [ReadChecksums] and [ParseChecksums] read the file as GoReleaser writes it,
// a lowercase SHA-256, two spaces and the asset's name on each line, and
// refuse a line that is anything else and a name listed twice, naming the
// line: either leaves nothing certain to compare a file against. [Digest] is
// the SHA-256 in that spelling.
//
// # Binaries
//
// [Identify] reads a whole file with debug/elf, debug/macho or debug/pe,
// chosen by its first bytes, and names its target: a 64-bit little-endian
// ELF file for linux, a 64-bit little-endian Mach-O file or a universal one
// of such slices for darwin, a PE file for windows, each for amd64 or arm64,
// which are the targets the release builds. Anything else is refused, with
// the machine named when the format was readable.
//
// [ReadELF] is the part of that a Linux binary is held to beyond its
// machine: the loader its PT_INTERP program header names, which the kernel
// starts it through. The release builds with -buildmode=pie, so its Linux
// binaries are dynamically linked and a loader the target system lacks
// kills them at exec.
//
// [ThinArch] and [UniversalOrder] read a Mach-O file's header words rather
// than the whole file: the cpu type and subtype of a thin file, and the
// slices a universal file lists, in its order. They are the words GoReleaser
// reads when it joins the darwin binaries and writes into the universal one,
// which is all a rebuild of that file has to match, so they read no further
// than those words and accept what GoReleaser would.
package releaseassets
