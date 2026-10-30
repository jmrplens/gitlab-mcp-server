// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// The release pin is the schema of the GitLab release the REST record is
// taken from, rendered like the gitlab.com pin. It is read from the dump
// GitLab's own CI publishes for the release's tag rather than from an
// introspection, because no instance of the release runs anywhere this
// command can reach without Docker, and the dump is what that release builds
// its schema from: the graphql-schema-dump job runs rake
// gitlab:graphql:schema:dump on the tag's commit, with every feature flag
// enabled, which is the schema an instance of the release serves (GitLab gates
// a GraphQL field at resolve time, not by leaving it out of the schema).
const (
	// defaultDumpBase is where GitLab's CI publishes the artifacts of the
	// latest successful job of a name for a ref, the release tag here.
	defaultDumpBase = "https://gitlab.com/gitlab-org/gitlab/-/jobs/artifacts"
	// dumpArtifactPath is the file the graphql-schema-dump job writes and
	// keeps as its artifact (.gitlab/ci/frontend.gitlab-ci.yml).
	dumpArtifactPath = "raw/tmp/tests/graphql/gitlab_schema.graphql"
	// dumpJob is the job that writes it.
	dumpJob = "graphql-schema-dump"
	// maxDumpBytes bounds the download. The 19.5.0 dump is 3.8 MiB, so this
	// is room for growth and not a limit any release comes near; it exists so
	// an answer that is not a schema at all cannot fill memory.
	maxDumpBytes = 64 << 20
	// regenerateRelease is the command a refusal names.
	regenerateRelease = "make gen-graphql-release-schema"
)

// releaseDumpURL names the schema dump of a release tag. The REST record
// spells the version as GitLab's image reports it ("19.5.0-ee"), and the tag
// GitLab cuts for it is that version behind a v.
func releaseDumpURL(base, version string) string {
	return base + "/" + url.PathEscape("v"+version) + "/" + dumpArtifactPath + "?job=" + dumpJob
}

// supportedRelease reads the release the REST record was taken from, which is
// the release this repository is pinned to: the fine-grained table is joined
// from it and the 1:1 audit judges the REST surface by it.
func supportedRelease(apiLiveDir string) (apilive.Source, error) {
	record, err := apilive.Read(apiLiveDir)
	if err != nil {
		return apilive.Source{}, err
	}
	if record.Source.Version == "" {
		return apilive.Source{}, fmt.Errorf("%s names no GitLab version, so nothing says which release to pin", apilive.Path(apiLiveDir))
	}
	return record.Source, nil
}

// generateRelease writes the release pin: the schema of the release the REST
// record names, from the dump of its tag or from the file -from hands it.
func generateRelease(cfg genRun, out, errOut io.Writer) int {
	release, err := supportedRelease(cfg.apiLiveDir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	dump, origin, err := readDump(cfg, release.Version, out)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	schema, err := graphqlintrospect.FromSDL(origin, dump)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	if graphqlintrospect.TruncatedAnswer(len(schema.Types)) {
		fmt.Fprintf(errOut, prefix+" %s holds %d types and a GitLab schema has more than %d: it is not the dump of a whole release\n",
			origin, len(schema.Types), graphqlintrospect.MinimumTypes)
		return 1
	}
	sdl := graphqlintrospect.RenderSDL(schema)
	// The rendering is loaded before it is written for the reason generate
	// gives: a renderer that mangled something writes a file that looks
	// fine and refuses every document later.
	loaded, err := graphqlschema.Load([]byte(sdl))
	if err != nil {
		fmt.Fprintln(errOut, prefix+" the converted release schema does not parse:", err)
		return 1
	}

	digest := sha256.Sum256(dump)
	source := graphqlschema.ReleaseSource{
		GitLabVersion:  release.Version,
		GitLabRevision: release.Revision,
		Origin:         origin,
		SHA256:         hex.EncodeToString(digest[:]),
		RetrievedAt:    provenance.Clock(cfg.now).UTC().Format(time.DateOnly),
		Types:          len(loaded.Types),
	}
	if err = writeReleaseArtifacts(cfg.dir, sdl, source); err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	fmt.Fprintf(out, prefix+" wrote %s (%d KiB) and %s; %s\n",
		graphqlschema.ReleaseSDLFileName, len(sdl)/1024, graphqlschema.ReleaseSourceFileName, source)
	return 0
}

// readDump returns the schema text of the release and the name of where it
// came from: the file -from names, or the dump of the release's tag, named by
// the URL the download ended at, which is the one that names GitLab's job.
func readDump(cfg genRun, version string, out io.Writer) (dump []byte, origin string, err error) {
	if cfg.from != "" {
		dump, err = os.ReadFile(cfg.from) //#nosec G304 -- the path is the operator's own -from flag
		if err != nil {
			return nil, "", fmt.Errorf("read the release schema: %w", err)
		}
		return dump, cfg.from, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), graphqlintrospect.FetchTimeout)
	defer cancel()
	location := releaseDumpURL(cfg.dumpBase, version)
	fmt.Fprintf(out, prefix+" downloading the schema of GitLab %s from %s\n", version, location)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("build the request for %s: %w", location, err)
	}
	response, err := cfg.client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("download %s: %w", location, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf(
			"%s answered %s: GitLab keeps a job's artifacts for a while and the tag's may be gone, in which case "+
				"run `bundle exec rake gitlab:graphql:schema:dump` at the tag in a GitLab checkout and pass "+
				"tmp/tests/graphql/gitlab_schema.graphql with -from",
			location, response.Status,
		)
	}
	limit := cmp.Or(cfg.dumpLimit, maxDumpBytes)
	dump, err = io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", location, err)
	}
	if int64(len(dump)) > limit {
		return nil, "", fmt.Errorf("%s answered with more than %d bytes, which is not a schema dump", location, limit)
	}
	return dump, response.Request.URL.String(), nil
}

// checkReleaseArtifacts is the CI half of the release pin: the committed files
// parse, agree with one another, and are the schema of the release the REST
// record is taken from. Like the gitlab.com pin's check it needs no network.
//
// It asks no age of the pin. A release's schema does not change once the
// release is cut; what ages is which release this repository is pinned to,
// and that is the REST record's to say, under the staleness window
// cmd/gen_api_live holds it to. Holding this pin to the record's version and
// revision is what moves it with the record.
func checkReleaseArtifacts(cfg genRun, out, errOut io.Writer) int {
	types, source, err := readReleaseArtifacts(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	release, err := supportedRelease(cfg.apiLiveDir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	problems := releaseProblems(source, release)
	if types != source.Types {
		problems = append(problems, fmt.Sprintf(
			"%s records %d types and %s loads with %d: the two files were not written by one regeneration",
			graphqlschema.ReleaseSourceFileName, source.Types, graphqlschema.ReleaseSDLFileName, types,
		))
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(errOut, prefix, problem)
		}
		fmt.Fprintf(errOut, prefix+" re-pin the release schema with `%s`\n", regenerateRelease)
		return 1
	}

	fmt.Fprintf(out, prefix+" the release schema parses, %d types; %s\n", types, source)
	return 0
}

// releaseProblems reports every way the release record fails to be the schema
// of the release the REST record names.
func releaseProblems(source graphqlschema.ReleaseSource, release apilive.Source) []string {
	var problems []string
	if source.GitLabVersion != release.Version || source.GitLabRevision != release.Revision {
		problems = append(problems, fmt.Sprintf(
			"the release schema is GitLab %s (%s) and the REST record is taken from GitLab %s (%s): the documents "+
				"would be held to a release the rest of the repository is not pinned to",
			source.GitLabVersion, source.GitLabRevision, release.Version, release.Revision,
		))
	}
	if graphqlintrospect.TruncatedAnswer(source.Types) {
		problems = append(problems, fmt.Sprintf(
			"the release schema carries %d types and a GitLab schema has more than %d: the dump was truncated or of a narrower edition",
			source.Types, graphqlintrospect.MinimumTypes,
		))
	}
	if source.Origin == "" || source.SHA256 == "" {
		problems = append(problems,
			"the release record names no origin or no digest, so nothing says which dump it was rendered from")
	}
	return problems
}

// writeReleaseArtifacts writes the release schema and its record into dir.
// The text is committed uncompressed for the reasons writeArtifacts gives.
func writeReleaseArtifacts(dir, sdl string, source graphqlschema.ReleaseSource) error {
	if err := docgen.WriteOrCheck(filepath.Join(dir, graphqlschema.ReleaseSDLFileName),
		[]byte(sdl), false, regenerateRelease); err != nil {
		return err
	}
	// Marshaling a struct built one line earlier cannot fail.
	record := cmdutil.Must(json.MarshalIndent(source, "", "  "))
	return docgen.WriteOrCheck(filepath.Join(dir, graphqlschema.ReleaseSourceFileName),
		record, false, regenerateRelease)
}

// readReleaseArtifacts loads the committed release schema and record from dir,
// from disk rather than the embedded copies for the reason readArtifacts
// gives.
func readReleaseArtifacts(dir string) (int, graphqlschema.ReleaseSource, error) {
	schemaPath := filepath.Join(dir, graphqlschema.ReleaseSDLFileName)
	sdl, err := os.ReadFile(schemaPath)
	if err != nil {
		return 0, graphqlschema.ReleaseSource{}, fmt.Errorf("read %s: %w", schemaPath, err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return 0, graphqlschema.ReleaseSource{}, fmt.Errorf("%s: %w", schemaPath, err)
	}

	recordPath := filepath.Join(dir, graphqlschema.ReleaseSourceFileName)
	record, err := os.ReadFile(recordPath)
	if err != nil {
		return 0, graphqlschema.ReleaseSource{}, fmt.Errorf("read %s: %w", recordPath, err)
	}
	source, err := graphqlschema.ParseReleaseSource(record)
	if err != nil {
		return 0, graphqlschema.ReleaseSource{}, fmt.Errorf("%s: %w", recordPath, err)
	}
	return len(schema.Types), source, nil
}

// errReleaseFlags refuses -from without -release, which would otherwise be
// ignored by the gitlab.com pin's run that has no use for it.
var errReleaseFlags = errors.New("-from names a release schema dump and needs -release")
