package actionrequests

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// Action is one catalog action, with what a construction site is joined to it
// on and the classification a reader judges it by.
type Action struct {
	// ID is the canonical action ID, which a finding is filed under.
	ID string
	// Name is the action name its spec declares, which a construction site
	// is found by.
	Name string
	// Owner is the package the catalog names as the action's owner.
	Owner string
	// Tool is the individual tool name the action projects, which tells apart
	// two sites declaring one name in one package. It is "" for an action the
	// individual surface does not publish.
	Tool string
	// ReadOnly is the catalog's classification of the action.
	ReadOnly bool
}

// catalogs is the catalog builder, a variable so a test can make it fail.
var catalogs = actionids.Catalogs

// Catalog returns every action this repository publishes, sorted by ID: the
// catalog at Ultimate for a self-managed instance and for GitLab.com, with the
// standalone surface actions, each action once.
//
// Both builds are read because each holds actions the other does not: Orbit's
// six only the GitLab.com build, and an action gated the other way only the
// self-managed one. An action both hold is taken from the first build, which
// is the self-managed one; the two agree on everything read here.
func Catalog() ([]Action, error) {
	built, err := catalogs()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var actions []Action
	for _, catalog := range built {
		for _, action := range catalog.Actions() {
			id := string(action.ID)
			if seen[id] {
				continue
			}
			seen[id] = true
			actions = append(actions, Action{
				ID:       id,
				Name:     action.Name,
				Owner:    action.OwnerPackage,
				Tool:     strings.TrimSpace(action.IndividualTool.Name),
				ReadOnly: action.ReadOnly,
			})
		}
	}
	slices.SortFunc(actions, func(a, b Action) int { return strings.Compare(a.ID, b.ID) })
	return actions, nil
}
