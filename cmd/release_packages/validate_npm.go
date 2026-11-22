// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

// The npm checks. A published npm version is permanent, so a broken tarball
// caught here is cheap and caught after the publish is not.
const (
	npmMinBinary      = 5_000_000
	npmHandshakeLimit = 60 * time.Second
)

// runValidateNpm is the validate-npm subcommand.
func runValidateNpm(root string, args []string, stdout, stderr io.Writer) int {
	flags := newValidatorFlags("validate-npm", stderr)
	packages := flags.set.String("packages", npmPackageDir, "the directory the npm subcommand wrote the platform packages to")
	main := flags.set.String("main", npmMainDir, "the launcher package")
	if !flags.parse(args, stderr) {
		return 2
	}
	v := newValidation("validate-npm", root, flags.version, npmMinBinary, stdout, stderr)
	work, err := os.MkdirTemp("", "validate-npm-")
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(work) }()
	n := &npmValidation{validation: v, packages: localPath(root, *packages), main: localPath(root, *main), work: work, pack: npmPack}
	n.structural()
	if flags.noInstall {
		fmt.Fprintln(stdout, "runtime: skipped (--no-install)")
	} else if len(v.problems) == 0 {
		n.install()
	}
	return v.finish("the npm distribution")
}

// npmValidation is one run of validate-npm.
type npmValidation struct {
	*validation
	packages, main, work string
	// pack packs a package directory into a tarball in dest and returns its
	// path, a seam so a test needs no npm.
	pack   func(v *validation, dir, dest string) (string, error)
	record *verified
}

// npmPack packs dir with npm pack, which is the point: the check reads the
// artifact that ships, not the directory it was packed from.
func npmPack(v *validation, dir, dest string) (string, error) {
	out, err := v.run(dir, nil, "npm", "pack", "--pack-destination", dest, "--silent")
	if err != nil {
		return "", fmt.Errorf("npm pack failed: %w\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return filepath.Join(dest, strings.TrimSpace(lines[len(lines)-1])), nil
}

// tarEntry is one file a tarball carries, with its mode and bytes.
type tarEntry struct {
	mode int64
	data []byte
}

// readTarball reads every regular file of a gzipped tarball by its name
// below package/.
func readTarball(file string) (map[string]tarEntry, error) {
	data, err := os.ReadFile(file) //#nosec G304 -- a tarball npm pack wrote into the validator's own directory
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	entries := map[string]tarEntry{}
	reader := tar.NewReader(gz)
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			return entries, nil
		}
		if nextErr != nil {
			return nil, nextErr
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		body, readErr := io.ReadAll(reader)
		if readErr != nil {
			return nil, readErr
		}
		entries[strings.TrimPrefix(header.Name, "package/")] = tarEntry{mode: header.Mode, data: body}
	}
}

// packed packs one package and reads its tarball, or reports why it cannot.
func (n *npmValidation) packed(label, dir string) (map[string]tarEntry, bool) {
	tgz, err := n.pack(n.validation, dir, n.work)
	if err != nil {
		n.failf("%s: %v", label, err)
		return nil, false
	}
	entries, err := readTarball(tgz)
	if err != nil {
		n.failf("%s: the tarball %s cannot be read: %v", label, tgz, err)
		return nil, false
	}
	return entries, true
}

// checkFileSet holds a tarball to exactly the files it ships, and its
// license texts to the repository's LICENSE and the release's notices.
func (n *npmValidation) checkFileSet(label string, entries map[string]tarEntry, want []string, license []byte) {
	shipped := mapKeys(entries)
	sort.Strings(want)
	n.check(slices.Equal(shipped, want), "%s: the tarball ships %q, want %q", label, shipped, want)
	if entry, ok := entries[licensePath]; ok {
		n.check(bytes.Equal(entry.data, license), "%s: LICENSE in the tarball is not the repository's LICENSE", label)
	}
	if entry, ok := entries[noticesName]; ok {
		digest := ""
		if n.record != nil {
			digest = n.record.Notices
		}
		n.checkNotices(label, entry.data, digest)
	}
}

// packageJSON is the part of a package.json the checks read.
type packageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	Libc                 []string          `json:"libc"`
	Bin                  map[string]string `json:"bin"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

// readPackageJSON reads dir/package.json, or reports why it cannot.
func (n *npmValidation) readPackageJSON(label, dir string) (packageJSON, bool) {
	var pkg packageJSON
	data, err := os.ReadFile(filepath.Join(dir, "package.json")) //#nosec G304 -- a package the npm subcommand wrote
	if err == nil {
		err = json.Unmarshal(data, &pkg)
	}
	if err != nil {
		n.failf("%s: package.json cannot be read: %v", label, err)
		return pkg, false
	}
	return pkg, true
}

// structural runs the checks that execute nothing, over all seven packages:
// each package.json's name, version and install gates, the launcher's bin
// and pins, and each packed tarball's exact file set, license texts, binary
// execute bit, size, format and machine, and digest.
func (n *npmValidation) structural() {
	n.record = n.readVerified(n.packages, false)
	license := n.repositoryLicense()
	for _, p := range releasePlatforms {
		n.validatePlatform(p, license)
	}
	label := "main"
	if pkg, ok := n.readPackageJSON(label, n.main); ok {
		n.check(pkg.Name == npmMainName, "main: name is %s", pkg.Name)
		n.check(pkg.Version == n.version, "main: version %s, want %s", pkg.Version, n.version)
		n.check(pkg.Bin["gitlab-mcp-server"] == "cli.js", "main: bin does not point at cli.js")
		for _, p := range releasePlatforms {
			dep := npmPlatformName(p)
			n.check(pkg.OptionalDependencies[dep] == n.version, "main: optionalDependency %s pinned to %q, want %s", dep, pkg.OptionalDependencies[dep], n.version)
		}
	}
	if entries, ok := n.packed(label, n.main); ok {
		n.checkFileSet(label, entries, []string{"cli.js", "package.json", "README.md", licensePath, noticesName}, license)
	}
	fmt.Fprintf(n.stdout, "structural: 7 packages checked (files, license, notices, exec bit, format, sha256 vs checksums.txt, os/cpu, pins)\n")
}

// validatePlatform checks one platform package.
func (n *npmValidation) validatePlatform(p releasePlatform, license []byte) {
	label := p.npmKey
	dir := filepath.Join(n.packages, p.npmKey)
	if pkg, ok := n.readPackageJSON(label, dir); ok {
		n.check(pkg.Name == npmPlatformName(p), "%s: name is %s, want %s", label, pkg.Name, npmPlatformName(p))
		n.check(pkg.Version == n.version, "%s: version %s, want %s", label, pkg.Version, n.version)
		n.check(slices.Equal(pkg.OS, []string{p.npmOS}), "%s: os %q, want [%s]", label, pkg.OS, p.npmOS)
		n.check(slices.Equal(pkg.CPU, []string{p.npmCPU}), "%s: cpu %q, want [%s]", label, pkg.CPU, p.npmCPU)
		var libc []string
		if p.npmLibc != "" {
			libc = []string{p.npmLibc}
		}
		n.check(slices.Equal(pkg.Libc, libc), "%s: libc %q, want %q", label, pkg.Libc, libc)
	}
	entries, ok := n.packed(label, dir)
	if !ok {
		return
	}
	n.checkFileSet(label, entries, []string{p.binary(), "package.json", "README.md", licensePath, noticesName}, license)
	binary, ok := entries[p.binary()]
	if !n.check(ok, "%s: binary %s not in the tarball", label, p.binary()) {
		return
	}
	// npm records the file mode in the tarball, so a binary packed without
	// the execute bit installs unable to run.
	n.check(binary.mode&0o111 != 0, "%s: the binary is not executable in the tarball (mode %o)", label, binary.mode)
	n.checkBinary(label, p, binary.data, n.record, p.npmKey)
}

// install installs the launcher and the host's platform package from their
// tarballs into a throwaway project, confirms npm resolved only the matching
// platform package, drives an MCP handshake through the launcher, and on
// Linux checks the server stops when npx is told to.
func (n *npmValidation) install() {
	host, ok := hostPlatform()
	if !ok {
		fmt.Fprintf(n.stdout, "runtime: host is %s/%s, no matching package to run; structural checks only\n", runtime.GOOS, runtime.GOARCH)
		return
	}
	platform, err := n.pack(n.validation, filepath.Join(n.packages, host.npmKey), n.work)
	if err != nil {
		n.failf("runtime: %v", err)
		return
	}
	launcher, err := n.pack(n.validation, n.main, n.work)
	if err != nil {
		n.failf("runtime: %v", err)
		return
	}
	project, err := os.MkdirTemp(n.work, "proj-")
	if err == nil {
		err = os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"name":"v","version":"1.0.0","private":true}`), 0o600)
	}
	if err != nil {
		n.failf("runtime: %v", err)
		return
	}
	if out, installErr := n.run(project, nil, "npm", "install", "--no-audit", "--no-fund", platform, launcher); installErr != nil {
		n.failf("runtime: npm install failed: %v\n%s", installErr, out)
		return
	}
	scope, err := os.ReadDir(filepath.Join(project, "node_modules", npmScope))
	if err != nil {
		n.failf("runtime: npm installed nothing under node_modules/%s: %v", npmScope, err)
		return
	}
	var installed []string
	for _, entry := range scope {
		installed = append(installed, entry.Name())
	}
	want := "gitlab-mcp-server-" + host.npmKey
	n.check(slices.Contains(installed, "gitlab-mcp-server") && slices.Contains(installed, want), "runtime: node_modules/%s holds %q", npmScope, installed)
	for _, name := range installed {
		n.check(!strings.HasPrefix(name, "gitlab-mcp-server-") || name == want, "runtime: a package for another platform was installed: %q", installed)
	}
	bin := filepath.Join(project, "node_modules", ".bin", binaryName)
	if runtime.GOOS == "windows" {
		bin += ".cmd"
	}
	n.handshake("npm launcher on "+host.npmKey, project, nil, npmHandshakeLimit, bin)
	n.stopUnderNpx(project, host)
}
