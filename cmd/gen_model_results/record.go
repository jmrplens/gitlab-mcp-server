// record.go is the committed document: what one published row is, and the fold
// from the shards a run wrote to the rows a page is drawn from.
//
// A row is deliberately wide. Every figure it publishes is a numerator over a
// denominator, and beside them it carries the whole configuration that produced
// them, because the tables this replaces printed a percentage with nothing
// around it: a reader could not tell which surface, which schema mode, which
// tier or how many attempts were behind a number, and so could not tell which
// two numbers it was honest to compare. The comparison rules in compare.go are
// written against these fields, and they can only be written because the fields
// are here.

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore"
)

// The committed artifacts, relative to the repository root.
//
// The JSON is the record and the two Markdown files are renderings of it, which
// is why they are redrawn from the committed document rather than from a run: a
// reader on any checkout can redraw them, and a gate can compare the bytes.
const (
	recordRelPath = "docs/development/testing/model-results.json"
	pageRelPath   = "docs/development/testing/model-results.md"
	readmeRelPath = "README.md"
)

// regenerate is the sentence a stale artifact is reported with.
const regenerate = "make gen-model-results"

// recordSchemaVersion is the version of the committed document's shape.
//
// It is refused rather than read half-way when it is not this one, on the terms
// every record in this repository is held to: a document written by another
// version of this command may spell a field differently, and a gate that read
// what it recognized and ignored the rest would compare a page against a record
// it had only half understood.
const recordSchemaVersion = 1

// recordNote is what the committed document says to a reader who opened it
// first.
//
// It states the re-scoring contract in the terms the command really offers it,
// which is not the same as the terms the shard record offers: a shard carries
// no verdict and can be scored again by any rule, and the columns below are one
// such scoring, made when the run was folded in. Redrawing a page re-reads them.
const recordNote = "What the model evaluation measured, one row per configuration, written by " +
	"`make model-results-record` from the shards of a run and scored, at that moment, against the corpus at HEAD. " +
	"The columns here are that scoring: redrawing a page re-reads them and scores nothing, so a scoring rule " +
	"corrected later reaches a published row only by re-folding the run's retained shards with " +
	"`make model-results-refold`, which drops the rows those shards publish and folds them again, naming each. " +
	"The pages at README.md and docs/development/testing/model-results.md are rendered from this " +
	"file, and `make check-model-results` compares all three without a GitLab and without a network. " +
	"Every figure is a numerator over a denominator: a rate with nothing behind it is what the tables " +
	"this replaces published."

// fakeProvider is the adapter that talks to nobody.
//
// It is spelled here rather than imported because the adapters live behind the
// e2e build tag and this command is untagged. What it names is the string a
// run writes on its provider line, which is the only thing this side ever sees
// of it.
const fakeProvider = "fake"

// document is the committed record.
type document struct {
	// SchemaVersion is [recordSchemaVersion].
	SchemaVersion int `json:"schema_version"`
	// Note says what this file is.
	Note string `json:"note"`
	// Rows are the published rows, sorted by their key so a re-fold of the
	// same runs produces the same bytes.
	Rows []row `json:"rows"`
}

// row is one published measurement: one model, on one server configuration,
// against one corpus.
type row struct {
	// Key is what two rows must agree on to be the same row. A second row
	// carrying a key the record already holds is refused by name rather than
	// replacing it.
	Key rowKey `json:"key"`
	// Provenance is everything a reader needs to know what produced the
	// figures, written at run time from the harness and the binary.
	Provenance provenance `json:"provenance"`
	// Counts is the shape of the row: how many attempts ended each way, and
	// what was counted apart from every column.
	Counts counts `json:"counts"`
	// Columns are the seven published figures.
	Columns columns `json:"columns"`
	// Tokens is what the row cost, as four numbers. Never one: folding a cache
	// read into an input token is how a table came to claim sixty thousand
	// tokens against five million.
	Tokens tokens `json:"tokens"`
}

// rowKey is the full identity of a row.
//
// Everything in it changes what was measured rather than how well it went, so
// two rows differing anywhere here are two measurements and never two readings
// of one. The comparison keys in compare.go are each a subset of it, and which
// subset is the whole of the decision about which tables may be read together.
type rowKey struct {
	// Model is the provider specification as the run was configured with it.
	Model string `json:"model"`
	// Surface is the tool surface the session served.
	Surface string `json:"surface"`
	// Mode is the protective mode it served under.
	Mode string `json:"mode"`
	// Tier is the tier the instance resolved to, and TierPin the tier the run
	// forced, empty when it forced none. They are apart because a run pinned
	// to premium on an ultimate instance is a different catalog from an
	// unpinned run on a premium one, and a single field could not say which
	// happened.
	Tier    string `json:"tier"`
	TierPin string `json:"tier_pin,omitempty"`
	// MetaParamSchema is the meta surface's input-schema mode, empty off it. A
	// meta row measured in the default opaque mode learned parameter names
	// from refusals, which is what its overhead column is mostly made of.
	MetaParamSchema string `json:"meta_param_schema,omitempty"`
	// SliceSize is how many individual tools the session served, empty off the
	// individual surface, where the full list is over every provider's context
	// window and the slice is what was sent instead.
	SliceSize int `json:"slice_size,omitempty"`
	// CorpusDigest and ContractDigest fingerprint what was asked and how it
	// was framed.
	CorpusDigest   string `json:"corpus_digest"`
	ContractDigest string `json:"contract_digest"`
	// ToolSchemaDigest is the tool list as this provider received it. It is
	// per provider because a provider-specific rewrite of the schemas is
	// exactly what it catches: two adapters of the evaluator this replaces
	// injected parameter names into the execute schema, so two columns of a
	// published table were measuring a surface the other two never saw and
	// nothing on the row said so.
	ToolSchemaDigest string `json:"tool_schema_digest"`
	// Repeat is how many times each attempt was run.
	Repeat int `json:"repeat"`
}

// String renders the key as a row is named by, which is what a collision is
// refused by and what a table's caption holds fixed.
func (k rowKey) String() string {
	parts := []string{
		"model=" + k.Model,
		"surface=" + k.Surface,
		"mode=" + k.Mode,
		"tier=" + k.Tier,
	}
	if k.TierPin != "" {
		parts = append(parts, "tier-pin="+k.TierPin)
	}
	if k.MetaParamSchema != "" {
		parts = append(parts, "meta-schema="+k.MetaParamSchema)
	}
	if k.SliceSize > 0 {
		parts = append(parts, "slice="+strconv.Itoa(k.SliceSize))
	}
	parts = append(parts,
		"corpus="+k.CorpusDigest,
		"contract="+k.ContractDigest,
		"tools="+k.ToolSchemaDigest,
		"repeat="+strconv.Itoa(k.Repeat),
	)
	return strings.Join(parts, " ")
}

// provenance is what produced the figures.
//
// Every field of it is written at run time from the harness and the binary
// rather than from a flag somebody passed, which is the difference between a
// row that says what it measured and a row that says what its author believed
// it measured. A row with a hole in it is refused rather than published with
// the hole, because the reader who would be misled is the one who compares it
// with another row.
type provenance struct {
	// Commit is the revision the run was made on, and Date the day it started.
	Commit string `json:"commit"`
	Date   string `json:"date"`
	// GitLabVersion and Edition are the instance it ran against.
	GitLabVersion string `json:"gitlab_version"`
	Edition       string `json:"edition"`
	// Tier is what the instance's license resolved to, TierConfirmed whether
	// the resolution was confirmed rather than assumed, and TierPin the tier
	// the run forced on top of it.
	Tier          string `json:"tier"`
	TierConfirmed bool   `json:"tier_confirmed"`
	TierPin       string `json:"tier_pin,omitempty"`
	// Surface, Mode, CapabilitySurface, MetaParamSchema and SliceSize are the
	// server configuration the session served.
	Surface           string `json:"surface"`
	Mode              string `json:"mode"`
	CapabilitySurface string `json:"capability_surface"`
	MetaParamSchema   string `json:"meta_param_schema,omitempty"`
	SliceSize         int    `json:"slice_size,omitempty"`
	// TokenScopes are the run credential's own, which decide the catalog
	// before anything is registered, and ServedTools is what the session
	// listed.
	TokenScopes []string `json:"token_scopes"`
	ServedTools int      `json:"served_tools"`
	// Provider and Model name who was asked, and RequestOptions how. A model
	// that refuses a temperature reads "provider default" here and never "0":
	// they are different requests, and printing the second for the first
	// claims a determinism the run did not ask for.
	Provider       string            `json:"provider"`
	Model          string            `json:"model"`
	RequestOptions map[string]string `json:"request_options"`
	// Repeat is how many times each attempt was run.
	Repeat int `json:"repeat"`
	// CorpusDigest and ContractDigest fingerprint what was asked.
	CorpusDigest   string `json:"corpus_digest"`
	ContractDigest string `json:"contract_digest"`
	// ToolSchemaDigest is the tool list as this provider received it, and the
	// one fact of the key nothing above repeats.
	//
	// It is here so that the rule refusing a row with a hole in it can see it,
	// because an empty one is not harmless. A run that never noted a digest for
	// a model leaves the empty string, two such rows agree on the empty string,
	// and the cross-vendor key seats them in one table captioned with the tool
	// schemas neither of them recorded: exactly the defect the per-provider
	// digest exists to catch, reopened through the key that was meant to close
	// it.
	ToolSchemaDigest string `json:"tool_schema_digest"`
}

// counts is the shape of a row: how many attempts ended each way, and what was
// counted apart from every column and why.
type counts struct {
	// Attempts is how many attempts are behind the columns.
	Attempts int `json:"attempts"`
	// Skipped, Unobserved, ProviderErrors, HarnessErrors and GitLabRefused are
	// the five that are counted and then left out of every column. None of
	// them is the model's: the instance did not meet the case's needs, the
	// server's span never arrived, the provider would not answer, this side
	// broke, or GitLab refused a call the model dispatched correctly.
	Skipped        int `json:"skipped"`
	Unobserved     int `json:"unobserved"`
	ProviderErrors int `json:"provider_errors"`
	HarnessErrors  int `json:"harness_errors"`
	GitLabRefused  int `json:"gitlab_refused"`
	// Turns is how many provider requests the row paid for.
	Turns int `json:"turns"`
	// Outcomes is how many attempts ended each way, the five above included,
	// so a reader can see the shape of a row and not only its rates.
	Outcomes map[string]int `json:"outcomes"`
	// Declines is how many mutating steps were correctly declined each way,
	// apart because declining in text is what a read-only deployment wants and
	// being refused the action is only the conversation ending correctly.
	Declines map[string]int `json:"declines,omitempty"`
	// Confirmations is how many destructive steps carried their approval each
	// way.
	Confirmations map[string]int `json:"confirmations,omitempty"`
}

// ratio is one published column: what happened over what could have.
//
// Both numbers are kept and both are printed. A column reading 100% over one
// attempt and a column reading 100% over ninety looked the same in the tables
// this replaces, and a column with nothing behind it read as a perfect score.
type ratio struct {
	Numerator   int `json:"numerator"`
	Denominator int `json:"denominator"`
}

// String renders the column as a reader sees it, and as a dash when there was
// nothing to divide.
func (r ratio) String() string {
	if r.Denominator == 0 {
		return "-"
	}
	return strconv.Itoa(r.Numerator) + " / " + strconv.Itoa(r.Denominator)
}

// fromRatio converts the scorer's column into the committed one.
func fromRatio(r modelscore.Ratio) ratio {
	return ratio{Numerator: r.Numerator, Denominator: r.Denominator}
}

// overhead is what a model spent getting where it got, with its two halves
// apart: a discovery call is the dynamic surface's declared cost, and an
// invalid_params refusal is a model learning a parameter name from a rejection.
// One rate over both would hide each inside the other.
type overhead struct {
	Discovery     int `json:"discovery"`
	InvalidParams int `json:"invalid_params"`
	Steps         int `json:"steps"`
}

// Ratio folds the two halves into the published column.
func (o overhead) Ratio() ratio {
	return ratio{Numerator: o.Discovery + o.InvalidParams, Denominator: o.Steps}
}

// columns are the seven figures a row publishes, named as the verdict names
// them.
type columns struct {
	Reached           ratio    `json:"reached"`
	AcceptedFirstTime ratio    `json:"accepted_first_time"`
	ArgumentFidelity  ratio    `json:"argument_fidelity"`
	Confirmation      ratio    `json:"confirmation"`
	Unaided           ratio    `json:"unaided"`
	Completion        ratio    `json:"completion"`
	Overhead          overhead `json:"overhead"`
}

// tokens is what a row cost, as the four numbers a provider bills apart.
type tokens struct {
	Input        int `json:"input"`
	Output       int `json:"output"`
	CacheCreated int `json:"cache_created"`
	CacheRead    int `json:"cache_read"`
}

// candidate is one row's worth of a run, before the refusals have judged it.
//
// It is what a refusal rule is handed, which is why it carries the session's
// own observation facts and the key's claim beside the attempts: every rule in
// refusals.go is a question about the configuration or about the record, and
// none of them is a question about how well the model did.
type candidate struct {
	// key is the row this would become.
	key rowKey
	// shard is the file it was read from, which is what a collision names.
	shard string
	// run, session and provider are the three lines the row's provenance is
	// assembled from.
	run      modelrecord.Run
	session  modelrecord.Session
	provider modelrecord.Provider
	// providerKnown is whether the run line described the model at all. A run
	// that recorded attempts for a model it never described has no request
	// options and no provider name, which the provenance rule refuses.
	providerKnown bool
	// attempts are this row's attempts, in the order the run made them.
	attempts []modelscore.Attempt
	// sessionCalls and sessionObserved are the session's own, over every
	// attempt of the shard rather than over this row's, because a dispatch
	// being observed is a property of the receiver and not of one model.
	sessionCalls    int
	sessionObserved int
	// claimedBy says where a row with this key already stands, empty when
	// nothing claims it.
	claimedBy string
}

// fold groups a run's shards into the rows they would publish.
//
// The grouping is per shard, because the run line is per shard: no attempt,
// turn, call or verify line names its run, so joining shards before grouping
// would lose the commit, the instance and the tier every row has to carry.
//
// claimed is seeded with whatever the committed record already holds, so a
// second run of one configuration is reported by name instead of replacing the
// figures already published.
func fold(shards []modelrecord.Shard, claimed map[string]string) ([]candidate, error) {
	var candidates []candidate
	for _, shard := range shards {
		attempts, err := modelscore.Attempts(shard.Records)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", shard.Path, err)
		}
		candidates = append(candidates, groupShard(shard.Path, attempts, claimed)...)
	}
	return candidates, nil
}

// groupShard turns one shard's attempts into candidates, one per session and
// model.
func groupShard(shard string, attempts []modelscore.Attempt, claimed map[string]string) []candidate {
	var order []string
	byKey := map[string]*candidate{}
	observed := sessionObservation(attempts)

	for _, attempt := range attempts {
		one := attempt
		key := keyOf(one)
		name := key.String()
		found, seen := byKey[name]
		if !seen {
			line, known := providerLine(one.Run, one.Line.Model)
			found = &candidate{
				key:           key,
				shard:         shard,
				run:           one.Run,
				session:       one.Session,
				provider:      line,
				providerKnown: known,
				claimedBy:     claimed[name],
			}
			dispatches := observed[one.Line.Session]
			found.sessionCalls, found.sessionObserved = dispatches.calls, dispatches.observed
			byKey[name] = found
			order = append(order, name)
			if _, taken := claimed[name]; !taken {
				claimed[name] = shard
			}
		}
		found.attempts = append(found.attempts, one)
	}

	candidates := make([]candidate, 0, len(order))
	for _, name := range order {
		candidates = append(candidates, *byKey[name])
	}
	return candidates
}

// observation is how many calls a session made and how many of them the
// server's own span described.
type observation struct {
	calls    int
	observed int
}

// sessionObservation counts, per session, the calls made and the dispatches
// seen.
//
// It is per session and not per row because the receiver either joined a run's
// spans or did not: a model whose own attempts happened to make no call says
// nothing about whether the session was observed, and a row refused on that
// reading would be refused for somebody else's silence.
func sessionObservation(attempts []modelscore.Attempt) map[string]observation {
	seen := map[string]observation{}
	for _, attempt := range attempts {
		one := seen[attempt.Line.Session]
		for _, call := range attempt.Calls {
			one.calls++
			if call.DispatchObserved {
				one.observed++
			}
		}
		seen[attempt.Line.Session] = one
	}
	return seen
}

// keyOf reads a row's identity off the three lines that carry it.
//
// The surface is read the way the scorer reads it, and that is not a detail: a
// row keyed off the session line while the verdicts behind it were computed
// against the attempt line's surface would publish figures from one catalog
// under the name of another, and nothing in the record would say so.
func keyOf(attempt modelscore.Attempt) rowKey {
	return rowKey{
		Model:            attempt.Line.Model,
		Surface:          surfaceOf(attempt),
		Mode:             attempt.Session.Mode,
		Tier:             attempt.Run.Tier,
		TierPin:          attempt.Session.TierPin,
		MetaParamSchema:  attempt.Session.MetaParamSchema,
		SliceSize:        attempt.Session.SliceSize,
		CorpusDigest:     attempt.Run.CorpusDigest,
		ContractDigest:   attempt.Run.ContractDigest,
		ToolSchemaDigest: attempt.Session.ToolSchemaDigests[attempt.Line.Model],
		Repeat:           attempt.Run.Repeat,
	}
}

// surfaceOf reads the surface an attempt ran on, the attempt's own line first
// and the session's only when that line left it out. It mirrors the scorer,
// which is the whole of why it exists rather than being read inline.
func surfaceOf(attempt modelscore.Attempt) string {
	if attempt.Line.Surface != "" {
		return attempt.Line.Surface
	}
	return attempt.Session.Surface
}

// providerLine finds what the run line said about one model.
func providerLine(run modelrecord.Run, model string) (modelrecord.Provider, bool) {
	for _, line := range run.Providers {
		if line.Spec == model {
			return line, true
		}
	}
	return modelrecord.Provider{}, false
}

// score turns one candidate into the row it publishes.
//
// The answer key is read from the corpus at HEAD rather than from anything the
// run carried, so what a row publishes is this tree's scoring of a run that
// carried no verdict of its own. That is what makes a corrected scoring rule
// able to re-score a past run, and what it takes is the run's shards: the
// columns are computed once, here, and re-folding those shards through
// [foldInto] with refold set is the only way a corrected rule reaches a row
// already published. A candidate whose corpus digest has moved never reaches
// here: the refusal rules judge first, so a record is never scored against an
// answer it was not put to.
func score(cand candidate, keys map[string]modelcorpus.Key) (row, error) {
	verdicts := make([]modelscore.Verdict, 0, len(cand.attempts))
	for _, attempt := range cand.attempts {
		key, known := keys[attempt.Line.Case]
		if !known {
			return row{}, fmt.Errorf("attempt %q of %s names case %q, which the corpus does not have",
				attempt.Line.ID, cand.shard, attempt.Line.Case)
		}
		verdict, err := modelscore.Score(attempt, key)
		if err != nil {
			return row{}, fmt.Errorf("%s: %w", cand.shard, err)
		}
		verdicts = append(verdicts, verdict)
	}
	totals := modelscore.Aggregate(verdicts)
	return row{
		Key:        cand.key,
		Provenance: provenanceOf(cand),
		Counts:     countsOf(totals, cand.attempts),
		Columns:    columnsOf(totals),
		Tokens:     tokensOf(cand.attempts),
	}, nil
}

// provenanceOf assembles what produced the row.
//
// The tool-schema digest is read off the key rather than off the session line,
// which is the one field here taken from there. The session's map is keyed by
// provider and a committed row carries one provider, so a row read back from
// the record could not rebuild the map; taking it from the key is what lets the
// refusal table judge a shard and a committed row with the same rule.
func provenanceOf(cand candidate) provenance {
	return provenance{
		Commit:            cand.run.Commit,
		Date:              runDate(cand.run),
		GitLabVersion:     cand.run.GitLabVersion,
		Edition:           cand.run.Edition,
		Tier:              cand.run.Tier,
		TierConfirmed:     cand.run.TierConfirmed,
		TierPin:           cand.session.TierPin,
		Surface:           cand.session.Surface,
		Mode:              cand.session.Mode,
		CapabilitySurface: cand.session.Capabilities,
		MetaParamSchema:   cand.session.MetaParamSchema,
		SliceSize:         cand.session.SliceSize,
		TokenScopes:       cand.session.TokenScopes,
		ServedTools:       cand.session.ServedTools,
		Provider:          cand.provider.Name,
		Model:             cand.provider.Model,
		RequestOptions:    cand.provider.Options,
		Repeat:            cand.run.Repeat,
		CorpusDigest:      cand.run.CorpusDigest,
		ContractDigest:    cand.run.ContractDigest,
		ToolSchemaDigest:  cand.key.ToolSchemaDigest,
	}
}

// runDate is the day a run started, as a record spells a date.
//
// The zero time renders as nothing rather than as the year one: a run line
// without a start is a hole in the provenance, and the rule that refuses holes
// should see one rather than a date nobody could have measured on.
func runDate(run modelrecord.Run) string {
	if run.StartedAt.IsZero() {
		return ""
	}
	return run.StartedAt.UTC().Format(time.DateOnly)
}

// countsOf reads the shape of the row off the totals and the attempts.
func countsOf(totals modelscore.Totals, attempts []modelscore.Attempt) counts {
	turns := 0
	for _, attempt := range attempts {
		turns += len(attempt.Turns)
	}
	return counts{
		Attempts:       totals.Attempts,
		Skipped:        totals.Skipped,
		Unobserved:     totals.Unobserved,
		ProviderErrors: totals.ProviderErrors,
		HarnessErrors:  totals.HarnessErrors,
		GitLabRefused:  totals.GitLabRefused,
		Turns:          turns,
		Outcomes:       namedCounts(totals.Outcomes),
		Declines:       namedCounts(totals.Declines),
		Confirmations:  namedCounts(totals.Confirmations),
	}
}

// namedCounts renders one of the scorer's tallies as the record spells it.
//
// An empty tally is nil rather than an empty object, so a row that had no
// destructive step publishes no confirmation tally at all instead of an empty
// one a reader would have to interpret.
func namedCounts[K ~string](tally map[K]int) map[string]int {
	if len(tally) == 0 {
		return nil
	}
	named := make(map[string]int, len(tally))
	for name, count := range tally {
		named[string(name)] = count
	}
	return named
}

// columnsOf reads the seven published figures off the totals.
func columnsOf(totals modelscore.Totals) columns {
	return columns{
		Reached:           fromRatio(totals.Reached),
		AcceptedFirstTime: fromRatio(totals.AcceptedFirstTime),
		ArgumentFidelity:  fromRatio(totals.ArgumentFidelity),
		Confirmation:      fromRatio(totals.Confirmation),
		Unaided:           fromRatio(totals.Unaided),
		Completion:        fromRatio(totals.Completion),
		Overhead: overhead{
			Discovery:     totals.Overhead.Discovery,
			InvalidParams: totals.Overhead.InvalidParams,
			Steps:         totals.Overhead.Steps,
		},
	}
}

// tokensOf adds up what the row cost, keeping the four numbers apart.
func tokensOf(attempts []modelscore.Attempt) tokens {
	var spent tokens
	for _, attempt := range attempts {
		for _, turn := range attempt.Turns {
			spent.Input += turn.Usage.Input
			spent.Output += turn.Usage.Output
			spent.CacheCreated += turn.Usage.CacheCreated
			spent.CacheRead += turn.Usage.CacheRead
		}
	}
	return spent
}

// marshal renders the document as the committed bytes.
//
// One space of indent, as the end-to-end coverage record beside it is written
// with, and a sorted row order, so a re-fold of the same runs produces the same
// file and a diff of two records is a diff of the measurements.
func (d document) marshal() ([]byte, error) {
	sorted := d
	sorted.Rows = append([]row(nil), d.Rows...)
	sort.Slice(sorted.Rows, func(i, j int) bool {
		return sorted.Rows[i].Key.String() < sorted.Rows[j].Key.String()
	})
	body, err := json.MarshalIndent(sorted, "", " ")
	if err != nil {
		return nil, fmt.Errorf("render the record: %w", err)
	}
	return body, nil
}

// unmarshalDocument reads a committed record, refusing one written under
// another schema version.
func unmarshalDocument(data []byte) (document, error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return document{}, fmt.Errorf("read %s: %w", recordRelPath, err)
	}
	if doc.SchemaVersion != recordSchemaVersion {
		return document{}, fmt.Errorf("%s carries schema version %d and this command writes %d; regenerate it with %s",
			recordRelPath, doc.SchemaVersion, recordSchemaVersion, regenerate)
	}
	return doc, nil
}

// runOf, sessionOf and providerOf read a committed row back into the three
// lines it was assembled from.
//
// They exist so that the refusal rules can judge a committed record with the
// same table that judged the shards, which is what keeps a rule added today
// from leaving yesterday's rows standing on terms nothing holds them to any
// more. What cannot be recovered is what the row never carried, and the three
// rules that need it say so themselves rather than reading a zero here as a
// fact.
func runOf(one row) modelrecord.Run {
	return modelrecord.Run{
		Commit:         one.Provenance.Commit,
		StartedAt:      parseDate(one.Provenance.Date),
		Edition:        one.Provenance.Edition,
		GitLabVersion:  one.Provenance.GitLabVersion,
		Tier:           one.Provenance.Tier,
		TierConfirmed:  one.Provenance.TierConfirmed,
		CorpusDigest:   one.Provenance.CorpusDigest,
		ContractDigest: one.Provenance.ContractDigest,
		Repeat:         one.Provenance.Repeat,
	}
}

// sessionOf reads the session line back off a committed row.
func sessionOf(one row) modelrecord.Session {
	return modelrecord.Session{
		Label:           one.Key.Surface + "-" + one.Key.Mode,
		Surface:         one.Provenance.Surface,
		Mode:            one.Provenance.Mode,
		Capabilities:    one.Provenance.CapabilitySurface,
		MetaParamSchema: one.Provenance.MetaParamSchema,
		TierPin:         one.Provenance.TierPin,
		SliceSize:       one.Provenance.SliceSize,
		TokenScopes:     one.Provenance.TokenScopes,
		ServedTools:     one.Provenance.ServedTools,
	}
}

// providerOf reads the provider line back off a committed row.
func providerOf(one row) modelrecord.Provider {
	return modelrecord.Provider{
		Spec:    one.Key.Model,
		Name:    one.Provenance.Provider,
		Model:   one.Provenance.Model,
		Options: one.Provenance.RequestOptions,
	}
}

// parseDate reads a recorded day back, and hands back the zero time for
// anything it cannot read, which the provenance rule then refuses as the hole
// it is.
func parseDate(date string) time.Time {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// claims indexes the rows a document already holds, so a fold can refuse a
// second row for a configuration already published rather than replace it.
func (d document) claims() map[string]string {
	claimed := make(map[string]string, len(d.Rows))
	for _, one := range d.Rows {
		claimed[one.Key.String()] = recordRelPath
	}
	return claimed
}
