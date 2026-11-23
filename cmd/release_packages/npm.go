// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// The npm distribution: one thin launcher package, checked in under
// npmMainDir, and one package per platform that carries its binary (the
// esbuild and biome model: npm installs only the package whose os and cpu
// fit, and no code runs at install time).
const (
	npmScope      = "@jmrp.io"
	npmMainName   = npmScope + "/gitlab-mcp-server"
	npmMainDir    = "npm/gitlab-mcp-server"
	npmPackageDir = "npm/packages"
)

// npmPlatformName is the npm name of a platform package.
func npmPlatformName(p releasePlatform) string { return npmMainName + "-" + p.npmKey }

// runNpm is the npm subcommand: it assembles the publish set from the
// release assets, or with --sync-only rewrites the launcher's version and
// its pins alone.
func runNpm(root string, args []string, stdout, stderr io.Writer) int {
	flags := newBuilderFlags("npm", npmPackageDir, "output directory: the six platform package directories in it are replaced, and anything else there is kept", stderr)
	syncOnly := flags.set.Bool("sync-only", false, "rewrite only the launcher's version and its optionalDependencies pins")
	if !flags.parse(args, stderr, false) {
		return 2
	}
	if !*syncOnly && flags.binaries == "" {
		fmt.Fprintf(stderr, "%s: --binaries <dir> is required (or pass --sync-only)\n", flags.set.Name())
		return 2
	}
	b := &packageBuild{root: root, binaries: flags.binaries, allowUnverified: flags.allowUnverified, stdout: stdout, stderr: stderr}
	if !plainVersion.MatchString(flags.version) {
		_ = b.refuse("--version must be semver, got %s", flags.version)
		return 1
	}
	if *syncOnly {
		if err := syncNpmMain(root, flags.version); err != nil {
			_ = b.refuse("%v", err)
			return 1
		}
		fmt.Fprintf(stdout, "npm main package synced to v%s (%s)\n", flags.version, npmMainName)
		return 0
	}
	if err := b.buildNpm(flags.version, flags.out); err != nil {
		if !errors.Is(err, errRefused) {
			_ = b.refuse("%v", err)
		}
		return 1
	}
	return 0
}

// syncNpmMain rewrites the launcher's package.json: its version, and every
// optional dependency pinned to the same version, so the set moves as one
// and a consumer never resolves a launcher against a binary package of
// another release. The rest of the file keeps its members and their order.
func syncNpmMain(root, version string) error {
	file, data, err := syncedNpmMain(root, version)
	if err != nil {
		return err
	}
	return writeFileMode(file, data, 0o644)
}

// syncedNpmMain reads the launcher's package.json and returns where it is
// and the bytes syncNpmMain writes there, so the npm build can refuse a
// launcher it cannot rewrite before it writes any package.
func syncedNpmMain(root, version string) (file string, data []byte, err error) {
	file = localPath(root, npmMainDir+"/package.json")
	data, err = os.ReadFile(file) //#nosec G304 -- the launcher's package.json in the repository
	if err != nil {
		return "", nil, err
	}
	pkg, err := parseJSON(data)
	if err != nil {
		return "", nil, fmt.Errorf("%s/package.json could not be read as JSON: %w", npmMainDir, err)
	}
	if pkg.kind != jsonObject {
		return "", nil, fmt.Errorf("%s/package.json is not a JSON object", npmMainDir)
	}
	pins := &jsonValue{kind: jsonObject}
	for _, p := range releasePlatforms {
		pins.set(npmPlatformName(p), jsonText(version))
	}
	pkg.set("version", jsonText(version))
	pkg.set("optionalDependencies", pins)
	return file, pkg.pretty(), nil
}

// npmPlatformManifest is the package.json of one platform package, member
// for member what the script wrote.
func npmPlatformManifest(p releasePlatform, version string) []byte {
	pkg := &jsonValue{kind: jsonObject}
	pkg.set("name", jsonText(npmPlatformName(p)))
	pkg.set("version", jsonText(version))
	pkg.set("description", jsonText(fmt.Sprintf(
		"gitlab-mcp-server prebuilt binary for %s %s. Installed automatically as an optional dependency of %s.", p.npmOS, p.npmCPU, npmMainName,
	)))
	pkg.set("license", jsonText("MIT"))
	pkg.set("funding", jsonList(
		&jsonValue{kind: jsonObject, members: []jsonMember{{key: "type", value: jsonText("github")}, {key: "url", value: jsonText("https://github.com/sponsors/jmrplens")}}},
		&jsonValue{kind: jsonObject, members: []jsonMember{{key: "type", value: jsonText("ko-fi")}, {key: "url", value: jsonText("https://ko-fi.com/jmrplens")}}},
	))
	pkg.set("author", jsonText("José M. Requena Plens"))
	pkg.set("homepage", jsonText("https://docs.jmrp.io/gitlab-mcp-server/"))
	pkg.set("repository", &jsonValue{kind: jsonObject, members: []jsonMember{
		{key: "type", value: jsonText("git")},
		{key: "url", value: jsonText("git+https://github.com/jmrplens/gitlab-mcp-server.git")},
	}})
	pkg.set("os", jsonList(jsonText(p.npmOS)))
	pkg.set("cpu", jsonList(jsonText(p.npmCPU)))
	if p.npmLibc != "" {
		pkg.set("libc", jsonList(jsonText(p.npmLibc)))
	}
	pkg.set("files", jsonList(jsonText(p.binary()), jsonText(licensePath), jsonText(noticesName)))
	pkg.set("preferUnplugged", &jsonValue{kind: jsonBool, boolean: true})
	return pkg.pretty()
}

// npmPlatformReadme is the README of one platform package.
func npmPlatformReadme(p releasePlatform) string {
	return fmt.Sprintf("# %s\n\nThe %s/%s binary for [%s](https://www.npmjs.com/package/%s). "+
		"You do not install this directly; it comes in as an optional dependency of the main package.\n",
		npmPlatformName(p), p.npmOS, p.npmCPU, npmMainName, npmMainName)
}

// buildNpm writes one directory per platform package under out, syncs the
// launcher and copies the license texts into it, and records what was
// verified in out/verified-binaries.json for the validator. The launcher's
// copies of the texts are git-ignored, so each text keeps one source.
func (b *packageBuild) buildNpm(version, out string) error {
	license, err := b.readLicense()
	if err != nil {
		return err
	}
	in, err := b.readRelease("binaries are being packaged")
	if err != nil {
		return err
	}
	launcher, launcherData, err := syncedNpmMain(b.root, version)
	if err != nil {
		return err
	}
	outDir := localPath(b.root, out)
	if writeErr := os.MkdirAll(outDir, 0o750); writeErr != nil {
		return writeErr
	}
	shown := filepath.ToSlash(out)
	var dirs []string
	var digests []jsonMember
	for _, p := range releasePlatforms {
		dir := filepath.Join(outDir, p.npmKey)
		if writeErr := freshDir(dir); writeErr != nil {
			return writeErr
		}
		files := []struct {
			name string
			data []byte
			mode os.FileMode
		}{
			{p.binary(), in.binaries[p.key], 0o755},
			{licensePath, license, 0o644},
			{noticesName, in.notices, 0o644},
			{"package.json", npmPlatformManifest(p, version), 0o644},
			{"README.md", []byte(npmPlatformReadme(p)), 0o644},
		}
		for _, f := range files {
			if writeErr := writeFileMode(filepath.Join(dir, f.name), f.data, f.mode); writeErr != nil {
				return writeErr
			}
		}
		dirs = append(dirs, path.Join(shown, p.npmKey))
		digests = append(digests, jsonMember{key: p.npmKey, value: jsonText(in.binaryDigests[p.key])})
	}
	if writeErr := writeFileMode(launcher, launcherData, 0o644); writeErr != nil {
		return writeErr
	}
	mainDir := localPath(b.root, npmMainDir)
	if writeErr := writeFileMode(filepath.Join(mainDir, licensePath), license, 0o644); writeErr != nil {
		return writeErr
	}
	if writeErr := writeFileMode(filepath.Join(mainDir, noticesName), in.notices, 0o644); writeErr != nil {
		return writeErr
	}
	// Beside the package directories, never inside one, so it cannot be
	// published.
	if writeErr := writeFileMode(filepath.Join(outDir, verifiedManifest), verifiedRecord(version, in.verified, digests, in.noticesDigest), 0o644); writeErr != nil {
		return writeErr
	}
	// The platform packages must be on the registry before the launcher that
	// lists them, or an install racing the publish resolves optional
	// dependencies that are not there yet.
	fmt.Fprintf(b.stdout, "npm distribution assembled for v%s\n\n", version)
	fmt.Fprintln(b.stdout, "Publish in this order (platform packages first):")
	for _, dir := range dirs {
		fmt.Fprintf(b.stdout, "  %s\n", dir)
	}
	fmt.Fprintf(b.stdout, "  %s   (%s)\n", npmMainDir, npmMainName)
	return nil
}
