package testutil

import (
	"errors"
	"fmt"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// GitLabAuthenticatedMaxComplexity is the complexity above which GitLab
// refuses a query from an authenticated user who is not an administrator,
// AUTHENTICATED_MAX_COMPLEXITY in app/graphql/gitlab_schema.rb. Every token
// this server is handed is held to it unless its user is an administrator
// (300) or the request carries none (200). GitLab computes the figure from
// the document and its variables before it runs anything, and refuses the
// whole query above it, so a document over it fails on every call however
// small the answer would have been.
const GitLabAuthenticatedMaxComplexity = 250

// gitLabDefaultMaxPageSize is GitlabSchema's default_max_page_size, the page
// a connection serves, and is charged for, when a query names neither first
// nor last.
const gitLabDefaultMaxPageSize = 100

// gitLabFieldCost is what GitLab charges for one field, as Types::BaseField
// computes it, where that differs from its default.
type gitLabFieldCost struct {
	// own is the field's cost before its children are added. GitLab's
	// default is one; a field that calls Gitaly costs one more
	// (base_complexity), a resolver adds one when the query carries a sort
	// argument and five for a search (Resolvers::BaseResolver
	// .resolver_complexity), and a field declared with complexity 0 costs
	// nothing.
	own int
	// multiplier is the resolver's complexity_multiplier, for a field GitLab
	// charges per item it may load. The field's cost, its children included,
	// grows by that cost times the page size times the multiplier, the page
	// size being the smallest of first, last and the default page size.
	multiplier float64
}

// gitLabFieldCosts are the fields whose cost GitLab computes as something
// other than one plus their children, keyed by the type that declares the
// field and its name. Every field missing here is charged one plus its
// children, which is GitLab's default and is wrong for a field that calls
// Gitaly, a resolver that charges per item or a field declared free, so an
// estimate is trusted only where a test holds it equal to a figure GitLab
// reported: each entry is here because a document measured on GitLab.com
// needed it to agree, and names the Ruby that sets it.
var gitLabFieldCosts = map[string]gitLabFieldCost{
	// Resolvers::WorkItems::WorkItemDiscussionsResolver: calls_gitaly!, a sort
	// argument GitLab always supplies a default for, and complexity_multiplier
	// 0.05 with calculate_ext_conn_complexity, so a hundred threads cost six
	// times what one does.
	"WorkItemWidgetNotes.discussions": {own: 3, multiplier: 0.05},
	// Types::Notes::NoteType author_is_contributor, calls_gitaly: true.
	"Note.authorIsContributor": {own: 2},
	// Types::WorkItems::WidgetInterface type, complexity: 0, resolved from the
	// widget already loaded.
	"WorkItemWidget.type": {own: 0},
}

// complexitySchema is the loader the estimate goes through. It is a variable
// so the load-failure path stays reachable from a test, since the embedded
// schema cannot be made to fail from outside its package.
var complexitySchema = graphqlschema.Schema

// GitLabQueryComplexity estimates the complexity GitLab computes for document
// sent with variables, which is the figure it refuses a query on
// ([GitLabAuthenticatedMaxComplexity]).
//
// No unit test reaches GitLab and the validating transport judges a document
// by the schema alone, which says nothing about cost, so a selection that grew
// past the limit used to pass every test and fail on every instance. This is
// GitLab's computation, legacy mode of graphql-ruby's QueryComplexity with
// Types::BaseField's field costs, run over the pinned schema: each field costs
// its own cost, one unless [gitLabFieldCosts] records otherwise, plus its
// children; a selection set costs, for each object type its selections can
// resolve to, the sum of the fields that apply to that type, whether selected
// on it or on an interface it implements, and the largest of those sums.
//
// It is an estimate because the field costs live in GitLab's Ruby rather than
// in its schema, and the table holds only those a measurement has needed. A
// caller therefore holds the estimate equal to a figure GitLab reported for
// the same document, so that a change to the selection changes the estimate
// and sends whoever made it to measure again.
//
// It refuses what it does not model rather than guessing: a document that
// GitLab would not accept, one defining other than one operation, a selection
// carrying a directive (skip and include change what is counted), a response
// key selected twice on one type, and a page size that is not a number.
func GitLabQueryComplexity(document string, variables map[string]any) (int, error) {
	schema, err := complexitySchema()
	if err != nil {
		return 0, err
	}
	parsed, err := graphqlschema.ParseAgainst(schema, document)
	if err != nil {
		return 0, err
	}
	if len(parsed.Operations) != 1 {
		return 0, fmt.Errorf("the document defines %d operations, and an estimate is of one", len(parsed.Operations))
	}
	estimate := complexityEstimate{schema: schema, variables: variables}
	return estimate.selectionSet(parsed.Operations[0].SelectionSet)
}

// complexityEstimate carries the schema and the variables one estimate reads,
// so the walk can recurse as methods rather than threading both through every
// call.
type complexityEstimate struct {
	schema    *ast.Schema
	variables map[string]any
}

// selectionSet returns what GitLab charges for one selection set: the fields
// grouped by the type each is selected on, every group of an interface or a
// union counted towards each object type that implements it, and the largest
// sum over the object types.
func (e complexityEstimate) selectionSet(set ast.SelectionSet) (int, error) {
	var fields []*ast.Field
	if err := e.collect(set, &fields); err != nil {
		return 0, err
	}
	perType := map[string][]*ast.Field{}
	var order []string
	for _, field := range fields {
		for _, concrete := range e.concreteTypes(field.ObjectDefinition) {
			if _, seen := perType[concrete]; !seen {
				order = append(order, concrete)
			}
			perType[concrete] = append(perType[concrete], field)
		}
	}
	largest := 0
	for _, concrete := range order {
		sum, err := e.sum(concrete, perType[concrete])
		if err != nil {
			return 0, err
		}
		largest = max(largest, sum)
	}
	return largest, nil
}

// collect flattens set into the fields it selects, through inline fragments
// and fragment spreads, each field keeping the type it was selected on.
func (e complexityEstimate) collect(set ast.SelectionSet, fields *[]*ast.Field) error {
	for _, selection := range set {
		switch node := selection.(type) {
		case *ast.Field:
			if len(node.Directives) > 0 {
				return fmt.Errorf("field %q carries a directive, which this estimate does not evaluate", node.Name)
			}
			*fields = append(*fields, node)
		case *ast.InlineFragment:
			if len(node.Directives) > 0 {
				return fmt.Errorf("the fragment on %q carries a directive, which this estimate does not evaluate", node.TypeCondition)
			}
			if err := e.collect(node.SelectionSet, fields); err != nil {
				return err
			}
		default:
			// A fragment spread, the one other selection gqlparser builds.
			spread, _ := node.(*ast.FragmentSpread)
			if len(spread.Directives) > 0 {
				return fmt.Errorf("the spread of %q carries a directive, which this estimate does not evaluate", spread.Name)
			}
			if err := e.collect(spread.Definition.SelectionSet, fields); err != nil {
				return err
			}
		}
	}
	return nil
}

// concreteTypes returns the object types a selection made on scope applies
// to: the type itself when it is one, and every object type implementing it
// or belonging to it when it is an interface or a union.
func (e complexityEstimate) concreteTypes(scope *ast.Definition) []string {
	if scope.Kind == ast.Object {
		return []string{scope.Name}
	}
	var names []string
	for _, possible := range e.schema.GetPossibleTypes(scope) {
		if possible.Kind == ast.Object {
			names = append(names, possible.Name)
		}
	}
	return names
}

// sum adds up the fields one concrete type receives, each under its response
// key, which is what Alias holds: gqlparser fills it with the field's name
// when the query gives none.
func (e complexityEstimate) sum(concrete string, fields []*ast.Field) (int, error) {
	seen := map[string]bool{}
	total := 0
	for _, field := range fields {
		key := field.Alias
		if seen[key] {
			return 0, fmt.Errorf("%s receives %q twice, and this estimate does not merge a repeated selection", concrete, key)
		}
		seen[key] = true
		cost, err := e.field(field)
		if err != nil {
			return 0, err
		}
		total += cost
	}
	return total, nil
}

// field returns what GitLab charges for one field and everything under it,
// following Types::BaseField: the field's own cost plus its children, and for
// a field charged per item that sum grown by itself times the page size times
// the multiplier, truncated to an integer as Ruby's to_i does.
func (e complexityEstimate) field(field *ast.Field) (int, error) {
	children, err := e.selectionSet(field.SelectionSet)
	if err != nil {
		return 0, err
	}
	cost := e.fieldCost(field.ObjectDefinition, field.Name)
	total := cost.own + children
	if cost.multiplier == 0 {
		return total, nil
	}
	limit, err := pageLimit(field.ArgumentMap(e.variables))
	if err != nil {
		return 0, fmt.Errorf("%s.%s: %w", field.ObjectDefinition.Name, field.Name, err)
	}
	// The conversion truncates toward zero, which for a positive cost is what
	// Ruby's to_i does.
	return int(float64(total) + float64(total)*(float64(limit)*cost.multiplier)), nil
}

// fieldCost returns what [gitLabFieldCosts] records for name selected on
// typ, reading the interfaces typ implements as well, since a field an
// interface declares keeps its cost on every type that inherits it, and
// GitLab's default of one when neither records it.
func (e complexityEstimate) fieldCost(typ *ast.Definition, name string) gitLabFieldCost {
	if cost, known := gitLabFieldCosts[typ.Name+"."+name]; known {
		return cost
	}
	for _, declaring := range e.schema.GetImplements(typ) {
		if cost, known := gitLabFieldCosts[declaring.Name+"."+name]; known {
			return cost
		}
	}
	return gitLabFieldCost{own: 1}
}

// errPageSizeNotNumber is returned for a first or last that is neither absent
// nor a number, which GitLab would have refused before counting anything.
var errPageSizeNotNumber = errors.New("the page size is not a number")

// pageLimit returns the page size GitLab charges a connection for: the
// smallest of first, last and the default page size.
func pageLimit(arguments map[string]any) (int64, error) {
	limit := int64(gitLabDefaultMaxPageSize)
	for _, name := range []string{"first", "last"} {
		var size int64
		switch value := arguments[name].(type) {
		case nil:
			continue
		case int:
			size = int64(value)
		case int64:
			size = value
		case float64:
			size = int64(value)
		default:
			return 0, fmt.Errorf("%s is %T: %w", name, value, errPageSizeNotNumber)
		}
		limit = min(limit, size)
	}
	return limit, nil
}
