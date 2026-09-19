package actionids

import (
	"fmt"
	"regexp"
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

// DottedToken matches an action-ID-shaped token. The shape alone is far too
// generous, which is why every match is also held to [IDs.Candidates]' test
// that one of its halves is one the catalog uses.
var DottedToken = regexp.MustCompile(`\b[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\b`)

// IDs is what an action ID is judged against: the canonical IDs the catalog
// builds, the aliases that resolve to one of them, and the two halves of every
// canonical ID.
//
// The halves are kept because the prose rule needs them. A dotted token in a
// sentence is only a candidate action ID when one of its halves names
// something the catalog uses; without that test the rule reports github.com,
// gitlab.com, go.mod and every JSON path an example binding spells.
type IDs struct {
	ids     map[string]struct{}
	aliases map[string]string
	domains map[string]struct{}
	// members is the right half of every canonical ID. Prose needs it because
	// the commonest wrong spelling invents the domain rather than the action:
	// "commit.list" names an action the catalog really has, under a domain it
	// does not have, so a rule that admits a token only when its domain is
	// known drops exactly the class these gates exist to find.
	members map[string]struct{}
	// sorted is the canonical ID set in order, which is what a suggestion is
	// searched over and what makes the suggestion deterministic.
	sorted []string
}

// Build builds the catalog twice, self-managed and GitLab.com, and returns the
// union.
//
// Both are needed, and the union rather than the wider one alone. Orbit's
// group is contributed only when the client is GitLab.com, so a self-managed
// build alone reports its six IDs as phantoms; and the union is what keeps the
// answer right if an action is ever gated the other way, which the catalog has
// a flag for and no build here could otherwise see.
//
// The standalone dynamic actions are added the way cmd/server adds them:
// gitlab_execute_action takes those IDs, so a cross-link naming one resolves
// and must not be called dead.
func Build() (*IDs, error) {
	selfManaged, cleanup := mcpsurface.NewStubClient()
	defer cleanup()

	built := &IDs{
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

// New builds an [IDs] over an explicit set of canonical IDs and aliases,
// which is what a caller with a small, stable catalog of its own uses in place
// of the whole tree's: a test asserting how a rule judges one ID does not want
// the answer to move when a domain is added.
//
// It applies the same two rules [Build] ends with, so a set made here and one
// read from the catalog cannot disagree: an alias that is also a canonical ID
// stays an ID, and the halves of every ID are what the prose rule filters on.
func New(ids []string, aliases map[string]string) *IDs {
	built := &IDs{
		ids:     map[string]struct{}{},
		aliases: map[string]string{},
		domains: map[string]struct{}{},
		members: map[string]struct{}{},
	}
	for _, id := range ids {
		built.addID(id)
	}
	for alias, canonical := range aliases {
		built.addAlias(alias, canonical)
	}
	built.finish()
	return built
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
func (o *IDs) addCatalog(catalog *actioncatalog.Catalog) {
	if catalog == nil {
		return
	}
	for _, action := range catalog.Actions() {
		id := o.addID(string(action.ID))
		if id == "" {
			continue
		}
		for _, alias := range action.Aliases {
			o.addAlias(alias, id)
		}
		for _, alias := range action.Compatibility.ActionAliases {
			o.addAlias(alias.Alias, id)
		}
	}
}

// addID records one canonical ID and the two halves the prose rule filters on,
// returning the normalized spelling, or the empty string for a value that is
// no ID at all.
func (o *IDs) addID(id string) string {
	normalized := Normalize(id)
	if normalized == "" {
		return ""
	}
	o.ids[normalized] = struct{}{}
	if domain, member, found := strings.Cut(normalized, "."); found && domain != "" {
		o.domains[domain] = struct{}{}
		if member != "" {
			o.members[member] = struct{}{}
		}
	}
	return normalized
}

// addAlias records one alias and what it resolves to.
func (o *IDs) addAlias(alias, canonical string) {
	key := Normalize(alias)
	if key == "" {
		return
	}
	if _, recorded := o.aliases[key]; recorded {
		return
	}
	o.aliases[key] = Normalize(canonical)
}

// finish drops the aliases that are canonical IDs in their own right and
// freezes the sorted ID list.
//
// An alias that is also an ID has to be an ID here, or a correct cross-link
// would be reported as an alias in the second catalog's pass.
func (o *IDs) finish() {
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

// IsID reports whether id is a canonical catalog action ID.
func (o *IDs) IsID(id string) bool {
	_, ok := o.ids[Normalize(id)]
	return ok
}

// Alias resolves a registered alias to the canonical ID it stands for.
func (o *IDs) Alias(id string) (string, bool) {
	canonical, ok := o.aliases[Normalize(id)]
	return canonical, ok
}

// HasDomain reports whether domain is the left half of some canonical ID.
func (o *IDs) HasDomain(domain string) bool {
	_, ok := o.domains[domain]
	return ok
}

// HasMember reports whether member is the right half of some canonical ID.
func (o *IDs) HasMember(member string) bool {
	_, ok := o.members[member]
	return ok
}

// Count is how many canonical IDs the catalog holds, which is what a report
// says it judged against.
func (o *IDs) Count() int { return len(o.ids) }

// AliasCount is how many registered aliases resolve to one of those IDs.
func (o *IDs) AliasCount() int { return len(o.aliases) }

// Aliases is every registered alias mapped to the canonical ID it stands for.
// The returned map is the one this holds: a caller reads it and does not write
// to it.
func (o *IDs) Aliases() map[string]string { return o.aliases }

// Sorted is the canonical ID set in order. The returned slice is the one this
// holds: a caller reads it and does not sort it in place.
func (o *IDs) Sorted() []string { return o.sorted }

// Candidates extracts the dotted tokens of a sentence that are offered as
// action IDs, deduplicated in the order they appear.
//
// A token qualifies when either half is one the catalog uses: a known domain,
// or a known action name. The reasoning is in the package comment, and it is
// the whole difference between a rule that sees work_item.get and one that
// reports go.mod.
func (o *IDs) Candidates(prose string) []string {
	var candidates []string
	seen := map[string]struct{}{}
	for _, token := range DottedToken.FindAllString(prose, -1) {
		domain, member, found := strings.Cut(token, ".")
		if !found || (!o.HasDomain(domain) && !o.HasMember(member)) {
			continue
		}
		if _, repeated := seen[token]; repeated {
			continue
		}
		seen[token] = struct{}{}
		candidates = append(candidates, token)
	}
	return candidates
}

// Closest names the nearest canonical ID, or nothing when the nearest is too
// far to be a lead. The bound is a third of the length: past that the nearest
// ID is an accident of the alphabet and naming it would send a reader off.
func (o *IDs) Closest(id string) string {
	best, bestDistance := "", 0
	bound := max(len(id)/3, 2)
	for _, candidate := range o.sorted {
		distance := editDistance(Normalize(id), candidate)
		if distance > bound {
			continue
		}
		if best == "" || distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// editDistance is the Levenshtein distance between two strings.
func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(left); i++ {
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 1
			if left[i-1] == right[j-1] {
				cost = 0
			}
			current[j] = min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}

// Normalize spells an ID the way the dynamic registry resolves one, so a
// judgement in a gate and a lookup at run time cannot disagree over case or
// spacing.
func Normalize(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
