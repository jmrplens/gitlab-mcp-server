package paths

import (
	"reflect"
	"testing"
)

// TestReadSDKOptions_TheStructAMethodIsGiven_IsReadWithItsRoutes verifies the
// first link of the always-sent join: which option struct reaches which
// endpoint, and which of its keys encoding/json writes whatever the caller
// filled in.
//
// Three of the cases are the ones that would silently unmake the rule. A field
// tagged omitzero is omitted exactly as one tagged omitempty is, and reading
// only omitempty reported four of client-go's slices as values a caller cannot
// decline to send. An embedded option struct's fields are promoted, so they are
// the enclosing struct's own as far as a body is concerned. And the variadic
// transport tail ends in Options too, so a rule that went by the name alone
// would attribute every route to it.
func TestReadSDKOptions_TheStructAMethodIsGiven_IsReadWithItsRoutes(t *testing.T) {
	dir := sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var (
	routeProjectsIDRules   = route("projects/%s/packages/protection/rules")
	routeProjectsIDRulesID = route("projects/%s/packages/protection/rules/%d")
)
`,
		"options.go": `package gitlab

type UpdateRulesOptions struct {
	ListOptions
	PackageNamePattern *string             ` + "`" + `url:"package_name_pattern" json:"package_name_pattern"` + "`" + `
	MinimumAccessLevel Nullable[Access]    ` + "`" + `json:"minimum_access_level,omitempty"` + "`" + `
	AllowedToPush      []*GroupAccessLevel ` + "`" + `json:"allowed_to_push,omitzero"` + "`" + `
	Position           *PositionOptions    ` + "`" + `json:"position,omitempty"` + "`" + `
	Untagged           *string
	Hidden             *string             ` + "`" + `json:"-"` + "`" + `
	unexported         *string             ` + "`" + `json:"unexported"` + "`" + `
}

type PositionOptions struct {
	PositionType *string ` + "`" + `json:"position_type"` + "`" + `
}

type ListOptions struct {
	Page *int ` + "`" + `json:"page,omitempty"` + "`" + `
}

type NotAStructOptions = UpdateRulesOptions
`,
		"service.go": `package gitlab

import "net/http"

func (s *ProtectedPackagesService) UpdateRules(pid any, id int64, opt *UpdateRulesOptions, options ...RequestOptionFunc) (*Rule, *Response, error) {
	return do[*Rule](s.client,
		withMethod(http.MethodPatch),
		withPath(routeProjectsIDRulesID, ProjectID{pid}, id),
		withAPIOpts(opt),
	)
}

func (s *ProtectedPackagesService) ListRules(pid any, options ...RequestOptionFunc) ([]*Rule, *Response, error) {
	return do[[]*Rule](s.client, withPath(routeProjectsIDRules, ProjectID{pid}))
}
`,
	})

	options := readSDKOptions(dir)

	rules, known := options.Types["UpdateRulesOptions"]
	if !known {
		t.Fatalf("read %v, want UpdateRulesOptions among them", mapKeys(options.Types))
	}
	if !reflect.DeepEqual(rules.Embedded, []string{"ListOptions"}) {
		t.Errorf("embedded = %v, want [ListOptions]", rules.Embedded)
	}
	want := []sdkOptionField{
		{GoName: "PackageNamePattern", Name: "package_name_pattern", Always: true},
		{GoName: "MinimumAccessLevel", Name: "minimum_access_level"},
		{GoName: "AllowedToPush", Name: "allowed_to_push", Many: true},
		{GoName: "Position", Name: "position", Nested: "PositionOptions"},
		{GoName: "Untagged", Name: "Untagged", Always: true},
	}
	if !reflect.DeepEqual(rules.Fields, want) {
		t.Errorf("fields = %+v, want %+v", rules.Fields, want)
	}
	if routes := options.Routes["UpdateRulesOptions"]; len(routes) != 1 || routes[0].operation() != "PATCH /projects/:/packages/protection/rules/:" {
		t.Errorf("routes = %+v, want the one PATCH the method sends", routes)
	}
	if routes, claimed := options.Routes[requestOptionType]; claimed {
		t.Errorf("the transport's variadic tail claimed %+v, want no route at all", routes)
	}
}

// TestWalkOptionParams_NestedStructs_AreNamedTheWayGrapeDeclaresThem verifies
// the naming the comparison joins on. GitLab declares a nested param with its
// parent's key in front of it, so PositionOptions.PositionType is
// position[position_type] and nothing under a plain name would ever match.
//
// The self-reference is the case that decides whether this terminates at all,
// and a bound that only counted depth would still walk six pointless levels
// first.
func TestWalkOptionParams_NestedStructs_AreNamedTheWayGrapeDeclaresThem(t *testing.T) {
	types := map[string]sdkOptionType{
		"CreateOptions": {
			Embedded: []string{"SharedOptions"},
			Fields: []sdkOptionField{
				{GoName: "Note", Name: "note", Always: true},
				{GoName: "Position", Name: "position", Nested: "PositionOptions"},
				{GoName: "Actions", Name: "actions", Nested: "ActionOptions", Many: true},
			},
		},
		"SharedOptions":   {Fields: []sdkOptionField{{GoName: "Sudo", Name: "sudo"}}},
		"PositionOptions": {Fields: []sdkOptionField{{GoName: "PositionType", Name: "position_type", Always: true}}},
		"ActionOptions": {Fields: []sdkOptionField{
			{GoName: "Action", Name: "action", Always: true},
			{GoName: "Self", Name: "self", Nested: "CreateOptions"},
		}},
	}

	var params []string
	walkOptionParams(types, "CreateOptions", "", nil, 0, func(found optionParam) {
		params = append(params, found.Param)
	})

	want := []string{"sudo", "note", "position", "position[position_type]", "actions", "actions[][action]", "actions[][self]"}
	if !reflect.DeepEqual(params, want) {
		t.Errorf("params = %v, want %v", params, want)
	}
}

// TestWalkOptionParams_AnUnknownType_VisitsNothing verifies that a field naming
// a struct the parse never read contributes no param, since inventing one would
// produce a comparison against a name GitLab was never asked about.
func TestWalkOptionParams_AnUnknownType_VisitsNothing(t *testing.T) {
	visited := 0
	walkOptionParams(map[string]sdkOptionType{}, "MissingOptions", "", nil, 0, func(optionParam) { visited++ })

	if visited != 0 {
		t.Errorf("visited %d param(s), want none", visited)
	}
}

// TestReadSDKOptions_AnUnreadableDirectory_ReadsNothing verifies the silence
// this shares with every other reader of the module cache: a directory it
// cannot parse leaves the check with nothing to say rather than failing the
// scope over somebody else's source tree.
func TestReadSDKOptions_AnUnreadableDirectory_ReadsNothing(t *testing.T) {
	for _, dir := range []string{"", t.TempDir()} {
		if options := readSDKOptions(dir); len(options.Types) != 0 || len(options.Routes) != 0 {
			t.Errorf("readSDKOptions(%q) = %+v, want nothing", dir, options)
		}
	}
}

// mapKeys names a map's keys for a failure message.
func mapKeys[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}
