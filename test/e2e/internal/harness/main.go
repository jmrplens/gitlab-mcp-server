//go:build e2e

// main.go is what a package's TestMain calls, and it does as little as it can.
//
// Everything that needs GitLab happens later, behind a sync.Once the first
// test triggers. Go runs TestMain for every package named on the command line,
// so a Main that probed would have every package contact GitLab even when the
// -run filter leaves it with nothing to do, and `go test -list` would need an
// instance to list names. Deferring the work is what keeps both free.
//
// Configuration is read into a map and never put back into this process's
// environment. A child's environment is built from nothing, so a variable that
// leaked into the harness would reach the server only by accident, and one
// that did not leak would be missing for no stated reason. Reading into a map
// removes the question.

package harness

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joho/godotenv"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// The configuration keys this harness reads. GITLAB_URL and GITLAB_TOKEN are
// GitLab's own spelling, which is what a developer already has exported; the
// rest are the suite's own and all carry the E2E_ prefix.
const (
	envGitLabURL       = "GITLAB_URL"
	envGitLabToken     = "GITLAB_TOKEN"
	envSkipTLSVerify   = "GITLAB_MCP_SKIP_TLS_VERIFY"
	envMode            = "E2E_MODE"
	envRunID           = "E2E_RUN_ID"
	envEnvFile         = "E2E_ENV_FILE"
	envRuntimeMismatch = "E2E_RUNTIME_MISMATCH"
	envExternalNetwork = "E2E_EXTERNAL_NETWORK"
	envFixtureURL      = "E2E_FIXTURE_URL"

	envBitbucketServerURL = "BITBUCKET_SERVER_URL"
	envGitHubToken        = "GH_TOKEN"
)

// dockerEnvFile and repoEnvFile are the two files a run may read, both
// resolved from the repository root so a package's own directory never decides
// what configuration it gets.
var (
	dockerEnvFile = filepath.Join("test", "e2e", ".env.docker")
	repoEnvFile   = ".env"
)

// bootstrapTimeout bounds the whole probe. It is generous because a Docker
// GitLab answers its first requests slowly, and short enough that a run
// pointed at nothing fails rather than hanging.
const bootstrapTimeout = 2 * time.Minute

// refusalKind says whether a refusal can be turned into skips.
type refusalKind int

const (
	// refusalNone is no refusal.
	refusalNone refusalKind = iota
	// refusalMismatch is an instance that works and is the wrong one.
	// E2E_RUNTIME_MISMATCH=skip turns it into skips.
	refusalMismatch
	// refusalFatal is a run that has no GitLab at all, or one that would not
	// answer. It always fails: a release gate that skipped this would pass
	// with nothing having run.
	refusalFatal
)

// runState is what Main resolved and what the bootstrap made of it. There is
// one per test binary, which is one package.
type runState struct {
	once sync.Once

	settings    settings
	requirement Requirement
	pkg         string
	runID       string
	loadErr     error

	inst    *instance
	refusal string
	kind    refusalKind

	reported     atomic.Bool
	firstRefused atomic.Pointer[string]
}

// state is this test binary's run.
var state runState

// Main runs one end-to-end package's tests against the runtime it requires.
//
// It resolves the configuration, computes the run identifier and returns the
// exit code the package's TestMain should exit with. Everything else waits for
// the first test that asks the harness for something.
func Main(m *testing.M, req Requirement) int {
	// The testing package has registered its flags by the time TestMain runs,
	// so this is what makes -run readable, which the run record names.
	flag.Parse()

	pkg := packageName()
	loaded, err := loadSettings()
	state.settings = loaded
	state.requirement = req
	state.pkg = pkg
	state.loadErr = err
	state.runID = configuredRunID(time.Now(), loaded.get(envRunID), pkg)

	log.Printf("e2e: package %s needs %s; run ID %s; filter %q", pkg, req, state.runID, testRunFilter())

	code := m.Run()

	removeBuiltBinary()
	return state.finish(code)
}

// finish reports what the run left behind and returns the exit code.
func (s *runState) finish(code int) int {
	if s.inst != nil && s.inst.snapshot != nil {
		if err := verifySnapshot(s.inst.client, s.inst.snapshot); err != nil {
			log.Printf("e2e: SNAPSHOT INTEGRITY FAILURE: %v", err)
			if code == 0 {
				code = 1
			}
		} else {
			log.Println("e2e: snapshot integrity verified: every pre-existing resource is unchanged")
		}
	}

	if s.kind == refusalFatal || (s.kind == refusalMismatch && !s.mismatchSkips()) {
		return 2
	}
	return code
}

// mismatchSkips reports whether the run asked for a runtime mismatch to skip
// its tests rather than fail them.
func (s *runState) mismatchSkips() bool {
	return strings.EqualFold(s.settings.get(envRuntimeMismatch), "skip")
}

// bootstrap runs the probe, the guard and the preparation once, and reports a
// refusal to every test that arrives after it.
//
// The first test to reach a refusal fails with the whole block; the rest skip
// with a pointer to it. Failing every test with the same wall of text would
// bury the one line that matters, and skipping every test would leave a run
// that refused looking like a run that passed.
func bootstrap(t *testing.T) *instance {
	t.Helper()

	state.once.Do(state.prepare)
	if state.kind != refusalNone {
		state.report(t)
	}
	return state.inst
}

// prepare probes the instance, guards it against the package's requirement and
// gets it ready for the first test.
func (s *runState) prepare() {
	if s.loadErr != nil {
		s.refuse(refusalFatal, s.loadErr.Error())
		return
	}

	url := s.settings.get(envGitLabURL)
	token := s.settings.get(envGitLabToken)
	if url == "" || token == "" {
		s.refuse(refusalFatal, missingCredentialsMessage(s.pkg))
		return
	}
	skipTLSVerify := strings.EqualFold(s.settings.get(envSkipTLSVerify), "true")

	client, err := gitlabclient.NewClientWithToken(url, token, skipTLSVerify)
	if err != nil {
		s.refuse(refusalFatal, unreachableMessage(s.pkg, url, err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()

	facts, err := probe(ctx, client, url, skipTLSVerify, token)
	if err != nil {
		s.refuse(refusalFatal, unreachableMessage(s.pkg, url, err))
		return
	}
	log.Printf("e2e: %s is GitLab %s (%s), tier %s, authenticated as %s (admin=%t)",
		url, facts.Version, facts.editionName(), facts.tierDescription(), facts.Username, facts.Admin)

	if message := guardMessage(facts, s.requirement, s.pkg); message != "" {
		s.refuse(refusalMismatch, message)
		return
	}

	s.inst = &instance{
		settings:    s.settings,
		requirement: s.requirement,
		pkg:         s.pkg,
		runID:       s.runID,
		facts:       facts,
		client:      client,
	}
	prepareInstance(s.inst)
}

// refuse records why no test of this package can run.
func (s *runState) refuse(kind refusalKind, message string) {
	s.kind = kind
	s.refusal = message
	log.Printf("e2e: %s", message)
}

// report tells one test about the refusal.
func (s *runState) report(t *testing.T) {
	t.Helper()

	if s.kind == refusalMismatch && s.mismatchSkips() {
		t.Skipf("%s\n\n(%s=skip is set, so this is a skip)", s.refusal, envRuntimeMismatch)
	}
	if s.reported.CompareAndSwap(false, true) {
		name := t.Name()
		s.firstRefused.Store(&name)
		t.Fatalf("%s", s.refusal)
	}
	first := "an earlier test"
	if name := s.firstRefused.Load(); name != nil {
		first = *name
	}
	t.Skipf("the harness refused this run; %s carries the reason", first)
}

// settings is the configuration one run resolved.
//
// It is a map rather than the process environment on purpose: a file this
// suite reads must not be able to configure the harness's own process, because
// the child's environment is built from nothing and a leaked variable would
// then reach the server through a route nobody declared.
type settings struct {
	values map[string]string
}

// get returns one setting, or the empty string.
func (s settings) get(key string) string { return s.values[key] }

// loadSettings resolves the run's configuration, highest precedence first: the
// process environment, the file E2E_ENV_FILE names, test/e2e/.env.docker in
// Docker mode, and the repository .env.
//
// The repository .env stays last and is read in Docker mode too. It carries
// the credentials .env.docker does not provision, GH_TOKEN and the Bitbucket
// pair, and dropping it would silently skip the importer scenarios in CI
// rather than run them.
func loadSettings() (settings, error) {
	resolved := settings{values: map[string]string{}}
	for _, entry := range os.Environ() {
		if key, value, found := strings.Cut(entry, "="); found {
			resolved.values[key] = value
		}
	}

	root, err := repoRoot()
	if err != nil {
		return resolved, err
	}

	// Read from the process environment rather than from what is resolved so
	// far, so a file this run loads cannot nominate another one.
	if named := os.Getenv(envEnvFile); named != "" {
		if !filepath.IsAbs(named) {
			return resolved, fmt.Errorf("%s must be an absolute path, and is %q: a relative one follows the "+
				"test binary into its own package directory, where it names a different file for each package",
				envEnvFile, named)
		}
		resolved.overlay(named)
	}
	if strings.EqualFold(os.Getenv(envMode), "docker") {
		resolved.overlay(filepath.Join(root, dockerEnvFile))
	}
	resolved.overlay(filepath.Join(root, repoEnvFile))

	return resolved, nil
}

// overlay adds the keys of one dotenv file that nothing has set yet.
//
// A file that is not there is not an error: the repository .env is optional,
// and .env.docker only exists once the stack has been provisioned.
func (s settings) overlay(path string) {
	values, err := godotenv.Read(path)
	if err != nil {
		return
	}
	for key, value := range values {
		if _, already := s.values[key]; !already {
			s.values[key] = value
		}
	}
}

// packageName names the package under test, which every resource this run
// creates carries.
//
// go test runs a package's binary in that package's own directory, so the
// working directory is the name; the binary's own name is the fallback for a
// binary run by hand from somewhere else.
func packageName() string {
	if dir, err := os.Getwd(); err == nil {
		if name := filepath.Base(dir); name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	return strings.TrimSuffix(filepath.Base(os.Args[0]), ".test")
}

// testRunFilter returns the -run pattern this binary was given, for the record
// of what the run actually covered.
func testRunFilter() string {
	if f := flag.Lookup("test.run"); f != nil {
		return f.Value.String()
	}
	return ""
}

// repoRoot walks up from the working directory to the directory holding
// go.mod.
//
// Every path this harness resolves starts here: a path relative to the working
// directory would name a different file for each of the three packages, which
// is how the suite this replaces ended up with three spellings of the same
// dotenv file.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving the working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s, so the repository root cannot be resolved", dir)
		}
		dir = parent
	}
}
