// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// reproduce rebuilds every binary a release published from a clone of the
// commit -ref names, and compares each with the published one.
func reproduce(ctx context.Context, o options) (report, error) {
	work, err := os.MkdirTemp("", toolName+"-")
	if err != nil {
		return report{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()

	src := filepath.Join(work, "src")
	commit, err := cloneAt(ctx, o.dir, o.ref, src)
	if err != nil {
		return report{}, err
	}
	f := fields{
		Version: strings.TrimPrefix(o.tag, "v"), Tag: o.tag,
		Commit: commit, FullCommit: commit, ShortCommit: shortCommitOr(ctx, src, o.shortCommit),
	}
	rel, err := readRelease(inTree(src, o.config))
	if err != nil {
		return report{}, err
	}
	thin, fat, err := rel.recipes(f)
	if err != nil {
		return report{}, err
	}
	name, err := render(rel.checksums, f)
	if err != nil {
		return report{}, fmt.Errorf("checksum name_template: %w", err)
	}
	listPath := filepath.Join(o.published, name)
	sums, err := readChecksums(listPath)
	if err != nil {
		return report{}, err
	}

	rebuild := releaseRun{src: src, commit: commit, published: o.published, out: outputRoot(o.out, work), sums: sums}
	rep := report{sides: againstRelease, header: fmt.Sprintf("rebuilt %s from %s and compared it with %s", o.tag, commit, listPath)}
	built := make(map[string][]byte, len(thin))
	for _, rc := range thin {
		got, data, thinErr := rebuild.thin(ctx, rc)
		if thinErr != nil {
			return report{}, thinErr
		}
		rep.outcomes = append(rep.outcomes, got)
		built[rc.asset] = data
	}
	for _, fr := range fat {
		got, fatErr := rebuild.fat(fr, built)
		if fatErr != nil {
			return report{}, fatErr
		}
		rep.outcomes = append(rep.outcomes, got)
	}
	rep.notes = append(unbuiltNote(sums, rep.outcomes), shortCommitHint(rep, o.shortCommit, rel, f)...)
	return rep, nil
}

// shortCommitHint is the note a reproduction that differs carries when the
// configuration stamps {{.ShortCommit}} and the command line did not give
// it: git abbreviates a commit to as many characters as the clone's object
// count needs, so the clone that built the release may have used another
// length than this one, and the binary's --version prints the one it used.
func shortCommitHint(rep report, given string, rel release, f fields) []string {
	if rep.ok() || given != "" || !rel.usesShortCommit() {
		return nil
	}
	return []string{fmt.Sprintf(
		"the configuration stamps {{.ShortCommit}}, rendered as %s, git's abbreviation of the commit in this clone; the clone that built the release may have abbreviated it to another length, which -short-commit gives (the commit the binary's --version prints)",
		f.ShortCommit,
	)}
}

// releaseRun is one reproduction of a release: the clone it rebuilds from,
// the commit that clone is at, the published assets with the checksum file
// listing them, and where the rebuilt binaries go.
type releaseRun struct {
	src, commit, published, out string
	sums                        checksums
}

// thin rebuilds one binary with the toolchain the published one records,
// after holding the published one to the checksum file and to the commit,
// and returns the comparison and the rebuilt binary.
func (r releaseRun) thin(ctx context.Context, rc recipe) (outcome, []byte, error) {
	published, err := readPublished(r.sums, r.published, rc.asset)
	if err != nil {
		return outcome{}, nil, err
	}
	info, err := buildinfo.Read(bytes.NewReader(published))
	if err != nil {
		return outcome{}, nil, fmt.Errorf("the published %s carries no Go build information: %w", rc.asset, err)
	}
	toolchain, err := provenance(info, r.commit)
	if err != nil {
		return outcome{}, nil, fmt.Errorf("the published %s %w", rc.asset, err)
	}
	rebuilt, err := goBuild(ctx, r.src, rc, r.out, "GOTOOLCHAIN="+toolchain)
	if err != nil {
		return outcome{}, nil, err
	}
	return judge(rc.asset, rc.target.String()+" with "+toolchain, published, rebuilt, againstRelease), rebuilt, nil
}

// fat joins the rebuilt darwin binaries into the universal binary, in the
// order the published one's header lists its slices, keeps it where -out
// says, and compares it with the published one.
func (r releaseRun) fat(fr fatRecipe, built map[string][]byte) (outcome, error) {
	published, err := readPublished(r.sums, r.published, fr.asset)
	if err != nil {
		return outcome{}, err
	}
	order, err := readFatOrder(published)
	if err != nil {
		return outcome{}, fmt.Errorf("the published %s %w", fr.asset, err)
	}
	thins := make([][]byte, 0, len(fr.slices))
	for _, rc := range fr.slices {
		thins = append(thins, built[rc.asset])
	}
	joined, err := assembleFat(order, thins)
	if err != nil {
		return outcome{}, fmt.Errorf("%s: %w", fr.asset, err)
	}
	path := filepath.Join(r.out, fr.dir, fr.file)
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o750); mkdirErr != nil {
		return outcome{}, mkdirErr
	}
	if writeErr := os.WriteFile(path, joined, 0o600); writeErr != nil {
		return outcome{}, writeErr
	}
	return judge(fr.asset, "darwin/all", published, joined, againstRelease), nil
}

// unbuiltNote names what the checksum file lists that no recipe rebuilt,
// which for this repository is the SBOMs, the notices and the OpenVEX
// document, so a reader sees what the comparison did not cover.
func unbuiltNote(sums checksums, outcomes []outcome) []string {
	var rest []string
	for name := range sums.digests {
		if !slices.ContainsFunc(outcomes, func(o outcome) bool { return o.asset == name }) {
			rest = append(rest, name)
		}
	}
	if len(rest) == 0 {
		return nil
	}
	slices.Sort(rest)
	return []string{"not rebuilt, since the configuration builds none of them: " + strings.Join(rest, ", ")}
}

// inTree is path inside the module at root, unless path is absolute.
func inTree(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

// outputRoot is where the rebuilt binaries go: the absolute -out when given,
// and otherwise a directory inside the run's own, removed with it.
func outputRoot(out, work string) string {
	if out != "" {
		return out
	}
	return filepath.Join(work, "out")
}
