// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// recordSchema is the layout of the record this command writes and reads.
const recordSchema = 1

// record is the committed account of what the complete runs reached in the
// measured packages, which is the half of the merge CI cannot take, since
// nothing there runs a GitLab. It says when it was taken and from which run
// profiles, each package with its statements and how many of them neither
// half reached, which is what the gate holds the package to, and every
// declaration the runs reached into, with the hash of its text and the
// blocks they reached, so the reach is credited only to a declaration that
// is still the code the runs ran.
type record struct {
	Schema       int                 `json:"schema"`
	Source       recordSource        `json:"source"`
	Packages     []recordPackage     `json:"packages"`
	Declarations []recordDeclaration `json:"declarations"`
}

// recordSource is when the record was taken, as a UTC date, and the file
// names of the run profiles it was folded from.
type recordSource struct {
	Recorded string   `json:"recorded"`
	Runs     []string `json:"runs"`
}

// recordPackage is one measured package as the record took it: whether any
// run profile carried it, how many statements it had, and how many of them
// neither half reached, the ceiling the gate holds it to.
type recordPackage struct {
	Package        string `json:"package"`
	MeasuredByRuns bool   `json:"measured_by_runs"`
	Statements     int    `json:"statements"`
	Unreached      int    `json:"unreached"`
}

// recordDeclaration is one declaration the runs reached into: its file below
// the module root, its key, the hash of its text, and each block they
// reached, spelled as its extent with lines counted from the declaration's
// first line, then its statements, so lines added above the declaration
// leave it standing.
type recordDeclaration struct {
	File        string   `json:"file"`
	Declaration string   `json:"declaration"`
	SHA256      string   `json:"sha256"`
	Reached     []string `json:"reached"`
}

// now is a seam over time.Now, so a test can say which day a record is taken.
var now = time.Now

// newRecord takes the record of the merge for the packages of rows: the runs
// sources names, each package as rows measured it, and every declaration of
// those packages the runs reached into, read from the source on disk. It
// refuses a package no offline profile carries, since the gate holds the
// offline half alone to the record and could never measure it, and a source
// that is not the one the profiles measured.
func (c *coverage) newRecord(rows []row, sources []source) (*record, error) {
	rec := &record{
		Schema: recordSchema,
		Source: recordSource{Recorded: now().UTC().Format(time.DateOnly), Runs: sourceNames(sources, onlineHalf)},
	}
	measured := map[string]bool{}
	for _, r := range rows {
		if !r.present[offlineHalf] {
			return nil, fmt.Errorf("no offline profile carries %s, and the gate holds the offline half alone to the record, so it could never check a record of it", r.pkg)
		}
		measured[r.pkg] = true
		rec.Packages = append(rec.Packages, recordPackage{
			Package:        goprogram.TrimModulePath(r.pkg),
			MeasuredByRuns: r.present[onlineHalf],
			Statements:     r.merged.statements,
			Unreached:      r.merged.missed(),
		})
	}
	decls, err := c.reachedDeclarations(measured)
	if err != nil {
		return nil, err
	}
	rec.Declarations = decls
	return rec, nil
}

// reachedDeclarations lists every declaration of the measured packages that
// holds a block the runs reached, sorted by file and key, each with the
// blocks they reached in it in file order. The spans come in file order and
// no two declarations of a file interleave, so a block belongs to the entry
// before it or opens one of its own.
func (c *coverage) reachedDeclarations(measured map[string]bool) ([]recordDeclaration, error) {
	var (
		entries []recordDeclaration
		decls   []declaration
		current string
	)
	for _, s := range c.sorted() {
		b := c.blocks[s]
		if !b.hit[onlineHalf] || !measured[path.Dir(s.file)] {
			continue
		}
		if s.file != current {
			var err error
			if decls, err = sourceDeclarations(s.file); err != nil {
				return nil, err
			}
			current = s.file
		}
		d, ok := holding(decls, s.start)
		if !ok {
			return nil, fmt.Errorf("%s: no declaration of the file holds the block at %s, so the source on disk is not the one the profiles measured",
				s.file, s.extent())
		}
		file := goprogram.TrimModulePath(s.file)
		if n := len(entries); n == 0 || entries[n-1].File != file || entries[n-1].Declaration != d.key {
			entries = append(entries, recordDeclaration{File: file, Declaration: d.key, SHA256: d.sum})
		}
		last := &entries[len(entries)-1]
		last.Reached = append(last.Reached, relativeExtent(s, d, b.statements))
	}
	slices.SortStableFunc(entries, func(a, b recordDeclaration) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Declaration, b.Declaration))
	})
	return entries, nil
}

// sourceDeclarations reads one measured file, named by the import path a
// profile gives it, from below the working directory, which is the module
// root wherever the command is run from make, and parses its declarations.
func sourceDeclarations(file string) ([]declaration, error) {
	rel := goprogram.TrimModulePath(file)
	src, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		return nil, fmt.Errorf("reading the source the profiles measured: %w", err)
	}
	return declarationsOf(rel, src)
}

// relativeExtent spells a block of a declaration the way the record keeps
// it: its lines counted from the declaration's first line, its columns as
// they are, then its statements.
func relativeExtent(s span, d declaration, statements int) string {
	return fmt.Sprintf("%d.%d,%d.%d %d",
		s.start.line-d.start.line, s.start.col, s.end.line-d.start.line, s.end.col, statements)
}

// absoluteBlock reads one reached block of a recorded declaration back into
// the span it takes in the file today and the statements it counts,
// refusing any spelling other than the one relativeExtent writes.
func absoluteBlock(file string, d declaration, extent string) (span, int, error) {
	var startLine, startCol, endLine, endCol, statements int
	_, err := fmt.Sscanf(extent, "%d.%d,%d.%d %d", &startLine, &startCol, &endLine, &endCol, &statements)
	s := span{
		file:  file,
		start: position{line: d.start.line + startLine, col: startCol},
		end:   position{line: d.start.line + endLine, col: endCol},
	}
	if err != nil || relativeExtent(s, d, statements) != extent {
		return span{}, 0, fmt.Errorf("the reached block %q is not spelled as the record writes one", extent)
	}
	return s, statements, nil
}

// encode renders the record as it is committed: indented JSON ending in a
// newline.
func (r *record) encode() []byte {
	return append(cmdutil.Must(json.MarshalIndent(r, "", "  ")), '\n')
}

// writeRecord writes the record to name.
func writeRecord(name string, rec *record) error {
	if err := os.WriteFile(name, rec.encode(), 0o600); err != nil {
		return fmt.Errorf("writing the record: %w", err)
	}
	return nil
}

// loadRecord reads the record at name. A record that does not exist yet,
// which is the state before the first complete run is recorded, is reported
// as not found rather than as an error; one that cannot be read, is not the
// layout this command writes, has text after it, is another schema, or
// counts what no package can, is refused.
func loadRecord(name string) (*record, bool, error) {
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	var rec *record
	if err == nil {
		rec, err = decodeRecord(data)
	}
	if err != nil {
		return nil, false, fmt.Errorf("record %s: %w", name, err)
	}
	return rec, true, nil
}

// decodeRecord reads a record in the layout this command writes, refusing a
// field the layout does not have, any text after the record but white space,
// since the gate would hold the packages to the first of two records and
// leave the rest unread, any other schema, and a package whose counts no
// merge leaves: fewer than no statements or none allowed, or more allowed
// than it has, which would let it lose every statement and pass.
func decodeRecord(data []byte) (*record, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var rec record
	if err := dec.Decode(&rec); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("text follows the record, and this command writes the record alone in its file")
	}
	if rec.Schema != recordSchema {
		return nil, fmt.Errorf("the record is schema %d, and this command reads schema %d", rec.Schema, recordSchema)
	}
	for _, p := range rec.Packages {
		if p.Statements < 0 || p.Unreached < 0 || p.Unreached > p.Statements {
			return nil, fmt.Errorf("%s counts %d statements and allows %d reached by neither half, which no package can",
				p.Package, p.Statements, p.Unreached)
		}
	}
	return &rec, nil
}

// matches refuses a record taken of other packages than the gate measures,
// or of the same ones in another order, since its ceilings are each a
// package's own.
func (r *record) matches(packages []string) error {
	recorded := make([]string, 0, len(r.Packages))
	for _, p := range r.Packages {
		recorded = append(recorded, p.Package)
	}
	asked := make([]string, 0, len(packages))
	for _, p := range packages {
		asked = append(asked, goprogram.TrimModulePath(p))
	}
	if !slices.Equal(recorded, asked) {
		return fmt.Errorf("the record measures %v, and the gate was asked for %v", recorded, asked)
	}
	return nil
}

// The reasons a recorded declaration's reach is dropped.
const (
	// whyChanged is a declaration whose text is no longer what the runs ran.
	whyChanged = "its text changed since the record"
	// whyGone is a declaration its file no longer declares.
	whyGone = "the file no longer declares it"
	// whyUnmeasured is a declaration of a file the offline half does not
	// measure, so it is not in the packages any longer.
	whyUnmeasured = "the offline tests no longer measure its file"
	// whyInstrumented is a declaration whose text is unchanged and whose
	// recorded blocks are not the blocks go test measures in it today, which
	// is what another toolchain's instrumentation looks like.
	whyInstrumented = "its blocks are not the ones this build instruments"
)

// dropped is a recorded declaration whose reach no longer counts, and why.
type dropped struct {
	file, declaration, why string
}

// credit folds the record's half into a merge that holds the offline half
// alone: the runs measured every block of a package the record says they
// did, and reached each recorded block of a declaration whose text is still
// what they ran. A declaration that no longer describes the tree is dropped
// whole, with the reason, rather than credited in part. A record whose reach
// cannot be read or lies outside its declaration, or whose file cannot be
// read, is refused.
func (c *coverage) credit(rec *record) ([]dropped, error) {
	files := map[string]string{}
	measured := map[string]bool{}
	for _, p := range rec.Packages {
		measured[p.Package] = p.MeasuredByRuns
	}
	for s, b := range c.blocks {
		files[goprogram.TrimModulePath(s.file)] = s.file
		b.seen[onlineHalf] = measured[goprogram.TrimModulePath(path.Dir(s.file))]
	}
	var (
		gone    []dropped
		decls   []declaration
		current string
	)
	for _, e := range rec.Declarations {
		file, ok := files[e.File]
		if !ok {
			gone = append(gone, dropped{file: e.File, declaration: e.Declaration, why: whyUnmeasured})
			continue
		}
		if file != current {
			var err error
			if decls, err = sourceDeclarations(file); err != nil {
				return nil, err
			}
			current = file
		}
		reached, why, err := c.match(file, decls, e)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", e.File, e.Declaration, err)
		}
		if why != "" {
			gone = append(gone, dropped{file: e.File, declaration: e.Declaration, why: why})
			continue
		}
		for _, s := range reached {
			c.blocks[s].hit[onlineHalf] = true
		}
	}
	return gone, nil
}

// match finds a recorded declaration in its file today and returns the
// spans of the blocks the runs reached in it, or why its reach no longer
// counts. A reached block outside the declaration is refused: the writer
// records each block under the declaration that holds it, and the text is
// unchanged, so such a block was put there by hand and would credit another
// declaration's block under this one's hash.
func (c *coverage) match(file string, decls []declaration, e recordDeclaration) ([]span, string, error) {
	i := slices.IndexFunc(decls, func(d declaration) bool { return d.key == e.Declaration })
	if i < 0 {
		return nil, whyGone, nil
	}
	if decls[i].sum != e.SHA256 {
		return nil, whyChanged, nil
	}
	reached := make([]span, 0, len(e.Reached))
	for _, extent := range e.Reached {
		s, statements, err := absoluteBlock(file, decls[i], extent)
		if err != nil {
			return nil, "", err
		}
		if !decls[i].holds(s) {
			return nil, "", fmt.Errorf("the reached block %q lies outside the declaration, and the record keeps each block under the declaration that holds it", extent)
		}
		if b, ok := c.blocks[s]; !ok || b.statements != statements {
			return nil, whyInstrumented, nil
		}
		reached = append(reached, s)
	}
	return reached, "", nil
}
