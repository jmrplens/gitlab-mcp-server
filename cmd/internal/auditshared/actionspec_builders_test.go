// actionspec_builders_test.go covers the buildXxxActionSpecs naming rule and
// the directory scan the manifest generator and the catalog-first auditor
// both rely on to agree on the set of catalog builders.
package auditshared

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIsActionSpecGroupBuilderName_Scenarios_MatchesConvention verifies only
// names of the form build<Domain>ActionSpecs with a non-empty domain count as
// builders.
func TestIsActionSpecGroupBuilderName_Scenarios_MatchesConvention(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "domain builder", in: "buildBranchActionSpecs", want: true},
		{name: "empty domain", in: "buildActionSpecs", want: false},
		{name: "wrong prefix", in: "makeBranchActionSpecs", want: false},
		{name: "wrong suffix", in: "buildBranchActionSpec", want: false},
		{name: "helper", in: "helper", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsActionSpecGroupBuilderName(tt.in); got != tt.want {
				t.Errorf("IsActionSpecGroupBuilderName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// writeBuilderFixture materializes files (slash-separated paths) under a
// fresh temporary directory and returns it.
func writeBuilderFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

// TestDiscoverActionSpecGroupBuilders_Scenarios_ScansTopLevelSources verifies
// the scan reads only the top-level non-test, non-generated Go files of the
// directory, returns builder names sorted, and fails on a duplicate builder,
// on a directory without builders, on a file that does not parse, and on a
// directory that does not exist.
func TestDiscoverActionSpecGroupBuilders_Scenarios_ScansTopLevelSources(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		missing bool
		want    []string
		wantErr string
		wantIs  error
	}{
		{
			name: "sorted names from top-level sources only",
			files: map[string]string{
				"zeta.go":             "package tools\n\nfunc buildZetaActionSpecs() {}\nfunc (x T) buildMethodActionSpecs() {}\n",
				"alpha.go":            "package tools\n\nfunc buildAlphaActionSpecs() {}\nfunc helper() {}\n",
				"alpha_test.go":       "package tools\n\nfunc buildTestActionSpecs() {}\n",
				"manifest_gen.go":     "package tools\n\nfunc buildGenActionSpecs() {}\n",
				"notes.txt":           "func buildTextActionSpecs() {}\n",
				"nested/nested.go":    "package nested\n\nfunc buildNestedActionSpecs() {}\n",
				"nested/deep/deep.go": "package deep\n\nfunc buildDeepActionSpecs() {}\n",
			},
			want: []string{"buildAlphaActionSpecs", "buildZetaActionSpecs"},
		},
		{
			name: "duplicate builder across files",
			files: map[string]string{
				"a.go": "package tools\n\nfunc buildSameActionSpecs() {}\n",
				"b.go": "package tools\n\nfunc buildSameActionSpecs() {}\n",
			},
			wantErr: "duplicate action spec group builder buildSameActionSpecs",
		},
		{
			name:    "no builders",
			files:   map[string]string{"a.go": "package tools\n\nfunc helper() {}\n"},
			wantErr: "no action spec group builders found",
		},
		{
			name:    "unparsable source",
			files:   map[string]string{"broken.go": "package tools\n\nfunc {\n"},
			wantErr: "parse ",
		},
		{
			name:    "missing directory",
			missing: true,
			// Asserted with errors.Is rather than by text: the text is the
			// operating system's, and Windows spells a missing path
			// differently from Linux.
			wantIs: fs.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeBuilderFixture(t, tt.files)
			if tt.missing {
				dir = filepath.Join(dir, "absent")
			}

			got, err := DiscoverActionSpecGroupBuilders(dir)
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("DiscoverActionSpecGroupBuilders() error = %v, want %v", err, tt.wantIs)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("DiscoverActionSpecGroupBuilders() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DiscoverActionSpecGroupBuilders() error = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("builders = %v, want %v", got, tt.want)
			}
		})
	}
}
