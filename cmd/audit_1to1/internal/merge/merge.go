// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package merge

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

const backlogNote = "Merged 1:1 audit backlog. The struct and action streams are candidate lists; intentional renames (e.g. branch->branch_name) and deliberately unexposed endpoints are expected false positives a human adjudicates per package. A struct stale declaration is a key of one of the struct stream's declaration tables that answered no candidate: delete it, or restore what it described. The enum stream is gated by -scope=sdk and is expected to be empty: an entry there is an SDK value we do not offer, or a value we offer that the SDK does not declare. The summary counts the tree whichever packages are listed: action_missing_methods counts each client-go method once, however many packages call its service."

// BuildBacklogFromPaths reads three report files and returns the merged
// backlog as indented JSON (with a trailing newline). This is the file-based
// path, for reports that already exist on disk.
func BuildBacklogFromPaths(structPath, actionPath, enumPath string) ([]byte, error) {
	var structRep structReport
	if err := readJSON(structPath, &structRep); err != nil {
		return nil, err
	}
	var actionRep actionReport
	if err := readJSON(actionPath, &actionRep); err != nil {
		return nil, err
	}
	var enumRep enumReport
	if err := readJSON(enumPath, &enumRep); err != nil {
		return nil, err
	}
	return marshalBacklog(mergeBacklog(structRep, actionRep, enumRep))
}

// BuildBacklogFromBytes merges three in-memory report byte-slices and returns
// the merged backlog as indented JSON (with a trailing newline). This is the
// in-process path: audit_1to1 runs the three analyzers, marshals each result,
// and feeds the bytes here so the merged output is produced by exactly the same
// merge pipeline as BuildBacklogFromPaths — guaranteeing byte-for-byte
// equivalence with the file-based oracle.
func BuildBacklogFromBytes(structBytes, actionBytes, enumBytes []byte) ([]byte, error) {
	var structRep structReport
	if err := json.Unmarshal(structBytes, &structRep); err != nil {
		return nil, fmt.Errorf("parse struct report: %w", err)
	}
	var actionRep actionReport
	if err := json.Unmarshal(actionBytes, &actionRep); err != nil {
		return nil, fmt.Errorf("parse action report: %w", err)
	}
	var enumRep enumReport
	if err := json.Unmarshal(enumBytes, &enumRep); err != nil {
		return nil, fmt.Errorf("parse enum report: %w", err)
	}
	return marshalBacklog(mergeBacklog(structRep, actionRep, enumRep))
}

// mergeBacklog is the pure merge over the decoded input types. Both public
// entry points funnel through here so the merge logic exists in exactly one
// place.
func mergeBacklog(structRep structReport, actionRep actionReport, enumRep enumReport) backlog {
	byPackage := map[string]*backlogPackage{}
	mergeStruct(byPackage, structRep)
	mergeActions(byPackage, actionRep)
	mergeEnums(byPackage, enumRep)

	// Three-way comparisons rather than a less function: no two packages share
	// a name and no package lists a service twice, so a less written with < and
	// one written with <= would sort every report alike, and a mutation tool
	// would count the difference as a test missing.
	packages := make([]backlogPackage, 0, len(byPackage))
	for _, pkg := range byPackage {
		slices.SortFunc(pkg.Actions, func(a, b actionGap) int { return strings.Compare(a.Service, b.Service) })
		packages = append(packages, *pkg)
	}
	slices.SortFunc(packages, func(a, b backlogPackage) int { return strings.Compare(a.Package, b.Package) })

	return backlog{
		SchemaVersion:           shared.SchemaVersion,
		Note:                    backlogNote,
		Summary:                 summarizeBacklog(packages, structRep, actionRep, enumRep),
		StructStaleDeclarations: structRep.StaleDeclarations,
		Packages:                packages,
	}
}

// marshalIndent is the JSON encoder, a variable so a test can reach the
// encoding failure branch that a backlog of strings, ints and raw JSON never
// produces.
var marshalIndent = json.MarshalIndent

func marshalBacklog(b backlog) ([]byte, error) {
	content, err := marshalIndent(b, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal backlog: %w", err)
	}
	return append(content, '\n'), nil
}

func mergeStruct(byPackage map[string]*backlogPackage, rep structReport) {
	for _, pkg := range rep.Packages {
		entry := packageFor(byPackage, pkg.Package)
		entry.Struct = &structGaps{
			MissingInput:  pkg.MissingInputCount,
			MissingOutput: pkg.MissingOutputCount,
			ExtraOutput:   pkg.ExtraOutputCount,
			Gaps:          pkg.Gaps,
		}
	}
}

func mergeActions(byPackage map[string]*backlogPackage, rep actionReport) {
	for _, svc := range rep.Services {
		if len(svc.MissingMethods) == 0 {
			continue
		}
		for _, pkgName := range svc.Packages {
			entry := packageFor(byPackage, pkgName)
			entry.Actions = append(entry.Actions, actionGap{
				Service:        svc.Service,
				APIMethods:     svc.APIMethods,
				CoveredMethods: svc.CoveredMethods,
				MissingMethods: svc.MissingMethods,
			})
		}
	}
}

// mergeEnums attaches the enum stream to the packages that carry a value gap.
// A package listed with no findings (a full report of a clean package) is
// left out, the way a service with no missing method is.
func mergeEnums(byPackage map[string]*backlogPackage, rep enumReport) {
	for _, pkg := range rep.Packages {
		if pkg.MissingValues == 0 && pkg.ExtraValues == 0 {
			continue
		}
		entry := packageFor(byPackage, pkg.Package)
		entry.Enums = &enumGaps{
			MissingValues: pkg.MissingValues,
			ExtraValues:   pkg.ExtraValues,
			Findings:      pkg.Findings,
		}
	}
}

func packageFor(byPackage map[string]*backlogPackage, name string) *backlogPackage {
	entry, ok := byPackage[name]
	if !ok {
		entry = &backlogPackage{Package: name}
		byPackage[name] = entry
	}
	return entry
}

// summarizeBacklog takes each count from the stream that owns it rather than
// from packages[], the way each stream's own summary counts the tree whichever
// entries it lists, so -gaps-only, which shortens the list, changes no count.
// The struct and enum counts are those streams' summaries; packages is how
// many packages the struct diff ran over, every package of internal/tools
// with a pair. action_missing_methods counts each client-go method once, where
// packages[] attributes it to every package that calls its service.
// packages_with_gaps is counted over packages[], which holds every package
// with a finding in either mode.
func summarizeBacklog(packages []backlogPackage, structRep structReport, actionRep actionReport, enumRep enumReport) backlogSummary {
	s := backlogSummary{
		Packages:                structRep.Summary.Packages,
		StructMissingInput:      structRep.Summary.MissingInputFields,
		StructMissingOutput:     structRep.Summary.MissingOutputFields,
		StructExtraOutput:       structRep.Summary.ExtraOutputFields,
		StructTypeMismatches:    structRep.Summary.TypeMismatches,
		StructStaleDeclarations: len(structRep.StaleDeclarations),
		ActionMissingMethods:    missingMethods(actionRep),
		EnumMissingValues:       enumRep.Summary.MissingValues,
		EnumExtraValues:         enumRep.Summary.ExtraValues,
	}
	for _, pkg := range packages {
		if pkg.hasFindings() {
			s.PackagesWithGaps++
		}
	}
	return s
}

// missingMethods counts the distinct client-go methods the action report
// lists as missing, keyed by service and method, so a method of a service
// several packages call is one finding.
func missingMethods(rep actionReport) int {
	distinct := map[string]struct{}{}
	for _, svc := range rep.Services {
		for _, method := range svc.MissingMethods {
			distinct[svc.Service+"."+method] = struct{}{}
		}
	}
	return len(distinct)
}

func readJSON(path string, target any) error {
	content, err := os.ReadFile(path) // #nosec G304 -- audit report paths provided by the caller.
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if unmarshalErr := json.Unmarshal(content, target); unmarshalErr != nil {
		return fmt.Errorf("parse %s: %w", path, unmarshalErr)
	}
	return nil
}
