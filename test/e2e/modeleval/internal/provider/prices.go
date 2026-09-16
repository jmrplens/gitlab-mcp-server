//go:build e2e

// prices.go is what a model's tokens cost, as a declaration table with a date.
//
// It exists so that a run can stop at a budget and a published row can carry
// four cost figures instead of one. Four, never one: a cache read is an order
// of magnitude cheaper than the write that created it, and a single number over
// a cached conversation reads as a model being frugal when it is being
// repeated, which is how a withdrawn table came to show one model at a fraction
// of another's budget while sending the same conversation more times.
//
// Every figure here was read off the provider's own pricing page on the date
// the entry carries, and the page is named beside it. A price nobody looked up
// is worse than no price at all: it produces a cost column that is wrong in a
// direction nobody can guess, and this record is committed. A model with no
// entry is therefore refused by a run rather than estimated, and the refusal
// names this file.
//
// One model the repository configures is deliberately absent. Neither Alibaba
// Cloud's Model Studio model list nor its billing page published a per-token
// figure for qwen3.8-flash that could be read on 2026-09-16; the figures in
// circulation come from third-party aggregators and from a regional price list
// in another currency. A run naming it says so and stops, or is told to
// continue without prices explicitly.

package provider

import (
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// maxPriceAge is how old a price may be before the test that reads this table
// refuses it.
//
// It is the window cmd/internal/provenance records for the pinned records, with
// its reasoning: this package cannot import that one, because cmd/internal is
// importable only from cmd, and a copy of the number is the honest way to say
// that a reason to move one is a reason to move both. The reason applies here
// with rather more force than it does to a GitLab record: a provider changes a
// price by publishing a new page, with no release to notice.
const maxPriceAge = 180 * 24 * time.Hour

// priceEntry is one model's prices and where they were read.
type priceEntry struct {
	// Provider is the adapter the model is called through.
	Provider string
	// Model is the identifier as a spec spells it. An alias is keyed as the
	// alias, since that is what a run configures and what a row names.
	Model string
	// Price is the published price, per million tokens.
	Price modelrecord.Price
	// Source is the page the figures were read from.
	Source string
	// Note carries what the figures do not say for themselves.
	Note string
}

// prices is the table. A model absent from it has no price, which is a run's
// refusal and not a zero.
var prices = []priceEntry{
	{
		Provider: Anthropic,
		Model:    "claude-haiku-4-5-20251001",
		Price: modelrecord.Price{
			InputPerMillionUSD:      1.00,
			OutputPerMillionUSD:     5.00,
			CacheWritePerMillionUSD: 1.25,
			CacheReadPerMillionUSD:  0.10,
			RetrievedOn:             "2026-09-16",
		},
		Source: "https://claude.com/pricing",
		Note:   "the cache figures are the 5 minute time to live, which is what a turn loop uses",
	},
	{
		Provider: Google,
		Model:    "gemini-flash-latest",
		Price: modelrecord.Price{
			InputPerMillionUSD:  0.75,
			OutputPerMillionUSD: 3.75,
			// This API bills a cache by the token-hour for storage and
			// charges the cheaper rate on the tokens read back. A turn loop
			// creates no explicit cache, so what it can be charged is the
			// read, and a write figure would be a number for something this
			// run does not do.
			CacheWritePerMillionUSD: 0,
			CacheReadPerMillionUSD:  0.075,
			RetrievedOn:             "2026-09-16",
		},
		Source: "https://ai.google.dev/gemini-api/docs/pricing",
		Note: "the alias resolved to gemini-3.8-flash on the retrieval date, and the page states " +
			"these prices through 2026-12-31 and double from 2027-01-01, so a run in 2027 is " +
			"reading a stale table even inside the window",
	},
	{
		Provider: OpenAI,
		Model:    "gpt-5.4-nano",
		Price: modelrecord.Price{
			InputPerMillionUSD:  0.20,
			OutputPerMillionUSD: 1.25,
			// This API caches without being asked and publishes one cached
			// input price; there is no separate charge for creating the
			// cache.
			CacheWritePerMillionUSD: 0,
			CacheReadPerMillionUSD:  0.02,
			RetrievedOn:             "2026-09-16",
		},
		Source: "https://developers.openai.com/api/docs/pricing",
		Note:   "standard processing, not the batch or flex tiers, which are what a run would have to opt into",
	},
}

// PriceFor returns what one model's tokens cost, and whether the table has it.
func PriceFor(spec Spec) (modelrecord.Price, bool) {
	for _, entry := range prices {
		if entry.Provider == spec.Provider && entry.Model == spec.Model {
			return entry.Price, true
		}
	}
	return modelrecord.Price{}, false
}

// UnpricedRefusal is what a run says when it was configured with a model this
// table has no price for.
//
// It is a sentence rather than a boolean because the reader is a person about
// to spend money: it has to say which model, where the table is, and what to
// set to proceed anyway.
func UnpricedRefusal(spec Spec, override string) string {
	return fmt.Sprintf("%s has no price in test/e2e/modeleval/internal/provider/prices.go, so this run "+
		"cannot say what it will cost: add the provider's published figures with the date you read "+
		"them, or set %s=yes to run without a budget", spec, override)
}

// priceProblems reports the ways this table stops being one a run can rest on:
// a date nothing can parse, a date that has not happened, and a date further
// back than [maxPriceAge].
//
// It takes the clock rather than reading it, so the test can reach every branch
// on any day, which is the rule every dated record in this repository follows.
func priceProblems(now time.Time) []string {
	var problems []string
	for _, entry := range prices {
		problems = append(problems, entry.problems(now)...)
	}
	return problems
}

// problems judges one entry.
func (e priceEntry) problems(now time.Time) []string {
	var problems []string
	if e.Source == "" {
		problems = append(problems, fmt.Sprintf("%s:%s names no source: a price nobody can re-read is "+
			"a number nobody can correct", e.Provider, e.Model))
	}
	if e.Price.InputPerMillionUSD <= 0 || e.Price.OutputPerMillionUSD <= 0 {
		problems = append(problems, fmt.Sprintf("%s:%s prices input or output at zero, which no provider "+
			"does: an entry that cannot be charged is worse than none, since a run would believe it is free",
			e.Provider, e.Model))
	}

	retrieved, err := time.Parse(time.DateOnly, e.Price.RetrievedOn)
	switch age := now.Sub(retrieved); {
	case err != nil:
		problems = append(problems, fmt.Sprintf("%s:%s says it was read on %q, which is not a date: "+
			"nothing can then say how old the figure is", e.Provider, e.Model, e.Price.RetrievedOn))
	case age < 0:
		problems = append(problems, fmt.Sprintf("%s:%s says it was read on %s, which has not happened yet",
			e.Provider, e.Model, e.Price.RetrievedOn))
	case age > maxPriceAge:
		problems = append(problems, fmt.Sprintf("%s:%s was read on %s, %d days ago, and the window is %d: "+
			"a provider changes a price by publishing a page, so a budget computed from this one is a "+
			"guess", e.Provider, e.Model, e.Price.RetrievedOn, int(age.Hours()/24),
			int(maxPriceAge.Hours()/24)))
	}
	return problems
}
