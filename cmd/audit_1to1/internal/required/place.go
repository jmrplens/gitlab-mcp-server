package required

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// This file is how a parameter GitLab declares on a route is found among the
// fields of an input schema. Most of them are found by name, because the
// inputs here spell a parameter the way GitLab does (the 1:1 policy R-INPUT
// already holds them to). The exception is a path parameter: GitLab writes the
// project of /projects/:id as `id`, and every input here writes it as
// `project_id`, since an input names several identifiers at once and a bare
// `id` would say none of them.

// placements maps each field name a route's parameters are found under to
// whether GitLab requires it there. A parameter nested under another
// (`position[base_sha]`) is left to its parent, which GitLab declares too and
// which is the name an input publishes.
//
// A parameter the path names is required whatever its declaration says,
// unless it sits in an optional group: no request reaches the route without a
// value in that segment. The record says otherwise for every path parameter
// a route never redeclares in a params block (GET /projects/:id/jobs/:job_id
// declares job_id and leaves id to the path, which the record writes as
// neither required nor typed), so reading the declaration alone would call the
// project of most project routes optional.
//
// aliases names, by GitLab parameter, the field an action fills it from where
// the rules above would find the wrong one (see [declaredAliases]); the
// parameters it names are reported in aliased, so a declaration that names
// none the action sends can be told stale.
func placements(route *apilive.Route, aliases map[string]string, aliased map[string]bool) map[string]bool {
	path := apilive.NormalizePath(route.Path)
	fields := pathFields(path)
	placed := make(map[string]bool, len(route.Params))
	for name, param := range route.Params {
		if strings.Contains(name, "[") {
			continue
		}
		field, required := name, param.Required
		if inPath, ok := fields[name]; ok {
			field = inPath.field
			required = required || inPath.always
		}
		if alias, ok := aliases[name]; ok {
			field = alias
			aliased[name] = true
		}
		placed[field] = placed[field] || required
	}
	return placed
}

// pathParam is one parameter a path names: the field an input names it by,
// and whether every spelling of the path carries it.
type pathParam struct {
	field  string
	always bool
}

// pathFields maps each parameter named in a path to the field an input names
// it by. A placeholder spelled `:id` is the collection in front of it in the
// singular with `_id` after it (`/projects/:id` is project_id, `/users/:id`
// user_id), and every other placeholder is its own name, which is how GitLab
// already spells the second identifier of a path (`:issue_iid`, `:user_id`).
// A wildcard (`*file_path`) is a placeholder too. A placeholder inside an
// optional group is one some spelling of the path leaves out, which is how it
// is told apart: [apilive.ExpandOptional] spells the path with each group
// taken and left out, and a placeholder every spelling carries is always sent.
func pathFields(path string) map[string]pathParam {
	variants := apilive.ExpandOptional(path)
	seen := map[string]int{}
	fields := map[string]pathParam{}
	for _, variant := range variants {
		segments := strings.Split(strings.Trim(variant, "/"), "/")
		for index, segment := range segments {
			name, isParam := placeholderName(segment)
			if !isParam {
				continue
			}
			// Two ifs rather than a switch, so the mutation tool, which
			// cannot see a case expression, measures both conditions. They
			// cannot both hold: "id" does not end in "able_id".
			field := name
			if name == "id" {
				field = idField(segments[:index])
			}
			if strings.HasSuffix(name, "able_id") {
				field = polymorphicField(name, segments[:index])
			}
			seen[name]++
			fields[name] = pathParam{field: field}
		}
	}
	for name, param := range fields {
		param.always = seen[name] == len(variants)
		fields[name] = param
	}
	return fields
}

// placeholderName reports the parameter a path segment names, when it names
// one.
func placeholderName(segment string) (string, bool) {
	for _, prefix := range []string{":", "*"} {
		if name, ok := strings.CutPrefix(segment, prefix); ok && name != "" {
			return name, true
		}
	}
	return "", false
}

// idField names the field a bare `:id` is spelled as: the nearest literal
// segment before it, singular, with `_id` after it. A path whose `:id` has no
// literal before it keeps the name `id`.
func idField(before []string) string {
	for _, segment := range slices.Backward(before) {
		if _, isParam := placeholderName(segment); isParam || segment == "-" {
			continue
		}
		return singular(segment) + "_id"
	}
	return "id"
}

// polymorphicIdentifiers names the field an input identifies a member of each
// collection by, where GitLab names the member polymorphically. One route
// serves notes, discussions, subscriptions and resource events of several
// kinds of object (`/projects/:id/issues/:noteable_id/notes`), and the
// parameter is the object's own identifier, which for an issue or a merge
// request is its internal ID and for a commit its SHA.
var polymorphicIdentifiers = map[string]string{
	"commits":        "sha",
	"epics":          "epic_iid",
	"issues":         "issue_iid",
	"merge_requests": "merge_request_iid",
	"snippets":       "snippet_id",
}

// polymorphicField names the field a polymorphic placeholder (`:noteable_id`,
// `:subscribable_id`, `:eventable_id`) is spelled as: the identifier of the
// collection in front of it, or the placeholder's own name for a collection
// the table does not hold.
func polymorphicField(name string, before []string) string {
	if len(before) > 0 {
		if field, ok := polymorphicIdentifiers[before[len(before)-1]]; ok {
			return field
		}
	}
	return name
}

// singular turns the plural name of a GitLab collection into the singular an
// input names one of its members by. The collections are English plurals in
// three shapes, and a name that is none of them is returned as written.
func singular(plural string) string {
	switch {
	case strings.HasSuffix(plural, "ies"):
		return strings.TrimSuffix(plural, "ies") + "y"
	case strings.HasSuffix(plural, "sses"), strings.HasSuffix(plural, "xes"):
		return strings.TrimSuffix(plural, "es")
	case strings.HasSuffix(plural, "s"):
		return strings.TrimSuffix(plural, "s")
	}
	return plural
}
