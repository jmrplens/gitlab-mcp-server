// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// The registries verify-published reads back, anonymously: npm's registry,
// PyPI's JSON and integrity APIs and nuget.org's flat container, the one the
// SDK itself installs from.
const (
	pypiIntegrity     = "https://pypi.org/integrity/"
	verifyUserAgent   = "gitlab-mcp-server-release-audit"
	inTotoStatement   = "https://in-toto.io/Statement/v1"
	pypiPublishType   = "https://docs.pypi.org/attestations/publish/v1"
	verifyMaxDownload = 1 << 30
)

// The allowances' defaults: seconds of sleep the npm and PyPI checks share,
// the pause between attempts, and the NuGet deadline counted from the start
// of the run. The budget is shared because the failures are correlated:
// either a registry has the version or it does not, and a budget per URL
// would turn one unpublished release into an hour of CI. NuGet's ends at an
// instant, since nuget.org validates while npm and PyPI are waited for: 3.1.0's
// packages were served 23 minutes after this check started.
const (
	defaultRetryBudget   = 600.0
	defaultRetryDelay    = 20.0
	defaultNugetDeadline = 2400.0
)

// pypiPublisher is the trusted publisher every wheel's provenance has to
// come from, as PyPI's integrity API names it, in the order a message names
// its fields.
var pypiPublisher = [][2]string{
	{"kind", "GitHub"},
	{"repository", "jmrplens/gitlab-mcp-server"},
	{"workflow", "release.yml"},
	{"environment", "release"},
}

// wheelFragments map a wheel file name's platform fragment to the release
// asset it carries, in the order a file name is matched against them.
var wheelFragments = [][2]string{
	{"manylinux_2_17_x86_64", "gitlab-mcp-server-linux-amd64"},
	{"manylinux_2_17_aarch64", "gitlab-mcp-server-linux-arm64"},
	{"macosx_11_0_x86_64", "gitlab-mcp-server-darwin-amd64"},
	{"macosx_11_0_arm64", "gitlab-mcp-server-darwin-arm64"},
	{"win_amd64", "gitlab-mcp-server-windows-amd64.exe"},
	{"win_arm64", "gitlab-mcp-server-windows-arm64.exe"},
}

// allowance is a wait between download attempts: the shared budget of npm
// and PyPI, or NuGet's deadline.
type allowance interface {
	wait() bool
}

// retryBudget is an allowance of seconds slept, shared between downloads.
type retryBudget struct {
	remaining, delay float64
	waits            int
	sleep            func(time.Duration)
}

func (b *retryBudget) wait() bool {
	if b.delay <= 0 || b.remaining < b.delay {
		return false
	}
	b.remaining -= b.delay
	b.waits++
	b.sleep(seconds(b.delay))
	return true
}

// retryDeadline is an allowance of waiting that ends at an instant fixed
// when it is made, so time spent before its first wait counts against it. A
// wait is allowed only when it would end by the deadline.
type retryDeadline struct {
	seconds, delay float64
	end            time.Time
	waits          int
	now            func() time.Time
	sleep          func(time.Duration)
}

// remaining is the seconds left until the deadline, never negative.
func (d *retryDeadline) remaining() float64 { return max(0, d.end.Sub(d.now()).Seconds()) }

func (d *retryDeadline) wait() bool {
	if d.delay <= 0 || d.remaining() < d.delay {
		return false
	}
	d.waits++
	d.sleep(seconds(d.delay))
	return true
}

// seconds is a float of seconds as a duration.
func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// fetchError is a download that never succeeded, with how many attempts it
// took to know that.
type fetchError struct {
	attempts int
	cause    error
}

func (e *fetchError) Error() string {
	return fmt.Sprintf("%v (still failing after %d attempt(s))", e.cause, e.attempts)
}

// httpStatusError is a registry's answer with an error status.
type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string {
	return "HTTP " + strconv.Itoa(e.code) + " " + http.StatusText(e.code)
}

// verifyPublished is one run of verify-published.
type verifyPublished struct {
	version  string
	digests  map[string]string
	stdout   io.Writer
	client   *http.Client
	budget   *retryBudget
	deadline *retryDeadline
	problems []string
}

// sayf writes one line of progress.
func (v *verifyPublished) sayf(format string, args ...any) {
	fmt.Fprintf(v.stdout, format+"\n", args...)
}

// failf records one problem.
func (v *verifyPublished) failf(format string, args ...any) {
	v.problems = append(v.problems, fmt.Sprintf(format, args...))
}

// fetch GETs rawURL, spending wait, the budget or the deadline, on anything
// that fails in transit, a 404 included: minutes after an upload it cannot be
// told from propagation lag. It is not forgiven, only waited for. A body in
// hand is never asked for again, so a mismatch is reported once.
func (v *verifyPublished) fetch(rawURL string, wait allowance) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		body, err := v.get(rawURL)
		if err == nil {
			return body, nil
		}
		if wait == nil || !wait.wait() {
			return nil, &fetchError{attempts: attempt, cause: err}
		}
		v.sayf("  .. %s: %v; retrying (attempt %d)", rawURL, err, attempt+1)
	}
}

// get makes one request.
func (v *verifyPublished) get(rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", verifyUserAgent)
	resp, err := v.client.Do(req) //#nosec G704 -- the fixed registries a release publishes to
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &httpStatusError{code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, verifyMaxDownload))
}

// runVerifyPublished is the verify-published subcommand: it compares what
// npm, PyPI and NuGet serve with the release's signed checksums.txt, holds
// each wheel to its PEP 740 provenance, and, given the digests the nuget job
// attested, each NuGet package to them once nuget.org's repository signature
// is removed. It runs after the publishes, in a job holding no publishing
// credential, and is the one step that reads back the bytes a user installs.
func runVerifyPublished(root string, args []string, stdout, stderr io.Writer) int {
	options, err := parseVerifyArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s verify-published: %v\n", toolName, err)
		return 2
	}
	return verifyRun(root, options, stdout, stderr, &http.Client{Timeout: 120 * time.Second}, time.Now, time.Sleep)
}

// verifyOptions is the command line of verify-published.
type verifyOptions struct {
	version, checksums, nugetDigests  string
	skipNpm, skipPypi, skipNuget      bool
	retryBudget, retryDelay, deadline float64
}

// parseVerifyArgs reads the version and checksums.txt, in either order with
// the flags, as the script read them.
func parseVerifyArgs(args []string) (verifyOptions, error) {
	o := verifyOptions{retryBudget: defaultRetryBudget, retryDelay: defaultRetryDelay, deadline: defaultNugetDeadline}
	var positional []string
	number := func(i int, name string, into *float64) (int, error) {
		if i+1 >= len(args) {
			return i, fmt.Errorf("%s needs a value", name)
		}
		value, err := strconv.ParseFloat(args[i+1], 64)
		if err != nil {
			return i, fmt.Errorf("%s needs a number of seconds, got %q", name, args[i+1])
		}
		*into = value
		return i + 1, nil
	}
	for i := 0; i < len(args); i++ {
		var err error
		switch arg := args[i]; arg {
		case "--skip-npm":
			o.skipNpm = true
		case "--skip-pypi":
			o.skipPypi = true
		case "--skip-nuget":
			o.skipNuget = true
		case "--nuget-digests":
			if i+1 >= len(args) {
				return o, errors.New("--nuget-digests needs a file")
			}
			i++
			o.nugetDigests = args[i]
		case "--retry-budget":
			i, err = number(i, arg, &o.retryBudget)
		case "--retry-delay":
			i, err = number(i, arg, &o.retryDelay)
		case "--nuget-deadline":
			i, err = number(i, arg, &o.deadline)
		default:
			if strings.HasPrefix(arg, "-") {
				return o, fmt.Errorf("unknown option %s", arg)
			}
			positional = append(positional, arg)
		}
		if err != nil {
			return o, err
		}
	}
	if len(positional) != 2 {
		return o, errors.New("takes the release version and the release's checksums.txt (or the directory holding it)")
	}
	o.version, o.checksums = positional[0], positional[1]
	return o, nil
}

// verifyRun runs the comparison with the client, the clock and the sleep
// given, seams so a test drives forty minutes of deadline in no time.
func verifyRun(root string, o verifyOptions, stdout, stderr io.Writer, client *http.Client, now func() time.Time, sleep func(time.Duration)) int {
	// Made before anything is read: the deadline is counted from the start,
	// and --retry-budget 0, the out-of-band run days later, waits for
	// nothing at all.
	nugetSeconds := 0.0
	if o.retryBudget > 0 {
		nugetSeconds = o.deadline
	}
	v := &verifyPublished{
		version: o.version, stdout: stdout, client: client,
		deadline: &retryDeadline{seconds: nugetSeconds, delay: o.retryDelay, end: now().Add(seconds(nugetSeconds)), now: now, sleep: sleep},
		budget:   &retryBudget{remaining: o.retryBudget, delay: o.retryDelay, sleep: sleep},
	}
	checksums := o.checksums
	if info, err := os.Stat(localPath(root, checksums)); err == nil && info.IsDir() {
		checksums = filepath.Join(checksums, checksumsName)
	}
	digests, err := releasedDigests(localPath(root, checksums))
	if err != nil {
		fmt.Fprintf(stderr, "%s verify-published: %v\n", toolName, err)
		return 1
	}
	v.digests = digests
	if _, notices := digests[noticesName]; len(digests) == 0 || notices && len(digests) == 1 {
		fmt.Fprintf(stderr, "%s verify-published: %s names none of the release binaries\n", toolName, checksums)
		return 1
	}
	v.sayf("Comparing published packages for v%s against %d entries in %s", o.version, len(digests), checksums)
	if _, notices := digests[noticesName]; !notices {
		v.sayf("  ..  %s names no %s, a release from before they were generated, so the packages' notices are not compared", checksums, noticesName)
	}
	if !o.skipNpm {
		v.checkNpm()
	}
	if !o.skipPypi {
		v.checkPypiProvenance(v.checkPypi())
	}
	var attested map[string]string
	if !o.skipNuget {
		if o.nugetDigests != "" {
			if attested, err = attestedDigests(localPath(root, o.nugetDigests)); err != nil {
				fmt.Fprintf(stderr, "%s verify-published: %v\n", toolName, err)
				return 1
			}
		} else {
			v.sayf("  ..  nuget: no --nuget-digests given, so the packages are checked by the binaries they carry only")
		}
		attested = v.checkNuget(attested, o.nugetDigests != "")
	}
	return v.finish(o, attested)
}

// finish reports the outcome and returns the exit code.
func (v *verifyPublished) finish(o verifyOptions, attested map[string]string) int {
	if len(v.problems) > 0 {
		v.sayf("\nFAILED (%d):", len(v.problems))
		for _, problem := range v.problems {
			v.sayf("  x %s", problem)
		}
		waited := v.budget.waits > 0 || v.deadline.waits > 0
		if waited {
			v.sayf("")
		}
		if v.budget.waits > 0 {
			v.sayf("npm and PyPI retried %d time(s); %ds of their %ds retry budget was left unspent.", v.budget.waits, int(v.budget.remaining), int(o.retryBudget))
		}
		if v.deadline.waits > 0 {
			v.sayf("NuGet retried %d time(s); %ds of its %ds deadline, counted from the start of this run, was left.", v.deadline.waits, int(v.deadline.remaining()), int(v.deadline.seconds))
		}
		if waited {
			v.sayf("An allowance spent to zero means a registry never served the version " +
				"(a download never came back, or the pointer's index never listed it), " +
				"which reads differently from a digest that did not match.")
		}
		return 1
	}
	v.sayf("\nEvery published package carries the binary the release signed.")
	if !o.skipPypi {
		v.sayf("Every wheel PyPI serves carries the publish attestation release.yml signed for its bytes.")
	}
	if len(attested) > 0 {
		v.sayf("Every NuGet package nuget.org serves is, without its repository signature, the one the release attested.")
	}
	return 0
}

// releasedDigests reads the digests the release's signed checksums.txt names
// for the six binaries and the notices, through cmd/internal/releaseassets,
// which refuses a line it cannot read rather than skipping it.
func releasedDigests(file string) (map[string]string, error) {
	sums, err := releaseassets.ReadChecksums(file)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{noticesName: true}
	for _, p := range releasePlatforms {
		wanted[p.asset] = true
	}
	digests := map[string]string{}
	for name, digest := range sums.Digests {
		if wanted[name] {
			digests[name] = digest
		}
	}
	return digests, nil
}

// attestedLine is a line the nuget job records for an attested package.
var attestedLine = regexp.MustCompile(`^[0-9a-f]{64}$`)

// attestedDigests reads the sha256sum lines the nuget job recorded for the
// packages it attested; a line that is not one is ignored.
func attestedDigests(file string) (map[string]string, error) {
	data, err := os.ReadFile(file) //#nosec G304 -- the digests file the release job wrote
	if err != nil {
		return nil, err
	}
	digests := map[string]string{}
	for line := range strings.SplitSeq(strings.ReplaceAll(string(data), "\r", ""), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		digest := strings.ToLower(fields[0])
		name := strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0])), "*")
		if attestedLine.MatchString(digest) && strings.HasSuffix(name, ".nupkg") {
			digests[name] = digest
		}
	}
	return digests, nil
}

// checkNotices holds one package's copy of the notices to the signed entry;
// a release whose checksums.txt names none predates them.
func (v *verifyPublished) checkNotices(label string, notices []byte, where string) {
	want, signed := v.digests[noticesName]
	switch {
	case !signed:
	case notices == nil:
		v.failf("%s: %s carries no %s, but the release signed one", label, where, noticesName)
	case releaseassets.Digest(notices) != want:
		v.failf("%s: %s in %s is sha256 %s, but the signed checksums.txt says %s", label, noticesName, where, releaseassets.Digest(notices), want)
	default:
		v.sayf("  ok  %-17s %s matches the signed one", label, noticesName)
	}
}

// checkNpm downloads each published platform package and holds its binary
// and notices to the signed ones.
func (v *verifyPublished) checkNpm() {
	for _, p := range releasePlatforms {
		want, named := v.digests[p.asset]
		if !named {
			v.failf("npm %s: checksums.txt does not name the release asset %s", p.npmKey, p.asset)
			continue
		}
		executable, notices, tarball, err := v.npmPackage(p)
		if err != nil {
			v.failf("npm %s: could not read the published package: %v", p.npmKey, err)
			continue
		}
		// Past this point the bytes are in hand, so nothing below is
		// retried.
		if got := releaseassets.Digest(executable); got != want {
			v.failf("npm %s: %s carries sha256 %s, but the signed checksums.txt says %s is %s", p.npmKey, tarball, got, p.asset, want)
		} else {
			v.sayf("  ok  npm %-13s matches %s", p.npmKey, p.asset)
		}
		v.checkNotices("npm "+p.npmKey, notices, tarball)
	}
}

// npmPackage downloads one platform package and returns its binary, its
// notices (nil when it carries none) and the tarball's URL.
func (v *verifyPublished) npmPackage(p releasePlatform) (executable, notices []byte, tarball string, err error) {
	name := npmPlatformName(p)
	body, err := v.fetch(npmRegistry+url.QueryEscape(name)+"/"+v.version, v.budget)
	if err != nil {
		return nil, nil, "", err
	}
	var meta struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	}
	if decodeErr := json.Unmarshal(body, &meta); decodeErr != nil || meta.Dist.Tarball == "" {
		return nil, nil, "", fmt.Errorf("%s@%s answers no dist.tarball", name, v.version)
	}
	tarball = meta.Dist.Tarball
	blob, err := v.fetch(tarball, v.budget)
	if err != nil {
		return nil, nil, tarball, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, nil, tarball, err
	}
	reader := tar.NewReader(gz)
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, nil, tarball, nextErr
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		base := path.Base(header.Name)
		switch {
		case base == binaryName || base == binaryName+".exe":
			executable, err = io.ReadAll(reader)
		case header.Name == "package/"+noticesName:
			notices, err = io.ReadAll(reader)
		}
		if err != nil {
			return nil, nil, tarball, err
		}
	}
	if executable == nil {
		return nil, nil, tarball, fmt.Errorf("%s@%s ships no gitlab-mcp-server binary", name, v.version)
	}
	return executable, notices, tarball, nil
}

// checkPypi holds each published wheel's binary and notices to the signed
// ones and returns the SHA-256 of every wheel read, by file name, for its
// provenance.
func (v *verifyPublished) checkPypi() map[string]string {
	served := map[string]string{}
	body, err := v.fetch(pypiAPI+pypiDist+"/"+v.version+"/json", v.budget)
	var meta struct {
		URLs []struct {
			PackageType string `json:"packagetype"`
			Filename    string `json:"filename"`
			URL         string `json:"url"`
		} `json:"urls"`
	}
	if err == nil {
		err = json.Unmarshal(body, &meta)
	}
	if err != nil {
		v.failf("pypi: could not list the published wheels: %v", err)
		return served
	}
	seen := map[string]bool{}
	for _, entry := range meta.URLs {
		if entry.PackageType != "bdist_wheel" {
			continue
		}
		asset := ""
		for _, fragment := range wheelFragments {
			if strings.Contains(entry.Filename, fragment[0]) {
				asset = fragment[1]
				break
			}
		}
		if asset == "" {
			v.failf("pypi %s: no release asset corresponds to this platform tag", entry.Filename)
			continue
		}
		seen[asset] = true
		want, named := v.digests[asset]
		if !named {
			v.failf("pypi %s: checksums.txt does not name the release asset %s", entry.Filename, asset)
			continue
		}
		executable, notices, digest, wheelErr := v.wheelContents(entry.URL)
		if wheelErr != nil {
			v.failf("pypi %s: could not read the wheel: %v", entry.Filename, wheelErr)
			continue
		}
		served[entry.Filename] = digest
		if got := releaseassets.Digest(executable); got != want {
			v.failf("pypi %s: carries sha256 %s, but the signed checksums.txt says %s is %s", entry.Filename, got, asset, want)
		} else {
			v.sayf("  ok  pypi %-60s matches %s", entry.Filename, asset)
		}
		v.checkNotices("pypi "+entry.Filename, notices, entry.URL)
	}
	var missing []string
	for _, fragment := range wheelFragments {
		if !seen[fragment[1]] {
			missing = append(missing, fragment[1])
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		v.failf("pypi: no wheel published for %s", strings.Join(missing, ", "))
	}
	return served
}

// wheelContents downloads one wheel and returns its binary, its notices (nil
// when it carries none) and the SHA-256 of the whole wheel.
func (v *verifyPublished) wheelContents(rawURL string) (executable, notices []byte, digest string, err error) {
	blob, err := v.fetch(rawURL, v.budget)
	if err != nil {
		return nil, nil, "", err
	}
	archive, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return nil, nil, "", err
	}
	files, names := zipIndex(archive)
	if f, ok := files[fmt.Sprintf("%s-%s.dist-info/licenses/%s", pypiNorm, v.version, noticesName)]; ok {
		if notices, err = readZipEntry(f); err != nil {
			return nil, nil, "", err
		}
	}
	prefix := fmt.Sprintf("%s-%s.data/scripts/", pypiNorm, v.version)
	for _, name := range names {
		if strings.HasPrefix(name, prefix) && strings.HasPrefix(path.Base(name), binaryName) {
			executable, err = readZipEntry(files[name])
			return executable, notices, releaseassets.Digest(blob), err
		}
	}
	return nil, nil, "", fmt.Errorf("%s ships no binary under %s", rawURL, prefix)
}

// provenanceBundle is one bundle of a provenance document: its publisher
// and the in-toto statements its attestations sign.
type provenanceBundle struct {
	publisher  map[string]any
	statements []map[string]any
}

// provenanceBundles reads a PEP 740 provenance document, refusing one that
// is not version 1 and an attestation whose statement is not base64 of a
// JSON object.
func provenanceBundles(body []byte) ([]provenanceBundle, error) {
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	rawBundles, isList := document["attestation_bundles"].([]any)
	if document["version"] != float64(1) || !isList {
		return nil, errors.New("it is not a version 1 provenance")
	}
	var bundles []provenanceBundle
	for _, raw := range rawBundles {
		bundle, _ := raw.(map[string]any)
		publisher, hasPublisher := bundle["publisher"].(map[string]any)
		attestations, hasAttestations := bundle["attestations"].([]any)
		if !hasPublisher || !hasAttestations {
			return nil, errors.New("a bundle names no publisher or no attestations")
		}
		entry := provenanceBundle{publisher: publisher}
		for _, rawAttestation := range attestations {
			attestation, _ := rawAttestation.(map[string]any)
			envelope, _ := attestation["envelope"].(map[string]any)
			encoded, isText := envelope["statement"].(string)
			if !isText {
				return nil, errors.New("an attestation's statement is not base64 of a JSON object: it carries no statement")
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("an attestation's statement is not base64 of a JSON object: %w", err)
			}
			var signed any
			if err = json.Unmarshal(decoded, &signed); err != nil {
				return nil, fmt.Errorf("an attestation's statement is not base64 of a JSON object: %w", err)
			}
			statement, isObject := signed.(map[string]any)
			if !isObject {
				return nil, errors.New("an attestation's statement is not a JSON object")
			}
			entry.statements = append(entry.statements, statement)
		}
		bundles = append(bundles, entry)
	}
	return bundles, nil
}

// pyText is a decoded JSON value as Python's str() spells it in a message:
// a string as itself, an absent value as None, a boolean capitalized.
func pyText(value any) string {
	switch value := value.(type) {
	case nil:
		return "None"
	case string:
		return value
	case bool:
		if value {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(value)
	}
}

// describePublisher names a trusted publisher the way a release engineer
// looks it up on pypi.org.
func describePublisher(publisher map[string]any) string {
	return fmt.Sprintf("%s %s %s (environment %s)", pyText(publisher["kind"]), pyText(publisher["repository"]),
		pyText(publisher["workflow"]), pyText(publisher["environment"]))
}

// describeStatement names what one statement says: its predicate and each
// subject, by file name and SHA-256.
func describeStatement(statement map[string]any) string {
	subjects, _ := statement["subject"].([]any)
	var named []string
	for _, raw := range subjects {
		subject, isObject := raw.(map[string]any)
		if text, isText := raw.(string); isText {
			// Python's repr() quotes a string.
			named = append(named, "'"+text+"'")
			continue
		}
		if !isObject {
			named = append(named, pyText(raw))
			continue
		}
		digest, _ := subject["digest"].(map[string]any)
		named = append(named, fmt.Sprintf("%s at sha256 %s", pyText(subject["name"]), pyText(digest["sha256"])))
	}
	if len(named) == 0 {
		named = []string{"no subject"}
	}
	return pyText(statement["predicateType"]) + " of " + strings.Join(named, ", ")
}

// checkPypiProvenance holds each wheel read back, by its file name and the
// SHA-256 of the bytes served, to a publish attestation from release.yml's
// trusted publisher. A provenance not served yet is waited for, as a wheel
// is; what one that is served says is judged once.
func (v *verifyPublished) checkPypiProvenance(served map[string]string) {
	for _, filename := range mapKeys(served) {
		digest := served[filename]
		provenanceURL := fmt.Sprintf("%s%s/%s/%s/provenance", pypiIntegrity, pypiDist, v.version, filename)
		body, err := v.fetch(provenanceURL, v.budget)
		if err != nil {
			v.failf("pypi %s: PyPI serves no provenance for it at %s: %v", filename, provenanceURL, err)
			continue
		}
		bundles, err := provenanceBundles(body)
		if err != nil {
			v.failf("pypi %s: the provenance at %s cannot be read: %v", filename, provenanceURL, err)
			continue
		}
		var ours []map[string]any
		var others []string
		for _, bundle := range bundles {
			others = append(others, describePublisher(bundle.publisher))
			if slices.IndexFunc(pypiPublisher, func(field [2]string) bool { return bundle.publisher[field[0]] != field[1] }) < 0 {
				ours = append(ours, bundle.statements...)
			}
		}
		if len(ours) == 0 && !slices.ContainsFunc(bundles, func(b provenanceBundle) bool { return len(b.statements) == 0 && ourPublisher(b) }) {
			named := strings.Join(others, "; ")
			if named == "" {
				named = "no publisher"
			}
			v.failf("pypi %s: no attestation in its provenance comes from %s; it names %s", filename, describeOurPublisher(), named)
			continue
		}
		want := []any{map[string]any{"name": filename, "digest": map[string]any{"sha256": digest}}}
		if slices.ContainsFunc(ours, func(s map[string]any) bool {
			return s["_type"] == inTotoStatement && s["predicateType"] == pypiPublishType && reflect.DeepEqual(s["subject"], want)
		}) {
			v.sayf("  ok  pypi %-60s attested by release.yml", filename)
			continue
		}
		var stated []string
		for _, statement := range ours {
			stated = append(stated, describeStatement(statement))
		}
		said := strings.Join(stated, "; ")
		if said == "" {
			said = "it states nothing"
		}
		v.failf("pypi %s: no attestation release.yml signed states %s of %s alone, but PyPI serves %s at sha256 %s: %s",
			filename, pypiPublishType, filename, filename, digest, said)
	}
}

// ourPublisher reports whether a bundle comes from release.yml's publisher.
func ourPublisher(bundle provenanceBundle) bool {
	return slices.IndexFunc(pypiPublisher, func(field [2]string) bool { return bundle.publisher[field[0]] != field[1] }) < 0
}

// describeOurPublisher names release.yml's trusted publisher.
func describeOurPublisher() string {
	publisher := map[string]any{}
	for _, field := range pypiPublisher {
		publisher[field[0]] = field[1]
	}
	return describePublisher(publisher)
}

// nugetPackageURL is the flat-container download of one package, the URL the
// SDK installs from; ids and versions are lower-cased there.
func nugetPackageURL(id, version string) string {
	id, version = strings.ToLower(id), strings.ToLower(version)
	return fmt.Sprintf("%s%s/%s/%s.%s.nupkg", nugetFlatURL, id, version, id, version)
}

// checkNugetListing checks the pointer's flat-container index lists the
// version, waiting from the deadline for an index that does not list it yet:
// while nuget.org validates a push the index answers with the versions it
// already serves. An index that cannot be read is reported at once, beyond
// what fetch waited for.
func (v *verifyPublished) checkNugetListing() {
	for attempt := 1; ; attempt++ {
		body, err := v.fetch(fmt.Sprintf("%s%s/index.json", nugetFlatURL, nugetID), v.deadline)
		var index struct {
			Versions []string `json:"versions"`
		}
		if err == nil {
			err = json.Unmarshal(body, &index)
		}
		if err != nil {
			v.failf("nuget %s: could not list the published versions: %v", nugetID, err)
			return
		}
		if slices.ContainsFunc(index.Versions, func(listed string) bool { return strings.EqualFold(listed, v.version) }) {
			v.sayf("  ok  nuget %-32s lists %s", nugetID, v.version)
			return
		}
		if !v.deadline.wait() {
			v.failf("nuget %s: version %s is not listed on nuget.org (still unlisted after %d attempt(s))", nugetID, v.version, attempt)
			return
		}
		v.sayf("  .. nuget %s: version %s is not listed yet; retrying (attempt %d)", nugetID, v.version, attempt+1)
	}
}

// checkNuget checks the pointer is listed and each runtime package carries
// the signed binary, and with attested digests holds all seven packages to
// them as well. It returns the attested digests it judged by, nil when none
// were given or they named nothing.
func (v *verifyPublished) checkNuget(attested map[string]string, given bool) map[string]string {
	// The pointer carries no binary, so its check is that the version is
	// listed: a runtime package nobody points at is not installable.
	v.checkNugetListing()
	if given && len(attested) == 0 {
		v.failf("nuget: the attested digests name no package, so nothing the nuget job attested can be compared; its nupkg_sha256 output did not reach this run")
		attested = nil
	}
	if attested != nil {
		published := map[string]bool{nugetPackageName(nugetID, v.version): true}
		for _, p := range releasePlatforms {
			published[nugetPackageName(nugetID+"."+p.rid, v.version)] = true
		}
		for _, name := range mapKeys(attested) {
			if !published[name] {
				v.failf("nuget: the nuget job attested %s, which is not one of the packages of %s", name, v.version)
			}
		}
		// The pointer is what dnx and dotnet tool install read first, and
		// its DotnetToolSettings.xml decides which package runs.
		pointerURL := nugetPackageURL(nugetID, v.version)
		if blob, err := v.fetch(pointerURL, v.deadline); err != nil {
			v.failf("nuget %s: could not read the published package: %v", nugetID, err)
		} else {
			v.checkAttestedNupkg(nugetID, blob, pointerURL, nugetPackageName(nugetID, v.version), attested)
		}
	}
	for _, p := range releasePlatforms {
		want, named := v.digests[p.asset]
		if !named {
			v.failf("nuget %s: checksums.txt does not name the release asset %s", p.rid, p.asset)
			continue
		}
		packageURL := nugetPackageURL(nugetID+"."+p.rid, v.version)
		blob, err := v.fetch(packageURL, v.deadline)
		var executable []byte
		if err == nil {
			executable, err = nugetBinary(blob, p.rid, packageURL)
		}
		if err != nil {
			v.failf("nuget %s: could not read the published package: %v", p.rid, err)
			continue
		}
		if got := releaseassets.Digest(executable); got != want {
			v.failf("nuget %s: %s carries sha256 %s, but the signed checksums.txt says %s is %s", p.rid, packageURL, got, p.asset, want)
		} else {
			v.sayf("  ok  nuget %-32s matches %s", p.rid, p.asset)
		}
		if attested != nil {
			v.checkAttestedNupkg(p.rid, blob, packageURL, nugetPackageName(nugetID+"."+p.rid, v.version), attested)
		}
	}
	return attested
}

// nugetBinary returns the binary a downloaded runtime package carries.
func nugetBinary(blob []byte, rid, packageURL string) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return nil, err
	}
	prefix := "tools/any/" + rid + "/"
	for _, f := range archive.File {
		if base := path.Base(f.Name); strings.HasPrefix(f.Name, prefix) && (base == binaryName || base == binaryName+".exe") {
			return readZipEntry(f)
		}
	}
	return nil, fmt.Errorf("%s ships no gitlab-mcp-server binary under %s", packageURL, prefix)
}

// checkAttestedNupkg holds one served package, its repository signature
// removed, to the digest the nuget job attested for it.
func (v *verifyPublished) checkAttestedNupkg(label string, blob []byte, packageURL, filename string, attested map[string]string) {
	want, recorded := attested[filename]
	if !recorded {
		v.failf("nuget %s: the nuget job recorded no attested digest for %s", label, filename)
		return
	}
	unsigned, err := nugetUnsigned(blob)
	if err != nil {
		v.failf("nuget %s: %s cannot be unsigned the way NuGet defines it: %v", label, packageURL, err)
		return
	}
	if got := releaseassets.Digest(unsigned); got != want {
		v.failf("nuget %s: %s without its repository signature is sha256 %s, but the nuget job attested %s as %s", label, packageURL, got, filename, want)
		return
	}
	v.sayf("  ok  nuget %-32s unsigned, equals the attested %s", label, filename)
}

// nugetUnsigned returns a served package as it was before nuget.org signed
// it. NuGet signs by writing a stored .signature.p7s local entry after the
// last local entry and its central directory record after the last record,
// and defines the unsigned package as what removing both gives back: every
// byte before the signature's local header, the directory without its
// record, and the end record counted down by one with the directory's new
// offset and size. A package carrying no signature entry comes back as it
// is.
func nugetUnsigned(blob []byte) ([]byte, error) {
	le := binary.LittleEndian
	eocd := bytes.LastIndex(blob, []byte("PK\x05\x06"))
	if eocd < 0 || eocd+22 > len(blob) {
		return nil, errors.New("no end-of-central-directory record")
	}
	nDisk, nTotal := le.Uint16(blob[eocd+8:]), le.Uint16(blob[eocd+10:])
	cdSize, cdOffset := int(le.Uint32(blob[eocd+12:])), int(le.Uint32(blob[eocd+16:]))
	if eocd+22+int(le.Uint16(blob[eocd+20:])) != len(blob) {
		return nil, errors.New("bytes follow the end-of-central-directory record")
	}
	type record struct {
		name               string
		at, size, localsAt int
	}
	var records []record
	offset := cdOffset
	for range nTotal {
		if offset < 0 || offset+46 > len(blob) || !bytes.Equal(blob[offset:offset+4], []byte("PK\x01\x02")) {
			return nil, fmt.Errorf("no central directory record at offset %d", offset)
		}
		nameLen, extraLen, commentLen := int(le.Uint16(blob[offset+28:])), int(le.Uint16(blob[offset+30:])), int(le.Uint16(blob[offset+32:]))
		size := 46 + nameLen + extraLen + commentLen
		records = append(records, record{
			name: string(blob[offset+46 : min(len(blob), offset+46+nameLen)]), at: offset, size: size,
			localsAt: int(le.Uint32(blob[offset+42:])),
		})
		offset += size
	}
	if offset != cdOffset+cdSize {
		return nil, errors.New("the central directory is not the size its end record says")
	}
	var signatures []record
	for _, r := range records {
		if r.name == ".signature.p7s" {
			signatures = append(signatures, r)
		}
	}
	switch {
	case len(signatures) == 0:
		return blob, nil
	case len(signatures) > 1:
		return nil, errors.New("more than one .signature.p7s entry")
	}
	signature := signatures[0]
	if slices.ContainsFunc(records, func(r record) bool { return r.localsAt > signature.localsAt }) {
		return nil, errors.New(".signature.p7s is not the last local entry")
	}
	out := bytes.Clone(blob[:signature.localsAt])
	out = append(out, blob[cdOffset:signature.at]...)
	out = append(out, blob[signature.at+signature.size:cdOffset+cdSize]...)
	end := bytes.Clone(blob[eocd : eocd+22])
	le.PutUint16(end[8:], nDisk-1)
	le.PutUint16(end[10:], nTotal-1)
	le.PutUint32(end[12:], uint32(cdSize-signature.size)) //#nosec G115 -- a directory size read from 32 bits, made smaller
	le.PutUint32(end[16:], uint32(signature.localsAt))    //#nosec G115 -- an offset read from 32 bits
	out = append(out, end...)
	return append(out, blob[eocd+22:]...), nil
}
