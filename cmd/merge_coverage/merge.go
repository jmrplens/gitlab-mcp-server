// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"golang.org/x/tools/cover"
)

// half is which of the two measurements a profile belongs to.
type half int

// The two halves, and how many there are, which sizes the arrays a block and
// a row keep one entry per half in.
const (
	// offlineHalf is the packages' own tests, run with no GitLab.
	offlineHalf half = iota
	// onlineHalf is the runs against a real GitLab.
	onlineHalf
	// halves is the number of halves.
	halves
)

// modeSet is the profile mode that records whether a block ran rather than
// how often, and the one the merge falls back to for inputs of two modes.
const modeSet = "set"

// source is one profile named on the command line and the half it measures.
type source struct {
	path string
	half half
}

// position is a line and a column of a source file, as a profile spells
// them.
type position struct{ line, col int }

// before reports whether p comes before q in a file.
func (p position) before(q position) bool {
	return p.line < q.line || p.line == q.line && p.col < q.col
}

// span is the place one block takes in its file.
type span struct {
	file       string
	start, end position
}

// extent spells the span's positions the way a profile does.
func (s span) extent() string {
	return fmt.Sprintf("%d.%d,%d.%d", s.start.line, s.start.col, s.end.line, s.end.col)
}

// compareSpans orders spans by file, then by start, then by end, which is
// the order a profile lists its blocks in.
func compareSpans(a, b span) int {
	return cmp.Or(
		strings.Compare(a.file, b.file),
		cmp.Compare(a.start.line, b.start.line),
		cmp.Compare(a.start.col, b.start.col),
		cmp.Compare(a.end.line, b.end.line),
		cmp.Compare(a.end.col, b.end.col),
	)
}

// block is what the profiles say of one span: how many statements it holds,
// its count added over every profile, which halves carried it and which of
// them reached it, and the profiles that carried it, in the order read, the
// first of which a refusal names.
type block struct {
	statements int
	count      int
	seen, hit  [halves]bool
	in         []string
}

// coverage is the union of every profile read: the mode the merged profile
// is written in, every block by its span, and the profiles that carry a
// block of each package, in the order read.
type coverage struct {
	mode     string
	blocks   map[span]*block
	packages map[string][]string
}

// read parses every source and merges them, refusing a profile it cannot
// parse and any two blocks that cannot have come from the same source.
func read(sources []source) (*coverage, error) {
	c := &coverage{blocks: map[span]*block{}, packages: map[string][]string{}}
	for _, src := range sources {
		if err := c.addSource(src); err != nil {
			return nil, err
		}
	}
	if err := c.checkOverlaps(); err != nil {
		return nil, err
	}
	if err := c.checkCarriers(); err != nil {
		return nil, err
	}
	return c, nil
}

// addSource parses one profile and folds every block of it in, noting each
// package it carries once.
func (c *coverage) addSource(src source) error {
	profiles, err := cover.ParseProfiles(src.path)
	if err != nil {
		return fmt.Errorf("profile %s: %w", src.path, err)
	}
	carried := map[string]bool{}
	for _, p := range profiles {
		c.addMode(p.Mode)
		if pkg := path.Dir(p.FileName); !carried[pkg] {
			carried[pkg] = true
			c.packages[pkg] = append(c.packages[pkg], src.path)
		}
		for _, b := range p.Blocks {
			if blockErr := c.addBlock(src, p.FileName, b); blockErr != nil {
				return blockErr
			}
		}
	}
	return nil
}

// addMode folds one profile's mode into the merged profile's: the first one
// read, kept while every later one agrees, and set mode once two differ.
func (c *coverage) addMode(mode string) {
	switch c.mode {
	case "":
		c.mode = mode
	case mode:
	default:
		c.mode = modeSet
	}
}

// addBlock folds one block of a profile in, refusing an extent another
// profile already gave another number of statements. A profile lists each
// block of a file once, since cover.ParseProfiles folds the repeats, so the
// profile is added to the block's once.
func (c *coverage) addBlock(src source, file string, pb cover.ProfileBlock) error {
	s := span{file: file, start: position{pb.StartLine, pb.StartCol}, end: position{pb.EndLine, pb.EndCol}}
	b, known := c.blocks[s]
	if !known {
		b = &block{statements: pb.NumStmt}
		c.blocks[s] = b
	}
	if b.statements != pb.NumStmt {
		return fmt.Errorf("%s: the block at %s counts %d statements in %s and %d in %s, so the two were not written from the same source",
			file, s.extent(), b.statements, b.in[0], pb.NumStmt, src.path)
	}
	b.in = append(b.in, src.path)
	b.count += pb.Count
	b.seen[src.half] = true
	if pb.Count > 0 {
		b.hit[src.half] = true
	}
	return nil
}

// sorted returns every span in the order compareSpans gives.
func (c *coverage) sorted() []span {
	spans := slices.Collect(maps.Keys(c.blocks))
	slices.SortFunc(spans, compareSpans)
	return spans
}

// checkOverlaps refuses two blocks of one file that overlap without being
// the same block. Within one profile written from one source no two blocks
// overlap, so an overlap is two versions of the file. In sorted order a
// block that overlaps any later one overlaps the one right after it, so
// neighbors are all that need comparing.
func (c *coverage) checkOverlaps() error {
	spans := c.sorted()
	for i := 1; i < len(spans); i++ {
		prev, next := spans[i-1], spans[i]
		if prev.file == next.file && next.start.before(prev.end) {
			return fmt.Errorf("%s: the block at %s in %s and the block at %s in %s overlap without being one block, so the profiles were not written from the same source",
				next.file, prev.extent(), c.blocks[prev].in[0], next.extent(), c.blocks[next].in[0])
		}
	}
	return nil
}

// checkCarriers refuses a block that a profile carrying its package does not
// carry. go test writes every block of every package a test binary measures,
// the ones no test reached included, so profiles written from one source
// carry a package whole or not at all, and a block one of them lacks is code
// only one version of the package has: a function or a file added or
// deleted. A deleted function after the unchanged ones overlaps no block of
// the other version, so this is the check that refuses it.
func (c *coverage) checkCarriers() error {
	for _, s := range c.sorted() {
		in, carriers := c.blocks[s].in, c.packages[path.Dir(s.file)]
		if len(in) != len(carriers) {
			return fmt.Errorf("%s: the block at %s is in %s and not in %s, which carries the package, so the profiles were not written from the same source",
				s.file, s.extent(), in[0], missingFrom(carriers, in))
		}
	}
	return nil
}

// missingFrom returns the first of the profiles carrying a package that in,
// the profiles carrying one of its blocks, leaves out. Both list profiles in
// the order read, and every profile in in carries the package.
func missingFrom(carriers, in []string) string {
	for i, profile := range in {
		if carriers[i] != profile {
			return carriers[i]
		}
	}
	return carriers[len(in)]
}

// profile renders the merged profile: the mode, then every block once in
// file order, with its count added over the profiles, or in set mode whether
// any of them reached it.
func (c *coverage) profile() []byte {
	var b strings.Builder
	b.WriteString("mode: " + c.mode + "\n")
	for _, s := range c.sorted() {
		blk := c.blocks[s]
		count := blk.count
		if c.mode == modeSet && count > 0 {
			count = 1
		}
		fmt.Fprintf(&b, "%s:%s %d %d\n", s.file, s.extent(), blk.statements, count)
	}
	return []byte(b.String())
}

// figure is how many of a package's statements a measurement counted, and
// how many of them it reached.
type figure struct{ statements, covered int }

// with returns the figure with the statements of one more block counted, as
// reached or not.
func (f figure) with(statements int, reached bool) figure {
	f.statements += statements
	if reached {
		f.covered += statements
	}
	return f
}

// missed is how many of the statements counted the measurement did not
// reach.
func (f figure) missed() int {
	return f.statements - f.covered
}

// String spells the figure as reached out of counted with the percentage
// truncated to one decimal, so a package short of its last statement never
// reads 100.0%; a figure that counted nothing has no percentage.
func (f figure) String() string {
	if f.statements == 0 {
		return "0/0"
	}
	permille := f.covered * 1000 / f.statements
	return fmt.Sprintf("%d/%d (%d.%d%%)", f.covered, f.statements, permille/10, permille%10)
}

// row is one package of the report: how many blocks the profiles carry of
// it, which halves carried any, each half's figure, the merged one, and the
// statements only the runs reached.
type row struct {
	pkg        string
	blocks     int
	present    [halves]bool
	halves     [halves]figure
	merged     figure
	onlyOnline int
}

// add counts one block of the row's package into every figure it belongs to.
func (r *row) add(b *block) {
	r.blocks++
	r.merged = r.merged.with(b.statements, b.hit[offlineHalf] || b.hit[onlineHalf])
	for h := range halves {
		if b.seen[h] {
			r.present[h] = true
			r.halves[h] = r.halves[h].with(b.statements, b.hit[h])
		}
	}
	if b.hit[onlineHalf] && !b.hit[offlineHalf] {
		r.onlyOnline += b.statements
	}
}

// rows measures each package, in the order given, refusing one no profile
// carries a block of: there is nothing to measure it by, and reporting none
// reached out of none would read as a package with nothing to reach.
func (c *coverage) rows(packages []string) ([]row, error) {
	rows := make([]row, 0, len(packages))
	for _, pkg := range packages {
		r := row{pkg: pkg}
		for s, b := range c.blocks {
			if path.Dir(s.file) == pkg {
				r.add(b)
			}
		}
		if r.blocks == 0 {
			return nil, fmt.Errorf("no profile carries %s: no test binary either half ran imports it, or neither half named it in -coverpkg", pkg)
		}
		rows = append(rows, r)
	}
	return rows, nil
}
