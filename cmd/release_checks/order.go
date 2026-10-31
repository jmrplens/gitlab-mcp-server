// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

// stableTag is the only tag a release is cut from: vMAJOR.MINOR.PATCH, with
// no prerelease or build suffix.
var stableTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// orderUsage is the command line of the order check.
const orderUsage = "usage: release_checks order TAG   (the tags the repository holds on stdin)\n"

// releaseOrder is where the tag being released stands among the tags the
// repository holds.
type releaseOrder struct {
	// tag is the tag being released.
	tag string
	// highest is the highest stable tag among tag and the listed ones.
	highest string
}

// newest reports whether no stable tag listed is higher than the tag being
// released.
func (o releaseOrder) newest() bool {
	return o.highest == o.tag
}

// distTag is the npm dist-tag the release publishes under: latest for the
// newest release, and release-MAJOR.MINOR for a backport, so npm's latest
// stays on the newest one.
func (o releaseOrder) distTag() string {
	if o.newest() {
		return "latest"
	}
	version := strings.TrimPrefix(o.tag, "v")
	return "release-" + version[:strings.LastIndexByte(version, '.')]
}

// runOrder runs the order check on its one argument, the tag being released,
// with the tags the repository holds read from stdin. It prints the three
// answers for $GITHUB_OUTPUT on stdout and returns 0, or refuses with an
// ::error:: line on stderr, nothing on stdout and 1, or returns 2 for
// arguments it cannot use.
func runOrder(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] == "" {
		fmt.Fprint(stderr, orderUsage)
		return 2
	}
	order, err := decideOrder(args[0], stdin)
	if err != nil {
		fmt.Fprintf(stderr, "::error::%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "stable=true\nnewest=%t\nnpm_dist_tag=%s\n", order.newest(), order.distTag())
	if !order.newest() {
		fmt.Fprintf(stderr, "%s is a backport: %s is newer, so the pointers to the newest release stay on it\n", order.tag, order.highest)
	}
	return 0
}

// decideOrder places tag among the tags listed in tags.
//
// A tag that is not a plain vX.Y.Z is refused before the list is read,
// because :latest, the GitHub release and the manifests on main would move
// to it. A list that cannot be read whole is refused too, since the tags it
// did not yield may hold a higher release.
func decideOrder(tag string, tags io.Reader) (releaseOrder, error) {
	if !stableTag.MatchString(tag) {
		return releaseOrder{}, fmt.Errorf("%s is not a release version: a release tag is vMAJOR.MINOR.PATCH, and a prerelease or build suffix is refused before anything publishes, because :latest, the GitHub release and the manifests on main would move to it", tag)
	}
	stable := []string{tag}
	lines := bufio.NewReader(tags)
	for {
		line, err := lines.ReadString('\n')
		if name := tagName(line); stableTag.MatchString(name) {
			stable = append(stable, name)
		}
		if errors.Is(err, io.EOF) {
			return releaseOrder{tag: tag, highest: slices.MaxFunc(stable, compareVersions)}, nil
		}
		if err != nil {
			return releaseOrder{}, fmt.Errorf("the tags the repository holds could not be read: %w", err)
		}
	}
}

// tagName is the tag a line of the list names: its last field, as
// `git ls-remote` prints the ref after the object name, with a refs/tags/
// prefix dropped. A line holding nothing names nothing.
func tagName(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[len(fields)-1], "refs/tags/")
}

// compareVersions orders two stable tags the way sort -V orders them:
// MAJOR, MINOR and PATCH compared as numbers in turn, and two tags whose
// numbers are equal but spelled apart (v3.1.0 and v3.01.0) by their bytes.
// It returns a negative number when a is lower, zero when the two are the
// same tag, and a positive number when a is higher.
func compareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range as {
		if c := compareNumbers(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return strings.Compare(a, b)
}

// compareNumbers orders two runs of decimal digits by the numbers they
// spell, whatever their length: the one with more digits after its leading
// zeros is higher, and two of one length are ordered digit by digit.
func compareNumbers(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if c := cmp.Compare(len(a), len(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}
