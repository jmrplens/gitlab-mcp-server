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
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
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
	// maxDumpBytes bounds the dump, downloaded or read from -from. The 19.5.0
	// dump is 3.8 MiB, so this is room for growth and not a limit any release
	// comes near; it exists so an answer or a file that is not a schema at all
	// cannot fill memory.
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
//
// Either way the version and revision recorded are the REST record's. The
// download asks for the dump of that version's tag, so the two agree by
// construction; a file names no release of its own, so a pin read from one is
// recorded as such (FromFile) and its release is taken on the REST record's
// word, which is what the person handing the file vouches for.
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

	source := graphqlschema.ReleaseSource{
		GitLabVersion:  release.Version,
		GitLabRevision: release.Revision,
		Origin:         origin,
		FromFile:       cfg.from != "",
		SHA256:         hexDigest(dump),
		SDLSHA256:      hexDigest([]byte(sdl)),
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
	limit := cmp.Or(cfg.dumpLimit, maxDumpBytes)
	if cfg.from != "" {
		var file *os.File
		file, err = os.Open(cfg.from) //#nosec G304 -- the path is the operator's own -from flag
		if err != nil {
			return nil, "", fmt.Errorf("read the release schema: %w", err)
		}
		defer func() { _ = file.Close() }()
		dump, err = io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil {
			return nil, "", fmt.Errorf("read the release schema: %w", err)
		}
		if int64(len(dump)) > limit {
			return nil, "", fmt.Errorf("%s holds more than %d bytes, which is not a schema dump", cfg.from, limit)
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
// The two files agree when the schema hashes to the digest the record carries
// and loads with the count it records. The digest is what a hand edit of the
// schema and a regeneration that wrote one file and not the other both fail,
// whatever the count says. It cannot say which release a dump read with -from
// was of: the dump is not committed, and a file names no release of its own,
// which the record states (FromFile) rather than this check pretending to
// verify it.
//
// It asks no age of the pin. A release's schema does not change once the
// release is cut; what ages is which release this repository is pinned to,
// and that is the REST record's to say, under the staleness window
// cmd/gen_api_live holds it to. Holding this pin to the record's version and
// revision is what moves it with the record.
func checkReleaseArtifacts(cfg genRun, out, errOut io.Writer) int {
	types, sdlDigest, source, err := readReleaseArtifacts(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	release, err := supportedRelease(cfg.apiLiveDir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	problems := releaseProblems(source, release, provenance.Clock(cfg.now))
	if sdlDigest != source.SDLSHA256 {
		problems = append(problems, fmt.Sprintf(
			"%s hashes to %s and %s records %q: the schema was changed after it was rendered, or the two files were not written by one regeneration",
			graphqlschema.ReleaseSDLFileName, sdlDigest, graphqlschema.ReleaseSourceFileName, source.SDLSHA256,
		))
	}
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
//
// The retrieval day is held to what the gitlab.com pin's is, less the window:
// a date nothing can read, or one that has not happened, is a record no
// regeneration wrote, and the line that reports a sound pin prints that day. It
// reads the date through [provenance.Age] rather than [provenance.Problems],
// whose third refusal is the age this pin is deliberately not held to, which
// is how cmd/audit_e2e_coverage reads a date held to a window of its own.
func releaseProblems(source graphqlschema.ReleaseSource, release apilive.Source, now time.Time) []string {
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
	switch age, err := provenance.Age(source.RetrievedAt, now); {
	case err != nil:
		problems = append(problems, fmt.Sprintf(
			"the release record says the dump was read on %q, which is not a date: every regeneration writes the day, so no regeneration wrote this record",
			source.RetrievedAt,
		))
	case age < 0:
		problems = append(problems, fmt.Sprintf(
			"the release record says the dump was read on %s, which has not happened yet: no regeneration writes a day in the future",
			source.RetrievedAt,
		))
	}
	return problems
}

// writeArtifact writes one release artifact, as docgen.WriteOrCheck does in
// write mode. It is a seam so a test can fail a write after it has changed the
// file, which is how a full disk ends one and which no fixture on disk can
// arrange: a path that refuses the write refuses it before touching anything.
var writeArtifact = func(path string, content []byte) error {
	return docgen.WriteOrCheck(path, content, false, regenerateRelease)
}

// writeReleaseArtifacts writes the release schema and its record into dir.
// The text is committed uncompressed for the reasons writeArtifacts gives.
//
// The schema is written first, so a record that cannot be written would leave
// a new schema beside the old record. The check refuses that pair, since the
// schema no longer hashes to the record's digest, and the files are put back
// as the directory held them as well, the bytes of each that was there and no
// file where there was none, because a write can fail after it has changed
// the file it writes: a schema write that fails puts the schema back, and a
// record write that fails puts back both. A failed run therefore leaves the
// pair it found. A file that is there and cannot be read is not replaced at
// all: it could not be put back, and removing the file that took its place
// would lose it.
func writeReleaseArtifacts(dir, sdl string, source graphqlschema.ReleaseSource) error {
	schemaPath := filepath.Join(dir, graphqlschema.ReleaseSDLFileName)
	recordPath := filepath.Join(dir, graphqlschema.ReleaseSourceFileName)
	schemaBefore, schemaErr := readPrior(schemaPath)
	if schemaErr != nil {
		return schemaErr
	}
	recordBefore, recordErr := readPrior(recordPath)
	if recordErr != nil {
		return recordErr
	}
	if err := writeArtifact(schemaPath, []byte(sdl)); err != nil {
		return errors.Join(err, schemaBefore.restore())
	}
	// Marshaling a struct built one line earlier cannot fail.
	record := cmdutil.Must(json.MarshalIndent(source, "", "  "))
	if err := writeArtifact(recordPath, record); err != nil {
		return errors.Join(err, schemaBefore.restore(), recordBefore.restore())
	}
	return nil
}

// priorFile is what a path held before writeReleaseArtifacts wrote it: its
// bytes, or that there was no file.
type priorFile struct {
	path    string
	content []byte
	existed bool
}

// readPrior reads what path holds before it is written. A path that does not
// stat holds no file, which is what a run into a directory not yet created
// meets; one that stats and cannot be read is refused, since it could not be
// put back.
func readPrior(path string) (priorFile, error) {
	prior := priorFile{path: path}
	if _, statErr := os.Stat(path); statErr == nil {
		content, err := os.ReadFile(path)
		if err != nil {
			return priorFile{}, fmt.Errorf("read %s, which a failed write would put back: %w", path, err)
		}
		prior.content, prior.existed = content, true
	}
	return prior, nil
}

// restore puts back what the path held. The bytes are written as they were
// read, not through writeArtifact, which would give a file that lacked one the
// trailing newline every artifact ends with. A file that was not there is
// removed. One the failed write never created is already as it was found, so
// the removal's answer is not reported then: it finds nothing there, or finds
// a file where the path's directory should be, which is what refused a write
// into that directory (Windows reports the second as the first).
func (p priorFile) restore() error {
	if p.existed {
		return os.WriteFile(p.path, p.content, docgen.GeneratedFileMode)
	}
	if err := os.Remove(p.path); !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
		return err
	}
	return nil
}

// readReleaseArtifacts loads the committed release schema and record from dir,
// from disk rather than the embedded copies for the reason readArtifacts
// gives, and returns the count the schema loads with and the digest of its
// bytes, the two things the record is held to.
func readReleaseArtifacts(dir string) (types int, sdlDigest string, source graphqlschema.ReleaseSource, err error) {
	schemaPath := filepath.Join(dir, graphqlschema.ReleaseSDLFileName)
	sdl, err := os.ReadFile(schemaPath)
	if err != nil {
		return 0, "", graphqlschema.ReleaseSource{}, fmt.Errorf("read %s: %w", schemaPath, err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return 0, "", graphqlschema.ReleaseSource{}, fmt.Errorf("%s: %w", schemaPath, err)
	}

	recordPath := filepath.Join(dir, graphqlschema.ReleaseSourceFileName)
	record, err := os.ReadFile(recordPath)
	if err != nil {
		return 0, "", graphqlschema.ReleaseSource{}, fmt.Errorf("read %s: %w", recordPath, err)
	}
	source, err = graphqlschema.ParseReleaseSource(record)
	if err != nil {
		return 0, "", graphqlschema.ReleaseSource{}, fmt.Errorf("%s: %w", recordPath, err)
	}
	return len(schema.Types), hexDigest(sdl), source, nil
}

// hexDigest is the SHA-256 of data as the record spells it.
func hexDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// The two command lines that would read -from and then never open the file.
var (
	// errReleaseFlags refuses -from without -release, which would otherwise be
	// ignored by the gitlab.com pin's run that has no use for it.
	errReleaseFlags = errors.New("-from names a release schema dump and needs -release")
	// errCheckFrom refuses -from with -check. The check judges the committed
	// pair and reads no dump, so it would report success naming the committed
	// record while the file it was handed went unread, which reads as that
	// file having been checked.
	errCheckFrom = errors.New("-from names a dump to pin and -check judges only the committed pin, which never reads it: pin the dump without -check, then run the check")
)

// fromFlagProblem is the refusal a -from value earns on this command line, nil
// when it is empty or names a dump a release generation will read.
func fromFlagProblem(from string, release, check bool) error {
	switch {
	case from == "":
		return nil
	case !release:
		return errReleaseFlags
	case check:
		return errCheckFrom
	default:
		return nil
	}
}
