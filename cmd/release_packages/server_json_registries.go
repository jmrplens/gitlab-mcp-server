// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// The hosts the npm, PyPI, NuGet and GitHub branches ask.
const (
	npmRegistry   = "https://registry.npmjs.org/"
	pypiAPI       = "https://pypi.org/pypi/"
	nugetIndexURL = "https://api.nuget.org/v3/index.json"
	nugetFlatURL  = "https://api.nuget.org/v3-flatcontainer/"
	githubAPI     = "https://api.github.com/repos/"
	// defaultRepository is the repository whose release is read when
	// GITHUB_REPOSITORY does not name one.
	defaultRepository = "jmrplens/gitlab-mcp-server"
)

// npmLauncher is the committed npm launcher manifest, which release.yml
// publishes before mcp-publisher runs, so it is the one live when the
// registry checks the npm entry.
const npmLauncher = "npm/gitlab-mcp-server/package.json"

// nugetEntries are what a .NET tool package with an MCP manifest carries,
// beside its nuspec.
var nugetEntries = []string{"tools/net10.0/any/DotnetToolSettings.xml", ".mcp/server.json"}

// version is a package's version as `jq -r '.version // "null"'` printed it,
// with "" for the version that is not there.
func version(pkg *jsonValue) string {
	if v := text(pkg, "version"); v != "null" {
		return v
	}
	return ""
}

// ownershipToken returns the expression that finds the "mcp-name: <server>"
// token the registry reads a PyPI or NuGet README for, on a line of its own
// or after whitespace, and followed by whitespace, the end of the line or,
// when the README is HTML or Markdown, by what may close it there.
func ownershipToken(name string, closers ...string) *regexp.Regexp {
	after := append([]string{`[[:space:]]`, `$`}, closers...)
	return regexp.MustCompile(`(?m)(^|[[:space:]])mcp-name: ` + regexp.QuoteMeta(name) + `(` + strings.Join(after, "|") + `)`)
}

// checkNPM checks every npm package. The registry validates npm ownership
// server-side: it fetches the published version's package.json and requires
// its mcpName to equal the server name; it also requires a version and
// rejects fileSha256. release.yml publishes npm before mcp-publisher runs, so
// the committed launcher manifest is the one live at registry-publish time,
// and the lockstep below is the real invariant.
func (c *serverJSONCheck) checkNPM() {
	for _, pkg := range c.declared("npm") {
		identifier, v := text(pkg, "identifier"), version(pkg)
		if identifier == "" {
			continue
		}
		c.checked++
		c.sayf("npm: %s@%s", identifier, orNull(v))
		if len(bannedFields(pkg, "fileSha256")) > 0 {
			c.failf("carries field(s) the registry rejects for npm packages: fileSha256")
		}
		if v == "" {
			c.failf("declares no version (the registry requires one for npm entries)")
			continue
		}
		c.checkNPMPackage(identifier, v)
	}
}

// orNull is a version as the messages print one that is not there.
func orNull(v string) string {
	if v == "" {
		return "null"
	}
	return v
}

// checkNPMPackage holds one versioned npm package to the committed launcher
// and to what the registry published.
func (c *serverJSONCheck) checkNPMPackage(identifier, v string) {
	data, err := os.ReadFile(c.local(npmLauncher)) //#nosec G304,G703 -- the committed npm launcher manifest, read and never written
	var launcher *jsonValue
	if err == nil {
		launcher, err = parseJSON(data)
	}
	if err != nil {
		c.failf("%s not found", npmLauncher)
		return
	}
	if name := text(launcher, "name"); identifier != name {
		c.failf("identifier is \"%s\" but the committed launcher is \"%s\"", identifier, name)
		return
	}
	if mcpName := textOr(launcher, "mcpName"); mcpName != c.name {
		c.failf("launcher mcpName is \"%s\", expected \"%s\" (the registry's npm ownership check fails without it)", mcpName, c.name)
		return
	}
	if launcherVersion := text(launcher, "version"); v != launcherVersion {
		c.failf("declared version %s does not match the committed launcher version %s", v, launcherVersion)
		return
	}
	answer, err := c.fetch.get(npmRegistry+strings.ReplaceAll(identifier, "/", "%2F")+"/"+v, true, nil)
	if err != nil {
		c.failf("version %s not found on registry.npmjs.org", v)
		return
	}
	switch published := textOr(parsedOrNull(answer.body), "mcpName"); published {
	case "":
		c.sayf("  NOTE: published %s predates mcpName; the registry accepts this entry starting with the release that publishes it", v)
	case c.name:
		c.sayf("  OK: name, mcpName and version in lockstep; published mcpName matches")
	default:
		c.failf("published mcpName is \"%s\", expected \"%s\"", published, c.name)
	}
}

// checkPyPI checks every pypi package. The registry validates ownership
// through an "mcp-name: <server-name>" token in the published version's
// README (its long_description), fetched from the PyPI JSON API, and
// rejects fileSha256 like the other types that are not mcpb.
func (c *serverJSONCheck) checkPyPI() {
	token := ownershipToken(c.name)
	for _, pkg := range c.declared("pypi") {
		identifier, v := text(pkg, "identifier"), version(pkg)
		c.checked++
		c.sayf("pypi package: %s@%s", identifier, orNull(v))
		if len(bannedFields(pkg, "fileSha256")) > 0 {
			c.failf("carries field(s) the registry rejects for pypi packages: fileSha256")
		}
		if v == "" {
			c.failf("declares no version (the registry requires one for pypi entries)")
			continue
		}
		answer, err := c.fetch.get(pypiAPI+identifier+"/"+v+"/json", true, nil)
		if err != nil {
			c.failf("version %s not found on pypi.org", v)
			continue
		}
		if description := textOr(orEmpty(parsedOrNull(answer.body).field("info")), "description"); !token.MatchString(description) {
			c.failf("published README lacks the \"mcp-name: %s\" ownership token the registry validates", c.name)
			continue
		}
		c.sayf("  OK: published version carries the mcp-name ownership token")
	}
}

// checkNuGet checks every nuget package. The registry validates ownership
// through an "mcp-name: <server-name>" token in the published version's
// README, read from the ReadmeUriTemplate resource of the v3 feed (the flat
// container's /readme endpoint), requires registryBaseUrl to be exactly the
// nuget.org v3 index, and rejects fileSha256. A version the feed does not
// list yet is a note rather than a failure, the way the npm branch treats a
// published version that predates mcpName: the entry is correct from the
// release that publishes it, and this check runs on main before that release
// exists.
func (c *serverJSONCheck) checkNuGet() {
	token := ownershipToken(c.name, "<", "-->")
	for _, pkg := range c.declared("nuget") {
		identifier, v := text(pkg, "identifier"), version(pkg)
		if identifier == "" {
			continue
		}
		c.checked++
		c.sayf("nuget package: %s@%s", identifier, orNull(v))
		if len(bannedFields(pkg, "fileSha256")) > 0 {
			c.failf("carries field(s) the registry rejects for nuget packages: fileSha256")
		}
		if base := textOr(pkg, "registryBaseUrl"); base != nugetIndexURL {
			c.failf("registryBaseUrl is \"%s\"; the registry accepts only %s for nuget entries", base, nugetIndexURL)
			continue
		}
		if v == "" {
			c.failf("declares no version (the registry requires one for nuget entries)")
			continue
		}
		c.checkNuGetPackage(identifier, v, token)
	}
}

// checkNuGetPackage holds one versioned NuGet package to what the feed
// lists, its README and the package it serves.
func (c *serverJSONCheck) checkNuGetPackage(identifier, v string, token *regexp.Regexp) {
	lowerID, lowerVersion := asciiLower(identifier), asciiLower(v)
	base := nugetFlatURL + lowerID + "/"
	// A 404 here is the expected state before the first release that
	// publishes the package, so the failure is not described.
	quiet := *c.fetch
	quiet.diagnostics = discard{}
	answer, err := quiet.get(base+"index.json", true, nil)
	if err != nil {
		c.sayf("  NOTE: %s is not on nuget.org yet; the registry accepts this entry starting with the release that publishes it", identifier)
		return
	}
	if !nugetLists(answer.body, lowerVersion) {
		c.sayf("  NOTE: nuget.org lists %s but not %s yet; the registry accepts this entry starting with the release that publishes it", identifier, v)
		return
	}
	readme, err := c.fetch.get(base+lowerVersion+"/readme", true, nil)
	if err != nil {
		c.failf("published version %s carries no README (the registry's ownership check reads it)", v)
		return
	}
	if !token.Match(readme.body) {
		c.failf("published README lacks the \"mcp-name: %s\" ownership token the registry validates", c.name)
		return
	}
	nupkg, err := c.fetch.download(base + lowerVersion + "/" + lowerID + "." + lowerVersion + ".nupkg")
	if err != nil {
		c.failf("version %s is listed but its package could not be downloaded", v)
		return
	}
	names, err := readZipNames(nupkg)
	if err != nil {
		fmt.Fprintln(c.stderr, "not a zip archive")
		c.failf("not a valid .NET tool package with an MCP manifest")
		return
	}
	for _, needed := range append([]string{identifier + ".nuspec"}, nugetEntries...) {
		if !slices.Contains(names, needed) {
			fmt.Fprintf(c.stderr, "package carries no %s\n", needed)
			c.failf("not a valid .NET tool package with an MCP manifest")
			return
		}
	}
	c.sayf("  OK: published version carries the mcp-name ownership token, the tool manifest and .mcp/server.json")
}

// nugetLists reports whether a flat-container index lists a version, in any
// case.
func nugetLists(body []byte, lowerVersion string) bool {
	return slices.ContainsFunc(parsedOrNull(body).field("versions").items, func(item *jsonValue) bool {
		return item.kind == jsonString && asciiLower(item.text) == lowerVersion
	})
}

// asciiLower lowers the ASCII letters of s and leaves every other byte, as
// tr '[:upper:]' '[:lower:]' does in the C locale.
func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
}

// discard is a writer that keeps nothing.
type discard struct{}

// Write keeps nothing and reports every byte written.
func (discard) Write(p []byte) (int, error) { return len(p), nil }

// checkRelease reads whether the GitHub release of the declared npm version
// is immutable. Every other artifact this check reads is pinned by a hash or
// an immutable registry version. The GitHub release is not: unless immutable
// releases are enabled on the repository, anyone holding contents: write can
// replace a published binary and its checksums.txt together, and both
// installers accept the pair. This is a drift alarm, not a gate: the setting
// lives in the repository, releases published before it was turned on stay
// mutable forever, and the installer-side signature check is the part that
// travels.
func (c *serverJSONCheck) checkRelease() {
	repository := c.getenv("GITHUB_REPOSITORY")
	if repository == "" {
		repository = defaultRepository
	}
	npm := c.declared("npm")
	if len(npm) == 0 {
		return
	}
	v := version(npm[0])
	if v == "" {
		return
	}
	c.sayf("release: v%s", v)
	headers := map[string]string{"Accept": "application/vnd.github+json"}
	if token := c.getenv("GH_TOKEN"); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	answer, err := c.fetch.get(githubAPI+repository+"/releases/tags/v"+v, true, headers)
	if err != nil || len(answer.body) == 0 {
		c.warnf("could not read the v%s release metadata; immutability not checked", v)
		return
	}
	if !parsedOrNull(answer.body).field("immutable").equal(&jsonValue{kind: jsonBool, boolean: true}) {
		c.warnf("release v%s is mutable: its assets and checksums.txt can be replaced together. Enable immutable releases in the repository settings; the workflow already publishes drafts, which the feature requires.", v)
		return
	}
	c.sayf("  OK: release v%s is immutable", v)
}
