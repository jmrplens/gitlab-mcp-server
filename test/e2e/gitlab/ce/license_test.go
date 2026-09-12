//go:build e2e

// license_test.go covers the license API on an instance that has no license,
// which is the one answer the ee package can never see.

package ce

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// actionLicenseGet reads the installed license through the admin group.
const actionLicenseGet harness.ActionID = "admin.license_get"

// TestLicense_UnlicensedInstanceHasNone runs admin.license_get on every
// surface and asserts the answer of an instance with no license: the
// endpoint does not exist on the Community image and answers 404; an
// Enterprise image left unlicensed answers the same, or 403 for a token
// without the right, or an empty license that the action reports as none
// installed. The action's own wording is checked too, since the error is
// wrapped with the operation's name and that is what a model reads.
//
// Replaces: TestMeta_AdminLicenseGetCE
func TestLicense_UnlicensedInstanceHasNone(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		text := harness.ExpectToolError(s, actionLicenseGet, nil, "license")

		if !strings.Contains(text, "404") && !strings.Contains(text, "403") && !strings.Contains(text, "none installed") {
			e.T.Errorf("the unlicensed instance answered %s with something other than a missing license: %q", actionLicenseGet, text)
		}
	})
}
