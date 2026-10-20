package dynamic

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	searchFieldCanonicalID    = "canonical_id"
	searchFieldAlias          = "alias"
	searchFieldTag            = "tag"
	searchFieldDomain         = "domain"
	searchFieldAction         = "action"
	searchFieldRequiredParam  = "required_param"
	searchFieldOptionalParam  = "optional_param"
	searchFieldRefResource    = "ref_resource"
	searchFieldSchemaProperty = "schema_property"
	searchFieldSchemaEnum     = "schema_enum"
	searchFieldSchemaDesc     = "schema_description"
	searchFieldSpecificity    = "action_specificity"
	searchFieldCompareIntent  = "compare_refs_intent"
	searchFieldPluralList     = "plural_list_intent"
	searchFieldDiscoverIntent = "discover_project_intent"
	searchFieldProjectIntent  = "project_get_intent"
	searchFieldSearchIntent   = "search_projects_intent"
	searchFieldServiceAccount = "service_account_intent"
	searchFieldScopeIntent    = "scope_intent"
	searchFieldTool           = "tool"
	searchFieldVerbIntent     = "verb_intent"
	searchFieldIDContains     = "id_contains"
	searchFieldDomainContains = "domain_contains"
	searchFieldActionContains = "action_contains"
	searchFieldFlatText       = "flat_text"
	searchFieldFuzzyToken     = "fuzzy_token"
)

// Scoring balances precision first, then recovery. Exact identifier matches
// (scoreCanonicalExact, scoreAliasExact, scoreTagExact) intentionally outrank
// domain/action and schema-field heuristics so canonical IDs stay stable at the
// top. minimumHighConfidenceScore and minimumHighConfidenceMargin gate when the
// best result is considered trustworthy, while scoreVerbIntentBoost,
// scoreVerbIntentPenalty, scoreRequiredParamBoost, and scoreCompoundTagBoost are
// tuning knobs for intent disambiguation without overriding exact matches.
const (
	scoreCanonicalExact       = 120
	scoreAliasExact           = 100
	scoreTagExact             = 90
	scoreDomainActionExact    = 80
	scoreDomainActionWord     = 65
	scoreIDContains           = 55
	scoreDomainActionContains = 45
	scoreRequiredParamMatch   = 35
	scoreOptionalParamMatch   = 22
	scoreSchemaEnumMatch      = 28
	scoreSchemaDescMatch      = 12
	scoreFieldContains        = 25
	scoreSynonymContains      = 18
	// scoreUnmatchedActionWordPenalty is taken off for each word of the
	// action name the query does not spell. It is written as the size of the
	// penalty and negated where it applies, as scoreVerbIntentPenalty is, so
	// that the sign is an operator in a statement the tests cover rather than
	// in a constant the mutation gate cannot see.
	scoreUnmatchedActionWordPenalty = 60
	// scoreSingularExact and scoreSingularWord are what the singular of a
	// plural scores against the action ID ([singularMatch]): one step below
	// the singular's own match, an exact domain or action as a word of one
	// and a word as a partial match, so that the spelling the caller wrote
	// scores above the one the search derived from it.
	scoreSingularExact = scoreDomainActionWord
	scoreSingularWord  = scoreDomainActionContains
	// scoreRefResource is what a branch, tag or commit named by a query that
	// says what to do scores on an action whose ref is required
	// ([refResourceMatch]): a word of a domain, one step below the action's
	// own domain, so that the resource the query acts on ranks above the one
	// it starts from. scoreRefResourcePlural is the same word written in the
	// plural, a step lower again, as the singular of a plural scores on the
	// action ID.
	scoreRefResource       = scoreDomainActionWord
	scoreRefResourcePlural = scoreSingularWord
)

const (
	minimumHighConfidenceScore  = 80
	minimumHighConfidenceMargin = 15
	scoreVerbIntentBoost        = 16
	// scoreVerbIntentPenalty is taken off an action that answers the query's
	// verb with the opposite one, written as its size and negated where it
	// applies.
	scoreVerbIntentPenalty      = 24
	scoreRequiredParamBoost     = 10
	scoreCompoundTagBoost       = 50
	scoreCompareRefsIntentBoost = 90
	scoreDiscoverIntentBoost    = 95
	scoreProjectGetIntentBoost  = 220
	scoreSearchProjectsBoost    = 90
	// scoreSearchCodeIntentBoost and scoreCurrentUserIntentBoost are intentionally
	// large. Both actions pass the match-ratio filter via an intent bypass (they
	// match only 2 of ~10 tokens in long queries), so after scaling their base
	// score is ~26. Competing actions such as search.projects reach ~980 through
	// high match counts and stacked boosts. The boost must exceed that deficit to
	// guarantee the correct action wins.
	scoreSearchCodeIntentBoost  = 1000
	scoreCurrentUserIntentBoost = 1000
	scoreServiceAccountBoost    = 80
	scoreServiceAccountScope    = 30
	scoreScopeIntentBoost       = 80
	// scorePluralListIntentBoost has to carry a list action past a read of one
	// item of the same domain that an incidental parameter signal lifts by up
	// to ten, and then by the high-confidence margin of fifteen, so that a
	// plural query is answered with confidence rather than by a few points.
	scorePluralListIntentBoost = 30
)

type verbIntent string

const (
	verbIntentRead        verbIntent = "read"
	verbIntentWrite       verbIntent = "write"
	verbIntentDestructive verbIntent = "destructive"
	verbIntentWorkflow    verbIntent = "workflow"
	verbIntentDiagnostic  verbIntent = "diagnostic"
)

type searchDocument struct {
	Backend          string
	Capability       string
	Resource         string
	Operation        string
	Scope            string
	CanonicalID      string
	IDWords          []string
	Tool             string
	Domain           string
	DomainWords      []string
	Action           string
	ActionWords      []string
	Aliases          []string
	Tags             []string
	RequiredParams   []string
	OptionalParams   []string
	SchemaProperties []string
	SchemaEnums      []string
	SchemaDescTerms  []string
	FlatText         string
	// WordizedValues maps each searchable multi-word value above to its
	// precomputed space-joined word form (see wordizeSearchDocument).
	WordizedValues map[string]string
}

type searchIndex struct {
	byToken  map[string][]int
	byAlias  map[string][]int
	byDomain map[string][]int
	byAction map[string][]int
	all      []int
}

// MatchReason explains one query-term match that contributed to an action score.
type MatchReason struct {
	Field        string `json:"field" jsonschema:"Action metadata field that matched the query term."`
	QueryTerm    string `json:"query_term" jsonschema:"Original normalized query term."`
	MatchedValue string `json:"matched_value" jsonschema:"Action metadata value that matched."`
	Alternative  string `json:"alternative,omitempty" jsonschema:"Expanded synonym or verb alternative that matched, when different from the original term."`
	Score        int    `json:"score" jsonschema:"Score contributed by this match before final match-ratio scaling."`
	Fuzzy        bool   `json:"fuzzy,omitempty" jsonschema:"Whether this match came from typo-tolerant fuzzy recovery."`
	Distance     int    `json:"distance,omitempty" jsonschema:"Levenshtein edit distance for fuzzy matches."`
}

// ScoringExplanation describes how a dynamic search result received its score.
type ScoringExplanation struct {
	TotalScore    int           `json:"total_score" jsonschema:"Final score returned for the action after match-ratio scaling."`
	MatchedTerms  int           `json:"matched_terms" jsonschema:"Number of normalized query terms that matched this action."`
	RequiredTerms int           `json:"required_terms" jsonschema:"Minimum number of normalized query terms required for this action to match."`
	LowConfidence bool          `json:"low_confidence,omitempty" jsonschema:"Whether the result is considered low confidence."`
	MarginToNext  int           `json:"margin_to_next,omitempty" jsonschema:"Score difference to the next ranked result when known."`
	Reasons       []MatchReason `json:"reasons,omitempty" jsonschema:"Deterministic match reasons that contributed to the score."`
}

// explanationSummary renders one scoring explanation as a table cell.
//
// The matched value it quotes is catalog text, but not always an identifier:
// on the schema-description branch it is a whole property description reduced
// to its words, and those are ordinary English. Two in the served catalog
// carry the characters at issue today, "Order by: id | name | username" and
// "(GitLab < 14 only)". This used to fold the pipe and the newline through a
// local helper and leave the angle bracket alone, which is half of the rule
// the shared escaper is, and a package-local half of a rule is how the next
// reader learns the wrong one.
func explanationSummary(explanation *ScoringExplanation) string {
	if explanation == nil || len(explanation.Reasons) == 0 {
		return "-"
	}
	reason := explanation.Reasons[0]
	matched := reason.MatchedValue
	if matched == "" {
		matched = reason.Alternative
	}
	if matched == "" {
		matched = reason.QueryTerm
	}
	text := fmt.Sprintf("%s matched %q", reason.Field, matched)
	if reason.Fuzzy {
		text = fmt.Sprintf("%s fuzzy-matched %q", reason.Field, matched)
	}
	return toolutil.EscapeMdTableCell(text)
}

func hasSearchExplanations(results []SearchResult) bool {
	for _, result := range results {
		if result.Explanation != nil {
			return true
		}
	}
	return false
}

func hasFindExplanations(results []FindResult) bool {
	for _, result := range results {
		if result.Explanation != nil {
			return true
		}
	}
	return false
}

// ambiguousTargetsFromSearchResults returns the deduplicated ambiguity set from
// the first result that carries AmbiguousWith. Search ambiguity is modeled as a
// single canonical set, so later rows are intentionally ignored.
func ambiguousTargetsFromSearchResults(results []SearchResult) []string {
	for _, result := range results {
		if len(result.AmbiguousWith) > 0 {
			return dedupeSortedStrings(result.AmbiguousWith)
		}
	}
	return nil
}
