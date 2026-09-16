package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// refusalCase is one rule, and the one thing broken in a publishable shard that
// must trip it.
//
// The table is written this way round on purpose: every case starts from the
// shard every rule lets through and breaks exactly one thing, so a case that
// stops tripping its rule is telling you the rule stopped working rather than
// that the fixture was never publishable.
type refusalCase struct {
	// name is the rule this case exists to trip.
	name string
	// break_ is what it does to a publishable shard.
	breakIt func(records []modelrecord.Record)
	// claimed is what already stands, for the one rule that is about the
	// record rather than about the run.
	claimed map[string]string
	// want is a fragment of the reason, so a rule that fires for another
	// reason than the one intended is still a failure.
	want string
}

// refusalCases is one case per declared rule. The staleness test below reads
// this table and the rule table together, so a rule added without a case fails
// and a case naming a rule that no longer exists fails too.
func refusalCases() []refusalCase {
	return []refusalCase{
		{
			name:    "filtered-run",
			breakIt: func(records []modelrecord.Record) { records[0].Run.Filter = "TestModelEval/MT-002" },
			want:    "put part of the corpus",
		},
		{
			name: "unobserved-session",
			breakIt: func(records []modelrecord.Record) {
				records[3].Call.DispatchObserved = false
			},
			want: "the server's span described none of them",
		},
		{
			name: "fake-provider",
			breakIt: func(records []modelrecord.Record) {
				records[0].Run.Providers[0].Name = fakeProvider
			},
			want: "the fake provider",
		},
		{
			name: "incomplete-provenance",
			breakIt: func(records []modelrecord.Record) {
				records[0].Run.Commit = ""
			},
			want: `the provenance field "commit" is empty`,
		},
		{
			name: "stale-corpus",
			breakIt: func(records []modelrecord.Record) {
				records[0].Run.CorpusDigest = "a corpus that has moved"
			},
			want: "the corpus at HEAD is",
		},
		{
			name:    "duplicate-row",
			breakIt: func([]modelrecord.Record) {},
			claimed: map[string]string{publishableKey().String(): docs},
			want:    "already stands in " + docs,
		},
	}
}

// docs is where the duplicate case says the first row already stands. It is a
// literal rather than [recordRelPath] so that the case is about a key being
// claimed and not about which file claimed it.
const docs = "an earlier run"

// publishableKey is the key the publishable fixture folds to, which the
// duplicate case has to claim before the fold sees it.
func publishableKey() rowKey {
	return rowKey{
		Model:            fixtureModel,
		Surface:          "dynamic",
		Mode:             "default",
		Tier:             "ultimate",
		CorpusDigest:     modelcorpus.Digest(),
		ContractDigest:   fixtureContract,
		ToolSchemaDigest: fixtureTools,
		Repeat:           1,
	}
}

// TestRefusals_EachRule_RefusesTheRowItIsFor drives every declared rule from a
// hand-written shard that trips it.
//
// It is the whole of what the refusal table is for. Each of these six is a way
// the tables this replaces published something that was not a measurement: a
// run of part of the corpus read as the whole of it, a session whose dispatches
// nobody saw, the fake's replay of our own answer key, a row whose
// configuration was not written down, a record scored against a corpus that had
// moved, and a second run of one configuration quietly replacing the first.
func TestRefusals_EachRule_RefusesTheRowItIsFor(t *testing.T) {
	for _, one := range refusalCases() {
		t.Run(one.name, func(t *testing.T) {
			records := publishableShard()
			one.breakIt(records)

			rows, refusals, err := judge(foldShard(t, records, one.claimed), modelcorpus.Keys())
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			if len(rows) != 0 {
				t.Fatalf("published %d row(s), want the shard refused", len(rows))
			}
			if len(refusals) != 1 {
				t.Fatalf("got %d refusals, want exactly one", len(refusals))
			}
			if refusals[0].Rule != one.name {
				t.Errorf("refused by rule %q, want %q (%s)", refusals[0].Rule, one.name, refusals[0].Reason)
			}
			if !strings.Contains(refusals[0].Reason, one.want) {
				t.Errorf("reason %q does not carry %q", refusals[0].Reason, one.want)
			}
			if !strings.Contains(refusals[0].String(), one.name) {
				t.Errorf("the reported line %q does not name the rule", refusals[0].String())
			}
		})
	}
}

// TestRefusals_EveryDeclaredRuleHasACase is the staleness test every
// declaration table in this repository carries.
//
// A rule nothing can trip is a rule nobody knows still works, and a case naming
// a rule that has been renamed is a case that passes by testing nothing. Both
// are findings here rather than silences.
func TestRefusals_EveryDeclaredRuleHasACase(t *testing.T) {
	covered := map[string]bool{}
	for _, one := range refusalCases() {
		covered[one.name] = true
	}
	declared := map[string]bool{}
	for _, one := range rules {
		if declared[one.Name] {
			t.Errorf("the rule %q is declared twice, so a refusal cannot name which fired", one.Name)
		}
		declared[one.Name] = true
		if one.Why == "" {
			t.Errorf("the rule %q says what it refuses and not why, which is the half a reader needs", one.Name)
		}
		if !covered[one.Name] {
			t.Errorf("the rule %q has no case that trips it", one.Name)
		}
	}
	for name := range covered {
		if !declared[name] {
			t.Errorf("a case names the rule %q, which is not declared any more", name)
		}
	}
}

// TestRefusals_OnlyTheFirstRuleFires keeps the report readable: a row refused
// for being the fake's is refused, and going on to say that its corpus has also
// moved is noise a reader has to sort through.
func TestRefusals_OnlyTheFirstRuleFires(t *testing.T) {
	records := publishableShard()
	records[0].Run.Filter = "TestModelEval/MT-002"
	records[0].Run.Providers[0].Name = fakeProvider
	records[0].Run.CorpusDigest = "a corpus that has moved"

	_, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(refusals) != 1 || refusals[0].Rule != "filtered-run" {
		t.Fatalf("got %v, want the one refusal the table meets first", refusals)
	}
}

// TestRefusals_ProvenanceRule_NamesEveryFieldItRequires drives the required
// list one field at a time, because a field dropped from that table is a hole
// that would be published rather than refused and no other test would see it.
func TestRefusals_ProvenanceRule_NamesEveryFieldItRequires(t *testing.T) {
	one := publishOne(t, publishableShard())
	for _, field := range requiredProvenance {
		t.Run(field.Name, func(t *testing.T) {
			if field.Why == "" {
				t.Fatalf("the field %q says nothing about what a reader loses", field.Name)
			}
			if field.Empty(one.Provenance) {
				t.Fatalf("the publishable fixture has no %q, so this rule could never be tested", field.Name)
			}
			blanked := blankProvenance(one.Provenance, field.Name)
			reason := missingProvenance(blanked)
			if !strings.Contains(reason, field.Name) {
				t.Errorf("blanking %q was reported as %q", field.Name, reason)
			}
		})
	}
}

// blankProvenance empties one named field, which is what the test above needs
// and what nothing in the command does.
func blankProvenance(p provenance, field string) provenance {
	switch field {
	case "commit":
		p.Commit = ""
	case "date":
		p.Date = ""
	case "gitlab_version":
		p.GitLabVersion = ""
	case "edition":
		p.Edition = ""
	case "tier":
		p.Tier = ""
	case "surface":
		p.Surface = ""
	case "mode":
		p.Mode = ""
	case "capability_surface":
		p.CapabilitySurface = ""
	case "token_scopes":
		p.TokenScopes = nil
	case "served_tools":
		p.ServedTools = 0
	case "provider":
		p.Provider = ""
	case "model":
		p.Model = ""
	case "request_options":
		p.RequestOptions = nil
	case "repeat":
		p.Repeat = 0
	case "corpus_digest":
		p.CorpusDigest = ""
	case "contract_digest":
		p.ContractDigest = ""
	case "tool_schema_digest":
		p.ToolSchemaDigest = ""
	}
	return p
}

// TestRefusals_AToolSchemaDigestNobodyNoted_IsRefusedOnBothSides is the hole
// that reopens the defect the per-provider digest was added to catch.
//
// A run notes a digest per provider, so a run that noted none for a model
// leaves the empty string in the key. Two such rows then agree on the empty
// string, which is all the cross-vendor key asks, and they are seated in one
// table captioned "tool schemas “" as if it had been checked and found equal.
// Both directions are held here: the shard on the way in and the committed row
// on review, because a row published before this rule existed must not go on
// standing.
func TestRefusals_AToolSchemaDigestNobodyNoted_IsRefusedOnBothSides(t *testing.T) {
	records := publishableShard()
	records[4].Session.ToolSchemaDigests = nil

	rows, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("published %d row(s) with no tool-schema digest", len(rows))
	}
	if len(refusals) != 1 || refusals[0].Rule != "incomplete-provenance" {
		t.Fatalf("got %v, want the provenance rule", refusals)
	}
	if !strings.Contains(refusals[0].Reason, `"tool_schema_digest"`) {
		t.Errorf("reason %q does not name the empty field", refusals[0].Reason)
	}

	published := publishOne(t, publishableShard())
	published.Key.ToolSchemaDigest = ""
	published.Provenance.ToolSchemaDigest = ""
	found := reviewRows(document{Rows: []row{published}})
	if len(found) != 1 || found[0].Rule != "incomplete-provenance" {
		t.Fatalf("got %v, want a committed row with no digest refused on review", found)
	}
}

// TestRefusals_AModelTheRunNeverDescribed_IsRefusedByName covers the branch the
// provenance rule reaches before it reads a field: a run that recorded attempts
// for a model its own run line does not describe has no provider and no request
// options, and neither can be invented.
func TestRefusals_AModelTheRunNeverDescribed_IsRefusedByName(t *testing.T) {
	records := publishableShard()
	records[0].Run.Providers = nil

	_, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(refusals) != 1 || refusals[0].Rule != "incomplete-provenance" {
		t.Fatalf("got %v, want the provenance rule", refusals)
	}
	if !strings.Contains(refusals[0].Reason, "describes no provider") {
		t.Errorf("reason %q does not say the run line described no provider", refusals[0].Reason)
	}
}

// TestRefusals_ASessionThatCalledNothing_IsNotRefused is the other half of the
// observation rule, and the half that would quietly delete a measurement if it
// were wrong: a read-only row where every mutating step was correctly declined
// in text makes no call at all, and there is nothing there to have been
// unobserved.
func TestRefusals_ASessionThatCalledNothing_IsNotRefused(t *testing.T) {
	records := publishableShard()
	records = append(records[:3], records[4:]...) // drop the call line

	_, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(refusals) != 0 {
		t.Fatalf("got %v, want a session that called nothing to be published", refusals)
	}
}

// TestReviewRows_ACommittedRecord_IsJudgedByTheSameTable is what keeps a rule
// added today from leaving yesterday's rows standing on terms nothing holds
// them to. The record is re-scored from the corpus at HEAD on every render, so
// a row the rules would refuse now is a row whose figures describe nothing.
func TestReviewRows_ACommittedRecord_IsJudgedByTheSameTable(t *testing.T) {
	one := publishOne(t, publishableShard())
	if found := reviewRows(document{Rows: []row{one}}); len(found) != 0 {
		t.Fatalf("a row this tree just published was refused on review: %v", found)
	}

	moved := one
	moved.Provenance.CorpusDigest = "a corpus that has moved"
	moved.Key.CorpusDigest = moved.Provenance.CorpusDigest
	found := reviewRows(document{Rows: []row{moved}})
	if len(found) != 1 || found[0].Rule != "stale-corpus" {
		t.Fatalf("got %v, want the stale corpus refused on review", found)
	}
}

// TestReviewRows_TwoRowsOfOneKey_AreRefusedByName holds the rule that a
// collision is never silently resolved: the record carries both, and the review
// names the second rather than picking one.
func TestReviewRows_TwoRowsOfOneKey_AreRefusedByName(t *testing.T) {
	one := publishOne(t, publishableShard())
	found := reviewRows(document{Rows: []row{one, one}})
	if len(found) != 1 || found[0].Rule != "duplicate-row" {
		t.Fatalf("got %v, want the second row refused", found)
	}
	if !strings.Contains(found[0].Key, fixtureModel) {
		t.Errorf("the refusal %q does not name the row", found[0].Key)
	}
}

// TestRuleNames_AreSortedAndComplete covers the sentence a run prints to say
// what judged it, which is the only place a reader learns the rules exist.
func TestRuleNames_AreSortedAndComplete(t *testing.T) {
	names := ruleNames()
	for _, one := range rules {
		if !strings.Contains(names, one.Name) {
			t.Errorf("the reported rule list %q does not name %q", names, one.Name)
		}
	}
	if !strings.HasPrefix(names, "duplicate-row") {
		t.Errorf("the rule list %q is not sorted", names)
	}
}
