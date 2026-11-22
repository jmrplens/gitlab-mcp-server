// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// releasePlatform is one of the six release binaries the npm, PyPI and NuGet
// packages carry, under each vocabulary that names it: Go's GOOS and GOARCH
// on the release asset, Node's platform and architecture on the npm side,
// the wheel's platform tag on the PyPI side and the runtime identifier on the
// NuGet side. This table is the one place they meet, so nothing downstream
// translates one into another.
type releasePlatform struct {
	// key is the asset name's platform part, linux-amd64, which the PyPI and
	// NuGet verified-binaries records name a binary by.
	key string
	// asset is the release asset's file name.
	asset string
	// exe is whether the binary is a Windows executable, which every package
	// names gitlab-mcp-server.exe.
	exe bool
	// npmKey is the npm package suffix and the platform the launcher looks
	// up at run time, and npmOS, npmCPU and npmLibc confine the install.
	npmKey, npmOS, npmCPU, npmLibc string
	// wheelTag is the wheel's platform tag.
	wheelTag string
	// rid is the .NET runtime identifier.
	rid string
}

// releasePlatforms is the whole distribution matrix, in the order every
// package format writes and lists it.
//
// The Linux binaries are PIE executables linked against the glibc dynamic
// loader (CGO_ENABLED=0 with -buildmode=pie), so the npm packages declare
// glibc, the wheels carry manylinux tags, and musl (Alpine) is left to the
// container image.
var releasePlatforms = []releasePlatform{
	{
		key: "linux-amd64", asset: "gitlab-mcp-server-linux-amd64", npmKey: "linux-x64", npmOS: "linux", npmCPU: "x64", npmLibc: "glibc",
		wheelTag: "manylinux_2_17_x86_64.manylinux2014_x86_64", rid: "linux-x64",
	},
	{
		key: "linux-arm64", asset: "gitlab-mcp-server-linux-arm64", npmKey: "linux-arm64", npmOS: "linux", npmCPU: "arm64", npmLibc: "glibc",
		wheelTag: "manylinux_2_17_aarch64.manylinux2014_aarch64", rid: "linux-arm64",
	},
	{
		key: "darwin-amd64", asset: "gitlab-mcp-server-darwin-amd64", npmKey: "darwin-x64", npmOS: "darwin", npmCPU: "x64",
		wheelTag: "macosx_11_0_x86_64", rid: "osx-x64",
	},
	{
		key: "darwin-arm64", asset: "gitlab-mcp-server-darwin-arm64", npmKey: "darwin-arm64", npmOS: "darwin", npmCPU: "arm64",
		wheelTag: "macosx_11_0_arm64", rid: "osx-arm64",
	},
	{
		key: "windows-amd64", asset: "gitlab-mcp-server-windows-amd64.exe", exe: true, npmKey: "win32-x64", npmOS: "win32", npmCPU: "x64",
		wheelTag: "win_amd64", rid: "win-x64",
	},
	{
		key: "windows-arm64", asset: "gitlab-mcp-server-windows-arm64.exe", exe: true, npmKey: "win32-arm64", npmOS: "win32", npmCPU: "arm64",
		wheelTag: "win_arm64", rid: "win-arm64",
	},
}

// goos and goarch are the target the platform's binary is built for.
func (p releasePlatform) goos() string   { goos, _, _ := strings.Cut(p.key, "-"); return goos }
func (p releasePlatform) goarch() string { _, goarch, _ := strings.Cut(p.key, "-"); return goarch }

// binary is the name the binary carries inside every package.
func (p releasePlatform) binary() string {
	if p.exe {
		return "gitlab-mcp-server.exe"
	}
	return binaryName
}

// The names the packages share: the MCP Registry server name and the
// ownership token each registry's README or metadata carries, the
// repository's license, and the file the builders record what they verified
// in, beside the packages and never inside one.
const (
	serverName       = "io.github.jmrplens/gitlab-mcp-server"
	mcpNameToken     = "mcp-name: " + serverName
	verifiedManifest = "verified-binaries.json"
	checksumsName    = "checksums.txt"
)

// The versions each builder takes: npm a plain release version, as its
// launcher pins it, and PyPI and NuGet a prerelease suffix besides.
var (
	plainVersion   = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	releaseVersion = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
)

// packageBuild is what the three builders share: the repository root the
// repository files are read from, the directory of release assets, and
// whether a dist without checksums.txt may be packaged.
type packageBuild struct {
	root, binaries  string
	allowUnverified bool
	stdout, stderr  io.Writer
	// wheels is set for the PyPI build, whose messages name a wheel where
	// the others name a package.
	wheels bool
}

// unit is what the build's messages call one of its packages.
func (b *packageBuild) unit() string {
	if b.wheels {
		return "wheel"
	}
	return "package"
}

// releaseInputs is everything a builder packs from the release assets, read
// and verified before any package is written: the notices and each binary,
// with their digests.
type releaseInputs struct {
	verified bool
	notices  []byte
	// noticesDigest and binaryDigests are what the verified-binaries record
	// holds; binaryDigests is keyed by platform key.
	noticesDigest string
	binaries      map[string][]byte
	binaryDigests map[string]string
}

// errRefused is returned by a builder that has already said why it refuses
// the run.
var errRefused = errors.New("refused")

// refuse writes why a run is refused and returns errRefused.
func (b *packageBuild) refuse(format string, args ...any) error {
	fmt.Fprintf(b.stderr, "%s: "+format+"\n", append([]any{toolName}, args...)...)
	return errRefused
}

// assetPath is where a release asset is read from, and assetShown how a
// message names it: the binaries directory as given, joined with a slash.
func (b *packageBuild) assetPath(name string) string  { return localPath(b.root, b.binaries+"/"+name) }
func (b *packageBuild) assetShown(name string) string { return path.Join(b.binaries, name) }

// readRelease reads the notices and the six binaries and holds each to the
// digest the release's checksums.txt names, the manifest cosign signed at
// build time. Without one, the run is refused unless --allow-unverified was
// given, and then warned about, warning saying what is being packaged. Every asset is checked before anything is written, so a refused
// run leaves no package behind, where the scripts had written each package
// before they looked at the next binary.
func (b *packageBuild) readRelease(warning string) (*releaseInputs, error) {
	in := &releaseInputs{binaries: map[string][]byte{}, binaryDigests: map[string]string{}}
	var sums releaseassets.Checksums
	checksums := b.assetPath(checksumsName)
	if _, err := os.Stat(checksums); err == nil {
		if sums, err = releaseassets.ReadChecksums(checksums); err != nil {
			return nil, b.refuse("%v", err)
		}
		in.verified = true
	} else if !b.allowUnverified {
		return nil, b.refuse("%s not found: refusing to package unverified binaries (pass --allow-unverified to override)",
			b.assetShown(checksumsName))
	} else {
		fmt.Fprintf(b.stderr, "WARNING: --allow-unverified: %s without a checksum manifest\n", warning)
	}
	read := func(name string) ([]byte, string, error) {
		data, err := os.ReadFile(b.assetPath(name)) //#nosec G304 -- a release asset in the directory the operator named
		if err != nil {
			return nil, "", err
		}
		digest := releaseassets.Digest(data)
		if !in.verified {
			return data, digest, nil
		}
		want, listed := sums.Digests[name]
		if !listed {
			return nil, "", b.refuse("%s is not listed in %s", name, b.assetShown(checksumsName))
		}
		if want != digest {
			return nil, "", b.refuse("%s is sha256 %s, but checksums.txt says %s", name, digest, want)
		}
		return data, digest, nil
	}
	notices, digest, err := read(noticesName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, b.refuse("%s not found: the release generates it beside the binaries (cmd/gen_third_party_notices), and every %s carries it",
			b.assetShown(noticesName), b.unit())
	}
	if err != nil {
		return nil, b.readFailure(err)
	}
	in.notices, in.noticesDigest = notices, digest
	for _, p := range releasePlatforms {
		binary, binaryDigest, readErr := read(p.asset)
		if errors.Is(readErr, fs.ErrNotExist) {
			return nil, b.refuse("missing release binary %s", b.assetShown(p.asset))
		}
		if readErr != nil {
			return nil, b.readFailure(readErr)
		}
		in.binaries[p.key], in.binaryDigests[p.key] = binary, binaryDigest
	}
	return in, nil
}

// readFailure reports an asset that could not be read for a reason other
// than its absence, unless the refusal was already written.
func (b *packageBuild) readFailure(err error) error {
	if errors.Is(err, errRefused) {
		return err
	}
	return b.refuse("%v", err)
}

// readLicense reads the repository's LICENSE, which every package carries
// beside the notices: the packages are copies of this project, and MIT's one
// condition is that its notice travels with each copy.
func (b *packageBuild) readLicense() ([]byte, error) {
	data, err := os.ReadFile(localPath(b.root, licensePath)) //#nosec G304 -- the repository's own LICENSE
	if err != nil {
		return nil, b.refuse("license file LICENSE not found at %s", licensePath)
	}
	return data, nil
}

// readTokenReadme reads a README the MCP Registry validates the package's
// ownership through, refusing one that lost the token.
func (b *packageBuild) readTokenReadme(rel string) ([]byte, error) {
	data, err := os.ReadFile(localPath(b.root, rel)) //#nosec G304 -- a README of the repository
	if err != nil {
		return nil, b.refuse("%s could not be read: %v", rel, err)
	}
	if !bytes.Contains(data, []byte(mcpNameToken)) {
		return nil, b.refuse("%s lost the mcp-name ownership token the MCP Registry validates", rel)
	}
	return data, nil
}

// verifiedRecord is the record a builder writes beside the packages of what
// it verified, which the validator holds the packed bytes to: the version,
// whether there was a manifest, each binary's digest by key and the
// notices'.
func verifiedRecord(version string, verified bool, binaries []jsonMember, notices string) []byte {
	return (&jsonValue{kind: jsonObject, members: []jsonMember{
		{key: "version", value: jsonText(version)},
		{key: "verified", value: &jsonValue{kind: jsonBool, boolean: verified}},
		{key: "binaries", value: &jsonValue{kind: jsonObject, members: binaries}},
		{key: "notices", value: jsonText(notices)},
	}}).pretty()
}

// freshDir removes dir and creates it empty, so a rebuild cannot mix
// versions.
func freshDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o750)
}

// writeFileMode writes data at path with mode set rather than left to the
// umask: npm records each file's mode in the tarball, and a binary packed
// without its execute bit installs unable to run.
func writeFileMode(file string, data []byte, mode fs.FileMode) error {
	if err := os.WriteFile(file, data, mode); err != nil { //#nosec G306,G703 -- a package file the build writes; the mode is the package's
		return err
	}
	return os.Chmod(file, mode)
}

// packageEntry is one entry of a wheel or a NuGet package.
type packageEntry struct {
	name       string
	data       []byte
	executable bool
}

// writePackageZip writes a wheel or a .nupkg: the entries in the order given,
// each a regular Unix file (0644, or 0755 for the binary), at the zip epoch,
// deflated, with no extra field, and with its CRC and sizes in its local
// header rather than in a data descriptor after its data. pip honors the
// execute bits only on an entry whose mode says it is a regular file, and
// nuget.org's repository signature can be removed again byte for byte only
// from an archive with no data descriptor, which is what the attestation of
// the NuGet packages is looked up by. The deflated bytes are
// compress/flate's rather than zlib's, so the archives differ from the ones
// Python's zipfile wrote, entry for entry equal.
func writePackageZip(file string, entries []packageEntry) error {
	out, err := os.Create(file) //#nosec G304,G703 -- a package under the output directory the operator named
	if err != nil {
		return err
	}
	return errors.Join(writeRawEntries(out, entries), out.Close())
}

// writeRawEntries writes the archive of entries to out.
func writeRawEntries(out io.Writer, entries []packageEntry) error {
	archive := zip.NewWriter(out)
	for _, entry := range entries {
		var compressed bytes.Buffer
		deflater, err := flate.NewWriter(&compressed, flate.BestCompression)
		if err != nil {
			return err
		}
		if _, err = deflater.Write(entry.data); err != nil {
			return err
		}
		if writeErr := deflater.Close(); writeErr != nil {
			return writeErr
		}
		header := &zip.FileHeader{
			Name:               entry.name,
			Method:             zip.Deflate,
			CRC32:              crc32.ChecksumIEEE(entry.data),
			CompressedSize64:   uint64(compressed.Len()), //#nosec G115 -- a length is never negative
			UncompressedSize64: uint64(len(entry.data)),
			ReaderVersion:      20,
			CreatorVersion:     20,
		}
		header.ModifiedDate = zipEpochDate //nolint:staticcheck // Modified would write an extra field (see addEntry).
		header.ModifiedTime = zipEpochTime //nolint:staticcheck // Modified would write an extra field (see addEntry).
		mode := fs.FileMode(0o644)
		if entry.executable {
			mode = 0o755
		}
		header.SetMode(mode)
		writer, err := archive.CreateRaw(header)
		if err != nil {
			return err
		}
		if _, err = writer.Write(compressed.Bytes()); err != nil {
			return err
		}
	}
	return archive.Close()
}

// builderFlags is the command line every builder takes: the release assets
// and the version, the output directory, and --allow-unverified.
type builderFlags struct {
	set             *flag.FlagSet
	binaries        string
	version         string
	out             string
	allowUnverified bool
}

// newBuilderFlags declares the shared flags, out defaulting to defaultOut.
func newBuilderFlags(name, defaultOut string, stderr io.Writer) *builderFlags {
	f := &builderFlags{set: flag.NewFlagSet(toolName+" "+name, flag.ContinueOnError)}
	f.set.SetOutput(stderr)
	f.set.StringVar(&f.binaries, "binaries", "", "directory holding the release assets under their published names, and checksums.txt")
	f.set.StringVar(&f.version, "version", "", "release version, e.g. 3.7.0")
	f.set.StringVar(&f.out, "out", defaultOut, "output directory, emptied first")
	f.set.BoolVar(&f.allowUnverified, "allow-unverified", false, "build without a checksums.txt manifest (never in CI)")
	return f
}

// parse reads args and refuses a run without the release assets or the
// version, writing why; it reports whether the run can go on.
func (f *builderFlags) parse(args []string, stderr io.Writer, needBinaries bool) bool {
	if err := f.set.Parse(args); err != nil {
		return false
	}
	if f.set.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: unexpected argument %q\n", f.set.Name(), f.set.Arg(0))
		return false
	}
	if f.version == "" {
		fmt.Fprintf(stderr, "%s: --version <x.y.z> is required\n", f.set.Name())
		return false
	}
	if needBinaries && f.binaries == "" {
		fmt.Fprintf(stderr, "%s: --binaries <dir> is required\n", f.set.Name())
		return false
	}
	return true
}
