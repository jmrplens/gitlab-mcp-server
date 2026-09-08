package paths

import (
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// Owner statuses.
const (
	statusDeclared   = "declared"
	statusUndeclared = "undeclared"
	// statusUnmapped is for an owner that is no package under internal/tools at
	// all, which the recording could not have seen either way. It is reported
	// and never gated, because the fault would be in the catalog's ownership
	// metadata rather than in any test.
	statusUnmapped = "unmapped"
)

// SilentOwner is one package the catalog owns actions in that was seen issuing
// no request.
type SilentOwner struct {
	Package string `json:"package"`
	Status  string `json:"status"`
	// Actions are the catalog actions this package owns, all of them unseen by
	// the recording, sorted.
	Actions  []string `json:"actions"`
	Category string   `json:"category,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

// observed classifies every action by whether the package that owns it was
// recorded issuing anything, and holds each silent package to a declaration.
func observed(root string, rows []requestinventory.Row, actions []requestinventory.Action) (requestinventory.Coverage, []SilentOwner) {
	coverage := requestinventory.Classify(root, rows, actions)

	owners := make([]SilentOwner, 0, len(coverage.SilentOwners)+len(coverage.UnmappedOwners))
	for _, owner := range coverage.SilentOwners {
		silent := SilentOwner{Package: owner.Package, Status: statusUndeclared, Actions: owner.Actions}
		if declaration, ok := declaredSilentOwners[owner.Package]; ok {
			silent.Status = statusDeclared
			silent.Category = declaration.Category
			silent.Reason = declaration.Reason
		}
		owners = append(owners, silent)
	}
	for _, owner := range coverage.UnmappedOwners {
		owners = append(owners, SilentOwner{Package: owner.Package, Status: statusUnmapped, Actions: owner.Actions})
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].Package < owners[j].Package })
	return coverage, owners
}

// staleDeclarations names the declarations that no longer describe the tree.
//
// A package that has since recorded a request, or that no longer owns any
// action, leaves behind a claim a later reader would trust. The claim is the
// only thing standing between that package and the gate, so it has to be
// retired the moment it stops being true.
func staleDeclarations(owners []SilentOwner) []string {
	silent := map[string]struct{}{}
	for _, owner := range owners {
		if owner.Status == statusDeclared {
			silent[owner.Package] = struct{}{}
		}
	}
	stale := make([]string, 0)
	for pkg, declaration := range declaredSilentOwners {
		if _, ok := silent[pkg]; !ok {
			stale = append(stale, fmt.Sprintf(
				"%s/%s is declared silent (%s) and is not: it either recorded a request or no longer owns an action",
				requestinventory.ToolsDir, pkg, declaration.Category,
			))
		}
	}
	sort.Strings(stale)
	return stale
}

// undeclaredSilent counts the actions whose owning package recorded nothing and
// declared nothing, which is what the gate fails on.
func undeclaredSilent(owners []SilentOwner) (packages, actions int) {
	for _, owner := range owners {
		if owner.Status != statusUndeclared {
			continue
		}
		packages++
		actions += len(owner.Actions)
	}
	return packages, actions
}
