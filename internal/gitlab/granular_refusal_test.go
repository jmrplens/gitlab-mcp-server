// granular_refusal_test.go holds the reader of GitLab's fine-grained refusal
// sentences to the texts authorize_granular_scopes_service.rb writes at
// v19.4.1-ee, and to nothing else.
package gitlab

import (
	"reflect"
	"strings"
	"testing"
)

// The sentences GitLab's fine-grained authorization writes, as a personal
// access token is refused them (token.class.model_name.human.downcase, and
// .pluralize where the sentence names the class of tokens).
const (
	missingMetadataSentence   = "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read]."
	missingTwoSentence        = "Access denied: This operation requires a fine-grained personal access token with the following project permissions: [Issue: Create, Merge Request: Approve]."
	missingBulkImportSentence = "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Bulk Import: Read]."
	unsupportedSentence       = "Access denied: This operation doesn't support fine-grained personal access tokens."
	disabledSentence          = "Access denied: Fine-grained personal access tokens are not yet supported."
)

// TestParseGranularRefusal_GitLabsSentences_AreReadIntoTheirParts verifies
// each of the three sentences GitLab writes, and that the parts come back as
// GitLab wrote them: the token class singular in the missing-permission
// sentence and plural in the other two, the boundary label, and the listed
// permissions in their order. The bulk-import row is a permission whose first
// assignable match is deprecated (read_bulk_import_entity resolves to the
// Bulk Import group before the Import one in GitLab's file order), and it is
// read like any other: the reader holds GitLab's words, not the ones the token
// page offers today. The personal projects row is a boundary label of two
// words, which is what GitLab's PersonalProjectsBoundary writes; the padded row
// is a sentence carried with the white space a transport added around it.
func TestParseGranularRefusal_GitLabsSentences_AreReadIntoTheirParts(t *testing.T) {
	tests := []struct {
		name     string
		sentence string
		want     GranularRefusal
	}{
		{
			name:     "one missing permission",
			sentence: missingMetadataSentence,
			want:     GranularRefusal{Kind: GranularRefusalMissingPermissions, TokenType: "personal access token", Boundary: "instance", Permissions: []string{"Metadata: Read"}},
		},
		{
			name:     "two missing permissions",
			sentence: missingTwoSentence,
			want:     GranularRefusal{Kind: GranularRefusalMissingPermissions, TokenType: "personal access token", Boundary: "project", Permissions: []string{"Issue: Create", "Merge Request: Approve"}},
		},
		{
			name:     "a deprecated first match",
			sentence: missingBulkImportSentence,
			want:     GranularRefusal{Kind: GranularRefusalMissingPermissions, TokenType: "personal access token", Boundary: "instance", Permissions: []string{"Bulk Import: Read"}},
		},
		{
			name:     "a two-word boundary",
			sentence: "Access denied: This operation requires a fine-grained personal access token with the following personal projects permissions: [CI/CD Setting: Update].",
			want:     GranularRefusal{Kind: GranularRefusalMissingPermissions, TokenType: "personal access token", Boundary: "personal projects", Permissions: []string{"CI/CD Setting: Update"}},
		},
		{
			name:     "operation not supported",
			sentence: unsupportedSentence,
			want:     GranularRefusal{Kind: GranularRefusalUnsupported, TokenType: "personal access tokens"},
		},
		{
			name:     "not yet supported",
			sentence: disabledSentence,
			want:     GranularRefusal{Kind: GranularRefusalDisabled, TokenType: "personal access tokens"},
		},
		{
			name:     "padded",
			sentence: "  " + disabledSentence + "\n",
			want:     GranularRefusal{Kind: GranularRefusalDisabled, TokenType: "personal access tokens"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseGranularRefusal(tt.sentence); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseGranularRefusal(%q) = %+v, want %+v", tt.sentence, got, tt.want)
			}
		})
	}
}

// TestParseGranularRefusal_AnythingElse_IsUnrecognized is the negative half:
// every way a text can fall short of one of GitLab's sentences, each one step
// away from a sentence that parses, so a reader loosened in any part reads one
// of them. A sentence quoted inside a longer text is among them, because a
// validation error echoing a title the caller chose is not GitLab refusing the
// call; so is "404 Not Found", which is GitLab's ordinary 404 wherever it is not
// a GraphQL errors[] entry.
func TestParseGranularRefusal_AnythingElse_IsUnrecognized(t *testing.T) {
	long := strings.Repeat("a", granularLabelMaxBytes+1)
	tests := map[string]string{
		"empty":                                   "",
		"GitLab's generic GraphQL refusal":        "The resource that you are attempting to access does not exist or you don't have permission to perform this action",
		"GitLab's ordinary not found":             GranularNotFound,
		"a sentence quoted inside another":        "Title " + unsupportedSentence + " is invalid",
		"a sentence with text after it":           missingMetadataSentence + " Ask an administrator.",
		"no boundary clause":                      "Access denied: This operation requires a fine-grained personal access token.",
		"no permission list":                      "Access denied: This operation requires a fine-grained personal access token with the following instance permissions.",
		"an unclosed list":                        "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read",
		"an empty list":                           "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [].",
		"an empty permission":                     "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read, ].",
		"a blank permission":                      "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [ ].",
		"an upper-case token type":                "Access denied: This operation requires a fine-grained Personal access token with the following instance permissions: [Metadata: Read].",
		"a token type with a byte past z":         "Access denied: This operation requires a fine-grained personal{access token with the following instance permissions: [Metadata: Read].",
		"an empty token type":                     "Access denied: This operation requires a fine-grained  with the following instance permissions: [Metadata: Read].",
		"a token type with a double space":        "Access denied: This operation requires a fine-grained personal  access token with the following instance permissions: [Metadata: Read].",
		"a token type past the bound":             "Access denied: This operation requires a fine-grained " + long + " with the following instance permissions: [Metadata: Read].",
		"a boundary with a digit":                 "Access denied: This operation requires a fine-grained personal access token with the following project1 permissions: [Metadata: Read].",
		"an empty boundary":                       "Access denied: This operation requires a fine-grained personal access token with the following  permissions: [Metadata: Read].",
		"a boundary past the bound":               "Access denied: This operation requires a fine-grained personal access token with the following " + long + " permissions: [Metadata: Read].",
		"not supported with no full stop":         "Access denied: This operation doesn't support fine-grained personal access tokens",
		"not supported naming nothing":            "Access denied: This operation doesn't support fine-grained .",
		"not supported with an upper-case class":  "Access denied: This operation doesn't support fine-grained Personal access tokens.",
		"not yet supported without its ending":    "Access denied: Fine-grained personal access tokens are not supported.",
		"not yet supported naming nothing":        "Access denied: Fine-grained  are not yet supported.",
		"not yet supported with a trailing space": "Access denied: Fine-grained personal access tokens  are not yet supported.",
	}
	for name, sentence := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ParseGranularRefusal(sentence); !reflect.DeepEqual(got, GranularRefusal{}) {
				t.Errorf("ParseGranularRefusal(%q) = %+v, want the zero refusal", sentence, got)
			}
		})
	}
}

// TestParseGranularRefusal_LabelsAtTheirEdges_AreRead pins the edges of a
// label from the side a mutant of each comparison would cross: a token type
// exactly [granularLabelMaxBytes] long is a label (one byte more is not, in the
// negative half above), and so is one made of the last lower-case letter,
// beside the byte past it that is not.
func TestParseGranularRefusal_LabelsAtTheirEdges_AreRead(t *testing.T) {
	tests := map[string]string{
		"at the bound":           strings.Repeat("a", granularLabelMaxBytes),
		"the last lower letters": "z y",
	}
	for name, label := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseGranularRefusal("Access denied: Fine-grained " + label + " are not yet supported.")
			if got.Kind != GranularRefusalDisabled || got.TokenType != label {
				t.Errorf("the token type %q = %+v, want it read as the disabled sentence's class", label, got)
			}
		})
	}
}

// TestParseGraphQLGranularRefusal_NotFound_IsTheServicesFourthAnswer verifies
// that a GraphQL errors[] entry reading exactly "404 Not Found" is the
// service's not-found answer, padded or not, and that the three sentences and
// everything else are read as [ParseGranularRefusal] reads them.
func TestParseGraphQLGranularRefusal_NotFound_IsTheServicesFourthAnswer(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    GranularRefusalKind
	}{
		{name: "not found", message: GranularNotFound, want: GranularRefusalNotFound},
		{name: "not found padded", message: " " + GranularNotFound + " ", want: GranularRefusalNotFound},
		{name: "a missing permission", message: missingTwoSentence, want: GranularRefusalMissingPermissions},
		{name: "operation not supported", message: unsupportedSentence, want: GranularRefusalUnsupported},
		{name: "not yet supported", message: disabledSentence, want: GranularRefusalDisabled},
		{name: "not found inside a sentence", message: "Project " + GranularNotFound, want: GranularRefusalUnrecognized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseGraphQLGranularRefusal(tt.message).Kind; got != tt.want {
				t.Errorf("ParseGraphQLGranularRefusal(%q).Kind = %d, want %d", tt.message, got, tt.want)
			}
		})
	}
}
