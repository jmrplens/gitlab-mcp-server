// Command audit_struct_completeness verifies the 1:1 field mapping between the
// MCP tool input/output structs and the client-go SDK Options/result structs
// they wrap.
//
// For every package under internal/tools it resolves, with full Go type
// information:
//
//   - input pairs: each &gl.XxxOptions{} composite literal constructed inside a
//     handler is attributed to that handler's MCP input struct. The SDK Options
//     fields (by url/json tag) are diffed against the MCP input fields (by json
//     tag) to find SDK fields with no MCP counterpart (R-INPUT) and advisory
//     Go-type divergences.
//   - output pairs: each converter func(...src *gl.Y...) LocalStruct maps an SDK
//     result struct to an MCP output struct. The SDK result fields (by json tag)
//     are diffed against the MCP output fields (R-OUTPUT).
//
// The report is the mechanical backlog that drives the 1:1 audit batches. It is
// intentionally high-signal on *missing fields* (the gap class that the
// client-go bumps repeatedly introduced) and advisory on type divergences,
// because the domain legitimately maps SDK enum/time types onto scalars.
//
// Usage:
//
//	go run ./cmd/audit_struct_completeness/                 # full report to stdout
//	go run ./cmd/audit_struct_completeness/ -gaps-only      # only packages with gaps
//	go run ./cmd/audit_struct_completeness/ -output dist/struct-completeness.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const (
	schemaVersion   = 1
	clientGoPkgPath = "gitlab.com/gitlab-org/api/client-go"
	toolsPkgInfix   = "/internal/tools/"
	optionsSuffix   = "Options"
	// responseStructName is the client-go pagination wrapper type. Converters that
	// take it as their SDK arg are list wrappers (the MCP list-output holds a
	// slice-of-element field plus pagination); they are validated through the
	// element converter, never paired against the wrapper. See nonResultSDKStruct.
	responseStructName = "Response"
)

// acceptedOutputRenames suppresses specific MCP output json tags from
// R-OUTPUT-EXTRA: deliberate 1:1 renames of a genuine scalar (1:1 means data
// fidelity, not byte-identical keys). It is intentionally conservative — only
// true scalar renames belong here, never flattened-duplication tags (those are
// genuine findings, e.g. branches `commit_id` flattening `commit.id`).
//
// Two forms are supported:
//   - "MCPType.tag": the rename is suppressed only for that MCP output type.
//   - "tag":         the rename is suppressed for any MCP output type (global).
//
// Add accepted renames here (with a one-line rationale) as the per-package
// audit adjudicates them.
var acceptedOutputRenames = map[string]bool{
	// branches Output renames the SDK `name` scalar to `branch_name` to match the
	// `branch_name` input parameter naming used across the branches tools.
	"Output.branch_name": true,
}

// isAcceptedRename reports whether (mcpType, tag) is an allowlisted deliberate
// rename, checking the type-scoped key first then the global tag key.
func isAcceptedRename(mcpType, tag string) bool {
	if acceptedOutputRenames[mcpType+"."+tag] {
		return true
	}
	return acceptedOutputRenames[tag]
}

// gap is one diffed MCP↔SDK struct pair under a package.
type gap struct {
	Kind           string         `json:"kind"` // "input" or "output"
	MCPType        string         `json:"mcp_type"`
	SDKType        string         `json:"sdk_type"`
	MissingFields  []missingField `json:"missing_fields,omitempty"`
	TypeMismatches []typeMismatch `json:"type_mismatches,omitempty"`
	// ExtraFields lists MCP output json tags with no SDK counterpart — invented
	// output scalars the 1:1 rule forbids (R-OUTPUT-EXTRA). Populated only for
	// output pairs; input pairs legitimately carry non-Options params (path ids).
	ExtraFields []extraField `json:"extra_fields,omitempty"`
}

type missingField struct {
	Tag     string `json:"tag"`
	SDKType string `json:"sdk_type"`
}

type typeMismatch struct {
	Tag     string `json:"tag"`
	MCPType string `json:"mcp_type"`
	SDKType string `json:"sdk_type"`
}

// extraField is one MCP output json tag with no SDK result counterpart.
type extraField struct {
	Tag     string `json:"tag"`
	MCPType string `json:"mcp_type"`
}

// envelopeKeys is the MCP-envelope carve-out per the 1:1 policy: resource output
// must mirror the SDK result, but MCP-protocol / structural keys (pagination
// envelope and LLM next-step hints) are legitimately additive and are therefore
// exempt from R-OUTPUT-EXTRA. A json tag of "-" is also exempt (not serialized).
var envelopeKeys = map[string]struct{}{
	"pagination":  {},
	"page":        {},
	"per_page":    {},
	"total":       {},
	"total_pages": {},
	"next_page":   {},
	"prev_page":   {},
	"next_steps":  {},
}

// packageReport aggregates the gap pairs for one internal/tools package.
type packageReport struct {
	Package            string `json:"package"`
	InputPairs         int    `json:"input_pairs"`
	OutputPairs        int    `json:"output_pairs"`
	MissingInputCount  int    `json:"missing_input_count"`
	MissingOutputCount int    `json:"missing_output_count"`
	ExtraOutputCount   int    `json:"extra_output_count"`
	Gaps               []gap  `json:"gaps"`
}

// report is the JSON document written to the output path.
type report struct {
	SchemaVersion int             `json:"schema_version"`
	ClientGoPath  string          `json:"client_go_path"`
	Summary       reportSummary   `json:"summary"`
	Packages      []packageReport `json:"packages"`
}

type reportSummary struct {
	Packages            int `json:"packages"`
	PackagesWithGaps    int `json:"packages_with_gaps"`
	InputPairs          int `json:"input_pairs"`
	OutputPairs         int `json:"output_pairs"`
	MissingInputFields  int `json:"missing_input_fields"`
	MissingOutputFields int `json:"missing_output_fields"`
	ExtraOutputFields   int `json:"extra_output_fields"`
	TypeMismatches      int `json:"type_mismatches"`
}

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include packages that have at least one missing field")
	flag.Parse()

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}
	rep, err := buildReport(root, *gapsOnly)
	if err != nil {
		cmdutil.Fatalf("build struct completeness report: %v", err)
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
	reports := make([]packageReport, 0, len(pkgs))
	for _, pkg := range pkgs {
		pr, ok := analyzePackage(pkg)
		if !ok {
			continue
		}
		if gapsOnly && pr.MissingInputCount == 0 && pr.MissingOutputCount == 0 && pr.ExtraOutputCount == 0 {
			continue
		}
		reports = append(reports, pr)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Package < reports[j].Package })
	return report{
		SchemaVersion: schemaVersion,
		ClientGoPath:  clientGoPkgPath,
		Summary:       summarize(reports),
		Packages:      reports,
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
		if !strings.Contains(pkg.PkgPath, toolsPkgInfix) {
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			continue
		}
		out = append(out, pkg)
	}
	if len(fatal) > 0 {
		return nil, fmt.Errorf("package load errors:\n%s", strings.Join(fatal, "\n"))
	}
	return out, nil
}

func analyzePackage(pkg *packages.Package) (packageReport, bool) {
	inputPairs := map[[2]string]structPair{}
	outputPairs := map[[2]string]structPair{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			collectConverter(pkg, fn, outputPairs)
			collectHandlerInputs(pkg, fn, inputPairs)
		}
	}
	if len(inputPairs) == 0 && len(outputPairs) == 0 {
		return packageReport{}, false
	}
	pr := packageReport{Package: shortPackage(pkg.PkgPath)}
	for _, pair := range sortedPairs(inputPairs) {
		pr.InputPairs++
		g := diffPair("input", pair)
		pr.MissingInputCount += len(g.MissingFields)
		appendGapIfAny(&pr, g)
	}
	for _, pair := range sortedPairs(outputPairs) {
		pr.OutputPairs++
		g := diffPair("output", pair)
		pr.MissingOutputCount += len(g.MissingFields)
		pr.ExtraOutputCount += len(g.ExtraFields)
		appendGapIfAny(&pr, g)
	}
	return pr, true
}

func appendGapIfAny(pr *packageReport, g gap) {
	if len(g.MissingFields) > 0 || len(g.TypeMismatches) > 0 || len(g.ExtraFields) > 0 {
		pr.Gaps = append(pr.Gaps, g)
	}
}

// structPair links an MCP struct with the SDK struct it must mirror.
type structPair struct {
	mcpName string
	mcpType *types.Struct
	sdkName string
	sdkType *types.Struct
	// tag preference for the SDK side: url first for Options, json for results.
	sdkURLTags bool
}

func sortedPairs(pairs map[[2]string]structPair) []structPair {
	out := make([]structPair, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, pair)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].mcpName != out[j].mcpName {
			return out[i].mcpName < out[j].mcpName
		}
		return out[i].sdkName < out[j].sdkName
	})
	return out
}

// collectConverter detects func(...src *gl.Y...) LocalStruct converters and
// records the (MCP output, SDK result) pair.
func collectConverter(pkg *packages.Package, fn *ast.FuncDecl, out map[[2]string]structPair) {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return
	}
	resultType := pkg.TypesInfo.TypeOf(fn.Type.Results.List[0].Type)
	mcpNamed, mcpStruct, ok := localNamedStruct(pkg, resultType)
	if !ok {
		return
	}
	var sdkNamed *types.Named
	var sdkStruct *types.Struct
	sdkCount := 0
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			named, st, isSDK := clientGoNamedStruct(pkg.TypesInfo.TypeOf(field.Type))
			if !isSDK {
				continue
			}
			// Exclude non-result SDK arguments so the output pair is not formed
			// against the wrong struct (R-OUTPUT-EXTRA false positives):
			//   - the pagination Response wrapper of list converters (the data
			//     slice would look "extra");
			//   - *Options request structs and time value types (ISOTime/time.Time)
			//     a converter may also accept alongside no genuine result struct.
			if nonResultSDKStruct(named) {
				continue
			}
			sdkNamed, sdkStruct = named, st
			sdkCount++
		}
	}
	// When no genuine result struct remains after exclusion (e.g. a converter that
	// only took a Response wrapper or an *Options/time arg), skip the pair rather
	// than mis-pairing the MCP output against a non-result SDK struct.
	if sdkCount != 1 {
		return
	}
	key := [2]string{mcpNamed.Obj().Name(), sdkNamed.Obj().Name()}
	out[key] = structPair{
		mcpName: mcpNamed.Obj().Name(), mcpType: mcpStruct,
		sdkName: sdkTypeName(sdkNamed), sdkType: sdkStruct, sdkURLTags: false,
	}
}

// collectHandlerInputs attributes every &gl.XxxOptions{} literal inside fn to
// fn's MCP input struct.
func collectHandlerInputs(pkg *packages.Package, fn *ast.FuncDecl, out map[[2]string]structPair) {
	if fn.Body == nil {
		return
	}
	mcpNamed, mcpStruct, ok := handlerInputStruct(pkg, fn)
	if !ok {
		return
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit {
			return true
		}
		named, st, isSDK := clientGoNamedStruct(pkg.TypesInfo.TypeOf(lit))
		if !isSDK || !strings.HasSuffix(named.Obj().Name(), optionsSuffix) {
			return true
		}
		key := [2]string{mcpNamed.Obj().Name(), named.Obj().Name()}
		out[key] = structPair{
			mcpName: mcpNamed.Obj().Name(), mcpType: mcpStruct,
			sdkName: sdkTypeName(named), sdkType: st, sdkURLTags: true,
		}
		return true
	})
}

// handlerInputStruct returns the first parameter whose type is a struct named
// in the handler's own package (the typed MCP input).
func handlerInputStruct(pkg *packages.Package, fn *ast.FuncDecl) (*types.Named, *types.Struct, bool) {
	if fn.Type.Params == nil {
		return nil, nil, false
	}
	for _, field := range fn.Type.Params.List {
		named, st, ok := localNamedStruct(pkg, pkg.TypesInfo.TypeOf(field.Type))
		if ok {
			return named, st, true
		}
	}
	return nil, nil, false
}

func diffPair(kind string, pair structPair) gap {
	mcpFields := flattenFields(pair.mcpType, []string{"json"})
	sdkTagKeys := []string{"json"}
	if pair.sdkURLTags {
		sdkTagKeys = []string{"url", "json"}
	}
	sdkFields := flattenFields(pair.sdkType, sdkTagKeys)

	g := gap{Kind: kind, MCPType: pair.mcpName, SDKType: pair.sdkName}
	tags := make([]string, 0, len(sdkFields))
	for tag := range sdkFields {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		sdkType := sdkFields[tag]
		mcpType, present := mcpFields[tag]
		if !present {
			// SDK url tags use array/negation notation (iids[], not[author_id])
			// that maps to snake_case MCP json names (iids, not_author_id).
			if alt, ok := mcpFields[normalizeSDKTag(tag)]; ok {
				mcpType, present = alt, true
			}
		}
		if !present {
			g.MissingFields = append(g.MissingFields, missingField{Tag: tag, SDKType: sdkType})
			continue
		}
		if !typesCompatible(mcpType, sdkType) {
			g.TypeMismatches = append(g.TypeMismatches, typeMismatch{Tag: tag, MCPType: mcpType, SDKType: sdkType})
		}
	}
	if kind == "output" {
		g.ExtraFields = extraOutputFields(pair.mcpName, mcpFields, sdkFields)
	}
	return g
}

// extraOutputFields reports MCP output json tags with no SDK result counterpart:
// invented output scalars the 1:1 rule forbids (R-OUTPUT-EXTRA). An MCP tag is
// extra when it is neither a key of sdkFields nor the normalizeSDKTag image of
// any SDK key, is not the MCP-envelope carve-out, is not the "-" sentinel, and
// is not an allowlisted deliberate rename (per the 1:1 data-fidelity policy).
func extraOutputFields(mcpType string, mcpFields, sdkFields map[string]string) []extraField {
	sdkNorm := make(map[string]struct{}, len(sdkFields))
	for sdkTag := range sdkFields {
		sdkNorm[normalizeSDKTag(sdkTag)] = struct{}{}
	}
	tags := make([]string, 0, len(mcpFields))
	for tag := range mcpFields {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var extras []extraField
	for _, tag := range tags {
		if tag == "-" {
			continue
		}
		if _, exempt := envelopeKeys[tag]; exempt {
			continue
		}
		if _, ok := sdkFields[tag]; ok {
			continue
		}
		if _, ok := sdkNorm[tag]; ok {
			continue
		}
		if isAcceptedRename(mcpType, tag) {
			continue
		}
		extras = append(extras, extraField{Tag: tag, MCPType: mcpFields[tag]})
	}
	return extras
}

// flattenFields walks a struct (recursing into embedded structs) and returns a
// map of tag-name → type string for every field carrying one of the tag keys.
func flattenFields(st *types.Struct, tagKeys []string) map[string]string {
	out := map[string]string{}
	flattenInto(st, tagKeys, out, 0)
	return out
}

func flattenInto(st *types.Struct, tagKeys []string, out map[string]string, depth int) {
	if st == nil || depth > 6 {
		return
	}
	for i := range st.NumFields() {
		field := st.Field(i)
		raw := reflect.StructTag(st.Tag(i))
		tagName := tagValue(raw, tagKeys)
		if field.Embedded() && tagName == "" {
			if embedded, ok := structUnder(field.Type()); ok {
				flattenInto(embedded, tagKeys, out, depth+1)
				continue
			}
		}
		if tagName == "" || tagName == "-" {
			continue
		}
		if _, exists := out[tagName]; !exists {
			out[tagName] = types.TypeString(field.Type(), shortQualifier)
		}
	}
}

// normalizeSDKTag maps a client-go url-tag name to the snake_case json name the
// MCP inputs use: trailing array notation ("iids[]" → "iids") and bracket
// negation notation ("not[author_id]" → "not_author_id").
func normalizeSDKTag(tag string) string {
	tag = strings.TrimSuffix(tag, "[]")
	tag = strings.ReplaceAll(tag, "[", "_")
	tag = strings.ReplaceAll(tag, "]", "")
	return tag
}

func tagValue(raw reflect.StructTag, keys []string) string {
	for _, key := range keys {
		if value, ok := raw.Lookup(key); ok {
			name, _, _ := strings.Cut(value, ",")
			if name != "" {
				return name
			}
		}
	}
	return ""
}

// typesCompatible reports whether an MCP field type acceptably represents an
// SDK field type. Pointer-ness is ignored (optional inputs stay pointers in the
// SDK); known scalar projections (enum Value types → int/string, time types →
// string) are treated as compatible because the domain maps them deliberately.
func typesCompatible(mcpType, sdkType string) bool {
	mcp := normalizeType(mcpType)
	sdk := normalizeType(sdkType)
	if mcp == sdk {
		return true
	}
	if scalarLike(mcp) && scalarLike(sdk) {
		return true
	}
	// SDK enum/value types projected to scalars.
	if strings.HasSuffix(sdk, "value") && (mcp == "int" || mcp == "string" || mcp == "int64") {
		return true
	}
	if sdkTimeLike(sdk) && (mcp == "string" || mcp == "int64") {
		return true
	}
	// SDK label collections (LabelOptions/Labels, both defined as []string) are
	// represented as []string on the MCP side.
	if mcp == "[]string" && (strings.HasSuffix(sdk, "labeloptions") || strings.HasSuffix(sdk, "labels")) {
		return true
	}
	return false
}

// sdkTimeLike reports whether a normalized SDK type is one of the client-go
// time representations (ISOTime, time.Time) that the domain renders as strings.
func sdkTimeLike(sdk string) bool {
	return sdk == "time" || sdk == "time.time" || strings.HasSuffix(sdk, "isotime")
}

func normalizeType(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "*")
	return s
}

func scalarLike(s string) bool {
	switch s {
	case "int", "int64", "int32", "float64", "float32", "bool", "string":
		return true
	default:
		return false
	}
}

// localNamedStruct returns the named struct if t (deref'd) is a struct declared
// in pkg itself.
func localNamedStruct(pkg *packages.Package, t types.Type) (*types.Named, *types.Struct, bool) {
	named, st, ok := derefNamedStruct(t)
	if !ok {
		return nil, nil, false
	}
	if named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != pkg.PkgPath {
		return nil, nil, false
	}
	return named, st, true
}

// clientGoNamedStruct returns the named struct if t (deref'd) is a struct in the
// client-go SDK module.
func clientGoNamedStruct(t types.Type) (*types.Named, *types.Struct, bool) {
	named, st, ok := derefNamedStruct(t)
	if !ok {
		return nil, nil, false
	}
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), clientGoPkgPath) {
		return nil, nil, false
	}
	return named, st, true
}

// nonResultSDKStruct reports whether a client-go named struct is NOT a result
// type and must therefore be excluded when forming an OUTPUT pair. This covers
// three false-positive classes the converter param scan would otherwise pair
// against:
//
//   - the pagination Response wrapper (named "Response"): list converters take
//     it so the MCP list-output's element slice looks "extra";
//   - *Options request structs (name ends in "Options"): request inputs, not
//     results, sometimes accepted by GraphQL converters;
//   - time value types (ISOTime, time.Time, anything in the `time` package):
//     scalar value args, not result structs.
func nonResultSDKStruct(named *types.Named) bool {
	obj := named.Obj()
	if nonResultStructName(obj.Name()) {
		return true
	}
	if pkg := obj.Pkg(); pkg != nil && pkg.Path() == "time" {
		return true
	}
	return false
}

// nonResultStructName reports whether a bare struct name identifies a non-result
// SDK type (pagination wrapper, *Options request struct, or a time value type).
// Split out from nonResultSDKStruct so the name-based rule is unit-testable
// without synthesizing go/types objects.
func nonResultStructName(name string) bool {
	switch {
	case name == responseStructName:
		return true
	case strings.HasSuffix(name, optionsSuffix):
		return true
	case name == "ISOTime" || name == "Time":
		return true
	default:
		return false
	}
}

func derefNamedStruct(t types.Type) (*types.Named, *types.Struct, bool) {
	if t == nil {
		return nil, nil, false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, nil, false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil, false
	}
	return named, st, true
}

func structUnder(t types.Type) (*types.Struct, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		t = named.Underlying()
	}
	st, ok := t.(*types.Struct)
	return st, ok
}

func sdkTypeName(named *types.Named) string {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return named.Obj().Name()
	}
	return lastPathSegment(pkg.Path()) + "." + named.Obj().Name()
}

func shortQualifier(pkg *types.Package) string {
	return lastPathSegment(pkg.Path())
}

func lastPathSegment(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func shortPackage(pkgPath string) string {
	_, after, ok := strings.Cut(pkgPath, toolsPkgInfix)
	if !ok {
		return lastPathSegment(pkgPath)
	}
	return after
}

func summarize(reports []packageReport) reportSummary {
	s := reportSummary{Packages: len(reports)}
	for _, pr := range reports {
		if pr.MissingInputCount > 0 || pr.MissingOutputCount > 0 || pr.ExtraOutputCount > 0 {
			s.PackagesWithGaps++
		}
		s.InputPairs += pr.InputPairs
		s.OutputPairs += pr.OutputPairs
		s.MissingInputFields += pr.MissingInputCount
		s.MissingOutputFields += pr.MissingOutputCount
		s.ExtraOutputFields += pr.ExtraOutputCount
		for _, g := range pr.Gaps {
			s.TypeMismatches += len(g.TypeMismatches)
		}
	}
	return s
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
