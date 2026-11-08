// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// stampSuffix is what the stamp of a run profile is named with in place of
// the profile's .out.
const stampSuffix = ".complete"

// stampPath names the stamp of a run profile, the file a complete run leaves
// beside it: <stem>.out's is <stem>.complete, and a profile named otherwise
// has the suffix added.
func stampPath(profile string) string {
	return strings.TrimSuffix(profile, ".out") + stampSuffix
}

// sourcesStamp returns the stamp of the packages as the tree holds them: a
// line for every Go file of each package's directory that is not a test, the
// module's root package's directory being the module root, each package once
// and by directory whatever order they are given in, as go test -coverpkg
// takes them in any, and the files by name, each with the SHA-256 of its
// bytes and its path below the module root, in sha256sum's format, so
// sha256sum -c checks a stamp from the module root. A file a build
// constraint leaves out is stamped too, so a stamp is never laxer than the
// build.
func sourcesStamp(packages []string) ([]byte, error) {
	dirs := make([]string, 0, len(packages))
	for _, pkg := range packages {
		dir := goprogram.TrimModulePath(pkg)
		if dir == "" {
			dir = "."
		}
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	var stamp bytes.Buffer
	for _, dir := range slices.Compact(dirs) {
		entries, err := os.ReadDir(filepath.FromSlash(dir))
		if err != nil {
			return nil, fmt.Errorf("stamping the sources of %s: %w", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file := path.Join(dir, name)
			data, readErr := os.ReadFile(filepath.FromSlash(file))
			if readErr != nil {
				return nil, fmt.Errorf("stamping the sources of %s: %w", dir, readErr)
			}
			fmt.Fprintf(&stamp, "%x  %s\n", sha256.Sum256(data), file)
		}
	}
	return stamp.Bytes(), nil
}

// writeStamp writes the stamp of the packages' sources to name, which is how
// run-docker-e2e.sh stamps a pass of a complete run before the pass builds
// its test binaries.
func writeStamp(name string, packages []string) error {
	stamp, err := sourcesStamp(packages)
	if err != nil {
		return err
	}
	if writeErr := os.WriteFile(name, stamp, 0o600); writeErr != nil {
		return fmt.Errorf("writing the stamp: %w", writeErr)
	}
	return nil
}

// checkStamps holds every run profile a record is taken of to the stamp of
// the packages' sources as this tree holds them, which a complete run leaves
// beside it. A profile with no stamp was not
// written by a complete run: run-docker-e2e.sh keeps a pass's stamp only once
// both passes passed and every skip was declared. A profile whose stamp is
// not the tree's was written from other sources, and the record would credit
// what the run reached in them to the declarations the tree holds now, whose
// blocks can be where the old ones were while the code differs.
func checkStamps(sources []source, packages []string) error {
	tree, err := sourcesStamp(packages)
	if err != nil {
		return err
	}
	for _, s := range sources {
		if s.half != onlineHalf {
			continue
		}
		name := stampPath(s.path)
		stamp, readErr := os.ReadFile(name)
		if errors.Is(readErr, fs.ErrNotExist) {
			return fmt.Errorf("%s has no stamp beside it (%s), which run-docker-e2e.sh leaves only after a complete run, so a record is not taken of it",
				s.path, filepath.Base(name))
		}
		if readErr != nil {
			return fmt.Errorf("reading the stamp of %s: %w", s.path, readErr)
		}
		if !bytes.Equal(stamp, tree) {
			return fmt.Errorf("%s was written from other sources than the tree holds, as its stamp %s says (sha256sum -c it from the module root to see which), so what the run reached is not this tree's",
				s.path, filepath.Base(name))
		}
	}
	return nil
}
