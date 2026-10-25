package main

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"
)

// gatingSeverities are the ratings at which a fixable finding fails the run.
var gatingSeverities = []string{"HIGH", "CRITICAL"}

// judged is one finding merged across every platform that carries it, and
// the declaration that accepts it, if one does.
type judged struct {
	finding
	declaration *declaration
	platforms   []string
}

// gates reports whether the finding fails the run unless declared: a fixed
// version exists and Trivy rates it HIGH or CRITICAL.
func (j judged) gates() bool {
	return j.fixed != "" && slices.Contains(gatingSeverities, j.severity)
}

// releasePlatforms are the platforms the release pushes, as [defaultPlatforms]
// names them.
var releasePlatforms = strings.Split(defaultPlatforms, ",")

// verdict is the outcome of one run. unjudged holds the declarations that
// accepted nothing in a run that did not scan every platform the release
// pushes, which that run cannot call stale.
type verdict struct {
	db                      database
	scans                   []scan
	findings, binaries      []judged
	stale, expired, invalid []string
	unjudged                []string
}

// judge merges the findings of every platform and holds the operating-system
// ones to the severity rule and the table, at now.
//
// A declaration no gating finding needed is one that has outlived what it
// excused, but only a run that scans every platform the release pushes can
// tell: one told to scan fewer may simply not hold the image the declaration
// was written for, so there it is unjudged rather than stale.
func judge(db database, scans []scan, declared map[string]declaration, now time.Time) verdict {
	v := verdict{db: db, scans: scans, invalid: invalidDeclarations(declared)}
	v.findings = merge(scans, func(s scan) []finding { return s.os })
	v.binaries = merge(scans, func(s scan) []finding { return s.binaries })

	matched := map[string]bool{}
	for i := range v.findings {
		key := declarationKey(v.findings[i].id, v.findings[i].pkg)
		if entry, ok := declared[key]; ok && v.findings[i].gates() {
			v.findings[i].declaration = &entry
			matched[key] = true
		}
	}
	complete := coversTheRelease(v.scanned())
	for key, entry := range declared {
		switch {
		case matched[key]:
			// It accepts a gating finding of this run.
		case complete:
			v.stale = append(v.stale, key)
		default:
			v.unjudged = append(v.unjudged, key)
		}
		// The day ends at midnight UTC, whatever zone the run is in.
		if last, err := time.Parse(time.DateOnly, entry.until); err == nil && entry.category == categoryFixNotYetAdoptable && !now.Before(last.AddDate(0, 0, 1)) {
			v.expired = append(v.expired, fmt.Sprintf("%q: its fix was to be adoptable by %s, and nothing has taken it", key, entry.until))
		}
	}
	sort.Strings(v.stale)
	sort.Strings(v.expired)
	sort.Strings(v.unjudged)
	return v
}

// scanned names the platform of every scan of the run, in the order scanned.
func (v verdict) scanned() []string {
	platforms := make([]string, 0, len(v.scans))
	for _, s := range v.scans {
		platforms = append(platforms, s.platform)
	}
	return platforms
}

// coversTheRelease reports whether the platforms scanned include every
// platform the release pushes.
func coversTheRelease(scanned []string) bool {
	for _, platform := range releasePlatforms {
		if !slices.Contains(scanned, platform) {
			return false
		}
	}
	return true
}

// merge gathers one kind of finding across the scans: one entry per advisory,
// package, version and rating, naming every platform that carries it in the
// order they were scanned, sorted by advisory and package.
func merge(scans []scan, of func(scan) []finding) []judged {
	merged := map[string]*judged{}
	var order []string
	for _, s := range scans {
		for _, f := range of(s) {
			key := strings.Join([]string{f.id, f.pkg, f.installed, f.fixed, f.severity, f.source, strings.Join(f.vendors, ","), f.title}, "\x00")
			j := merged[key]
			if j == nil {
				j = &judged{finding: f}
				merged[key] = j
				order = append(order, key)
			}
			j.platforms = append(j.platforms, s.platform)
		}
	}
	sort.Strings(order)
	out := make([]judged, 0, len(order))
	for _, key := range order {
		out = append(out, *merged[key])
	}
	return out
}

// undeclared counts the gating findings no declaration accepts.
func (v verdict) undeclared() int {
	n := 0
	for _, j := range v.findings {
		if j.gates() && j.declaration == nil {
			n++
		}
	}
	return n
}

// ok reports whether the run passes: nothing undeclared, and every
// declaration matched (or, in a run of fewer platforms than the release
// pushes, unjudged), current and readable.
func (v verdict) ok() bool {
	return v.undeclared() == 0 && len(v.stale) == 0 && len(v.expired) == 0 && len(v.invalid) == 0
}

// describe is how a finding is named on its line: the advisory, the package
// at its version, the fix, the rating and where the rating came from, and the
// platforms.
func (j judged) describe() string {
	fixed := "no fixed version"
	if j.fixed != "" {
		fixed = "fixed in " + j.fixed
	}
	source := "no source"
	if j.source != "" {
		source = j.source
	} else if len(j.vendors) > 0 {
		source = "other vendors (" + strings.Join(j.vendors, ", ") + ")"
	}
	return fmt.Sprintf("%s %s %s, %s, %s from %s, in %s", j.id, j.pkg, j.installed, fixed, j.severity, source, strings.Join(j.platforms, ", "))
}

// line is how an operating-system finding is printed: accepted, with the
// declaration that accepts it; a FINDING, when it gates and nothing does; and
// otherwise reported.
func (j judged) line() string {
	if d := j.declaration; d != nil {
		if d.until != "" {
			return fmt.Sprintf("accepted %s [%s until %s]: %s", j.describe(), d.category, d.until, d.reason)
		}
		return fmt.Sprintf("accepted %s [%s]: %s", j.describe(), d.category, d.reason)
	}
	if j.gates() {
		return fmt.Sprintf("FINDING %s: %s", j.describe(), j.title)
	}
	return "reported " + j.describe()
}

// write prints what was scanned and judged against, every finding, every
// declaration that did not hold or could not be judged, and the verdict.
func (v verdict) write(w io.Writer) {
	for _, s := range v.scans {
		fmt.Fprintf(w, "%s: %s: %s, %d OS packages, in %s, image %s, scanned by Trivy %s\n",
			toolName, s.platform, s.distro, s.packages, s.artifact, s.image, s.trivyVersion)
	}
	fmt.Fprintf(w, "%s: judged against Trivy's vulnerability database, schema %d, updated %s, next update due %s\n",
		toolName, v.db.schema, v.db.updatedAt.UTC().Format(time.RFC3339), v.db.nextUpdate.UTC().Format(time.RFC3339))
	fmt.Fprintf(w, "%s: an OS package finding fails the run when it has a fixed version and is rated HIGH or CRITICAL, unless declarations.go accepts it; "+
		"the rating is the one Trivy selects by default, from the advisory's own source (Alpine's secdb rates nothing), then NVD, then other vendors, "+
		"and each finding names where its rating came from\n", toolName)
	for _, j := range v.findings {
		fmt.Fprintln(w, j.line())
	}
	for _, j := range v.binaries {
		fmt.Fprintf(w, "binary %s: make check-binary-vulns judges the binary\n", j.describe())
	}
	for _, key := range v.stale {
		fmt.Fprintf(w, "STALE declaration %q: no image of this run carries a fixable HIGH or CRITICAL finding it accepts\n", key)
	}
	for _, key := range v.unjudged {
		fmt.Fprintf(w, "unjudged declaration %q: no image of this run carries a fixable HIGH or CRITICAL finding it accepts, "+
			"and the run scans %s rather than every platform the release pushes (%s), so a run that scans them all decides whether it is stale\n",
			key, strings.Join(v.scanned(), ", "), strings.Join(releasePlatforms, ", "))
	}
	for _, problem := range v.expired {
		fmt.Fprintf(w, "EXPIRED declaration %s\n", problem)
	}
	for _, problem := range v.invalid {
		fmt.Fprintf(w, "INVALID declaration %s\n", problem)
	}
	gating := 0
	for _, j := range v.findings {
		if j.gates() {
			gating++
		}
	}
	if !v.ok() {
		fmt.Fprintf(w, "%s: FAILED: %d undeclared findings, %d stale, %d expired and %d invalid declarations\n",
			toolName, v.undeclared(), len(v.stale), len(v.expired), len(v.invalid))
		return
	}
	if gating == 0 {
		fmt.Fprintf(w, "%s: no fixable HIGH or CRITICAL advisory affects an OS package of the %d images\n", toolName, len(v.scans))
		return
	}
	fmt.Fprintf(w, "%s: %d fixable HIGH or CRITICAL findings in the %d images, each accepted by a declaration\n", toolName, gating, len(v.scans))
}
