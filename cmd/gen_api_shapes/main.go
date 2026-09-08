package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apishapes"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/provenance"
)

const (
	// prefix names this command on every line it writes, so a failure in a
	// composite make target says which of them produced it.
	prefix = "gen_api_shapes:"
	// defaultRef is the branch GitLab generates the document on. A tag would be
	// reproducible where this is not, and would also pin the gate to a release
	// older than the GitLab a self-managed instance is about to run, which is
	// the drift this record exists to notice. The retrieval date and the
	// document's own digest are what make a regeneration comparable instead.
	defaultRef = "master"
	// defaultDir is where the committed record lives, beside the request
	// inventory it is compared with. It is the package's own constant so the
	// generator and every reader join on one directory.
	defaultDir = apishapes.DefaultDir
	// fetchTimeout bounds one whole download. The document is under 4 MB.
	fetchTimeout = 2 * time.Minute
	// note is written into the record so a reader who opens it first learns
	// what an empty list in it means.
	note = "What GitLab's own generated OpenAPI document says each REST operation accepts and returns, extracted by cmd/gen_api_shapes. " +
		"An empty response list means the document names no schema for that operation, not that GitLab sends nothing. " +
		"Paths are spelled the way GitLab spells them, /api/v4 prefix and {braces} included."
)

// genRun is one configured run: which ref to ask for, where to write, and
// whether to ask anything at all.
type genRun struct {
	ref    string
	dir    string
	check  bool
	client *http.Client
	// url overrides the fetch target, so a test drives a local server rather
	// than gitlab.com.
	url string
	// now supplies the day recorded, as a parameter so a test can assert on
	// the record it produced.
	now func() time.Time
}

func (c genRun) target() string {
	if c.url != "" {
		return c.url
	}
	return fmt.Sprintf(apishapes.RawURLTemplate, c.ref)
}

func main() {
	ref := flag.String("ref", defaultRef, "gitlab-org/gitlab ref to read the OpenAPI document from")
	dir := flag.String("dir", defaultDir, "directory holding the committed record")
	check := flag.Bool("check", false, "read the committed record instead of fetching, and fail when it is not usable")
	flag.Parse()

	os.Exit(run(genRun{
		ref:    *ref,
		dir:    *dir,
		check:  *check,
		client: &http.Client{Timeout: fetchTimeout},
		now:    time.Now,
	}, os.Stdout, os.Stderr))
}

// run is main with its streams, its clock and its HTTP client handed to it, so
// the ways this command ends are reachable from a test instead of only from a
// process.
func run(cfg genRun, out, errOut io.Writer) int {
	if cfg.check {
		return checkRecord(cfg, out, errOut)
	}
	return generate(cfg, out, errOut)
}

// checkRecord is the CI half: it proves the committed record is one this build
// can read and is not stale. It needs no network, which is what lets it gate.
func checkRecord(cfg genRun, out, errOut io.Writer) int {
	doc, err := apishapes.Read(cfg.dir)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	problems := recordProblems(doc, provenance.Clock(cfg.now))
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(errOut, prefix, problem)
		}
		fmt.Fprintln(errOut, prefix+" refresh it with `make gen-api-shapes`")
		return 1
	}

	fmt.Fprintf(out, prefix+" the record is usable; %s\n", doc.Source)
	return 0
}

// recordProblems reports every way the committed record fails to be one a
// comparison can rest on.
func recordProblems(doc apishapes.Document, now time.Time) []string {
	var problems []string
	if len(doc.Operations) < apishapes.MinimumOperations {
		problems = append(problems, fmt.Sprintf(
			"the record carries %d operations and GitLab's API has more than %d: the download was truncated, or the document read was not this one",
			len(doc.Operations), apishapes.MinimumOperations,
		))
	}
	if doc.Source.Ref == "" || doc.Source.SHA256 == "" {
		problems = append(problems,
			"the record does not say which ref it came from or what it hashed: nothing can then say which GitLab the gate speaks for")
	}
	return append(problems, provenance.Problems(provenance.Subject{
		Noun:        "record",
		Consequence: "GitLab narrows fields in place, so one this old can no longer report a field that changed since",
	}, doc.Source.RetrievedAt, now)...)
}

// generate is the network half: fetch, extract, and write.
func generate(cfg genRun, out, errOut io.Writer) int {
	url := cfg.target()
	fmt.Fprintf(out, prefix+" fetching %s\n", url)

	document, err := fetch(cfg, url)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	operations, openAPIVersion, apiVersion, err := apishapes.Extract(document)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	// A short extraction is refused before it is written, so a truncated
	// download cannot replace a whole record and report success. The floor is
	// the one --check refuses a record with, so both sides stop believing a
	// document at the same count.
	if len(operations) < apishapes.MinimumOperations {
		fmt.Fprintf(errOut, prefix+" %s yielded %d operations and GitLab's API has more than %d: refusing to replace the record with a truncated read\n",
			url, len(operations), apishapes.MinimumOperations)
		return 1
	}

	digest := sha256.Sum256(document)
	doc := apishapes.Document{
		Note: note,
		Source: apishapes.Source{
			URL:            url,
			Ref:            cfg.ref,
			RetrievedAt:    provenance.Clock(cfg.now).UTC().Format(time.DateOnly),
			SHA256:         hex.EncodeToString(digest[:]),
			OpenAPIVersion: openAPIVersion,
			APIVersion:     apiVersion,
			Operations:     len(operations),
		},
		Operations: operations,
	}
	if writeErr := apishapes.Write(cfg.dir, doc); writeErr != nil {
		fmt.Fprintln(errOut, prefix, writeErr)
		return 1
	}

	withResponse, withBody := 0, 0
	for _, op := range operations {
		if len(op.Response) > 0 {
			withResponse++
		}
		if len(op.Body) > 0 {
			withBody++
		}
	}
	fmt.Fprintf(out, prefix+" wrote %s; %d operations, %d with a response schema, %d with a request body\n",
		apishapes.FileName, len(operations), withResponse, withBody)
	return 0
}

// fetch downloads the document.
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
