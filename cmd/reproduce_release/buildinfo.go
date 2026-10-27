// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"debug/buildinfo"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
)

// sides are the words a comparison uses for the two copies of a binary, and
// for its verdict: a published binary and its rebuild, or the two builds of
// the offline check.
type sides struct {
	// before and after name the two copies in a line that compares them.
	before, after string
	// same is the word a line opens with when the copies agree.
	same string
	// agree and differ end the verdict, after "N of M binaries".
	agree, differ string
}

var (
	// againstRelease are the words of a reproduction of a release.
	againstRelease = sides{
		before: "published", after: "rebuilt", same: "reproduced",
		agree: "rebuilt bit for bit", differ: "differ from what the release published",
	}
	// againstItself are the words of the offline check.
	againstItself = sides{
		before: "first build", after: "second build", same: "repeated",
		agree: "built twice to the same bytes", differ: "differ between the two builds",
	}
)

// setting returns the value info records under key, or "" when it records
// none.
func setting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// provenance holds a published binary's build information to the commit
// being rebuilt, and returns the toolchain it was built with, spelled the way
// GOTOOLCHAIN takes it.
//
// A rebuild says something only about the tree it was built from, so a
// binary that records another commit, or no commit, or a tree with changes
// nobody committed, is refused before anything is built, and so is one built
// by a toolchain GOTOOLCHAIN cannot name, since the rebuild has to use the
// same one. The messages read after the binary's name.
func provenance(info *debug.BuildInfo, commit string) (string, error) {
	switch revision := setting(info, "vcs.revision"); revision {
	case "":
		return "", errors.New("records no commit (no vcs.revision), so nothing says which tree it was built from")
	case commit:
	default:
		return "", fmt.Errorf("was built from %s, and the commit rebuilt is %s", revision, commit)
	}
	if modified := setting(info, "vcs.modified"); modified != "false" {
		return "", fmt.Errorf("was built from a tree with changes nobody committed (vcs.modified=%s)", modified)
	}
	toolchain, _, _ := strings.Cut(info.GoVersion, " ")
	if !strings.HasPrefix(toolchain, "go1.") {
		return "", fmt.Errorf("records the toolchain %q, which GOTOOLCHAIN cannot name", info.GoVersion)
	}
	return toolchain, nil
}

// differences names every part of the build information two binaries
// disagree on, in a fixed order: the toolchain, the main package, the main
// module, each module by path, each setting by key. What agrees is left out.
func differences(before, after *debug.BuildInfo, s sides) []string {
	var notes []string
	note := func(what, a, b string) {
		if a != b {
			notes = append(notes, fmt.Sprintf("%s: %s %q, %s %q", what, s.before, a, s.after, b))
		}
	}
	note("go version", before.GoVersion, after.GoVersion)
	note("main package", before.Path, after.Path)
	note("main module", moduleText(before.Main), moduleText(after.Main))
	beforeDeps, afterDeps := moduleTexts(before.Deps), moduleTexts(after.Deps)
	for _, path := range unionKeys(beforeDeps, afterDeps) {
		note("module "+path, beforeDeps[path], afterDeps[path])
	}
	beforeSet, afterSet := settingValues(before.Settings), settingValues(after.Settings)
	for _, key := range unionKeys(beforeSet, afterSet) {
		note("setting "+key, beforeSet[key], afterSet[key])
	}
	return notes
}

// moduleText is a module as a line compares it: its path, its version, its
// sum and what replaces it, each when there is one.
func moduleText(m debug.Module) string {
	text := m.Path
	if m.Version != "" {
		text += "@" + m.Version
	}
	if m.Sum != "" {
		text += " " + m.Sum
	}
	if m.Replace != nil {
		text += " => " + moduleText(*m.Replace)
	}
	return text
}

// moduleTexts is each module by its path.
func moduleTexts(deps []*debug.Module) map[string]string {
	texts := make(map[string]string, len(deps))
	for _, m := range deps {
		texts[m.Path] = moduleText(*m)
	}
	return texts
}

// settingValues is each setting's value by its key.
func settingValues(settings []debug.BuildSetting) map[string]string {
	values := make(map[string]string, len(settings))
	for _, s := range settings {
		values[s.Key] = s.Value
	}
	return values
}

// unionKeys is every key either map holds, sorted.
func unionKeys(a, b map[string]string) []string {
	keys := slices.Collect(maps.Keys(a))
	keys = slices.AppendSeq(keys, maps.Keys(b))
	slices.Sort(keys)
	return slices.Compact(keys)
}

// explain says why two copies of a binary differ: what their build
// information disagrees on when both carry it, or that it agrees, which
// points at what it does not record (with -trimpath, the linker flags among
// them), and where the bytes first part.
func explain(before, after []byte, s sides) []string {
	var notes []string
	beforeInfo, beforeErr := buildinfo.Read(bytes.NewReader(before))
	afterInfo, afterErr := buildinfo.Read(bytes.NewReader(after))
	if beforeErr == nil && afterErr == nil {
		notes = differences(beforeInfo, afterInfo, s)
		if len(notes) == 0 {
			notes = append(notes, "the build information is the same, so the difference is in what it does not record, the linker flags among them")
		}
	}
	return append(notes, firstDifference(before, after, s))
}

// firstDifference says at which offset two files first differ, and how long
// each is. For two that agree, the offset is where both end.
func firstDifference(before, after []byte, s sides) string {
	n := min(len(before), len(after))
	offset := 0
	for offset < n && before[offset] == after[offset] {
		offset++
	}
	return fmt.Sprintf("the bytes first differ at offset %d; the %s file is %d bytes and the %s one %d", offset, s.before, len(before), s.after, len(after))
}
