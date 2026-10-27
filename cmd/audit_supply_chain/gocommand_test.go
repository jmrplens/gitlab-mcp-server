// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Tests for the Go-command demands of rule 2: what a credentialed job builds
// with the go command is this module's code and the standard library's alone,
// since any code the job runs holds the job's credential.
package main

import (
	"strings"
	"testing"
)

// goGraphReason is the half every finding about another module's code carries
// after its colon.
const goGraphReason = "any code a credentialed job runs holds the job's credential, " +
	"so a Go command run there is built from this module and the standard library alone"

// goRunModuleFinding is the pattern and reason the go-run-module rule prints
// after "matches ".
const goRunModuleFinding = `'\\bgo\\s+run\\b[^\\n]*@': go run of a package at a version builds a module this ` +
	`checkout does not hold, which the job fetches when it runs (build the command from this module, or hand the ` +
	`job a binary held to a recorded SHA-256)`

// dirtyImports is how a finding lists the two imports internal/yamlish makes
// outside the module.
const dirtyImports = "go.yaml.in/yaml/v3 (from internal/yamlish), golang.org/x/mod/semver (from internal/yamlish)"

// goModuleRoot builds the module the Go-command fixtures are read from: a
// command that stays inside the module and the standard library, one that
// reaches two other modules through a package of its own, one whose test file
// alone reaches another, a directory whose files differ, one that does not
// parse, one importing a directory that is not there, one with no Go file and
// one reaching four other modules. A file whose name starts with an underscore
// or a dot is one go ignores, and so does this audit.
func goModuleRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	files := map[string]string{
		"go.mod":                       "module example.test/m\n\ngo 1.27\n",
		"cmd/clean/main.go":            "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.test/m/internal/lib\"\n)\n\nfunc main() { fmt.Println(lib.Name) }\n",
		"cmd/dirty/main.go":            "package main\n\nimport _ \"example.test/m/internal/yamlish\"\n\nfunc main() {}\n",
		"cmd/tested/main.go":           "package main\n\nfunc main() {}\n",
		"cmd/tested/main_test.go":      "package main\n\nimport (\n\t\"testing\"\n\n\t\"github.com/google/go-cmp/cmp\"\n)\n\nfunc TestX(t *testing.T) { _ = cmp.Diff }\n",
		"cmd/files/tool.go":            "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(0) }\n",
		"cmd/files/other.go":           "package main\n\nimport _ \"example.test/m/internal/yamlish\"\n",
		"cmd/broken/main.go":           "package main\n\nimport (\n",
		"cmd/gap/main.go":              "package main\n\nimport _ \"example.test/m/internal/nothere\"\n\nfunc main() {}\n",
		"cmd/nothing/README.md":        "nothing to build\n",
		"cmd/many/main.go":             "package main\n\nimport (\n\t_ \"a.test/one\"\n\t_ \"b.test/two\"\n\t_ \"c.test/three\"\n\t_ \"d.test/four\"\n)\n\nfunc main() {}\n",
		"cmd/three/main.go":            "package main\n\nimport (\n\t_ \"a.test/one\"\n\t_ \"b.test/two\"\n\t_ \"c.test/three\"\n)\n\nfunc main() {}\n",
		"internal/lib/lib.go":          "package lib\n\nimport \"strings\"\n\n// Name is a name.\nvar Name = strings.ToUpper(\"m\")\n",
		"internal/lib/_ignored.go":     "package lib\n\nimport _ \"ignored.test/underscore\"\n",
		"internal/lib/.hidden.go":      "package lib\n\nimport _ \"ignored.test/dot\"\n",
		"internal/yamlish/yamlish.go":  "package yamlish\n\nimport (\n\t_ \"go.yaml.in/yaml/v3\"\n\t_ \"golang.org/x/mod/semver\"\n)\n",
		"internal/yamlish/again.go":    "package yamlish\n\nimport _ \"go.yaml.in/yaml/v3\"\n",
		"internal/testdata/bad/bad.go": "package bad\n\nimport _ \"testdata.test/never\"\n",
		"internal/.cache/cache.go":     "package cache\n\nimport _ \"skipped.test/dot\"\n",
		"internal/_old/old.go":         "package old\n\nimport _ \"skipped.test/underscore\"\n",
		"internal/vendor/vendor.go":    "package vendor\n\nimport _ \"skipped.test/vendor\"\n",
		"internal/nested/go.mod":       "module nested.test\n",
		"internal/nested/nested.go":    "package nested\n\nimport _ \"nested.test/x\"\n",
		"cmd/order/main.go":            "package main\n\nimport (\n\t_ \"example.test/m/cmd/order/zeta\"\n\t_ \"example.test/m/cmd/order/alpha\"\n\t_ \"example.test/m/internal/lib\"\n)\n\nfunc main() {}\n",
		"cmd/order/zeta/zeta.go":       "package zeta\n\nimport (\n\t_ \"go.yaml.in/yaml/v3\"\n\n\t_ \"example.test/m/internal/lib\"\n)\n",
		"cmd/order/alpha/alpha.go":     "package alpha\n\nimport _ \"go.yaml.in/yaml/v3\"\n",
	}
	for relative, body := range files {
		writeFile(t, root, relative, body)
	}
	return root
}

// TestCheckCredentialedJob_GoCommand_BuildsFromTheModuleAlone verifies that a
// Go command a credentialed job runs builds nothing but this module's packages
// and the standard library, however deep its imports go.
//
// The job holding the release's OIDC identity can be made to mint that token
// by any code it runs, and a Go command runs every package it links: a port of
// a script into Go that imported one more module would put that module's code
// beside the token, with nothing pinning it but go.sum. go run, go build, go
// install and go test are held to that, each package read through its imports
// as go reads them, test files only for go test; go generate, which runs what
// its directives name, and a go tool the toolchain does not ship are refused.
// A package at a version is left to the rule that pins it, so one command is
// one finding, and what this audit cannot read (an option it does not know, a
// value only the run knows, a wrapper it does not read) is a finding too.
func TestCheckCredentialedJob_GoCommand_BuildsFromTheModuleAlone(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	const origin = "wf.yml: job release: step 0: "
	outside := ", outside the standard library and this module: " + goGraphReason
	cannot := ", so what it builds cannot be judged"

	cases := []struct {
		name string
		run  string
		want []string
	}{
		{name: "a command of this module and the standard library", run: "go run ./cmd/clean -flag value"},
		{name: "a command reaching other modules", run: "go run ./cmd/dirty", want: []string{origin + "go run ./cmd/dirty imports " + dirtyImports + outside}},
		{name: "a build of it", run: "go build -trimpath -o bin/dirty ./cmd/dirty", want: []string{origin + "go build ./cmd/dirty imports " + dirtyImports + outside}},
		{name: "an install of it", run: "go install ./cmd/dirty", want: []string{origin + "go install ./cmd/dirty imports " + dirtyImports + outside}},
		{
			name: "a test reads the test files", run: "go test -count=1 -run TestX ./cmd/tested",
			want: []string{origin + "go test ./cmd/tested imports github.com/google/go-cmp/cmp (from cmd/tested)" + outside},
		},
		{name: "a build leaves the test files out", run: "go build ./cmd/tested"},
		{name: "files named alone are read alone", run: "go run ./cmd/files/tool.go input.txt"},
		{name: "files of two directories", run: "go run ./cmd/files/tool.go ./cmd/clean/main.go"},
		{name: "files reaching other modules", run: "go run ./cmd/files/tool.go ./cmd/files/other.go", want: []string{origin + "go run ./cmd/files/tool.go ./cmd/files/other.go imports " + dirtyImports + outside}},
		{
			name: "a file only the run knows", run: `go run "$DIR/tool.go"`,
			want: []string{origin + "go run $DIR/tool.go names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannot},
		},
		{
			name: "a file outside the repository", run: "go run ../tool.go",
			want: []string{origin + "go run ../tool.go names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannot},
		},
		{
			name: "an import named first by the package that comes first", run: "go run ./cmd/order",
			want: []string{origin + "go run ./cmd/order imports go.yaml.in/yaml/v3 (from cmd/order/alpha)" + outside},
		},
		{name: "the package of those files", run: "go run ./cmd/files", want: []string{origin + "go run ./cmd/files imports " + dirtyImports + outside}},
		{name: "a package named by its import path", run: "go run example.test/m/cmd/clean"},
		{name: "a package of the standard library", run: "go build fmt"},
		{name: "the toolchain's own meta-patterns", run: "go build std cmd"},
		{
			name: "the tools go.mod declares", run: "go install tool",
			want: []string{origin + "go install tool builds packages of other modules, not this checkout's: " + goGraphReason},
		},
		{
			name: "a build of the tools go.mod declares", run: "go build tool",
			want: []string{origin + "go build tool builds packages of other modules, not this checkout's: " + goGraphReason},
		},
		{
			name: "every package of the workspace", run: "go test work",
			want: []string{origin + "go test work builds packages of other modules, not this checkout's: " + goGraphReason},
		},
		{name: "files handed to go build", run: "go build -o bin/tool ./cmd/files/tool.go"},
		{name: "files handed to go build from the directory -C names", run: "go build -C cmd/files tool.go"},
		{
			name: "files handed to go build reaching other modules", run: "go build ./cmd/files/tool.go ./cmd/files/other.go",
			want: []string{origin + "go build ./cmd/files/tool.go ./cmd/files/other.go imports " + dirtyImports + outside},
		},
		{
			name: "test files handed to go test are read", run: "go test ./cmd/tested/main.go ./cmd/tested/main_test.go -run TestX",
			want: []string{origin + "go test ./cmd/tested/main.go ./cmd/tested/main_test.go imports github.com/google/go-cmp/cmp (from cmd/tested)" + outside},
		},
		{
			name: "-toolexec", run: "go build -toolexec=/tmp/wrap ./cmd/clean",
			want: []string{origin + "go build passes -toolexec, which runs every tool of the build through a program this audit does not read" + cannot},
		},
		{
			name: "-toolexec with its value in the next word", run: "go test -toolexec /tmp/wrap ./cmd/clean",
			want: []string{origin + "go test passes -toolexec, which runs every tool of the build through a program this audit does not read" + cannot},
		},
		{
			name: "-exec", run: "go run -exec /tmp/wrap ./cmd/clean",
			want: []string{origin + "go run passes -exec, which runs the program it builds through a program this audit does not read" + cannot},
		},
		{
			name: "-overlay", run: "go build --overlay=/tmp/overlay.json ./cmd/clean",
			want: []string{origin + "go build passes --overlay, which builds files this audit does not read in place of the module's" + cannot},
		},
		{
			name: "-modfile", run: "go build -modfile=/tmp/alt.mod ./cmd/clean",
			want: []string{origin + "go build passes -modfile, which reads another go.mod in place of the module's" + cannot},
		},
		{
			name: "-pkgdir", run: "go install -pkgdir /tmp/pkg ./cmd/clean",
			want: []string{origin + "go install passes -pkgdir, which installs and loads packages from a directory this audit does not read" + cannot},
		},
		{
			name: "two such options, the first named", run: "go build -overlay /tmp/o.json -toolexec=/tmp/wrap ./cmd/clean",
			want: []string{origin + "go build passes -overlay, which builds files this audit does not read in place of the module's" + cannot},
		},
		{
			name: "-linkshared", run: "go build -linkshared ./cmd/clean",
			want: []string{origin + "go build passes -linkshared, which links against shared libraries this audit does not read" + cannot},
		},
		{name: "go vet builds nothing it runs", run: "go vet -composites=false ./cmd/dirty"},
		{
			name: "go vet handed another analyzer", run: "go vet -vettool=/tmp/analyzer ./...",
			want: []string{origin + "go vet passes -vettool, which runs a program this audit does not read as the analyzer, so what it runs cannot be judged"},
		},
		{
			name: "go vet handed a build option that runs a program", run: "go vet vettool -toolexec /tmp/wrap ./...",
			want: []string{origin + "go vet passes -toolexec, which runs every tool of the build through a program this audit does not read, so what it runs cannot be judged"},
		},
		{name: "go vet behind a wrapper it does not read", run: "xargs go vet", want: []string{origin + "go behind xargs is written in a way this audit does not read" + cannot}},
		{
			name: "a package of another module", run: "go run golang.org/x/vuln/cmd/govulncheck ./...",
			want: []string{origin + "go run golang.org/x/vuln/cmd/govulncheck builds packages of other modules, not this checkout's: " + goGraphReason},
		},
		{
			name: "every package of the build list", run: "go build all",
			want: []string{origin + "go build all builds packages of other modules, not this checkout's: " + goGraphReason},
		},
		{
			name: "an install at an exact version", run: "go install golang.org/x/vuln/cmd/govulncheck@v1.1.4",
			want: []string{origin + "go install golang.org/x/vuln/cmd/govulncheck@v1.1.4 builds packages of other modules, not this checkout's: " + goGraphReason},
		},
		{name: "an install at latest is the latest rule's", run: "go install golang.org/x/vuln/cmd/govulncheck@latest", want: []string{origin + "run block matches " + latestFinding}},
		{name: "an install at a branch is the go-install rule's", run: "go install example.test/tool@main", want: []string{origin + "run block matches " + goInstallFinding}},
		{name: "a run at a version is the go-run-module rule's", run: "go run golang.org/x/vuln/cmd/govulncheck@v1.1.4", want: []string{origin + "run block matches " + goRunModuleFinding}},
		{name: "a build at a version", run: "go build example.test/tool@v1.2.3", want: []string{origin + "go build example.test/tool@v1.2.3 builds packages of other modules, not this checkout's: " + goGraphReason}},
		{name: "the directory -C changes into", run: "go run -C cmd/clean ."},
		{name: "a pattern", run: "go vet ./... && go build ./internal/...", want: []string{origin + "go build ./internal/... imports " + dirtyImports + outside}},
		{
			name: "more than three", run: "go run ./cmd/many",
			want: []string{origin + "go run ./cmd/many imports a.test/one (from cmd/many), b.test/two (from cmd/many), c.test/three (from cmd/many) and 1 more" + outside},
		},
		{
			name: "exactly three", run: "go run ./cmd/three",
			want: []string{origin + "go run ./cmd/three imports a.test/one (from cmd/three), b.test/two (from cmd/three), c.test/three (from cmd/three)" + outside},
		},
		{name: "two commands in one text", run: "go build ./cmd/clean && go run ./cmd/dirty", want: []string{origin + "go run ./cmd/dirty imports " + dirtyImports + outside}},
		{name: "an option taking nothing before the package", run: "go build -race ./cmd/dirty", want: []string{origin + "go build ./cmd/dirty imports " + dirtyImports + outside}},
		{name: "an option given its value with = before the package", run: "go build -o=bin/x ./cmd/dirty", want: []string{origin + "go build ./cmd/dirty imports " + dirtyImports + outside}},
		{
			name: "two wrappers it does not read", run: "xargs go run ./cmd/clean; eval go build ./cmd/clean",
			want: []string{origin + "go behind xargs is written in a way this audit does not read" + cannot, origin + "go behind eval is written in a way this audit does not read" + cannot},
		},
		{name: "a file that does not parse", run: "go run ./cmd/broken", want: []string{origin + "go run ./cmd/broken reads cmd/broken/main.go, which does not parse, so what it imports cannot be judged"}},
		{
			name: "an import of a directory that is not there", run: "go run ./cmd/gap",
			want: []string{origin + "go run ./cmd/gap imports example.test/m/internal/nothere, which names no directory holding Go files in this repository" + cannot},
		},
		{name: "a directory with no Go file", run: "go run ./cmd/nothing", want: []string{origin + "go run ./cmd/nothing names no directory holding Go files in this repository" + cannot}},
		{
			name: "a package only the run knows", run: `go run "$TOOL"`,
			want: []string{origin + "go run $TOOL names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannot},
		},
		{
			name: "a package outside the repository", run: "go build ../elsewhere",
			want: []string{origin + "go build ../elsewhere names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannot},
		},
		{
			name: "the directory above the repository", run: "go build ..",
			want: []string{origin + "go build .. names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannot},
		},
		{
			name: "an absolute path", run: "go build /opt/elsewhere/cmd/x",
			want: []string{origin + "go build /opt/elsewhere/cmd/x names a package this audit cannot resolve (a value only the run knows, or a path outside this repository)" + cannot},
		},
		{name: "the module's root package, which holds no Go file here", run: "go build example.test/m", want: []string{origin + "go build example.test/m names no directory holding Go files in this repository" + cannot}},
		{name: "every package of the repository", run: "go build ./...", want: []string{origin + "go build ./... reads cmd/broken/main.go, which does not parse, so what it imports cannot be judged"}},
		{name: "an option this audit does not read", run: "go run -gobble ./cmd/clean", want: []string{origin + "go run passes -gobble, an option this audit does not read" + cannot}},
		{name: "an option missing its value", run: "go build -o", want: []string{origin + "go build passes -o without the value it takes" + cannot}},
		{name: "no package to run", run: "go run", want: []string{origin + "go run names no package, so what it runs cannot be judged"}},
		{name: "a build with no package builds the directory", run: "go build -tags e2e", want: []string{origin + "go build . names no directory holding Go files in this repository" + cannot}},
		{
			name: "go generate", run: "go generate ./...",
			want: []string{origin + "go generate runs whatever the //go:generate lines of its packages name, which this audit does not read, so what it runs cannot be judged"},
		},
		{name: "a tool the toolchain ships", run: "go tool cover -func=coverage.out"},
		{
			name: "a tool go.mod declares", run: "go tool gotestsum --format testdox",
			want: []string{origin + "go tool gotestsum is not a tool the Go toolchain ships, so it runs one go.mod declares, which is another module's code: " + goGraphReason},
		},
		{name: "behind a wrapper it does not read", run: "xargs go run ./cmd/clean", want: []string{origin + "go behind xargs is written in a way this audit does not read" + cannot}},
		{name: "inside the string sh -c is handed", run: "sh -c 'go run ./cmd/dirty'", want: []string{origin + "go run ./cmd/dirty imports " + dirtyImports + outside}},
		{name: "behind an assignment and a wrapper", run: "GOFLAGS=-mod=mod timeout 5m go build -ldflags '-s -w' -o bin/x ./cmd/clean"},
		{name: "a mention in prose", run: `echo "then go run ./cmd/dirty"`},
		{name: "test flags after the packages", run: "go test ./cmd/clean -count=1 -args -run ignored"},
		{name: "a command on continued lines", run: "go build \\\n  -o bin/x \\\n  ./cmd/dirty", want: []string{origin + "go build ./cmd/dirty imports " + dirtyImports + outside}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkCredentialedJob(%q) =\n%s\nwant\n%s", testCase.run, strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestCheckCredentialedJob_GoCommandInAFile_IsJudgedWhereItRuns verifies that
// a Go command in a script or a make recipe the job reaches is held to the
// module like one in a run block, its packages resolved from the directory the
// file runs in, and that a job holding no credential is not held at all.
func TestCheckCredentialedJob_GoCommandInAFile_IsJudgedWhereItRuns(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	writeFile(t, root, "scripts/stamp.sh", "set -eu\ngo run ./cmd/dirty \"$@\"\n")
	writeFile(t, root, "Makefile", "stamp:\n\t$(MAKE) -C cmd stamp\n")
	writeFile(t, root, "cmd/Makefile", "stamp:\n\tgo run ./dirty\n")
	outside := ", outside the standard library and this module: " + goGraphReason

	cases := []struct {
		name string
		job  map[string]any
		want []string
	}{
		{
			name: "a script",
			job:  map[string]any{"permissions": map[string]any{"contents": "write"}, "steps": []any{map[string]any{"run": "bash scripts/stamp.sh"}}},
			want: []string{"wf.yml: job j: scripts/stamp.sh: go run ./cmd/dirty imports " + dirtyImports + outside},
		},
		{
			name: "a recipe run in another directory",
			job:  map[string]any{"permissions": map[string]any{"contents": "write"}, "steps": []any{map[string]any{"run": "make stamp"}}},
			want: []string{"wf.yml: job j: cmd/Makefile:stamp (via Makefile:stamp): go run ./dirty imports " + dirtyImports + outside},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "j", testCase.job, nil)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkCredentialedJob() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}

	t.Run("a job holding no credential", func(t *testing.T) {
		t.Parallel()

		doc := map[string]any{"jobs": map[string]any{"j": map[string]any{
			"permissions": map[string]any{"contents": "read"},
			"steps":       []any{map[string]any{"run": "go run ./cmd/dirty\ngo generate ./..."}},
		}}}
		if problems := newAudit(root, tables{}).checkWorkflowJobs("wf.yml", doc, nil); len(problems) != 0 {
			t.Errorf("checkWorkflowJobs() = %v, want nothing for a job holding no credential", problems)
		}
	})
}

// TestCheckCredentialedJob_GoCommandWithoutAModule_CannotBeJudged verifies
// that a Go command is not judged against a guess when the repository root's
// go.mod names no module: what is this module's own and what is another's is
// read from that line.
func TestCheckCredentialedJob_GoCommandWithoutAModule_CannotBeJudged(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		gomod string
		run   string
	}{
		{name: "no go.mod at all", run: "go run ./cmd/x"},
		{name: "a go.mod naming no module", gomod: "go 1.27\n", run: "go run ./cmd/x"},
		{name: "files with no go.mod", run: "go run ./cmd/x/main.go"},
		{name: "a module named in quotes", gomod: "module \"example.test/q\"\n", run: "go run example.test/q/cmd/x"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeFile(t, root, "cmd/x/main.go", "package main\n\nfunc main() {}\n")
			if testCase.gomod != "" {
				writeFile(t, root, "go.mod", testCase.gomod)
			}
			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": testCase.run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			var want []string
			if !strings.HasPrefix(testCase.gomod, "module") {
				want = []string{"wf.yml: job release: step 0: " + testCase.run + " is judged against the module the repository " +
					"root's go.mod names, and it names none, so what it builds cannot be judged"}
			}
			if strings.Join(problems, "\n") != strings.Join(want, "\n") {
				t.Errorf("checkCredentialedJob() = %v, want %v", problems, want)
			}
		})
	}
}

// TestCheckCredentialedJob_GoCommandReachingTheModuleRoot_ReadsItsPackage
// verifies that the module's root package, built by the module's own path or
// imported by another package of the module, is read from the repository root
// like any other package of the module.
func TestCheckCredentialedJob_GoCommandReachingTheModuleRoot_ReadsItsPackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.test/r\n")
	writeFile(t, root, "r.go", "package r\n\nimport _ \"go.yaml.in/yaml/v3\"\n")
	writeFile(t, root, "cmd/x/main.go", "package main\n\nimport _ \"example.test/r\"\n\nfunc main() {}\n")
	const origin = "wf.yml: job release: step 0: "
	outside := " imports go.yaml.in/yaml/v3 (from .), outside the standard library and this module: " + goGraphReason

	for _, run := range []string{"go build example.test/r", "go run ./cmd/x"} {
		t.Run(run, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{
				"permissions": map[string]any{"id-token": "write"},
				"steps":       []any{map[string]any{"run": run}},
			}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if want := origin + run + outside; len(problems) != 1 || problems[0] != want {
				t.Errorf("checkCredentialedJob() = %v, want exactly [%q]", problems, want)
			}
		})
	}
}

// TestCheckCredentialedJob_GoCommandStatedLimits_AreNotFollowed pins what
// doc.go states the Go-command rules do not see, so a change that starts or
// stops seeing one of them is a change to that statement too: a go command
// named by a variable, a cd in front of one, which leaves its package resolved
// from the directory the step runs in, a tool a builder makes through a make
// target, what GoReleaser's builds compile, and a GOFLAGS a command writes from
// a value it builds.
func TestCheckCredentialedJob_GoCommandStatedLimits_AreNotFollowed(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	writeFile(t, root, "Makefile", "tool:\n\tgo build -o bin/stamp ./cmd/dirty\n")
	writeFile(t, root, ".goreleaser.yml", "builds:\n  - main: ./cmd/dirty\n    binary: dirty\n")
	const origin = "wf.yml: job release: step 0: "

	for _, testCase := range []struct {
		name string
		run  string
		want []string
	}{
		{name: "a go command named by a variable", run: `"$GO" run ./cmd/dirty`},
		{name: "a GOFLAGS written from a value the command builds", run: `printf 'GOFLAGS=%s\n' "-toolexec=/tmp/wrap" >> "$GITHUB_ENV"`},
		{
			name: "a cd in front of a go command", run: "cd cmd && go run ./dirty",
			want: []string{origin + "go run ./dirty names no directory holding Go files in this repository, so what it builds cannot be judged"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := map[string]any{"permissions": map[string]any{"id-token": "write"}, "steps": []any{map[string]any{"run": testCase.run}}}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkCredentialedJob() = %v, want %v", problems, testCase.want)
			}
		})
	}

	t.Run("a tool built through a make target", func(t *testing.T) {
		t.Parallel()

		doc := map[string]any{"jobs": map[string]any{
			"tools": map[string]any{"steps": []any{
				map[string]any{"run": "make tool"},
				map[string]any{"uses": "actions/upload-artifact@" + testSHA, "with": map[string]any{"name": "tools"}},
			}},
			"publish": map[string]any{"permissions": map[string]any{"id-token": "write"}, "steps": []any{
				map[string]any{"uses": "actions/download-artifact@" + testSHA, "with": map[string]any{"name": "tools"}},
				map[string]any{"run": "./bin/stamp"},
			}},
		}}
		if problems := newAudit(root, tables{}).checkWorkflowJobs("wf.yml", doc, []string{"tools", "publish"}); len(problems) != 0 {
			t.Errorf("checkWorkflowJobs() = %v, want nothing: a tool built through a make target is not recognized as one", problems)
		}
	})

	t.Run("what GoReleaser's builds compile", func(t *testing.T) {
		t.Parallel()

		job := map[string]any{"permissions": map[string]any{"contents": "write"}, "steps": []any{goreleaserStep(nil)}}
		if problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, nil); len(problems) != 0 {
			t.Errorf("checkCredentialedJob() = %v, want nothing: a build's main is the release, not a command the job runs", problems)
		}
	})
}

// TestCheckCredentialedJob_GoFlags_AreHeldToTheOptionsTheAuditReads verifies
// that GOFLAGS, which every go command a job runs reads before its own
// options, is held to the options a go command written out is held to,
// wherever a credentialed job sets it: assigned or exported in a text rule 2
// reads, written to the environment file of later steps, or in the env of the
// workflow, the job or the step.
//
// An option that runs another program or builds files this audit does not
// read changes what a go command builds as surely on GOFLAGS as on its command
// line, and GOFLAGS reaches every go command after it, the ones rule 2 judges
// included. Its own value carried along is no new option, and a value only the
// run knows is a finding, as it is in a package.
func TestCheckCredentialedJob_GoFlags_AreHeldToTheOptionsTheAuditReads(t *testing.T) {
	t.Parallel()

	root := goModuleRoot(t)
	const origin = "wf.yml: job release: step 0: "
	cannot := ", so what it builds cannot be judged"
	toolexec := "GOFLAGS passes -toolexec, which runs every tool of the build through a program this audit does not read" + cannot

	cases := []struct {
		job  map[string]any
		doc  map[string]any
		name string
		run  string
		want []string
	}{
		{name: "options the audit reads", run: "GOFLAGS='-mod=mod -trimpath' go build ./cmd/clean"},
		{name: "its own value carried along", run: "export GOFLAGS=\"${GOFLAGS:-} -trimpath -count=1\"\ngo test ./cmd/clean"},
		{name: "an entry that is not an option, which go refuses", run: "GOFLAGS=trimpath go build ./cmd/clean"},
		{name: "an empty value", run: "GOFLAGS= go build ./cmd/clean"},
		{name: "the second of two settings", run: "export GOFLAGS=-mod=mod\nGOFLAGS=-toolexec=/tmp/wrap go build ./cmd/clean", want: []string{origin + toolexec}},
		{name: "a quote left open", run: "export GOFLAGS=\"-toolexec=/tmp/wrap", want: []string{origin + toolexec}},
		{name: "in front of a go command", run: "GOFLAGS=-toolexec=/tmp/wrap go build ./cmd/clean", want: []string{origin + toolexec}},
		{name: "exported for every later command", run: "export GOFLAGS=\"-mod=mod -toolexec=/tmp/wrap\"\ngo build ./cmd/clean", want: []string{origin + toolexec}},
		{name: "written to the environment of later steps", run: "echo \"GOFLAGS=-toolexec=/tmp/wrap\" >> \"$GITHUB_ENV\"", want: []string{origin + toolexec}},
		{name: "set in go's own environment file", run: "go env -w GOFLAGS=-toolexec=/tmp/wrap", want: []string{origin + toolexec}},
		{
			name: "an option the audit does not read", run: "GOFLAGS=-gobble go build ./cmd/clean",
			want: []string{origin + "GOFLAGS passes -gobble, an option this audit does not read" + cannot},
		},
		{
			name: "a value only the run knows", run: "GOFLAGS=\"-trimpath $EXTRA\" go build ./cmd/clean",
			want: []string{origin + "GOFLAGS is set to -trimpath $EXTRA, a value only the run knows" + cannot},
		},
		{
			name: "the env of the step",
			job:  map[string]any{"steps": []any{map[string]any{"run": "go build ./cmd/clean", "env": map[string]any{"GOFLAGS": "-overlay=/tmp/o.json"}}}},
			want: []string{origin + "env: GOFLAGS passes -overlay, which builds files this audit does not read in place of the module's" + cannot},
		},
		{
			name: "the env of the job",
			job:  map[string]any{"env": map[string]any{"GOFLAGS": "${{ inputs.flags }}"}, "steps": []any{map[string]any{"run": "go build ./cmd/clean"}}},
			want: []string{"wf.yml: job release: env: GOFLAGS is set to ${{ inputs.flags }}, a value only the run knows" + cannot},
		},
		{
			name: "the env of the workflow",
			doc:  map[string]any{"env": map[string]any{"GOFLAGS": "-mod=mod -exec=/tmp/wrap"}},
			run:  "go test ./cmd/clean",
			want: []string{"wf.yml: env: GOFLAGS passes -exec, which runs the program it builds through a program this audit does not read" + cannot},
		},
		{name: "an env that sets no GOFLAGS", job: map[string]any{"env": map[string]any{"GOPROXY": "off"}, "steps": []any{map[string]any{"run": "go build ./cmd/clean"}}}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			job := testCase.job
			if job == nil {
				job = map[string]any{"steps": []any{map[string]any{"run": testCase.run}}}
			}
			job["permissions"] = map[string]any{"id-token": "write"}
			problems := newAudit(root, tables{}).checkCredentialedJob("wf.yml", "release", job, testCase.doc)
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkCredentialedJob() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}

	t.Run("the env of the workflow is reported once for every job it reaches", func(t *testing.T) {
		t.Parallel()

		job := func() map[string]any {
			return map[string]any{"permissions": map[string]any{"contents": "write"}, "steps": []any{map[string]any{"run": "go build ./cmd/clean"}}}
		}
		doc := map[string]any{
			"env":  map[string]any{"GOFLAGS": "-toolexec=/tmp/wrap"},
			"jobs": map[string]any{"one": job(), "two": job(), "lint": map[string]any{"steps": []any{}}},
		}
		problems := newAudit(root, tables{}).checkWorkflowJobs("wf.yml", doc, []string{"lint", "one", "two"})
		if want := "wf.yml: env: " + toolexec; len(problems) != 1 || problems[0] != want {
			t.Errorf("checkWorkflowJobs() = %v, want exactly [%q]", problems, want)
		}
	})
}

// TestUnquote_ShellWord_LosesOnlyTheQuotesAroundIt verifies that a value is
// shown as the shell hands it over: one pair of matching quotes around it
// goes, and anything else, a lone quote or two that do not match, stays.
func TestUnquote_ShellWord_LosesOnlyTheQuotesAroundIt(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct{ word, want string }{
		{word: `"-a -b"`, want: "-a -b"},
		{word: `'-a'`, want: "-a"},
		{word: `""`, want: ""},
		{word: `"`, want: `"`},
		{word: `'`, want: `'`},
		{word: `"-a'`, want: `"-a'`},
		{word: `-a"`, want: `-a"`},
		{word: "-a", want: "-a"},
		{word: "", want: ""},
	} {
		t.Run(testCase.word, func(t *testing.T) {
			t.Parallel()

			if got := unquote(testCase.word); got != testCase.want {
				t.Errorf("unquote(%q) = %q, want %q", testCase.word, got, testCase.want)
			}
		})
	}
}

// TestGoToolProblems_Name_DecidesWhoseCodeRuns verifies that go tool runs the
// toolchain's own code for a tool the toolchain ships and another module's for
// any other name, whatever options stand before the name, the one that takes
// a value included.
func TestGoToolProblems_Name_DecidesWhoseCodeRuns(t *testing.T) {
	t.Parallel()

	tool := func(name string) []string {
		return []string{"o: go tool " + name + " is not a tool the Go toolchain ships, so it runs one go.mod declares, " +
			"which is another module's code: " + goGraphReason}
	}
	cases := []struct {
		name  string
		words []string
		want  []string
	}{
		{name: "no tool named, which lists them"},
		{name: "a toolchain tool behind -n", words: []string{"-n", "cover", "-html=c.out"}},
		{name: "a tool behind -modfile and its value", words: []string{"-modfile", "tools.mod", "gotestsum"}, want: tool("gotestsum")},
		{name: "a toolchain tool behind -modfile=", words: []string{"-modfile=tools.mod", "vet"}},
		{name: "a tool named by its path", words: []string{"golang.org/x/vuln/cmd/govulncheck", "./..."}, want: tool("golang.org/x/vuln/cmd/govulncheck")},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := goToolProblems(reach{origin: "o"}, testCase.words)
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("goToolProblems(%q) = %v, want %v", testCase.words, got, testCase.want)
			}
		})
	}
}
