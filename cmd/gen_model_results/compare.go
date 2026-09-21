// compare.go holds the two keys that decide which rows may be read down one
// table, and nothing else.
//
// Two rules and not one. A cross-vendor table compares models and holds the
// surface fixed; a cross-surface table compares surfaces and holds the model
// fixed. The first draft of the rebuild plan had a single rule requiring
// agreement on the surface, which would have forbidden the cross-surface
// comparison the same draft promised, and that is exactly the class of mistake
// a key written once in code and read by every renderer prevents.
//
// What is in a key is what changes the measurement. What is left out of the
// cross-surface key is what a surface decides for itself and could therefore
// never agree across surfaces: the meta schema mode, which exists only on meta;
// the slice size, which exists only on individual; and the tool-schema digest,
// which is a hash of the tool list and so differs between surfaces by
// construction. A cross-surface row is labeled with whichever of the first two
// it has, so a reader sees what it could not hold fixed.

package main

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
)

// caseCoverage is which of the corpus a row was measured on.
//
// The corpus digest already in the key says what the corpus *is*. It does not
// say how much of it was asked, and those are different questions: a run
// narrowed with MODELEVAL_CASES puts a handful of cases and publishes a row
// whose key is identical to a full run's. Seated in one table under a caption
// asserting the two agree on everything that matters, a model measured on
// twelve cases reads beside one measured on 258 as though the comparison were
// honest, and the only thing distinguishing them is a denominator a reader has
// to notice on their own.
//
// So a row carries the set it covers. Two rows sharing a table must have
// measured the same cases and not merely the same number of them, which is why
// the key holds a digest of the identifiers rather than a count; the count is
// what the caption prints, because a reader wants to know the size and cannot
// read a hash.
type caseCoverage struct {
	// Digest fingerprints the case identifiers behind the row, empty for a row
	// that records none.
	Digest string
	// Cases is how many there are, and Corpus how many the corpus at HEAD has.
	// Every published row was measured against that corpus: a row whose run
	// answered an older one is refused by the stale-corpus rule before it can
	// reach here.
	Cases  int
	Corpus int
}

// coverageOf reads what a row covers.
func coverageOf(one row) caseCoverage {
	names := sortedCaseNames(one.Cases)
	sum := fnv.New64a()
	for _, name := range names {
		_, _ = sum.Write([]byte(name))
		_, _ = sum.Write([]byte{0})
	}
	digest := ""
	if len(names) > 0 {
		digest = strconv.FormatUint(sum.Sum64(), 16)
	}
	return caseCoverage{Digest: digest, Cases: len(names), Corpus: len(modelcorpus.Keys())}
}

// String says how much of the corpus a row covers, for the caption, and
// nothing at all for a row that records no cases.
//
// Silence rather than a sentence: the caption is a list of things two rows were
// found to agree on, and "a coverage neither of them recorded" is not an
// agreement. A row like that is one written before the cases were carried, and
// saying so in every caption would be noise a reader has to step over.
//
// The whole-corpus arm deliberately tests only for equality with the corpus. A
// guard on the corpus being counted at all used to stand beside it and could
// never decide anything: the arm above has already established that Cases is
// not zero, so a corpus of zero is a corpus Cases cannot equal. Both arms ask
// about the same value, which is what makes this a switch on it.
func (c caseCoverage) String() string {
	switch c.Cases {
	case 0:
		return ""
	case c.Corpus:
		return "the whole corpus (" + strconv.Itoa(c.Cases) + " cases)"
	default:
		return strconv.Itoa(c.Cases) + " of " + strconv.Itoa(c.Corpus) + " cases (`" + c.Digest + "`)"
	}
}

// vendorKey is what two rows must agree on before they may be read down one
// table as a comparison between models.
type vendorKey struct {
	Surface          string
	Mode             string
	Tier             string
	TierPin          string
	MetaParamSchema  string
	SliceSize        int
	CorpusDigest     string
	ContractDigest   string
	ToolSchemaDigest string
	Repeat           int
	Coverage         caseCoverage
}

// crossVendorKey is the row key with the model taken out: everything else has
// to agree, the tool-schema digest included, so a provider-specific rewrite of
// the schemas puts its row in a table of its own rather than in a column beside
// models that were sent something else.
func crossVendorKey(one row) vendorKey {
	return vendorKey{
		Surface:          one.Key.Surface,
		Mode:             one.Key.Mode,
		Tier:             one.Key.Tier,
		TierPin:          one.Key.TierPin,
		MetaParamSchema:  one.Key.MetaParamSchema,
		SliceSize:        one.Key.SliceSize,
		CorpusDigest:     one.Key.CorpusDigest,
		ContractDigest:   one.Key.ContractDigest,
		ToolSchemaDigest: one.Key.ToolSchemaDigest,
		Repeat:           one.Key.Repeat,
		Coverage:         coverageOf(one),
	}
}

// String renders what the table holds fixed, which is what its caption says.
func (k vendorKey) String() string {
	parts := []string{
		"surface `" + k.Surface + "`",
		"mode `" + k.Mode + "`",
		"tier `" + k.Tier + "`",
	}
	if k.TierPin != "" {
		parts = append(parts, "tier pinned to `"+k.TierPin+"`")
	}
	if k.MetaParamSchema != "" {
		parts = append(parts, "meta schema `"+k.MetaParamSchema+"`")
	}
	if k.SliceSize > 0 {
		parts = append(parts, "slice of "+strconv.Itoa(k.SliceSize)+" tools")
	}
	parts = append(parts, "corpus `"+k.CorpusDigest+"`")
	if covered := k.Coverage.String(); covered != "" {
		parts = append(parts, covered)
	}
	parts = append(parts,
		"contract `"+k.ContractDigest+"`",
		"tool schemas `"+k.ToolSchemaDigest+"`",
		"repeat "+strconv.Itoa(k.Repeat),
	)
	return strings.Join(parts, ", ")
}

// surfaceKey is what two rows must agree on before they may be read as one
// model measured on two surfaces.
type surfaceKey struct {
	Model          string
	Mode           string
	Tier           string
	TierPin        string
	CorpusDigest   string
	ContractDigest string
	Repeat         int
	Coverage       caseCoverage
}

// crossSurfaceKey is the row key with the surface and everything the surface
// decides taken out.
func crossSurfaceKey(one row) surfaceKey {
	return surfaceKey{
		Model:          one.Key.Model,
		Mode:           one.Key.Mode,
		Tier:           one.Key.Tier,
		TierPin:        one.Key.TierPin,
		CorpusDigest:   one.Key.CorpusDigest,
		ContractDigest: one.Key.ContractDigest,
		Repeat:         one.Key.Repeat,
		Coverage:       coverageOf(one),
	}
}

// surfaceLabel is what one row decided for itself, which a cross-surface
// comparison cannot hold fixed and therefore has to print beside the row.
//
// It is the other half of the cross-surface rule. The key above says what two
// rows agree on; this says what they could not, so that a reader comparing a
// dynamic row with an individual one can see that the second was measured on a
// slice of the catalog and the first on all of it. Leaving it out would make
// the two look alike in the one respect they are not, which is the whole reason
// the individual surface is a class of its own.
//
// A row can carry both: the schema mode is a property of the child a run
// started, so a run that pinned one has it on every surface's session, while
// the slice size is the individual surface's alone.
func surfaceLabel(one row) string {
	var parts []string
	if one.Key.SliceSize > 0 {
		// The budget, and beside it what the attempts were actually shown. A
		// case whose own domains outnumber the budget is shown all of them
		// rather than fewer tools than it needs, so the two figures differ on
		// the individual surface and the caption has to carry both: a reader
		// told only the budget would compare an attempt shown 312 tools with
		// one shown 96 as though they had seen the same catalog.
		slice := "slice of " + strconv.Itoa(one.Key.SliceSize) + " tools"
		if one.Counts.Shown != nil {
			slice += " (shown " + one.Counts.Shown.String() + ")"
		}
		parts = append(parts, slice)
	}
	if one.Key.MetaParamSchema != "" {
		parts = append(parts, "meta schema `"+one.Key.MetaParamSchema+"`")
	}
	return strings.Join(parts, ", ")
}

// String renders what a cross-surface comparison holds fixed.
func (k surfaceKey) String() string {
	parts := []string{
		"model `" + k.Model + "`",
		"mode `" + k.Mode + "`",
		"tier `" + k.Tier + "`",
	}
	if k.TierPin != "" {
		parts = append(parts, "tier pinned to `"+k.TierPin+"`")
	}
	parts = append(parts, "corpus `"+k.CorpusDigest+"`")
	if covered := k.Coverage.String(); covered != "" {
		parts = append(parts, covered)
	}
	parts = append(parts,
		"contract `"+k.ContractDigest+"`",
		"repeat "+strconv.Itoa(k.Repeat),
	)
	return strings.Join(parts, ", ")
}

// group is a set of rows that may be read down one table, with the caption
// naming what they hold fixed.
type group struct {
	// Caption is what the key renders to.
	Caption string
	// Rows are the rows of the table, in the order a reader reads them.
	Rows []row
}

// surfaces counts how many distinct surfaces the group's rows were measured on,
// which is what makes a cross-surface group a comparison rather than one row
// with nothing to compare it to.
func (g group) surfaces() int {
	seen := map[string]bool{}
	for _, one := range g.Rows {
		seen[one.Key.Surface] = true
	}
	return len(seen)
}

// groupByVendor collects the rows into cross-vendor tables, each sorted by
// model and the tables themselves sorted by their caption, so a re-render of
// one record produces the same page.
func groupByVendor(rows []row) []group {
	return collect(rows,
		func(one row) string { return crossVendorKey(one).String() },
		func(a, b row) bool { return a.Key.Model < b.Key.Model },
	)
}

// groupBySurface collects the rows into cross-surface comparisons, each sorted
// by surface.
func groupBySurface(rows []row) []group {
	return collect(rows,
		func(one row) string { return crossSurfaceKey(one).String() },
		func(a, b row) bool { return a.Key.Surface < b.Key.Surface },
	)
}

// collect groups rows by a caption and orders each group's rows.
func collect(rows []row, caption func(row) string, less func(a, b row) bool) []group {
	byCaption := map[string][]row{}
	for _, one := range rows {
		key := caption(one)
		byCaption[key] = append(byCaption[key], one)
	}
	captions := make([]string, 0, len(byCaption))
	for key := range byCaption {
		captions = append(captions, key)
	}
	sort.Strings(captions)

	groups := make([]group, 0, len(captions))
	for _, key := range captions {
		members := byCaption[key]
		sort.SliceStable(members, func(i, j int) bool { return less(members[i], members[j]) })
		groups = append(groups, group{Caption: key, Rows: members})
	}
	return groups
}
