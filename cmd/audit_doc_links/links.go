// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// siteAddress matches the documentation site's own address,
// docs.jmrp.io/gitlab-mcp-server, the custom domain GitHub Pages serves it at
// and the address every page here links to, and the two older spellings that
// still redirect to it, path preserved: the github.io origin it was served
// from before and the jmrp.io vanity redirect. A link to any of them is a link
// into site/src/content/docs, which this command can read, so it is held to
// its page and its anchor like a relative link rather than skipped as
// somebody else's server. Issue 1163 made the site the one home of the user
// documentation; without this, every link the README and the files that used
// to point into docs/ carry would go unchecked. [siteRest] holds the
// character after the address to one that ends it.
var siteAddress = regexp.MustCompile(`(?i)^https?://(?:docs\.jmrp\.io|jmrp\.io/docs|jmrplens\.github\.io)/gitlab-mcp-server`)

// scheme matches a target that names a scheme or is root-relative.
var scheme = regexp.MustCompile(`(?i)^(?:[a-z][a-z0-9+.-]*:|/)`)

// placeholder matches a target that is a word standing for one in prose.
var placeholder = regexp.MustCompile(`(?i)^(?:url|link|path|file|filename|directory|fn)$`)

// templated matches a target written as a template, in angle brackets or
// braces.
var templated = regexp.MustCompile(`^[<{].*[>}]$`)

// fileRoute matches a route on the site that ends in an extension.
var fileRoute = regexp.MustCompile(`(?i)\.[a-z0-9]+$`)

// sitemap matches the sitemap files the site's build writes.
var sitemap = regexp.MustCompile(`^sitemap(?:-index|-\d+)?\.xml$`)

// publishAs matches the name under which site/scripts/gen-llms.mjs
// republishes a file into site/public.
var publishAs = regexp.MustCompile(`\bpublishAs:\s*"([^"]+)"`)

// markdownFile matches a path whose headings answer a fragment.
var markdownFile = regexp.MustCompile(`(?i)\.mdx?$`)

// The names of the page inside a folder that a link to the folder lands on:
// the README GitHub draws below the listing, and the index Starlight routes
// the folder to, in either of the two extensions a page here is written in.
const (
	readmePage   = "README.md"
	indexMDPage  = "index.md"
	indexMDXPage = "index.mdx"
)

// issue is one broken link: the file and line it is on, the target as
// written, and why it does not resolve.
type issue struct {
	file   string
	line   int
	target string
	reason string
}

// checker holds the links of one repository to what they name.
type checker struct {
	root string
	// generated are the files the site serves that no page answers for and
	// site/public does not hold.
	generated map[string]bool
	issues    []issue
	anchors   map[string]map[string]bool
	checked   int
}

// newChecker prepares a check of the repository at root.
func newChecker(root string) *checker {
	return &checker{root: root, generated: readSiteGeneratedFiles(root), anchors: map[string]map[string]bool{}}
}

// siteContentRoot, siteDocsRoot and sitePublicRoot are where the site's
// pages, its documentation collection and the files it serves as they are
// live, below the repository.
func (c *checker) siteContentRoot() string { return filepath.Join(c.root, "site", "src", "content") }

func (c *checker) siteDocsRoot() string {
	return filepath.Join(c.root, "site", "src", "content", "docs")
}
func (c *checker) sitePublicRoot() string { return filepath.Join(c.root, "site", "public") }

// checkFile holds every link of one file to what it names. file is
// repository-relative, with slashes.
func (c *checker) checkFile(file, content string) {
	eachProseLine(content, func(line string, number int) {
		for _, target := range inlineLinks(line) {
			c.checkTarget(file, number, target)
		}
		if target, ok := referenceTarget(line); ok {
			c.checkTarget(file, number, target)
		}
	})
}

// checkTarget holds one link to the file it names and the anchor it names
// there.
func (c *checker) checkTarget(file string, line int, raw string) {
	target := normalizeTarget(raw)
	if rest, ok := siteRest(target); ok {
		c.checkSiteTarget(file, line, target, rest)
		return
	}
	if shouldSkipTarget(target) {
		return
	}

	withoutFragment, fragment, _ := strings.Cut(target, "#")
	withoutFragment, _, _ = strings.Cut(withoutFragment, "?")
	page := filepath.Join(c.root, filepath.FromSlash(file))

	// A bare "#anchor" points into the file the link is written in.
	if withoutFragment == "" {
		c.checkFragment(file, line, target, page, fragment)
		return
	}

	base := filepath.Join(filepath.Dir(page), filepath.FromSlash(safeDecode(withoutFragment)))
	for _, candidate := range []string{
		base, base + ".md", base + ".mdx",
		filepath.Join(base, readmePage), filepath.Join(base, indexMDPage), filepath.Join(base, indexMDXPage),
	} {
		if exists(candidate) {
			c.checkFragment(file, line, target, pageOf(candidate), fragment)
			return
		}
	}
	c.issues = append(c.issues, issue{file: file, line: line, target: target, reason: "no such file"})
}

// siteRest returns what follows the site's address in target, and whether
// target is a link to the site at all: the address must end where the path,
// the fragment or the query starts, or at the end, so a link to
// gitlab-mcp-server-other is somebody else's.
func siteRest(target string) (string, bool) {
	match := siteAddress.FindString(target)
	if match == "" {
		return "", false
	}
	rest := target[len(match):]
	if rest != "" && !strings.ContainsAny(rest[:1], "/#?") {
		return "", false
	}
	return rest, true
}

// checkSiteTarget holds a link to the documentation site to the page that
// serves it. Starlight routes a page by its path under the docs collection, a
// folder by the index inside it, and the Spanish pages by the same path under
// es/, so the route alone names the file. A route ending in an extension is a
// file the site serves as it is (llms.txt, a chart, the sitemap), which has no
// headings to hold it to and is held to being a file the site serves.
func (c *checker) checkSiteTarget(file string, line int, target, rest string) {
	path, fragment, _ := strings.Cut(rest, "#")
	path, _, _ = strings.Cut(path, "?")
	route := strings.Trim(safeDecode(path), "/")
	if fileRoute.MatchString(route) {
		if !c.siteServesFile(route) {
			c.issues = append(c.issues, issue{file: file, line: line, target: target, reason: "no file the site serves /" + route})
		}
		return
	}
	name := route
	if name == "" {
		name = "index"
	}
	base := filepath.Join(c.siteDocsRoot(), filepath.FromSlash(name))
	for _, candidate := range []string{base + ".mdx", base + ".md", filepath.Join(base, indexMDXPage), filepath.Join(base, indexMDPage)} {
		if exists(candidate) {
			c.checkFragment(file, line, target, candidate, fragment)
			return
		}
	}
	c.issues = append(c.issues, issue{file: file, line: line, target: target, reason: "no site page serves /" + route})
}

// siteServesFile answers whether a route ending in an extension is a file the
// site serves: one under site/public, one the build generates, or the
// sitemap. A route that climbs out of site/public serves nothing.
func (c *checker) siteServesFile(route string) bool {
	if c.generated[route] || sitemap.MatchString(route) {
		return true
	}
	public := c.sitePublicRoot()
	candidate := filepath.Join(public, filepath.FromSlash(route))
	if !strings.HasPrefix(candidate, public+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(candidate)
	return err == nil && info.Mode().IsRegular()
}

// readSiteGeneratedFiles names the files site/scripts/gen-llms.mjs writes into
// site/public: the English and Spanish indexes it builds, and each file it
// republishes under its publishAs name. The names are read from the script
// that publishes them, so a file it stops publishing stops being a valid link
// target here in the same change. A tree without the script publishes only
// the two indexes, so a link to an llms file fails there rather than passing
// on a name nothing writes.
func readSiteGeneratedFiles(root string) map[string]bool {
	generated := map[string]bool{"llms.txt": true, "es/llms.txt": true}
	data, err := os.ReadFile(filepath.Join(root, "site", "scripts", "gen-llms.mjs")) //#nosec G304 -- the site's own generator, below the repository the check runs in
	if err != nil {
		return generated
	}
	for _, match := range publishAs.FindAllStringSubmatch(string(data), -1) {
		generated[match[1]] = true
	}
	return generated
}

// pageOf names the file whose headings answer a fragment. A link can resolve
// to a directory, since that is the first candidate and a directory exists,
// and both renderers answer one with the page inside it: GitHub draws the
// README below the listing, Starlight routes to the index. Without this the
// fragment on such a link was dropped for landing on something that is not
// Markdown.
//
// The page is looked for inside the directory rather than by letting the
// candidate list run on, because the next candidates are "<dir>.md" and
// "<dir>.mdx", siblings of the directory rather than anything a reader of
// that link would be shown. A file holds no README or index, so a file is
// its own page.
func pageOf(resolved string) string {
	for _, name := range []string{readmePage, indexMDPage, indexMDXPage} {
		if candidate := filepath.Join(resolved, name); exists(candidate) {
			return candidate
		}
	}
	return resolved
}

// checkFragment holds the other half of a link: the anchor. A link to a
// heading that was renamed, or that never existed, resolved its file and
// passed before the anchor was judged.
//
// Only a Markdown target is judged. A fragment on anything else is a line
// reference into source a renderer numbers for itself, and nothing here can
// read it.
func (c *checker) checkFragment(file string, line int, target, page, fragment string) {
	if fragment == "" || !markdownFile.MatchString(page) {
		return
	}
	anchor := safeDecode(fragment)
	c.checked++
	if c.anchorsOf(page)[anchor] {
		return
	}
	// Both paths are the root joined with a relative one, so Rel always has
	// an answer.
	rel, _ := filepath.Rel(c.root, page)
	c.issues = append(c.issues, issue{
		file: file, line: line, target: target,
		reason: "no " + c.dialectOf(page).name + " anchor #" + anchor + " in " + filepath.ToSlash(rel),
	})
}

// dialectOf is the renderer that names the headings of page.
func (c *checker) dialectOf(page string) dialect {
	if strings.HasPrefix(page, c.siteContentRoot()+string(filepath.Separator)) {
		return starlight
	}
	return gitHub
}

// anchorsOf returns the anchors of page, read once. A page that cannot be
// read answers to none, which reports every anchor named in it as missing.
func (c *checker) anchorsOf(page string) map[string]bool {
	anchors, read := c.anchors[page]
	if !read {
		data, _ := os.ReadFile(page) //#nosec G304 -- a page a link of the repository's documentation names
		anchors = collectAnchors(string(data), c.dialectOf(page))
		c.anchors[page] = anchors
	}
	return anchors
}

// normalizeTarget trims a target and the angle brackets that may wrap it.
func normalizeTarget(raw string) string {
	target := strings.TrimSpace(raw)
	target = strings.TrimPrefix(target, "<")
	return strings.TrimSuffix(target, ">")
}

// shouldSkipTarget names what this command cannot answer for. A scheme is
// somebody else's server, the documentation site's own address aside, which
// [checker.checkTarget] hands to [checker.checkSiteTarget] first; a
// root-relative path is a route on the documentation site rather than a file
// here, and the site validates its own routes and their anchors at build time
// through starlight-links-validator. A word or a template standing for a
// target in prose names nothing.
//
// A "#anchor" is deliberately absent from this list: it is the one shape that
// is always answerable, since the file it points into is the file it is
// written in.
func shouldSkipTarget(target string) bool {
	return target == "" || scheme.MatchString(target) || placeholder.MatchString(target) || templated.MatchString(target)
}

// safeDecode decodes the percent escapes of a path or a fragment, and leaves
// one that does not decode as it is written.
func safeDecode(value string) string {
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}

// exists reports whether path names something on disk.
func exists(path string) bool {
	_, err := os.Stat(path) //#nosec G703 -- a candidate for a path a link of the repository's documentation names, only stated
	return err == nil
}
