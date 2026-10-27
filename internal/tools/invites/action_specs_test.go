// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go holds the invitations' input schemas to what GitLab
// takes as an invitation's email: one address or several separated by
// commas, which no single-address format describes.
package invites

import (
	"strings"
	"testing"
)

// TestActionSpecs_InviteEmail_PublishesNoSingleAddressFormat verifies that
// the two invitations publish email with no format and a description naming
// the comma-separated form. GitLab's invitations API declares the email
// email_or_email_list and splits it on commas before it checks each address,
// so the email format every other endpoint's email carries, which names one
// address, would misdescribe it (issue 1272). The two listings take no email.
func TestActionSpecs_InviteEmail_PublishesNoSingleAddressFormat(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			email, has := properties["email"].(map[string]any)
			if want := spec.Name == "invite_project" || spec.Name == "invite_group"; has != want {
				t.Fatalf("%s has an email property: %t, want %t", spec.Name, has, want)
			}
			if !has {
				return
			}
			if format, set := email["format"]; set {
				t.Errorf("%s email carries format %v, want none", spec.Name, format)
			}
			if description, _ := email["description"].(string); !strings.Contains(description, "separated by commas") {
				t.Errorf("%s email description = %q, want it to name the comma-separated form", spec.Name, description)
			}
		})
	}
}
