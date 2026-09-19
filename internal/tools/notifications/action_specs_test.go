// action_specs_test.go holds the tests over the canonical action IDs the
// notification specs and their Markdown hints publish.
package notifications

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// hintActionID matches the ID inside the text toolutil.HintAction renders,
// "Use action 'user.notification_global_get' to …", which is how a model reads
// a cross-link out of a card.
var hintActionID = regexp.MustCompile(`Use action '([^']+)' to `)

// registeredNotificationIDs is the set of canonical IDs this package really
// registers: one per spec, qualified the way the catalog qualifies it.
//
// It is built from the specs rather than written out, because a list written
// out beside the constants would agree with them whatever either of them said.
func registeredNotificationIDs(t *testing.T) map[string]bool {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	specs := ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs returned nothing")
	}
	ids := make(map[string]bool, len(specs))
	for _, spec := range specs {
		ids[canonicalID(spec.Name)] = true
	}
	return ids
}

// TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds asserts that every
// related action a notification spec publishes is an ID this package really
// registers.
//
// All eighteen of them used to be the bare spec name, "notification_group_get"
// and the like, with no domain in front. A bare name resolves to no action, so
// a model following one is answered "unknown action", and nothing checked it:
// the constants read like IDs, and they were the names the specs are
// registered under. Every notification cross-link points at a sibling
// notification action, so the set they are held against is this package's own.
func TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	registered := registeredNotificationIDs(t)

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatalf("%s publishes no related actions", spec.Name)
			}
			for _, related := range spec.RelatedActions {
				if !registered[related] {
					t.Errorf("%s relates to %q, which no notification action is registered under", spec.Name, related)
				}
			}
		})
	}
}

// TestFormatMarkdownString_HintsNameActionsTheCatalogHolds asserts that every
// action ID the settings card invites a model to call next is one this package
// registers.
//
// The card is the second place these IDs are published, and it is the half
// that was right while the specs were wrong, which is exactly how the two
// drifted: a reader fixing one had no reason to look at the other. Both now
// read the same constants, and this holds the rendered text to the specs so
// that staying in step is checked rather than assumed.
func TestFormatMarkdownString_HintsNameActionsTheCatalogHolds(t *testing.T) {
	registered := registeredNotificationIDs(t)

	markdown := FormatMarkdownString(Output{Level: "custom", Events: &EventOutput{}})
	matches := hintActionID.FindAllStringSubmatch(markdown, -1)
	if len(matches) == 0 {
		t.Fatalf("the settings card names no action to call next:\n%s", markdown)
	}
	for _, match := range matches {
		t.Run(match[1], func(t *testing.T) {
			if !registered[match[1]] {
				t.Errorf("the settings card invites %q, which no notification action is registered under", match[1])
			}
		})
	}
}

// TestCanonicalID_QualifiesEverySpecName asserts that a spec name and the ID a
// caller passes are two different strings, and that the second is the first
// under this package's catalog domain.
//
// It is the invariant the whole fix rests on: the specs are registered under
// bare names, the catalog publishes them under a domain, and anything
// published to a model has to be the qualified form.
func TestCanonicalID_QualifiesEverySpecName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			id := canonicalID(spec.Name)
			if id == spec.Name {
				t.Fatalf("canonicalID(%q) returned the spec name unchanged", spec.Name)
			}
			if want := catalogDomain + "." + spec.Name; id != want {
				t.Errorf("canonicalID(%q) = %q, want %q", spec.Name, id, want)
			}
		})
	}
}
