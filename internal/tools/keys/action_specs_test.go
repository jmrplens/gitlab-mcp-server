// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go holds the fingerprint lookup's input schema to the
// forms GitLab reads a fingerprint in.
package keys

import (
	"regexp"
	"strings"
	"testing"
)

// TestActionSpecs_Fingerprint_IsHeldToTheFormsGitLabReads verifies that the
// fingerprint lookup publishes fingerprintPattern, so a model knows the forms
// and find's example sends one where it wrote "value", which GitLab refused
// with 400 (issue 1272); the lookup by ID takes no fingerprint. The pattern
// is then held to GitLab's own rule (KeysFinder#valid_fingerprint_param?):
// a SHA256 fingerprint whose base64 holds 32 bytes, behind either spelling of
// its label, and an MD5 one of 16 hex pairs, behind the MD5: label the
// handler removes, are taken, and every other form is refused.
func TestActionSpecs_Fingerprint_IsHeldToTheFormsGitLabReads(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			fingerprint, has := properties["fingerprint"].(map[string]any)
			if has != (spec.Name == "key_get_by_fingerprint") {
				t.Fatalf("%s has a fingerprint property: %t, want it on the fingerprint lookup alone", spec.Name, has)
			}
			if has && fingerprint["pattern"] != fingerprintPattern {
				t.Errorf("%s fingerprint pattern = %v, want fingerprintPattern", spec.Name, fingerprint["pattern"])
			}
		})
	}

	pairs := strings.TrimSuffix(strings.Repeat("0f:", 16), ":")
	pattern := regexp.MustCompile(fingerprintPattern)
	cases := []struct {
		value string
		want  bool
	}{
		{value: "SHA256:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", want: true},
		{value: "sha256:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=", want: true},
		{value: pairs, want: true},
		{value: strings.ToUpper(pairs), want: true},
		{value: "MD5:" + pairs, want: true},
		{value: "SHA256:abc123", want: false},
		{value: "Sha256:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", want: false},
		{value: "aa:bb:cc", want: false},
		{value: pairs + ":0f", want: false},
		{value: "MD5:aa:bb:cc", want: false},
		{value: "md5:" + pairs, want: false},
		{value: "value", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := pattern.MatchString(tc.value); got != tc.want {
				t.Errorf("fingerprintPattern matches %q = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}
