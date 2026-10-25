// Command reproduce_release rebuilds the binaries a release publishes and
// compares them with the published ones byte for byte, or builds every
// release target of a tree twice and compares the two builds.
//
// # Why
//
// A release's binaries are built by GoReleaser in the release workflow, and
// everything a user installs is one of them or is packed from one. The
// signature over checksums.txt says the workflow published those bytes; it
// does not say the bytes are what the tagged source compiles to. A rebuild
// from the tag that comes out identical does, and anybody can make it: the
// toolchain, the flags and the commit are all recorded or published, and
// nothing in the build depends on the machine that ran it. That is what the
// OpenSSF Best Practices criteria build_repeatable and build_reproducible ask
// a project to show.
//
// # Against a release (-published)
//
// The release's commit, the one -ref names, is cloned into a temporary
// directory, so nothing in the tree differs from that commit and its tags
// are there: the go command stamps a tree with changes as modified, and a
// tagged commit's version from the tag. The GoReleaser configuration at that
// commit says how each binary was built, and is read the way
// cmd/audit_binary_vulns reads it, refusing every setting that bears on the
// binaries and that this command does not apply, rather than rebuilding
// something GoReleaser never built. It reads each build's main package, env,
// flags and ldflags, its overrides, its targets (linux, darwin and windows on
// amd64 and arm64, whose file names and levels GoReleaser's defaults fix),
// the universal darwin binaries, the one archive, which has to publish each
// binary as it is, and the checksum file's name. It reads past the top-level
// sections that only write files beside the binaries or describe the release
// (version, snapshot, changelog, signs, sboms and release), and refuses any
// other, since one may rewrite a binary after it is built (upx, notarize).
// The templates are evaluated with the fields a release renders them with:
// the tag, the version (the tag without its v), the commit, and the
// abbreviated commit, which is git's abbreviation of it in the clone unless
// -short-commit gives it. A template naming anything else stops the run.
//
// Each binary is then held to what the release published before it is
// rebuilt. The checksum file in -published has to list it, the file there
// has to be the one the list names, and its build information has to record
// the commit being rebuilt, a tree nobody had changed and a release
// toolchain. The binary is rebuilt with that toolchain (GOTOOLCHAIN), with
// the configuration's flags and environment in GoReleaser's order and the
// caller's GOFLAGS cleared, into the directory GoReleaser would write it to,
// and compared. A universal binary is joined from the rebuilt darwin
// binaries in the order the published one's header lists them, which is the
// one thing about it its inputs do not fix, and laid out as GoReleaser lays
// it out. What the checksum file lists beyond the binaries (the SBOMs, the
// notices) is named as not rebuilt. With -trimpath the go command keeps the
// linker flags out of the build information, so a binary that differs while
// its build information agrees points at them.
//
// A release stamped with the abbreviated commit, as every release up to
// 3.3.0 was, reproduces only with the abbreviation it was stamped with, since
// git gives a commit as many characters as the clone's object count needs:
// -short-commit takes it, and `gitlab-mcp-server --version` prints it. The
// configuration stamps the full commit from the release after 3.3.0 on. The
// configuration of a release before 3.2.0 names no ELF loader for its PIE
// Linux binaries, which leaves the loader to the host that links them, so
// those binaries rebuild the same only on a host that offers the loader the
// release's host did.
//
// # Twice (-twice)
//
// Every release target of the tree at -dir is built twice, as it stands,
// into two directories, the second time with an empty build cache so every
// package is compiled again rather than reused, and the two builds are
// compared. It needs no release and no git, and no network once the module
// cache holds the modules, so it runs on a copy of the tree synced without
// .git, where the commit renders as "none", GoReleaser's own value outside a
// repository. The version is -tag, or v followed by the tree's VERSION. A
// universal binary is not built: the order of its slices is a release's own,
// so it is compared against a release only.
//
// # Usage
//
//	go run ./cmd/reproduce_release/ -published dist/reproduce/vX.Y.Z -tag vX.Y.Z -ref vX.Y.Z
//	go run ./cmd/reproduce_release/ -published dist/reproduce/v3.3.0 -tag v3.3.0 -ref v3.3.0 -short-commit f8813193
//	go run ./cmd/reproduce_release/ -twice
//
// make check-reproducible TAG=vX.Y.Z fetches and verifies a release's assets
// with scripts/fetch-release-assets.sh first; make check-repeatable is the
// second form.
//
// # Exit codes
//
// 0 when every binary's two copies are the same bytes; 1 when one differs
// (said on stdout, with what its build information disagrees on and where
// the bytes first part) or when the comparison could not be made (said on
// stderr); 2 for arguments it cannot use.
package main
