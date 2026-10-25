package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

// trivySchema is the version of Trivy's JSON report this command reads.
const trivySchema = 2

// The classes of result a report carries findings in: the image's
// operating-system packages, which this command judges, and the packages of
// the language ecosystems it finds, the server binary's Go modules among them,
// which it prints.
const (
	classOSPackages   = "os-pkgs"
	classLangPackages = "lang-pkgs"
)

// severities are the ratings Trivy assigns, by name.
var severities = []string{"UNKNOWN", "LOW", "MEDIUM", "HIGH", "CRITICAL"}

// trivyReport is the part of `trivy image --format json` this command reads.
type trivyReport struct {
	Trivy struct {
		Version string
	}
	ArtifactName string
	Metadata     struct {
		OS          *trivyOS
		ImageConfig trivyImageConfig
		ImageID     string
	}
	Results []struct {
		Class           string
		Packages        []json.RawMessage
		Vulnerabilities []trivyVulnerability
	}
	SchemaVersion int
}

// trivyOS is the operating system Trivy detected in the image, which a report
// leaves out when it detected none.
type trivyOS struct {
	Family string
	Name   string
	EOSL   bool
}

// trivyImageConfig is the platform the image's configuration names.
type trivyImageConfig struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant"`
}

// trivyVulnerability is one entry of a result's Vulnerabilities.
type trivyVulnerability struct {
	VendorSeverity   map[string]int
	VulnerabilityID  string
	PkgName          string
	InstalledVersion string
	FixedVersion     string
	Severity         string
	SeveritySource   string
	Title            string
}

// finding is one advisory against one package of one image, with the rating
// Trivy gave it and where that rating came from: source when Trivy names one,
// and otherwise the vendors whose ratings its database merged, sorted.
type finding struct {
	vendors                                            []string
	id, pkg, installed, fixed, severity, source, title string
}

// scan is what one report says about one platform of the image: which image
// it is, by the digest of its configuration, the findings against its
// operating-system packages and those against the binaries it carries.
type scan struct {
	os, binaries                                          []finding
	path, platform, artifact, image, trivyVersion, distro string
	packages                                              int
}

// readScan reads one Trivy report, refusing one that cannot vouch for the
// image it names.
func readScan(path string) (scan, error) {
	body, err := os.ReadFile(path) // #nosec G304 -- the reports are the files the caller names on the command line.
	if err != nil {
		return scan{}, err
	}
	var report trivyReport
	if parseErr := json.Unmarshal(body, &report); parseErr != nil {
		return scan{}, fmt.Errorf("%s: is not a Trivy JSON report: %w", path, parseErr)
	}
	if report.SchemaVersion != trivySchema {
		return scan{}, fmt.Errorf("%s: has SchemaVersion %d, and this command reads Trivy's schema %d", path, report.SchemaVersion, trivySchema)
	}
	platform := reportPlatform(report)
	if platform == "" {
		return scan{}, fmt.Errorf("%s: names no platform in its image configuration", path)
	}
	detected := report.Metadata.OS
	if detected == nil || detected.Family == "" {
		return scan{}, fmt.Errorf("%s: Trivy detected no operating system in it, so none of its packages was judged", path)
	}
	distro := detected.Family + " " + detected.Name
	if detected.EOSL {
		return scan{}, fmt.Errorf("%s: %s is past its end of service life, so its packages get no new advisories and a clean scan of them says nothing", path, distro)
	}
	s := scan{
		path: path, platform: platform, artifact: report.ArtifactName, image: report.Metadata.ImageID,
		trivyVersion: report.Trivy.Version, distro: distro,
	}
	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			if !slices.Contains(severities, v.Severity) {
				return scan{}, fmt.Errorf("%s: rates %s in %s %q, which is not a severity Trivy defines", path, v.VulnerabilityID, v.PkgName, v.Severity)
			}
			f := finding{
				id: v.VulnerabilityID, pkg: v.PkgName, installed: v.InstalledVersion, fixed: v.FixedVersion,
				severity: v.Severity, source: v.SeveritySource, vendors: slices.Sorted(maps.Keys(v.VendorSeverity)), title: v.Title,
			}
			switch result.Class {
			case classOSPackages:
				s.os = append(s.os, f)
			case classLangPackages:
				s.binaries = append(s.binaries, f)
			default:
				return scan{}, fmt.Errorf("%s: reports %s in a %q result, which this command does not read", path, v.VulnerabilityID, result.Class)
			}
		}
		if result.Class == classOSPackages {
			s.packages += len(result.Packages)
		}
	}
	if s.packages == 0 {
		return scan{}, fmt.Errorf("%s: lists no operating-system packages for %s: Trivy read none, or ran without --list-all-pkgs", path, platform)
	}
	return s, nil
}

// reportPlatform is the platform a report's image configuration names, as
// os/architecture with the variant after it, but for arm64's default v8,
// which containerd drops and the push names no variant for. It is empty when
// the configuration names no operating system or no architecture.
func reportPlatform(report trivyReport) string {
	config := report.Metadata.ImageConfig
	if config.OS == "" || config.Architecture == "" {
		return ""
	}
	platform := config.OS + "/" + config.Architecture
	if config.Variant != "" && (config.Architecture != "arm64" || config.Variant != "v8") {
		platform += "/" + config.Variant
	}
	return platform
}

// checkPlatforms holds the reports of a run to the platforms the release
// pushes: each scanned once, none missing, and nothing scanned that is not
// pushed, since a report of another image is no evidence about this one.
func checkPlatforms(scans []scan, want []string) error {
	seen := map[string]string{}
	for _, s := range scans {
		if !slices.Contains(want, s.platform) {
			return fmt.Errorf("%s scans %s, which is not one of %s", s.path, s.platform, strings.Join(want, ", "))
		}
		if earlier, twice := seen[s.platform]; twice {
			return fmt.Errorf("%s and %s both scan %s", earlier, s.path, s.platform)
		}
		seen[s.platform] = s.path
	}
	for _, platform := range want {
		if seen[platform] == "" {
			return fmt.Errorf("no report scans %s, which the release pushes", platform)
		}
	}
	return nil
}
