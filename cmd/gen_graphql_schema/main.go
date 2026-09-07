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
	// maxPinAge is how long a pin may stand before --check refuses it. GitLab
	// ships monthly and narrows fields in place, so half a year is roughly six
	// releases of drift: long enough not to ambush an unrelated change often,
	// short enough that a narrowing is noticed within a release cycle or two.
	maxPinAge = 180 * 24 * time.Hour
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

// clock is now with a default, so a run that only checks the committed pair
// does not have to supply one to ask how old it is.
func (c genRun) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func main() {
	endpoint := flag.String("url", defaultEndpoint, "GraphQL endpoint to introspect")
	dir := flag.String("dir", defaultDir, "directory holding the pinned schema and its provenance record")
	check := flag.Bool("check", false, "load the committed schema instead of fetching one, and fail when it does not parse")
	flag.Parse()

	os.Exit(run(genRun{
		endpoint: *endpoint,
		dir:      *dir,
		check:    *check,
		token:    os.Getenv("GITLAB_TOKEN"),
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

	problems := pinProblems(source, cfg.clock())
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
// Age is here for the opposite reason: the pin can only report a document that
// was already broken when it was taken, so an old pin is a gate that has
// stopped asking, and the only honest way to say so is to fail.
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
	if source.GitLabVersion == graphqlintrospect.UnknownVersion {
		problems = append(problems,
			"the pin records no GitLab version, which is what an introspection without GITLAB_TOKEN produces: nothing can then say which release the gate speaks for")
	}
	if age, ok := pinAge(source, now); ok && age > maxPinAge {
		problems = append(problems, fmt.Sprintf(
			"the pin is %d days old and the window is %d: GitLab narrows fields in place, so a pin this old can no longer report a document that broke since",
			int(age.Hours()/24), int(maxPinAge.Hours()/24),
		))
	}
	return problems
}

// pinAge reports how long ago the pin was taken. An unparseable date is left to
// the record's own decoding, which has already accepted it, rather than turned
// into a second complaint about the same field.
func pinAge(source graphqlschema.Source, now time.Time) (time.Duration, bool) {
	retrieved, err := time.Parse(time.DateOnly, source.RetrievedAt)
	if err != nil {
		return 0, false
	}
	return now.UTC().Sub(retrieved), true
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

	source := graphqlschema.Source{
		Instance:       cfg.endpoint,
		GitLabVersion:  version,
		GitLabRevision: revision,
		RetrievedAt:    cfg.now().UTC().Format(time.DateOnly),
		Types:          len(schema.Types),
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
	for _, problem := range pinProblems(source, cfg.clock()) {
		fmt.Fprintln(errOut, prefix+" warning:", problem)
	}
	return 0
}
