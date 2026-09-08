package apidocs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

const (
	// DefaultAreasURL lists the files under doc/api in the canonical GitLab
	// repository, which is the only complete answer to "what does GitLab
	// document".
	//
	// The obvious source is doc/api/api_resources.md, the index the docs
	// themselves publish, and it is not complete: 101 of the 253 pages under
	// doc/api are not linked from it or from doc/api/rest/_index.md, and
	// following the links out of every page those two do list still leaves 77
	// unreached, among them the pages documenting alert management, the
	// dependency proxy, attestations and group integrations. An endpoint
	// checked against a corpus with those holes in it is reported as
	// undocumented because of where the index stops, which is the false failure
	// a path audit cannot afford.
	DefaultAreasURL = "https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab/repository/tree?path=doc/api&recursive=true"

	// areasPerPage is how many tree entries one page carries. It is the
	// endpoint's maximum, so the whole listing costs three requests.
	areasPerPage = 100

	// areasPrefix is the directory the listing is rooted at, trimmed off each
	// entry so what comes back is an area name Fetch accepts.
	areasPrefix = "doc/api/"

	// areasCacheFile holds the listing between runs, beside the pages
	// themselves. The leading underscore keeps it out of the way of an area
	// name, which is always a path under doc/api and never begins with one.
	areasCacheFile = "_areas.json"
)

// treeEntry is the part of one repository tree entry this reads.
type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// Areas lists every documentation area under doc/api, as the names Fetch
// takes: a path below doc/api with the .md suffix removed, so "branches",
// "packages/npm" and "admin/token" are all areas.
//
// The listing is cached like a page and obeys the same rules: Offline serves
// any age and never asks the network, Refresh ignores a fresh copy, and a copy
// older than MaxAge is re-fetched. A cached listing that will not parse is
// treated as no listing at all rather than as a failure, because the file is a
// derived artifact and re-fetching it costs three requests.
func (f *Fetcher) Areas(ctx context.Context) ([]string, error) {
	cachePath := filepath.Join(f.cacheDir, areasCacheFile)
	if cached, ok := f.cachedIfUsable(cachePath); ok {
		var areas []string
		if json.Unmarshal([]byte(cached), &areas) == nil && len(areas) > 0 {
			return areas, nil
		}
	}
	if f.offline {
		return nil, errors.New("apidocs: the doc/api listing is not cached and offline was asked for")
	}

	areas, err := f.downloadAreas(ctx)
	if err != nil {
		return nil, err
	}
	if mkdirErr := os.MkdirAll(f.cacheDir, 0o750); mkdirErr != nil {
		return nil, fmt.Errorf("apidocs: create cache dir: %w", mkdirErr)
	}
	// The marshal goes through cmdutil.Must because it cannot fail on a slice
	// of strings, and a caller handed that error could only print it and stop,
	// which is what the panic already does.
	encoded := cmdutil.Must(json.Marshal(areas))
	if writeErr := os.WriteFile(cachePath, encoded, 0o600); writeErr != nil {
		return nil, fmt.Errorf("apidocs: write cache %s: %w", cachePath, writeErr)
	}
	return areas, nil
}

// downloadAreas walks the tree listing page by page.
//
// Pagination is by page number and stops on a short page rather than by
// reading the endpoint's own x-next-page header, because a page that happens
// to be exactly full then costs one extra request that comes back empty, and
// that is cheaper than a second code path through the fetcher for the sake of
// one header.
func (f *Fetcher) downloadAreas(ctx context.Context) ([]string, error) {
	base := f.areasURL
	seen := map[string]struct{}{}
	for page := 1; ; page++ {
		body, err := f.request(ctx, fmt.Sprintf("the doc/api listing (page %d)", page), pageURL(base, page))
		if err != nil {
			return nil, err
		}
		var entries []treeEntry
		if unmarshalErr := json.Unmarshal(body, &entries); unmarshalErr != nil {
			return nil, fmt.Errorf("apidocs: parse the doc/api listing: %w", unmarshalErr)
		}
		for _, entry := range entries {
			if area, ok := areaOf(entry); ok {
				seen[area] = struct{}{}
			}
		}
		if len(entries) < areasPerPage {
			break
		}
	}
	if len(seen) == 0 {
		return nil, errors.New("apidocs: the doc/api listing came back empty, which means it is no longer where this looks for it")
	}
	areas := make([]string, 0, len(seen))
	for area := range seen {
		areas = append(areas, area)
	}
	slices.Sort(areas)
	return areas, nil
}

// areaOf turns one tree entry into an area name, and reports whether it is one
// at all: a directory is not, and neither is a file that is not markdown.
func areaOf(entry treeEntry) (string, bool) {
	if entry.Type != "blob" {
		return "", false
	}
	trimmed, ok := strings.CutPrefix(entry.Path, areasPrefix)
	if !ok {
		return "", false
	}
	name, ok := strings.CutSuffix(trimmed, ".md")
	if !ok || name == "" {
		return "", false
	}
	return name, true
}

// pageURL adds the paging parameters to the listing URL, keeping whatever the
// caller already put there.
func pageURL(base string, page int) string {
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + url.Values{
		"per_page": []string{strconv.Itoa(areasPerPage)},
		"page":     []string{strconv.Itoa(page)},
	}.Encode()
}
