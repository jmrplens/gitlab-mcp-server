package actioncatalog

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestRewriteSeeAlso_Resolver_ProjectsTheClause pins how a clause is
// projected into a surface's names: a nil resolver is the individual
// surface's path, where the hand-written names are already the ones it
// registers, so the text passes untouched; names the resolver knows are
// rewritten and the rest dropped; and a clause left with no name is removed
// whole, with the space or line break before it, since "See also: ." would be
// an instruction to nowhere.
func TestRewriteSeeAlso_Resolver_ProjectsTheClause(t *testing.T) {
	const withSeeAlso = "Get one project. See also: gitlab_get_group, gitlab_list_projects."
	knowsGroup := func(name string) (string, bool) {
		if name == "gitlab_get_group" {
			return "group.get", true
		}
		return "", false
	}
	knowsNothing := func(string) (string, bool) { return "", false }
	tests := []struct {
		name        string
		description string
		resolve     SeeAlsoResolver
		want        string
	}{
		{name: "nil resolver leaves the text", description: withSeeAlso, resolve: nil, want: withSeeAlso},
		{name: "known names rewritten, unknown dropped", description: withSeeAlso, resolve: knowsGroup, want: "Get one project. See also: group.get."},
		{name: "empty clause removed with its space", description: withSeeAlso, resolve: knowsNothing, want: "Get one project."},
		{name: "empty clause removed with its line breaks", description: "Get one project.\n\nSee also: gitlab_gone.", resolve: knowsNothing, want: "Get one project."},
		{name: "text without a clause unchanged", description: "Get one project.", resolve: knowsNothing, want: "Get one project."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RewriteSeeAlso(tt.description, tt.resolve); got != tt.want {
				t.Errorf("RewriteSeeAlso() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestServedDescription_Action_DescriptionRewrittenUsageVerbatim pins which
// text an action is served with: its individual tool's Description through
// the clause rewrite, and its Usage line as it is when it has no Description,
// a clause in it included, because that is what gitlab://tools serves and
// what the site's tool reference publishes as the same text.
func TestServedDescription_Action_DescriptionRewrittenUsageVerbatim(t *testing.T) {
	resolve := func(name string) (string, bool) {
		if name == "gitlab_widget_get" {
			return "widget.get", true
		}
		return "", false
	}
	tests := []struct {
		name   string
		action Action
		want   string
	}{
		{
			name:   "description rewritten",
			action: Action{Usage: "Use it.", IndividualTool: toolutil.IndividualToolSpec{Description: "Get it. See also: gitlab_widget_get, gitlab_gone."}},
			want:   "Get it. See also: widget.get.",
		},
		{
			name:   "usage verbatim when undescribed",
			action: Action{Usage: "Use it. See also: gitlab_widget_get.\n"},
			want:   "Use it. See also: gitlab_widget_get.\n",
		},
		{
			name:   "nothing to serve",
			action: Action{},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ServedDescription(tt.action, resolve); got != tt.want {
				t.Errorf("ServedDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSeeAlsoClause_ClauseShapes_MatchExactlyTheClause pins what the one
// definition of the clause consumes, since both of its readers act on exactly
// that span: the manifests rewrite it and the served-prose gate skips it. The
// match must stop at the clause's own period, so a sentence after it stays
// outside, and a clause the pattern cannot consume must match nothing rather
// than a prefix of it.
func TestSeeAlsoClause_ClauseShapes_MatchExactlyTheClause(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantMatch   string
		wantNames   string
	}{
		{
			name:        "individual tool names",
			description: "Get a thing. See also: gitlab_thing_list, gitlab_thing_delete.",
			wantMatch:   "See also: gitlab_thing_list, gitlab_thing_delete.",
			wantNames:   "gitlab_thing_list, gitlab_thing_delete",
		},
		{
			name:        "canonical IDs, whose dots the class admits",
			description: "Resolve a remote. See also: project.get, server.status.",
			wantMatch:   "See also: project.get, server.status.",
			wantNames:   "project.get, server.status",
		},
		{
			name:        "a sentence after the clause stays outside it",
			description: "List things. See also: gitlab_a. Reversible via gitlab_b.",
			wantMatch:   "See also: gitlab_a.",
			wantNames:   "gitlab_a",
		},
		{
			name:        "a clause without its period matches nothing",
			description: "Get a thing.\n\nSee also: gitlab_thing_list, gitlab_thing_delete",
		},
		{
			name:        "a parenthetical annotation matches nothing",
			description: "Resolve. See also: gitlab_thing_get (full CRUD).",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := SeeAlsoClause.FindStringSubmatch(tt.description)
			if tt.wantMatch == "" {
				if match != nil {
					t.Errorf("SeeAlsoClause matched %q in %q, want no match", match[0], tt.description)
				}
				return
			}
			if match == nil {
				t.Fatalf("SeeAlsoClause matched nothing in %q, want %q", tt.description, tt.wantMatch)
			}
			if match[0] != tt.wantMatch || match[1] != tt.wantNames {
				t.Errorf("SeeAlsoClause match = %q with names %q, want %q with names %q", match[0], match[1], tt.wantMatch, tt.wantNames)
			}
		})
	}
}
