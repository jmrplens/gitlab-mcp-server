package main

import (
	"debug/buildinfo"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
)

// toolName is how the command names itself in its messages.
const toolName = "gen_third_party_notices"

// config is one run: the binaries to read, the file to write, the targets the
// binaries must cover, where GOROOT and the module cache are when the caller
// names them, and how build information and `go env` are read.
type config struct {
	patterns []string
	out      string
	targets  []string
	goroot   string
	modcache string
	readInfo infoReader
	goEnv    func() ([]byte, error)
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decides is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs what it describes, returning the
// process exit code: 2 for arguments it cannot parse, 1 for notices it could
// not generate, 0 once the file is written.
func runMain(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("o", "THIRD_PARTY_NOTICES", "file the notices are written to")
	targets := flags.String("targets", "", "comma-separated GOOS/GOARCH pairs the binaries must cover exactly; empty accepts the ones they are")
	goroot := flags.String("goroot", "", "Go root whose license covers the standard library; empty asks `go env GOROOT`")
	modcache := flags.String("modcache", "", "module cache the license files are read from; empty asks `go env GOMODCACHE`")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 {
		fmt.Fprintf(stderr, "%s: name at least one binary, or a glob of them\n", toolName)
		return 2
	}
	cfg := config{
		patterns: flags.Args(),
		out:      *out,
		targets:  splitList(*targets),
		goroot:   *goroot,
		modcache: *modcache,
		readInfo: buildinfo.ReadFile,
		goEnv:    runGoEnv,
	}
	summary, err := run(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	fmt.Fprintln(stdout, summary)
	return 0
}

// splitList reads a comma-separated flag value, dropping empty items, so an
// empty flag is an empty list.
func splitList(value string) []string {
	var items []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// run reads the binaries, resolves where the texts live, renders the notices
// and writes them, returning a one-line summary of what it wrote.
func run(cfg config) (string, error) {
	paths, err := expand(cfg.patterns)
	if err != nil {
		return "", err
	}
	set, err := collect(paths, cfg.readInfo)
	if err != nil {
		return "", err
	}
	if targetErr := set.requireTargets(cfg.targets); targetErr != nil {
		return "", targetErr
	}
	env, err := resolveGoEnv(cfg.goroot, cfg.modcache, cfg.goEnv)
	if err != nil {
		return "", err
	}
	doc, err := render(set, env)
	if err != nil {
		return "", err
	}
	if writeErr := os.WriteFile(cfg.out, doc, docgen.GeneratedFileMode); writeErr != nil {
		return "", writeErr
	}
	return fmt.Sprintf("%s: %s covers %d modules and the Go standard library for %s",
		toolName, cfg.out, len(set.modules), strings.Join(set.targetList(), ", ")), nil
}
