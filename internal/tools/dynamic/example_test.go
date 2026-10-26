// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package dynamic

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// exampleCatalog names one catalog a server can serve: a tier on a
// self-managed instance or on GitLab.com.
type exampleCatalog struct {
	name   string
	tier   edition.Tier
	dotcom bool
}

// servedCatalogs is every tier on both instance classes. The tier prunes
// fields from the input schemas and GitLab.com adds the Orbit group, so an
// example that satisfies its schema on one build can fail on another.
var servedCatalogs = []exampleCatalog{
	{name: "free", tier: edition.Free},
	{name: "premium", tier: edition.Premium},
	{name: "ultimate", tier: edition.Ultimate},
	{name: "gitlab.com free", tier: edition.Free, dotcom: true},
	{name: "gitlab.com premium", tier: edition.Premium, dotcom: true},
	{name: "gitlab.com ultimate", tier: edition.Ultimate, dotcom: true},
}

// buildServedCatalog builds the catalog the dynamic surface serves for one
// tier and instance class, standalone actions included.
func buildServedCatalog(t *testing.T, served exampleCatalog) *actioncatalog.Catalog {
	t.Helper()
	var client *gitlabclient.Client
	if served.dotcom {
		var err error
		client, err = gitlabclient.NewClientWithToken("https://gitlab.com", "test-token", false)
		if err != nil {
			t.Fatalf("NewClientWithToken(gitlab.com) error = %v", err)
		}
	}
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: served.tier, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog(%s) error = %v", served.name, err)
	}
	catalog, err = AddStandaloneCatalog(catalog, client, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog(%s) error = %v", served.name, err)
	}
	return catalog
}

// TestFind_EveryExampleSatisfiesItsInputSchema verifies that the execute call
// find and describe offer for an action is one its own input schema accepts,
// for every action of every catalog a server can serve. The example is what a
// model copies into its first call, so one its schema refuses teaches the
// model to send a call that fails before it reaches GitLab: a hex color filled
// with "value", every branch of an anyOf filled at once, a date written as
// its template. The schema is validated as published, and the formats the
// validator leaves to annotation (date, date-time, uri, email, hostname) are
// checked here. The
// example is also put to execute's own check of the params, which is what
// refuses a call before any handler runs.
//
// The same walk holds the two lists the example is built from: required_params
// names what every call carries and nothing an alternative names, and the
// example fills exactly those plus the first alternative group.
func TestFind_EveryExampleSatisfiesItsInputSchema(t *testing.T) {
	for _, served := range servedCatalogs {
		t.Run(served.name, func(t *testing.T) {
			registry := NewRegistryFromCatalog(buildServedCatalog(t, served))
			for _, entry := range registry.entries {
				checkPublishedExample(t, entry, registry.describeEntry(entry))
			}
		})
	}
}

// formatBearingKind is one kind of string whose form GitLab checks, told by
// the parameter's name or description: what the parameter's published schema
// states about it, and what find's example sends for it.
type formatBearingKind struct {
	// what names the kind in a failure.
	what string
	// names matches the names of the parameters that hold it.
	names *regexp.Regexp
	// descriptions matches the descriptions that say a parameter holds it,
	// whatever its name. Nil when the name alone tells.
	descriptions *regexp.Regexp
	// declared reports what the schema of the parameter at path, in the
	// action id, fails to state, or nil.
	declared func(id, path string, schema map[string]any) error
	// example reports what is wrong with the value the example sends, or
	// nil. params is the whole example, for a value GitLab holds to another
	// parameter.
	example func(value string, params map[string]any) error
}

// fullSHAParams are the parameters GitLab holds to a full commit SHA, each
// with the reason. Every other parameter named for a SHA also takes a ref
// name or an abbreviated SHA, which GitLab resolves to the commit, so a
// pattern there would refuse the branch names that work today; it gets a hex
// example and no pattern, as decided on issue 1272.
var fullSHAParams = map[string]string{
	"external_status_check.set_project_mr_status sha": "GitLab compares it with the merge request's diff_head_sha and answers 409 when the two differ",
	"repository.update_submodule commit_sha":          "Gitaly refuses an object ID shorter than the repository's hash, which the service answers as invalid parameters",
	"repository.commit_statuses sha":                  "GitLab finds the commit by any revision and then selects the statuses whose pipeline sha equals the value, so a branch name or an abbreviated SHA answers an empty list",
}

// severalPerValueParams are the parameters that take several values of their
// kind in one string, each with the reason, keyed as "action path". A format
// names a single value, so they declare none, and their example is still one
// value of the kind. An invitation's email, which takes several addresses
// separated by commas, is not here only because no example fills it. A key
// no catalog fills fails the test.
var severalPerValueParams = map[string]string{
	"admin.application_create redirect_uri": "Doorkeeper splits the value on whitespace and checks each URI, so an application takes several, one per line",
}

// formatBearingExemptions are the parameters a kind's name or description
// matches that do not hold it, each with the reason, keyed as "action path".
// A key no catalog serves any more fails the test.
var formatBearingExemptions = map[string]string{
	"project.dependency_firewall_evaluate version": "a package's version is free text to GitLab, any string of at most 255 characters, since each package type writes its own",
}

// formatBearingKinds are the kinds of string GitLab checks the form of that a
// required parameter of the catalog holds. A duration and a SHA on a route
// that takes a ref name declare no pattern, and say why.
var formatBearingKinds = []formatBearingKind{
	{
		what:         "an email address",
		names:        regexp.MustCompile(`^(.+_)?email$`),
		descriptions: regexp.MustCompile(`(?i)\bemail address\b`),
		declared:     declaresFormatOfOne("email"),
		example:      satisfiesFormat("email"),
	},
	{
		what:     "an absolute URL",
		names:    regexp.MustCompile(`^(.+_)?(url|uri)$`),
		declared: declaresFormatOfOne("uri"),
		example:  satisfiesFormat("uri"),
	},
	{
		what:         "a host name",
		names:        regexp.MustCompile(`^(domain|hostname)$`),
		descriptions: regexp.MustCompile(`(?i)\bdomain name\b`),
		declared:     declaresFormat("hostname"),
		example:      satisfiesFormat("hostname"),
	},
	{
		what:     "a date",
		names:    regexp.MustCompile(`^((.+_)?date|.+_(at|after|before))$`),
		declared: declaresOneOfFormats("date", "date-time"),
		example: func(value string, _ map[string]any) error {
			if checkExampleFormat("date", value, "") == nil {
				return nil
			}
			return checkExampleFormat("date-time", value, fmt.Sprintf("%q", value))
		},
	},
	{
		what:  "a git object SHA",
		names: regexp.MustCompile(`^(.+_)?sha$`),
		// The element of a list of SHAs, named for a list that is not
		// (merge_request.context_commits_create's commits). A list that
		// also takes branch names says so beside its SHAs, as
		// repository.merge_base's refs does, and a ref that also takes a
		// SHA says "commit SHA".
		descriptions: regexp.MustCompile(`(?i)\blist of commit SHAs\b`),
		declared: func(id, path string, schema map[string]any) error {
			if _, full := fullSHAParams[id+" "+path]; full {
				return declaresPattern(
					[]string{strings.Repeat("0a", 20), strings.Repeat("0a", 32)},
					[]string{strings.Repeat("0A", 20), "0a1b2c3", "main", strings.Repeat("0a", 26)},
				)(id, path, schema)
			}
			return declaresNoPattern("it also takes a ref name or an abbreviated SHA")(id, path, schema)
		},
		example: matches(`^[0-9a-f]{40}$`),
	},
	{
		what:         "a cron expression",
		names:        regexp.MustCompile(`^(cron|freeze_start|freeze_end)$`),
		descriptions: regexp.MustCompile(`(?i)\bcron expression\b`),
		declared: declaresPattern(
			[]string{"0 1 * * *", "*/15 9-17 * * mon-fri", "0 0 1,15 * *", "0 0 * * 1#2", "0 0 L * *", "30 0 1 * * *", "@daily", "@hourly", " 0 1 * * * "},
			[]string{"every day", "daily", "0 1 * *", "0 1 * * * * *", "@reboot", "@DAILY", ""},
		),
		example: matches(`^[0-9*/,-]+( [0-9*/,-]+){4}$`),
	},
	{
		// GitLab matches Gitlab::Regex.unbounded_semver_regex anywhere in
		// the value, so a prefix and a fourth number are taken.
		what:         "a semantic version",
		names:        regexp.MustCompile(`^version$`),
		descriptions: regexp.MustCompile(`(?i)\bsemantic versioning\b`),
		declared: declaresPattern(
			[]string{"1.0.0", "v1.2.3", "1.0.0-rc.1+build.5", "release-10.20.30", "1.2.3.4"},
			[]string{"value", "1.0", "v1", "1.02.3", ""},
		),
		example: matches(`^[0-9]+\.[0-9]+\.[0-9]+$`),
	},
	{
		what:     "a duration",
		names:    regexp.MustCompile(`^duration$`),
		declared: declaresNoPattern("GitLab also takes forms such as 1 hour that a pattern would refuse"),
		example:  matches(`^([0-9]+(mo|[wdhm]))+$`),
	},
	{
		what:  "a SHA-256 digest",
		names: regexp.MustCompile(`^(.+_)?digest$`),
		declared: declaresPattern(
			[]string{strings.Repeat("0a", 32), strings.Repeat("0A", 32), "sha256:" + strings.Repeat("0a", 32)},
			[]string{strings.Repeat("0a", 20), strings.Repeat("0a", 31) + "0", "SHA256:" + strings.Repeat("0a", 32), "value"},
		),
		example: matches(`^[0-9a-f]{64}$`),
	},
	{
		what:  "an SSH key fingerprint",
		names: regexp.MustCompile(`^fingerprint$`),
		declared: declaresPattern(
			[]string{
				"SHA256:" + strings.Repeat("A", 43),
				"sha256:" + strings.Repeat("a", 43) + "=",
				strings.TrimSuffix(strings.Repeat("aa:", 16), ":"),
				"MD5:" + strings.TrimSuffix(strings.Repeat("0F:", 16), ":"),
			},
			[]string{"SHA256:abc123", "aa:bb:cc", "MD5:aa:bb:cc", "value"},
		),
		example: func(value string, _ map[string]any) error {
			digest, ok := strings.CutPrefix(value, "SHA256:")
			if !ok {
				return fmt.Errorf("%q is not the SHA256 form ssh-keygen prints", value)
			}
			if decoded, err := base64.RawStdEncoding.DecodeString(digest); err != nil || len(decoded) != 32 {
				return fmt.Errorf("%q does not hold the 32 bytes of a SHA-256 digest", value)
			}
			return nil
		},
	},
	{
		what:  "a container repository path pattern",
		names: regexp.MustCompile(`^repository_path_pattern$`),
		declared: declaresPattern(
			[]string{"group/project/*", "group/project/image-*", "group/project", "my.group/my_project/*"},
			[]string{"Group/Project/*", "*/image", "group//project", "group/project/"},
		),
		example: func(value string, params map[string]any) error {
			project, _ := params["project_id"].(string)
			if project == "" || !strings.HasPrefix(strings.ToLower(value), strings.ToLower(project)) {
				return fmt.Errorf("%q does not start with the example's project path %q, which GitLab requires", value, project)
			}
			return nil
		},
	},
}

// declaresFormat returns a declared check requiring the format given.
func declaresFormat(format string) func(id, path string, schema map[string]any) error {
	return declaresOneOfFormats(format)
}

// declaresFormatOfOne returns a declared check requiring the format given,
// which names one value, of every parameter but those severalPerValueParams
// names, which take several in one string and declare no format.
func declaresFormatOfOne(format string) func(id, path string, schema map[string]any) error {
	return func(id, path string, schema map[string]any) error {
		if reason, several := severalPerValueParams[id+" "+path]; several {
			if declared, ok := schema["format"]; ok {
				return fmt.Errorf("declares format %v, and it should declare none: %s", declared, reason)
			}
			return nil
		}
		return declaresFormat(format)(id, path, schema)
	}
}

// declaresOneOfFormats returns a declared check requiring one of the formats
// given.
func declaresOneOfFormats(formats ...string) func(id, path string, schema map[string]any) error {
	return func(_, _ string, schema map[string]any) error {
		if format, _ := schema["format"].(string); slices.Contains(formats, format) {
			return nil
		}
		return fmt.Errorf("declares format %v, want %s", schema["format"], strings.Join(formats, " or "))
	}
}

// declaresPattern returns a declared check requiring a pattern that accepts
// every value of accepts and refuses every value of refuses: values a call is
// served with, as GitLab or the handler before it reads them, and values
// GitLab refuses, so the pattern states the rule rather than any shape.
func declaresPattern(accepts, refuses []string) func(id, path string, schema map[string]any) error {
	return func(_, _ string, schema map[string]any) error {
		pattern, ok := schema["pattern"].(string)
		if !ok {
			return errors.New("declares no pattern")
		}
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("declares the pattern %q, which does not compile: %w", pattern, err)
		}
		for _, value := range accepts {
			if !expression.MatchString(value) {
				return fmt.Errorf("declares the pattern %q, which refuses %q, a value GitLab takes", pattern, value)
			}
		}
		for _, value := range refuses {
			if expression.MatchString(value) {
				return fmt.Errorf("declares the pattern %q, which accepts %q, a value GitLab refuses", pattern, value)
			}
		}
		return nil
	}
}

// declaresNoPattern returns a declared check requiring no pattern, for the
// reason given.
func declaresNoPattern(reason string) func(id, path string, schema map[string]any) error {
	return func(_, _ string, schema map[string]any) error {
		if pattern, ok := schema["pattern"]; ok {
			return fmt.Errorf("declares the pattern %v, and it should declare none: %s", pattern, reason)
		}
		return nil
	}
}

// satisfiesFormat returns an example check holding the value to a format, as
// the catalog-wide test checks the formats the validator leaves to
// annotation.
func satisfiesFormat(format string) func(value string, params map[string]any) error {
	return func(value string, _ map[string]any) error {
		return checkExampleFormat(format, value, fmt.Sprintf("%q", value))
	}
}

// matches returns an example check holding the value to an expression.
func matches(expression string) func(value string, params map[string]any) error {
	compiled := regexp.MustCompile(expression)
	return func(value string, _ map[string]any) error {
		if !compiled.MatchString(value) {
			return fmt.Errorf("%q does not match %s", value, expression)
		}
		return nil
	}
}

// TestFind_FormatBearingParams_DeclareTheirFormatAndExampleIt verifies, for
// every action of every catalog a server can serve, that a string find's
// example fills whose form GitLab checks, as the parameter's name or
// description says, is published with that form and filled with a value of
// it. The example used to write "value" for each of them, an email address,
// a commit SHA, a cron expression, a duration, a redirect URI among them, and
// GitLab refused the call it built (issue 1272).
//
// The kinds are formatBearingKinds. Each states what the schema declares, a
// format, or a pattern held to values GitLab takes and refuses, and what the
// example sends. A duration declares no pattern, since GitLab also takes forms
// such as 1 hour, a SHA declares one only where GitLab holds it to a full
// SHA, fullSHAParams, and a parameter that takes several values of its kind
// in one string declares no format, severalPerValueParams; each table also
// fails on an entry no catalog fills. Only the parameters find's example
// fills are held, the required ones and the first alternative group, as
// decided on the issue.
func TestFind_FormatBearingParams_DeclareTheirFormatAndExampleIt(t *testing.T) {
	seen := map[string]bool{}
	for _, served := range servedCatalogs {
		t.Run(served.name, func(t *testing.T) {
			registry := NewRegistryFromCatalog(buildServedCatalog(t, served))
			for _, entry := range registry.entries {
				description := registry.describeEntry(entry)
				params, _ := description.Example.Arguments["params"].(map[string]any)
				visitExampleStrings(description.InputSchema, params, "", "", func(path, name string, schema map[string]any, value string) {
					seen[description.ID+" "+path] = true
					checkFormatBearingString(t, description.ID, params, path, name, schema, value)
				})
			}
		})
	}
	for table, declared := range map[string]map[string]string{"fullSHAParams": fullSHAParams, "severalPerValueParams": severalPerValueParams, "formatBearingExemptions": formatBearingExemptions} {
		t.Run(table, func(t *testing.T) {
			for key := range declared {
				if !seen[key] {
					t.Errorf("%q is declared in %s, and no example of any catalog fills it", key, table)
				}
			}
		})
	}
}

// checkFormatBearingString holds one string find's example fills, the one at
// path in the example params of the action id, to every kind its name or its
// schema's description says it holds, unless an exemption names it.
func checkFormatBearingString(t *testing.T, id string, params map[string]any, path, name string, schema map[string]any, value string) {
	t.Helper()
	if _, exempt := formatBearingExemptions[id+" "+path]; exempt {
		return
	}
	paramDescription, _ := schema["description"].(string)
	for _, kind := range formatBearingKinds {
		if !kind.names.MatchString(name) && (kind.descriptions == nil || !kind.descriptions.MatchString(paramDescription)) {
			continue
		}
		if err := kind.declared(id, path, schema); err != nil {
			t.Errorf("%s %s holds %s, and its schema %v", id, path, kind.what, err)
		}
		if err := kind.example(value, params); err != nil {
			t.Errorf("%s %s holds %s, and its example %v", id, path, kind.what, err)
		}
	}
}

// visitExampleStrings calls visit for every string an example value holds,
// with its path in the params, the name of the parameter it belongs to and
// the schema that describes it. An element of a list is named for the list
// and its path ends in []. Its schema is the list's item schema, described by
// the list's own description when it carries none: a list of strings is
// described where the list is ("List of commit SHAs ..."), and an element
// read without it would hold no kind, as the context commits' did.
func visitExampleStrings(schema map[string]any, value any, path, name string, visit func(path, name string, schema map[string]any, value string)) {
	switch typed := value.(type) {
	case string:
		visit(path, name, schema, typed)
	case map[string]any:
		properties, _ := schema["properties"].(map[string]any)
		for child, childValue := range typed {
			childSchema, _ := properties[child].(map[string]any)
			childPath := child
			if path != "" {
				childPath = path + "." + child
			}
			visitExampleStrings(childSchema, childValue, childPath, child, visit)
		}
	case []any:
		items := elementSchema(schema)
		for _, childValue := range typed {
			visitExampleStrings(items, childValue, path+"[]", name, visit)
		}
	}
}

// elementSchema returns the schema of an element of the list schema
// describes: its item schema, given the list's description when it carries
// none of its own.
func elementSchema(schema map[string]any) map[string]any {
	items, _ := schema["items"].(map[string]any)
	description, described := schema["description"]
	if _, own := items["description"]; own || !described {
		return items
	}
	element := maps.Clone(items)
	if element == nil {
		element = map[string]any{}
	}
	element["description"] = description
	return element
}

// TestExampleValue_SchemaExamples_AreOfferedFirst verifies that a parameter
// whose schema carries a JSON Schema example is offered that example, as the
// schema writes it: a value its own schema accepts that the name alone could
// never produce. A schema with no example, or an empty list of them, keeps
// the value its type and name give.
func TestExampleValue_SchemaExamples_AreOfferedFirst(t *testing.T) {
	const statement = "MATCH (p:Project {id: 1}) RETURN p.id"
	cases := []struct {
		name   string
		schema map[string]any
		want   any
	}{
		{name: "an example decoded from JSON", schema: map[string]any{"type": []any{"string", "object"}, "examples": []any{statement}}, want: statement},
		{name: "examples an override wrote in Go", schema: map[string]any{"type": "string", "examples": []string{"first", "second"}}, want: "first"},
		{name: "an example ahead of an enum", schema: map[string]any{"type": "string", "enum": []any{"a", "b"}, "examples": []any{"b"}}, want: "b"},
		{name: "no example", schema: map[string]any{"type": "string"}, want: "value"},
		{name: "an empty list", schema: map[string]any{"type": "string", "examples": []any{}}, want: "value"},
		{name: "a list of another kind", schema: map[string]any{"type": "string", "examples": "first"}, want: "value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exampleValue("query", tc.schema); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("exampleValue() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestFind_OrbitQueryExample_IsTheGQLStatementItsSchemaOffers verifies the
// call find and describe offer for orbit.query on GitLab.com: its query is the
// GQL statement the action's schema carries as its example, which GitLab.com
// answers, rather than the placeholder its name gives, which GitLab.com
// refuses as GQL that does not parse (issue 1414).
func TestFind_OrbitQueryExample_IsTheGQLStatementItsSchemaOffers(t *testing.T) {
	const want = "MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1"
	registry := NewRegistryFromCatalog(buildServedCatalog(t, exampleCatalog{name: "gitlab.com ultimate", tier: edition.Ultimate, dotcom: true}))
	entry, ok := registry.byID["orbit.query"]
	if !ok {
		t.Fatal("the GitLab.com catalog serves no orbit.query")
	}
	params, _ := registry.describeEntry(entry).Example.Arguments["params"].(map[string]any)
	if got := params["query"]; got != want {
		t.Errorf("orbit.query example query = %#v, want the GQL statement %q", got, want)
	}
}

// checkPublishedExample holds one described action to the four rules the
// catalog-wide test states: its example satisfies its input schema, execute's
// check of the params accepts it, its required params name nothing only an
// alternative requires, and its example fills exactly the required params and
// the first alternative group.
func checkPublishedExample(t *testing.T, entry actionEntry, description ActionDescription) {
	t.Helper()
	params, ok := description.Example.Arguments["params"].(map[string]any)
	if !ok {
		t.Errorf("%s example carries no params object: %#v", description.ID, description.Example.Arguments)
		return
	}
	if err := validateExampleParams(description.InputSchema, params); err != nil {
		t.Errorf("%s example params %v fail the action's input schema: %v", description.ID, params, err)
	}
	if refused := validateDynamicExecuteParams(entry, params); refused != nil {
		t.Errorf("%s example params %v are refused by execute: %s", description.ID, params, textContent(refused))
	}
	want := append([]string(nil), description.RequiredParams...)
	for _, group := range description.RequiredParamsAnyOf {
		if shared := slices.IndexFunc(group, func(name string) bool { return slices.Contains(description.RequiredParams, name) }); shared >= 0 {
			t.Errorf("%s required_params %v names %q, which only an alternative requires", description.ID, description.RequiredParams, group[shared])
		}
	}
	if len(description.RequiredParamsAnyOf) > 0 {
		want = append(want, description.RequiredParamsAnyOf[0]...)
	}
	if got := slices.Sorted(maps.Keys(params)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("%s example fills %v, want the required params and the first alternative %v", description.ID, got, want)
	}
}

// TestFind_AnAnyOfActionPublishesItsAlternativesApart verifies the case the
// defect was found on, through the three published answers: find, describe
// and the internal search. security_attribute.update requires attribute_id
// and at least one of name, description or color, so required_params names
// attribute_id alone, the three alternatives are their own groups, and the
// example sends one of them with a value its schema accepts.
func TestFind_AnAnyOfActionPublishesItsAlternativesApart(t *testing.T) {
	const id = "security_attribute.update"
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))
	wantRequired := []string{"attribute_id"}
	wantGroups := [][]string{{"name"}, {"description"}, {"color"}}

	_, found, err := registry.Find(t.Context(), nil, FindInput{Query: "update security attribute color", Limit: 50})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	index := slices.IndexFunc(found.Results, func(result FindResult) bool { return result.ID == id })
	if index < 0 {
		t.Fatalf("Find() did not return %s: %+v", id, found.Results)
	}
	result := found.Results[index]
	if !slices.Equal(result.RequiredParams, wantRequired) || !reflect.DeepEqual(result.RequiredParamsAnyOf, wantGroups) {
		t.Errorf("find %s required_params = %v, required_params_any_of = %v, want %v and %v", id, result.RequiredParams, result.RequiredParamsAnyOf, wantRequired, wantGroups)
	}
	wantParams := map[string]any{"attribute_id": 123, "name": "value"}
	if got := result.Example.Arguments["params"]; !reflect.DeepEqual(got, wantParams) {
		t.Errorf("find %s example params = %#v, want %#v", id, got, wantParams)
	}

	_, described, err := registry.Describe(t.Context(), nil, DescribeInput{Action: id})
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if got := described.Actions[0]; !slices.Equal(got.RequiredParams, wantRequired) || !reflect.DeepEqual(got.RequiredParamsAnyOf, wantGroups) {
		t.Errorf("describe %s required_params = %v, required_params_any_of = %v, want %v and %v", id, got.RequiredParams, got.RequiredParamsAnyOf, wantRequired, wantGroups)
	}

	_, searched, err := registry.Search(t.Context(), nil, SearchInput{Query: "update security attribute color", Limit: 50})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	index = slices.IndexFunc(searched.Results, func(result SearchResult) bool { return result.ID == id })
	if index < 0 {
		t.Fatalf("Search() did not return %s: %+v", id, searched.Results)
	}
	if got := searched.Results[index]; !slices.Equal(got.RequiredParams, wantRequired) || !reflect.DeepEqual(got.RequiredParamsAnyOf, wantGroups) {
		t.Errorf("search %s required_params = %v, required_params_any_of = %v, want %v and %v", id, got.RequiredParams, got.RequiredParamsAnyOf, wantRequired, wantGroups)
	}
}

// TestFind_PublishedAlternativesAreCopies verifies that a result's
// alternative groups are its own, on the two paths that publish them: the
// description find and describe answer with, and the internal search. The
// entry they come from is shared by every server of the process, so a caller
// that edits a group of one answer must not change the next answer anyone
// gets.
func TestFind_PublishedAlternativesAreCopies(t *testing.T) {
	const id = "security_attribute.update"
	wantGroups := [][]string{{"name"}, {"description"}, {"color"}}
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))

	t.Run("describe", func(t *testing.T) {
		entry, ok := registry.resolveAction(id)
		if !ok {
			t.Fatalf("resolveAction(%s) found nothing", id)
		}
		first := registry.describeEntry(entry)
		first.RequiredParamsAnyOf[0][0] = "edited"
		first.RequiredParamsAnyOf = append(first.RequiredParamsAnyOf, []string{"added"})
		if second := registry.describeEntry(entry); !reflect.DeepEqual(second.RequiredParamsAnyOf, wantGroups) {
			t.Errorf("a second answer carries %v after the first was edited, want the groups unchanged", second.RequiredParamsAnyOf)
		}
	})

	t.Run("search", func(t *testing.T) {
		search := func() SearchResult {
			t.Helper()
			_, searched, err := registry.Search(t.Context(), nil, SearchInput{Query: "update security attribute color", Limit: 50})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			index := slices.IndexFunc(searched.Results, func(result SearchResult) bool { return result.ID == id })
			if index < 0 {
				t.Fatalf("Search() did not return %s: %+v", id, searched.Results)
			}
			return searched.Results[index]
		}
		first := search()
		first.RequiredParamsAnyOf[0][0] = "edited"
		if second := search(); !reflect.DeepEqual(second.RequiredParamsAnyOf, wantGroups) {
			t.Errorf("a second search carries %v after the first was edited, want the groups unchanged", second.RequiredParamsAnyOf)
		}
	})
}

// TestFind_ExamplesUseValuesGitLabAccepts verifies the example values whose
// schema accepts more than GitLab does. A label's color is a free string in
// its schema and GitLab takes a hex color or a CSS color name, so "value"
// passed the schema and failed at GitLab, which is issue 1175's own case on
// another action; and an access level is an integer whose schema lists the
// levels in its description, where 1 is none of them. Developer, 30, is a
// level every one of these routes accepts, the member roles' 10 to 40
// included, and an enum that holds it is answered with it rather than with
// its first value, which for an LDAP group link is Minimal access.
//
// The rest are the strings of issue 1272, whose form GitLab checks and which
// the example filled with "value": an email address, a redirect or site URL,
// a cron expression, a duration, a Pages domain, a digest, a commit SHA where
// a full one is required and where a ref would do, a key fingerprint, a
// container path pattern starting with the project's path, the access
// levels and the check status GitLab lists, a changelog's semantic version,
// and the commit SHAs of a merge request's context commits. The
// catalog-wide rule over the same strings is
// TestFind_FormatBearingParams_DeclareTheirFormatAndExampleIt; these name the
// value each action is offered.
func TestFind_ExamplesUseValuesGitLabAccepts(t *testing.T) {
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))
	tests := []struct {
		action string
		param  string
		want   any
	}{
		{action: "project.label_create", param: "color", want: exampleHexColor},
		{action: "group.group_label_create", param: "color", want: exampleHexColor},
		{action: "access.invite_group", param: "access_level", want: 30},
		{action: "access.invite_project", param: "access_level", want: 30},
		{action: "group.saml_link_add", param: "access_level", want: 30},
		{action: "group.share_with_group", param: "group_access", want: 30},
		{action: "project.share_with_group", param: "group_access", want: 30},
		{action: "group.ldap_link_add", param: "group_access", want: 30},
		{action: "member_role.create_group", param: "base_access_level", want: 30},
		{action: "member_role.create_instance", param: "base_access_level", want: 30},
		// Issue 1272: a string whose form GitLab checks, each of which the
		// example filled with "value".
		{action: "user.add_email", param: "email", want: exampleEmail},
		{action: "user.create", param: "email", want: exampleEmail},
		{action: "admin.import_bitbucket", param: "bitbucket_email", want: exampleEmail},
		{action: "admin.application_create", param: "redirect_uri", want: exampleURL},
		{action: "admin.bulk_import_start", param: "configuration.url", want: exampleURL},
		{action: "geo.create", param: "url", want: exampleURL},
		{action: "pipeline.schedule_create", param: "cron", want: exampleCron},
		{action: "environment.freeze_create", param: "freeze_start", want: exampleCron},
		{action: "environment.freeze_create", param: "freeze_end", want: exampleCron},
		{action: "issue.spent_time_add", param: "duration", want: exampleDuration},
		{action: "merge_request.time_estimate_set", param: "duration", want: exampleDuration},
		{action: "project.pages_domain_create", param: "domain", want: exampleHostname},
		{action: "attestation.list", param: "subject_digest", want: exampleDigest},
		{action: "external_status_check.set_project_mr_status", param: "sha", want: exampleSHA},
		{action: "external_status_check.set_project_mr_status", param: "status", want: "passed"},
		{action: "repository.update_submodule", param: "commit_sha", want: exampleSHA},
		{action: "repository.commit_get", param: "sha", want: exampleSHA},
		{action: "repository.commit_discussion_list", param: "commit_sha", want: exampleSHA},
		{action: "environment.deployment_create", param: "sha", want: exampleSHA},
		{action: "user.key_get_by_fingerprint", param: "fingerprint", want: exampleFingerprint},
		{action: "package.registry_rule_create", param: "repository_path_pattern", want: exampleProjectPath + "/*"},
		{action: "package.registry_rule_create", param: "minimum_access_level_for_push", want: "maintainer"},
		{action: "package.registry_tag_rule_create", param: "minimum_access_level_for_push", want: "maintainer"},
		{action: "package.registry_tag_rule_create", param: "minimum_access_level_for_delete", want: "maintainer"},
		{action: "repository.changelog_add", param: "version", want: exampleVersion},
		{action: "repository.changelog_generate", param: "version", want: exampleVersion},
		{action: "merge_request.context_commits_create", param: "commits", want: []any{exampleSHA}},
		{action: "merge_request.context_commits_delete", param: "commits", want: []any{exampleSHA}},
	}
	for _, tc := range tests {
		t.Run(tc.action+" "+tc.param, func(t *testing.T) {
			entry, ok := registry.resolveAction(tc.action)
			if !ok {
				t.Fatalf("resolveAction(%s) found nothing", tc.action)
			}
			got := registry.describeEntry(entry).Example.Arguments["params"]
			for name := range strings.SplitSeq(tc.param, ".") {
				object, _ := got.(map[string]any)
				got = object[name]
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("%s example %s = %#v, want %#v", tc.action, tc.param, got, tc.want)
			}
		})
	}
}

// TestFind_LDAPLinkExamples_NameTheCommonNameAlone verifies the call find and
// describe offer for the two LDAP link actions GitLab declares with
// exactly_one_of :cn, :filter. Their schemas published both as optional and
// stated no choice, so the example sent neither and GitLab refused the call
// it built (issue 1270). Now the two are the alternatives of a oneOf, which
// find publishes as exclusive groups, and the example fills the first, the
// common name, beside the parameters every call carries, and never the
// filter as well, which GitLab refuses just the same.
func TestFind_LDAPLinkExamples_NameTheCommonNameAlone(t *testing.T) {
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))
	wantGroups := [][]string{{"cn"}, {"filter"}}
	cases := []struct {
		action      string
		wantParams  []string
		wantConfirm bool
	}{
		{action: "group.ldap_link_add", wantParams: []string{"cn", "group_access", "group_id", "provider"}},
		{action: "group.ldap_link_delete", wantParams: []string{"cn", "group_id", "provider"}, wantConfirm: true},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			entry, ok := registry.resolveAction(tc.action)
			if !ok {
				t.Fatalf("resolveAction(%s) found nothing", tc.action)
			}
			described := registry.describeEntry(entry)
			if !reflect.DeepEqual(described.RequiredParamsAnyOf, wantGroups) || !described.ExclusiveGroups {
				t.Errorf("%s required_params_any_of = %v, exclusive %t, want %v, exclusive", tc.action, described.RequiredParamsAnyOf, described.ExclusiveGroups, wantGroups)
			}
			params, _ := described.Example.Arguments["params"].(map[string]any)
			if got := slices.Sorted(maps.Keys(params)); !slices.Equal(got, tc.wantParams) {
				t.Errorf("%s example fills %v, want %v", tc.action, got, tc.wantParams)
			}
			if confirm, _ := described.Example.Arguments["confirm"].(bool); confirm != tc.wantConfirm {
				t.Errorf("%s example confirm = %t, want %t", tc.action, confirm, tc.wantConfirm)
			}
		})
	}
}

// TestFind_ProtectedEnvironmentExamples_DeployAsDevelopers verifies the call
// find and describe offer for the project and the group protect. Each deploy
// access level names exactly one of a role, a user or a group, and the item
// schema says so with a oneOf, which the example used to read nothing of: it
// filled an item's required list alone, which is empty, so it sent
// deploy_access_levels [{}] and GitLab refused the call it built (issue
// 1271). Now it fills the first alternative of the item, the role, with the
// level an example grants everywhere, Developer.
func TestFind_ProtectedEnvironmentExamples_DeployAsDevelopers(t *testing.T) {
	registry := NewRegistryFromCatalog(mustCachedCatalog(t, true))
	for _, action := range []string{"environment.protected_protect", "group.protected_env_protect"} {
		t.Run(action, func(t *testing.T) {
			entry, ok := registry.resolveAction(action)
			if !ok {
				t.Fatalf("resolveAction(%s) found nothing", action)
			}
			params, _ := registry.describeEntry(entry).Example.Arguments["params"].(map[string]any)
			if got := fmt.Sprint(params["deploy_access_levels"]); got != "[map[access_level:30]]" {
				t.Errorf("%s example deploy_access_levels = %s, want [map[access_level:30]]", action, got)
			}
		})
	}
}

// TestExampleFor_FillsTheRequiredParamsAndTheFirstAlternative verifies the
// shape of the example call: every parameter the schema's root requires, the
// first alternative group and none of the others, and top-level confirm only
// on a destructive action.
func TestExampleFor_FillsTheRequiredParamsAndTheFirstAlternative(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"attribute_id"},
		"anyOf": []any{
			map[string]any{"required": []any{"color"}},
			map[string]any{"required": []any{"name"}},
		},
		"properties": map[string]any{
			"attribute_id": map[string]any{"type": "integer"},
			"name":         map[string]any{"type": "string", "minLength": 1},
			"color":        map[string]any{"type": "string", "pattern": "^#[0-9A-Fa-f]{6}$"},
		},
	}

	t.Run("not destructive", func(t *testing.T) {
		got := exampleFor(actionEntry{ID: "security_attribute.update"}, schema)
		want := ActionExample{Tool: executeActionToolName, Arguments: map[string]any{
			"action": "security_attribute.update",
			"params": map[string]any{"attribute_id": 123, "color": exampleHexColor},
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exampleFor() = %#v, want %#v", got, want)
		}
	})

	t.Run("destructive", func(t *testing.T) {
		got := exampleFor(actionEntry{ID: "security_attribute.delete", Destructive: true}, map[string]any{"type": "object"})
		want := ActionExample{Tool: executeActionToolName, Arguments: map[string]any{
			"action":  "security_attribute.delete",
			"params":  map[string]any{},
			"confirm": true,
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exampleFor() = %#v, want %#v", got, want)
		}
	})
}

// TestExampleValue_SatisfiesEachConstraint verifies the value written for one
// parameter, constraint by constraint. Each case is a shape a required
// parameter of the catalog takes, and the value is the one the example
// publishes, so a change here changes what a model is taught to send.
func TestExampleValue_SatisfiesEachConstraint(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		schema map[string]any
		want   any
	}{
		{name: "a string enum takes its first value", param: "mode", schema: map[string]any{"type": "string", "enum": []any{"ADD", "REMOVE"}}, want: "ADD"},
		{name: "an integer enum takes its first value", param: "group_access", schema: map[string]any{"type": "integer", "enum": []any{0, 5}}, want: 0},
		{name: "an enum holding the placeholder takes it", param: "group_access", schema: map[string]any{"type": "integer", "enum": []any{0, 5, 10, 30, 40}}, want: 30},
		{name: "a decoded enum holding the placeholder keeps its own value", param: "access_level", schema: map[string]any{"type": "integer", "enum": []any{float64(0), float64(30)}}, want: float64(30)},
		{name: "a color with no pattern is still a color", param: "color", schema: map[string]any{"type": "string"}, want: exampleHexColor},
		{name: "an access level is Developer", param: "access_level", schema: map[string]any{"type": "integer"}, want: 30},
		{name: "a group's access level is Developer", param: "group_access", schema: map[string]any{"type": "integer"}, want: 30},
		{name: "a role's base access level is Developer", param: "base_access_level", schema: map[string]any{"type": "integer"}, want: 30},
		{name: "an enum written in Go takes its first value", param: "mode", schema: map[string]any{"type": "string", "enum": []string{"ADD", "REMOVE"}}, want: "ADD"},
		{name: "an empty enum constrains nothing", param: "title", schema: map[string]any{"type": "string", "enum": []any{}}, want: "value"},
		{name: "an empty enum written in Go constrains nothing", param: "title", schema: map[string]any{"type": "string", "enum": []string{}}, want: "value"},
		{name: "a hex pattern is answered with a color", param: "color", schema: map[string]any{"type": "string", "pattern": "^#[0-9A-Fa-f]{6}$"}, want: exampleHexColor},
		{name: "a pattern the placeholder satisfies keeps it", param: "url", schema: map[string]any{"type": "string", "pattern": "^(https?|ftp)://"}, want: exampleURL},
		{name: "a pattern only the URL satisfies takes the URL", param: "target", schema: map[string]any{"type": "string", "pattern": "^https://"}, want: exampleURL},
		{name: "a pattern nothing satisfies keeps the placeholder", param: "title", schema: map[string]any{"type": "string", "pattern": "^z+$"}, want: "value"},
		{name: "a pattern that does not compile keeps the placeholder", param: "title", schema: map[string]any{"type": "string", "pattern": "("}, want: "value"},
		{name: "a date format takes a date", param: "expires_at", schema: map[string]any{"type": "string", "format": "date"}, want: exampleDate},
		{name: "a date-time format takes a timestamp", param: "expires_at", schema: map[string]any{"type": "string", "format": "date-time"}, want: exampleDateTime},
		{name: "a uri format takes a URL whatever the name", param: "bitbucket_server_url", schema: map[string]any{"type": "string", "format": "uri"}, want: exampleURL},
		{name: "an unknown format keeps the placeholder", param: "title", schema: map[string]any{"type": "string", "format": "markdown"}, want: "value"},
		{name: "an email format takes an address", param: "email", schema: map[string]any{"type": "string", "format": "email"}, want: exampleEmail},
		{name: "an email format takes an address whatever the name", param: "bitbucket_email", schema: map[string]any{"type": "string", "format": "email"}, want: exampleEmail},
		{name: "an email with no format is an address by its name", param: "email", schema: map[string]any{"type": "string"}, want: exampleEmail},
		{name: "a redirect URI with no format is a URL by its name", param: "redirect_uri", schema: map[string]any{"type": "string"}, want: exampleURL},
		{name: "a hostname format takes a host name", param: "domain", schema: map[string]any{"type": "string", "format": "hostname"}, want: exampleHostname},
		{name: "a SHA is a full hex SHA", param: "sha", schema: map[string]any{"type": "string"}, want: exampleSHA},
		{name: "a name ending in a SHA is a full hex SHA", param: "commit_sha", schema: map[string]any{"type": "string"}, want: exampleSHA},
		{name: "a full SHA pattern keeps the SHA", param: "sha", schema: map[string]any{"type": "string", "pattern": toolutil.FullCommitSHAPattern}, want: exampleSHA},
		{name: "a duration is an hour", param: "duration", schema: map[string]any{"type": "string"}, want: exampleDuration},
		{name: "a digest pattern takes a digest", param: "subject_digest", schema: map[string]any{"type": "string", "pattern": `^(sha256:)?[0-9A-Fa-f]{64}$`}, want: exampleDigest},
		{name: "a cron pattern takes a cron expression", param: "freeze_start", schema: map[string]any{"type": "string", "pattern": toolutil.CronPattern}, want: exampleCron},
		{
			name:   "a semantic version pattern takes a version",
			param:  "version",
			schema: map[string]any{"type": "string", "pattern": `(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)`},
			want:   exampleVersion,
		},
		{
			name:   "a fingerprint pattern takes the SHA256 form",
			param:  "fingerprint",
			schema: map[string]any{"type": "string", "pattern": `^((sha256|SHA256):[A-Za-z0-9+/]{43}=?|(MD5:)?[0-9A-Fa-f]{2}(:[0-9A-Fa-f]{2}){15})$`},
			want:   exampleFingerprint,
		},
		{
			name:   "a repository path pattern starts with the project's path",
			param:  "repository_path_pattern",
			schema: map[string]any{"type": "string", "pattern": `^[a-z0-9]+(([._/]|__|-*)[a-z0-9*])*$`},
			want:   exampleProjectPath + "/*",
		},
		{name: "a string named like an identifier takes its digits", param: "user_id", schema: map[string]any{"type": "string"}, want: "123"},
		{name: "an integer named like an identifier", param: "attribute_id", schema: map[string]any{"type": "integer"}, want: 123},
		{name: "an integer whose placeholder is a path", param: "namespace_id", schema: map[string]any{"type": "integer"}, want: 123},
		{name: "an integer with no identifier name", param: "serial", schema: map[string]any{"type": "integer", "minimum": 0}, want: 1},
		{name: "a number", param: "weight", schema: map[string]any{"type": "number"}, want: 1},
		{name: "a project path where a string is allowed", param: "project_id", schema: map[string]any{"type": []any{"string", "integer"}}, want: "group/project"},
		{name: "an internal ID where an integer is allowed", param: "merge_request_iid", schema: map[string]any{"type": []any{"string", "integer"}}, want: 123},
		{name: "a boolean", param: "active", schema: map[string]any{"type": "boolean"}, want: true},
		{name: "a nullable array of identifiers", param: "user_achievement_ids", schema: map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "integer"}}, want: []any{123}},
		{name: "an array of strings", param: "scopes", schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, want: []any{"value"}},
		{name: "an array of commits holds a full hex SHA", param: "commits", schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, want: []any{exampleSHA}},
		{
			name:  "an array of objects fills each item's required fields",
			param: "files",
			schema: map[string]any{"type": []any{"null", "array"}, "minItems": 1, "items": map[string]any{
				"type":     "object",
				"required": []any{"file_path", "content"},
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string"},
					"content":   map[string]any{"type": "string"},
					"encoding":  map[string]any{"type": "string"},
				},
			}},
			want: []any{map[string]any{"content": "value", "file_path": "path/to/file"}},
		},
		{
			name:  "an array of objects fills the first alternative of each item",
			param: "deploy_access_levels",
			schema: map[string]any{"type": []any{"null", "array"}, "minItems": 1, "items": map[string]any{
				"type": "object",
				"oneOf": []any{
					map[string]any{"required": []string{"access_level"}},
					map[string]any{"required": []string{"user_id"}},
				},
				"properties": map[string]any{
					"access_level": map[string]any{"type": []any{"null", "integer"}, "enum": []any{20, 30, 40, 60, nil}},
					"user_id":      map[string]any{"type": []any{"null", "integer"}},
				},
			}},
			want: []any{map[string]any{"access_level": 30}},
		},
		{
			name:  "an object fills its required fields and its first alternative",
			param: "rule",
			schema: map[string]any{
				"type":     "object",
				"required": []any{"name"},
				"anyOf":    []any{map[string]any{"required": []any{"group_id"}}, map[string]any{"required": []any{"user_id"}}},
				"properties": map[string]any{
					"name":     map[string]any{"type": "string"},
					"group_id": map[string]any{"type": "integer"},
					"user_id":  map[string]any{"type": "integer"},
				},
			},
			want: map[string]any{"group_id": 123, "name": "value"},
		},
		{
			name:  "an object fills its required fields only",
			param: "configuration",
			schema: map[string]any{"type": "object", "required": []any{"url", "access_token"}, "properties": map[string]any{
				"url":          map[string]any{"type": "string"},
				"access_token": map[string]any{"type": "string"},
				"note":         map[string]any{"type": "string"},
			}},
			want: map[string]any{"access_token": "value", "url": exampleURL},
		},
		{name: "an open object is empty", param: "settings", schema: map[string]any{"type": "object", "additionalProperties": true}, want: map[string]any{}},
		{name: "an untyped oneOf takes its first branch", param: "value", schema: map[string]any{"oneOf": []any{"ignored", map[string]any{"type": "boolean"}, map[string]any{"type": "string"}}}, want: true},
		{name: "an untyped anyOf takes its first branch", param: "value", schema: map[string]any{"anyOf": []any{map[string]any{"type": "integer"}}}, want: 1},
		{name: "a type that is only null keeps the placeholder", param: "title", schema: map[string]any{"type": "null"}, want: "value"},
		{name: "an unknown type keeps the placeholder", param: "title", schema: map[string]any{"type": "date"}, want: "value"},
		{name: "a type list naming nothing keeps the placeholder", param: "title", schema: map[string]any{"type": []any{"null", 42}}, want: "value"},
		{name: "no schema keeps the placeholder", param: "issue_iid", schema: nil, want: 123},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exampleValue(tc.param, tc.schema); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("exampleValue(%q, %v) = %#v, want %#v", tc.param, tc.schema, got, tc.want)
			}
		})
	}
}

// validateExampleParams validates params against a copy of schema, so
// resolving it cannot touch the schema every server shares, and then checks
// the string formats the validator does not.
func validateExampleParams(schema, params map[string]any) error {
	data, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	var copied jsonschema.Schema
	if err = json.Unmarshal(data, &copied); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	resolved, err := copied.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema: %w", err)
	}
	if err = resolved.Validate(params); err != nil {
		return err
	}
	return checkExampleFormats(schema, params, "params")
}

// checkExampleFormats walks a value beside the schema that describes it and
// refuses a string whose declared format it does not satisfy.
func checkExampleFormats(schema map[string]any, value any, path string) error {
	switch typed := value.(type) {
	case string:
		format, _ := schema["format"].(string)
		return checkExampleFormat(format, typed, path)
	case map[string]any:
		properties, _ := schema["properties"].(map[string]any)
		for name, child := range typed {
			childSchema, _ := properties[name].(map[string]any)
			if err := checkExampleFormats(childSchema, child, path+"."+name); err != nil {
				return err
			}
		}
	case []any:
		items, _ := schema["items"].(map[string]any)
		for i, child := range typed {
			if err := checkExampleFormats(items, child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkExampleFormat checks one string against the formats the catalog
// declares.
func checkExampleFormat(format, value, path string) error {
	var err error
	switch format {
	case "date":
		_, err = time.Parse(time.DateOnly, value)
	case "date-time":
		_, err = time.Parse(time.RFC3339, value)
	case "uri":
		var parsed *url.URL
		parsed, err = url.Parse(value)
		if err == nil && (parsed.Scheme == "" || parsed.Host == "") {
			err = fmt.Errorf("%q is not an absolute URI", value)
		}
	case "email":
		var address *mail.Address
		address, err = mail.ParseAddress(value)
		if err == nil && address.Address != value {
			err = fmt.Errorf("%q is not a bare address", value)
		}
	case "hostname":
		err = checkHostname(value)
	}
	if err != nil {
		return fmt.Errorf("%s does not satisfy format %q: %w", path, format, err)
	}
	return nil
}

// hostnameLabel is one label of a host name as RFC 1123 writes it, which is
// what JSON Schema's hostname format names.
var hostnameLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// checkHostname refuses a value that is no host name: longer than 253
// characters, or holding a label RFC 1123 does not allow.
func checkHostname(value string) error {
	if len(value) > 253 {
		return fmt.Errorf("%q is longer than a host name may be", value)
	}
	for label := range strings.SplitSeq(value, ".") {
		if !hostnameLabel.MatchString(label) {
			return fmt.Errorf("%q holds the label %q, which no host name does", value, label)
		}
	}
	return nil
}
