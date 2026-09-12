package main

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// The tree the static gate reads, relative to the module root, and the
// harness it reads it against.
const (
	// gitlabTestDir is the directory the three runtime packages live under.
	gitlabTestDir = "test/e2e/gitlab"
	// harnessImportPath is the harness package every test calls through.
	harnessImportPath = "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	// actionIDTypeName is the harness type every canonical id is spelled as.
	actionIDTypeName = "ActionID"
	// tierFuncName is the harness constructor of the tier requirement.
	tierFuncName = "Tier"
	// needsFuncName is the harness option that hands requirements to New,
	// which is the only place a Tier requirement means anything.
	needsFuncName = "Needs"
	// e2eBuildTag is the constraint every e2e file carries.
	e2eBuildTag = "e2e"
)

// staticPatterns are the packages the gate loads: the runtime packages, and
// the harness and fixture libraries they consume.
var staticPatterns = []string{"./" + gitlabTestDir + "/...", "./test/e2e/internal/..."}

// placements are the three runtime packages and what each may run.
const (
	placementCommon = "common"
	placementCE     = "ce"
	placementEE     = "ee"
)

// staticConfig is one run of the static gate.
type staticConfig struct {
	// dir is the module root.
	dir string
	// harnessPath is the import path of the harness package.
	harnessPath string
	// patterns are the packages to load.
	patterns []string
	// catalog is every action the gate accepts, with its minimum tier.
	catalog map[string]edition.Tier
	// ratchet is whether the ratchet is on.
	ratchet bool
	// exemptions is the ratchet's declaration table.
	exemptions map[string]actionExemption
	// overlay supplies source that is not on disk.
	overlay map[string][]byte
}

// staticFinding is one thing the gate fails on.
type staticFinding struct {
	// Kind names the rule.
	Kind string `json:"kind"`
	// Pos is file:line, empty for a finding about the tree rather than a
	// site.
	Pos string `json:"pos,omitempty"`
	// Message says what is wrong.
	Message string `json:"message"`
}

// String spells the finding for the terminal.
func (f staticFinding) String() string {
	if f.Pos == "" {
		return f.Kind + ": " + f.Message
	}
	return f.Pos + ": " + f.Kind + ": " + f.Message
}

// The finding kinds.
const (
	findingUnknownID          = "unknown-id"
	findingTierPlacement      = "tier-placement"
	findingUltimateUndeclared = "ultimate-undeclared"
	findingDiscardedResult    = "discarded-result"
	findingDeadExport         = "dead-export"
	findingUnexercised        = "unexercised-action"
	findingStaleExemption     = "stale-exemption"
	findingPlacement          = "unknown-placement"
)

// staticNote is something the gate lists without failing on it.
type staticNote struct {
	Pos  string `json:"pos"`
	Text string `json:"text"`
}

// staticResult is what one run of the gate found.
type staticResult struct {
	// Skipped is whether the gate had no tree to read.
	Skipped bool `json:"skipped"`
	// Packages lists the packages the gate read.
	Packages []string `json:"packages"`
	// Sites lists every constant id site.
	Sites []idSite `json:"sites"`
	// NonConstant lists the verb calls whose id is not a constant.
	NonConstant []staticNote `json:"non_constant"`
	// DeadExports lists the harness exports nothing uses.
	DeadExports []string `json:"dead_exports"`
	// Findings is what the gate fails on.
	Findings []staticFinding `json:"findings"`
	// unassertedIDs are the actions whose every result-bearing call site
	// discards the result, for the classification.
	unassertedIDs map[string]bool
	// testIDs maps each Test function to the ids it names, directly or
	// through the helpers it calls.
	testIDs map[string]map[string]bool
}

// failed reports whether the gate found anything to fail on.
func (r *staticResult) failed() bool {
	return len(r.Findings) > 0
}

// runStatic runs the gate.
func runStatic(cfg staticConfig) (*staticResult, error) {
	result := &staticResult{unassertedIDs: map[string]bool{}, testIDs: map[string]map[string]bool{}}
	if _, err := os.Stat(filepath.Join(cfg.dir, gitlabTestDir)); os.IsNotExist(err) {
		result.Skipped = true
		return result, nil
	}
	loaded, err := goprogram.LoadWith(cfg.dir, cfg.patterns, goprogram.Options{
		Tests: true, BuildTags: []string{e2eBuildTag}, Overlay: cfg.overlay,
	})
	if err != nil {
		return nil, err
	}
	selected := selectPackages(loaded)
	// From everything loaded, not from the selection: the selection keeps the
	// test variant of each package, and the harness's test variant exports
	// every Test function of the harness, which nothing outside it can use.
	harness := findHarness(loaded, cfg.harnessPath)
	if harness == nil {
		return nil, fmt.Errorf("the harness package %s was not loaded", cfg.harnessPath)
	}
	scanner := newPackageScanner(cfg.harnessPath, harness)
	var scans []*packageScan
	for _, pkg := range selected {
		result.Packages = append(result.Packages, pkg.PkgPath)
		placement, underGitlab := placementOf(pkg.PkgPath, cfg.harnessPath)
		if !underGitlab {
			continue
		}
		scans = append(scans, scanner.scan(pkg, placement))
	}
	sort.Strings(result.Packages)
	result.collect(scans)
	result.DeadExports = deadExports(harness, selected, cfg.harnessPath)
	result.judge(cfg, scans)
	return result, nil
}

// selectPackages keeps one variant per package path: the test variant when
// there is one, since it holds the test files, and never the generated test
// main.
func selectPackages(loaded []*packages.Package) []*packages.Package {
	best := map[string]*packages.Package{}
	for _, pkg := range loaded {
		if pkg.Name == "main" && strings.HasSuffix(pkg.PkgPath, ".test") {
			continue
		}
		current, seen := best[pkg.PkgPath]
		if !seen || len(pkg.CompiledGoFiles) > len(current.CompiledGoFiles) {
			best[pkg.PkgPath] = pkg
		}
	}
	selected := make([]*packages.Package, 0, len(best))
	for _, pkg := range best {
		selected = append(selected, pkg)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].PkgPath < selected[j].PkgPath })
	return selected
}

// findHarness returns the harness package as its consumers see it: the plain
// variant, whose objects are the ones a consumer's type information names
// and whose scope holds no Test function. It is given the loaded packages
// rather than the selected ones, since the selection prefers the test
// variant, and falls back to whichever variant is there.
func findHarness(loaded []*packages.Package, harnessPath string) *packages.Package {
	var found *packages.Package
	for _, pkg := range loaded {
		if pkg.PkgPath != harnessPath {
			continue
		}
		if found == nil || !strings.Contains(pkg.ID, "[") {
			found = pkg
		}
	}
	return found
}

// placementOf names the runtime package a package path belongs to, and
// reports false for a package outside test/e2e/gitlab.
//
// The module path is read off the harness import path rather than asked of
// the loader, so a fake module in a test places its packages the same way.
func placementOf(pkgPath, harnessPath string) (string, bool) {
	module := strings.TrimSuffix(harnessPath, "/test/e2e/internal/harness")
	root := module + "/" + gitlabTestDir + "/"
	if !strings.HasPrefix(pkgPath, root) {
		return "", false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(pkgPath, root), "_test")
	placement, _, _ := strings.Cut(rest, "/")
	return placement, true
}

// isActionIDType reports whether a type is the harness's ActionID, whichever
// variant of the package declared it.
func isActionIDType(t types.Type, harnessPath string) bool {
	named, isNamed := t.(*types.Named)
	if !isNamed || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == harnessPath && named.Obj().Name() == actionIDTypeName
}

// collect folds the per-package scans into the result's lists.
func (r *staticResult) collect(scans []*packageScan) {
	resultSites := map[string]int{}
	discarded := map[string]int{}
	for _, scan := range scans {
		r.Sites = append(r.Sites, scan.sites...)
		r.NonConstant = append(r.NonConstant, scan.nonConstant...)
		for id, n := range scan.resultSites {
			resultSites[id] += n
		}
		for id, n := range scan.discardedSites {
			discarded[id] += n
		}
		for test, ids := range scan.testIDs() {
			if r.testIDs[test] == nil {
				r.testIDs[test] = map[string]bool{}
			}
			for id := range ids {
				r.testIDs[test][id] = true
			}
		}
	}
	for id, n := range resultSites {
		if n > 0 && discarded[id] == n {
			r.unassertedIDs[id] = true
		}
	}
	sort.Slice(r.Sites, func(i, j int) bool {
		if r.Sites[i].Pos != r.Sites[j].Pos {
			return r.Sites[i].Pos < r.Sites[j].Pos
		}
		return r.Sites[i].ID < r.Sites[j].ID
	})
	sort.Slice(r.NonConstant, func(i, j int) bool { return r.NonConstant[i].Pos < r.NonConstant[j].Pos })
}

// judge applies the rules to what was collected.
func (r *staticResult) judge(cfg staticConfig, scans []*packageScan) {
	exercised := map[string]map[string]bool{}
	for _, scan := range scans {
		r.judgePlacement(cfg, scan)
		for _, site := range scan.sites {
			if exercised[site.ID] == nil {
				exercised[site.ID] = map[string]bool{}
			}
			exercised[site.ID][site.Placement] = true
		}
		for _, note := range scan.discarded {
			r.Findings = append(r.Findings, staticFinding{Kind: findingDiscardedResult, Pos: note.Pos, Message: note.Text})
		}
	}
	r.judgeExemptions(cfg, exercised)
	if cfg.ratchet {
		r.judgeRatchet(cfg, exercised)
		for _, name := range r.DeadExports {
			r.Findings = append(r.Findings, staticFinding{Kind: findingDeadExport, Message: name + " is exported by the harness and used by nothing"})
		}
	}
	sort.Slice(r.Findings, func(i, j int) bool {
		if r.Findings[i].Pos != r.Findings[j].Pos {
			return r.Findings[i].Pos < r.Findings[j].Pos
		}
		return r.Findings[i].Message < r.Findings[j].Message
	})
}

// judgePlacement checks every site of one package against the catalog and
// the package's placement.
func (r *staticResult) judgePlacement(cfg staticConfig, scan *packageScan) {
	if scan.placement != placementCommon && scan.placement != placementCE && scan.placement != placementEE {
		r.Findings = append(r.Findings, staticFinding{
			Kind:    findingPlacement,
			Message: fmt.Sprintf("package %s is under %s and is not common, ce or ee", scan.pkgPath, gitlabTestDir),
		})
	}
	for _, site := range scan.sites {
		tier, known := cfg.catalog[site.ID]
		if !known {
			r.Findings = append(r.Findings, staticFinding{
				Kind: findingUnknownID, Pos: site.Pos,
				Message: site.ID + " is not a catalog action",
			})
			continue
		}
		switch {
		case scan.placement != placementEE && tier > edition.Free:
			r.Findings = append(r.Findings, staticFinding{
				Kind: findingTierPlacement, Pos: site.Pos,
				Message: fmt.Sprintf("%s is %s and %s runs on every runtime", site.ID, tier, scan.placement),
			})
		case scan.placement == placementEE && tier == edition.Ultimate:
			r.judgeUltimate(scan, site)
		}
	}
}

// judgeUltimate checks that every test reaching an Ultimate site declares
// the tier.
func (r *staticResult) judgeUltimate(scan *packageScan, site idSite) {
	for _, test := range scan.testsReaching(site) {
		if scan.declaresUltimate(test) {
			continue
		}
		r.Findings = append(r.Findings, staticFinding{
			Kind: findingUltimateUndeclared, Pos: site.Pos,
			Message: fmt.Sprintf("%s is Ultimate and %s does not declare Needs(Tier(edition.Ultimate))", site.ID, test),
		})
	}
}

// judgeExemptions reports the exemption entries that no longer hold.
func (r *staticResult) judgeExemptions(cfg staticConfig, exercised map[string]map[string]bool) {
	for id, exemption := range cfg.exemptions {
		tier, known := cfg.catalog[id]
		switch {
		case !known:
			r.Findings = append(r.Findings, staticFinding{
				Kind:    findingStaleExemption,
				Message: id + " is exempted and is not a catalog action",
			})
		case !declaredCategories[exemption.Category]:
			r.Findings = append(r.Findings, staticFinding{
				Kind:    findingStaleExemption,
				Message: fmt.Sprintf("%s is exempted under the unknown category %q", id, exemption.Category),
			})
		case canRun(tier, exercised[id]):
			r.Findings = append(r.Findings, staticFinding{
				Kind:    findingStaleExemption,
				Message: id + " is exempted and a package that can run it names it",
			})
		}
	}
}

// judgeRatchet reports every catalog action no package that can run it
// names, unless it is exempted.
func (r *staticResult) judgeRatchet(cfg staticConfig, exercised map[string]map[string]bool) {
	ids := make([]string, 0, len(cfg.catalog))
	for id := range cfg.catalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, exempt := cfg.exemptions[id]; exempt || canRun(cfg.catalog[id], exercised[id]) {
			continue
		}
		r.Findings = append(r.Findings, staticFinding{
			Kind:    findingUnexercised,
			Message: id + " has no scenario in a package that can run it and no entry in exemptions.go",
		})
	}
}

// canRun reports whether an action of the given tier is named in a package
// that can run it: any of the three for a Free action, ee for a licensed one.
func canRun(tier edition.Tier, placements map[string]bool) bool {
	if tier > edition.Free {
		return placements[placementEE]
	}
	return placements[placementCommon] || placements[placementCE] || placements[placementEE]
}
