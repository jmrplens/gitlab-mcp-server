// recipe_test.go covers how a configuration becomes the go command line,
// the environment and the names of each binary: GoReleaser's templates
// evaluated with the fields this command knows, an override's ldflags in
// place of the build's, and the file names an archive publishes.
package main

import (
	"reflect"
	"strings"
	"testing"
)

// fixtureFields are the values a release of the fixture at a made-up commit
// renders its templates with.
var fixtureFields = fields{
	Version:     "1.2.3",
	Tag:         "v1.2.3",
	Commit:      strings.Repeat("c", 40),
	FullCommit:  strings.Repeat("c", 40),
	ShortCommit: "cccccccc",
}

// TestRender_EvaluatesTheFieldsItKnows covers each field a template may name,
// spelled the way GoReleaser's documentation spells them.
func TestRender_EvaluatesTheFieldsItKnows(t *testing.T) {
	t.Parallel()

	f := fixtureFields
	f.Binary, f.Os, f.Arch = "fixture", "linux", "amd64"
	for _, tc := range []struct {
		text, want string
	}{
		{text: "plain", want: "plain"},
		{text: "{{.Version}} {{ .Tag }}", want: "1.2.3 v1.2.3"},
		{text: "{{.Commit}}/{{.FullCommit}}/{{.ShortCommit}}", want: f.Commit + "/" + f.FullCommit + "/cccccccc"},
		{text: "{{ .Binary }}-{{ .Os }}-{{ .Arch }}", want: "fixture-linux-amd64"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()

			got, err := render(tc.text, f)
			if err != nil || got != tc.want {
				t.Errorf("render(%q) = %q, %v, want %q", tc.text, got, err, tc.want)
			}
		})
	}
}

// TestRender_RefusesATemplateItCannotEvaluate covers the three ways a
// template can name what this command does not hold: a field GoReleaser has
// and this command does not, a function, and text that is no template.
func TestRender_RefusesATemplateItCannotEvaluate(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"{{ .ProjectName }}", `{{ trimprefix .Tag "v" }}`, "{{ .Version"} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			if got, err := render(text, fixtureFields); err == nil {
				t.Errorf("render(%q) = %q, want an error", text, got)
			}
		})
	}
}

// TestRenderEach_DropsWhatRendersEmpty covers GoReleaser's rule for a list:
// an entry that renders empty is left out rather than passed as an empty
// argument, and an entry that does not render is named in the error.
func TestRenderEach_DropsWhatRendersEmpty(t *testing.T) {
	t.Parallel()

	got, err := renderEach([]string{"-s", "{{ if .Os }}{{ end }}", "-X v={{.Version}}"}, fixtureFields)
	if err != nil || !reflect.DeepEqual(got, []string{"-s", "-X v=1.2.3"}) {
		t.Errorf("renderEach = %q, %v", got, err)
	}
	if _, failed := renderEach([]string{"ok", "{{ .Nope }}"}, fixtureFields); failed == nil || !strings.Contains(failed.Error(), `"{{ .Nope }}"`) {
		t.Errorf("renderEach = %v, want an error naming the entry", failed)
	}
}

// TestRelease_Recipes_EachTargetAsGoReleaserBuildsIt reads the fixture
// configuration into recipes and holds three of them to what GoReleaser runs:
// a Linux target taking its override's ldflags, a Windows target taking the
// build's own with the .exe GoReleaser appends, and the universal binary
// joining the two darwin builds.
func TestRelease_Recipes_EachTargetAsGoReleaserBuildsIt(t *testing.T) {
	t.Parallel()

	rel, err := readRelease(writeConfig(t, fixtureConfig))
	if err != nil {
		t.Fatalf("readRelease: %v", err)
	}
	thin, fat, err := rel.recipes(fixtureFields)
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	if len(thin) != 6 || len(fat) != 1 {
		t.Fatalf("recipes = %d binaries and %d universal ones, want 6 and 1", len(thin), len(fat))
	}
	stamp := "-s -w -X main.version=1.2.3 -X main.commit=" + fixtureFields.FullCommit
	linux := recipe{
		build: "fixture", target: target{goos: "linux", goarch: "amd64"}, asset: "fixture-linux-amd64",
		dir: "fixture_linux_amd64_v1", file: "fixture", main: "./cmd/fixture", env: []string{"CGO_ENABLED=0"},
		flags: []string{"-trimpath", "-buildmode=pie", "-ldflags=" + stamp + " -I /lib64/ld-linux-x86-64.so.2"},
	}
	windows := recipe{
		build: "fixture", target: target{goos: "windows", goarch: "arm64"}, asset: "fixture-windows-arm64.exe",
		dir: "fixture_windows_arm64_v8.0", file: "fixture.exe", main: "./cmd/fixture", env: []string{"CGO_ENABLED=0"},
		flags: []string{"-trimpath", "-buildmode=pie", "-ldflags=" + stamp},
	}
	for i, want := range map[int]recipe{0: linux, 5: windows} {
		t.Run(want.asset, func(t *testing.T) {
			t.Parallel()

			if !reflect.DeepEqual(thin[i], want) {
				t.Errorf("recipe %d =\n%#v\nwant\n%#v", i, thin[i], want)
			}
		})
	}
	wantFat := fatRecipe{asset: "fixture-darwin-all", dir: "fixture-universal_darwin_all", file: "fixture", slices: []recipe{thin[2], thin[3]}}
	if !reflect.DeepEqual(fat[0], wantFat) {
		t.Errorf("universal recipe =\n%#v\nwant\n%#v", fat[0], wantFat)
	}
}

// TestBuild_LinkerFlagsFor covers which ldflags a target is built with: the
// first matching override's, unless it sets none, and the build's own when no
// override matches.
func TestBuild_LinkerFlagsFor(t *testing.T) {
	t.Parallel()

	b := build{ldflags: []string{"base"}, overrides: []override{
		{goos: "linux", goarch: "amd64"},
		{goos: "linux", goarch: "arm64", ldflags: []string{"first"}},
		{goos: "linux", goarch: "arm64", ldflags: []string{"second"}},
	}}
	for _, tc := range []struct {
		target target
		want   string
	}{
		{target: target{goos: "linux", goarch: "amd64"}, want: "base"},
		{target: target{goos: "linux", goarch: "arm64"}, want: "first"},
		{target: target{goos: "darwin", goarch: "arm64"}, want: "base"},
	} {
		t.Run(tc.target.String(), func(t *testing.T) {
			t.Parallel()

			if got := b.linkerFlagsFor(tc.target); !reflect.DeepEqual(got, []string{tc.want}) {
				t.Errorf("linkerFlagsFor(%s) = %q, want [%s]", tc.target, got, tc.want)
			}
		})
	}
}

// TestBuild_Recipe_LdflagsThatRenderEmptyAreNotPassed covers GoReleaser's
// rule for the linker flags: when none render, no -ldflags argument is
// passed at all, rather than an empty one.
func TestBuild_Recipe_LdflagsThatRenderEmptyAreNotPassed(t *testing.T) {
	t.Parallel()

	b := build{id: "fixture", main: ".", binary: "fixture", flags: []string{"-trimpath"}, ldflags: []string{"{{ if .Os }}{{ end }}"}}
	rc, err := b.recipe(target{goos: "linux", goarch: "amd64"}, fixtureFields, "fixture", "{{ .Binary }}")
	if err != nil || !reflect.DeepEqual(rc.flags, []string{"-trimpath"}) {
		t.Errorf("recipe flags = %q, %v, want the build's flags alone", rc.flags, err)
	}
}

// TestRecipe_Arguments covers the go command line: build, the entry's flags
// and its -ldflags, the output and the main package last, which is the order
// GoReleaser writes them in.
func TestRecipe_Arguments(t *testing.T) {
	t.Parallel()

	rc := recipe{flags: []string{"-trimpath", "-ldflags=-s"}, main: "./cmd/x"}
	want := []string{"build", "-trimpath", "-ldflags=-s", "-o", "/out/x", "./cmd/x"}
	if got := rc.arguments("/out/x"); !reflect.DeepEqual(got, want) {
		t.Errorf("arguments = %q, want %q", got, want)
	}
}

// TestRelease_Recipes_ATemplateItCannotEvaluate covers each template a recipe
// renders failing, and a universal binary left with no darwin build to join.
// Each error names the build or the universal binary, and what failed.
func TestRelease_Recipes_ATemplateItCannotEvaluate(t *testing.T) {
	t.Parallel()

	good := func() release {
		return release{
			builds: []build{{
				id: "fixture", main: ".", binary: "fixture", env: []string{"A=1"}, flags: []string{"-trimpath"},
				ldflags: []string{"-s"}, targets: []target{{goos: "darwin", goarch: "arm64"}},
			}},
			universals: []universal{{id: "u", name: "fixture", ids: []string{"fixture"}}},
			archive:    "{{ .Binary }}-{{ .Os }}-{{ .Arch }}",
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*release)
		want   string
	}{
		{name: "binary", change: func(r *release) { r.builds[0].binary = "{{ .Nope }}" }, want: "build fixture: binary"},
		{name: "flags", change: func(r *release) { r.builds[0].flags = []string{"{{ .Nope }}"} }, want: "build fixture for darwin/arm64: flags"},
		{name: "env", change: func(r *release) { r.builds[0].env = []string{"{{ .Nope }}"} }, want: "build fixture for darwin/arm64: env"},
		{name: "ldflags", change: func(r *release) { r.builds[0].ldflags = []string{"{{ .Nope }}"} }, want: "build fixture for darwin/arm64: ldflags"},
		{name: "archive", change: func(r *release) { r.archive = "{{ .Nope }}" }, want: "build fixture for darwin/arm64: archive name_template"},
		{name: "universal name", change: func(r *release) { r.universals[0].name = "{{ .Nope }}" }, want: "universal binary u: name_template"},
		{
			name:   "universal archive name",
			change: func(r *release) { r.archive = `{{ if eq .Arch "all" }}{{ .Nope }}{{ end }}x` },
			want:   "universal binary u: archive name_template",
		},
		{name: "no darwin build", change: func(r *release) { r.builds[0].targets = []target{{goos: "linux", goarch: "arm64"}} }, want: "joins no darwin binary"},
		{name: "another build's darwin binaries", change: func(r *release) { r.universals[0].ids = []string{"other"} }, want: "joins no darwin binary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rel := good()
			tc.change(&rel)
			if _, _, err := rel.recipes(fixtureFields); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("recipes = %v, want an error containing %q", err, tc.want)
			}
		})
	}
	if _, _, err := good().recipes(fixtureFields); err != nil {
		t.Errorf("the unchanged release does not render: %v", err)
	}
}
