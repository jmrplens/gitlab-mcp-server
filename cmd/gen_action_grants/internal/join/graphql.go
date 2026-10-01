package join

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// framing are the selections a connection carries around its items, which do
// not stop the answer spine: the items are what the spine follows.
var framing = map[string]bool{"pageInfo": true, "count": true, "__typename": true, "cursor": true}

// connectionItems are the fields of a connection or an edge that hold its
// items.
var connectionItems = map[string]bool{"nodes": true, "node": true}

// signature is the shape of one position: whether it is a list, whether its
// items and the field itself are non-null.
type signature struct {
	list         bool
	itemNonNull  bool
	fieldNonNull bool
}

// parseSignature reads a signature as the record writes it: "T", "T!",
// "[T]", "[T!]!".
func parseSignature(text string) signature {
	sig := signature{fieldNonNull: strings.HasSuffix(text, "!")}
	inner := strings.TrimSuffix(text, "!")
	if strings.HasPrefix(inner, "[") {
		sig.list = true
		sig.itemNonNull = strings.HasSuffix(strings.TrimSuffix(inner, "]"), "!")
	}
	return sig
}

// schemaSignature reads a signature from the pinned schema's type.
func schemaSignature(typ *gqlast.Type) signature {
	sig := signature{fieldNonNull: typ.NonNull}
	if typ.Elem != nil {
		sig.list = true
		sig.itemNonNull = typ.Elem.NonNull
	}
	return sig
}

// position is one object position a document selects.
type position struct {
	path   string
	parent *position
	// typeName is the named type GitLab answers with there.
	typeName string
	sig      signature
	// connectionItem marks the items of a connection, reached through nodes
	// or an edge's node.
	connectionItem bool
	// owner and field name the parent type and the field, for a field-level
	// declaration.
	owner, field string
	// children are the object positions it selects, in order, and scalars the
	// names of the other fields it selects.
	children []*position
	scalars  []string
}

// Element is one judged position of an operation, before it is indexed.
type Element struct {
	Path       string
	Type       string
	Members    []string
	Groups     []requirement
	Undeclared bool
	Effect     finegrained.Effect
	// Fatal is set when a denial here takes the answer with it.
	Fatal bool
	// Skip is set for a position every directive of which opts out.
	Skip bool
}

// Operation is one GraphQL operation, judged.
type Operation struct {
	// Name is the operation's kind and root fields with the document it comes
	// from, which is how a reader finds it.
	Name string
	// Mutation is set for a mutation.
	Mutation bool
	// RootFields are the operation's root fields.
	RootFields []string
	// Groups are a mutation's own requirement groups.
	Groups []requirement
	// Skip is set for a mutation that opts out of the check.
	Skip bool
	// Undeclared names a mutation that declares nothing, which refuses every
	// fine-grained token.
	Undeclared string
	// Elements are every judged position the operation selects.
	Elements []Element
	// Collection is set when the spine ends in a list or a connection.
	Collection bool
	// Paths are the path of every object position the document selects, for
	// the record of what this server sends.
	Paths []string
	// Fallbacks counts the positions whose signature the record does not
	// carry and the pinned schema answered.
	Fallbacks int
}

// analyzer judges documents against the record.
type analyzer struct {
	authz  *apilive.GraphQLAuthz
	schema *gqlast.Schema
}

// operations judges one document. A document the pinned schema refuses is an
// error: it is one the server sends and GitLab would refuse.
func (a *analyzer) operations(name, text string) ([]Operation, error) {
	doc, err := graphqlschema.ParseAgainst(a.schema, text)
	if err != nil {
		return nil, fmt.Errorf("document %s does not validate against the pinned schema: %w", name, err)
	}
	var out []Operation
	for _, op := range doc.Operations {
		out = append(out, a.operation(name, op))
	}
	return out, nil
}

// operation judges one operation.
func (a *analyzer) operation(name string, op *gqlast.OperationDefinition) Operation {
	out := Operation{Mutation: op.Operation == gqlast.Mutation}
	rootType := "Query"
	if out.Mutation {
		rootType = "Mutation"
	}
	spine := map[*position]bool{}
	var positions []*position
	skipped := 0
	for _, field := range fields(op.SelectionSet) {
		if !isObject(field) {
			continue
		}
		out.RootFields = append(out.RootFields, field.Name)
		root := a.build(nil, rootType, field, &out)
		positions = append(positions, flatten(root)...)
		start := root
		if out.Mutation {
			if a.mutation(root, &out) {
				skipped++
			}
			start = payloadObject(root)
		}
		for _, on := range spineFrom(start) {
			spine[on] = true
			out.Collection = on.sig.list || on.connectionItem
		}
	}
	out.Skip = skipped > 0 && skipped == len(out.RootFields)
	out.Name = fmt.Sprintf("%s %s", op.Operation, strings.Join(out.RootFields, " "))
	if name != "" {
		out.Name += " (" + name + ")"
	}
	for _, at := range positions {
		out.Paths = append(out.Paths, at.path)
		if element, ok := a.judge(at, spine); ok {
			out.Elements = append(out.Elements, element)
		}
	}
	sort.Strings(out.Paths)
	out.Paths = slices.Compact(out.Paths)
	return out
}

// mutation reads a mutation field's own requirement into its operation: its
// groups beside any other field's, and the first field that declares nothing.
// It reports whether the field opts out of the check, which leaves the
// operation to GitLab only when every field does.
func (a *analyzer) mutation(root *position, out *Operation) (skipped bool) {
	mutation, known := a.authz.Mutations[root.field]
	switch {
	case !known || len(mutation.Granular) == 0:
		if out.Undeclared == "" {
			out.Undeclared = root.field
		}
	case anySkip(mutation.Granular):
		return true
	default:
		out.Groups = dedupeRequirements(append(out.Groups, directiveGroups(mutation.Granular)...))
	}
	return false
}

// payloadObject is where a mutation's answer spine starts: the first object
// its payload selects other than errors, or nil for a payload that selects
// none.
func payloadObject(root *position) *position {
	for _, child := range root.children {
		if child.field != "errors" {
			return child
		}
	}
	return nil
}

// spineFrom follows the answer spine from a position: on, while the position
// selects one object and nothing but connection framing beside it.
func spineFrom(start *position) []*position {
	if start == nil {
		return nil
	}
	spine := []*position{start}
	at := start
	for {
		var next *position
		others := 0
		for _, child := range at.children {
			switch {
			case framing[child.field]:
			case next == nil:
				next = child
			default:
				others++
			}
		}
		for _, scalar := range at.scalars {
			if !framing[scalar] {
				others++
			}
		}
		if next == nil || others > 0 {
			return spine
		}
		at = next
		spine = append(spine, at)
	}
}

// build builds the position a field selects and every object position under
// it. Its signature is the record's where the record describes the parent
// type, and the pinned schema's elsewhere.
func (a *analyzer) build(parent *position, owner string, field *gqlast.Field, out *Operation) *position {
	at := &position{
		parent:   parent,
		typeName: field.Definition.Type.Name(),
		owner:    owner,
		field:    field.Name,
		path:     field.Alias,
	}
	if parent != nil {
		at.path = parent.path + "." + at.path
		at.connectionItem = connectionItems[field.Name] && isConnection(parent)
	}
	if recorded, ok := a.authz.Types[owner].ObjectFields[field.Name]; ok {
		at.sig = parseSignature(recorded.Type)
	} else {
		at.sig = schemaSignature(field.Definition.Type)
		out.Fallbacks++
	}
	for _, selected := range fields(field.SelectionSet) {
		if isObject(selected) {
			at.children = append(at.children, a.build(at, at.typeName, selected, out))
		} else {
			at.scalars = append(at.scalars, selected.Name)
		}
	}
	return at
}

// flatten lists a position and every position under it, depth first.
func flatten(at *position) []*position {
	out := []*position{at}
	for _, child := range at.children {
		out = append(out, flatten(child)...)
	}
	return out
}

// isConnection reports whether a position is a connection or one of its
// edges, read from the type's name as GitLab names them.
func isConnection(at *position) bool {
	return strings.HasSuffix(at.typeName, "Connection") || strings.HasSuffix(at.typeName, "Edge")
}

// judge reads the element one position is, and whether it is one: a position
// GitLab checks, by its type or by a declaration on its field.
func (a *analyzer) judge(at *position, spine map[*position]bool) (Element, bool) {
	members := []string{at.typeName}
	abstract, isAbstract := a.authz.Abstract[at.typeName]
	element := Element{Path: at.path, Type: at.typeName}
	if isAbstract {
		members = slices.Clone(abstract.PossibleTypes)
		element.Members = members
	}
	checked, allSkip := false, true
	for _, member := range members {
		recorded := a.authz.Types[member]
		if !recorded.Enforced || member == "Query" || member == "Mutation" {
			continue
		}
		checked = true
		if anySkip(recorded.Granular) {
			continue
		}
		allSkip = false
		if len(recorded.Granular) == 0 {
			element.Undeclared = true
			continue
		}
		element.Groups = append(element.Groups, directiveGroups(recorded.Granular)...)
	}
	if fieldLevel := a.authz.Fields[at.owner+"."+at.field]; len(fieldLevel) > 0 && !anySkip(fieldLevel) {
		checked, allSkip = true, false
		element.Groups = append(element.Groups, directiveGroups(fieldLevel)...)
	}
	if !checked {
		return Element{}, false
	}
	element.Skip = allSkip
	element.Groups = dedupeRequirements(element.Groups)
	landing, effect := a.landing(at)
	element.Effect = effect
	element.Fatal = spine[landing]
	return element, true
}

// landing reads where a denial at a position takes the answer, and what it
// does there: a redacted list loses the item, a nullable position goes null,
// and a non-null one hands its null to the nearest nullable ancestor.
func (a *analyzer) landing(at *position) (*position, finegrained.Effect) {
	if (at.sig.list || at.connectionItem) && a.redacted(at) {
		return at, finegrained.EffectRemoved
	}
	return nullAt(at)
}

// nullAt follows a null up from a position: an item of a list of nullable
// items goes null in place, a list of non-null items goes null whole, a
// nullable object goes null, and a non-null one hands it to its parent.
func nullAt(at *position) (*position, finegrained.Effect) {
	if at.sig.list {
		if !at.sig.itemNonNull {
			return at, finegrained.EffectNull
		}
		if !at.sig.fieldNonNull {
			return at, finegrained.EffectListNull
		}
	} else if !at.sig.fieldNonNull {
		return at, finegrained.EffectNull
	}
	if at.parent == nil {
		return at, finegrained.EffectNull
	}
	return nullAt(at.parent)
}

// redacted reports whether GitLab removes the denied items of a list rather
// than nulling them: a connection's when the item type carries abilities or a
// directive, a plain list's when it carries abilities. An abstract position
// is redacted only when every member type would be.
func (a *analyzer) redacted(at *position) bool {
	members := []string{at.typeName}
	if abstract, ok := a.authz.Abstract[at.typeName]; ok {
		members = abstract.PossibleTypes
	}
	for _, member := range members {
		recorded := a.authz.Types[member]
		hasAbilities := len(recorded.Abilities) > 0
		if !hasAbilities && (!at.connectionItem || len(recorded.Granular) == 0) {
			return false
		}
	}
	return true
}

// directiveGroups reads a set of directives into requirement groups: one per
// requirement group, with the permissions of the group's first directive
// only, as GitLab reads them, held at any boundary type a directive of the
// group names.
func directiveGroups(directives []apilive.Directive) []requirement {
	var order []string
	byGroup := map[string]*requirement{}
	for _, directive := range directives {
		group, seen := byGroup[directive.RequirementGroup]
		if !seen {
			group = &requirement{perms: sorted(directive.Permissions)}
			byGroup[directive.RequirementGroup] = group
			order = append(order, directive.RequirementGroup)
		}
		group.any |= boundaryOf(directive.BoundaryType)
	}
	out := make([]requirement, 0, len(order))
	for _, name := range order {
		group := byGroup[name]
		if group.any == 0 {
			group.any = finegrained.AllBoundaries
		}
		out = append(out, *group)
	}
	return out
}

// anySkip reports whether some directive opts out, which opts the whole set
// out (GitLab's skip_authorization?).
func anySkip(directives []apilive.Directive) bool {
	for _, directive := range directives {
		if directive.SkipReason != "" {
			return true
		}
	}
	return false
}

// dedupeRequirements drops repeated groups, keeping the first of each.
func dedupeRequirements(groups []requirement) []requirement {
	seen := map[string]bool{}
	var out []requirement
	for _, group := range groups {
		if !seen[group.key()] {
			seen[group.key()] = true
			out = append(out, group)
		}
	}
	return out
}

// fields flattens a selection set to its fields, fragments spread in.
func fields(set gqlast.SelectionSet) []*gqlast.Field {
	var out []*gqlast.Field
	for _, selection := range set {
		switch typed := selection.(type) {
		case *gqlast.Field:
			out = append(out, typed)
		case *gqlast.InlineFragment:
			out = append(out, fields(typed.SelectionSet)...)
		case *gqlast.FragmentSpread:
			out = append(out, fields(typed.Definition.SelectionSet)...)
		}
	}
	return out
}

// isObject reports whether a field selects an object: it carries a selection
// set of its own.
func isObject(field *gqlast.Field) bool {
	return len(field.SelectionSet) > 0
}
