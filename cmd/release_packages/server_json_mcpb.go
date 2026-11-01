// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// servedPlatform is one platform a declared bundle's manifest lists.
type servedPlatform struct {
	platform, identifier string
}

// checkMCPB checks every mcpb package: a .mcpb is a zip carrying
// manifest.json and the server binaries, and declaring a bare executable
// under this registry type parses fine and installs nowhere. A release
// declares one bundle per operating system, and server.json declared the
// single universal bundle before that, so either shape is accepted as long
// as each system is served exactly once, which is judged once every bundle
// passed its own checks.
func (c *serverJSONCheck) checkMCPB() {
	packages := c.declared("mcpb")
	allPassed := true
	var served []servedPlatform
	for _, pkg := range packages {
		identifier := text(pkg, "identifier")
		if identifier == "" {
			continue
		}
		c.checked++
		c.sayf("mcpb: %s", identifier)
		platforms, ok := c.checkBundle(pkg, identifier)
		if !ok {
			allPassed = false
			continue
		}
		for _, platform := range platforms {
			served = append(served, servedPlatform{platform: platform, identifier: identifier})
		}
	}
	if len(packages) > 0 && allPassed {
		c.checkCoverage(served)
	}
}

// checkBundle checks one declared bundle and returns the platforms its
// manifest lists.
func (c *serverJSONCheck) checkBundle(pkg *jsonValue, identifier string) ([]string, bool) {
	declared := textOr(pkg, "fileSha256")
	if !strings.HasSuffix(identifier, ".mcpb") {
		c.failf("identifier does not name a .mcpb bundle")
		return nil, false
	}
	if declared == "" {
		c.failf("no fileSha256 declared")
		return nil, false
	}
	data, err := c.fetch.download(identifier)
	if err != nil {
		c.failf("not downloadable")
		return nil, false
	}
	actual := releaseassets.Digest(data)
	if actual != declared {
		c.failf("fileSha256 mismatch: declared %s..., actual %s...", prefix(declared, 16), prefix(actual, 16))
		return nil, false
	}
	platforms, err := c.readBundle(data)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		c.failf("not a valid MCP bundle")
		return nil, false
	}
	listed := "no platform"
	if len(platforms) > 0 {
		listed = strings.Join(platforms, " ")
	}
	c.sayf("  OK: %s..., valid bundle for %s", prefix(actual, 16), listed)
	return platforms, true
}

// prefix is the first n bytes of s, or s when it is shorter.
func prefix(s string, n int) string { return s[:min(n, len(s))] }

// readBundle opens a downloaded bundle as a zip and checks that every path
// its manifest names relative to the extension directory, in the base
// command and args and in each platform override, and its entry point, is in
// the archive: Claude Desktop substitutes ${__dirname} and starts that path,
// so a missing one is a platform on which the extension installs and never
// starts. It returns the platforms the manifest lists, the strings of
// compatibility.platforms when that is a list.
func (c *serverJSONCheck) readBundle(data []byte) ([]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a zip archive (magic bytes: %s)", magicBytes(data))
	}
	names := map[string]bool{}
	var manifestFile *zip.File
	for _, file := range archive.File {
		names[file.Name] = true
		if file.Name == "manifest.json" && manifestFile == nil {
			manifestFile = file
		}
	}
	if manifestFile == nil {
		return nil, errors.New("zip archive carries no manifest.json")
	}
	manifest, err := readManifest(manifestFile)
	if err != nil {
		return nil, fmt.Errorf("manifest.json is not JSON: %w", err)
	}
	if problems := bundlePathProblems(manifest, names); len(problems) > 0 {
		return nil, errors.New("the manifest names paths the bundle does not carry: " + strings.Join(problems, "; "))
	}
	c.sayf("  %d entries; every path the manifest names is in the archive", len(names))

	platforms := orEmpty(orEmpty(manifest).field("compatibility")).field("platforms")
	return platforms.strings(), nil
}

// magicBytes is the first four bytes of a file as space-separated hex.
func magicBytes(data []byte) string {
	head := data[:min(4, len(data))]
	parts := make([]string, len(head))
	for i, b := range head {
		parts[i] = hex.EncodeToString([]byte{b})
	}
	return strings.Join(parts, " ")
}

// orEmpty is Python's `value or {}` for the object a manifest holds: a value
// that is null, false, empty or not an object reads as an empty one.
func orEmpty(v *jsonValue) *jsonValue {
	if v.kind != jsonObject {
		return &jsonValue{kind: jsonObject}
	}
	return v
}

// bundlePathProblems lists the paths a manifest names under ${__dirname}
// that the archive does not carry, and those it names other than as the
// leading component, the base command line first and then each override in
// the order of its platform's name.
func bundlePathProblems(manifest *jsonValue, names map[string]bool) []string {
	server := orEmpty(orEmpty(manifest).field("server"))
	config := orEmpty(server.field("mcp_config"))
	type word struct {
		where string
		value *jsonValue
	}
	line := func(where string, of *jsonValue) []word {
		words := []word{{where, of.field("command")}}
		for _, arg := range of.field("args").items {
			words = append(words, word{where, arg})
		}
		return words
	}
	named := line("mcp_config", config)
	overrides := orEmpty(config.field("platform_overrides"))
	keys := make([]string, 0, len(overrides.members))
	for _, member := range overrides.members {
		keys = append(keys, member.key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		named = append(named, line(key, orEmpty(overrides.get(key)))...)
	}

	var problems []string
	for _, w := range named {
		if w.value.kind != jsonString || !strings.Contains(w.value.text, "${__dirname}") {
			continue
		}
		rel, leading := strings.CutPrefix(w.value.text, dirname)
		switch {
		case !leading:
			problems = append(problems, fmt.Sprintf("%s: %s uses ${__dirname} other than as a leading path component", w.where, w.value.text))
		case !names[rel]:
			problems = append(problems, fmt.Sprintf("%s: %s is not in the archive", w.where, rel))
		}
	}
	if entryPoint := server.field("entry_point"); entryPoint.truthy() && (entryPoint.kind != jsonString || !names[entryPoint.text]) {
		problems = append(problems, fmt.Sprintf("server.entry_point: %s is not in the archive", entryPoint.interpolate()))
	}
	return problems
}

// checkCoverage holds the declared bundles to serving each of darwin, win32
// and linux exactly once between them. A registry entry has no platform
// field, so a client cannot choose between bundles by system: none is a
// system the listing offers nothing it can install, and two is a system
// offered two bundles with nothing to tell them apart, which is what
// declaring the universal bundle beside the per-OS ones would do.
func (c *serverJSONCheck) checkCoverage(served []servedPlatform) {
	c.sayf("mcpb: platforms served")
	ok := true
	for _, platform := range desktopPlatforms {
		var serving []string
		for _, s := range served {
			if s.platform == platform {
				serving = append(serving, s.identifier)
			}
		}
		switch len(serving) {
		case 0:
			c.failf("no declared bundle serves %s, so a client there is offered nothing it can install", platform)
			ok = false
		case 1:
		default:
			c.failf("%s is served by %d declared bundles (%s), and a registry entry has no platform field to choose between them", platform, len(serving), strings.Join(serving, " "))
			ok = false
		}
	}
	for _, s := range served {
		if !slices.Contains(desktopPlatforms, s.platform) {
			c.failf("%s lists %s, which is not a platform Claude Desktop reports", s.identifier, s.platform)
			ok = false
		}
	}
	if ok {
		c.sayf("  OK: darwin, win32 and linux are each served by exactly one declared bundle")
	}
}

// readZipNames returns the names of the entries of a zip held in memory.
func readZipNames(data []byte) ([]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(archive.File))
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	return names, nil
}
