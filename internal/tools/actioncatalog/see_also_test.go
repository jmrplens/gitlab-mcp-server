package actioncatalog

import "testing"

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
