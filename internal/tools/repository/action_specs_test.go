// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go holds the changelog actions' input schemas to the
// semantic version GitLab requires of a release's version.
package repository

import (
	"regexp"
	"testing"
)

// TestActionSpecs_ChangelogVersion_IsHeldToTheSemanticVersionGitLabRequires
// verifies that the two changelog actions, and no other, publish version with
// semanticVersionPattern, so a model knows the form and find's example sends
// one where it wrote "value", which GitLab answered with 400 (issue 1272).
//
// The pattern is then held to GitLab's own rule: both routes declare version
// with Gitlab::Regex.unbounded_semver_regex, which Grape matches anywhere in
// the value. Every value below was put to that regular expression, loaded
// from GitLab's source, the way Grape's RegexpValidator puts it, and the
// pattern answers each one as GitLab does: a leading v, a prerelease, a build,
// a fourth number and a prefix are all taken, since the core is found inside
// them, and a value with two numbers or a leading zero on the minor one is
// refused.
func TestActionSpecs_ChangelogVersion_IsHeldToTheSemanticVersionGitLabRequires(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			version, has := properties["version"].(map[string]any)
			want := spec.Name == "changelog_add" || spec.Name == "changelog_generate"
			if has != want {
				t.Fatalf("%s has a version property: %t, want %t", spec.Name, has, want)
			}
			if has && version["pattern"] != semanticVersionPattern {
				t.Errorf("%s version pattern = %v, want semanticVersionPattern", spec.Name, version["pattern"])
			}
		})
	}

	pattern := regexp.MustCompile(semanticVersionPattern)
	cases := []struct {
		value string
		want  bool
	}{
		{value: "1.0.0", want: true},
		{value: "0.0.0", want: true},
		{value: "v1.2.3", want: true},
		{value: "1.0.0-rc.1+build.5", want: true},
		{value: "2.0.0-alpha", want: true},
		{value: "release-10.20.30", want: true},
		{value: "1.2.3.4", want: true},
		{value: "01.2.3", want: true},
		{value: "1.0.01", want: true},
		{value: "value", want: false},
		{value: "1.0", want: false},
		{value: "v1", want: false},
		{value: "1.02.3", want: false},
		{value: "1..0.0", want: false},
		{value: "1.0.x", want: false},
		{value: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := pattern.MatchString(tc.value); got != tc.want {
				t.Errorf("semanticVersionPattern matches %q = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}
