//go:build httpe2e

// gateway_test.go covers what an MCP gateway in front of this server reads
// and can be made to read.
//
// A gateway validates the catalog before admitting it, under rules its
// operator chooses and this project cannot predict. The served text is kept
// pure ASCII with no semicolons so the common rules pass unaided, and
// --description-substitutions is the escape hatch for the next rule, which
// rewrites listed text on the way out without a release.
//
// That knob had no end-to-end coverage at all. It is exactly the kind of
// feature a unit test can be green about while the served catalog is
// unchanged, because what it must affect is the bytes on the wire.
package httpe2e

import (
	"net/http"
	"strings"
	"testing"
)

// substitutionSource is a word every surface's listed text contains, so a
// rewrite of it is observable in tools/list.
const substitutionSource = "catalog"

// substitutionTarget is what it is rewritten to. Distinctive enough that
// finding it proves the rewrite rather than coinciding with catalog prose.
const substitutionTarget = "QQcatalogueQQ"

// TestGateway_DescriptionSubstitutionsRewriteTheServedCatalog is the knob a
// deployment reaches for when the gateway in front of it refuses a character
// this server has no reason to stop using.
//
// The assertion is on the wire rather than on the rewriter, because the
// rewriter being correct is not the claim: the claim is that what tools/list
// serves went through it. A substitution wired to a surface the gateway does
// not read would pass every unit test and change nothing the gateway sees.
func TestGateway_DescriptionSubstitutionsRewriteTheServedCatalog(t *testing.T) {
	gitlab := startFakeGitLabServingAProject(t)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.URL,
		"--description-substitutions="+substitutionSource+"="+substitutionTarget,
	)

	got := srv.do(t, request{
		body:    toolsListBody,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-substitutions"},
	})
	if got.status != http.StatusOK {
		t.Fatalf("tools/list = %d: %s", got.status, truncate(got.body))
	}
	if !strings.Contains(got.body, substitutionTarget) {
		t.Errorf("the served catalog carries no %q; the substitution did not reach tools/list", substitutionTarget)
	}

	// The names are not rewritten, whatever the substitution says. A gateway
	// that rejected a description would be satisfied by rewriting prose; a
	// server that also rewrote the tool names would break every client
	// configuration and every documented example at once.
	for _, name := range []string{"gitlab_find_action", "gitlab_execute_action"} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(got.body, name) {
				t.Errorf("tool %s is missing from the served catalog; a substitution must never touch a name", name)
			}
		})
	}
}

// TestGateway_AMalformedSubstitutionStopsStartup keeps the failure where an
// operator can see it.
//
// The setting exists because a gateway is refusing the catalog. Starting with
// a value that could not be parsed would serve that same gateway the
// unrewritten text it already refused, and the deployment would look
// configured while being exactly as broken as before.
func TestGateway_AMalformedSubstitutionStopsStartup(t *testing.T) {
	for _, value := range []string{"nopairhere", "=empty-old"} {
		t.Run(value, func(t *testing.T) {
			out, err := runServerExpectingExit(t, serverBinary(t),
				"--http", "--allow-any-gitlab-url",
				"--http-addr=127.0.0.1:0",
				"--description-substitutions="+value,
			)
			if err == nil {
				t.Fatalf("the server started with --description-substitutions=%q; output:\n%s", value, out)
			}
			// The refusal names the environment variable rather than the
			// flag, because the flag writes that variable and one reader
			// parses it. Asserted as it is rather than as it might read
			// better, so the assertion stays a fact about the binary.
			if !strings.Contains(out, "GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS") {
				t.Errorf("the refusal must name the setting it refused; output:\n%s", out)
			}
			if !strings.Contains(out, value) {
				t.Errorf("the refusal must quote the value it could not parse; output:\n%s", out)
			}
		})
	}
}
