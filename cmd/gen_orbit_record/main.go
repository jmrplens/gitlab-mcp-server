package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
)

const (
	// prefix names this command on every line it writes, so a failure in a
	// composite make target says which of them produced it.
	prefix = "gen_orbit_record:"
	// regenerate is the command a stale or refused record is fixed with.
	regenerate = "make gen-orbit-record"
	// defaultNamespace is the fixture namespace scripts/setup-orbit-fixtures.sh
	// provisions when ORBIT_FIXTURES_NAMESPACE names none.
	defaultNamespace = "plens1"
)

// genRun is one configured run.
type genRun struct {
	// dir holds the record.
	dir string
	// check selects the offline half.
	check bool
	// token is the GitLab.com credential the calls are made with.
	token string
	// namespace is the fixture namespace the indexing status and the query
	// ask about.
	namespace string
	// upstream is where the recording proxy forwards, which is the instance
	// the record says it was taken from.
	upstream string
	// client makes the proxy's requests. A test replaces its transport to
	// answer for the upstream without a network.
	client *http.Client
	// now supplies the retrieval day and the day the check judges it by.
	now func() time.Time
	// timeout bounds a whole recording, every request through the proxy
	// included: a run whose deadline passes fails and writes nothing.
	timeout time.Duration
	// git runs git in a directory and returns its standard output, which is
	// how a recording reads the record committed at HEAD. A test states what
	// HEAD holds through it without making a commit.
	git func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Seams over what main does that a test cannot follow it into: ending the
// process, and reaching GitLab.com. upstreamTransport is nil in a real run,
// which is net/http's default transport.
var (
	osExit            = os.Exit
	upstreamTransport http.RoundTripper
)

func main() {
	osExit(runMain(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// runMain parses the flags and the environment, and returns the exit status.
// It takes both as parameters so a test drives every dispatch without
// touching the process's own.
func runMain(args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("gen_orbit_record", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dir := flags.String("dir", orbitrecord.DefaultDir, "directory holding the Orbit response record")
	check := flags.Bool("check", false, "judge the committed record offline instead of recording one")
	namespace := flags.String("namespace", "", "fixture namespace to ask about (default ORBIT_FIXTURES_NAMESPACE, then "+defaultNamespace+")")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	// Twelve calls, the slowest of them a query the indexer answers in a
	// second or two.
	timeout := 5 * time.Minute
	return run(genRun{
		dir:       *dir,
		check:     *check,
		token:     getenv("GITLAB_COM_TOKEN"),
		namespace: firstNonEmpty(*namespace, getenv("ORBIT_FIXTURES_NAMESPACE"), defaultNamespace),
		upstream:  orbitrecord.Instance,
		client:    &http.Client{Timeout: timeout, Transport: upstreamTransport},
		now:       time.Now,
		timeout:   timeout,
		git:       runGit,
	}, out, errOut)
}

// firstNonEmpty is the first of values that is not empty.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// run dispatches one configured run and returns its exit status.
func run(cfg genRun, out, errOut io.Writer) int {
	if cfg.check {
		return checkRecord(cfg, out, errOut)
	}
	return record(cfg, out, errOut)
}

// checkRecord is the gate: the committed record decodes, is whole, current
// and value-free, and is in the form this command writes. It needs no token
// and no network.
func checkRecord(cfg genRun, out, errOut io.Writer) int {
	doc, err := orbitrecord.Read(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	problems := orbitrecord.Problems(doc, provenance.Clock(cfg.now))
	if formErr := docgen.WriteOrCheck(orbitrecord.Path(cfg.dir), orbitrecord.Encode(doc), true, regenerate); formErr != nil {
		problems = append(problems, "the record is not in the form this command writes, so it was edited by hand or by another build: "+formErr.Error())
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(errOut, prefix, problem)
		}
		fmt.Fprintf(errOut, "%s re-record with `%s` (GITLAB_COM_TOKEN set)\n", prefix, regenerate)
		return 1
	}
	fmt.Fprintf(out, "%s %d calls recorded from %s, Orbit %s, namespace %s, on %s\n",
		prefix, len(doc.Calls), doc.Source.Instance, doc.Source.OrbitVersion, doc.Source.Namespace, doc.Source.RetrievedAt)
	return 0
}

// record is the network half: make every expected call through the handlers,
// write the record, and say what changed since the one committed at HEAD.
//
// The comparison is with HEAD's copy and never with the file on disk, which
// this run has just replaced: compared with the file, a second run would find
// nothing to report and pass with the change still uncommitted. Compared with
// HEAD, every run fails while the key tree differs from the committed one,
// until somebody has read the change and committed the record.
func record(cfg genRun, out, errOut io.Writer) int {
	if cfg.token == "" {
		fmt.Fprintln(errOut, prefix, "GITLAB_COM_TOKEN is not set: Orbit answers no anonymous caller, so nothing can be recorded")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	doc, err := recordCalls(ctx, cfg)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	// A recording that fails the gate is not written: it would replace a
	// whole record with one the next -check refuses.
	if problems := orbitrecord.Problems(doc, provenance.Clock(cfg.now)); len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(errOut, prefix, problem)
		}
		fmt.Fprintln(errOut, prefix, "refusing to write a record the check would refuse")
		return 1
	}

	if writeErr := docgen.WriteOrCheck(orbitrecord.Path(cfg.dir), orbitrecord.Encode(doc), false, regenerate); writeErr != nil {
		fmt.Fprintln(errOut, prefix, writeErr)
		return 1
	}
	fmt.Fprintf(out, "%s wrote %s: %d calls, Orbit %s\n", prefix, orbitrecord.Path(cfg.dir), len(doc.Calls), doc.Source.OrbitVersion)
	committed, committedErr := committedRecord(ctx, cfg)
	if committedErr != nil {
		fmt.Fprintf(out, "%s no record committed at HEAD could be read (%v), so there is nothing to compare this one with\n", prefix, committedErr)
		return 0
	}
	changes := orbitrecord.Diff(committed, doc)
	if len(changes) == 0 {
		fmt.Fprintln(out, prefix, "the key tree is the one committed at HEAD")
		return 0
	}
	for _, change := range changes {
		fmt.Fprintln(errOut, prefix, change)
	}
	fmt.Fprintf(errOut, "%s the key tree differs from the one committed at HEAD in %d places: read them, run make audit-1to1-paths, and commit the record, since every recording fails until it is\n", prefix, len(changes))
	return 1
}
