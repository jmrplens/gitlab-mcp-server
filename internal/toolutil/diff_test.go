package toolutil

import (
	"reflect"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// TestCapturedDiffs_EachDiffsFlags_AreReadInOrder verifies the two readers of
// the flags client-go's Diff drops: a list of file diffs, which is what a
// commit's diff answers with, and a comparison, whose diffs sit under diffs
// beside its commits. Each flag of each diff has a value of its own, so one
// read into another's field, or one diff's flags on the next, fails.
func TestCapturedDiffs_EachDiffsFlags_AreReadInOrder(t *testing.T) {
	want := []DiffExtra{{Collapsed: true}, {TooLarge: true}, {GeneratedFile: true}}
	list := `[{"collapsed":true},{"too_large":true},{"generated_file":true}]`

	got, err := CapturedDiffs(gitlabclient.CapturedBody([]byte(list)), 3)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CapturedDiffs() = %+v, %v; want %+v", got, err, want)
	}
	got, err = CapturedCompareDiffs(gitlabclient.CapturedBody([]byte(`{"commits":[],"diffs":`+list+`}`)), 3)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CapturedCompareDiffs() = %+v, %v; want %+v", got, err, want)
	}
}

// TestDiffOutputStatus_EveryChangeAndFlag_IsNamed verifies what a file's row
// says: which of the four changes it was, and, when GitLab sent no diff text
// or marked the file generated, why, each note in a fixed order.
func TestDiffOutputStatus_EveryChangeAndFlag_IsNamed(t *testing.T) {
	cases := []struct {
		name string
		diff DiffOutput
		want string
	}{
		{name: "a modified file", want: "modified"},
		{name: "an added file", diff: DiffOutput{NewFile: true}, want: "added"},
		{name: "a deleted file", diff: DiffOutput{DeletedFile: true}, want: "deleted"},
		{name: "a renamed file", diff: DiffOutput{RenamedFile: true}, want: "renamed"},
		{name: "a collapsed diff", diff: DiffOutput{Collapsed: true}, want: "modified (collapsed)"},
		{name: "a diff too large to send", diff: DiffOutput{NewFile: true, TooLarge: true}, want: "added (too large)"},
		{name: "a generated file", diff: DiffOutput{GeneratedFile: true}, want: "modified (generated)"},
		{name: "every flag at once", diff: DiffOutput{Collapsed: true, TooLarge: true, GeneratedFile: true}, want: "modified (collapsed, too large, generated)"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.diff.Status(); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCapturedDiffs_AnAnswerTheFlagsCannotBePairedWith_IsAnError verifies that
// the flags are never put on the wrong diff: an answer holding a different
// number of diffs than the SDK decoded, or one that does not decode into the
// flags' shape, is an error rather than a list whose flags are shifted.
func TestCapturedDiffs_AnAnswerTheFlagsCannotBePairedWith_IsAnError(t *testing.T) {
	cases := []struct {
		name string
		read func() error
	}{
		{name: "a list shorter than the SDK's", read: func() error {
			_, err := CapturedDiffs(gitlabclient.CapturedBody([]byte(`[{}]`)), 2)
			return err
		}},
		{name: "a comparison shorter than the SDK's", read: func() error {
			_, err := CapturedCompareDiffs(gitlabclient.CapturedBody([]byte(`{"diffs":[{}]}`)), 2)
			return err
		}},
		{name: "a comparison whose flag is not a boolean", read: func() error {
			_, err := CapturedCompareDiffs(gitlabclient.CapturedBody([]byte(`{"diffs":[{"collapsed":"yes"}]}`)), 1)
			return err
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.read(); err == nil {
				t.Error("read = nil error, want one")
			}
		})
	}
}
