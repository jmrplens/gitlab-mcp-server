package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

const (
	// prefix names this command on every line it writes, so a failure in a
	// composite make target says which of them produced it.
	prefix = "gen_graphql_schema:"
	// defaultEndpoint is gitlab.com, which answers introspection to anyone.
	defaultEndpoint = "https://gitlab.com/api/graphql"
	// defaultDir is where the package that embeds the schema lives.
	defaultDir = "internal/graphqlschema"
)

// genRun is one configured run: which instance to ask, where to write, and
// whether to ask anything at all.
type genRun struct {
	endpoint string
	dir      string
	check    bool
	token    string
	client   *http.Client
	// now supplies the day recorded in the provenance file, as a parameter so
	// a test can assert on the record it produced.
	now func() time.Time
}

// target is the instance this run asks, in the shape the introspection package
// takes.
func (c genRun) target() graphqlintrospect.Target {
	return graphqlintrospect.Target{Endpoint: c.endpoint, Token: c.token, Client: c.client}
}

func main() {
	endpoint := flag.String("url", defaultEndpoint, "GraphQL endpoint to introspect")
	dir := flag.String("dir", defaultDir, "directory holding the pinned schema and its provenance record")
	check := flag.Bool("check", false, "load the committed schema instead of fetching one, and fail when it does not parse")
	flag.Parse()

	// -url takes an arbitrary endpoint, so the credential is resolved against
	// the instance GITLAB_URL names rather than followed wherever the flag
	// points. Pinning gitlab.com with a version recorded therefore asks for
	// GITLAB_URL=https://gitlab.com beside the token, which is the honest
	// requirement: the version comes from a credential, and a credential is
	// for one instance.
	credential, withheld := graphqlintrospect.CredentialFor(*endpoint, os.Getenv("GITLAB_URL"), os.Getenv("GITLAB_TOKEN"))
	if withheld != "" {
		fmt.Fprintln(os.Stderr, prefix+" note:", withheld)
	}
	os.Exit(run(genRun{
		endpoint: *endpoint,
		dir:      *dir,
		check:    *check,
		token:    credential,
		client:   &http.Client{Timeout: graphqlintrospect.FetchTimeout},
		now:      time.Now,
	}, os.Stdout, os.Stderr))
}

// run is main with its streams, its clock and its HTTP client handed to it, so
// the ways this command ends are reachable from a test instead of only from a
// process. It returns the exit status rather than calling os.Exit.
func run(cfg genRun, out, errOut io.Writer) int {
	if cfg.check {
		return checkArtifacts(cfg, out, errOut)
	}
	return generate(cfg, out, errOut)
}

// checkArtifacts is the CI half: it proves the committed files parse and that
// they are a pin of what this project claims to be pinned to. It needs no
// network, which is what lets it be a gate at all.
func checkArtifacts(cfg genRun, out, errOut io.Writer) int {
	types, source, err := readArtifacts(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	problems := pinProblems(source, provenance.Clock(cfg.now))
	// The record and the schema are two files, and everything above judges
	// the record alone. Holding the record's count to the count the file
	// beside it loads with is what makes them one pin rather than a record
	// vouching for whatever happens to sit next to it: a truncated schema
	// that still parses, committed beside a record from a whole one, passed
	// every check here until this existed.
	if types != source.Types {
		problems = append(problems, fmt.Sprintf(
			"%s records %d types and %s loads with %d: the two files were not written by one regeneration",
			graphqlschema.SourceFileName, source.Types, graphqlschema.SDLFileName, types,
		))
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(errOut, prefix, problem)
		}
		fmt.Fprintf(errOut, prefix+" re-pin with `make gen-graphql-schema` (GITLAB_TOKEN set, so the version is recorded)\n")
		return 1
	}

	fmt.Fprintf(out, prefix+" the pinned schema parses, %d types; %s\n", types, source)
	return 0
}

// pinProblems reports every way the committed provenance record fails to be the
// pin this project's guarantee rests on.
//
// A schema that parses says nothing about what it is a schema of, and until
// this existed nothing asked: a run against a self-managed instance, or one
// without a token, wrote a narrower or anonymous pin that every gate accepted
// in silence. Each check below stands for a way the guarantee quietly shrinks.
// Age is asked for the opposite reason and is asked elsewhere: the pin can only
// report a document that was already broken when it was taken, so an old pin is
// a gate that has stopped asking, which is the decision
// [provenance.Problems] holds for every record this repository pins.
func pinProblems(source graphqlschema.Source, now time.Time) []string {
	var problems []string
	if source.Instance != defaultEndpoint {
		problems = append(problems, fmt.Sprintf(
			"the pin was taken from %s, not %s: the gate would then promise what that instance accepts, which is not what this server targets",
			source.Instance, defaultEndpoint,
		))
	}
	if graphqlintrospect.TruncatedAnswer(source.Types) {
		problems = append(problems, fmt.Sprintf(
			"the pin carries %d types and gitlab.com answers with more than %d: the introspection was truncated or the instance was a narrower edition",
			source.Types, graphqlintrospect.MinimumTypes,
		))
	}
	// A blank version is refused alongside the recorded "unknown": the
	// decoder accepts any string here, so a record with the field emptied by
	// hand would otherwise pass the one check that asks about it.
	if source.GitLabVersion == "" || source.GitLabVersion == graphqlintrospect.UnknownVersion {
		problems = append(problems,
			"the pin records no GitLab version, which is what an introspection without GITLAB_TOKEN produces: nothing can then say which release the gate speaks for")
	}
	return append(problems, provenance.Problems(provenance.Subject{
		Noun:        "pin",
		Consequence: "GitLab narrows fields in place, so one this old can no longer report a document that broke since",
	}, source.RetrievedAt, now)...)
}

// generate is the network half: introspect, convert, and write.
func generate(cfg genRun, out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), graphqlintrospect.FetchTimeout)
	defer cancel()

	fmt.Fprintf(out, prefix+" introspecting %s\n", cfg.endpoint)
	schema, err := graphqlintrospect.Introspect(ctx, cfg.target())
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	// An answer too short to be a GitLab schema does not replace one that was
	// whole. The floor alone cannot tell a truncation from a narrower edition,
	// which is why a probe of such an instance is still allowed to write into
	// an empty directory for `-schema` to read; what it must not do is
	// overwrite a pin that already cleared the floor and exit reporting
	// success. `--check` would refuse the result, but only after the good pin
	// was already gone from the working tree.
	if graphqlintrospect.TruncatedAnswer(len(schema.Types)) {
		if _, existing, readErr := readArtifacts(cfg.dir); readErr == nil && !graphqlintrospect.TruncatedAnswer(existing.Types) {
			fmt.Fprintf(errOut, prefix+" %s answered with %d types and the pin in %s carries %d: refusing to replace a whole schema with a truncated answer\n",
				cfg.endpoint, len(schema.Types), cfg.dir, existing.Types)
			return 1
		}
	}

	version, revision := graphqlintrospect.InstanceVersion(ctx, cfg.target())
	sdl := graphqlintrospect.RenderSDL(schema)

	// Loading what is about to be committed is the only check that the
	// conversion produced SDL at all. A renderer that dropped an implements
	// clause or mangled a default value writes a file that looks fine and
	// refuses every document later, so the artifact is parsed before it lands
	// rather than after somebody's test fails.
	loaded, err := graphqlschema.Load([]byte(sdl))
	if err != nil {
		fmt.Fprintln(errOut, prefix+" the converted schema does not parse:", err)
		return 1
	}

	// The record carries the count the loaded schema has, not the one the
	// introspection had, because the loaded count is the one --check can
	// recompute from the file on disk and hold the record to. The two agree
	// for a whole GitLab schema anyway: the renderer omits the built-in
	// scalars and the __ types, and the loader's prelude puts the same
	// thirteen back.
	source := graphqlschema.Source{
		Instance:       cfg.endpoint,
		GitLabVersion:  version,
		GitLabRevision: revision,
		RetrievedAt:    cfg.now().UTC().Format(time.DateOnly),
		Types:          len(loaded.Types),
	}
	if err = writeArtifacts(cfg.dir, sdl, source); err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	fmt.Fprintf(out, prefix+" wrote %s (%d KiB) and %s; %s, %d loaded\n",
		graphqlschema.SDLFileName, len(sdl)/1024, graphqlschema.SourceFileName, source, len(loaded.Types))
	// A pin taken from anywhere but gitlab.com narrows what the gate promises,
	// and the person who ran this is the only one in a position to notice.
	// Saying so here rather than only in CI is the difference between a
	// sentence and a red pipeline an hour later.
	for _, problem := range pinProblems(source, provenance.Clock(cfg.now)) {
		fmt.Fprintln(errOut, prefix+" warning:", problem)
	}
	return 0
}
