package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// toolsDir is where a domain package lives, and the prefix a recorded package
// name carries when the catalog knows it as an owner.
const toolsDir = "internal/tools"

// coverage counts what the recording could and could not see of the catalog.
//
// The three counts are disjoint and none of them is per action, which is the
// point of keeping them apart. Covered means the package owning the action
// issued some request, not that this action's request was seen; silent means
// the package issued none; unmapped means the action's owner is not a package
// under internal/tools at all, so the recording could not have seen it either
// way.
//
// Silent is weaker than "never exercised" for a second reason beyond the
// coarse grain, and internal/tools/adminspecs is the whole of it today: a
// package may declare specs whose handlers live in other packages, and the
// request is then recorded under the package that made it while the count of
// silent actions blames the one that declared them. Reading the silent list as
// a work list means reading it package by package, not action by action.
type coverage struct {
	Total    int
	Covered  int
	Silent   int
	Unmapped int
	// SilentOwners and UnmappedOwners name the packages behind those two
	// counts, sorted, so -v can print the work list rather than the score.
	SilentOwners   []string
	UnmappedOwners []string
}

// buildCatalog returns the owning package of every action in the catalog at
// the widest tier, one entry per action, so the counts below are of actions
// and the surface is the whole one rather than one licence's.
//
// It is a variable so the failure branch stays reachable from a test: the
// catalog is compiled into this binary and cannot be made to fail from
// outside.
var buildCatalog = func() ([]string, error) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		return nil, err
	}
	owners := make([]string, 0, catalog.CountActions())
	for _, action := range catalog.Actions() {
		owners = append(owners, action.OwnerPackage)
	}
	return owners, nil
}

// summarize prints what the merged inventory holds, and how much of the
// catalog the recording could see.
//
// A failure to build the catalog costs the coverage line and nothing else: the
// inventory is the artifact this command exists to write, and it is complete
// with or without a count of what is missing from it.
func summarize(progress io.Writer, root string, rows []row, verbose bool) {
	fmt.Fprintf(progress, "request inventory: %d rows, %d distinct paths, %d packages\n",
		len(rows), countDistinct(rows, func(r row) string { return r.Path }), countDistinct(rows, func(r row) string { return r.Package }))

	owners, err := buildCatalog()
	if err != nil {
		fmt.Fprintf(progress, "  catalog coverage unavailable: %v\n", err)
		return
	}
	summary := coverageOf(root, rows, owners)
	fmt.Fprintf(progress, "  %d of %d catalog actions are owned by a package that recorded a request\n", summary.Covered, summary.Total)
	fmt.Fprintf(progress, "  %d in %d package(s) recorded none, and %d are owned by a name that is no package under %s\n",
		summary.Silent, len(summary.SilentOwners), summary.Unmapped, toolsDir)
	if !verbose {
		return
	}
	for _, owner := range summary.SilentOwners {
		fmt.Fprintf(progress, "  silent: %s/%s\n", toolsDir, owner)
	}
	for _, owner := range summary.UnmappedOwners {
		fmt.Fprintf(progress, "  no package of that name: %s\n", owner)
	}
}

// countDistinct counts the distinct values of one field across the rows.
func countDistinct(rows []row, field func(row) string) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[field(r)] = struct{}{}
	}
	return len(seen)
}

// coverageOf classifies every action, by the owning package the catalog gives
// it, according to whether that package was recorded issuing anything.
func coverageOf(root string, rows []row, owners []string) coverage {
	recorded := map[string]struct{}{}
	for _, r := range rows {
		if owner, ok := strings.CutPrefix(r.Package, toolsDir+"/"); ok {
			recorded[owner] = struct{}{}
		}
	}

	summary := coverage{Total: len(owners)}
	isPackage := map[string]bool{}
	silent := map[string]struct{}{}
	unmapped := map[string]struct{}{}
	for _, owner := range owners {
		if _, ok := recorded[owner]; ok {
			summary.Covered++
			continue
		}
		known, asked := isPackage[owner]
		if !asked {
			known = directoryExists(filepath.Join(root, toolsDir, owner))
			isPackage[owner] = known
		}
		if known {
			summary.Silent++
			silent[owner] = struct{}{}
			continue
		}
		summary.Unmapped++
		unmapped[owner] = struct{}{}
	}
	summary.SilentOwners = slices.Sorted(maps.Keys(silent))
	summary.UnmappedOwners = slices.Sorted(maps.Keys(unmapped))
	return summary
}

// directoryExists reports whether path is a directory.
func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
