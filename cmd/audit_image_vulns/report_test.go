// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// report_test.go covers how one Trivy report is read: the fields the judge
// uses, every shape of report it refuses to vouch for, and how the reports of
// a run are held to the platforms the release pushes.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureVuln is one entry of a result's Vulnerabilities, spelled the way
// `trivy image --format json` writes it.
type fixtureVuln struct {
	Vendors   map[string]int `json:"VendorSeverity,omitempty"`
	ID        string         `json:"VulnerabilityID"`
	Pkg       string         `json:"PkgName"`
	Installed string         `json:"InstalledVersion"`
	Fixed     string         `json:"FixedVersion,omitempty"`
	Status    string         `json:"Status,omitempty"`
	Severity  string         `json:"Severity"`
	Source    string         `json:"SeveritySource,omitempty"`
	Title     string         `json:"Title,omitempty"`
}

// fixtureReport is the report Trivy writes for a smoke image of the given
// architecture: the image's Alpine packages, with osVulns against them, and
// the server binary, with binaryVulns against its modules. It is a map so a
// test can take out or change exactly the field it is about.
func fixtureReport(arch string, osVulns, binaryVulns []fixtureVuln) map[string]any {
	return map[string]any{
		"SchemaVersion": 2,
		"Trivy":         map[string]any{"Version": "0.76.1"},
		"ArtifactName":  "gitlab-mcp-server:smoke-" + arch,
		"ArtifactType":  "container_image",
		"Metadata": map[string]any{
			"OS":          map[string]any{"Family": "alpine", "Name": "3.24.2"},
			"ImageConfig": map[string]any{"architecture": arch, "os": "linux"},
		},
		"Results": []any{
			map[string]any{
				"Target": "gitlab-mcp-server:smoke-" + arch + " (alpine 3.24.2)",
				"Class":  "os-pkgs",
				"Type":   "alpine",
				"Packages": []any{
					map[string]any{"Name": "musl", "Version": "1.2.6-r2"},
					map[string]any{"Name": "zlib", "Version": "1.3.2-r0"},
				},
				"Vulnerabilities": osVulns,
			},
			map[string]any{
				"Target":          "usr/local/bin/gitlab-mcp-server",
				"Class":           "lang-pkgs",
				"Type":            "gobinary",
				"Packages":        []any{map[string]any{"Name": "stdlib", "Version": "v1.27.3"}},
				"Vulnerabilities": binaryVulns,
			},
		},
	}
}

// metadata is the Metadata object of a fixture report, for a test to change.
func metadata(report map[string]any) map[string]any {
	return report["Metadata"].(map[string]any)
}

// osResult is the os-pkgs result of a fixture report, for a test to change.
func osResult(report map[string]any) map[string]any {
	return report["Results"].([]any)[0].(map[string]any)
}

// writeJSON writes value as JSON to a file of its own and returns the path.
func writeJSON(t *testing.T, value any) string {
	t.Helper()

	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return writeFile(t, body)
}

// writeFile writes body to a file of its own and returns the path.
func writeFile(t *testing.T, body []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestReadScan_ReadsTheReleasedImage reads the report Trivy 0.76.1 wrote for
// the published 3.3.0 image, trimmed of the prose the judge does not read, and
// checks every field the judge uses comes out of it: the platform from the
// image configuration, the distribution, the OS packages, the three Alpine
// findings (two advisories, one of them against two packages) and the one
// against the binary, kept apart.
func TestReadScan_ReadsTheReleasedImage(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "release-3.3.0-amd64.json")
	got, err := readScan(path)
	if err != nil {
		t.Fatalf("readScan(%s): %v", path, err)
	}
	if got.path != path || got.platform != "linux/amd64" || got.distro != "alpine 3.24.2" || got.packages != 18 ||
		got.artifact != "ghcr.io/jmrplens/gitlab-mcp-server:3.3.0" || got.trivyVersion != "0.76.1" ||
		got.image != "sha256:1f4c18bc835754dda536f82048d1d68776008439313354efb02677072a03ced7" {
		t.Errorf("readScan = %+v, want linux/amd64, alpine 3.24.2, 18 packages, the tag, its image configuration and Trivy 0.76.1", got)
	}
	wantOS := []finding{
		{id: "CVE-2026-107891", pkg: "musl", installed: "1.2.6-r2", fixed: "1.2.6-r3", severity: "UNKNOWN"},
		{id: "CVE-2026-107891", pkg: "musl-utils", installed: "1.2.6-r2", fixed: "1.2.6-r3", severity: "UNKNOWN"},
		{
			id: "CVE-2026-85091", pkg: "zlib", installed: "1.3.2-r0", fixed: "1.3.2-r1", severity: "MEDIUM",
			vendors: []string{"ubuntu"}, title: "zlib versions 1.3.1.2 through 1.3.2 contain a heap buffer overflow vul ...",
		},
	}
	if !slices.EqualFunc(got.os, wantOS, findingsEqual) {
		t.Errorf("os findings =\n%+v\nwant\n%+v", got.os, wantOS)
	}
	if len(got.binaries) != 1 || got.binaries[0].pkg != "stdlib" || got.binaries[0].severity != "HIGH" || got.binaries[0].fixed != "1.26.10, 1.27.3" {
		t.Errorf("binary findings = %+v, want the one HIGH stdlib advisory", got.binaries)
	}
}

// findingsEqual compares two findings field by field.
func findingsEqual(a, b finding) bool {
	return a.id == b.id && a.pkg == b.pkg && a.installed == b.installed && a.fixed == b.fixed &&
		a.severity == b.severity && a.source == b.source && slices.Equal(a.vendors, b.vendors) && a.title == b.title
}

// TestReadScan_ReadsTheSeveritySourceAndTheVendors covers the two ways Trivy
// says where a severity came from: SeveritySource when it took a vendor's or
// NVD's rating, and only VendorSeverity, sorted here, when it took the rating
// its database merged from other vendors.
func TestReadScan_ReadsTheSeveritySourceAndTheVendors(t *testing.T) {
	t.Parallel()

	got, err := readScan(writeJSON(t, fixtureReport("amd64", []fixtureVuln{
		{ID: "CVE-2026-1", Pkg: "zlib", Installed: "1", Fixed: "2", Severity: "HIGH", Source: "nvd", Vendors: map[string]int{"nvd": 3}},
		{ID: "CVE-2026-2", Pkg: "zlib", Installed: "1", Severity: "LOW", Vendors: map[string]int{"ubuntu": 1, "debian": 1}},
	}, nil)))
	if err != nil {
		t.Fatalf("readScan: %v", err)
	}
	if len(got.os) != 2 || got.os[0].source != "nvd" || !slices.Equal(got.os[1].vendors, []string{"debian", "ubuntu"}) {
		t.Errorf("os findings = %+v, want nvd as the first's source and the second's vendors sorted", got.os)
	}
}

// TestReadScan_NamesThePlatformAsThePushDoes covers the platform read from the
// image configuration: arm64's default variant v8 is dropped, as containerd
// normalizes it, so the report matches linux/arm64, and any other variant is
// kept, so linux/arm/v7 is not mistaken for linux/arm/v6.
func TestReadScan_NamesThePlatformAsThePushDoes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		arch, variant, want string
	}{
		{arch: "arm64", want: "linux/arm64"},
		{arch: "arm64", variant: "v8", want: "linux/arm64"},
		{arch: "arm64", variant: "v9", want: "linux/arm64/v9"},
		{arch: "arm", variant: "v7", want: "linux/arm/v7"},
		{arch: "amd64", variant: "v8", want: "linux/amd64/v8"},
	} {
		t.Run(tc.want+" from "+tc.arch+" "+tc.variant, func(t *testing.T) {
			t.Parallel()

			report := fixtureReport(tc.arch, nil, nil)
			config := metadata(report)["ImageConfig"].(map[string]any)
			config["variant"] = tc.variant
			got, err := readScan(writeJSON(t, report))
			if err != nil || got.platform != tc.want {
				t.Fatalf("readScan platform = %q, %v; want %q", got.platform, err, tc.want)
			}
		})
	}
}

// TestReadScan_RefusesAReportItCannotVouchFor covers every report the judge
// must not read as clean: one it cannot read, one that is not Trivy's JSON or
// not the schema this command reads, one that names no platform, and the
// three a scan that judged nothing produces, which are no operating system
// detected, a distribution past its end of life, and no OS package listed.
// A severity Trivy does not define is refused rather than ranked.
func TestReadScan_RefusesAReportItCannotVouchFor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		edit func(map[string]any)
		name string
		raw  string
		want string
	}{
		{name: "not JSON", raw: "{", want: "is not a Trivy JSON report"},
		{name: "another schema", edit: func(r map[string]any) { r["SchemaVersion"] = 1 }, want: "has SchemaVersion 1, and this command reads Trivy's schema 2"},
		{name: "no platform", edit: func(r map[string]any) { delete(metadata(r), "ImageConfig") }, want: "names no platform in its image configuration"},
		{
			name: "no architecture", want: "names no platform in its image configuration",
			edit: func(r map[string]any) { metadata(r)["ImageConfig"] = map[string]any{"os": "linux"} },
		},
		{
			name: "no operating system", want: "Trivy detected no operating system in it, so none of its packages was judged",
			edit: func(r map[string]any) { delete(metadata(r), "OS") },
		},
		{
			name: "no family", want: "Trivy detected no operating system in it, so none of its packages was judged",
			edit: func(r map[string]any) { metadata(r)["OS"] = map[string]any{"Name": "3.24.2"} },
		},
		{
			name: "end of life", want: "alpine 3.24.2 is past its end of service life, so its packages get no new advisories and a clean scan of them says nothing",
			edit: func(r map[string]any) { metadata(r)["OS"].(map[string]any)["EOSL"] = true },
		},
		{
			name: "no OS result", want: "lists no operating-system packages for linux/amd64",
			edit: func(r map[string]any) { r["Results"] = r["Results"].([]any)[1:] },
		},
		{
			name: "no OS package", want: "lists no operating-system packages for linux/amd64",
			edit: func(r map[string]any) { osResult(r)["Packages"] = []any{} },
		},
		{
			name: "unknown OS severity", want: `rates CVE-2026-1 in zlib "critical", which is not a severity Trivy defines`,
			edit: func(r map[string]any) {
				osResult(r)["Vulnerabilities"] = []fixtureVuln{{ID: "CVE-2026-1", Pkg: "zlib", Severity: "critical"}}
			},
		},
		{
			name: "unknown binary severity", want: `rates CVE-2026-2 in stdlib "", which is not a severity Trivy defines`,
			edit: func(r map[string]any) {
				r["Results"].([]any)[1].(map[string]any)["Vulnerabilities"] = []fixtureVuln{{ID: "CVE-2026-2", Pkg: "stdlib"}}
			},
		},
		{
			name: "a finding in a class it does not read", want: `reports CVE-2026-3 in a "custom" result, which this command does not read`,
			edit: func(r map[string]any) {
				r["Results"] = append(r["Results"].([]any), map[string]any{
					"Class": "custom", "Vulnerabilities": []fixtureVuln{{ID: "CVE-2026-3", Pkg: "x", Severity: "HIGH"}},
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var path string
			if tc.raw != "" {
				path = writeFile(t, []byte(tc.raw))
			} else {
				report := fixtureReport("amd64", nil, nil)
				tc.edit(report)
				path = writeJSON(t, report)
			}
			_, err := readScan(path)
			if err == nil || !strings.HasPrefix(err.Error(), path+": ") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readScan error = %v, want one naming %s and saying %q", err, path, tc.want)
			}
		})
	}
}

// TestReadScan_AReportThatIsNotThere_IsAnError covers the file itself being
// absent.
func TestReadScan_AReportThatIsNotThere_IsAnError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent.json")
	if _, err := readScan(path); err == nil || !strings.Contains(err.Error(), "absent.json") {
		t.Fatalf("readScan error = %v, want one naming the file", err)
	}
}

// TestCheckPlatforms_EachPlatformExactlyOnce holds the reports of a run to the
// platforms the release pushes: every one scanned, none twice, and nothing
// scanned that the release does not push, since a report of another image is
// no evidence about this one.
func TestCheckPlatforms_EachPlatformExactlyOnce(t *testing.T) {
	t.Parallel()

	amd64 := scan{path: "a.json", platform: "linux/amd64"}
	arm64 := scan{path: "b.json", platform: "linux/arm64"}
	want := []string{"linux/amd64", "linux/arm64"}
	for _, tc := range []struct {
		name    string
		problem string
		scans   []scan
	}{
		{name: "both", scans: []scan{arm64, amd64}},
		{name: "one missing", scans: []scan{amd64}, problem: "no report scans linux/arm64, which the release pushes"},
		{
			name:    "one twice",
			scans:   []scan{amd64, arm64, {path: "c.json", platform: "linux/amd64"}},
			problem: "a.json and c.json both scan linux/amd64",
		},
		{
			name:    "one not pushed",
			scans:   []scan{amd64, arm64, {path: "d.json", platform: "linux/s390x"}},
			problem: "d.json scans linux/s390x, which is not one of linux/amd64, linux/arm64",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkPlatforms(tc.scans, want)
			if tc.problem == "" {
				if err != nil {
					t.Fatalf("checkPlatforms: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.problem {
				t.Fatalf("checkPlatforms error = %v, want %q", err, tc.problem)
			}
		})
	}
}
