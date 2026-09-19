package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncompat"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// oracle is what an action ID is judged against: the canonical IDs the catalog
// builds, the aliases that resolve to one of them, and the domain of every
// canonical ID.
//
// The domains are kept because the prose rule needs them. A dotted token in a
// Usage line is only a candidate action ID when its left half names a domain
// the catalog has; without that test the rule reports github.com, gitlab.com,
// e.g and every JSON path an example binding spells.
type oracle struct {
	ids     map[string]struct{}
	aliases map[string]string
	domains map[string]struct{}
	// members is the right half of every canonical ID. Prose needs it because
	// the commonest wrong spelling invents the domain rather than the action:
	// "commit.list" names an action the catalog really has, under a domain it
	// does not have, so a rule that admits a token only when its domain is
	// known drops exactly the class this command exists to find.
	members map[string]struct{}
	// sorted is the canonical ID set in order, which is what a suggestion is
	// searched over and what makes the suggestion deterministic.
	sorted []string
}

// buildOracle builds the catalog twice, self-managed and GitLab.com, and
// returns the union.
//
// Both are needed, and the union rather than the wider one alone. Orbit's
// group is contributed only when the client is GitLab.com, so a self-managed
// build alone reports its six IDs as phantoms; and the union is what keeps the
// answer right if an action is ever gated the other way, which the catalog has
// a flag for and no build here could otherwise see.
//
// The standalone dynamic actions are added the way cmd/server adds them:
// gitlab_execute_action takes those IDs, so a hint naming one resolves and
// must not be called dead.
func buildOracle() (*oracle, error) {
	selfManaged, cleanup := mcpsurface.NewStubClient()
	defer cleanup()

	built := &oracle{
		ids:     map[string]struct{}{},
		aliases: map[string]string{},
		domains: map[string]struct{}{},
		members: map[string]struct{}{},
	}
	for _, client := range []*gitlabclient.Client{selfManaged, mcpsurface.NewGitLabComClient()} {
		catalog, err := catalogFor(client)
		if err != nil {
			return nil, err
		}
		built.addCatalog(catalog)
	}
	for _, alias := range actioncompat.ActionAliases() {
		built.addAlias(alias.Alias, alias.Canonical)
	}
	built.finish()
	return built, nil
}

// catalogFor builds the Ultimate catalog for one client, standalone dynamic
// actions included.
func catalogFor(client *gitlabclient.Client) (*actioncatalog.Catalog, error) {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add standalone dynamic actions: %w", err)
	}
	return catalog, nil
}

// addCatalog records one catalog's IDs, the aliases each action carries and
// the compatibility aliases its spec declares.
func (o *oracle) addCatalog(catalog *actioncatalog.Catalog) {
	if catalog == nil {
		return
	}
	for _, action := range catalog.Actions() {
		id := normalizeID(string(action.ID))
		if id == "" {
			continue
		}
		o.ids[id] = struct{}{}
		if domain, member, found := strings.Cut(id, "."); found && domain != "" {
			o.domains[domain] = struct{}{}
			if member != "" {
				o.members[member] = struct{}{}
			}
		}
		for _, alias := range action.Aliases {
			o.addAlias(alias, id)
		}
		for _, alias := range action.Compatibility.ActionAliases {
			o.addAlias(alias.Alias, id)
		}
	}
}

// addAlias records one alias and what it resolves to.
func (o *oracle) addAlias(alias, canonical string) {
	key := normalizeID(alias)
	if key == "" {
		return
	}
	if _, recorded := o.aliases[key]; recorded {
		return
	}
	o.aliases[key] = normalizeID(canonical)
}

// finish drops the aliases that are canonical IDs in their own right and
// freezes the sorted ID list.
//
// An alias that is also an ID has to be an ID here, or a correct cross-link
// would be reported as an alias in the second catalog's pass.
func (o *oracle) finish() {
	for alias := range o.aliases {
		if _, isID := o.ids[alias]; isID {
			delete(o.aliases, alias)
		}
	}
	o.sorted = make([]string, 0, len(o.ids))
	for id := range o.ids {
		o.sorted = append(o.sorted, id)
	}
	sort.Strings(o.sorted)
}

// isID reports whether id is a canonical catalog action ID.
func (o *oracle) isID(id string) bool {
	_, ok := o.ids[normalizeID(id)]
	return ok
}

// alias resolves a registered alias to the canonical ID it stands for.
func (o *oracle) alias(id string) (string, bool) {
	canonical, ok := o.aliases[normalizeID(id)]
	return canonical, ok
}

// hasDomain reports whether domain is the left half of some canonical ID.
func (o *oracle) hasDomain(domain string) bool {
	_, ok := o.domains[domain]
	return ok
}

// hasMember reports whether member is the right half of some canonical ID.
func (o *oracle) hasMember(member string) bool {
	_, ok := o.members[member]
	return ok
}

// normalizeID spells an ID the way the dynamic registry resolves one, so a
// judgement here and a lookup at runtime cannot disagree over case or spacing.
func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
