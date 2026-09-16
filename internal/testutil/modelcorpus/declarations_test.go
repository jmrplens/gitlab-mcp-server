package modelcorpus

import "testing"

// TestDeclarations_EachOneIsWellFormed refuses a declaration the gate could
// never act on: one naming a category that does not exist, or giving no reason
// for a reader to disagree with.
func TestDeclarations_EachOneIsWellFormed(t *testing.T) {
	for _, declaration := range declaredPromptFindings {
		t.Run(declaration.key(), func(t *testing.T) {
			if problem := promptDeclarationProblem(declaration); problem != "" {
				t.Error(problem)
			}
		})
	}
}

// TestDeclarationProblem_NamesWhatIsWrong covers the shapes the table itself
// does not contain, which is the only way to know the check would catch them.
func TestDeclarationProblem_NamesWhatIsWrong(t *testing.T) {
	sound := promptDeclaration{
		Case:     "MT-001",
		Value:    "project_id",
		Category: promptCategoryLiteralIsTheRequest,
		Reason:   "because",
	}
	tests := []struct {
		name        string
		declaration promptDeclaration
		want        string
	}{
		{name: "sound", declaration: sound, want: ""},
		{
			name:        "no case",
			declaration: promptDeclaration{Value: "project_id", Category: sound.Category, Reason: "because"},
			want:        " project_id: names no case or no literal",
		},
		{
			name:        "no literal",
			declaration: promptDeclaration{Case: "MT-001", Category: sound.Category, Reason: "because"},
			want:        "MT-001 : names no case or no literal",
		},
		{
			name: "invented category",
			declaration: promptDeclaration{
				Case: "MT-001", Value: "project_id", Category: "it-seemed-fine", Reason: "because",
			},
			want: "MT-001 project_id: category it-seemed-fine is not one of the declared kinds",
		},
		{
			name:        "no reason",
			declaration: promptDeclaration{Case: "MT-001", Value: "project_id", Category: sound.Category},
			want:        "MT-001 project_id: gives no reason",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := promptDeclarationProblem(tc.declaration); got != tc.want {
				t.Errorf("promptDeclarationProblem() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDeclarationCovers_MatchesOnBothHalvesOfTheKey pins what a declaration
// answers. Both halves matter: one that matched on the case alone would excuse
// every finding in a case that has one declared literal, which is the whole of
// what the table is meant to hold apart.
func TestDeclarationCovers_MatchesOnBothHalvesOfTheKey(t *testing.T) {
	declaration := promptDeclaration{Case: "MS-001", Value: "remote_url"}
	tests := []struct {
		name   string
		caseID string
		value  string
		want   bool
	}{
		{name: "both halves", caseID: "MS-001", value: "remote_url", want: true},
		{name: "another literal of the same case", caseID: "MS-001", value: "project_id", want: false},
		{name: "the same literal in another case", caseID: "MS-002", value: "remote_url", want: false},
		{name: "neither", caseID: "MT-001", value: "project_id", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaration.covers(tc.caseID, tc.value); got != tc.want {
				t.Errorf("covers(%q, %q) = %t, want %t", tc.caseID, tc.value, got, tc.want)
			}
		})
	}
}

// TestDeclarations_EachOneAnswersALiveFinding is the staleness half. A
// declaration that matches nothing is either a case that was rewritten and
// took its finding with it, or a rule that stopped reporting what it used to,
// and both are things a reader of this table must be told about rather than
// left to trust.
func TestDeclarations_EachOneAnswersALiveFinding(t *testing.T) {
	findings := auditCorpus(t)
	for _, declaration := range declaredPromptFindings {
		t.Run(declaration.key(), func(t *testing.T) {
			for _, finding := range findings {
				if finding.Site == auditSiteCase && declaration.covers(finding.Case, finding.Value) {
					return
				}
			}
			t.Error("answers no finding the audit reports; the case was rewritten or the rule changed")
		})
	}
}

// TestDeclarations_CoverNothingTwice keeps one finding from being answered by
// two entries, which is how a table comes to hold a reason nobody reads.
func TestDeclarations_CoverNothingTwice(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range declaredPromptFindings {
		if seen[declaration.key()] {
			t.Errorf("%s is declared twice", declaration.key())
		}
		seen[declaration.key()] = true
	}
}
