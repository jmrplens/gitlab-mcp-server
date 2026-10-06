package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestRenderRequests_SpellsEachRequestByItsKind verifies each request is
// written in the record's spelling for its kind: a route as the live record
// names it with the derivation's spelling beside it where a declaration
// placed it, an operation with its root fields and positions, and an
// unresolved one by its reason; each request with the classic scope it needs
// and what departed from GitLab's rule to decide it, and each action with its
// own and the scopes its group demands, unknown for an action with no row; an
// action declared to send nothing keeps an empty list of paths rather than
// none.
func TestRenderRequests_SpellsEachRequestByItsKind(t *testing.T) {
	actions := []join.Action{
		{
			ID: "a.mixed", Handlers: []string{"pkg.Mixed"}, Paths: [][]int{{0, 1}, {2}},
			Row: &finegrained.Requirement{ID: "a.mixed", Classic: finegrained.ClassicReadAPI},
			Requests: []join.Request{
				{
					Kind: derive.KindREST, Method: "HEAD", Path: "/projects/:/files/:/raw",
					Class: derive.ClassAlternative, Sites: []string{"pkg.Mixed"}, SDKMethods: []string{"Files.Raw"},
					Name: "GET /projects/:id/files/:file_path/raw", Route: "HEAD /projects/:/files/:/raw",
					Classic: finegrained.ClassicReadAPI,
				},
				{
					Kind: derive.KindGraphQL, Document: "query { a }",
					Class: derive.ClassAlternative, Directives: []string{"mandatory: why"}, Declaration: "sdk-graphql-template",
					Name: "query a (x)", RootFields: []string{"a"}, Positions: []string{"a"},
					Classic: finegrained.ClassicAPI, ClassicDeclaration: "api-only-field Issue.createNoteEmail",
				},
				{Kind: derive.KindUnresolved, Reason: "raw-path pkg.Mixed", Class: derive.ClassAlternative},
			},
		},
		{ID: "a.nothing", Handlers: []string{"pkg.Nothing"}, Declaration: "sends-nothing"},
	}
	want := `{
  "note": "` + actionrequests.RecordNote + `",
  "actions": [
    {
      "id": "a.mixed",
      "handlers": [
        "pkg.Mixed"
      ],
      "classic": "read_api",
      "group_scopes": [
        "admin_mode"
      ],
      "requests": [
        {
          "kind": "rest",
          "route": "GET /projects/:id/files/:file_path/raw",
          "derived": "HEAD /projects/:/files/:/raw",
          "class": "alternative",
          "classic": "read_api",
          "sites": [
            "pkg.Mixed"
          ],
          "sdk_methods": [
            "Files.Raw"
          ]
        },
        {
          "kind": "graphql",
          "operation": "query a (x)",
          "root_fields": [
            "a"
          ],
          "positions": [
            "a"
          ],
          "class": "alternative",
          "classic": "api",
          "classic_declaration": "api-only-field Issue.createNoteEmail",
          "directives": [
            "mandatory: why"
          ],
          "declaration": "sdk-graphql-template"
        },
        {
          "kind": "unresolved",
          "reason": "raw-path pkg.Mixed",
          "class": "alternative",
          "classic": "unknown"
        }
      ],
      "paths": [
        [
          0,
          1
        ],
        [
          2
        ]
      ]
    },
    {
      "id": "a.nothing",
      "handlers": [
        "pkg.Nothing"
      ],
      "declaration": "sends-nothing",
      "classic": "unknown",
      "paths": []
    }
  ]
}`
	if got := string(renderRequests(actions, map[string][]string{"a.mixed": {"admin_mode"}})); got != want {
		t.Errorf("renderRequests =\n%s\nwant\n%s", got, want)
	}
}
