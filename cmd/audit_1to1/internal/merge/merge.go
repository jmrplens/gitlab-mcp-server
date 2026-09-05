package merge

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
)

const backlogNote = "Merged 1:1 audit backlog. The struct, action and metadata streams are candidate lists; intentional renames (e.g. branch->branch_name) and deliberately unexposed endpoints are expected false positives a human adjudicates per package. The enum stream is gated by -scope=sdk and is expected to be empty: an entry there is an SDK value we do not offer, or a value we offer that the SDK does not declare."

// BuildBacklogFromPaths reads four report files and returns the merged backlog
// as indented JSON (with a trailing newline). This is the file-based path used
// when the auditor reports already exist on disk (e.g. via the make target).
func BuildBacklogFromPaths(structPath, actionPath, metadataPath, enumPath string) ([]byte, error) {
	var structRep structReport
	if err := readJSON(structPath, &structRep); err != nil {
		return nil, err
	}
	var actionRep actionReport
	if err := readJSON(actionPath, &actionRep); err != nil {
		return nil, err
	}
	var metaRep metadataReport
	if err := readJSON(metadataPath, &metaRep); err != nil {
		return nil, err
	}
	var enumRep enumReport
	if err := readJSON(enumPath, &enumRep); err != nil {
		return nil, err
	}
	return marshalBacklog(mergeBacklog(structRep, actionRep, metaRep, enumRep))
}

// BuildBacklogFromBytes merges four in-memory report byte-slices and returns
// the merged backlog as indented JSON (with a trailing newline). This is the
// in-process path: audit_1to1 runs the four analyzers, marshals each result,
// and feeds the bytes here so the merged output is produced by exactly the same
// merge pipeline as BuildBacklogFromPaths — guaranteeing byte-for-byte
// equivalence with the file-based oracle.
func BuildBacklogFromBytes(structBytes, actionBytes, metadataBytes, enumBytes []byte) ([]byte, error) {
	var structRep structReport
	if err := json.Unmarshal(structBytes, &structRep); err != nil {
		return nil, fmt.Errorf("parse struct report: %w", err)
	}
	var actionRep actionReport
	if err := json.Unmarshal(actionBytes, &actionRep); err != nil {
		return nil, fmt.Errorf("parse action report: %w", err)
	}
	var metaRep metadataReport
	if err := json.Unmarshal(metadataBytes, &metaRep); err != nil {
		return nil, fmt.Errorf("parse metadata report: %w", err)
	}
	var enumRep enumReport
	if err := json.Unmarshal(enumBytes, &enumRep); err != nil {
		return nil, fmt.Errorf("parse enum report: %w", err)
	}
	return marshalBacklog(mergeBacklog(structRep, actionRep, metaRep, enumRep))
}

// mergeBacklog is the pure merge over the decoded input types. Both public
// entry points funnel through here so the merge logic exists in exactly one
// place.
func mergeBacklog(structRep structReport, actionRep actionReport, metaRep metadataReport, enumRep enumReport) backlog {
	byPackage := map[string]*backlogPackage{}
	mergeStruct(byPackage, structRep)
	mergeActions(byPackage, actionRep)
	mergeMetadata(byPackage, metaRep)
	mergeEnums(byPackage, enumRep)

	packages := make([]backlogPackage, 0, len(byPackage))
	for _, pkg := range byPackage {
		sort.Slice(pkg.Actions, func(i, j int) bool { return pkg.Actions[i].Service < pkg.Actions[j].Service })
		packages = append(packages, *pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Package < packages[j].Package })

	return backlog{
		SchemaVersion: shared.SchemaVersion,
		Note:          backlogNote,
		Summary:       summarizeBacklog(packages),
		Packages:      packages,
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

func mergeMetadata(byPackage map[string]*backlogPackage, rep metadataReport) {
	for _, pkg := range rep.Packages {
		if len(pkg.Findings) == 0 {
			continue
		}
		entry := packageFor(byPackage, pkg.Package)
		entry.Metadata = &metadataGaps{Findings: pkg.Findings}
		for _, finding := range pkg.Findings {
			for _, flag := range finding.Flags {
				switch flag {
				case "generic_usage":
					entry.Metadata.GenericUsage++
				case "aliases_only_toolname":
					entry.Metadata.AliasesOnlyToolname++
				case "empty_related":
					entry.Metadata.EmptyRelated++
				case "weak_individual_description":
					entry.Metadata.WeakIndividualDescription++
				}
			}
		}
	}
}

// mergeEnums attaches the enum stream to the packages that carry a value gap.
// A package listed with no findings (a full report of a clean package) is
// left out, the way the metadata stream is.
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

func summarizeBacklog(packages []backlogPackage) backlogSummary {
	s := backlogSummary{Packages: len(packages)}
	for _, pkg := range packages {
		if pkg.Struct != nil {
			s.StructMissingInput += pkg.Struct.MissingInput
			s.StructMissingOutput += pkg.Struct.MissingOutput
			s.StructExtraOutput += pkg.Struct.ExtraOutput
		}
		for _, action := range pkg.Actions {
			s.ActionMissingMethods += len(action.MissingMethods)
		}
		if pkg.Metadata != nil {
			s.MetaGenericUsage += pkg.Metadata.GenericUsage
			s.MetaAliasesOnlyToolname += pkg.Metadata.AliasesOnlyToolname
			s.MetaEmptyRelated += pkg.Metadata.EmptyRelated
			s.MetaWeakIndividualDescription += pkg.Metadata.WeakIndividualDescription
		}
		if pkg.Enums != nil {
			s.EnumMissingValues += pkg.Enums.MissingValues
			s.EnumExtraValues += pkg.Enums.ExtraValues
		}
	}
	return s
}

func readJSON(path string, target any) error {
	content, err := os.ReadFile(path) // #nosec G304 -- audit report paths provided by the make target.
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if unmarshalErr := json.Unmarshal(content, target); unmarshalErr != nil {
		return fmt.Errorf("parse %s: %w", path, unmarshalErr)
	}
	return nil
}
