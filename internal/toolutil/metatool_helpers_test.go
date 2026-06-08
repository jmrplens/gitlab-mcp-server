// metatool_helpers_test.go covers internal helpers used by the metatool
// dispatch infrastructure that are not exercised end-to-end through the
// public API surface. These direct unit tests document the contract of
// each helper and protect the subtle branches (canonical-alias detection,
// path decoding, structured-value coercion) from regressions.
package toolutil

import (
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// aliasStructInput is a small struct with JSON-tagged fields used to drive
// reflect-based helpers that introspect a target type's fields.
type aliasStructInput struct {
	ProjectID StringOrInt `json:"project_id"`
	MRIID     int64       `json:"merge_request_iid"`
	Name      string      `json:"name"`
	Paused    bool        `json:"paused"`
}

// accessLevelEntryInput mirrors the GitLab protected-branch access level
// shape so structured-value coercion has a struct target to inspect.
type accessLevelEntryInput struct {
	AccessLevel       *int   `json:"access_level"`
	RequiredApprovals *int64 `json:"required_approvals"`
}

// acceptsTrue is a convenience acceptor predicate for tests that drive
// alias helpers directly.
func acceptsTrue(_ string) bool { return true }

func acceptsFalse(_ string) bool { return false }

func fieldSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

// aliasClone returns a "clone" callback matching the production
// normalizeParamAliasesWithFields pattern. For test purposes we return
// the same map on every call so that mutations made through the clone
// callback are visible to the caller; this mirrors what the production
// helper does after the first clone (it reuses the same backing map).
// Tests using this helper should treat the supplied params as
// in-out: the same map will be mutated in place.
func aliasClone(initial map[string]any) (clone func() map[string]any, _ *map[string]any) {
	clone = func() map[string]any { return initial }
	return clone, nil
}

// TestStructHasJSONField verifies structHasJSONField returns true for
// fields with JSON tags, false for unexported or absent fields, and
// handles nil, pointer, and non-struct target types defensively.
func TestStructHasJSONField(t *testing.T) {
	target := reflect.TypeFor[aliasStructInput]()

	if !structHasJSONField(target, "project_id") {
		t.Error("structHasJSONField(project_id) = false, want true")
	}
	if !structHasJSONField(target, "merge_request_iid") {
		t.Error("structHasJSONField(merge_request_iid) = false, want true")
	}
	if structHasJSONField(target, "missing") {
		t.Error("structHasJSONField(missing) = true, want false")
	}
	if structHasJSONField(target, "MRIID") {
		t.Error("structHasJSONField(MRIID, no json tag) = true, want false")
	}
	if structHasJSONField(reflect.TypeFor[int](), "x") {
		t.Error("structHasJSONField(non-struct) = true, want false")
	}
	if structHasJSONField(nil, "x") {
		t.Error("structHasJSONField(nil) = true, want false")
	}
}

// TestValidGitLabRoleAccessLevelDirect verifies the access-level whitelist
// accepts the canonical 0/10/20/30/40/50/60 values and rejects every other
// integer, including negative and out-of-range values.
func TestValidGitLabRoleAccessLevelDirect(t *testing.T) {
	cases := []struct {
		value  int
		wantOK bool
	}{
		{0, true},
		{10, true},
		{20, true},
		{30, true},
		{40, true},
		{50, true},
		{60, true},
		{1, false},
		{5, false},
		{15, false},
		{25, false},
		{70, false},
		{100, false},
		{-1, false},
		{-10, false},
	}
	for _, tc := range cases {
		got, ok := validGitLabRoleAccessLevel(tc.value)
		if ok != tc.wantOK {
			t.Errorf("validGitLabRoleAccessLevel(%d) ok = %v, want %v", tc.value, ok, tc.wantOK)
		}
		if ok && got != tc.value {
			t.Errorf("validGitLabRoleAccessLevel(%d) = %d, want %d", tc.value, got, tc.value)
		}
	}
}

// TestValidGitLabRoleAccessLevelInt64Direct verifies the int64 variant
// rejects out-of-int-range and negative values before narrowing.
func TestValidGitLabRoleAccessLevelInt64Direct(t *testing.T) {
	for _, value := range []int64{0, 10, 20, 30, 40, 50, 60} {
		if got, ok := validGitLabRoleAccessLevelInt64(value); !ok || got != int(value) {
			t.Errorf("validGitLabRoleAccessLevelInt64(%d) = %d/%v, want %d/true", value, got, ok, value)
		}
	}
	for _, value := range []int64{-1, 5, 70, 1 << 40, 1 << 60} {
		if _, ok := validGitLabRoleAccessLevelInt64(value); ok {
			t.Errorf("validGitLabRoleAccessLevelInt64(%d) ok = true, want false", value)
		}
	}
}

// TestValidGitLabRoleAccessLevelFloat64Direct verifies the float64 variant
// only accepts whole-number values in the canonical set.
func TestValidGitLabRoleAccessLevelFloat64Direct(t *testing.T) {
	for _, value := range []float64{0, 10, 20, 30, 40, 50, 60} {
		if got, ok := validGitLabRoleAccessLevelFloat64(value); !ok || got != int(value) {
			t.Errorf("validGitLabRoleAccessLevelFloat64(%v) = %d/%v, want %d/true", value, got, ok, int(value))
		}
	}
	for _, value := range []float64{1.5, 5, 70, -10} {
		if _, ok := validGitLabRoleAccessLevelFloat64(value); ok {
			t.Errorf("validGitLabRoleAccessLevelFloat64(%v) ok = true, want false", value)
		}
	}
}

// TestNumericRoleAccessLevel verifies the type-switch dispatch accepts
// int, int64, and float64 numeric types and rejects everything else.
// Note: this helper does not validate the canonical access-level
// whitelist — it only narrows the type. Validation happens in the
// subsequent validGitLabRoleAccessLevel{Int64,Float64} call sites.
func TestNumericRoleAccessLevel(t *testing.T) {
	if got, ok := numericRoleAccessLevel(int(30)); !ok || got != 30 {
		t.Errorf("numericRoleAccessLevel(int 30) = %d/%v, want 30/true", got, ok)
	}
	if got, ok := numericRoleAccessLevel(int64(40)); !ok || got != 40 {
		t.Errorf("numericRoleAccessLevel(int64 40) = %d/%v, want 40/true", got, ok)
	}
	if got, ok := numericRoleAccessLevel(float64(50)); !ok || got != 50 {
		t.Errorf("numericRoleAccessLevel(float64 50) = %d/%v, want 50/true", got, ok)
	}
	// int64 and float64 variants enforce the canonical access-level
	// whitelist, so out-of-range values are rejected at this layer.
	if _, ok := numericRoleAccessLevel(int64(70)); ok {
		t.Error("numericRoleAccessLevel(int64 70) ok = true, want false")
	}
	if _, ok := numericRoleAccessLevel(float64(70)); ok {
		t.Error("numericRoleAccessLevel(float64 70) ok = true, want false")
	}
	for _, value := range []any{"30", true, nil} {
		if _, ok := numericRoleAccessLevel(value); ok {
			t.Errorf("numericRoleAccessLevel(%T %v) ok = true, want false", value, value)
		}
	}
}

// TestExplainIIDParamAliasDirect verifies the explanation helper
// detects the iid→merge_request_iid alias only when the canonical
// field is present in the target schema, the alias is provided, and
// there is exactly one candidate canonical field.
func TestExplainIIDParamAliasDirect(t *testing.T) {
	// accepts("iid") must be false for the helper to consider it an alias.
	acceptsNoIID := func(name string) bool { return name != "iid" }
	fields := fieldSet("merge_request_iid", "title")
	params := map[string]any{"iid": "1"}

	explanation, ok := explainIIDParamAlias(params, fields, acceptsNoIID)
	if !ok {
		t.Fatal("explainIIDParamAlias = false, want true")
	}
	if explanation.Canonical != "merge_request_iid" {
		t.Errorf("Canonical = %q, want merge_request_iid", explanation.Canonical)
	}
	if explanation.Alias != "iid" {
		t.Errorf("Alias = %q, want iid", explanation.Alias)
	}
	if explanation.Source != "schema_common" {
		t.Errorf("Source = %q, want schema_common", explanation.Source)
	}

	// canonical field is absent from fields → no explanation
	if _, hasMatch := explainIIDParamAlias(params, fieldSet("title"), acceptsNoIID); hasMatch {
		t.Error("explainIIDParamAlias(canonical missing) = true, want false")
	}

	// iid param is absent → no explanation
	if _, hasMatch := explainIIDParamAlias(map[string]any{}, fields, acceptsNoIID); hasMatch {
		t.Error("explainIIDParamAlias(no iid) = true, want false")
	}

	// multiple _iid fields → no explanation (ambiguous)
	if _, hasMatch := explainIIDParamAlias(params, fieldSet("merge_request_iid", "issue_iid"), acceptsNoIID); hasMatch {
		t.Error("explainIIDParamAlias(ambiguous) = true, want false")
	}

	// accepts("iid") is true → no explanation
	if _, hasMatch := explainIIDParamAlias(params, fields, acceptsTrue); hasMatch {
		t.Error("explainIIDParamAlias(iid accepted) = true, want false")
	}
}

// TestExplainEnvironmentIDParamAliasDirect verifies the helper produces
// an explanation only when environment_id is supplied, environment is
// accepted, and environment_id is not accepted.
func TestExplainEnvironmentIDParamAliasDirect(t *testing.T) {
	// accepts("environment_id") must be false; accepts("environment") must be true.
	acceptsEnv := func(name string) bool { return name == "environment" }
	params := map[string]any{"environment_id": "prod"}

	explanation, ok := explainEnvironmentIDParamAlias(params, acceptsEnv)
	if !ok {
		t.Fatal("explainEnvironmentIDParamAlias = false, want true")
	}
	if explanation.Alias != "environment_id" || explanation.Canonical != "environment" {
		t.Errorf("explanation = %+v, want environment_id→environment", explanation)
	}

	if _, hasMatch := explainEnvironmentIDParamAlias(map[string]any{}, acceptsEnv); hasMatch {
		t.Error("explainEnvironmentIDParamAlias(no param) = true, want false")
	}
	if _, hasMatch := explainEnvironmentIDParamAlias(params, acceptsFalse); hasMatch {
		t.Error("explainEnvironmentIDParamAlias(no env accepted) = true, want false")
	}
	// environment_id is accepted → no explanation
	if _, hasMatch := explainEnvironmentIDParamAlias(params, acceptsTrue); hasMatch {
		t.Error("explainEnvironmentIDParamAlias(environment_id accepted) = true, want false")
	}
}

// TestRemoveContextOnlyDiscussionIDDirect verifies the helper drops a
// stray discussion_id when the target schema accepts note_id and the
// caller has already supplied note_id.
func TestRemoveContextOnlyDiscussionIDDirect(t *testing.T) {
	acceptsNoteID := func(name string) bool { return name == "note_id" }
	acceptsAll := func(_ string) bool { return true }

	t.Run("removes when note_id is canonical and present", func(t *testing.T) {
		params := map[string]any{"discussion_id": "abc", "note_id": 7}
		clone, _ := aliasClone(params)
		removeContextOnlyDiscussionID(params, acceptsNoteID, clone)
		if _, ok := params["discussion_id"]; ok {
			t.Errorf("discussion_id not removed: %+v", params)
		}
		if params["note_id"] != 7 {
			t.Errorf("note_id = %v, want 7", params["note_id"])
		}
	})

	t.Run("keeps when schema accepts discussion_id", func(t *testing.T) {
		params := map[string]any{"discussion_id": "abc", "note_id": 7}
		clone, _ := aliasClone(params)
		removeContextOnlyDiscussionID(params, acceptsAll, clone)
		if _, ok := params["discussion_id"]; !ok {
			t.Error("discussion_id removed even though schema accepts it")
		}
	})

	t.Run("keeps when note_id is absent", func(t *testing.T) {
		params := map[string]any{"discussion_id": "abc"}
		clone, _ := aliasClone(params)
		removeContextOnlyDiscussionID(params, acceptsNoteID, clone)
		if _, ok := params["discussion_id"]; !ok {
			t.Error("discussion_id removed even though note_id is absent")
		}
	})
}

// TestNormalizeActiveAliasDirect verifies the active→paused negation
// helper behaves correctly for accepted/present/missing combinations
// and non-bool inputs.
func TestNormalizeActiveAliasDirect(t *testing.T) {
	acceptsPaused := func(name string) bool { return name == "paused" }

	t.Run("active false becomes paused true", func(t *testing.T) {
		params := map[string]any{"active": false}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if v, ok := params["paused"]; !ok || v != true {
			t.Errorf("paused = %v/%v, want true", v, ok)
		}
		if _, ok := params["active"]; ok {
			t.Error("active not removed")
		}
	})

	t.Run("active true becomes paused false", func(t *testing.T) {
		params := map[string]any{"active": true}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if v, ok := params["paused"]; !ok || v != false {
			t.Errorf("paused = %v/%v, want false", v, ok)
		}
	})

	t.Run("no-op when paused not accepted", func(t *testing.T) {
		params := map[string]any{"active": false}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsFalse, clone)
		if _, ok := params["paused"]; ok {
			t.Error("paused added when schema does not accept it")
		}
	})

	t.Run("preserves existing paused value", func(t *testing.T) {
		params := map[string]any{"active": false, "paused": true}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if params["paused"] != true {
			t.Errorf("paused = %v, want true (preserved)", params["paused"])
		}
	})

	t.Run("non-bool active is ignored", func(t *testing.T) {
		params := map[string]any{"active": "yes"}
		clone, _ := aliasClone(params)
		normalizeActiveAlias(params, acceptsPaused, clone)
		if _, ok := params["paused"]; ok {
			t.Error("paused added for non-bool active")
		}
	})
}

// TestNormalizeFilePathAliasDirect verifies the file_path → path+filename
// split helper covers each branch.
func TestNormalizeFilePathAliasDirect(t *testing.T) {
	acceptsBoth := func(name string) bool { return name == "path" || name == "filename" }

	t.Run("splits file_path into path+filename", func(t *testing.T) {
		params := map[string]any{"file_path": "packages/npm/pkg.tgz"}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if params["path"] != "packages/npm" {
			t.Errorf("path = %q, want packages/npm", params["path"])
		}
		if params["filename"] != "pkg.tgz" {
			t.Errorf("filename = %q, want pkg.tgz", params["filename"])
		}
		if _, ok := params["file_path"]; ok {
			t.Error("file_path not removed")
		}
	})

	t.Run("preserves existing path", func(t *testing.T) {
		params := map[string]any{"file_path": "packages/npm/pkg.tgz", "path": "custom"}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if params["path"] != "custom" {
			t.Errorf("path = %q, want custom (preserved)", params["path"])
		}
	})

	t.Run("non-string file_path is ignored", func(t *testing.T) {
		params := map[string]any{"file_path": 42}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if _, ok := params["path"]; ok {
			t.Error("path added for non-string file_path")
		}
	})

	t.Run("empty file_path is ignored", func(t *testing.T) {
		params := map[string]any{"file_path": ""}
		clone, _ := aliasClone(params)
		normalizeFilePathAlias(params, acceptsBoth, clone)
		if _, ok := params["path"]; ok {
			t.Error("path added for empty file_path")
		}
	})
}

// TestNormalizeIIDAliasDirect verifies the iid→canonical-_iid mapping
// for single, missing, ambiguous, and accepted-iid scenarios.
func TestNormalizeIIDAliasDirect(t *testing.T) {
	acceptsMRIID := func(name string) bool { return name == "merge_request_iid" }
	acceptsNoIID := func(name string) bool { return name != "iid" }

	t.Run("iid is renamed to merge_request_iid", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("merge_request_iid"), acceptsMRIID, clone)
		if params["merge_request_iid"] != "5" {
			t.Errorf("merge_request_iid = %v, want 5", params["merge_request_iid"])
		}
		if _, ok := params["iid"]; ok {
			t.Error("iid not removed")
		}
	})

	t.Run("no rename when iid is accepted", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("iid"), acceptsNoIID, clone)
		if _, ok := params["merge_request_iid"]; ok {
			t.Error("iid was renamed even though it is accepted")
		}
	})

	t.Run("ambiguous canonical field → no rename", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("merge_request_iid", "issue_iid"), acceptsNoIID, clone)
		if _, ok := params["merge_request_iid"]; ok {
			t.Error("iid was renamed when canonical was ambiguous")
		}
	})

	t.Run("missing canonical field → no rename", func(t *testing.T) {
		params := map[string]any{"iid": "5"}
		clone, _ := aliasClone(params)
		normalizeIIDAlias(params, fieldSet("title"), acceptsNoIID, clone)
		if _, ok := params["merge_request_iid"]; ok {
			t.Error("iid was renamed when no canonical field exists")
		}
	})
}

// TestNormalizeEnvironmentNameAliasDirect verifies the environment→name
// alias rewrite covers acceptance, name-already-present, and
// environment_scope-accepted branches.
func TestNormalizeEnvironmentNameAliasDirect(t *testing.T) {
	acceptsName := func(name string) bool { return name == "name" }
	acceptsScope := func(name string) bool { return name == "name" || name == "environment_scope" }

	t.Run("environment is renamed to name", func(t *testing.T) {
		params := map[string]any{"environment": "production"}
		clone, _ := aliasClone(params)
		normalizeEnvironmentNameAlias(params, acceptsName, clone)
		if params["name"] != "production" {
			t.Errorf("name = %q, want production", params["name"])
		}
		if _, ok := params["environment"]; ok {
			t.Error("environment not removed")
		}
	})

	t.Run("preserves existing name and drops environment", func(t *testing.T) {
		params := map[string]any{"environment": "production", "name": "stage"}
		clone, _ := aliasClone(params)
		normalizeEnvironmentNameAlias(params, acceptsName, clone)
		if params["name"] != "stage" {
			t.Errorf("name = %q, want stage (preserved)", params["name"])
		}
		if _, ok := params["environment"]; ok {
			t.Error("environment not removed when name was present")
		}
	})

	t.Run("no-op when environment_scope is accepted", func(t *testing.T) {
		params := map[string]any{"environment": "production"}
		clone, _ := aliasClone(params)
		normalizeEnvironmentNameAlias(params, acceptsScope, clone)
		if _, ok := params["name"]; ok {
			t.Error("name was added despite environment_scope being accepted")
		}
	})
}

// TestDecodeEncodedPathIdentifierDirect verifies the %2f decoder
// returns the decoded path with the changed flag and rejects paths
// that do not contain an encoded slash.
func TestDecodeEncodedPathIdentifierDirect(t *testing.T) {
	got, changed := decodeEncodedPathIdentifier("group%2Fsubgroup%2Fproject")
	if !changed {
		t.Fatal("decodeEncodedPathIdentifier(%2F) changed = false, want true")
	}
	if got != "group/subgroup/project" {
		t.Errorf("decoded = %q, want group/subgroup/project", got)
	}

	// URL percent-encoding is case-insensitive: lowercase %2f must
	// decode the same as uppercase %2F.
	gotLower, changedLower := decodeEncodedPathIdentifier("group%2fsubgroup%2fproject")
	if !changedLower {
		t.Fatal("decodeEncodedPathIdentifier(%2f) changed = false, want true")
	}
	if gotLower != "group/subgroup/project" {
		t.Errorf("decoded = %q, want group/subgroup/project", gotLower)
	}

	if _, isChanged := decodeEncodedPathIdentifier("plain/path"); isChanged {
		t.Error("decodeEncodedPathIdentifier(no %2F) changed = true, want false")
	}
	if _, isChanged := decodeEncodedPathIdentifier(""); isChanged {
		t.Error("decodeEncodedPathIdentifier(empty) changed = true, want false")
	}
	if _, isChanged := decodeEncodedPathIdentifier("no-slash-here"); isChanged {
		t.Error("decodeEncodedPathIdentifier(no slash) changed = true, want false")
	}
}

// TestAppendNormalizedRouteStringsDirect verifies the route-string
// helper dedupes, lowercases, trims whitespace, drops empty entries,
// and returns nil when nothing remains.
func TestAppendNormalizedRouteStringsDirect(t *testing.T) {
	got := appendNormalizedRouteStrings([]string{"  LIST ", "list"}, "GET", "post", " Post ")
	want := []string{"list", "get", "post"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	if isNil := appendNormalizedRouteStrings(nil); isNil != nil {
		t.Errorf("appendNormalizedRouteStrings(no values) = %v, want nil", isNil)
	}
	if isNil := appendNormalizedRouteStrings([]string{" "}); isNil != nil {
		t.Errorf("appendNormalizedRouteStrings(only whitespace) = %v, want nil", isNil)
	}
	if isNil := appendNormalizedRouteStrings([]string{}, "  ", "  "); isNil != nil {
		t.Errorf("appendNormalizedRouteStrings(only blank values) = %v, want nil", isNil)
	}
}

// TestCoercePaginationBooleanDirect verifies the boolean→page-size
// coercion handles page=1, others=100, non-bool, and unsupported
// parameter names.
func TestCoercePaginationBooleanDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// name == "page" → 1
	got, ok := coercePaginationBoolean("page", true, intType)
	if !ok || got != 1 {
		t.Errorf("coercePaginationBoolean(page, true) = %v/%v, want 1/true", got, ok)
	}

	// name in {first,last,per_page} → 100
	for _, name := range []string{"first", "last", "per_page"} {
		got, ok = coercePaginationBoolean(name, true, intType)
		if !ok || got != 100 {
			t.Errorf("coercePaginationBoolean(%s, true) = %v/%v, want 100/true", name, got, ok)
		}
	}

	// bool false → unchanged
	got, ok = coercePaginationBoolean("page", false, intType)
	if ok || got != false {
		t.Errorf("coercePaginationBoolean(page, false) = %v/%v, want false/false", got, ok)
	}

	// non-bool value → unchanged
	got, ok = coercePaginationBoolean("page", 5, intType)
	if ok || got != 5 {
		t.Errorf("coercePaginationBoolean(page, 5) = %v/%v, want 5/false", got, ok)
	}

	// unsupported param name → unchanged
	got, ok = coercePaginationBoolean("name", true, intType)
	if ok || got != true {
		t.Errorf("coercePaginationBoolean(name, true) = %v/%v, want true/false", got, ok)
	}

	// non-numeric target kind → unchanged
	stringType := reflect.TypeFor[string]()
	got, ok = coercePaginationBoolean("page", true, stringType)
	if ok || got != true {
		t.Errorf("coercePaginationBoolean(page, true, string) = %v/%v, want true/false", got, ok)
	}
}

// TestCloneAccessLevelAliasesDirect verifies the alias-to-access_level
// promotion covers the access_level-already-present, known-alias
// matches, no-alias-found, and unhandled-alias scenarios.
func TestCloneAccessLevelAliasesDirect(t *testing.T) {
	// access_level present → no clone
	original := map[string]any{"access_level": 30, "deploy_access_level": 40}
	cloneFn := func() map[string]any {
		t.Fatal("clone() should not be called when access_level is present")
		return nil
	}
	if got := cloneAccessLevelAliases(original, cloneFn); !reflect.DeepEqual(got, original) {
		t.Errorf("got = %+v, want original (access_level present)", got)
	}

	// deploy_access_level present → promoted
	original = map[string]any{"deploy_access_level": 30}
	called := false
	cloned := cloneAccessLevelAliases(original, func() map[string]any {
		called = true
		return map[string]any{}
	})
	if !called {
		t.Error("clone() not called for deploy_access_level")
	}
	if cloned["access_level"] != 30 {
		t.Errorf("access_level = %v, want 30", cloned["access_level"])
	}
	if _, ok := cloned["deploy_access_level"]; ok {
		t.Error("deploy_access_level not removed")
	}

	// no access-level field at all → unchanged
	original = map[string]any{"name": "main"}
	called = false
	got := cloneAccessLevelAliases(original, func() map[string]any {
		called = true
		return map[string]any{}
	})
	if called {
		t.Error("clone() called when no access-level alias is present")
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("got = %+v, want original (no alias)", got)
	}
}

// TestNormalizeAccessLevelScalarDirect verifies the access-level scalar
// normalizer coerces string role names to int, leaves unsupported names
// and non-access-level params untouched, and respects the target kind.
func TestNormalizeAccessLevelScalarDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()
	stringType := reflect.TypeFor[string]()

	got, ok := normalizeAccessLevelScalar("push_access_level", "maintainer", intType)
	if !ok || got != 40 {
		t.Errorf("normalizeAccessLevelScalar(maintainer) = %v/%v, want 40/true", got, ok)
	}

	// name does not look like an access level → unchanged
	got, ok = normalizeAccessLevelScalar("title", "maintainer", intType)
	if ok || got != "maintainer" {
		t.Errorf("normalizeAccessLevelScalar(title) = %v/%v, want maintainer/false", got, ok)
	}

	// target kind is not numeric → unchanged
	got, ok = normalizeAccessLevelScalar("push_access_level", "maintainer", stringType)
	if ok || got != "maintainer" {
		t.Errorf("normalizeAccessLevelScalar(string target) = %v/%v, want maintainer/false", got, ok)
	}

	// pointer to int → still works
	got, ok = normalizeAccessLevelScalar("access_level", "owner", reflect.TypeFor[*int]())
	if !ok || got != 50 {
		t.Errorf("normalizeAccessLevelScalar(*int) = %v/%v, want 50/true", got, ok)
	}

	// unknown role name → unchanged
	got, ok = normalizeAccessLevelScalar("access_level", "wizard", intType)
	if ok || got != "wizard" {
		t.Errorf("normalizeAccessLevelScalar(wizard) = %v/%v, want wizard/false", got, ok)
	}
}

// TestHasStructuredApprovalCountDirect verifies the approval-count
// detector accepts both the canonical field and the alias family.
func TestHasStructuredApprovalCountDirect(t *testing.T) {
	for _, key := range []string{"required_approvals", "required_approval_count", "approval_count", "approvals_required"} {
		if !hasStructuredApprovalCount(map[string]any{key: 2}) {
			t.Errorf("hasStructuredApprovalCount(%s) = false, want true", key)
		}
	}
	if hasStructuredApprovalCount(map[string]any{"other": 1}) {
		t.Error("hasStructuredApprovalCount(other) = true, want false")
	}
	if hasStructuredApprovalCount(map[string]any{}) {
		t.Error("hasStructuredApprovalCount(empty) = true, want false")
	}
}

// TestHasStructuredApprovalPrincipalDirect verifies the principal
// detector accepts both canonical fields and the access-level alias
// family used for protected-branch entries.
func TestHasStructuredApprovalPrincipalDirect(t *testing.T) {
	canonical := []string{"access_level", "user_id", "group_id"}
	aliases := []string{"deploy_access_level", "group_access_level", "project_access_level", "machine_user_access_level"}

	for _, key := range append(canonical, aliases...) {
		if !hasStructuredApprovalPrincipal(map[string]any{key: 1}) {
			t.Errorf("hasStructuredApprovalPrincipal(%s) = false, want true", key)
		}
	}
	if hasStructuredApprovalPrincipal(map[string]any{"name": "alice"}) {
		t.Error("hasStructuredApprovalPrincipal(name) = true, want false")
	}
	if hasStructuredApprovalPrincipal(map[string]any{}) {
		t.Error("hasStructuredApprovalPrincipal(empty) = true, want false")
	}
}

// TestIsStringSliceTypeAndIsStringTypeDirect verify the pointer-aware
// reflection helpers across a representative set of Go types.
func TestIsStringSliceTypeAndIsStringTypeDirect(t *testing.T) {
	cases := []struct {
		name     string
		typ      reflect.Type
		isString bool
		isSlice  bool
	}{
		{"string", reflect.TypeFor[string](), true, false},
		{"*string", reflect.TypeFor[*string](), true, false},
		{"[]string", reflect.TypeFor[[]string](), false, true},
		{"[]int", reflect.TypeFor[[]int](), false, false},
		{"int", reflect.TypeFor[int](), false, false},
		{"bool", reflect.TypeFor[bool](), false, false},
	}
	for _, tc := range cases {
		if got := isStringType(tc.typ); got != tc.isString {
			t.Errorf("%s: isStringType = %v, want %v", tc.name, got, tc.isString)
		}
		if got := isStringSliceType(tc.typ); got != tc.isSlice {
			t.Errorf("%s: isStringSliceType = %v, want %v", tc.name, got, tc.isSlice)
		}
	}
}

// TestSchemaPropertyHasTypeDirect verifies the schema-property type
// detector covers string, []string, []any, and non-object inputs.
func TestSchemaPropertyHasTypeDirect(t *testing.T) {
	if !schemaPropertyHasType(map[string]any{"type": "string"}, "string") {
		t.Error("schemaPropertyHasType(string) = false, want true")
	}
	if !schemaPropertyHasType(map[string]any{"type": []string{"integer", "string"}}, "string") {
		t.Error("schemaPropertyHasType([]string) = false, want true")
	}
	if !schemaPropertyHasType(map[string]any{"type": []any{"integer", "string"}}, "integer") {
		t.Error("schemaPropertyHasType([]any) = false, want true")
	}
	if schemaPropertyHasType(map[string]any{"type": "string"}, "integer") {
		t.Error("schemaPropertyHasType(mismatch) = true, want false")
	}
	if schemaPropertyHasType("not-an-object", "string") {
		t.Error("schemaPropertyHasType(non-object) = true, want false")
	}
	if schemaPropertyHasType(map[string]any{}, "string") {
		t.Error("schemaPropertyHasType(empty) = true, want false")
	}
}

// TestCoerceStructuredValueDirect verifies the structured-value
// coercion helper handles map-with-access_level shorthand, plain
// role strings, scalar values that cannot be interpreted, and
// non-slice targets.
func TestCoerceStructuredValueDirect(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	elemType := sliceType.Elem()
	_ = elemType // documented for direct use elsewhere
}

// TestCoerceStructuredValue_MapWithAccessLevelKey verifies the helper
// wraps a map containing the access_level key into a single-element slice.
func TestCoerceStructuredValue_MapWithAccessLevelKey(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	value, ok := coerceStructuredValue("access_level", map[string]any{"access_level": 30}, sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue(map) = false, want true")
	}
	gotSlice, isSlice := value.([]any)
	if !isSlice || len(gotSlice) != 1 {
		t.Fatalf("value = %v (type %T), want []any with 1 entry", value, value)
	}
}

// TestCoerceStructuredValue_MapWithoutAccessLevelKeyIsStillWrapped verifies
// the helper wraps any map input regardless of the keys it carries.
func TestCoerceStructuredValue_MapWithoutAccessLevelKeyIsStillWrapped(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	value, ok := coerceStructuredValue("access_level", map[string]any{"name": "x"}, sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue(map) = false, want true (map always wrapped)")
	}
	wrapped2, isSlice := value.([]any)
	if !isSlice || len(wrapped2) != 1 {
		t.Errorf("value = %v, want []any with 1 entry", value)
	}
}

// TestCoerceStructuredValue_ScalarAccessLevelString verifies a scalar role
// string is wrapped into a single-element slice keyed by access_level.
func TestCoerceStructuredValue_ScalarAccessLevelString(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	value, ok := coerceStructuredValue("access_level", "maintainer", sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue(maintainer) = false, want true")
	}
	wrapped, isSlice := value.([]any)
	if !isSlice || len(wrapped) != 1 {
		t.Fatalf("value = %v (type %T), want []any with 1 entry", value, value)
	}
	m, isMap := wrapped[0].(map[string]any)
	if !isMap || m["access_level"] != 40 {
		t.Errorf("wrapped[0] = %v, want {access_level: 40}", wrapped[0])
	}
}

// TestCoerceStructuredValue_SliceWithMixedRoleStringsAndMaps verifies a
// []any input containing role strings and a map is coerced element-wise.
func TestCoerceStructuredValue_SliceWithMixedRoleStringsAndMaps(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	items := []any{"developer", map[string]any{"access_level": 60}}
	value, ok := coerceStructuredValue("access_level", items, sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue([]any) = false, want true")
	}
	updated, isSlice := value.([]any)
	if !isSlice || len(updated) != 2 {
		t.Fatalf("value = %v, want []any with 2 entries", value)
	}
	first, isMap := updated[0].(map[string]any)
	if !isMap || first["access_level"] != 30 {
		t.Errorf("updated[0] = %v, want {access_level: 30}", updated[0])
	}
	if !reflect.DeepEqual(updated[1], items[1]) {
		t.Errorf("updated[1] = %v, want unchanged %v", updated[1], items[1])
	}
}

// TestCoerceStructuredValue_NonSliceTargetFallsThrough verifies the helper
// returns the input unchanged when the target type is not a slice.
func TestCoerceStructuredValue_NonSliceTargetFallsThrough(t *testing.T) {
	intType := reflect.TypeFor[int]()
	value, ok := coerceStructuredValue("title", "hello", intType)
	if ok || value != "hello" {
		t.Errorf("coerceStructuredValue(title) = %v/%v, want hello/false", value, ok)
	}
}

// TestCoerceStructuredValue_SliceWithNonStructElemUnchanged verifies the
// helper leaves the input unchanged when the slice element is not a struct.
func TestCoerceStructuredValue_SliceWithNonStructElemUnchanged(t *testing.T) {
	stringSliceType := reflect.TypeFor[[]string]()
	value, ok := coerceStructuredValue("access_level", "maintainer", stringSliceType)
	if ok {
		t.Errorf("coerceStructuredValue(non-struct slice) = %v/true, want false", value)
	}
}

// TestGitLabRoleAccessLevelStringAliases verifies that all canonical
// GitLab role names — including plural forms and underscored/hyphenated
// variants — map to the expected numeric access level.
func TestGitLabRoleAccessLevelStringAliases(t *testing.T) {
	cases := []struct {
		role string
		want int
	}{
		{"guest", 10},
		{"guests", 10},
		{"reporter", 20},
		{"reporters", 20},
		{"developer", 30},
		{"developers", 30},
		{"maintainer", 40},
		{"maintainers", 40},
		{"owner", 50},
		{"owners", 50},
		{"admin", 60},
		{"admins", 60},
		{"administrator", 60},
		{"administrators", 60},
		{"no access", 0},
		{"no one", 0},
		{"nobody", 0},
		{"none", 0},
		// case-insensitive
		{"MAINTAINER", 40},
	}
	for _, tc := range cases {
		got, ok := gitLabRoleAccessLevel(tc.role)
		if !ok || got != tc.want {
			t.Errorf("gitLabRoleAccessLevel(%q) = %d/%v, want %d/true", tc.role, got, ok, tc.want)
		}
	}

	// unknown role → rejected
	if _, ok := gitLabRoleAccessLevel("wizard"); ok {
		t.Error("gitLabRoleAccessLevel(wizard) ok = true, want false")
	}
	// non-string, non-numeric value → rejected
	if _, ok := gitLabRoleAccessLevel(true); ok {
		t.Error("gitLabRoleAccessLevel(bool) ok = true, want false")
	}
}

// TestCoerceSingleStringSlicesDirect verifies the string→[]string
// promotion helper returns the input unchanged when the target type
// has no fields, when a value is not a string, and when the field is
// not a string slice type.
func TestCoerceSingleStringSlicesDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()
	sliceStructType := reflect.TypeOf(struct {
		Labels []string `json:"labels"`
	}{})

	// non-struct target → unchanged
	params := map[string]any{"labels": "bug"}
	if got := coerceSingleStringSlices(params, intType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringSlices(non-struct) = %+v, want %+v", got, params)
	}

	// value is not a string → unchanged
	params = map[string]any{"labels": 42}
	if got := coerceSingleStringSlices(params, sliceStructType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringSlices(non-string value) = %+v, want %+v", got, params)
	}

	// happy path → wrapped as single-element slice
	params = map[string]any{"labels": "bug"}
	got := coerceSingleStringSlices(params, sliceStructType)
	if list, ok := got["labels"].([]string); !ok || len(list) != 1 || list[0] != "bug" {
		t.Errorf("labels = %v, want [bug]", got["labels"])
	}
}

// TestCoerceStringListParamsDirect verifies the comma-separated
// string helper for the labels-style field family.
func TestCoerceStringListParamsDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// non-struct target → unchanged
	params := map[string]any{"labels": "bug,feature"}
	if got := coerceStringListParams(params, intType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceStringListParams(non-struct) = %+v, want %+v", got, params)
	}

	// happy path → joined as single CSV string
	type labelsInput struct {
		Labels string `json:"labels"`
	}
	labelsType := reflect.TypeFor[labelsInput]()
	params = map[string]any{"labels": []string{"bug", "feature"}}
	got := coerceStringListParams(params, labelsType)
	if got["labels"] != "bug,feature" {
		t.Errorf("labels = %v, want bug,feature", got["labels"])
	}
}

// TestCoerceStringIDNumbersDirect verifies the numeric→string coercion
// for the id/_id/_iid parameter family.
func TestCoerceStringIDNumbersDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// non-struct target → unchanged
	params := map[string]any{"id": 42}
	if got := coerceStringIDNumbers(params, intType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceStringIDNumbers(non-struct) = %+v, want %+v", got, params)
	}

	// happy path → string conversion
	type idInput struct {
		ID int `json:"id"`
	}
	idType := reflect.TypeFor[idInput]()
	_ = idType
	// the target must be a string field for the coercion to apply
	type stringIDInput struct {
		ID string `json:"id"`
	}
	stringIDType := reflect.TypeFor[stringIDInput]()
	params = map[string]any{"id": 42}
	got := coerceStringIDNumbers(params, stringIDType)
	if got["id"] != "42" {
		t.Errorf("id = %v, want 42", got["id"])
	}
}

// TestCoerceNumericParamsDirect verifies the numeric coercion for the
// standard integer/float parameter shapes, including the empty-fields
// early return.
func TestCoerceNumericParamsDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// non-struct target → unchanged
	params := map[string]any{"count": "5"}
	got, err := coerceNumericParams(params, intType)
	if err != nil {
		t.Errorf("coerceNumericParams(non-struct) error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, params) {
		t.Errorf("coerceNumericParams(non-struct) = %+v, want %+v", got, params)
	}
}

// TestCoerceUnsignedIntegerValueDirect verifies the unsigned-integer
// coercion helper returns the input unchanged for non-string values.
func TestCoerceUnsignedIntegerValueDirect(t *testing.T) {
	// non-string value → unchanged
	value, changed, err := coerceUnsignedIntegerValue("count", 42)
	if err != nil || changed || value != 42 {
		t.Errorf("coerceUnsignedIntegerValue(int) = %v/%v/%v, want 42/false/nil", value, changed, err)
	}
}

// TestCoerceFloatValueDirect verifies the float coercion helper
// returns the input unchanged for non-string values.
func TestCoerceFloatValueDirect(t *testing.T) {
	// non-string value → unchanged
	value, changed, err := coerceFloatValue("weight", 3.14)
	if err != nil || changed || value != 3.14 {
		t.Errorf("coerceFloatValue(float) = %v/%v/%v, want 3.14/false/nil", value, changed, err)
	}
}

// TestCoerceSliceValueForTargetTypeDirect verifies the slice element
// coercion helper, including the non-numeric-element early return.
func TestCoerceSliceValueForTargetTypeDirect(t *testing.T) {
	stringType := reflect.TypeFor[string]()

	// non-numeric elem → unchanged
	value, changed, err := coerceSliceValueForTargetType("ids", []string{"a", "b"}, stringType)
	if err != nil || changed {
		t.Errorf("coerceSliceValueForTargetType(string elem) = %v/%v/%v, want no-op", value, changed, err)
	}
}

// TestCoerceSchemaParamTypesDirect verifies the schema-aware coercion
// returns the input unchanged for schemas without a "properties" map.
func TestCoerceSchemaParamTypesDirect(t *testing.T) {
	params := map[string]any{"id": 1}
	if got := coerceSchemaParamTypes(params, map[string]any{}); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSchemaParamTypes(no props) = %+v, want %+v", got, params)
	}
}

// TestCoerceSchemaArrayValueDirect verifies the array-value coercion
// returns the input unchanged for properties that are not "array" typed.
func TestCoerceSchemaArrayValueDirect(t *testing.T) {
	params := []string{"1", "2"}
	got, changed := coerceSchemaArrayValue(params, map[string]any{"type": "string"})
	if changed || !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSchemaArrayValue(non-array) = %v/%v, want %+v/false", got, changed, params)
	}
}

// TestStringListToCSVDirect verifies the string-list→CSV helper
// rejects unsupported input types and joins homogeneous string slices.
func TestStringListToCSVDirect(t *testing.T) {
	// non-slice input → empty
	if csv, ok := stringListToCSV("not-a-list"); ok || csv != "" {
		t.Errorf("stringListToCSV(string) = %q/%v, want empty/false", csv, ok)
	}
	// []any with non-string entry → empty
	if csv, ok := stringListToCSV([]any{"a", 2}); ok || csv != "" {
		t.Errorf("stringListToCSV(mixed) = %q/%v, want empty/false", csv, ok)
	}
	// []any with all strings → joined CSV
	if csv, ok := stringListToCSV([]any{"a", "b"}); !ok || csv != "a,b" {
		t.Errorf("stringListToCSV([]any) = %q/%v, want a,b/true", csv, ok)
	}
	// []string → joined CSV (concrete slice type)
	if csv, ok := stringListToCSV([]string{"a", "b"}); !ok || csv != "a,b" {
		t.Errorf("stringListToCSV([]string) = %q/%v, want a,b/true", csv, ok)
	}
}

// TestCoerceSingleStringArraysForSchemaDirect verifies the
// schema-aware single-string→[]string coercion falls through for
// non-array properties.
func TestCoerceSingleStringArraysForSchemaDirect(t *testing.T) {
	params := map[string]any{"name": "alice"}
	schema := map[string]any{
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	if got := coerceSingleStringArraysForSchema(params, schema); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringArraysForSchema(non-array) = %+v, want %+v", got, params)
	}
}

// TestHasUnknownParamNamesDirect verifies the unknown-parameter
// detector handles empty params, missing properties, and unknown keys.
func TestHasUnknownParamNamesDirect(t *testing.T) {
	// empty params → false
	if hasUnknownParamNames(map[string]any{"x": 1}, map[string]any{}) {
		t.Error("hasUnknownParamNames(empty) = true, want false")
	}
	// schema has no properties → false
	if hasUnknownParamNames(map[string]any{}, map[string]any{"unknown": 1}) {
		t.Error("hasUnknownParamNames(no props) = true, want false")
	}
	// unknown key present → true
	schema := map[string]any{"properties": map[string]any{"known": map[string]any{}}}
	if !hasUnknownParamNames(schema, map[string]any{"unknown": 1}) {
		t.Error("hasUnknownParamNames(unknown) = false, want true")
	}
	// only known keys → false
	if hasUnknownParamNames(schema, map[string]any{"known": 1}) {
		t.Error("hasUnknownParamNames(known only) = true, want false")
	}
}

// TestValidateMetaToolParamsDirect verifies the meta-tool input
// validator returns the "params is required" error for nil params
// with a non-empty required-params list.
func TestValidateMetaToolParamsDirect(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
		"required": []any{"project_id"},
	}
	route := ActionRoute{InputSchema: schema}
	input := &MetaToolInput{Action: "get_project", Params: nil}

	result := validateMetaToolParams("tool", route, input)
	if result == nil {
		t.Fatal("validateMetaToolParams(nil params, required present) = nil, want error result")
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want true")
	}
}

// TestValidateMetaToolParamsMissingRequiredDirect covers the branch that
// fires when Params is provided but is missing required fields and does not
// contain unknown names (so the unknown-name short-circuit does not apply).
func TestValidateMetaToolParamsMissingRequiredDirect(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
		"required": []any{"project_id"},
	}
	route := ActionRoute{InputSchema: schema}
	input := &MetaToolInput{Action: "get_project", Params: map[string]any{}}

	result := validateMetaToolParams("tool", route, input)
	if result == nil {
		t.Fatal("validateMetaToolParams(missing required) = nil, want error result")
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want true")
	}
}

// TestCollectJSONFieldTypesDirect verifies the recursive JSON field
// type collector handles embedded (anonymous) struct fields.
func TestCollectJSONFieldTypesDirect(t *testing.T) {
	type inner struct {
		Name string `json:"name"`
	}
	type outer struct {
		inner
		ID int `json:"id"`
	}
	fields := map[string]reflect.Type{}
	collectJSONFieldTypes(reflect.TypeFor[outer](), fields)
	if _, ok := fields["name"]; !ok {
		t.Error("collectJSONFieldTypes missing embedded 'name' field")
	}
	if _, ok := fields["id"]; !ok {
		t.Error("collectJSONFieldTypes missing 'id' field")
	}
}

// TestMetaToolParameterGuidanceSummaryEmptyItem verifies the
// parameter-guidance renderer skips entries that carry no semantic
// role, value source, or common confusions — even when the parent
// action has a non-empty ParameterGuidance map.
func TestMetaToolParameterGuidanceSummaryEmptyItem(t *testing.T) {
	routes := ActionMap{
		"do_thing": {
			ParameterGuidance: map[string]ParameterGuidance{
				"empty_param": {},
				"described":   {SemanticRole: "scope"},
			},
		},
	}
	got := metaToolParameterGuidanceSummary(routes, []string{"do_thing"})
	if !strings.Contains(got, "do_thing.described") {
		t.Errorf("summary = %q, want to contain 'do_thing.described'", got)
	}
	if strings.Contains(got, "do_thing.empty_param") {
		t.Errorf("summary = %q, must not contain 'do_thing.empty_param'", got)
	}
}

// TestEnrichWithHints_NonObjectJSONResult verifies the JSON-result
// path that requires the marshaled result to start with '{'. Result
// values that marshal to non-object JSON (e.g. arrays) are returned
// unchanged without crashing.
func TestEnrichWithHints_NonObjectJSONResult(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "<!-- HINTS: [\"step-1\"] -->\nSome text."},
		},
	}
	result := []string{"a", "b"}
	got := enrichWithHints(result, callResult)
	if _, ok := got.([]string); !ok {
		t.Errorf("enrichWithHints(non-object) = %T, want original []string", got)
	}
}
