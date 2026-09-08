package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apiexposes"
)

const (
	// prefix names this command on every line it writes, so a failure in a
	// composite make target says which of them produced it.
	prefix = "gen_api_exposes:"
	// defaultRef is the branch GitLab develops on, for the reason
	// gen_api_shapes gives: a tag would pin the record to a release older
	// than the GitLab a self-managed instance is about to run. The commit the
	// archives name and the retrieval date are what make two records
	// comparable.
	defaultRef = "master"
	// defaultBase is where the archives and the feature table are fetched
	// from. The project id is gitlab-org/gitlab's; the archive endpoint takes
	// a subtree path, which is what keeps three directories from being a
	// clone of the whole repository.
	defaultBase = "https://gitlab.com"
	projectID   = "278964"
	// maxAge is how long a record may stand before --check refuses it: the
	// window the OpenAPI record and the GraphQL pin stand for, for the same
	// reason.
	maxAge = 180 * 24 * time.Hour
	// fetchTimeout bounds one download. The largest archive is under 100 KB.
	fetchTimeout = 2 * time.Minute
	// featuresPath is GitLab's licensed-feature table.
	featuresPath = "ee/app/models/gitlab_subscriptions/features.rb"
	note         = "The condition under which GitLab sends each field a REST entity exposes, read out of gitlab-org/gitlab's Grape entities by cmd/gen_api_exposes. " +
		"An entity's fields are its own in declaration order, Enterprise prepends last; the parent's come first in what GitLab sends and are listed under the parent. " +
		"A field without a condition is always sent when the entity is; a field with one is sent only when it holds, and its tier is the license tier the named feature belongs to."
)

// entityDirs are the subtrees holding entities, fetched in this order, which
// is also the order the digest covers them in.
var entityDirs = []string{"lib/api/entities", "ee/lib/api/entities", "ee/lib/ee/api/entities"} //nolint:gochecknoglobals // the fixed list of sources

// archiveCommit reads the commit an archive's root directory names:
// gitlab-<ref>-<commit>-<path>.
var archiveCommit = regexp.MustCompile(`-([0-9a-f]{40})-`)

// genRun is one configured run.
type genRun struct {
	ref    string
	dir    string
	check  bool
	report string
	// source reads the entities from a local checkout instead of fetching,
	// for a run without the network or against a tree GitLab has not pushed.
	source string
	client *http.Client
	// base overrides the host, so a test drives a local server rather than
	// gitlab.com.
	base string
	// now supplies the day recorded, as a parameter so a test can assert on
	// the record it produced.
	now func() time.Time
}

func (c genRun) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func main() {
	ref := flag.String("ref", defaultRef, "gitlab-org/gitlab ref to read the entities from")
	dir := flag.String("dir", apiexposes.DefaultDir, "directory holding the committed record")
	check := flag.Bool("check", false, "read the committed record instead of fetching, and fail when it is not usable")
	report := flag.String("report", "", "print every field the named entity sends, with its conditions, from the committed record")
	source := flag.String("source", "", "read the entities from this local checkout of gitlab-org/gitlab instead of fetching")
	flag.Parse()

	os.Exit(run(genRun{
		ref:    *ref,
		dir:    *dir,
		check:  *check,
		report: *report,
		source: *source,
		client: &http.Client{Timeout: fetchTimeout},
		base:   defaultBase,
		now:    time.Now,
	}, os.Stdout, os.Stderr))
}

// run is main with its streams, its clock and its HTTP client handed to it.
func run(cfg genRun, out, errOut io.Writer) int {
	switch {
	case cfg.check:
		return checkRecord(cfg, out, errOut)
	case cfg.report != "":
		return reportEntity(cfg, out, errOut)
	default:
		return generate(cfg, out, errOut)
	}
}

// checkRecord is the CI half: it proves the committed record is one this
// build can read and is not stale. It needs no network.
func checkRecord(cfg genRun, out, errOut io.Writer) int {
	doc, err := apiexposes.Read(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	problems := recordProblems(doc, cfg.clock())
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(errOut, prefix, problem)
		}
		fmt.Fprintln(errOut, prefix+" refresh it with `make gen-api-exposes`")
		return 1
	}
	fmt.Fprintf(out, prefix+" the record is usable; %s\n", doc.Source)
	return 0
}

// recordProblems reports every way the committed record fails to be one a
// reader can rest on.
func recordProblems(doc apiexposes.Document, now time.Time) []string {
	var problems []string
	if len(doc.Entities) < apiexposes.MinimumEntities {
		problems = append(problems, fmt.Sprintf(
			"the record carries %d entities and GitLab declares more than %d: the download was truncated, or the tree read was not this one",
			len(doc.Entities), apiexposes.MinimumEntities,
		))
	}
	if len(doc.Features) < apiexposes.MinimumFeatures {
		problems = append(problems, fmt.Sprintf(
			"the record carries %d licensed features and GitLab's table lists more than %d: the table read was not this one",
			len(doc.Features), apiexposes.MinimumFeatures,
		))
	}
	if doc.Source.Ref == "" || doc.Source.Commit == "" || doc.Source.SHA256 == "" {
		problems = append(problems,
			"the record does not say which ref and commit it came from or what it hashed: nothing can then say which GitLab it speaks for")
	}
	switch age, err := recordAge(doc, now); {
	case err != nil:
		problems = append(problems, fmt.Sprintf(
			"the record says it was taken on %q, which is not a date: nothing can then say how old it is",
			doc.Source.RetrievedAt,
		))
	case age < 0:
		problems = append(problems, fmt.Sprintf(
			"the record says it was taken on %s, which has not happened yet: no regeneration writes a day in the future",
			doc.Source.RetrievedAt,
		))
	case age > maxAge:
		problems = append(problems, fmt.Sprintf(
			"the record is %d days old and the window is %d: GitLab moves fields behind and out from under conditions release by release, so one this old no longer says when a field is sent",
			int(age.Hours()/24), int(maxAge.Hours()/24),
		))
	}
	return problems
}

func recordAge(doc apiexposes.Document, now time.Time) (time.Duration, error) {
	retrieved, err := time.Parse(time.DateOnly, doc.Source.RetrievedAt)
	if err != nil {
		return 0, err
	}
	return now.UTC().Sub(retrieved), nil
}

// reportEntity prints what one entity sends, field by field, the way a
// reviewer reads it: the parent chain's fields first, then the entity's own,
// each with its condition, tier and edition when it has them.
func reportEntity(cfg genRun, out, errOut io.Writer) int {
	doc, err := apiexposes.Read(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	name := apiexposes.OpenAPIName(cfg.report)
	fields, ok := doc.Effective(name)
	if !ok {
		fmt.Fprintf(errOut, prefix+" %s is not an entity the record holds (%d entities; names look like APIEntitiesProject)\n", name, len(doc.Entities))
		return 1
	}
	entity := doc.Entities[name]
	fmt.Fprintf(out, "%s (%s:%d", name, entity.File, entity.Line)
	if entity.Parent != "" {
		fmt.Fprintf(out, ", inherits %s", entity.Parent)
	}
	if entity.Edition != "" {
		fmt.Fprintf(out, ", %s only", entity.Edition)
	}
	fmt.Fprintf(out, "): %d field(s)\n", len(fields))
	printFields(out, fields, "  ")
	return 0
}

// printFields renders fields one per line, nested ones indented under theirs.
func printFields(out io.Writer, fields []apiexposes.Field, indent string) {
	for _, field := range fields {
		var notes []string
		if field.Tier != "" {
			notes = append(notes, field.Tier)
		} else if field.Edition != "" {
			notes = append(notes, field.Edition)
		}
		if field.Using != "" {
			notes = append(notes, "as "+field.Using)
		}
		if field.If != "" {
			notes = append(notes, "if "+field.If)
		}
		if field.Unless != "" {
			notes = append(notes, "unless "+field.Unless)
		}
		fmt.Fprintf(out, "%s%s", indent, field.Name)
		if len(notes) > 0 {
			fmt.Fprintf(out, "  [%s]", strings.Join(notes, "; "))
		}
		fmt.Fprintln(out)
		printFields(out, field.Nested, indent+"  ")
	}
}

// generate reads the sources, from a checkout or the network, and writes the
// record.
func generate(cfg genRun, out, errOut io.Writer) int {
	var (
		files   map[string][]byte
		table   []byte
		commit  string
		digest  string
		err     error
		fetched string
	)
	if cfg.source != "" {
		files, table, err = readCheckout(cfg.source)
		commit, digest, fetched = "local checkout", "", cfg.source
	} else {
		files, table, commit, digest, err = fetchAll(cfg)
		fetched = cfg.base
	}
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	features := apiexposes.ParseFeatures(table)
	entities, exposes, err := apiexposes.Parse(files, features)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	// A short extraction is refused before it is written, so a truncated
	// download cannot replace a whole record and report success. The floors
	// are the ones --check refuses a record with.
	if len(entities) < apiexposes.MinimumEntities || len(features) < apiexposes.MinimumFeatures {
		fmt.Fprintf(errOut, prefix+" %s yielded %d entities and %d licensed features, and GitLab has more than %d and %d: refusing to replace the record with a truncated read\n",
			fetched, len(entities), len(features), apiexposes.MinimumEntities, apiexposes.MinimumFeatures)
		return 1
	}

	doc := apiexposes.Document{
		Note: note,
		Source: apiexposes.Source{
			Ref:         cfg.ref,
			Commit:      commit,
			RetrievedAt: cfg.clock().UTC().Format(time.DateOnly),
			SHA256:      digest,
			Files:       len(files),
			Entities:    len(entities),
			Exposes:     exposes,
			Features:    len(features),
		},
		Entities: entities,
		Features: features,
	}
	if writeErr := apiexposes.Write(cfg.dir, doc); writeErr != nil {
		fmt.Fprintln(errOut, prefix, writeErr)
		return 1
	}
	fmt.Fprintf(out, prefix+" wrote %s; %s, %d exposes in %d files\n", apiexposes.FileName, doc.Source, exposes, len(files))
	return 0
}

// fetchAll downloads the three entity subtrees and the feature table at the
// ref, and returns the files keyed by repository path, the commit every
// archive named, and the digest of everything downloaded in order.
func fetchAll(cfg genRun) (files map[string][]byte, table []byte, commit, digest string, err error) {
	hasher := sha256.New()
	files = map[string][]byte{}
	for _, dir := range entityDirs {
		archive, fetchErr := fetch(cfg, archiveURL(cfg.base, cfg.ref, dir))
		if fetchErr != nil {
			return nil, nil, "", "", fetchErr
		}
		hasher.Write(archive)
		found, named, readErr := readArchive(archive)
		if readErr != nil {
			return nil, nil, "", "", fmt.Errorf("%s: %w", dir, readErr)
		}
		if commit == "" {
			commit = named
		} else if named != commit {
			return nil, nil, "", "", fmt.Errorf("%s resolved %s to %s where an earlier archive resolved it to %s: the ref moved between downloads, run again",
				dir, cfg.ref, named, commit)
		}
		maps.Copy(files, found)
	}
	table, err = fetch(cfg, rawURL(cfg.base, cfg.ref, featuresPath))
	if err != nil {
		return nil, nil, "", "", err
	}
	hasher.Write(table)
	return files, table, commit, hex.EncodeToString(hasher.Sum(nil)), nil
}

// archiveURL is the subtree archive of one directory at a ref.
func archiveURL(base, ref, dir string) string {
	return fmt.Sprintf("%s/api/v4/projects/%s/repository/archive.tar.gz?sha=%s&path=%s", base, projectID, ref, dir)
}

// rawURL is one file at a ref.
func rawURL(base, ref, path string) string {
	return fmt.Sprintf("%s/gitlab-org/gitlab/-/raw/%s/%s", base, ref, path)
}

// fetch downloads one URL.
func fetch(cfg genRun, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build the request for %s: %w", url, err)
	}
	response, err := cfg.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %s", url, response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil
}

// readArchive reads the Ruby files out of a subtree archive, keyed by their
// path in the repository, and the commit the archive's root directory names.
func readArchive(archive []byte) (files map[string][]byte, commit string, err error) {
	unzipped, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, "", fmt.Errorf("the archive is not gzip: %w", err)
	}
	files = map[string][]byte{}
	reader := tar.NewReader(unzipped)
	for {
		header, next := reader.Next()
		if errors.Is(next, io.EOF) {
			break
		}
		if next != nil {
			return nil, "", fmt.Errorf("read the archive: %w", next)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			// git archive opens with a pax global header carrying the commit
			// as a comment; the root directory below it says the same thing
			// in the form every archive has, so the header is passed over.
			continue
		}
		root, rest, _ := strings.Cut(header.Name, "/")
		if commit == "" {
			match := archiveCommit.FindStringSubmatch(root)
			if match == nil {
				return nil, "", fmt.Errorf("the archive's root directory %q names no commit", root)
			}
			commit = match[1]
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(rest, ".rb") {
			continue
		}
		content, readErr := io.ReadAll(reader)
		if readErr != nil {
			return nil, "", fmt.Errorf("read %s: %w", rest, readErr)
		}
		files[rest] = content
	}
	if commit == "" {
		return nil, "", errors.New("the archive is empty")
	}
	return files, commit, nil
}

// readCheckout reads the same sources out of a local checkout. The checkout
// is opened as a root, so every path read stays inside it whatever a symlink
// in it points at.
func readCheckout(root string) (files map[string][]byte, table []byte, err error) {
	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read the checkout: %w", err)
	}
	defer func() { _ = opened.Close() }()
	tree := opened.FS()

	files = map[string][]byte{}
	for _, dir := range entityDirs {
		walkErr := fs.WalkDir(tree, dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".rb") {
				return nil
			}
			content, readErr := fs.ReadFile(tree, path)
			if readErr != nil {
				return readErr
			}
			files[path] = content
			return nil
		})
		if walkErr != nil {
			return nil, nil, fmt.Errorf("read %s: %w", dir, walkErr)
		}
	}
	table, err = fs.ReadFile(tree, featuresPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read the feature table: %w", err)
	}
	return files, table, nil
}
