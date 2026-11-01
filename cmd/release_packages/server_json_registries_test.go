// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// server_json_registries_test.go covers the npm, PyPI, NuGet and release
// branches on what the record leaves out: entries with no identifier, a
// release no npm version names, answers that are not JSON, and how the
// ownership token is matched.
package main

import (
	"strings"
	"testing"
)

// TestServerJSONCheck_EntriesWithNoIdentifier_AreSkipped verifies that an
// npm or NuGet entry whose identifier is empty is not checked or counted,
// as the script's guard skipped it, while the first npm entry's version
// still names the release to read, as the script read it; and that an npm
// entry with no version names no release.
func TestServerJSONCheck_EntriesWithNoIdentifier_AreSkipped(t *testing.T) {
	t.Parallel()

	got := runCheck(t, ociJSON(
		`{"registryType":"npm","identifier":"","version":"1.0.0"}`,
		`{"registryType":"nuget","identifier":"","version":"1.0.0"}`,
	), map[string]route{
		"GET https://api.github.com/repos/jmrplens/gitlab-mcp-server/releases/tags/v1.0.0": {Body: `{"immutable":true}`},
	}, nil, nil)
	if want := "ERROR: no mcpb, oci, npm, pypi or nuget packages found in server.json\n"; got.code != 1 || got.stderr != want ||
		got.stdout != "release: v1.0.0\n  OK: release v1.0.0 is immutable\n" {
		t.Errorf("exit %d, stdout %q, stderr %q, want 1, the release and %q", got.code, got.stdout, got.stderr, want)
	}
	unversioned := runCheck(t, ociJSON(`{"registryType":"npm","identifier":"x"}`), nil, nil, nil)
	if strings.Contains(unversioned.stdout, "release:") || len(unversioned.sent) != 0 {
		t.Errorf("an npm entry with no version read a release: %q, %d requests", unversioned.stdout, len(unversioned.sent))
	}
}

// TestServerJSONCheck_AnswersThatAreNotJSON_AreReadAsEmpty covers each branch
// that reads a JSON answer, handed something else: the npm registry's (no
// mcpName, so a note), PyPI's (no description, so no token), NuGet's index
// (no versions, so a note) and GitHub's release (no immutable flag, so the
// warning), and a release answer with no body at all.
func TestServerJSONCheck_AnswersThatAreNotJSON_AreReadAsEmpty(t *testing.T) {
	t.Parallel()

	launcher := map[string]string{npmLauncher: `{"name":"x","version":"1.0.0","mcpName":"io.github.jmrplens/gitlab-mcp-server"}`}
	for _, tc := range []struct {
		name, serverJSON string
		routes           map[string]route
		files            map[string]string
		stdout, stderr   string
	}{
		{
			name:       "npm and a release that is not JSON",
			serverJSON: ociJSON(`{"registryType":"npm","identifier":"x","version":"1.0.0"}`),
			routes: map[string]route{
				"GET https://registry.npmjs.org/x/1.0.0":                                           {Body: "<html>"},
				"GET https://api.github.com/repos/jmrplens/gitlab-mcp-server/releases/tags/v1.0.0": {Body: "<html>"},
			},
			files:  launcher,
			stdout: "  NOTE: published 1.0.0 predates mcpName",
			stderr: "  WARNING: release v1.0.0 is mutable: ",
		},
		{
			name:       "a release answered with nothing",
			serverJSON: ociJSON(`{"registryType":"npm","identifier":"x","version":"1.0.0"}`),
			routes: map[string]route{
				"GET https://registry.npmjs.org/x/1.0.0":                                           {Body: `{"mcpName":"io.github.jmrplens/gitlab-mcp-server"}`},
				"GET https://api.github.com/repos/jmrplens/gitlab-mcp-server/releases/tags/v1.0.0": {},
			},
			files:  launcher,
			stdout: "  OK: name, mcpName and version in lockstep",
			stderr: "  WARNING: could not read the v1.0.0 release metadata; immutability not checked",
		},
		{
			name:       "PyPI",
			serverJSON: ociJSON(`{"registryType":"pypi","identifier":"x","version":"1.0.0"}`),
			routes:     map[string]route{"GET https://pypi.org/pypi/x/1.0.0/json": {Body: "<html>"}},
			stderr:     `  FAIL: published README lacks the "mcp-name: io.github.jmrplens/gitlab-mcp-server" ownership token`,
		},
		{
			name:       "the NuGet index",
			serverJSON: ociJSON(`{"registryType":"nuget","identifier":"X","version":"1.0.0","registryBaseUrl":"https://api.nuget.org/v3/index.json"}`),
			routes:     map[string]route{"GET https://api.nuget.org/v3-flatcontainer/x/index.json": {Body: "<html>"}},
			stdout:     "  NOTE: nuget.org lists X but not 1.0.0 yet",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := runCheck(t, tc.serverJSON, tc.routes, tc.files, nil)
			if !strings.Contains(got.stdout, tc.stdout) || !strings.Contains(got.stderr, tc.stderr) {
				t.Errorf("stdout %q and stderr %q, want %q and %q", got.stdout, got.stderr, tc.stdout, tc.stderr)
			}
		})
	}
}

// TestOwnershipToken_TheServerName_IsMatchedLiterally verifies the token on
// each side of the name: after whitespace or at the start of a line, before
// whitespace, the end of a line or a closer the README's markup allows, and
// the name itself matched as written, where the script's grep read its dots
// as any character.
func TestOwnershipToken_TheServerName_IsMatchedLiterally(t *testing.T) {
	t.Parallel()

	token := ownershipToken("io.github.a/b", "<", "-->")
	for _, tc := range []struct {
		text string
		want bool
	}{
		{text: "mcp-name: io.github.a/b", want: true},
		{text: "intro\n  mcp-name: io.github.a/b\nmore", want: true},
		{text: "<!-- mcp-name: io.github.a/b -->", want: true},
		{text: "mcp-name: io.github.a/b-->", want: true},
		{text: "mcp-name: io.github.a/b<br>", want: true},
		{text: "xmcp-name: io.github.a/b", want: false},
		{text: "mcp-name: io.github.a/bc", want: false},
		{text: "mcp-name: ioXgithubXa/b", want: false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()

			if got := token.MatchString(tc.text); got != tc.want {
				t.Errorf("match %q = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestASCIILower_LowersOnlyASCIILetters covers the case folding of NuGet
// identifiers and versions, which tr does byte by byte in the C locale.
func TestASCIILower_LowersOnlyASCIILetters(t *testing.T) {
	t.Parallel()

	if got := asciiLower("Example.NoMcp-RC1.Ä"); got != "example.nomcp-rc1.Ä" {
		t.Errorf("asciiLower = %q", got)
	}
}
