// Package enums holds the 1:1 audit rule for enumerated values (R-ENUM).
//
// The struct rule (R-INPUT/R-OUTPUT) asks whether every SDK field has an MCP
// counterpart, and it answers "yes" for a field of an enum type the moment a
// same-named scalar exists on our side: a `type XxxValue string` is projected
// to a string in the field comparison, so the VALUES the SDK declares for it
// are never read. A constant added upstream is therefore invisible while the
// field stays covered. That is exactly what a hand-written pin test in the
// domain package used to guard, one enum at a time, by naming each constant,
// which is a list that has to be extended by hand to keep guarding anything.
//
// This rule reads the values instead. For every client-go type of the form
// `type T string` (or an integer kind) with a const block of T values, it finds
// the fields of that type an action exposes, through the same (MCP struct,
// SDK struct) pairs the struct rule diffs, and compares the SDK's constant set
// with the values our surface offers: the input schema's `enum`, or the
// property description where that is how a value set is surfaced. Values the
// SDK declares that we do not offer are reported, and so are values we offer
// that the SDK does not declare.
//
// Like the sdk scope this rule is a gate, with a table of exemptions each
// carrying a reason, and an exemption that no longer matches a field or a gap
// is itself a finding.
package enums

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// Where a field's offered values come from.
const (
	// sourceEnum: the schema property carries an `enum` list, which is the
	// form a model can be held to.
	sourceEnum = "enum"
	// sourceDescription: the property has no enum and its description names
	// the values. Only the SDK values it mentions can be read off prose, so a
	// description-sourced field can be missing values but never offer extra
	// ones.
	sourceDescription = "description"
	// sourceNone: nothing on the surface says what the values are, so every
	// SDK value is missing.
	sourceNone = "none"

	kindInput  = "input"
	kindOutput = "output"
	tagJSON    = "json"
	tagURL     = "url"
	tagExclude = "-"
)

// Finding is one field of an SDK enum type that an action exposes, held
// against the SDK's constant set.
type Finding struct {
	// Action is the canonical action ID (domain.action).
	Action string `json:"action"`
	// Kind is input or output.
	Kind string `json:"kind"`
	// MCPType and Field name the MCP struct and its json tag.
	MCPType string `json:"mcp_type"`
	Field   string `json:"field"`
	// SDKType and SDKField name the client-go struct and field the MCP field
	// mirrors.
	SDKType  string `json:"sdk_type"`
	SDKField string `json:"sdk_field"`
	// Enum is the client-go value type whose constants are the reference set.
	Enum string `json:"sdk_enum"`
	// Source says where the offered values were read from.
	Source string `json:"source"`
	// SDKValues and Offered are the two sets compared, sorted.
	SDKValues []string `json:"sdk_values"`
	Offered   []string `json:"offered_values"`
	// Missing lists SDK values we do not offer; Extra lists offered values
	// the SDK does not declare. An exempted value is dropped from both.
	Missing []string `json:"missing_values,omitempty"`
	Extra   []string `json:"extra_values,omitempty"`
	// Exemption is the recorded reason when acceptedEnumGaps covers the field
	// or one of its values.
	Exemption string `json:"exemption,omitempty"`
}

// hasGap reports whether the finding still carries a value after exemptions.
func (f Finding) hasGap() bool { return len(f.Missing) > 0 || len(f.Extra) > 0 }

// packageReport groups the findings of one internal/tools package.
type packageReport struct {
	Package       string    `json:"package"`
	Fields        int       `json:"fields"`
	MissingValues int       `json:"missing_values"`
	ExtraValues   int       `json:"extra_values"`
	Findings      []Finding `json:"findings"`
}

// Report is the JSON document of the enums scope. It is exported for the sdk
// gate, which folds the same findings into its own summary.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	ClientGoPath  string          `json:"client_go_path"`
	Summary       Summary         `json:"summary"`
	Packages      []packageReport `json:"packages"`
	// StaleExemptions lists acceptedEnumGaps entries that no longer name an
	// exposed field or a gap, so a reason cannot outlive what it excused.
	StaleExemptions []string `json:"stale_exemptions,omitempty"`
}

// Summary is the enums scope's counters.
type Summary struct {
	Packages int `json:"packages"`
	// SDKEnums is how many enum value types client-go declares.
	SDKEnums int `json:"sdk_enums"`
	// Fields is how many (action, field) pairs of an enum type are exposed.
	Fields         int `json:"fields"`
	FieldsWithGaps int `json:"fields_with_gaps"`
	// UnsurfacedOutputFields counts the output fields of an enum type whose
	// schema names no value set, so nothing could be compared. They are the
	// boundary of what this rule sees, not findings.
	UnsurfacedOutputFields int `json:"unsurfaced_output_fields"`
	MissingValues          int `json:"missing_values"`
	ExtraValues            int `json:"extra_values"`
	StaleExemptions        int `json:"stale_exemptions"`
}

// Clean reports whether the gate passes: no value gap and no stale exemption.
func (s Summary) Clean() bool {
	return s.MissingValues == 0 && s.ExtraValues == 0 && s.StaleExemptions == 0
}

// marshalIndent is the JSON encoder, a variable so a test can reach the
// encoding failure branch that a struct of strings and ints never produces.
var marshalIndent = json.MarshalIndent

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline) together with the gate outcome.
// gapsOnly keeps only the fields with a finding, matching the other scopes.
func Run(root string, gapsOnly bool) (content []byte, clean bool, err error) {
	rep, err := Analyze(root, gapsOnly)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(rep, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), rep.Summary.Clean(), nil
}

// Analyze builds the report against the tool packages under root and the
// action catalog compiled into this binary.
func Analyze(root string, gapsOnly bool) (Report, error) {
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		return Report{}, err
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		return Report{}, err
	}
	sdkEnums := collectSDKEnums(clientGo)

	client, cleanup := auditclient.NewMock()
	defer cleanup()
	offered := collectOffered(auditshared.CachedActionSpecs(client, true))

	return buildReport(pkgs, sdkEnums, offered, acceptedEnumGaps, gapsOnly), nil
}

// buildReport is the pure rule over its three inputs: the exposed fields
// found in pkgs, the SDK's constant sets, and the values the catalog offers.
// The exemption table is a parameter so a test can state the whole universe
// it asserts about.
func buildReport(pkgs []*packages.Package, sdkEnums map[string]sdkEnum, offered offeredIndex, exemptions map[string]string, gapsOnly bool) Report {
	matched := map[string]bool{}
	byPackage := map[string]*packageReport{}
	unsurfaced := 0
	for _, field := range collectExposedFields(pkgs, sdkEnums) {
		for _, offer := range offered[field.structKey()] {
			if offer.Kind != field.Kind {
				// One struct can serve as an action's input and another's
				// output; a field is held to the schema of its own side.
				continue
			}
			finding := compare(field, sdkEnums[field.Enum], offer)
			applyExemptions(&finding, field.Package, exemptions, matched)
			pr := packageFor(byPackage, field.Package)
			pr.Fields++
			pr.MissingValues += len(finding.Missing)
			pr.ExtraValues += len(finding.Extra)
			if finding.Kind == kindOutput && finding.Source == sourceNone {
				unsurfaced++
			}
			if gapsOnly && !finding.hasGap() {
				continue
			}
			pr.Findings = append(pr.Findings, finding)
		}
	}
	stale := staleExemptions(exemptions, matched)

	packagesOut := make([]packageReport, 0, len(byPackage))
	for _, pr := range byPackage {
		sort.Slice(pr.Findings, func(i, j int) bool { return findingLess(pr.Findings[i], pr.Findings[j]) })
		packagesOut = append(packagesOut, *pr)
	}
	sort.Slice(packagesOut, func(i, j int) bool { return packagesOut[i].Package < packagesOut[j].Package })

	// The summary describes the whole tree whichever packages the report
	// lists, the way the other scopes' summaries do.
	summary := summarize(packagesOut, len(sdkEnums), len(stale))
	summary.UnsurfacedOutputFields = unsurfaced
	if gapsOnly {
		packagesOut = keepPackagesWithFindings(packagesOut)
	}
	return Report{
		SchemaVersion:   shared.SchemaVersion,
		ClientGoPath:    shared.ClientGoPkgPath,
		Summary:         summary,
		Packages:        packagesOut,
		StaleExemptions: stale,
	}
}

// keepPackagesWithFindings drops the packages that list nothing, for the
// gaps-only report.
func keepPackagesWithFindings(packagesOut []packageReport) []packageReport {
	kept := make([]packageReport, 0, len(packagesOut))
	for _, pr := range packagesOut {
		if len(pr.Findings) > 0 {
			kept = append(kept, pr)
		}
	}
	return kept
}

// findingLess orders findings by action, kind and field, so the report is
// stable across runs.
func findingLess(a, b Finding) bool {
	if a.Action != b.Action {
		return a.Action < b.Action
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Field < b.Field
}

func packageFor(byPackage map[string]*packageReport, name string) *packageReport {
	pr, ok := byPackage[name]
	if !ok {
		pr = &packageReport{Package: name}
		byPackage[name] = pr
	}
	return pr
}

func summarize(packagesOut []packageReport, sdkEnums, stale int) Summary {
	s := Summary{Packages: len(packagesOut), SDKEnums: sdkEnums, StaleExemptions: stale}
	for _, pr := range packagesOut {
		s.Fields += pr.Fields
		s.MissingValues += pr.MissingValues
		s.ExtraValues += pr.ExtraValues
		for _, finding := range pr.Findings {
			if finding.hasGap() {
				s.FieldsWithGaps++
			}
		}
	}
	return s
}

// --- The SDK side: enum value types and their constants ---

// sdkEnum is one client-go value type with the constants declared for it.
type sdkEnum struct {
	Name string
	// Values are the constants' values rendered as text, sorted and
	// deduplicated: two constants with one value (a renamed constant kept as
	// an alias) are one value to a caller.
	Values []string
}

// collectSDKEnums returns every client-go type of the form `type T string`
// or `type T <integer>` that has at least one constant of type T, keyed by
// type name. A named basic type without constants is a plain alias (an ID
// type, say), and there is nothing to compare it against.
func collectSDKEnums(clientGo *types.Package) map[string]sdkEnum {
	scope := clientGo.Scope()
	values := map[string]map[string]struct{}{}
	for _, name := range scope.Names() {
		obj, ok := scope.Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		named, ok := obj.Type().(*types.Named)
		if !ok || !isEnumUnderlying(named) || named.Obj().Pkg() != clientGo {
			continue
		}
		typeName := named.Obj().Name()
		if values[typeName] == nil {
			values[typeName] = map[string]struct{}{}
		}
		values[typeName][renderConstant(obj.Val())] = struct{}{}
	}
	out := make(map[string]sdkEnum, len(values))
	for name, set := range values {
		out[name] = sdkEnum{Name: name, Values: shared.SortedSet(set)}
	}
	return out
}

// isEnumUnderlying reports whether a named type's underlying type is a
// string or an integer kind, the two shapes client-go declares value sets in.
func isEnumUnderlying(named *types.Named) bool {
	basic, ok := named.Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsString|types.IsInteger) != 0
}

// renderConstant renders a constant value the way it appears on the wire and
// in a schema enum: the bare string, or the decimal integer.
func renderConstant(value constant.Value) string {
	if value.Kind() == constant.String {
		return constant.StringVal(value)
	}
	return value.ExactString()
}

// enumTypeOf returns the name of the client-go enum type a field carries,
// through pointers and slices (*T, []T, *[]T, []*T), or "" when the field is
// not of an enum type.
func enumTypeOf(t types.Type, sdkEnums map[string]sdkEnum) string {
	for {
		switch typed := t.(type) {
		case *types.Pointer:
			t = typed.Elem()
			continue
		case *types.Slice:
			t = typed.Elem()
			continue
		case *types.Named:
			if !shared.IsClientGoObject(typed.Obj()) {
				return ""
			}
			if _, ok := sdkEnums[typed.Obj().Name()]; ok {
				return typed.Obj().Name()
			}
			return ""
		default:
			return ""
		}
	}
}

// --- Our side: the fields of an enum type an action exposes ---

// exposedField is one MCP struct field that mirrors an SDK field of an enum
// type, found through the struct rule's pairs.
type exposedField struct {
	// PkgPath and Package identify the tool package, in full and short form.
	PkgPath string
	Package string
	Kind    string
	MCPType string
	Tag     string
	// SDKType and SDKField name the client-go struct and field paired to it.
	SDKType  string
	SDKField string
	// Enum is the client-go value type of the SDK field.
	Enum string
}

// structKey identifies the MCP struct the way the catalog's reflect types
// spell it, so the two halves of the rule meet on the same name.
func (f exposedField) structKey() string { return f.PkgPath + "." + f.MCPType }

// fieldIdentity is what makes two exposed fields the same field: an output
// type built by several converters meets the same MCP tag once per SDK
// struct, and the rule holds the tag once, against the first pairing.
type fieldIdentity struct {
	pkgPath, kind, mcpType, tag, enum string
}

func (f exposedField) identity() fieldIdentity {
	return fieldIdentity{pkgPath: f.PkgPath, kind: f.Kind, mcpType: f.MCPType, tag: f.Tag, enum: f.Enum}
}

// collectExposedFields walks every (MCP struct, SDK struct) pair of every tool
// package and keeps the SDK fields of an enum type that have an MCP
// counterpart under the same tag (or its normalized form). An SDK enum field
// with no counterpart is not exposed at all, which is the struct rule's
// finding, not this one's.
func collectExposedFields(pkgs []*packages.Package, sdkEnums map[string]sdkEnum) []exposedField {
	var out []exposedField
	seen := map[fieldIdentity]struct{}{}
	for _, pkg := range pkgs {
		pkgName := shared.ShortPackage(pkg.PkgPath)
		for _, pair := range structs.CollectPairs(pkg) {
			mcpTags := tagsOf(pair.MCPType, []string{tagJSON})
			sdkKeys := []string{tagJSON}
			if pair.SDKURLTags {
				sdkKeys = []string{tagURL, tagJSON}
			}
			for _, sdkField := range taggedFields(pair.SDKType, sdkKeys) {
				enum := enumTypeOf(sdkField.typ, sdkEnums)
				if enum == "" {
					continue
				}
				mcpTag, ok := matchTag(sdkField.tag, mcpTags)
				if !ok {
					continue
				}
				field := exposedField{
					PkgPath: pkg.PkgPath, Package: pkgName, Kind: pair.Kind,
					MCPType: pair.MCPName, Tag: mcpTag,
					SDKType: pair.SDKName, SDKField: sdkField.name, Enum: enum,
				}
				// An output type built by several converters (union pairing)
				// meets the same MCP tag once per converter.
				if _, dup := seen[field.identity()]; dup {
					continue
				}
				seen[field.identity()] = struct{}{}
				out = append(out, field)
			}
		}
	}
	return out
}

// matchTag finds the MCP json tag an SDK tag maps to: the tag itself, or its
// snake_case normalization (iids[] → iids, not[author_id] → not_author_id).
func matchTag(sdkTag string, mcpTags map[string]struct{}) (string, bool) {
	if _, ok := mcpTags[sdkTag]; ok {
		return sdkTag, true
	}
	normalized := shared.NormalizeSDKTag(sdkTag)
	if _, ok := mcpTags[normalized]; ok {
		return normalized, true
	}
	return "", false
}

// taggedField is one struct field that carries a serialization tag.
type taggedField struct {
	name string
	tag  string
	typ  types.Type
}

// taggedFields lists the tagged fields of st, recursing into embedded structs
// the way the struct rule flattens them, in declaration order.
func taggedFields(st *types.Struct, tagKeys []string) []taggedField {
	var out []taggedField
	collectTaggedFields(st, tagKeys, &out, 0)
	return out
}

// maxEmbedDepth bounds the embedded-struct recursion, matching the struct
// rule's own limit.
const maxEmbedDepth = 6

func collectTaggedFields(st *types.Struct, tagKeys []string, out *[]taggedField, depth int) {
	if st == nil || depth > maxEmbedDepth {
		return
	}
	for i := range st.NumFields() {
		field := st.Field(i)
		tag := shared.TagName(reflect.StructTag(st.Tag(i)), tagKeys)
		if field.Embedded() && tag == "" {
			if embedded, ok := structUnder(field.Type()); ok {
				collectTaggedFields(embedded, tagKeys, out, depth+1)
				continue
			}
		}
		if tag == "" || tag == tagExclude {
			continue
		}
		*out = append(*out, taggedField{name: field.Name(), tag: tag, typ: field.Type()})
	}
}

// tagsOf returns the set of tags st carries under tagKeys, embedded structs
// flattened.
func tagsOf(st *types.Struct, tagKeys []string) map[string]struct{} {
	fields := taggedFields(st, tagKeys)
	out := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		out[field.tag] = struct{}{}
	}
	return out
}

// structUnder returns the struct a field type reaches through a pointer and a
// name, for embedded-struct recursion.
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

// --- The catalog side: the values each action offers for a field ---

// property is one schema property's value-bearing parts.
type property struct {
	// Enum is the property's enum list rendered as text; nil when it has none.
	Enum []string
	// Description is the property's description, read only when Enum is nil.
	Description string
}

// actionOffer is one action's schema properties for one MCP struct.
type actionOffer struct {
	Action     string
	Kind       string
	Properties map[string]property
}

// offeredIndex maps an MCP struct (pkgPath.Name) to the actions that route
// it and what each one's schema says about its properties. One struct can be
// routed by several actions, and each action may override a property
// differently, so the unit of a finding is the (action, field) pair.
type offeredIndex map[string][]actionOffer

// collectOffered reads every action's input and output schema off the
// catalog, keyed by the reflect identity of the struct the schema was built
// from. That identity (import path plus type name) is what go/types reports
// for the same struct, which is how a finding from the AST half lands on the
// schema from the catalog half.
func collectOffered(groups []tools.ActionSpecGroup) offeredIndex {
	index := offeredIndex{}
	for _, group := range groups {
		domain := actioncatalog.DomainFromToolName(group.ToolName)
		for _, spec := range group.Actions {
			action := domain + "." + spec.Name
			index.add(spec.Route.InputType, actionOffer{Action: action, Kind: kindInput, Properties: schemaProperties(spec.Route.InputSchema)})
			index.add(spec.Route.OutputType, actionOffer{Action: action, Kind: kindOutput, Properties: schemaProperties(spec.Route.OutputSchema)})
		}
	}
	return index
}

// add records offer under the reflect identity of rt, through one pointer.
// A nil or unnamed type (a route built without a typed input) is skipped.
func (index offeredIndex) add(rt reflect.Type, offer actionOffer) {
	if rt == nil {
		return
	}
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Name() == "" {
		return
	}
	key := rt.PkgPath() + "." + rt.Name()
	index[key] = append(index[key], offer)
}

// schemaProperties reads the top-level properties of a JSON schema into the
// value-bearing form the comparison needs. An array property offers the enum
// of its items, since that is where a value set on a list parameter lives.
func schemaProperties(schema map[string]any) map[string]property {
	props, _ := schema["properties"].(map[string]any)
	out := make(map[string]property, len(props))
	for name, raw := range props {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		p := property{Enum: enumValues(prop)}
		if p.Enum == nil {
			if items, isMap := prop["items"].(map[string]any); isMap {
				p.Enum = enumValues(items)
			}
		}
		if p.Enum == nil {
			p.Description, _ = prop["description"].(string)
		}
		out[name] = p
	}
	return out
}

// enumValues renders a schema node's enum list as sorted text, or nil when
// the node has no enum. Integers are rendered bare (30, not 30.0) whether the
// override wrote a Go int or a JSON round-trip made it a float.
func enumValues(node map[string]any) []string {
	raw, ok := node["enum"].([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, v := range raw {
		values = append(values, fmt.Sprint(v))
	}
	sort.Strings(values)
	return values
}

// --- The comparison ---

// compare holds one exposed field against the SDK's constant set for one
// action's offer of it.
func compare(field exposedField, sdk sdkEnum, offer actionOffer) Finding {
	finding := Finding{
		Action: offer.Action, Kind: field.Kind,
		MCPType: field.MCPType, Field: field.Tag,
		SDKType: field.SDKType, SDKField: field.SDKField,
		Enum: sdk.Name, SDKValues: sdk.Values,
		Source: sourceNone, Offered: []string{},
	}
	prop, present := offer.Properties[field.Tag]
	switch {
	case !present:
		// The struct field exists but the schema never surfaced it (a
		// hidden or pruned property): nothing is offered.
	case prop.Enum != nil:
		finding.Source = sourceEnum
		finding.Offered = prop.Enum
	case len(mentionedValues(prop.Description, sdk.Values)) > 0:
		finding.Source = sourceDescription
		finding.Offered = mentionedValues(prop.Description, sdk.Values)
	}
	// An output field that surfaces no value set at all is not held to one:
	// the output relays whatever GitLab answers, and its schema is reflected
	// from the response struct, whose fields carry no descriptions. It is
	// counted as unsurfaced instead, so the boundary stays visible. An input
	// field in the same state is a gap on every value, since a model has to
	// choose one and nothing says what the choices are.
	if finding.Source == sourceNone && field.Kind == kindOutput {
		return finding
	}
	finding.Missing = difference(sdk.Values, finding.Offered)
	if finding.Source == sourceEnum {
		finding.Extra = difference(finding.Offered, sdk.Values)
	}
	return finding
}

// negatedSentence marks a sentence of a description that names values in
// order to exclude them ("60=Admin is not valid", "5 and 15 are not accepted
// for group shares"), so the values it names are not read as offered.
var negatedSentence = regexp.MustCompile(`(?i)\b(not valid|invalid|not accepted|not supported|not allowed|rejected|refused|cannot)\b`)

// mentionedValues returns the SDK values a description names, as whole
// tokens, case-insensitively: "Package ecosystem: cargo, composer, ..." names
// cargo and composer, and "set to 30 or 40" names 30 and 40. A sentence that
// exists to exclude values is skipped. Prose can only confirm a value, never
// declare a foreign one, so the result is a subset of values.
func mentionedValues(description string, values []string) []string {
	tokens := map[string]struct{}{}
	for _, sentence := range strings.FieldsFunc(description, isSentenceBoundary) {
		if negatedSentence.MatchString(sentence) {
			continue
		}
		for _, token := range strings.FieldsFunc(sentence, isTokenBoundary) {
			tokens[strings.ToLower(token)] = struct{}{}
		}
	}
	out := []string{}
	for _, value := range values {
		if _, ok := tokens[strings.ToLower(value)]; ok {
			out = append(out, value)
		}
	}
	return out
}

// isSentenceBoundary splits a description into the clauses a negation
// applies to: sentences and semicolon-separated remarks.
func isSentenceBoundary(r rune) bool {
	return r == '.' || r == ';'
}

// isTokenBoundary separates description tokens: anything but a letter, a
// digit or an underscore, so snake_case values stay whole and punctuation
// around a value falls away.
func isTokenBoundary(r rune) bool {
	return r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
}

// difference returns the values of a absent from b, sorted; nil when none.
func difference(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, v := range b {
		inB[v] = struct{}{}
	}
	var out []string
	for _, v := range a {
		if _, ok := inB[v]; !ok {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// --- Exemptions ---

// exemptionKey renders the field-level exemption key, "<pkg>.<MCPType>.<tag>".
func exemptionKey(pkg string, f Finding) string {
	return pkg + "." + f.MCPType + "." + f.Field
}

// applyExemptions drops the exempted values from a finding and records the
// reason, marking every exemption key it used so the unused ones can be
// reported stale. A field-level key excuses every gap on the field; a
// value-level key ("<field key>=<value>") excuses that one value, in either
// direction.
func applyExemptions(f *Finding, pkg string, exemptions map[string]string, matched map[string]bool) {
	key := exemptionKey(pkg, *f)
	if reason, ok := exemptions[key]; ok {
		if f.hasGap() {
			matched[key] = true
		}
		f.Exemption = reason
		f.Missing, f.Extra = nil, nil
		return
	}
	f.Missing = exemptValues(f, key, f.Missing, exemptions, matched)
	f.Extra = exemptValues(f, key, f.Extra, exemptions, matched)
}

// exemptValues returns values with the exempted ones removed.
func exemptValues(f *Finding, fieldKey string, values []string, exemptions map[string]string, matched map[string]bool) []string {
	var kept []string
	for _, value := range values {
		key := fieldKey + "=" + value
		reason, ok := exemptions[key]
		if !ok {
			kept = append(kept, value)
			continue
		}
		matched[key] = true
		f.Exemption = reason
	}
	return kept
}

// staleExemptions lists the exemption keys that excused nothing: a field that
// is no longer exposed, or a gap that has since been closed. Either way the
// reason describes code that has moved on.
func staleExemptions(exemptions map[string]string, matched map[string]bool) []string {
	var stale []string
	for key := range exemptions {
		if !matched[key] {
			stale = append(stale, "enum "+key)
		}
	}
	sort.Strings(stale)
	return stale
}
