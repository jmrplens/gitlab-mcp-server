// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// server_json_oci_test.go covers the image check on what the record leaves
// out: identifiers of every shape, an entry with no identifier, and
// registries that answer with something other than what was asked for. On
// each of those the script either did the same or ended the whole run under
// set -e with no line naming the package; the check fails the package and
// goes on.
package main

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestParseImageReference_Identifiers_SplitAsTheScriptSplitThem covers how
// an identifier is taken apart, as the shell's parameter expansions took it
// apart.
func TestParseImageReference_Identifiers_SplitAsTheScriptSplitThem(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		identifier string
		want       imageReference
	}{
		{identifier: "ghcr.io/a/b:1.0@sha256:x", want: imageReference{host: "ghcr.io", repo: "a/b", tag: "1.0", digest: "sha256:x"}},
		{identifier: "ghcr.io/a/b", want: imageReference{host: "ghcr.io", repo: "a/b"}},
		{identifier: "ghcr.io/a/b@sha256:x", want: imageReference{host: "ghcr.io", repo: "a/b", digest: "sha256:x"}},
		{identifier: "docker.io/a/b:c:d", want: imageReference{host: "docker.io", repo: "a/b", tag: "d"}},
		{identifier: "image", want: imageReference{host: "image", repo: "image"}},
		{identifier: "ghcr.io/@sha256:x", want: imageReference{host: "ghcr.io", digest: "sha256:x"}},
		{identifier: "ghcr.io/:1", want: imageReference{host: "ghcr.io", tag: "1"}},
		{identifier: "image:1", want: imageReference{host: "image:1", repo: "image", tag: "1"}},
	} {
		t.Run(tc.identifier, func(t *testing.T) {
			t.Parallel()

			if got := parseImageReference(tc.identifier); got != tc.want {
				t.Errorf("parseImageReference(%q) = %+v, want %+v", tc.identifier, got, tc.want)
			}
		})
	}
}

// ociJSON is a server.json declaring the given packages under the server's
// own name.
func ociJSON(packages ...string) string {
	return `{"name":"io.github.jmrplens/gitlab-mcp-server","packages":[` + strings.Join(packages, ",") + `]}`
}

// tokenRoute answers the ghcr.io pull token of repo.
func tokenRoute(repo, body string) (string, route) {
	return "GET https://ghcr.io/token?scope=repository:" + repo + ":pull", route{Body: body}
}

// TestCheckImage_RegistriesThatAnswerOddly_FailThePackage covers a pull
// token that cannot be had or read, an index or a manifest that is not
// JSON, a manifest that cannot be fetched, a config blob that is not JSON,
// a platform with neither an operating system nor an architecture, and an
// entry with no identifier, which is skipped as the script skipped it.
func TestCheckImage_RegistriesThatAnswerOddly_FailThePackage(t *testing.T) {
	t.Parallel()

	label := `{"config":{"Labels":{"io.modelcontextprotocol.server.name":"io.github.jmrplens/gitlab-mcp-server"},"Cmd":["--transport","stdio"]}}`
	for _, tc := range []struct {
		name    string
		repo    string
		noToken bool
		routes  map[string]route
		stdout  []string
		stderr  []string
	}{
		{
			name:    "a token the registry refuses",
			repo:    "a/refused",
			noToken: true,
			routes:  map[string]route{},
			stderr:  []string{"  FAIL: no pull token for a/refused could be read from the registry"},
		},
		{
			name:    "a token answer that is not JSON",
			repo:    "a/garbled",
			noToken: true,
			routes:  map[string]route{"GET https://ghcr.io/token?scope=repository:a/garbled:pull": {Body: "<html>"}},
			stderr:  []string{"  FAIL: no pull token for a/garbled could be read from the registry"},
		},
		{
			name: "an index that is not JSON",
			repo: "a/index",
			routes: map[string]route{
				"GET https://ghcr.io/v2/a/index/manifests/1": {Body: "not json"},
			},
			stderr: []string{"  FAIL: single: the image manifest declares no config digest"},
		},
		{
			name: "a platform manifest that cannot be fetched, and one that is not JSON",
			repo: "a/children",
			routes: map[string]route{
				"GET https://ghcr.io/v2/a/children/manifests/1":        {Body: `{"manifests":[{"digest":"sha256:1","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:2"}]}`},
				"GET https://ghcr.io/v2/a/children/manifests/sha256:2": {Body: "not json"},
			},
			stderr: []string{
				"  FAIL: linux/amd64: the image manifest could not be fetched",
				"  FAIL: ?/?: the image manifest declares no config digest",
			},
		},
		{
			name: "a config blob that is not JSON",
			repo: "a/blob",
			routes: map[string]route{
				"GET https://ghcr.io/v2/a/blob/manifests/1":    {Body: `{"config":{"digest":"sha256:c"}}`},
				"GET https://ghcr.io/v2/a/blob/blobs/sha256:c": {Body: "not json"},
			},
			stderr: []string{`  FAIL: single: ownership label is "", expected "io.github.jmrplens/gitlab-mcp-server"`},
		},
		{
			name: "an image that passes on a stdio CMD",
			repo: "a/ok",
			routes: map[string]route{
				"GET https://ghcr.io/v2/a/ok/manifests/1":    {Body: `{"config":{"digest":"sha256:c"}}`},
				"GET https://ghcr.io/v2/a/ok/blobs/sha256:c": {Body: label},
			},
			stdout: []string{"  checked 1 platform manifest(s)", "  OK: ownership label matches, CMD infers the transport", "All 1 declared package(s) validated"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !tc.noToken {
				key, value := tokenRoute(tc.repo, `{"token":"t"}`)
				tc.routes[key] = value
			}
			got := runCheck(t, ociJSON(`{"registryType":"oci","identifier":""}`, `{"registryType":"oci","identifier":"ghcr.io/`+tc.repo+`:1"}`), tc.routes, nil, nil)
			wantOut := strings.Join(append([]string{"oci: ghcr.io/" + tc.repo + ":1"}, tc.stdout...), "\n") + "\n"
			if got.stdout != wantOut {
				t.Errorf("stdout\n%s\nwant\n%s", got.stdout, wantOut)
			}
			wantErr := slices.Clone(tc.stderr)
			if len(wantErr) > 0 {
				wantErr = append(wantErr, strconv.Itoa(len(tc.stderr))+" of 1 declared package(s) failed validation")
			}
			if gotErr := commandStderr(got.stderr); !slices.Equal(gotErr, wantErr) {
				t.Errorf("stderr\n%s\nwant\n%s", strings.Join(gotErr, "\n"), strings.Join(wantErr, "\n"))
			}
		})
	}
}
