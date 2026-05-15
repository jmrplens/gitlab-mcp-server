// Command audit_action_spec_coverage generates a source-discovered inventory of
// internal/tools domain coverage for the ActionSpec migration.
//
// Usage:
//
//	go run ./cmd/audit_action_spec_coverage/
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncompat"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/surfaces"
)

const (
	defaultOutputPath = "dist/action-spec-coverage.json"
	schemaVersion     = 1
)

var metaOnlyProjectionActions = map[string]string{
	"server.health_check": "meta-only alias for gitlab_server status; the individual surface uses gitlab_server_status",
}

type coverageReport struct {
	SchemaVersion int                `json:"schema_version"`
	Architecture  architectureReport `json:"architecture"`
	Summary       coverageSummary    `json:"summary"`
	Domains       []domainCoverage   `json:"domains"`
}

type architectureReport struct {
	CatalogSource                          string   `json:"catalog_source"`
	ManifestSource                         string   `json:"manifest_source"`
	MetaRegistrationSource                 string   `json:"meta_registration_source"`
	IndividualRegistrationSource           string   `json:"individual_registration_source"`
	DynamicAliasSource                     string   `json:"dynamic_alias_source"`
	SurfaceSpecCount                       int      `json:"surface_spec_count"`
	LegacyBridgeCount                      int      `json:"legacy_bridge_count"`
	LegacyBridges                          []string `json:"legacy_bridges,omitempty"`
	DynamicActionAliasCount                int      `json:"dynamic_action_alias_count"`
	DynamicParameterAliasCount             int      `json:"dynamic_parameter_alias_count"`
	DynamicSpecMetadataParameterAliasCount int      `json:"dynamic_spec_metadata_parameter_alias_count"`
}

type coverageSummary struct {
	DomainCount                 int            `json:"domain_count"`
	RegisterToolsCount          int            `json:"register_tools_count"`
	RegisterMetaCount           int            `json:"register_meta_count"`
	ActionSpecDomainCount       int            `json:"action_spec_domain_count"`
	DynamicCatalogDomainCount   int            `json:"dynamic_catalog_domain_count"`
	SurfaceSpecDomainCount      int            `json:"surface_spec_domain_count"`
	StandaloneOnlyDomainCount   int            `json:"standalone_only_domain_count"`
	NoGitLabActionSurfaceCount  int            `json:"no_gitlab_action_surface_count"`
	OrdinaryGitLabActionCount   int            `json:"ordinary_gitlab_action_count"`
	UtilitySurfaceActionCount   int            `json:"utility_surface_action_count"`
	SurfaceSpecCount            int            `json:"surface_spec_count"`
	SurfaceClassificationCounts map[string]int `json:"surface_classification_counts"`
	SurfaceKindCounts           map[string]int `json:"surface_kind_counts"`
}

type domainCoverage struct {
	Package                   string         `json:"package"`
	HasRegisterTools          bool           `json:"has_register_tools"`
	HasRegisterMeta           bool           `json:"has_register_meta"`
	HasMarkdown               bool           `json:"has_markdown"`
	HasTests                  bool           `json:"has_tests"`
	SurfaceClassification     string         `json:"surface_classification"`
	ClientType                string         `json:"client_type"`
	MetaGroup                 string         `json:"meta_group"`
	Notes                     []string       `json:"notes"`
	RegisteredInRegisterAll   bool           `json:"registered_in_register_all"`
	DelegatedMeta             bool           `json:"delegated_meta"`
	HasMetaSpecs              bool           `json:"has_meta_specs"`
	HasIndividualTools        bool           `json:"has_individual_tools"`
	HasDynamicCatalogEntries  bool           `json:"has_dynamic_catalog_entries"`
	HasSurfaceSpecs           bool           `json:"has_surface_specs"`
	HasStandaloneOnlyTools    bool           `json:"has_standalone_only_tools"`
	ActionSpecCount           int            `json:"action_spec_count"`
	OrdinaryGitLabActionCount int            `json:"ordinary_gitlab_action_count"`
	UtilitySurfaceActionCount int            `json:"utility_surface_action_count"`
	DynamicCatalogActionCount int            `json:"dynamic_catalog_action_count"`
	SurfaceSpecCount          int            `json:"surface_spec_count"`
	SurfaceKinds              []string       `json:"surface_kinds"`
	SurfaceKindCounts         map[string]int `json:"surface_kind_counts,omitempty"`
}

type domainSource struct {
	Package                       string
	HasRegisterTools              bool
	HasRegisterMeta               bool
	HasActionSpecsFunction        bool
	HasDynamicCatalogRegistration bool
	HasMarkdown                   bool
	HasTests                      bool
	ClientType                    string
}

type packageActionCoverage struct {
	ActionSpecCount           int
	OrdinaryGitLabActionCount int
	UtilitySurfaceActionCount int
	DynamicCatalogActionCount int
	SurfaceSpecCount          int
	SurfaceKindCounts         map[string]int
	MetaGroups                map[string]struct{}
}

func main() {
	outputPath := flag.String("output", defaultOutputPath, "path to write action spec coverage JSON, or '-' for stdout")
	flag.Parse()

	root, err := repositoryRoot(".")
	if err != nil {
		fatalf("find repository root: %v", err)
	}
	report, err := buildCoverageReport(root)
	if err != nil {
		fatalf("build coverage report: %v", err)
	}
	content, err := marshalReport(report)
	if err != nil {
		fatalf("marshal coverage report: %v", err)
	}
	writeErr := writeReport(*outputPath, content)
	if writeErr != nil {
		fatalf("write coverage report: %v", writeErr)
	}
}

func buildCoverageReport(root string) (coverageReport, error) {
	if err := auditCatalogFirstSource(root); err != nil {
		return coverageReport{}, err
	}
	sources, err := discoverDomainSources(root)
	if err != nil {
		return coverageReport{}, err
	}
	registeredPackages, err := referencedRegisterAllPackages(root)
	if err != nil {
		return coverageReport{}, err
	}
	delegatedMetaPackages, err := referencedRegisterMetaPackages(root)
	if err != nil {
		return coverageReport{}, err
	}
	actionCoverage, err := collectPackageActionCoverage()
	if err != nil {
		return coverageReport{}, err
	}

	domains := make([]domainCoverage, 0, len(sources))
	for _, source := range sources {
		coverage := domainCoverage{
			Package:                 source.Package,
			HasRegisterTools:        source.HasRegisterTools,
			HasRegisterMeta:         source.HasRegisterMeta,
			HasMarkdown:             source.HasMarkdown,
			HasTests:                source.HasTests,
			ClientType:              source.ClientType,
			RegisteredInRegisterAll: registeredPackages[source.Package],
			DelegatedMeta:           delegatedMetaPackages[source.Package],
			HasMetaSpecs:            source.HasActionSpecsFunction,
		}
		if packageCoverage, ok := actionCoverage[source.Package]; ok {
			coverage.ActionSpecCount = packageCoverage.ActionSpecCount
			coverage.OrdinaryGitLabActionCount = packageCoverage.OrdinaryGitLabActionCount
			coverage.UtilitySurfaceActionCount = packageCoverage.UtilitySurfaceActionCount
			coverage.DynamicCatalogActionCount = packageCoverage.DynamicCatalogActionCount
			coverage.SurfaceSpecCount = packageCoverage.SurfaceSpecCount
			coverage.HasSurfaceSpecs = packageCoverage.SurfaceSpecCount > 0
			coverage.HasMetaSpecs = coverage.HasMetaSpecs || packageCoverage.ActionSpecCount > 0
			coverage.HasDynamicCatalogEntries = packageCoverage.DynamicCatalogActionCount > 0
			coverage.MetaGroup = joinSortedSet(packageCoverage.MetaGroups)
			coverage.SurfaceKinds = surfaceKinds(packageCoverage.SurfaceKindCounts)
			coverage.SurfaceKindCounts = cloneStringIntMap(packageCoverage.SurfaceKindCounts)
		}
		coverage.HasIndividualTools = source.HasRegisterTools && isGitLabClientType(source.ClientType)
		coverage.HasStandaloneOnlyTools = source.HasRegisterTools && !coverage.HasIndividualTools
		coverage.SurfaceClassification = classifySurface(source, coverage)
		coverage.Notes = coverageNotes(source, coverage)
		domains = append(domains, coverage)
	}

	sort.Slice(domains, func(first, second int) bool {
		return domains[first].Package < domains[second].Package
	})
	if invariantErr := assertCoverageInvariants(domains); invariantErr != nil {
		return coverageReport{}, invariantErr
	}
	client, err := clientForAudit()
	if err != nil {
		return coverageReport{}, err
	}
	if projectionErr := assertCatalogActionsHaveIndividualProjectionPolicy(client); projectionErr != nil {
		return coverageReport{}, projectionErr
	}

	summary := summarizeCoverage(domains)
	architecture, err := buildArchitectureReport(root, summary)
	if err != nil {
		return coverageReport{}, err
	}

	return coverageReport{
		SchemaVersion: schemaVersion,
		Architecture:  architecture,
		Summary:       summary,
		Domains:       domains,
	}, nil
}

func assertCoverageInvariants(domains []domainCoverage) error {
	var gaps []string
	for _, domain := range domains {
		if domain.HasRegisterMeta {
			gaps = append(gaps, fmt.Sprintf("%s still defines package-level RegisterMeta", domain.Package))
		}
		if domain.HasIndividualTools && !domain.HasMetaSpecs {
			gaps = append(gaps, fmt.Sprintf("%s has GitLab-client RegisterTools without canonical ActionSpecs", domain.Package))
		}
		if domain.SurfaceClassification == "individual-only" {
			gaps = append(gaps, fmt.Sprintf("%s is individual-only; ordinary GitLab actions must be catalog-backed", domain.Package))
		}
	}
	if len(gaps) > 0 {
		sort.Strings(gaps)
		return fmt.Errorf("action spec coverage invariants failed: %s", strings.Join(gaps, "; "))
	}
	return nil
}

func assertCatalogActionsHaveIndividualProjectionPolicy(client *gitlabclient.Client) error {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return fmt.Errorf("build action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return fmt.Errorf("add standalone dynamic catalog actions: %w", err)
	}
	missing := catalogActionsMissingIndividualProjectionPolicy(catalog)
	if len(missing) > 0 {
		return fmt.Errorf("catalog actions missing individual projection policy: %s", strings.Join(missing, ", "))
	}
	return nil
}

func catalogActionsMissingIndividualProjectionPolicy(catalog *actioncatalog.Catalog) []string {
	if catalog == nil {
		return nil
	}
	var missing []string
	for _, group := range catalog.Groups() {
		for _, action := range group.ActionsInOrder() {
			if strings.TrimSpace(action.IndividualTool.Name) == "" {
				actionID := string(action.ID)
				if actionID == "" {
					actionID = group.ToolName + "." + action.Name
				}
				if _, ok := metaOnlyProjectionActions[actionID]; ok {
					continue
				}
				missing = append(missing, actionID)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func auditCatalogFirstSource(root string) error {
	if err := assertNoProductionSelectorCall(root, "toolutil", "CaptureMetaToolDefinitions"); err != nil {
		return err
	}
	if err := assertActionCatalogHasNoLegacyReferences(filepath.Join(root, "internal", "tools", "action_catalog.go")); err != nil {
		return err
	}
	if err := assertNoLegacyRuntimeBridges(root); err != nil {
		return err
	}
	if err := assertDynamicCompatibilityPolicyOwnedByActionCompat(root); err != nil {
		return err
	}
	return assertActionSpecManifestCurrent(root)
}

func buildArchitectureReport(root string, summary coverageSummary) (architectureReport, error) {
	bridges, err := legacyRuntimeBridgeFindings(root)
	if err != nil {
		return architectureReport{}, err
	}
	parameterAliases := actioncompat.ParameterAliases()
	specMetadataParameterAliasCount := 0
	for _, alias := range parameterAliases {
		if alias.SpecMetadata {
			specMetadataParameterAliasCount++
		}
	}
	return architectureReport{
		CatalogSource:                          "ActionSpec groups collected from action_specs_manifest_gen.go",
		ManifestSource:                         "internal/tools/action_specs_manifest_gen.go",
		MetaRegistrationSource:                 "catalog projection via RegisterMetaCatalog",
		IndividualRegistrationSource:           "catalog projection via RegisterIndividualCatalogTools",
		DynamicAliasSource:                     "actioncompat compatibility policy projected into ActionSpec metadata and Dynamic normalization",
		SurfaceSpecCount:                       summary.SurfaceSpecCount,
		LegacyBridgeCount:                      len(bridges),
		LegacyBridges:                          bridges,
		DynamicActionAliasCount:                len(actioncompat.ActionAliases()),
		DynamicParameterAliasCount:             len(parameterAliases),
		DynamicSpecMetadataParameterAliasCount: specMetadataParameterAliasCount,
	}, nil
}

func assertNoLegacyRuntimeBridges(root string) error {
	bridges, err := legacyRuntimeBridgeFindings(root)
	if err != nil {
		return err
	}
	if len(bridges) > 0 {
		return fmt.Errorf("production legacy bridge count = %d: %s", len(bridges), strings.Join(bridges, "; "))
	}
	return nil
}

func legacyRuntimeBridgeFindings(root string) ([]string, error) {
	checks := map[string][]string{
		filepath.Join(root, "internal", "tools", "action_catalog.go"): {
			"CaptureMetaToolDefinitions",
			"registerAllMetaGroups(",
			"groupFromMetaToolDefinition",
			".RegisterMeta(",
		},
		filepath.Join(root, "internal", "tools", "register_meta.go"): {
			"registerAllMetaGroups(",
			".RegisterMeta(",
		},
		filepath.Join(root, "internal", "tools", "register.go"): {
			".RegisterTools(",
			"registerAllLegacy",
			"legacyIndividualToolDescriptions",
			"listToolsForDescriptionCapture",
		},
		filepath.Join(root, "internal", "toolutil", "metatool.go"): {
			"CaptureMetaToolDefinitions",
			"MetaToolDefinition",
		},
	}
	findings := make([]string, 0)
	for path, forbidden := range checks {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		findings = append(findings, legacyBridgeFindingsInContent(path, string(content), forbidden)...)
	}
	sort.Strings(findings)
	return findings, nil
}

func legacyBridgeFindingsInContent(path, content string, forbidden []string) []string {
	findings := make([]string, 0)
	for _, needle := range forbidden {
		if strings.Contains(content, needle) {
			findings = append(findings, fmt.Sprintf("%s contains %q", path, needle))
		}
	}
	return findings
}

func assertDynamicCompatibilityPolicyOwnedByActionCompat(root string) error {
	path := filepath.Join(root, "internal", "tools", "dynamic", "register.go")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, forbidden := range []string{
		"return annotateCompatibilityAliases([]actionAlias{",
		"func buildSnippetCreateFilesFromSingleFileParams(",
		"func gitlabAccessLevelValue(",
		"func boolStringValue(",
	} {
		if strings.Contains(string(content), forbidden) {
			return fmt.Errorf("%s owns compatibility policy %q; move policy to actioncompat or ActionSpec metadata", path, forbidden)
		}
	}
	return nil
}

func assertNoProductionSelectorCall(root, qualifier, selectorName string) error {
	fileSet := token.NewFileSet()
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "dist" || name == "site" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		var found bool
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != selectorName {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && identifier.Name == qualifier {
				found = true
				return false
			}
			return true
		})
		if found {
			return fmt.Errorf("production source %s calls %s.%s; catalog construction must use specs directly", path, qualifier, selectorName)
		}
		return nil
	})
}

func assertActionCatalogHasNoLegacyReferences(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	source := string(content)
	for _, forbidden := range []string{"registerAllMetaGroups(", ".RegisterMeta("} {
		if strings.Contains(source, forbidden) {
			return fmt.Errorf("%s contains %q; BuildActionCatalog must not depend on legacy meta registration", path, forbidden)
		}
	}
	return nil
}

func assertActionSpecManifestCurrent(root string) error {
	sourceBuilders, err := discoverActionSpecGroupBuilderNames(filepath.Join(root, "internal", "tools"))
	if err != nil {
		return err
	}
	manifestBuilders, err := readManifestActionSpecGroupBuilders(filepath.Join(root, "internal", "tools", "action_specs_manifest_gen.go"))
	if err != nil {
		return err
	}
	if strings.Join(sourceBuilders, "\x00") != strings.Join(manifestBuilders, "\x00") {
		return fmt.Errorf("action spec manifest is stale: source builders %v, manifest builders %v; run go run ./cmd/gen_action_catalog_manifest/", sourceBuilders, manifestBuilders)
	}
	return nil
}

func discoverActionSpecGroupBuilderNames(toolsDir string) ([]string, error) {
	fileSet := token.NewFileSet()
	builders := make(map[string]string)
	err := filepath.WalkDir(toolsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != toolsDir {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !isActionSpecGroupBuilderName(function.Name.Name) {
				continue
			}
			if previousPath, exists := builders[function.Name.Name]; exists {
				return fmt.Errorf("duplicate action spec group builder %s in %s and %s", function.Name.Name, previousPath, path)
			}
			builders[function.Name.Name] = path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(builders) == 0 {
		return nil, errors.New("no action spec group builders found")
	}
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func readManifestActionSpecGroupBuilders(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "actionSpecGroupBuilders" {
			continue
		}
		return manifestBuilderNames(function), nil
	}
	return nil, fmt.Errorf("%s does not define actionSpecGroupBuilders", path)
}

func manifestBuilderNames(function *ast.FuncDecl) []string {
	var names []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		returnStmt, ok := node.(*ast.ReturnStmt)
		if !ok || len(returnStmt.Results) != 1 {
			return true
		}
		literal, ok := returnStmt.Results[0].(*ast.CompositeLit)
		if !ok {
			return false
		}
		for _, element := range literal.Elts {
			identifier, isIdentifier := element.(*ast.Ident)
			if isIdentifier {
				names = append(names, identifier.Name)
			}
		}
		return false
	})
	return names
}

func isActionSpecGroupBuilderName(name string) bool {
	return strings.HasPrefix(name, "build") && strings.HasSuffix(name, "ActionSpecs") && len(name) > len("buildActionSpecs")
}

func discoverDomainSources(root string) ([]domainSource, error) {
	toolsDir := filepath.Join(root, "internal", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("read tools directory: %w", err)
	}

	sources := make([]domainSource, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		source, inspectErr := inspectDomainSource(filepath.Join(toolsDir, entry.Name()), entry.Name())
		if inspectErr != nil {
			return nil, inspectErr
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func inspectDomainSource(domainDir, packageName string) (domainSource, error) {
	source := domainSource{Package: packageName}
	fileSet := token.NewFileSet()
	err := filepath.WalkDir(domainDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != domainDir {
				return filepath.SkipDir
			}
			return nil
		}

		name := entry.Name()
		if name == "markdown.go" {
			source.HasMarkdown = true
		}
		if strings.HasSuffix(name, "_test.go") {
			source.HasTests = true
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil {
				continue
			}
			switch function.Name.Name {
			case "RegisterTools":
				source.HasRegisterTools = true
				source.ClientType = registerToolsClientType(fileSet, function)
			case "RegisterMeta":
				source.HasRegisterMeta = true
			case "ActionSpecs":
				source.HasActionSpecsFunction = true
			case "RegisterCatalogTools", "RegisterCatalogFindExecuteTools":
				source.HasDynamicCatalogRegistration = true
			}
		}
		return nil
	})
	if err != nil {
		return domainSource{}, err
	}
	return source, nil
}

func registerToolsClientType(fileSet *token.FileSet, function *ast.FuncDecl) string {
	if function.Type.Params == nil {
		return ""
	}
	for _, field := range function.Type.Params.List {
		typeName := exprString(fileSet, field.Type)
		if typeName == "*mcp.Server" {
			continue
		}
		return typeName
	}
	return ""
}

func exprString(fileSet *token.FileSet, expression ast.Expr) string {
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fileSet, expression); err != nil {
		return ""
	}
	return buffer.String()
}

func referencedRegisterAllPackages(root string) (map[string]bool, error) {
	return referencedPackages(filepath.Join(root, "internal", "tools", "register.go"), "RegisterTools")
}

func referencedRegisterMetaPackages(root string) (map[string]bool, error) {
	return referencedPackages(filepath.Join(root, "internal", "tools", "register_meta.go"), "RegisterMeta")
}

func referencedPackages(path, selectorName string) (map[string]bool, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	references := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != selectorName {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		references[identifier.Name] = true
		return true
	})
	return references, nil
}

func collectPackageActionCoverage() (map[string]packageActionCoverage, error) {
	client, err := clientForAudit()
	if err != nil {
		return nil, err
	}

	coverage := make(map[string]packageActionCoverage)
	for _, group := range tools.CollectActionSpecs(client, true) {
		kind := normalizedSurfaceKind(group.SurfaceKind)
		for _, spec := range group.Actions {
			owner := strings.TrimSpace(spec.OwnerPackage)
			if owner == "" {
				continue
			}
			packageCoverage := coverageForPackage(coverage, owner)
			packageCoverage.ActionSpecCount++
			packageCoverage.recordActionKind(kind)
			packageCoverage.MetaGroups[group.ToolName] = struct{}{}
			coverage[owner] = packageCoverage
		}
	}
	recordSurfaceSpecs(coverage, collectSurfaceSpecs(client))

	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add standalone dynamic catalog actions: %w", err)
	}
	for _, group := range catalog.Groups() {
		for _, action := range group.ActionsInOrder() {
			owner := actionOwnerPackage(action)
			if owner == "" {
				continue
			}
			packageCoverage := coverageForPackage(coverage, owner)
			packageCoverage.DynamicCatalogActionCount++
			packageCoverage.MetaGroups[action.ToolName] = struct{}{}
			coverage[owner] = packageCoverage
		}
	}

	return coverage, nil
}

func clientForAudit() (*gitlabclient.Client, error) {
	client, err := gitlabclient.NewClient(&config.Config{ //#nosec G101 -- audit-only dummy token.
		GitLabURL:   config.DefaultGitLabURL,
		GitLabToken: "audit-token",
	})
	if err != nil {
		return nil, fmt.Errorf("create audit GitLab client: %w", err)
	}
	return client, nil
}

func collectSurfaceSpecs(client *gitlabclient.Client) []actioncatalog.SurfaceToolSpec {
	updater := autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Mode:           autoupdate.ModeCheck,
		Repository:     autoupdate.DefaultRepository,
		CurrentVersion: "0.0.0",
	}, nil)
	specs := make([]actioncatalog.SurfaceToolSpec, 0, 11)
	specs = append(specs, surfaces.StandaloneToolSpecs(client)...)
	specs = append(specs, surfaces.ServerMaintenanceToolSpecs(updater)...)
	specs = append(specs, dynamictools.ControllerSurfaceSpecs(nil, true)...)
	return specs
}

func recordSurfaceSpecs(coverage map[string]packageActionCoverage, specs []actioncatalog.SurfaceToolSpec) {
	for _, spec := range specs {
		owner := strings.TrimSpace(spec.OwnerPackage)
		if owner == "" {
			continue
		}
		kind := normalizedSurfaceKind(spec.SurfaceKind)
		packageCoverage := coverageForPackage(coverage, owner)
		packageCoverage.SurfaceSpecCount++
		packageCoverage.recordActionKind(kind)
		packageCoverage.MetaGroups[spec.GroupToolName] = struct{}{}
		coverage[owner] = packageCoverage
	}
}

func coverageForPackage(coverage map[string]packageActionCoverage, packageName string) packageActionCoverage {
	packageCoverage := coverage[packageName]
	if packageCoverage.MetaGroups == nil {
		packageCoverage.MetaGroups = make(map[string]struct{})
	}
	if packageCoverage.SurfaceKindCounts == nil {
		packageCoverage.SurfaceKindCounts = make(map[string]int)
	}
	return packageCoverage
}

func (coverage *packageActionCoverage) recordActionKind(kind actioncatalog.SurfaceKind) {
	if isOrdinaryGitLabActionKind(kind) {
		coverage.OrdinaryGitLabActionCount++
	} else {
		coverage.UtilitySurfaceActionCount++
	}
	coverage.recordSurfaceKind(kind)
}

func (coverage *packageActionCoverage) recordSurfaceKind(kind actioncatalog.SurfaceKind) {
	if coverage.SurfaceKindCounts == nil {
		coverage.SurfaceKindCounts = make(map[string]int)
	}
	coverage.SurfaceKindCounts[string(kind)]++
}

func normalizedSurfaceKind(kind actioncatalog.SurfaceKind) actioncatalog.SurfaceKind {
	if kind == "" {
		return actioncatalog.SurfaceKindMetaGroup
	}
	return kind
}

func isOrdinaryGitLabActionKind(kind actioncatalog.SurfaceKind) bool {
	switch normalizedSurfaceKind(kind) {
	case actioncatalog.SurfaceKindGitLabAction, actioncatalog.SurfaceKindMetaGroup:
		return true
	default:
		return false
	}
}

func actionOwnerPackage(action actioncatalog.Action) string {
	owner := strings.TrimSpace(action.OwnerPackage)
	if owner != "" {
		return owner
	}
	return strings.TrimSpace(action.Domain)
}

func classifySurface(source domainSource, coverage domainCoverage) string {
	switch {
	case source.HasDynamicCatalogRegistration && coverage.HasSurfaceSpecs:
		return "dynamic-controller-surface"
	case coverage.HasSurfaceSpecs || coverage.UtilitySurfaceActionCount > 0:
		return "surface-backed"
	case source.HasDynamicCatalogRegistration:
		return "dynamic-catalog-surface"
	case coverage.HasIndividualTools && (coverage.HasMetaSpecs || coverage.HasDynamicCatalogEntries):
		return "spec-backed"
	case coverage.HasIndividualTools:
		return "individual-only"
	case source.HasRegisterMeta && coverage.HasDynamicCatalogEntries:
		return "standalone-meta"
	case coverage.HasMetaSpecs || coverage.HasDynamicCatalogEntries:
		return "catalog-only"
	case coverage.HasStandaloneOnlyTools || source.HasRegisterMeta:
		return "standalone-only"
	default:
		return "no-gitlab-action-surface"
	}
}

func coverageNotes(source domainSource, coverage domainCoverage) []string {
	notes := make([]string, 0, 4)
	if source.HasDynamicCatalogRegistration {
		notes = append(notes, "dynamic search/describe/execute surface registered from the canonical action catalog")
	}
	if coverage.HasSurfaceSpecs {
		notes = append(notes, fmt.Sprintf("%d explicit surface specs: %s", coverage.SurfaceSpecCount, strings.Join(coverage.SurfaceKinds, ",")))
	}
	if coverage.UtilitySurfaceActionCount > 0 {
		notes = append(notes, fmt.Sprintf("%d utility/controller actions are outside ordinary GitLab API action counting", coverage.UtilitySurfaceActionCount))
	}
	if coverage.HasStandaloneOnlyTools {
		notes = append(notes, "RegisterTools does not use a GitLab client constructor")
	}
	if source.HasRegisterTools && !coverage.RegisteredInRegisterAll {
		notes = append(notes, "RegisterTools is not referenced from internal/tools/register.go")
	}
	if source.HasRegisterMeta && coverage.DelegatedMeta {
		notes = append(notes, "delegated RegisterMeta is referenced from internal/tools/register_meta.go")
	}
	if coverage.SurfaceClassification == "no-gitlab-action-surface" {
		notes = append(notes, "no GitLab action surface discovered from source or catalog metadata")
	}
	return notes
}

func summarizeCoverage(domains []domainCoverage) coverageSummary {
	summary := coverageSummary{
		SurfaceClassificationCounts: make(map[string]int),
		SurfaceKindCounts:           make(map[string]int),
	}
	for _, domain := range domains {
		summary.DomainCount++
		if domain.HasRegisterTools {
			summary.RegisterToolsCount++
		}
		if domain.HasRegisterMeta {
			summary.RegisterMetaCount++
		}
		if domain.HasMetaSpecs {
			summary.ActionSpecDomainCount++
		}
		if domain.HasDynamicCatalogEntries {
			summary.DynamicCatalogDomainCount++
		}
		if domain.HasSurfaceSpecs {
			summary.SurfaceSpecDomainCount++
		}
		if domain.HasStandaloneOnlyTools {
			summary.StandaloneOnlyDomainCount++
		}
		if domain.SurfaceClassification == "no-gitlab-action-surface" {
			summary.NoGitLabActionSurfaceCount++
		}
		summary.OrdinaryGitLabActionCount += domain.OrdinaryGitLabActionCount
		summary.UtilitySurfaceActionCount += domain.UtilitySurfaceActionCount
		summary.SurfaceSpecCount += domain.SurfaceSpecCount
		summary.SurfaceClassificationCounts[domain.SurfaceClassification]++
		for kind, count := range domain.SurfaceKindCounts {
			summary.SurfaceKindCounts[kind] += count
		}
	}
	return summary
}

func surfaceKinds(counts map[string]int) []string {
	if len(counts) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		if kind != "" {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}

func isGitLabClientType(typeName string) bool {
	return strings.Contains(typeName, "gitlab") && strings.Contains(typeName, "Client")
}

func joinSortedSet(values map[string]struct{}) string {
	if len(values) == 0 {
		return ""
	}
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func cloneStringIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	return maps.Clone(values)
}

func marshalReport(report coverageReport) ([]byte, error) {
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func writeReport(outputPath string, content []byte) error {
	if outputPath == "-" {
		_, err := os.Stdout.Write(content)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(outputPath, content, 0o600)
}

func repositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", start)
		}
		current = parent
	}
}

func fatalf(message string, args ...any) {
	fmt.Fprintf(os.Stderr, message+"\n", args...)
	os.Exit(1)
}
