package requestinventory

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// ToolsDir is where a domain package lives, and the prefix a recorded package
// name carries when the catalog knows it as an owner.
const ToolsDir = "internal/tools"

// RootOwner is what the catalog calls the orchestration package itself, which
// is ToolsDir rather than a directory under it: a spec group that declares no
// owner is given this one, and TestCollectedActionSpecs_DeclareCatalogOwnership
// admits it beside the domain names. Resolving it to the root package is what
// keeps an action owned by it from being classified as owned by nothing.
const RootOwner = "tools"

// Action is one catalog action as this dimension sees it: an identity and the
// package that owns it, which is the only handle the recording can be joined
// on.
type Action struct {
	ID    string
	Owner string
}

// buildCatalog is a seam: the catalog is compiled into whichever binary asks
// for it and cannot be made to fail from outside, so the branch that reports
// the failure is only reachable from a test that replaces this.
var buildCatalog = tools.BuildActionCatalog

// Actions returns every action in the catalog at the widest tier, so the counts
// below are of the whole surface rather than one licence's.
func Actions() ([]Action, error) {
	catalog, err := buildCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		return nil, err
	}
	actions := make([]Action, 0, catalog.CountActions())
	for _, action := range catalog.Actions() {
		actions = append(actions, Action{ID: string(action.ID), Owner: action.OwnerPackage})
	}
	return actions, nil
}

// Owner is one package the catalog names as owning actions, with the actions it
// owns, listed so a report can name the work rather than score it.
type Owner struct {
	Package string   `json:"package"`
	Actions []string `json:"actions"`
}

// Coverage counts what the recording could and could not see of the catalog.
//
// The three counts are disjoint and none of them is per action, which is the
// point of keeping them apart. Covered means the package owning the action
// issued some request, not that this action's request was seen; silent means
// the package issued none; unmapped means the action's owner names no package
// at all, neither the orchestration package nor a domain under it, so the
// recording could not have seen it either way and the fault is in the
// catalog's ownership metadata rather than in any test.
//
// Silent is weaker than "never exercised" for a second reason beyond the
// coarse grain, and internal/tools/adminspecs is the whole of it today: a
// package may declare specs whose handlers live in other packages, and the
// request is then recorded under the package that made it while the count of
// silent actions blames the one that declared them. Reading the silent list as
// a work list means reading it package by package, not action by action.
type Coverage struct {
	Total    int
	Covered  int
	Silent   int
	Unmapped int
	// Silent and Unmapped owners are sorted by package, and each one's actions
	// are sorted too, so a report of them is stable between runs.
	SilentOwners   []Owner
	UnmappedOwners []Owner
}

// Classify sorts every action by whether the package that owns it was recorded
// issuing anything. The root is needed to tell a package that recorded nothing
// from a name that is no package at all.
func Classify(root string, rows []Row, actions []Action) Coverage {
	recorded := map[string]struct{}{}
	for _, row := range rows {
		if owner, ok := strings.CutPrefix(row.Package, ToolsDir+"/"); ok {
			recorded[owner] = struct{}{}
			continue
		}
		if row.Package == ToolsDir {
			recorded[RootOwner] = struct{}{}
		}
	}

	coverage := Coverage{Total: len(actions)}
	isPackage := map[string]bool{}
	silent := map[string][]string{}
	unmapped := map[string][]string{}
	for _, action := range actions {
		if _, ok := recorded[action.Owner]; ok {
			coverage.Covered++
			continue
		}
		known, asked := isPackage[action.Owner]
		if !asked {
			known = directoryExists(PackageDir(root, action.Owner))
			isPackage[action.Owner] = known
		}
		if known {
			coverage.Silent++
			silent[action.Owner] = append(silent[action.Owner], action.ID)
			continue
		}
		coverage.Unmapped++
		unmapped[action.Owner] = append(unmapped[action.Owner], action.ID)
	}
	coverage.SilentOwners = ownersOf(silent)
	coverage.UnmappedOwners = ownersOf(unmapped)
	return coverage
}

// ownersOf renders an owner map as the sorted list a report prints.
func ownersOf(byPackage map[string][]string) []Owner {
	owners := make([]Owner, 0, len(byPackage))
	for pkg, actions := range byPackage {
		sort.Strings(actions)
		owners = append(owners, Owner{Package: pkg, Actions: actions})
	}
	slices.SortFunc(owners, func(a, b Owner) int { return strings.Compare(a.Package, b.Package) })
	return owners
}

// Packages names the owners in a list, for a report that wants the packages
// without the actions under them.
func Packages(owners []Owner) []string {
	names := make([]string, 0, len(owners))
	for _, owner := range owners {
		names = append(names, owner.Package)
	}
	return names
}

// PackageDir is where the package an owner names lives, which is ToolsDir
// itself for [RootOwner] and a directory under it for every domain.
func PackageDir(root, owner string) string {
	if owner == RootOwner {
		return filepath.Join(root, filepath.FromSlash(ToolsDir))
	}
	return filepath.Join(root, filepath.FromSlash(ToolsDir), owner)
}

// directoryExists reports whether path is a directory.
func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
