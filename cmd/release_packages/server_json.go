// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// serverJSONCheck is one run of the server-json-packages subcommand: every
// package server.json declares, downloaded or looked up where it is
// published and held to what it claims to be. `make check-server-json`
// validates the manifest against the registry schema, and a schema cannot
// see that a package declared as registryType mcpb points at a raw ELF
// binary, that an OCI tag was never pushed, that an OCI entry carries a field
// the registry rejects only at publish time, or that a NuGet version was
// published without the README the registry reads its ownership token from.
// Those are exactly the mistakes that leave a directory listing scoring the
// server on an artifact no client can install.
type serverJSONCheck struct {
	// root is the directory relative paths are read from, and serverJSON the
	// manifest's path as given and as the messages name it.
	root, serverJSON string
	stdout, stderr   io.Writer
	getenv           func(string) string
	fetch            *fetcher

	name     string
	packages []*jsonValue
	checked  int
	failures int
}

// newServerJSONCheck returns a run over the network.
func newServerJSONCheck(root, serverJSON string, stdout, stderr io.Writer, getenv func(string) string) *serverJSONCheck {
	return &serverJSONCheck{root: root, serverJSON: serverJSON, stdout: stdout, stderr: stderr, getenv: getenv, fetch: newFetcher(stderr)}
}

// local is a path named relative to the working directory, on disk.
func (c *serverJSONCheck) local(rel string) string { return localPath(c.root, rel) }

// sayf prints a line on stdout.
func (c *serverJSONCheck) sayf(format string, args ...any) {
	fmt.Fprintf(c.stdout, format+"\n", args...)
}

// failf reports one failed check of the package being checked, and counts
// it.
func (c *serverJSONCheck) failf(format string, args ...any) {
	fmt.Fprintf(c.stderr, "  FAIL: "+format+"\n", args...)
	c.failures++
}

// warnf reports what is not a failure of a package.
func (c *serverJSONCheck) warnf(format string, args ...any) {
	fmt.Fprintf(c.stderr, "  WARNING: "+format+"\n", args...)
}

// run checks every declared package and returns the exit code.
func (c *serverJSONCheck) run() int {
	if !regularFile(c.local(c.serverJSON)) {
		fmt.Fprintf(c.stderr, "ERROR: %s not found\n", c.serverJSON)
		return 1
	}
	data, err := os.ReadFile(c.local(c.serverJSON)) //#nosec G304,G703 -- the server.json the operator named, read and never written
	var doc *jsonValue
	if err == nil {
		doc, err = parseJSON(data)
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "ERROR: %s could not be read as JSON: %v\n", c.serverJSON, err)
		return 1
	}
	c.name = doc.field("name").interpolate()
	c.packages = doc.field("packages").items

	c.checkMCPB()
	c.checkOCI()
	c.checkNPM()
	c.checkPyPI()
	c.checkNuGet()
	c.checkRelease()

	if c.checked == 0 {
		fmt.Fprintf(c.stderr, "ERROR: no mcpb, oci, npm, pypi or nuget packages found in %s\n", c.serverJSON)
		return 1
	}
	if c.failures > 0 {
		fmt.Fprintf(c.stderr, "%d of %d declared package(s) failed validation\n", c.failures, c.checked)
		return 1
	}
	c.sayf("All %d declared package(s) validated", c.checked)
	return 0
}

// declared returns the packages of one registry type, in their order.
func (c *serverJSONCheck) declared(registryType string) []*jsonValue {
	var out []*jsonValue
	for _, pkg := range c.packages {
		if pkg.field("registryType").equal(jsonText(registryType)) {
			out = append(out, pkg)
		}
	}
	return out
}

// text is a package field as jq -r prints it: a string as itself, null as
// "null", anything else as its JSON.
func text(pkg *jsonValue, key string) string { return pkg.field(key).interpolate() }

// textOr is a package field as `jq -r '.key // ""'` prints it: empty for a
// field that is null, false or absent.
func textOr(pkg *jsonValue, key string) string {
	if value := pkg.field(key); value.truthy() {
		return value.interpolate()
	}
	return ""
}

// bannedFields lists, in the package's own order, the fields it carries that
// the registry rejects for its type.
func bannedFields(pkg *jsonValue, banned ...string) []string {
	var found []string
	for _, member := range pkg.members {
		for _, key := range banned {
			if member.key == key {
				found = append(found, key)
			}
		}
	}
	return found
}

// sibling is a file beside server.json, as the messages name it.
func (c *serverJSONCheck) sibling(name string) string {
	return path.Dir(filepath.ToSlash(c.serverJSON)) + "/" + name
}
