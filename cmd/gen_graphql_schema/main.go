package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

const (
	// defaultEndpoint is gitlab.com, which answers introspection to anyone.
	defaultEndpoint = "https://gitlab.com/api/graphql"
	// defaultDir is where the package that embeds the schema lives.
	defaultDir = "internal/graphqlschema"
	// unknownVersion is recorded when the instance would not say what it runs,
	// which is what GitLab answers an anonymous caller.
	unknownVersion = "unknown"
	// fetchTimeout bounds the whole generation. The introspection payload is
	// tens of megabytes of JSON and gitlab.com takes seconds to produce it.
	fetchTimeout = 3 * time.Minute
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
		client:   &http.Client{Timeout: fetchTimeout},
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

// checkArtifacts is the CI half: it proves the committed files parse. It needs
// no network, which is what lets it be a gate at all.
func checkArtifacts(cfg genRun, out, errOut io.Writer) int {
	types, source, err := readArtifacts(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, "gen_graphql_schema:", err)
		return 1
	}
	fmt.Fprintf(out, "gen_graphql_schema: the pinned schema parses, %d types; %s\n", types, source)
	return 0
}

// generate is the network half: introspect, convert, and write.
func generate(cfg genRun, out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	fmt.Fprintf(out, "gen_graphql_schema: introspecting %s\n", cfg.endpoint)
	schema, err := introspect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(errOut, "gen_graphql_schema:", err)
		return 1
	}

	version, revision := instanceVersion(ctx, cfg)
	compressed := compress(renderSDL(schema))

	// Loading what is about to be committed is the only check that the
	// conversion produced SDL at all. A renderer that dropped an implements
	// clause or mangled a default value writes a file that looks fine and
	// refuses every document later, so the artifact is parsed before it lands
	// rather than after somebody's test fails.
	loaded, err := graphqlschema.Load(compressed)
	if err != nil {
		fmt.Fprintln(errOut, "gen_graphql_schema: the converted schema does not parse:", err)
		return 1
	}

	source := graphqlschema.Source{
		Instance:       cfg.endpoint,
		GitLabVersion:  version,
		GitLabRevision: revision,
		RetrievedAt:    cfg.now().UTC().Format(time.DateOnly),
		Types:          len(schema.Types),
	}
	if err = writeArtifacts(cfg.dir, compressed, source); err != nil {
		fmt.Fprintln(errOut, "gen_graphql_schema:", err)
		return 1
	}

	fmt.Fprintf(out, "gen_graphql_schema: wrote %s (%d KiB compressed) and %s; %s, %d loaded\n",
		graphqlschema.SDLFileName, len(compressed)/1024, graphqlschema.SourceFileName, source, len(loaded.Types))
	return 0
}
