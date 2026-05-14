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
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
)

const (
	defaultOutputPath = "dist/action-spec-coverage.json"
	schemaVersion     = 1
)

type coverageReport struct {
	SchemaVersion int              `json:"schema_version"`
	Summary       coverageSummary  `json:"summary"`
	Domains       []domainCoverage `json:"domains"`
}

type coverageSummary struct {
	DomainCount                 int            `json:"domain_count"`
	RegisterToolsCount          int            `json:"register_tools_count"`
	RegisterMetaCount           int            `json:"register_meta_count"`
	ActionSpecDomainCount       int            `json:"action_spec_domain_count"`
	DynamicCatalogDomainCount   int            `json:"dynamic_catalog_domain_count"`
	StandaloneOnlyDomainCount   int            `json:"standalone_only_domain_count"`
	NoGitLabActionSurfaceCount  int            `json:"no_gitlab_action_surface_count"`
	SurfaceClassificationCounts map[string]int `json:"surface_classification_counts"`
}

type domainCoverage struct {
	Package                   string   `json:"package"`
	HasRegisterTools          bool     `json:"has_register_tools"`
	HasRegisterMeta           bool     `json:"has_register_meta"`
	HasMarkdown               bool     `json:"has_markdown"`
	HasTests                  bool     `json:"has_tests"`
	SurfaceClassification     string   `json:"surface_classification"`
	ClientType                string   `json:"client_type"`
	MetaGroup                 string   `json:"meta_group"`
	Notes                     []string `json:"notes"`
	RegisteredInRegisterAll   bool     `json:"registered_in_register_all"`
	DelegatedMeta             bool     `json:"delegated_meta"`
	HasMetaSpecs              bool     `json:"has_meta_specs"`
	HasIndividualTools        bool     `json:"has_individual_tools"`
	HasDynamicCatalogEntries  bool     `json:"has_dynamic_catalog_entries"`
	HasStandaloneOnlyTools    bool     `json:"has_standalone_only_tools"`
	ActionSpecCount           int      `json:"action_spec_count"`
	DynamicCatalogActionCount int      `json:"dynamic_catalog_action_count"`
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
	DynamicCatalogActionCount int
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
			coverage.DynamicCatalogActionCount = packageCoverage.DynamicCatalogActionCount
			coverage.HasMetaSpecs = coverage.HasMetaSpecs || packageCoverage.ActionSpecCount > 0
			coverage.HasDynamicCatalogEntries = packageCoverage.DynamicCatalogActionCount > 0
			coverage.MetaGroup = joinSortedSet(packageCoverage.MetaGroups)
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

	return coverageReport{
		SchemaVersion: schemaVersion,
		Summary:       summarizeCoverage(domains),
		Domains:       domains,
	}, nil
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
	client, err := gitlabclient.NewClient(&config.Config{ //#nosec G101 -- audit-only dummy token.
		GitLabURL:   config.DefaultGitLabURL,
		GitLabToken: "audit-token",
	})
	if err != nil {
		return nil, fmt.Errorf("create audit GitLab client: %w", err)
	}

	coverage := make(map[string]packageActionCoverage)
	for _, group := range tools.CollectActionSpecs(client, true) {
		for _, spec := range group.Specs {
			owner := strings.TrimSpace(spec.OwnerPackage)
			if owner == "" {
				continue
			}
			packageCoverage := coverageForPackage(coverage, owner)
			packageCoverage.ActionSpecCount++
			packageCoverage.MetaGroups[group.ToolName] = struct{}{}
			coverage[owner] = packageCoverage
		}
	}

	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add standalone dynamic catalog actions: %w", err)
	}
	for _, action := range catalog.Actions() {
		owner := actionOwnerPackage(action)
		if owner == "" {
			continue
		}
		packageCoverage := coverageForPackage(coverage, owner)
		packageCoverage.DynamicCatalogActionCount++
		packageCoverage.MetaGroups[action.ToolName] = struct{}{}
		coverage[owner] = packageCoverage
	}

	return coverage, nil
}

func coverageForPackage(coverage map[string]packageActionCoverage, packageName string) packageActionCoverage {
	packageCoverage := coverage[packageName]
	if packageCoverage.MetaGroups == nil {
		packageCoverage.MetaGroups = make(map[string]struct{})
	}
	return packageCoverage
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
	summary := coverageSummary{SurfaceClassificationCounts: make(map[string]int)}
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
		if domain.HasStandaloneOnlyTools {
			summary.StandaloneOnlyDomainCount++
		}
		if domain.SurfaceClassification == "no-gitlab-action-surface" {
			summary.NoGitLabActionSurfaceCount++
		}
		summary.SurfaceClassificationCounts[domain.SurfaceClassification]++
	}
	return summary
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
