//go:build e2e

package provider

import (
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// TestPrices_EveryEntryIsInsideTheProvenanceWindow is the gate the table's
// dates exist for.
//
// A provider changes a price by publishing a page, with no release to notice
// and nothing to break, so a table nobody re-reads becomes wrong in silence and
// every cost figure derived from it becomes wrong with it. The window is the
// one cmd/internal/provenance records for the pinned records; this package
// cannot import that one, because cmd/internal is importable only from cmd.
func TestPrices_EveryEntryIsInsideTheProvenanceWindow(t *testing.T) {
	if problems := priceProblems(time.Now()); len(problems) > 0 {
		t.Errorf("the price table is no longer one a run can rest on:\n  %s\n"+
			"re-read each provider's pricing page and write what it says with today's date",
			strings.Join(problems, "\n  "))
	}
	if len(prices) == 0 {
		t.Fatal("the price table is empty, so the check above passes without asking anything")
	}
}

func TestPrices_ReportsEveryWayADateStopsAnswering(t *testing.T) {
	// The clock is a parameter for the reason every dated record here takes
	// one: these branches are otherwise reachable only from a process running
	// on the right day, and a gate nobody has seen work is not a gate.
	entry := prices[0]
	read, err := time.Parse(time.DateOnly, entry.Price.RetrievedOn)
	if err != nil {
		t.Fatalf("the first entry's date does not parse: %v", err)
	}

	for _, one := range []struct {
		name string
		now  time.Time
		says string
	}{
		{"just read", read.Add(time.Hour), ""},
		{"inside the window", read.Add(maxPriceAge - time.Hour), ""},
		{"past the window", read.Add(maxPriceAge + 48*time.Hour), "and the window is"},
		{"before it was read", read.Add(-48 * time.Hour), "has not happened yet"},
	} {
		t.Run(one.name, func(t *testing.T) {
			problems := entry.problems(one.now)
			if one.says == "" {
				if len(problems) > 0 {
					t.Errorf("a price read on %s is refused at %s: %v",
						entry.Price.RetrievedOn, one.now.Format(time.DateOnly), problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], one.says) {
				t.Errorf("problems = %v, want one saying %q", problems, one.says)
			}
		})
	}
}

func TestPrices_RefusesAnEntryNothingCanJudge(t *testing.T) {
	for _, one := range []struct {
		name  string
		entry priceEntry
		says  string
	}{
		{
			"no source",
			priceEntry{
				Provider: "p", Model: "m", Source: "",
				Price: modelrecord.Price{InputPerMillionUSD: 1, OutputPerMillionUSD: 2, RetrievedOn: today()},
			},
			"names no source",
		},
		{
			"a free model",
			priceEntry{
				Provider: "p", Model: "m", Source: "s",
				Price: modelrecord.Price{RetrievedOn: today()},
			},
			"prices input or output at zero",
		},
		{
			"a date nothing can read",
			priceEntry{
				Provider: "p", Model: "m", Source: "s",
				Price: modelrecord.Price{InputPerMillionUSD: 1, OutputPerMillionUSD: 2, RetrievedOn: "last tuesday"},
			},
			"is not a date",
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			problems := one.entry.problems(time.Now())
			if len(problems) == 0 {
				t.Fatal("the entry was accepted")
			}
			if !strings.Contains(strings.Join(problems, "; "), one.says) {
				t.Errorf("problems = %v, want one saying %q", problems, one.says)
			}
		})
	}
}

func TestPriceFor_AnswersForTheModelsTheTableHasAndRefusesTheRest(t *testing.T) {
	for _, entry := range prices {
		t.Run(entry.Provider+":"+entry.Model, func(t *testing.T) {
			spec, err := ParseSpec(entry.Provider + ":" + entry.Model)
			if err != nil {
				t.Fatalf("ParseSpec: %v", err)
			}
			price, known := PriceFor(spec)
			if !known {
				t.Fatalf("the table holds %s and PriceFor does not find it", spec)
			}
			if price != entry.Price {
				t.Errorf("PriceFor returned %+v, want %+v", price, entry.Price)
			}
		})
	}

	unknown, err := ParseSpec("anthropic:claude-opus-9")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if _, known := PriceFor(unknown); known {
		t.Error("a model the table does not have was priced anyway")
	}
}

func TestUnpricedRefusal_SaysWhichModelWhereTheTableIsAndHowToProceed(t *testing.T) {
	spec, err := ParseSpec("qwen:qwen3.8-flash")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	// This model is the reason the sentence has to be good: its provider
	// publishes no per-token figure that could be read, so it is the one a
	// maintainer will meet.
	if _, known := PriceFor(spec); known {
		t.Skip("the table now prices this model, so the refusal is no longer reachable through it")
	}
	refusal := UnpricedRefusal(spec, "MODELEVAL_UNPRICED")
	for _, wanted := range []string{spec.Raw, "prices.go", "MODELEVAL_UNPRICED=yes"} {
		t.Run(wanted, func(t *testing.T) {
			if !strings.Contains(refusal, wanted) {
				t.Errorf("the refusal does not name %q: %s", wanted, refusal)
			}
		})
	}
}

// today is the retrieval date a fixture entry carries when its date is not what
// is being tested.
func today() string { return time.Now().Format(time.DateOnly) }
