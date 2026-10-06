package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// listCall is one call a fake lister received.
type listCall struct {
	dir  string
	env  []string
	args []string
}

// fakeLister answers each target with the listing keyed by its GOOS/GOARCH
// pair, records every call, and fails the target named in failOn.
func fakeLister(listings map[string]string, failOn string, calls *[]listCall) lister {
	return func(_ context.Context, dir string, env []string, args ...string) ([]byte, error) {
		*calls = append(*calls, listCall{dir: dir, env: env, args: args})
		target := strings.TrimPrefix(env[0], "GOOS=") + "/" + strings.TrimPrefix(env[1], "GOARCH=")
		if target == failOn {
			return nil, errors.New("planted failure")
		}
		return []byte(listings[target]), nil
	}
}

// row writes one listing row the way listFormat does.
func row(path, module string, files ...string) string {
	return strings.Join(append([]string{path, module}, files...), "\t") + "\n"
}

// TestConstrainedPackages_Listings_NameThePackagesWhoseFilesDiffer verifies the
// rule: a package is constrained when the files the go command builds for it
// differ between two release targets, in any of the four lists and whichever
// target differs, including only the last; the answer is sorted and written
// ./ and its directory; and every listing is asked for the whole module under
// its own GOOS, GOARCH and cgo, in the directory given.
func TestConstrainedPackages_Listings_NameThePackagesWhoseFilesDiffer(t *testing.T) {
	t.Parallel()
	const module = "example.com/m"
	same := row(module+"/same", module, "a.go", "", "a_test.go", "")
	listings := map[string]string{}
	for _, target := range releaseTargets {
		windows := strings.HasPrefix(target, "windows/")
		listing := same
		if windows {
			listing += row(module+"/zeta", module, "z.go z_windows.go", "", "", "")
		} else {
			listing += row(module+"/zeta", module, "z.go", "", "", "")
		}
		lastOnly := "x.go"
		if target == "windows/arm64" {
			lastOnly = "x.go x_arm64.go"
		}
		listing += row(module+"/alpha/last", module, lastOnly, "", "", "")
		testOnly := "t_unix_test.go"
		if windows {
			testOnly = ""
		}
		listing += "\r\n" + row(module+"/tests", module, "t.go", "", testOnly, "")
		listings[target] = listing
	}
	var calls []listCall
	got, err := constrainedPackages(context.Background(), "the/module", fakeLister(listings, "", &calls))
	if err != nil {
		t.Fatalf("constrainedPackages() error = %v", err)
	}
	want := []string{"./alpha/last", "./tests", "./zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("constrainedPackages() = %q, want %q", got, want)
	}
	if len(calls) != len(releaseTargets) {
		t.Fatalf("lister called %d times, want %d", len(calls), len(releaseTargets))
	}
	for i, call := range calls {
		goos, goarch, _ := strings.Cut(releaseTargets[i], "/")
		wantEnv := []string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=1"}
		wantArgs := []string{"list", "-e", "-f", listFormat, "./..."}
		if call.dir != "the/module" || !slices.Equal(call.env, wantEnv) || !slices.Equal(call.args, wantArgs) {
			t.Errorf("call %d = %+v, want dir the/module, env %q, args %q", i, call, wantEnv, wantArgs)
		}
	}
}

// TestConstrainedPackages_Failures_AreReturnedWithTheTarget verifies that a
// listing the toolchain refused, a row of another shape, and a constrained
// package that cannot be named below its module each end the search with an
// error naming what went wrong.
func TestConstrainedPackages_Failures_AreReturnedWithTheTarget(t *testing.T) {
	t.Parallel()
	const module = "example.com/m"
	everywhere := func(listing string) map[string]string {
		listings := map[string]string{}
		for _, target := range releaseTargets {
			listings[target] = listing
		}
		return listings
	}
	rootVaries := everywhere(row(module, module, "r.go", "", "", ""))
	rootVaries["darwin/arm64"] = row(module, module, "r.go r_darwin.go", "", "", "")
	cases := []struct {
		name     string
		listings map[string]string
		failOn   string
		want     string
	}{
		{
			name: "the toolchain refused", listings: everywhere(""), failOn: "darwin/amd64",
			want: "go list for darwin/amd64: planted failure",
		},
		{
			name: "a row of another shape", listings: everywhere("only\ttwo\n"),
			want: `go list for linux/amd64: unexpected go list row "only\ttwo"`,
		},
		{
			name: "the module root varies", listings: rootVaries,
			want: `go list named "example.com/m", which is not below its module path "example.com/m"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []listCall
			got, err := constrainedPackages(context.Background(), ".", fakeLister(tc.listings, tc.failOn, &calls))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("constrainedPackages() = %q, %v; want error %q", got, err, tc.want)
			}
		})
	}
}

// TestRelativePackage_ModulePaths_AreCutAtASegment verifies that a package is
// named below its module only at a whole path segment: a module whose path is
// a prefix of another's does not claim the other's packages.
func TestRelativePackage_ModulePaths_AreCutAtASegment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		pkg     listedPackage
		want    string
		wantErr bool
	}{
		{name: "below", pkg: listedPackage{path: "example.com/m/cmd/server", module: "example.com/m"}, want: "./cmd/server"},
		{name: "a sibling module", pkg: listedPackage{path: "example.com/m2/cmd", module: "example.com/m"}, wantErr: true},
		{name: "no module", pkg: listedPackage{path: "example.com/m/cmd", module: ""}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := relativePackage(tc.pkg)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("relativePackage(%+v) = %q, %v; want %q, error %t", tc.pkg, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

// TestConstrainedPackages_FixtureModule_ListsWithTheRealToolchain verifies
// the rule against the go command itself, over a module holding a package
// split by a _windows.go name, one split by an _arm64.go name, one whose test
// file carries a !windows constraint, one with a file behind the race tag
// alone and one with no constrained file: the first three are named and the
// last two are not.
func TestConstrainedPackages_FixtureModule_ListsWithTheRealToolchain(t *testing.T) {
	t.Parallel()
	got, err := constrainedPackages(context.Background(), filepath.Join("testdata", "platforms"), goList)
	if err != nil {
		t.Fatalf("constrainedPackages() error = %v", err)
	}
	want := []string{"./arch", "./split", "./testonly"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("constrainedPackages() = %q, want %q", got, want)
	}
}

// TestGoList_MissingDirectory_ReportsTheToolchainsWords verifies that a go
// command that failed is returned as an error carrying what it wrote to
// stderr, so the plan says why it could not list rather than only that.
func TestGoList_MissingDirectory_ReportsTheToolchainsWords(t *testing.T) {
	t.Parallel()
	_, err := goList(context.Background(), ".", nil, "list", "./nosuchpackagehere")
	if err == nil || !strings.Contains(err.Error(), "nosuchpackagehere") {
		t.Errorf("goList() error = %v, want one naming the missing package", err)
	}
}

// TestParseListing_Rows_AreReadOrRefused verifies the listing reader: blank
// lines and carriage returns are dropped, the four file lists are kept as one
// signature, and a row with another number of fields is refused.
func TestParseListing_Rows_AreReadOrRefused(t *testing.T) {
	t.Parallel()
	rows, err := parseListing([]byte("\np\tm\ta.go\t\tb_test.go\tc_test.go\r\n\n"))
	if err != nil {
		t.Fatalf("parseListing() error = %v", err)
	}
	want := []listedPackage{{path: "p", module: "m", files: "a.go\t\tb_test.go\tc_test.go"}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("parseListing() = %+v, want %+v", rows, want)
	}
	for _, malformed := range []string{"a\tb\tc\td\te\n", "a\tb\tc\td\te\tf\tg\n"} {
		t.Run(malformed, func(t *testing.T) {
			t.Parallel()
			if _, refused := parseListing([]byte(malformed)); refused == nil {
				t.Errorf("parseListing(%q) error = nil, want a refusal", malformed)
			}
		})
	}
}

// TestReleaseTargets_GoReleaser_AreTheTargetsTheReleaseBuilds holds the list
// the discovery reads every package under to the goos crossed with the goarch
// of every build .goreleaser.yml declares, so a platform the release gains is
// one the discovery looks at.
func TestReleaseTargets_GoReleaser_AreTheTargetsTheReleaseBuilds(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}
	var config struct {
		Builds []struct {
			Goos   []string `yaml:"goos"`
			Goarch []string `yaml:"goarch"`
		} `yaml:"builds"`
	}
	if err = yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse .goreleaser.yml: %v", err)
	}
	built := map[string]bool{}
	for _, build := range config.Builds {
		for _, goos := range build.Goos {
			for _, goarch := range build.Goarch {
				built[goos+"/"+goarch] = true
			}
		}
	}
	listed := map[string]bool{}
	for _, target := range releaseTargets {
		listed[target] = true
	}
	if len(built) == 0 || !reflect.DeepEqual(built, listed) {
		t.Errorf("releaseTargets = %q, want the targets .goreleaser.yml builds: %v", releaseTargets, built)
	}
}
