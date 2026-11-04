// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apidocs

import (
	"cmp"
	"fmt"
	"net/url"
	"regexp"
)

// refPattern is what a ref may be spelled as here: one path segment of
// letters, digits, dots, dashes and underscores, beginning with a letter or a
// digit, which every release tag (v19.5.0-ee) and master are. The ref names a
// cache directory as well as a URL, and a slash, or a leading dot, would put
// that directory somewhere other than beside the shared cache.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// NewAtRef returns a Fetcher that reads doc/api as it stood at ref of the
// canonical repository, rather than at master: a release tag such as
// v19.5.0-ee, which is how a comparison is held to the GitLab a recording was
// taken from instead of to whatever the documentation says this week.
//
// Its pages are read through the repository files API (opts.FilesURL,
// [DefaultFilesURL] when empty) rather than the raw web route, for the reason
// DefaultFilesURL gives, and its listing names the ref. Both are cached beside
// the shared cache, in a directory named after the ref
// (.cache/gitlab-api-docs@v19.5.0-ee), so a page read at one release never
// answers for another. AreasURL and CacheDir set in opts win, which is how a
// test points it at a fixture. A ref that is not one path segment is refused,
// for the reason [refPattern] gives.
func NewAtRef(repoRoot, ref string, opts Options) (*Fetcher, error) {
	if !refPattern.MatchString(ref) {
		return nil, fmt.Errorf("apidocs: %q is not a ref this can read doc/api at: a tag or branch of one path segment, such as v19.5.0-ee", ref)
	}
	if opts.AreasURL == "" {
		opts.AreasURL = DefaultAreasURL + "&" + url.Values{"ref": []string{ref}}.Encode()
	}
	if opts.CacheDir == "" {
		opts.CacheDir = CacheDir(repoRoot) + "@" + ref
	}
	f := New(repoRoot, opts)
	f.filesURL = cmp.Or(opts.FilesURL, DefaultFilesURL)
	f.ref = ref
	return f, nil
}

// pageURL is where one area's markdown is read: through the repository files
// API at the fetcher's ref when it has one, and under the doc root otherwise.
// The files API takes the file's path as one escaped segment, so
// doc/api/packages/npm.md is sent as doc%2Fapi%2Fpackages%2Fnpm.md.
func (f *Fetcher) pageURL(area string) string {
	if f.filesURL != "" {
		return f.filesURL + url.PathEscape(areasPrefix+area+".md") + "/raw?" + url.Values{"ref": []string{f.ref}}.Encode()
	}
	return f.baseURL + area + ".md"
}
