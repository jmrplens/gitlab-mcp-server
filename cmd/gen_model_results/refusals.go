// refusals.go is what may not be published, as a table.
//
// It is a table rather than a chain of conditions in the fold for the reason
// every declaration table in this repository is one: a rule with a name and a
// reason can be read, reported by name and tested one at a time, and a rule
// nothing can trip is itself a finding. The tables this replaces had no such
// list at all, which is how a run whose prompts contained the answer, and a run
// against a tree nobody could check out, both came to be published beside
// figures that read like measurements.
//
// Every rule here is a question about the configuration or about the record.
// None is a question about how well the model did: a model that did badly is a
// row, and refusing it would be publishing only the runs that flattered us.

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
)

// rule is one reason a row may not be published.
type rule struct {
	// Name is what a refusal is reported as, and what the staleness test
	// pairs a fixture with.
	Name string
	// Why says what publishing such a row would claim, which is the half a
	// reader needs and a condition cannot carry.
	Why string
	// refuse returns the reason this rule refuses the candidate, and the empty
	// string when it does not apply.
	refuse func(cand candidate) string
}

// refusal is one row that will not be published, and why.
type refusal struct {
	// Key names the row, so a maintainer can find what was dropped.
	Key string
	// Rule is the rule that refused it and Reason what that rule found.
	Rule   string
	Reason string
}

// String renders a refusal as a run reports it, one line per row.
func (r refusal) String() string {
	return fmt.Sprintf("refused [%s] %s: %s", r.Rule, r.Key, r.Reason)
}

// requiredProvenance are the provenance fields every row must carry, each with
// what a reader loses when it is empty.
//
// Three fields of the provenance are deliberately not here, because each is
// empty for a configuration rather than for a hole: a tier pin is empty when
// the run pinned nothing, a meta schema mode is empty off the meta surface, and
// a slice size is empty off the individual surface. Refusing those would refuse
// every dynamic row ever measured.
var requiredProvenance = []struct {
	Name  string
	Why   string
	Empty func(p provenance) bool
}{
	{"commit", "nothing says which tree was measured", func(p provenance) bool { return p.Commit == "" }},
	{"date", "nothing says when", func(p provenance) bool { return p.Date == "" }},
	{"gitlab_version", "nothing says which GitLab answered", func(p provenance) bool { return p.GitLabVersion == "" }},
	{"edition", "nothing says whether the instance was licensed", func(p provenance) bool { return p.Edition == "" }},
	{"tier", "nothing says which catalog was served", func(p provenance) bool { return p.Tier == "" }},
	{"surface", "nothing says which tool surface the model was given", func(p provenance) bool { return p.Surface == "" }},
	{"mode", "nothing says whether mutations were withheld or previewed", func(p provenance) bool { return p.Mode == "" }},
	{"capability_surface", "nothing says which resources and prompts were served", func(p provenance) bool { return p.CapabilitySurface == "" }},
	{"token_scopes", "nothing says what the credential could reach", func(p provenance) bool { return len(p.TokenScopes) == 0 }},
	{"served_tools", "nothing says how large a tool list the model read", func(p provenance) bool { return p.ServedTools <= 0 }},
	{"provider", "nothing says who was asked", func(p provenance) bool { return p.Provider == "" }},
	{"model", "nothing says which model", func(p provenance) bool { return p.Model == "" }},
	{"request_options", "nothing says how it was asked", func(p provenance) bool { return len(p.RequestOptions) == 0 }},
	{"repeat", "nothing says how many times each attempt was made", func(p provenance) bool { return p.Repeat <= 0 }},
	{"corpus_digest", "nothing fingerprints what was asked", func(p provenance) bool { return p.CorpusDigest == "" }},
	{"contract_digest", "nothing fingerprints how it was framed", func(p provenance) bool { return p.ContractDigest == "" }},
	{
		"tool_schema_digest",
		"nothing fingerprints the tool list this provider was served, and two rows whose runs both failed to note one " +
			"agree on the empty string, which seats them in one cross-vendor table captioned with tool schemas neither recorded",
		func(p provenance) bool { return p.ToolSchemaDigest == "" },
	},
}

// rules are the refusals of section 4.7 of the rebuild plan, in the order a
// reader meets them: what the run was, what the session saw, who answered, what
// the row says about itself, what it was scored against, and what the record
// already holds.
// fakeProviderRule names the one rule a dry run sets aside.
//
// It is named rather than matched by string at the call site so that renaming
// the rule cannot silently turn the dry run into a run that publishes a fake
// into the committed record.
const fakeProviderRule = "fake-provider"

var rules = []rule{
	{
		Name: "filtered-run",
		Why:  "a partial corpus is not a claim about the rest, which is the rule the end-to-end coverage record is held to as well",
		refuse: func(cand candidate) string {
			if cand.run.Filter == "" {
				return ""
			}
			return fmt.Sprintf("the run was filtered with -run %q, so it put part of the corpus and the row would read as the whole of it",
				cand.run.Filter)
		},
	},
	{
		Name: "unobserved-session",
		Why:  "every verdict of a session whose spans never arrived is a claim about what the model asked for rather than about what the server ran",
		refuse: func(cand candidate) string {
			if cand.sessionCalls == 0 || cand.sessionObserved > 0 {
				return ""
			}
			return fmt.Sprintf("session %q made %d call(s) and the server's span described none of them",
				cand.session.Label, cand.sessionCalls)
		},
	},
	{
		Name: fakeProviderRule,
		Why:  "the fake replays the corpus key to prove the pipe, so its figures are a reading of this repository's own answer and not of a model",
		refuse: func(cand candidate) string {
			if cand.provider.Name != fakeProvider {
				return ""
			}
			return fmt.Sprintf("%q is the fake provider, which answers from the corpus key", cand.key.Model)
		},
	},
	{
		Name: "incomplete-provenance",
		Why:  "a figure whose configuration is not fully written down cannot be compared with another figure, which is the only thing a published row is for",
		refuse: func(cand candidate) string {
			if !cand.providerKnown {
				return fmt.Sprintf("the run line describes no provider for model %q, so the row carries neither a provider nor the options it was asked with",
					cand.key.Model)
			}
			return missingProvenance(provenanceOf(cand))
		},
	},
	{
		Name: "stale-corpus",
		Why:  "a row's columns are this tree's scoring of the run, so a corpus that has moved since leaves figures answering a question the corpus no longer asks",
		refuse: func(cand candidate) string {
			if cand.run.CorpusDigest == modelcorpus.Digest() {
				return ""
			}
			return fmt.Sprintf("the run was put the corpus %s and the corpus at HEAD is %s",
				cand.run.CorpusDigest, modelcorpus.Digest())
		},
	},
	{
		Name: "duplicate-row",
		Why:  "two measurements of one configuration are two runs, and silently keeping the second would publish the luckier of them",
		refuse: func(cand candidate) string {
			if cand.claimedBy == "" {
				return ""
			}
			return "a row with this key already stands in " + cand.claimedBy
		},
	},
}

// missingProvenance names the first provenance field a row has a hole in.
//
// The first and not all of them, because a row with one hole and a row with six
// are equally unpublishable and a reader fixing the first will see the rest on
// the next run; naming them all would make the common case, a run recorded
// before an environment variable was set, a paragraph.
func missingProvenance(p provenance) string {
	for _, field := range requiredProvenance {
		if field.Empty(p) {
			return fmt.Sprintf("the provenance field %q is empty, so %s", field.Name, field.Why)
		}
	}
	return ""
}

// judge applies every rule to every candidate, in the order the rules are
// declared, and returns the rows that survive and the refusals.
//
// One rule per row and not all of them: a row refused for being the fake's is
// refused, and going on to say that its corpus has also moved is noise. The
// order of the table is therefore the order a reader is told about, which is
// why it runs from what the run was to what the record already holds.
// admitFake sets aside the one rule a dry run must: the fake exists to prove
// the pipe, and a dry run is the pipe being proved. Every other rule still
// runs, and the committed record is never what a dry run writes.
func judge(candidates []candidate, keys map[string]modelcorpus.Key, admitFake bool) ([]row, []refusal, error) {
	var (
		rows     []row
		refusals []refusal
	)
	for _, cand := range candidates {
		if found, refused := refuseCandidate(cand, admitFake); refused {
			refusals = append(refusals, found)
			continue
		}
		one, err := score(cand, keys)
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, one)
	}
	return rows, refusals, nil
}

// refuseCandidate asks every rule about one candidate and returns the first
// refusal.
func refuseCandidate(cand candidate, admitFake bool) (refusal, bool) {
	for _, one := range rules {
		if admitFake && one.Name == fakeProviderRule {
			continue
		}
		if reason := one.refuse(cand); reason != "" {
			return refusal{Key: cand.key.String(), Rule: one.Name, Reason: reason}, true
		}
	}
	return refusal{}, false
}

// reviewRows re-applies the rules to a record that is already committed, which
// is what makes the gate more than a comparison of bytes.
//
// A row published when a rule did not exist, or before the corpus moved under
// it, is refused here rather than left standing. Nothing redraws its figures:
// they were computed once, when its run was folded in, so a row the rules would
// refuse today is a row whose figures no longer describe anything, and the only
// answer is to re-fold its shards or drop it.
//
// It builds a candidate out of the committed row rather than out of a shard,
// which is what lets one table serve both directions. The three facts a
// committed row cannot carry are supplied as what a published row must have
// been: its session was observed, its provider was described, and nothing had
// claimed its key when it was written.
func reviewRows(doc document) []refusal {
	var refusals []refusal
	seen := map[string]string{}
	for _, one := range doc.Rows {
		cand := candidate{
			key:             one.Key,
			shard:           recordRelPath,
			run:             runOf(one),
			session:         sessionOf(one),
			provider:        providerOf(one),
			providerKnown:   true,
			sessionCalls:    1,
			sessionObserved: 1,
			claimedBy:       seen[one.Key.String()],
		}
		if found, refused := refuseCandidate(cand, false); refused {
			refusals = append(refusals, found)
		}
		if _, taken := seen[one.Key.String()]; !taken {
			seen[one.Key.String()] = recordRelPath
		}
	}
	return refusals
}

// comparisonSummary says what the record can be read as: how many rows, how
// many cross-vendor tables and how many cross-surface comparisons.
//
// It is printed rather than committed, because it is a property of the record
// and not a measurement: a reader who has just folded a run in wants to know
// whether the rows they added can be compared with anything.
func comparisonSummary(rows []row) string {
	vendors := len(groupByVendor(rows))
	surfaces := 0
	for _, group := range groupBySurface(rows) {
		if group.surfaces() > 1 {
			surfaces++
		}
	}
	return fmt.Sprintf("%d row(s), %d cross-vendor table(s), %d cross-surface comparison(s)",
		len(rows), vendors, surfaces)
}

// ruleNames spells the declared rules, for a report that says what judged a
// record.
func ruleNames() string {
	names := make([]string, 0, len(rules))
	for _, one := range rules {
		names = append(names, one.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
