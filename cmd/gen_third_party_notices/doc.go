// Command gen_third_party_notices writes THIRD_PARTY_NOTICES: the license,
// notice and patent texts of every module built into the release binaries,
// read from the binaries' own build information and from the module cache.
//
// # Why it exists
//
// gitlab-mcp-server is MIT and its LICENSE travels with every package. The
// binaries also carry the Go standard library and some fifty modules under
// BSD-3-Clause, Apache-2.0, MIT and MPL-2.0, whose terms ask for their
// license and notice texts to accompany a binary redistribution, and until
// 3.1.0 no channel shipped them: the SPDX SBOM names each license and
// carries none of the texts. This file is those texts, generated at release
// time from what the binaries actually link rather than from go.mod, which
// also lists modules only tests, tools or other platforms use.
//
// # What it reads
//
// Each argument is a binary or a glob of them. The build information each
// binary records, the same list `go version -m` prints, names the modules
// linked into it and the GOOS and GOARCH it was built for. The modules of
// every binary are merged, and a module one target links and another does
// not says so: at 3.1.0 the darwin builds link github.com/ebitengine/purego
// and the windows builds github.com/go-ole/go-ole, which no linux build
// carries. For each module the texts are read from its directory in the
// module cache, which GoReleaser's `go mod download` hook fills in the
// release job and the Dockerfile's builder fills in the image build: every
// regular file at the module's root named LICENSE (in either spelling),
// COPYING, COPYRIGHT, NOTICE or PATENTS, alone or with a prefix or suffix
// (LICENSE.md, LICENSE-APACHE, MIT-LICENSE). The standard library's texts
// are read from GOROOT's root the same way, after its VERSION is held to the
// toolchain the binaries record, so the text is the one of the Go that built
// them.
//
// A module can also carry a license of its own below its root, which is
// what code copied into it from another project under another license looks
// like. Build information records modules, not packages, so for each binary
// the packages it links are listed again with `go list -deps` on its main
// package, under the GOOS, GOARCH, CGO_ENABLED, other GO settings, build
// tags and instrumentation flags (-race, -msan, -asan) it records, and the
// license files in the directory of every linked package, and of each parent
// of one below the module root, are reproduced too, each named with the
// import path of its directory ("LICENSE in example.com/m/pkg/vendored"). A
// directory holding only packages no binary links is not read, so texts
// covering code no binary carries (test fuzzers, internal tools) stay out.
// The listing is held to each binary's own build information in both
// directions, the replacement each module is built from included, which is
// what ties it to the go.mod the binaries were built from: the command runs
// inside the module checkout, as every caller already does. That is a check
// of the module set and not of the source, so a checkout at another commit
// with the same requirements but other imports would pass it. None of the
// modules 3.1.0 links has such a file, so the section exists for the
// dependency that one day will.
//
// No directory below GOROOT's root is read. The standard library carries two
// kinds of license file there, and neither is a text these binaries are
// short of: the copies of golang.org/x modules it vendors under src/vendor
// carry the Go Authors' license and patent grant, the same texts as GOROOT's
// root, and crypto/internal/boring's covers the BoringSSL object only a
// GOEXPERIMENT=boringcrypto build links, which needs cgo, while every build
// this runs against sets CGO_ENABLED=0.
//
// # What it refuses
//
// Generation stops, and with it the release, rather than write a file that
// is short of something: a pattern that matches no file, a binary with no
// build information, binaries built from different main modules, by
// different toolchains, twice for one target, or linking one module at two
// versions, a target list other than the one -targets names, a package
// listing that fails or disagrees with a binary's build information (a
// package from a module it does not name or names at another version or
// replacement, or a module it names that the listing gives no package of), a
// module the cache
// does not hold or replaced by a local directory, and a module or a GOROOT
// that publishes no license file at all. A package directory below a root
// may hold none, which is the usual case.
//
// A main module recorded as (devel), which is how a build from a tree with
// no version control metadata names it, the image's builder among them, is
// written without a version.
//
// It reproduces texts and classifies none: which SPDX identifier a text is
// lives in the release's SBOMs. GOROOT and the module cache are read from
// `go env` unless -goroot and -modcache name them.
//
// # Where it runs
//
// GoReleaser runs it once all six release binaries are built and before
// checksums.txt is computed and signed, through an `sboms` entry with
// `artifacts: any` (.goreleaser.yml), so THIRD_PARTY_NOTICES is a release
// asset listed in the signed checksums.txt like every binary. The npm, PyPI
// and NuGet packages and the Claude Desktop bundles carry that file; the
// image's builder stage runs this command against the image's own binary,
// which is built separately. `make mcpb` runs it against the binaries it
// builds.
//
// # Exit codes
//
// 0 when the file was written; 1 when it could not be generated; 2 for
// arguments that do not parse.
package main
