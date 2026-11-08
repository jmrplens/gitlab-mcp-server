// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// links_test.go holds how a link is read: a link to the documentation site
// to the page that serves it and the heading its anchor names, under every
// address the site answers at (the cases scripts/check_doc_links_test.py held
// the script to), and a relative link to the file and the heading it names.
//
// Issue 1163 made the documentation site the one home of the user
// documentation, so the README and every file that used to link into docs/
// now link to https://docs.jmrp.io/gitlab-mcp-server/... instead. The
// repository's own corpus only shows that every link it carries resolves; it
// cannot show that a broken one would be reported, which is the half that
// fails silently: a pattern that stops recognizing the site's address, or a
// return before the anchor is judged, leaves the job green with nothing
// checked.
package main

import (
	"maps"
	"strings"
	"testing"
)

// The site's address, and the two older spellings that still redirect to it
// with the path kept: the github.io origin the site was served from before
// its custom domain, and the jmrp.io vanity redirect.
const (
	site   = "https://docs.jmrp.io/gitlab-mcp-server"
	pages  = "https://jmrplens.github.io/gitlab-mcp-server"
	vanity = "https://jmrp.io/docs/gitlab-mcp-server"
)

// siteTree is the stand-in site the cases link into.
var siteTree = map[string]string{
	"site/src/content/docs/index.mdx":             "---\ntitle: Home\n---\n\nWelcome.\n",
	"site/src/content/docs/configuration.mdx":     "---\ntitle: Configuration\n---\n\n## Environment variables\n\nText.\n",
	"site/src/content/docs/install/index.mdx":     "---\ntitle: Install\n---\n\n## Pick a channel\n\nText.\n",
	"site/src/content/docs/install/docker.mdx":    "---\ntitle: Docker\n---\n\n## Configure your client\n\nText.\n",
	"site/src/content/docs/es/configuration.mdx":  "---\ntitle: Configuración\n---\n\n## Variables de entorno\n\nTexto.\n",
	"site/public/robots.txt":                      "User-agent: *\n",
	"site/public/benchmarks/en/latency.light.svg": "<svg/>\n",
	"site/scripts/gen-llms.mjs":                   "const REFERENCE_FILES = [\n\t{ name: \"llms.txt\", publishAs: \"llms-server.txt\" },\n];\n",
	"docs/guide.md":                               "# Guide\n\n## Usage\n",
	"docs/folder/README.md":                       "# Folder\n\n## Inside\n",
	"docs/index-only/index.mdx":                   "# Index\n\n## Here\n",
	"docs/space name.md":                          "# Spaced\n",
	"docs/source.go":                              "package docs\n",
}

// checkLinks runs the command over the stand-in tree and a README carrying
// one link per line, and returns its exit code and both streams together.
func checkLinks(t *testing.T, links ...string) (int, string) {
	t.Helper()
	files := maps.Clone(siteTree)
	var readme strings.Builder
	readme.WriteString("# Readme\n\n")
	for _, link := range links {
		readme.WriteString("- [link](" + link + ")\n")
	}
	files["README.md"] = readme.String()
	code, stdout, stderr := check(layOut(t, files))
	return code, stdout + stderr
}

// assertPasses runs each link alone and wants it to pass.
func assertPasses(t *testing.T, links ...string) {
	t.Helper()
	for _, link := range links {
		t.Run(link, func(t *testing.T) {
			t.Parallel()

			code, output := checkLinks(t, link)
			if code != 0 || !strings.Contains(output, "local links are valid") {
				t.Errorf("%s was refused:\n%s", link, output)
			}
		})
	}
}

// assertRefused runs each link alone and wants it refused with its reason.
func assertRefused(t *testing.T, cases map[string]string) {
	t.Helper()
	for link, reason := range cases {
		t.Run(link, func(t *testing.T) {
			t.Parallel()

			code, output := checkLinks(t, link)
			if code != 1 || !strings.Contains(output, "-> "+link+" ("+reason) {
				t.Errorf("%s passed or was refused for another reason, want %q:\n%s", link, reason, output)
			}
		})
	}
}

// TestCheck_APageAndItsHeading_PassUnderEveryAddress verifies the page and
// its anchor under the three addresses, and a page named with no anchor, with
// a trailing slash and with a query.
func TestCheck_APageAndItsHeading_PassUnderEveryAddress(t *testing.T) {
	t.Parallel()

	assertPasses(t,
		site+"/configuration/#environment-variables",
		pages+"/configuration/#environment-variables",
		vanity+"/configuration/#environment-variables",
		site+"/configuration",
		site+"/configuration/?ref=readme",
	)
}

// TestCheck_AFolderIndexASpanishPageAndTheRoot_Pass verifies the routes that
// name a folder's index, a page under es/, and the site's root and its _top
// anchor.
func TestCheck_AFolderIndexASpanishPageAndTheRoot_Pass(t *testing.T) {
	t.Parallel()

	assertPasses(t,
		site+"/install/#pick-a-channel",
		site+"/install/docker/#configure-your-client",
		site+"/es/configuration/#variables-de-entorno",
		site,
		site+"/",
		site+"#_top",
	)
}

// TestCheck_AMissingPage_IsRefused verifies a route no page serves, under
// each address.
func TestCheck_AMissingPage_IsRefused(t *testing.T) {
	t.Parallel()

	assertRefused(t, map[string]string{
		site + "/nope/":                   "no site page serves /nope",
		site + "/es/install/docker/":      "no site page serves /es/install/docker",
		pages + "/operations/telemetry/":  "no site page serves /operations/telemetry",
		vanity + "/operations/telemetry/": "no site page serves /operations/telemetry",
	})
}

// TestCheck_AMissingHeading_IsRefused verifies an anchor the page does not
// carry, including one its English twin carries.
func TestCheck_AMissingHeading_IsRefused(t *testing.T) {
	t.Parallel()

	assertRefused(t, map[string]string{
		site + "/configuration/#nope":                     "no Starlight anchor #nope in site/src/content/docs/configuration.mdx",
		site + "/install/#configure-your-client":          "no Starlight anchor",
		site + "/es/configuration/#environment-variables": "no Starlight anchor",
	})
}

// TestCheck_TheFilesTheSiteServes_Pass verifies a file under site/public,
// the llms files the build writes and republishes, and the sitemap.
func TestCheck_TheFilesTheSiteServes_Pass(t *testing.T) {
	t.Parallel()

	assertPasses(t,
		site+"/robots.txt",
		site+"/benchmarks/en/latency.light.svg",
		site+"/llms.txt",
		site+"/es/llms.txt",
		site+"/llms-server.txt",
		pages+"/sitemap-index.xml",
		site+"/sitemap-0.xml",
	)
}

// TestCheck_AFileTheSiteDoesNotServe_IsRefused verifies a file nothing
// writes, one republished under one language only, a directory of
// site/public, and a route climbing out of it.
func TestCheck_AFileTheSiteDoesNotServe_IsRefused(t *testing.T) {
	t.Parallel()

	assertRefused(t, map[string]string{
		site + "/llms-nope.txt":             "no file the site serves /llms-nope.txt",
		site + "/benchmarks/en/missing.svg": "no file the site serves /benchmarks/en/missing.svg",
		pages + "/es/llms-server.txt":       "no file the site serves /es/llms-server.txt",
		site + "/../README.md":              "no file the site serves /../README.md",
		site + "/benchmarks.en":             "no file the site serves /benchmarks.en",
	})
}

// TestCheck_AnotherHost_IsSkipped verifies addresses that only look like the
// site's: another path on jmrp.io, a project whose name starts with the
// site's, and the site's path on another host.
func TestCheck_AnotherHost_IsSkipped(t *testing.T) {
	t.Parallel()

	assertPasses(t,
		"https://jmrp.io/projects/",
		"https://docs.jmrp.io/gitlab-mcp-server-other/nope/",
		"https://example.com/docs/gitlab-mcp-server/nope/",
	)
}

// TestCheck_EveryBrokenLink_IsReportedAtOnce verifies that one finding does
// not hide another.
func TestCheck_EveryBrokenLink_IsReportedAtOnce(t *testing.T) {
	t.Parallel()

	code, output := checkLinks(t, site+"/nope/", site+"/configuration/#nope", site+"/llms-nope.txt")
	for _, want := range []string{"no site page serves /nope", "no Starlight anchor #nope", "no file the site serves /llms-nope.txt"} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

// TestCheck_ARelativeLink_IsHeldToItsFileAndHeading verifies the candidates
// a relative link resolves through (the path, with .md or .mdx, and a
// folder's README or index), an anchor judged in GitHub's dialect, a query
// dropped, an escaped path decoded, and an anchor on a file that is not
// Markdown left alone.
func TestCheck_ARelativeLink_IsHeldToItsFileAndHeading(t *testing.T) {
	t.Parallel()

	assertPasses(t,
		"docs/guide.md#usage",
		"docs/guide#usage",
		"docs/guide.md?plain=1#usage",
		"docs/folder#inside",
		"docs/folder/#inside",
		"docs/index-only#here",
		"docs/space%20name.md",
		"<docs/space name.md>",
		"docs/source.go#L1",
		// A folder with no page inside answers the fragment with nothing a
		// reader is shown as Markdown, so the fragment is not judged.
		"site/public/benchmarks#anything",
		"#readme",
	)
	assertRefused(t, map[string]string{
		"docs/missing.md":      "no such file",
		"docs/guide.md#nope":   "no GitHub anchor #nope in docs/guide.md",
		"docs/folder#nope":     "no GitHub anchor #nope in docs/folder/README.md",
		"#nope":                "no GitHub anchor #nope in README.md",
		"docs/space%zzname.md": "no such file",
	})
}

// TestCheck_WhatNamesNoFileHere_IsSkipped verifies the targets the check
// cannot answer for: a scheme, a root-relative route, a placeholder word and
// a template.
func TestCheck_WhatNamesNoFileHere_IsSkipped(t *testing.T) {
	t.Parallel()

	assertPasses(t,
		"mailto:someone@example.com",
		"https://example.com/missing.md",
		"/route/on/the/site",
		"url",
		"PATH",
		"<{template}>",
		"{placeholder}",
	)
}

// TestNormalizeTarget_DropsSpacesAndOneBracketEachSide verifies the trimming
// a destination gets before it is read.
func TestNormalizeTarget_DropsSpacesAndOneBracketEachSide(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]string{" a.md ": "a.md", "<a b.md>": "a b.md", "<<a>>": "<a>", "a.md>": "a.md"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			if got := normalizeTarget(raw); got != want {
				t.Errorf("normalizeTarget(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}
