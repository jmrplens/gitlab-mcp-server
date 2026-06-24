// Command gen_1to1_backlog merges the three 1:1-audit gap reports — struct
// completeness (R-INPUT/R-OUTPUT), action coverage (R-ACTION), and metadata
// completeness (R-META) — into a single per-package backlog that drives the
// audit batches.
//
// It is a pure JSON transform over the reports emitted by audit_struct_
// completeness, audit_action_coverage, and audit_metadata_completeness; the
// make audit-1to1 target runs those auditors first and feeds their output here.
//
// Usage:
//
//	go run ./cmd/gen_1to1_backlog/ \
//	  -struct dist/1to1/struct.json \
//	  -action dist/1to1/action.json \
//	  -metadata dist/1to1/metadata.json \
//	  -output plan/1to1-backlog.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const schemaVersion = 1

func main() {
	structPath := flag.String("struct", "dist/1to1/struct.json", "path to audit_struct_completeness JSON")
	actionPath := flag.String("action", "dist/1to1/action.json", "path to audit_action_coverage JSON")
	metadataPath := flag.String("metadata", "dist/1to1/metadata.json", "path to audit_metadata_completeness JSON")
	outputPath := flag.String("output", "plan/1to1-backlog.json", "path to write the merged backlog JSON")
	flag.Parse()

	backlog, err := buildBacklog(*structPath, *actionPath, *metadataPath)
	if err != nil {
		cmdutil.Fatalf("build 1:1 backlog: %v", err)
	}
	content, err := json.MarshalIndent(backlog, "", "  ")
	if err != nil {
		cmdutil.Fatalf("marshal backlog: %v", err)
	}
	content = append(content, '\n')
	if writeErr := writeFile(*outputPath, content); writeErr != nil {
		cmdutil.Fatalf("write backlog: %v", writeErr)
	}
}

func buildBacklog(structPath, actionPath, metadataPath string) (backlog, error) {
	var structRep structReport
	if err := readJSON(structPath, &structRep); err != nil {
		return backlog{}, err
	}
	var actionRep actionReport
	if err := readJSON(actionPath, &actionRep); err != nil {
		return backlog{}, err
	}
	var metaRep metadataReport
	if err := readJSON(metadataPath, &metaRep); err != nil {
		return backlog{}, err
	}

	byPackage := map[string]*backlogPackage{}
	mergeStruct(byPackage, structRep)
	mergeActions(byPackage, actionRep)
	mergeMetadata(byPackage, metaRep)

	packages := make([]backlogPackage, 0, len(byPackage))
	for _, pkg := range byPackage {
		sort.Slice(pkg.Actions, func(i, j int) bool { return pkg.Actions[i].Service < pkg.Actions[j].Service })
		packages = append(packages, *pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Package < packages[j].Package })

	return backlog{
		SchemaVersion: schemaVersion,
		Note:          "Merged 1:1 audit backlog. Each stream is a candidate list; intentional renames (e.g. branch→branch_name) and deliberately unexposed endpoints are expected false positives a human adjudicates per package.",
		Summary:       summarizeBacklog(packages),
		Packages:      packages,
	}, nil
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

func writeFile(outputPath string, content []byte) error {
	if outputPath == "-" {
		_, err := os.Stdout.Write(content)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(outputPath, content, 0o600)
}
