// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// manifest_test.go holds the derivation of the per-OS manifests to what
// mcpb/platform.jq derived, byte for byte, and the committed manifest to
// what each per-OS bundle needs from it.
//
// testdata/platform_jq.json is the record: every case below was run through
// `jq --arg platform P -f mcpb/platform.jq` at fe8773534, the revision of
// main this port started from, and its exit status, the bytes jq wrote and
// the message of its error were kept. The inputs are testdata/manifests: a
// copy of mcpb/manifest.json as that revision held it, the mutations
// scripts/build_mcpb_sh_test.py and scripts/mcpb_manifest_test.py made of it,
// and escapes.json, which spells every non-ASCII character as an escape so
// that the record holds jq's own spelling of each.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// platformJQRecord is testdata/platform_jq.json.
type platformJQRecord struct {
	RecordedFrom string `json:"recorded_from"`
	Cases        []struct {
		Manifest string `json:"manifest"`
		Platform string `json:"platform"`
		Exit     int    `json:"exit"`
		Stdout   string `json:"stdout"`
		Error    string `json:"error"`
	} `json:"cases"`
}

// readRecord decodes a record in testdata.
func readRecord(t *testing.T, name string, into any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if decodeErr := json.Unmarshal(data, into); decodeErr != nil {
		t.Fatalf("decoding the record %s: %v", name, decodeErr)
	}
}

// TestPlatformManifest_PlatformJQRecord_DerivesTheBytesJQWrote verifies that
// the mcpb-manifest subcommand writes, for every recorded case, exactly the
// bytes mcpb/platform.jq wrote: the same members in the same order, the same
// indent, every string escaped as jq escapes it and every number as written.
// A case jq refused is refused with jq's message and nothing on stdout.
func TestPlatformManifest_PlatformJQRecord_DerivesTheBytesJQWrote(t *testing.T) {
	t.Parallel()

	var record platformJQRecord
	readRecord(t, "platform_jq.json", &record)
	if len(record.Cases) == 0 {
		t.Fatal("the record holds no case")
	}
	for _, tc := range record.Cases {
		t.Run(tc.Manifest+"/"+tc.Platform, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := runIn("testdata", []string{"mcpb-manifest", tc.Platform, "manifests/" + tc.Manifest + ".json"}, &stdout, &stderr, noEnv)
			if tc.Exit == 0 {
				if code != 0 {
					t.Fatalf("exit %d, want 0 (stderr %q)", code, stderr.String())
				}
				if got := stdout.String(); got != tc.Stdout {
					t.Errorf("derived manifest differs from what jq wrote:\n%s", firstDifference(got, tc.Stdout))
				}
				return
			}
			if code != 1 || stdout.Len() != 0 {
				t.Errorf("exit %d with %d bytes on stdout, want 1 and nothing, as jq refused it", code, stdout.Len())
			}
			if tc.Error == "" || !strings.Contains(stderr.String(), tc.Error) {
				t.Errorf("stderr %q, want jq's message %q", stderr.String(), tc.Error)
			}
		})
	}
}

// firstDifference describes where two texts part, for a failure message that
// a multi-kilobyte manifest would otherwise drown.
func firstDifference(got, want string) string {
	at := 0
	for at < len(got) && at < len(want) && got[at] == want[at] {
		at++
	}
	from := max(0, at-80)
	return "at byte " + strconv.Itoa(at) + "\n got: ..." + got[from:min(len(got), at+80)] + "\nwant: ..." + want[from:min(len(want), at+80)]
}

// noEnv is an environment that sets nothing.
func noEnv(string) string { return "" }

// committedManifest reads mcpb/manifest.json as the tree holds it now.
func committedManifest(t *testing.T) *jsonValue {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "mcpb", "manifest.json"))
	if err != nil {
		t.Fatalf("reading the committed manifest: %v", err)
	}
	manifest, err := parseJSON(data)
	if err != nil {
		t.Fatalf("parsing the committed manifest: %v", err)
	}
	return manifest
}

// TestDerivePlatformManifest_CommittedManifest_GivesEachPlatformItsOwnCommand
// verifies, on the manifest the tree holds now rather than the recorded copy,
// that each per-OS manifest lists only its own platform, starts its own
// command as the base command with no override left, names a path inside
// the bundle as its entry point, and keeps the base env whole, since no
// override carries one. Spelled out rather than derived, so a change to the
// derivation that still agrees with a reading of it is caught here.
func TestDerivePlatformManifest_CommittedManifest_GivesEachPlatformItsOwnCommand(t *testing.T) {
	t.Parallel()

	committed := committedManifest(t)
	for _, tc := range []struct {
		platform, command, entryPoint string
		args                          []string
	}{
		{platform: "darwin", command: "${__dirname}/server/gitlab-mcp-server", entryPoint: "server/gitlab-mcp-server", args: nil},
		{platform: "win32", command: "${__dirname}/server/gitlab-mcp-server.exe", entryPoint: "server/gitlab-mcp-server.exe", args: nil},
		{platform: "linux", command: "/bin/sh", entryPoint: "server/linux/launch.sh", args: []string{"${__dirname}/server/linux/launch.sh"}},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			t.Parallel()

			derived, err := derivePlatformManifest(committed, tc.platform)
			if err != nil {
				t.Fatalf("derivePlatformManifest: %v", err)
			}
			config := derived.field("server").field("mcp_config")
			if got := derived.field("compatibility").field("platforms").strings(); !reflect.DeepEqual(got, []string{tc.platform}) {
				t.Errorf("compatibility.platforms = %q, want only %q", got, tc.platform)
			}
			if got := config.field("command").text; got != tc.command {
				t.Errorf("command = %q, want %q", got, tc.command)
			}
			if got := config.field("args").strings(); !reflect.DeepEqual(got, tc.args) || config.field("args").kind != jsonArray {
				t.Errorf("args = %q, want the array %q", got, tc.args)
			}
			if config.has("platform_overrides") {
				t.Error("the derived manifest keeps platform_overrides")
			}
			if got := derived.field("server").field("entry_point").text; got != tc.entryPoint {
				t.Errorf("entry_point = %q, want %q", got, tc.entryPoint)
			}
			if !config.field("env").equal(committed.field("server").field("mcp_config").field("env")) {
				t.Error("the derived env is not the base env")
			}
		})
	}
}

// TestDerivePlatformManifest_CommittedManifest_KeepsEverythingButTheLaunch
// verifies that a per-OS manifest is the committed one in every member but
// the launch and the platform list, the name above all: the per-OS bundles
// are one extension, so installing one replaces the universal bundle rather
// than adding a second copy beside it.
func TestDerivePlatformManifest_CommittedManifest_KeepsEverythingButTheLaunch(t *testing.T) {
	t.Parallel()

	committed := committedManifest(t)
	for _, platform := range desktopPlatforms {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()

			derived, err := derivePlatformManifest(committed, platform)
			if err != nil {
				t.Fatalf("derivePlatformManifest: %v", err)
			}
			assertKept(t, "", committed, derived, "server", "compatibility")
			assertKept(t, "server.", committed.field("server"), derived.field("server"), "entry_point", "mcp_config")
			assertKept(t, "compatibility.", committed.field("compatibility"), derived.field("compatibility"), "platforms")
		})
	}
}

// assertKept holds every member of an object other than the skipped ones to
// its copy in another, and the two to the same number of members.
func assertKept(t *testing.T, prefix string, from, to *jsonValue, skipped ...string) {
	t.Helper()
	for _, member := range from.members {
		if slices.Contains(skipped, member.key) {
			continue
		}
		if got := to.get(member.key); got == nil || !member.value.equal(got) {
			t.Errorf("%s%s changed", prefix, member.key)
		}
	}
	if len(to.members) != len(from.members) {
		t.Errorf("%s has %d members, the committed manifest %d", strings.TrimSuffix(prefix, "."), len(to.members), len(from.members))
	}
}

// TestDerivePlatformManifest_ShapesNoCommittedManifestHas_AreReadAsJQReadThem
// covers the derivation on shapes the record leaves out: the kinds jq
// refused with an error of its own, which are refused here too, a null
// manifest or config, which has no command line to name a path, an object
// for args, whose values jq iterates, and a command naming the bundle's
// directory itself.
func TestDerivePlatformManifest_ShapesNoCommittedManifestHas_AreReadAsJQReadThem(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, manifest, platform, want, refused string
	}{
		{name: "an override that is a string", manifest: `{"server":{"mcp_config":{"command":"x","platform_overrides":{"linux":"sh"}}}}`, platform: "linux", refused: "cannot check whether string has a string key"},
		{name: "overrides that are an array", manifest: `{"server":{"mcp_config":{"platform_overrides":[1]}}}`, platform: "linux", refused: `cannot index array with "linux"`},
		{name: "a server that is a string", manifest: `{"server":"x"}`, platform: "darwin", refused: `cannot index string with "mcp_config"`},
		{name: "a manifest that is a list", manifest: `[]`, platform: "darwin", refused: `cannot index array with "server"`},
		{name: "a config that is a number", manifest: `{"server":{"mcp_config":7}}`, platform: "darwin", refused: "cannot delete field at object index of number"},
		{name: "args that are a string", manifest: `{"server":{"mcp_config":{"command":"${__dirname}/x","args":"y"}}}`, platform: "darwin", refused: "cannot iterate over string"},
		{name: "a command that is a list", manifest: `{"server":{"mcp_config":{"command":["${__dirname}/x"]}}}`, platform: "darwin", refused: "the darwin command line names no path inside the bundle"},
		{name: "a null manifest", manifest: `null`, platform: "linux", refused: "the linux command line names no path inside the bundle"},
		{name: "a null config", manifest: `{"server":{"mcp_config":null}}`, platform: "win32", refused: "the win32 command line names no path inside the bundle"},
		{name: "compatibility that is a string", manifest: `{"server":{"mcp_config":{"command":"${__dirname}/x"}},"compatibility":"all"}`, platform: "darwin", refused: `cannot index string with "platforms"`},
		{name: "an object's args are its values", manifest: `{"server":{"mcp_config":{"command":"sh","args":{"a":"${__dirname}/b"}}}}`, platform: "darwin", want: `{"server":{"mcp_config":{"command":"sh","args":{"a":"${__dirname}/b"}},"entry_point":"b"},"compatibility":{"platforms":["darwin"]}}`},
		{name: "the bundle's directory itself", manifest: `{"server":{"mcp_config":{"command":"${__dirname}/"}}}`, platform: "win32", want: `{"server":{"mcp_config":{"command":"${__dirname}/"},"entry_point":""},"compatibility":{"platforms":["win32"]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := parseJSON([]byte(tc.manifest))
			if err != nil {
				t.Fatalf("parseJSON: %v", err)
			}
			derived, err := derivePlatformManifest(manifest, tc.platform)
			if tc.refused != "" {
				if err == nil || err.Error() != tc.refused {
					t.Errorf("derivePlatformManifest error = %v, want %q", err, tc.refused)
				}
				return
			}
			if err != nil {
				t.Fatalf("derivePlatformManifest: %v", err)
			}
			if got := derived.compact(); got != tc.want {
				t.Errorf("derived %s, want %s", got, tc.want)
			}
		})
	}
}

// TestStampVersion_AManifest_KeepsTheVersionsPlaceAndWhatItRead verifies
// that the version is set where the manifest holds it, or last when it holds
// none, as jq's `.version = $v` sets it, and that the manifest read is left
// as it was.
func TestStampVersion_AManifest_KeepsTheVersionsPlaceAndWhatItRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, manifest, want string }{
		{name: "a version", manifest: `{"a":1,"version":"0","b":2}`, want: `{"a":1,"version":"9.8.7","b":2}`},
		{name: "no version", manifest: `{"a":1}`, want: `{"a":1,"version":"9.8.7"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := parseJSON([]byte(tc.manifest))
			if err != nil {
				t.Fatal(err)
			}
			if got := stampVersion(manifest, "9.8.7").compact(); got != tc.want {
				t.Errorf("stampVersion = %s, want %s", got, tc.want)
			}
			if got := manifest.compact(); got != tc.manifest {
				t.Errorf("the manifest read became %s", got)
			}
		})
	}
}
