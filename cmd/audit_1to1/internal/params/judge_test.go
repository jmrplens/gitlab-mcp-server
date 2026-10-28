// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// TestPathParams_EveryPlaceholderAndSplat_IsAPathParam verifies the params
// a route's path places, which are never judged: each placeholder and each
// splat, in every spelling of the route's optional groups, and no param for a
// bare marker that names none.
func TestPathParams_EveryPlaceholderAndSplat_IsAPathParam(t *testing.T) {
	path := apilive.EndpointPrefix + "/projects/:id/packages/generic/:package_name/*package_version/(*path/):file_name/*"
	got := slices.Sorted(maps.Keys(pathParams(path)))
	want := []string{"file_name", "id", "package_name", "package_version", "path"}
	if !slices.Equal(got, want) {
		t.Errorf("pathParams = %v, want %v", got, want)
	}
}

// TestWorkhorseAttribute_OnlyAnAttributeOfAnUploadedFile_IsLeftOut verifies
// which dotted params Workhorse fills: those under a param the record types
// as a Workhorse file, and not a dotted key under anything else, such as a
// webhook payload's.
func TestWorkhorseAttribute_OnlyAnAttributeOfAnUploadedFile_IsLeftOut(t *testing.T) {
	params := map[string]apilive.Param{
		"file": {Type: workhorseFile}, "file.sha256": {}, "pull_request": {Type: "Hash"}, "pull_request.number": {},
	}
	cases := map[string]bool{"file.sha256": true, "file": false, "pull_request.number": false, "absent.key": false}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := workhorseAttribute(params, name); got != want {
				t.Errorf("workhorseAttribute(%q) = %t, want %t", name, got, want)
			}
		})
	}
}

// TestDeclaredParent_ANestedParam_SitsUnderItsNearestDeclaredAncestor
// verifies the tree a route's params are judged as: a member sits under the
// nearest param the route declares above it, a list's empty subscript read
// through, past an ancestor the route does not declare, and at the top level
// under none or under a path param.
func TestDeclaredParent_ANestedParam_SitsUnderItsNearestDeclaredAncestor(t *testing.T) {
	params := map[string]apilive.Param{
		"a": {}, "a[b]": {}, "a[b][c]": {}, "x[y][z]": {}, "list": {}, "list[][key]": {}, "id": {}, "id[deep]": {},
	}
	skipped := map[string]bool{"id": true}
	cases := map[string]string{
		"a": "", "a[b]": "a", "a[b][c]": "a[b]", "x[y][z]": "", "list[][key]": "list", "id[deep]": "",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := declaredParent(params, skipped, name); got != want {
				t.Errorf("declaredParent(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// TestClassOf_WhoCanSendIt_NamesTheClass verifies the class of a param
// nothing recorded carries, in its order of precedence: an input that
// offers it, then an option struct that models it, then options the walk
// could not read, and otherwise nothing that can send it.
func TestClassOf_WhoCanSendIt_NamesTheClass(t *testing.T) {
	cases := []struct {
		sdkModels, offered, unread bool
		want                       string
	}{
		{offered: true, sdkModels: true, unread: true, want: ClassOfferedUnrecorded},
		{sdkModels: true, unread: true, want: ClassNotOffered},
		{unread: true, want: ClassUnjudged},
		{want: ClassUnmodeled},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := classOf(tc.sdkModels, tc.offered, tc.unread); got != tc.want {
				t.Errorf("classOf(%t, %t, %t) = %q, want %q", tc.sdkModels, tc.offered, tc.unread, got, tc.want)
			}
		})
	}
}

// TestJudgeRoute_AContainer_IsCoveredByAMemberOrStandsForThem verifies the
// three ways a container comes out: covered when a member is, its other
// members reported on their own; one finding standing for every member when
// none is covered and all share its class, a member's own members folded
// in; and dropped for its members' findings when their classes differ.
func TestJudgeRoute_AContainer_IsCoveredByAMemberOrStandsForThem(t *testing.T) {
	use := newUse("GET")
	use.route.Path = apilive.EndpointPrefix + "/things"
	use.route.Params = map[string]apilive.Param{
		"not": {}, "not[a]": {}, "not[b]": {},
		"deep": {}, "deep[x]": {}, "deep[x][y]": {},
		"mixed": {}, "mixed[m]": {}, "mixed[n]": {},
	}
	use.sends["mixed[m]"] = true
	verdict := judgeRoute(use, map[string]bool{"not[a]": true}, func(string) bool { return false })

	var got []string
	for _, finding := range verdict.findings {
		got = append(got, finding.Param+" "+finding.Class+" "+slicesString(finding.Children))
	}
	want := []string{
		"deep unmodeled [deep[x] deep[x][y]]",
		"mixed[m] not_offered []",
		"mixed[n] unmodeled []",
		"not[b] unmodeled []",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
	if verdict.judged != 9 || verdict.covered != 1 {
		t.Errorf("judged %d, covered %d; want 9 and 1", verdict.judged, verdict.covered)
	}
}

// slicesString spells a list for a comparison.
func slicesString(values []string) string {
	return "[" + strings.Join(values, " ") + "]"
}
