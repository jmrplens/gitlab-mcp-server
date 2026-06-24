// Command audit_action_coverage reports client-go SDK endpoints that no MCP
// action invokes (R-ACTION). For every package under internal/tools it resolves,
// with full Go type information, each call site of the form
// client.GL().{Service}.{Method}(...). The receiver type is a client-go service
// interface; its API methods are those whose signature ends in a variadic
// ...RequestOptionFunc. Methods on a used service that no handler calls are
// reported as candidate missing actions, grouped by the service and the
// internal/tools packages that reference it.
//
// The output is a candidate backlog, not a hard gate: a method may be
// intentionally unexposed, or owned by a sibling package. A human adjudicates
// each entry.
//
// Usage:
//
//	go run ./cmd/audit_action_coverage/                 # full report to stdout
//	go run ./cmd/audit_action_coverage/ -gaps-only      # only services with missing methods
//	go run ./cmd/audit_action_coverage/ -output dist/action-coverage.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const (
	schemaVersion       = 1
	clientGoPkgPath     = "gitlab.com/gitlab-org/api/client-go"
	toolsPkgInfix       = "/internal/tools/"
	requestOptionMarker = "RequestOptionFunc"
)

// serviceCoverage reports the SDK method coverage of one client-go service
// interface across the packages that reference it.
type serviceCoverage struct {
	Service        string   `json:"service"`
	Packages       []string `json:"packages"`
	APIMethods     int      `json:"api_methods"`
	CoveredMethods int      `json:"covered_methods"`
	MissingMethods []string `json:"missing_methods,omitempty"`
}

type report struct {
	SchemaVersion int               `json:"schema_version"`
	ClientGoPath  string            `json:"client_go_path"`
	Summary       reportSummary     `json:"summary"`
	Services      []serviceCoverage `json:"services"`
}

type reportSummary struct {
	Services         int `json:"services"`
	ServicesWithGaps int `json:"services_with_gaps"`
	APIMethods       int `json:"api_methods"`
	CoveredMethods   int `json:"covered_methods"`
	MissingMethods   int `json:"missing_methods"`
}

// serviceUsage accumulates, for one client-go service interface, the set of
// methods called and the internal/tools packages that reference it.
type serviceUsage struct {
	named  *types.Named
	called map[string]struct{}
	pkgs   map[string]struct{}
}

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include services that have at least one missing method")
	flag.Parse()

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}
	rep, err := buildReport(root, *gapsOnly)
	if err != nil {
		cmdutil.Fatalf("build action coverage report: %v", err)
	}
	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		cmdutil.Fatalf("marshal report: %v", err)
	}
	content = append(content, '\n')
	if writeErr := writeReport(*outputPath, content); writeErr != nil {
		cmdutil.Fatalf("write report: %v", writeErr)
	}
}

func buildReport(root string, gapsOnly bool) (report, error) {
	pkgs, err := loadToolPackages(root)
	if err != nil {
		return report{}, err
	}
	usage := map[string]*serviceUsage{}
	for _, pkg := range pkgs {
		collectServiceUsage(pkg, usage)
	}
	services := make([]serviceCoverage, 0, len(usage))
	for _, use := range usage {
		cov := coverageForService(use)
		if gapsOnly && len(cov.MissingMethods) == 0 {
			continue
		}
		services = append(services, cov)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Service < services[j].Service })
	return report{
		SchemaVersion: schemaVersion,
		ClientGoPath:  clientGoPkgPath,
		Summary:       summarize(services),
		Services:      services,
	}, nil
}

func loadToolPackages(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: root,
	}
	loaded, err := packages.Load(cfg, "./internal/tools/...")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	var fatal []string
	out := make([]*packages.Package, 0, len(loaded))
	for _, pkg := range loaded {
		for _, perr := range pkg.Errors {
			fatal = append(fatal, perr.Error())
		}
		if !strings.Contains(pkg.PkgPath, toolsPkgInfix) || pkg.TypesInfo == nil {
			continue
		}
		out = append(out, pkg)
	}
	if len(fatal) > 0 {
		return nil, fmt.Errorf("package load errors:\n%s", strings.Join(fatal, "\n"))
	}
	return out, nil
}

// collectServiceUsage walks every call expression in pkg and records calls whose
// receiver type is a client-go service interface.
func collectServiceUsage(pkg *packages.Package, usage map[string]*serviceUsage) {
	pkgName := shortPackage(pkg.PkgPath)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			named, ok := clientGoServiceInterface(pkg.TypesInfo.TypeOf(sel.X))
			if !ok {
				return true
			}
			use := usageFor(usage, named)
			use.called[sel.Sel.Name] = struct{}{}
			use.pkgs[pkgName] = struct{}{}
			return true
		})
	}
}

func usageFor(usage map[string]*serviceUsage, named *types.Named) *serviceUsage {
	key := named.Obj().Name()
	use, ok := usage[key]
	if !ok {
		use = &serviceUsage{named: named, called: map[string]struct{}{}, pkgs: map[string]struct{}{}}
		usage[key] = use
	}
	return use
}

func coverageForService(use *serviceUsage) serviceCoverage {
	apiMethods := apiMethodNames(use.named)
	covered := 0
	var missing []string
	for _, method := range apiMethods {
		if _, ok := use.called[method]; ok {
			covered++
			continue
		}
		missing = append(missing, method)
	}
	sort.Strings(missing)
	return serviceCoverage{
		Service:        use.named.Obj().Name(),
		Packages:       sortedSet(use.pkgs),
		APIMethods:     len(apiMethods),
		CoveredMethods: covered,
		MissingMethods: missing,
	}
}

// apiMethodNames returns the exported methods of a client-go service interface
// whose signature ends in a variadic ...RequestOptionFunc (the REST endpoint
// marker).
func apiMethodNames(named *types.Named) []string {
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	var names []string
	for method := range iface.Methods() {
		if !method.Exported() {
			continue
		}
		if sig, isSig := method.Type().(*types.Signature); isSig && signatureIsAPICall(sig) {
			names = append(names, method.Name())
		}
	}
	sort.Strings(names)
	return names
}

func signatureIsAPICall(sig *types.Signature) bool {
	if !sig.Variadic() || sig.Params().Len() == 0 {
		return false
	}
	last := sig.Params().At(sig.Params().Len() - 1)
	slice, ok := last.Type().(*types.Slice)
	if !ok {
		return false
	}
	named, ok := slice.Elem().(*types.Named)
	if !ok {
		return false
	}
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), clientGoPkgPath) {
		return false
	}
	return strings.Contains(named.Obj().Name(), requestOptionMarker)
}

// clientGoServiceInterface returns the named interface if t is a client-go
// service interface (e.g. BranchesServiceInterface).
func clientGoServiceInterface(t types.Type) (*types.Named, bool) {
	if t == nil {
		return nil, false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, false
	}
	if _, isIface := named.Underlying().(*types.Interface); !isIface {
		return nil, false
	}
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), clientGoPkgPath) {
		return nil, false
	}
	return named, true
}

func summarize(services []serviceCoverage) reportSummary {
	s := reportSummary{Services: len(services)}
	for _, svc := range services {
		if len(svc.MissingMethods) > 0 {
			s.ServicesWithGaps++
		}
		s.APIMethods += svc.APIMethods
		s.CoveredMethods += svc.CoveredMethods
		s.MissingMethods += len(svc.MissingMethods)
	}
	return s
}

func sortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func shortPackage(pkgPath string) string {
	_, after, ok := strings.Cut(pkgPath, toolsPkgInfix)
	if !ok {
		if last := strings.LastIndex(pkgPath, "/"); last >= 0 {
			return pkgPath[last+1:]
		}
		return pkgPath
	}
	return after
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
