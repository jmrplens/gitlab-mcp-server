// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// repeat builds every release target of the tree at -dir twice, the second
// time with an empty build cache, and compares the two builds.
//
// It is the check a contributor can run anywhere, offline once the module
// cache holds the modules: it needs no release and no git, which is how it
// runs on a copy of the tree synced without .git. A build cache that is
// empty for the second build means every package is compiled again rather
// than reused from the first, so the two binaries agreeing is the compiler
// and the linker agreeing with themselves, not the cache agreeing with
// itself.
func repeat(ctx context.Context, o options) (report, error) {
	work, err := os.MkdirTemp("", toolName+"-")
	if err != nil {
		return report{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()

	tag, err := tagOf(o.dir, o.tag)
	if err != nil {
		return report{}, err
	}
	commit := gitOrNone(ctx, o.dir, "rev-parse", "HEAD")
	f := fields{
		Version: strings.TrimPrefix(tag, "v"), Tag: tag,
		Commit: commit, FullCommit: commit, ShortCommit: shortCommitOr(ctx, o.dir, o.shortCommit),
	}
	rel, err := readRelease(inTree(o.dir, o.config))
	if err != nil {
		return report{}, err
	}
	thin, fat, err := rel.recipes(f)
	if err != nil {
		return report{}, err
	}

	out := outputRoot(o.out, work)
	cache := filepath.Join(work, "gocache")
	builds := []struct {
		dir   string
		extra []string
	}{
		{dir: filepath.Join(out, "first")},
		{dir: filepath.Join(out, "second"), extra: []string{"GOCACHE=" + cache}},
	}
	rep := report{
		sides:  againstItself,
		header: fmt.Sprintf("built every release target of %s at %s twice from %s, the second time with an empty build cache (%s)", tag, commit, o.dir, cache),
	}
	for _, rc := range thin {
		var copies [2][]byte
		for i, b := range builds {
			data, buildErr := goBuild(ctx, o.dir, rc, b.dir, b.extra...)
			if buildErr != nil {
				return report{}, buildErr
			}
			copies[i] = data
		}
		rep.outcomes = append(rep.outcomes, judge(rc.asset, rc.target.String(), copies[0], copies[1], againstItself))
	}
	rep.notes = fatNote(fat)
	return rep, nil
}

// tagOf is the tag the offline check stamps: -tag when given, and v followed
// by the VERSION file of the tree otherwise, which is what a release of the
// tree would be tagged.
func tagOf(dir, tag string) (string, error) {
	if tag != "" {
		return tag, nil
	}
	version, err := os.ReadFile(filepath.Join(dir, "VERSION")) //#nosec G304 -- the VERSION file of the operator's own -dir
	if err != nil {
		return "", fmt.Errorf("no -tag given, and the tree's VERSION cannot say: %w", err)
	}
	return "v" + strings.TrimSpace(string(version)), nil
}

// fatNote names the universal binaries the offline check does not build: a
// universal binary joins the darwin binaries above in the order a release's
// own header lists them, which no build of the tree fixes, so it is compared
// against a release only.
func fatNote(fat []fatRecipe) []string {
	if len(fat) == 0 {
		return nil
	}
	names := make([]string, 0, len(fat))
	for _, fr := range fat {
		names = append(names, fr.asset)
	}
	return []string{"not built twice: " + strings.Join(names, ", ") + ", which joins the darwin binaries above in the order a release's own header lists them, so it is compared against a release only"}
}
