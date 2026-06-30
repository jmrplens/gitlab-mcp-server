// Command audit_edition_tier reports the doc-grounded licensing tier of every
// canonical MCP action and compares it to the action's current gating.
//
// GitLab marks the required licensing tier per API endpoint in its
// documentation: a page-level `{{< details >}}` block carries a default
// `- Tier:` badge, and individual endpoint sections may override it with their
// own details block. The values seen are "Free, Premium, Ultimate" (all tiers),
// "Premium, Ultimate" (Premium minimum) and "Ultimate" (Ultimate only).
//
// The current MCP gating is binary and positional: an action is either present
// in the Community Edition catalog (effectively Free) or only in the Enterprise
// catalog (an undifferentiated Premium-or-Ultimate). This auditor surfaces, per
// owner domain, the doc tier distribution against that current gating so the
// per-action tier-assignment waves can be planned and later verified.
//
// Usage:
//
//	go run ./cmd/audit_edition_tier/                 # full report to stdout
//	go run ./cmd/audit_edition_tier/ -gaps-only      # only domains needing work
//	go run ./cmd/audit_edition_tier/ -offline        # use only cached docs
//	go run ./cmd/audit_edition_tier/ -output dist/edition-tier.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

const schemaVersion = 1

// tier is the canonical 3-tier licensing level, mirroring internal/edition.Tier
// but kept local so the auditor has no dependency on runtime config.
type tier int

const (
	tierFree tier = iota
	tierPremium
	tierUltimate
)

func (t tier) String() string {
	switch t {
	case tierPremium:
		return "premium"
	case tierUltimate:
		return "ultimate"
	default:
		return "free"
	}
}

// parseEditionTier maps an action's Edition metadata string to its tier.
// Empty/"core"/unknown is Free; "premium" and "ultimate" map to their tiers.
func parseEditionTier(edition string) tier {
	switch strings.ToLower(strings.TrimSpace(edition)) {
	case "premium":
		return tierPremium
	case "ultimate":
		return tierUltimate
	default:
		return tierFree
	}
}

// parseTierBadge maps a doc `- Tier:` badge value to the minimum required tier.
// The badge lists the tiers an endpoint is available in; the minimum is the
// lowest listed tier.
//
//   - contains "Free"     → free
//   - contains "Premium"  → premium
//   - only "Ultimate"     → ultimate
func parseTierBadge(value string) (tier, bool) {
	v := strings.ToLower(value)
	switch {
	case strings.Contains(v, "free"):
		return tierFree, true
	case strings.Contains(v, "premium"):
		return tierPremium, true
	case strings.Contains(v, "ultimate"):
		return tierUltimate, true
	default:
		return tierFree, false
	}
}

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include domains that need tier work")
	offline := flag.Bool("offline", false, "use only cached docs; do not fetch")
	refresh := flag.Bool("refresh", false, "force re-fetch docs even when cached and fresh")
	maxAge := flag.Duration("max-age", apidocs.DefaultMaxAge, "re-download cached docs older than this (default 7 days)")
	flag.Parse()

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}

	fetcher := apidocs.New(root, apidocs.Options{Refresh: *refresh, Offline: *offline, MaxAge: *maxAge})
	rep, err := buildReport(fetcher)
	if err != nil {
		cmdutil.Fatalf("build edition tier report: %v", err)
	}

	if *gapsOnly {
		filtered := rep.Domains[:0]
		for _, d := range rep.Domains {
			if d.NeedsWork {
				filtered = append(filtered, d)
			}
		}
		rep.Domains = filtered
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		cmdutil.Fatalf("marshal report: %v", err)
	}
	if *outputPath == "-" {
		fmt.Println(string(data))
		return
	}
	if writeErr := os.WriteFile(*outputPath, append(data, '\n'), 0o600); writeErr != nil {
		cmdutil.Fatalf("write report: %v", writeErr)
	}
}

// report is the top-level JSON output.
type report struct {
	SchemaVersion   int            `json:"schema_version"`
	GeneratedAt     string         `json:"generated_at"`
	Summary         reportSummary  `json:"summary"`
	UnmappedDomains []string       `json:"unmapped_domains,omitempty"`
	Domains         []domainReport `json:"domains"`
}

type reportSummary struct {
	Actions           int            `json:"actions"`
	Domains           int            `json:"domains"`
	DomainsNeedWork   int            `json:"domains_need_work"`
	CurrentFree       int            `json:"current_free"`
	CurrentEnterprise int            `json:"current_enterprise"`
	TierMismatches    int            `json:"tier_mismatches"`
	Classification    map[string]int `json:"classification"`
}

// domainReport summarizes one owner domain (one doc area).
type domainReport struct {
	Domain            string         `json:"domain"`
	DocArea           string         `json:"doc_area"`
	Classification    string         `json:"classification"`
	NeedsWork         bool           `json:"needs_work"`
	PageTier          string         `json:"page_tier"`
	OverrideTiers     []string       `json:"override_tiers,omitempty"`
	Actions           int            `json:"actions"`
	CurrentFree       int            `json:"current_free"`
	CurrentEnterprise int            `json:"current_enterprise"`
	Mismatches        int            `json:"mismatches"`
	DocFetched        bool           `json:"doc_fetched"`
	Note              string         `json:"note,omitempty"`
	ActionDetails     []actionDetail `json:"action_details,omitempty"`
}

type actionDetail struct {
	ID          string `json:"id"`
	OwnerPkg    string `json:"owner_pkg"`
	CurrentGate string `json:"current_gate"` // free | enterprise
	Edition     string `json:"edition"`      // current Edition field (may be empty)
	Expected    string `json:"expected"`     // expected min tier (free/premium/ultimate)
	Mismatch    bool   `json:"mismatch,omitempty"`
	Note        string `json:"note,omitempty"` // exception rationale, when applicable
}

// expectedTierForAction returns the doc-grounded minimum tier expected for an
// action and a rationale note. An accepted exception (an action whose true doc
// page differs from its owner domain's page, or a per-section override) wins;
// otherwise the domain page tier applies.
func expectedTierForAction(id string, pageTier tier) (expected tier, reason string) {
	if ex, ok := acceptedTierExceptions[id]; ok {
		return ex.tier, ex.reason
	}
	return pageTier, ""
}

func buildReport(fetcher *apidocs.Fetcher) (*report, error) {
	// Mechanical current-state: diff the CE and EE catalogs to learn each
	// action's current binary gate.
	ceCatalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: false, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build CE catalog: %w", err)
	}
	eeCatalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build EE catalog: %w", err)
	}
	ceIDs := make(map[string]struct{})
	for _, a := range ceCatalog.Actions() {
		ceIDs[string(a.ID)] = struct{}{}
	}

	// Group EE actions by owner package (the domain key for doc mapping).
	type domainAcc struct {
		actions []actionDetail
	}
	domains := map[string]*domainAcc{}
	var totalActions int
	unmapped := map[string]struct{}{}
	for _, a := range eeCatalog.Actions() {
		totalActions++
		pkg := a.OwnerPackage
		if pkg == "" {
			pkg = a.Domain
		}
		_, inCE := ceIDs[string(a.ID)]
		gate := "enterprise"
		if inCE {
			gate = "free"
		}
		d, ok := domains[pkg]
		if !ok {
			d = &domainAcc{}
			domains[pkg] = d
		}
		d.actions = append(d.actions, actionDetail{
			ID:          string(a.ID),
			OwnerPkg:    pkg,
			CurrentGate: gate,
			Edition:     a.Edition,
		})
	}

	rep := &report{
		SchemaVersion: schemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Summary:       reportSummary{Classification: map[string]int{}},
	}

	pkgNames := make([]string, 0, len(domains))
	for k := range domains {
		pkgNames = append(pkgNames, k)
	}
	sort.Strings(pkgNames)

	for _, pkg := range pkgNames {
		dr := buildDomainReport(pkg, domains[pkg].actions, fetcher)
		if !dr.DocFetched && dr.Note == "no doc-area mapping" {
			unmapped[pkg] = struct{}{}
		}
		rep.Summary.Actions += dr.Actions
		rep.Summary.CurrentFree += dr.CurrentFree
		rep.Summary.CurrentEnterprise += dr.CurrentEnterprise
		rep.Summary.Classification[dr.Classification]++
		rep.Summary.TierMismatches += dr.Mismatches
		if dr.NeedsWork {
			rep.Summary.DomainsNeedWork++
		}
		rep.Domains = append(rep.Domains, dr)
	}
	rep.Summary.Domains = len(rep.Domains)
	for k := range unmapped {
		rep.UnmappedDomains = append(rep.UnmappedDomains, k)
	}
	sort.Strings(rep.UnmappedDomains)
	_ = totalActions
	return rep, nil
}

// buildDomainReport assembles the report for one owner package: it resolves the
// doc area, fetches and parses the tier badges, tallies the current gating, and
// classifies the domain for wave planning.
func buildDomainReport(pkg string, actions []actionDetail, fetcher *apidocs.Fetcher) domainReport {
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })

	docArea, mapped := docAreaForPackage(pkg)
	pageTier := tierFree
	var overrideTiers []tier
	docFetched := false
	note := ""
	switch {
	case !mapped:
		note = "no doc-area mapping"
	default:
		content, ferr := fetcher.Fetch(docArea)
		if ferr != nil {
			note = "doc fetch failed: " + ferr.Error()
		} else {
			docFetched = true
			pageTier, overrideTiers = parseDocTiers(content)
		}
	}

	dr := domainReport{
		Domain:        pkg,
		DocArea:       docArea,
		PageTier:      pageTier.String(),
		DocFetched:    docFetched,
		Note:          note,
		Actions:       len(actions),
		ActionDetails: actions,
	}
	for _, ot := range overrideTiers {
		dr.OverrideTiers = append(dr.OverrideTiers, ot.String())
	}
	for i := range dr.ActionDetails {
		a := &dr.ActionDetails[i]
		if a.CurrentGate == "free" {
			dr.CurrentFree++
		} else {
			dr.CurrentEnterprise++
		}
		if !docFetched {
			continue
		}
		exp, reason := expectedTierForAction(a.ID, pageTier)
		a.Expected = exp.String()
		a.Note = reason
		if edition := parseEditionTier(a.Edition); edition != exp {
			a.Mismatch = true
			dr.Mismatches++
		}
	}
	dr.Classification, dr.NeedsWork = classifyDomain(dr, overrideTiers, pageTier, docFetched)
	return dr
}

// classifyDomain assigns a wave classification and whether the domain needs
// per-action tier work.
//
//   - unmapped      : no doc-area mapping yet (manual triage)
//   - green         : page Free, no higher-tier overrides, all actions already CE
//   - uniform-ee    : single non-Free tier across the whole domain
//   - mixed         : doc has multiple tiers (needs careful per-action mapping)
//   - review        : doc tier and current gating disagree at the domain level
func classifyDomain(dr domainReport, overrides []tier, page tier, fetched bool) (string, bool) {
	if !fetched {
		return "unmapped", true
	}
	hasHigherOverride := false
	allOverridesEqualPage := true
	for _, o := range overrides {
		if o != page {
			allOverridesEqualPage = false
		}
		if o > page {
			hasHigherOverride = true
		}
	}
	switch {
	case page == tierFree && !hasHigherOverride:
		// Whole domain is Free. Needs work only if something is currently gated EE.
		return "green", dr.CurrentEnterprise > 0
	case len(overrides) == 0 || allOverridesEqualPage:
		// Uniform non-Free domain. Needs work to split premium vs ultimate and/or
		// to set the Tier field where currently undifferentiated.
		return "uniform-ee", true
	default:
		return "mixed", true
	}
}

// parseDocTiers extracts the page-level tier and the set of distinct
// per-section override tiers from a GitLab API doc.
//
// A `{{< details >}}` block contains lines like `- Tier: Premium, Ultimate`.
// The first such block (before any `## ` heading) is the page default; later
// blocks attached to endpoint sections are overrides.
func parseDocTiers(content string) (page tier, overrides []tier) {
	page = tierFree
	pageSet := false
	seen := map[tier]struct{}{}
	lines := strings.Split(content, "\n")
	sawHeading := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			sawHeading = true
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "- Tier:"); ok {
			value := strings.TrimSpace(rest)
			t, valid := parseTierBadge(value)
			if !valid {
				continue
			}
			if !sawHeading && !pageSet {
				page = t
				pageSet = true
				continue
			}
			if _, dup := seen[t]; !dup {
				seen[t] = struct{}{}
				overrides = append(overrides, t)
			}
		}
	}
	slices.Sort(overrides)
	return page, overrides
}
