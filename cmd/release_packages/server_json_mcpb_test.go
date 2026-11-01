// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// server_json_mcpb_test.go covers the bundle check's readers on what the
// record leaves out: the magic bytes of a file too short to hold four, and
// manifests whose objects are of other kinds, which read as empty ones the
// way Python's `value or {}` read them.
package main

import (
	"testing"
)

// TestMagicBytes_AFileOfAnyLength_IsShownInHex covers the bytes a refusal
// names, for an empty file, a short one and a long one.
func TestMagicBytes_AFileOfAnyLength_IsShownInHex(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		data []byte
		want string
	}{
		{data: nil, want: ""},
		{data: []byte("MZ"), want: "4d 5a"},
		{data: []byte("\x7fELF\x02"), want: "7f 45 4c 46"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := magicBytes(tc.data); got != tc.want {
				t.Errorf("magicBytes(%q) = %q, want %q", tc.data, got, tc.want)
			}
		})
	}
}

// TestBundlePathProblems_ManifestsOfOtherKinds_ReadAsEmpty covers a
// manifest, a server, a config and an override that are not objects, and
// args that are not a list: each reads as having nothing to check, and a
// path named as a list item is still checked.
func TestBundlePathProblems_ManifestsOfOtherKinds_ReadAsEmpty(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, manifest string
		want           int
	}{
		{name: "a manifest that is a list", manifest: `[1]`},
		{name: "a server that is a string", manifest: `{"server":"x"}`},
		{name: "a config that is null", manifest: `{"server":{"mcp_config":null}}`},
		{name: "args that are a string", manifest: `{"server":{"mcp_config":{"args":"${__dirname}/x"}}}`},
		{name: "an override that is a string", manifest: `{"server":{"mcp_config":{"platform_overrides":{"linux":"${__dirname}/x"}}}}`},
		{name: "a path in the args", manifest: `{"server":{"mcp_config":{"args":["${__dirname}/x"]}}}`, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := parseJSON([]byte(tc.manifest))
			if err != nil {
				t.Fatal(err)
			}
			if got := bundlePathProblems(manifest, map[string]bool{}); len(got) != tc.want {
				t.Errorf("problems %q, want %d", got, tc.want)
			}
		})
	}
}
