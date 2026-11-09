// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// guide_test.go serves the proxy configurations the documentation prints,
// read from the pages at test time, and holds each to what the remote
// deployment guide's requirement table says every proxy must get right.
//
// The cases used to serve copies. A copy is what runs and the page is what a
// reader pastes, so a page edit could drift with every test green, and two of
// the copies had already drifted (issue 1112). Here the page is the input: a
// block is read by the e2e=<case> key on its fence, through
// internal/testutil/docblocks, and changed only by a closed table of edits a
// case declares. Each edit names a token of the block, what the case serves in
// its place, and how many times the token appears, so a change to the block
// anywhere else is served as the page prints it, and a change to a token a case
// replaces fails here, naming the token, rather than being served unedited.
// What a case adds that the page does not print (the frame a block is
// included from, a test-only header, a certificate a test can issue) lives in
// the case's own files, never inside the block.
package httpe2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/docblocks"
)

// The pages whose blocks the cases serve, relative to the site's content root.
const (
	remoteDeploymentPage = "operations/remote-deployment.mdx"
	loadBalancingPage    = "enterprise/load-balancing.mdx"
)

// publicHost is the name every TLS block of the guide serves, and the one its
// requirement table starts the server with in --public-url.
const publicHost = "mcp.example.com"

// docsDir is the site's content root, where the English pages live and
// their Spanish twins live under es/.
func docsDir() string {
	return filepath.Join(repoRoot(), "site", "src", "content", "docs")
}

// edit is one entry of a case's substitution table: a token of a documented
// block, what the case serves in its place, and how many times the token
// appears in the block.
type edit struct {
	old   string
	new   string
	count int
}

// errEditCount is a token that does not appear in a block as many times as
// the case that replaces it says.
var errEditCount = errors.New("the block does not carry the token as many times as the case replaces it")

// applyEdits applies a case's substitution table to a block.
//
// Every count is checked against the block as the page prints it before
// anything is replaced, and the replacements are made in one pass, so a
// replacement never feeds the next edit's count. An edit whose count is wrong
// fails the whole table: a token the page no longer carries is a block the
// case would otherwise serve unedited, and a token it now carries twice is a
// second line the case would rewrite without anyone deciding to.
func applyEdits(body string, edits []edit) (string, error) {
	pairs := make([]string, 0, 2*len(edits))
	for _, e := range edits {
		if got := strings.Count(body, e.old); got != e.count {
			return "", fmt.Errorf("%q appears %d time(s) and the case replaces it %d time(s): %w", e.old, got, e.count, errEditCount)
		}
		pairs = append(pairs, e.old, e.new)
	}
	return strings.NewReplacer(pairs...).Replace(body), nil
}

// guideBlock reads the block keyed key from page and applies the case's
// edits, failing the test when the page lost the key or a count is wrong.
func guideBlock(t *testing.T, page, key string, edits ...edit) string {
	t.Helper()
	doc, err := docblocks.Open(docsDir(), page)
	if err != nil {
		t.Fatalf("reading %s: %v", page, err)
	}
	block, err := doc.Block(key)
	if err != nil {
		t.Fatalf("the case serves the block keyed e2e=%s: %v", key, err)
	}
	served, err := applyEdits(block.Body, edits)
	if err != nil {
		t.Fatalf("%s, the block keyed e2e=%s at line %d: %v; the page changed a token the case replaces, so the case has to change with it", page, key, block.Line, err)
	}
	return served
}

// dockerPolicyVar names what a Docker case that cannot run does, which the
// job running the module states (dockerMustRun).
const dockerPolicyVar = "E2E_HTTP_DOCKER"

// The two values dockerPolicyVar takes.
const (
	// dockerRequired fails the case.
	dockerRequired = "required"
	// dockerOptional skips it, as an unset policy does.
	dockerOptional = "optional"
)

// dockerNoticesVar names a file every skipped Docker case appends one line
// to, for the job to print once go test is done. go test prints nothing the
// tests of a passing package wrote, a skip's reason included, so a skip that
// only told the test log would not be seen.
const dockerNoticesVar = "E2E_HTTP_DOCKER_NOTICES"

// dockerMustRun reports whether a Docker case that cannot run fails rather
// than skips, under the policy a job states in dockerPolicyVar.
//
// The job states it rather than the module guessing it from the machine,
// because two jobs on the same Linux runner image answer differently. ci.yml's
// e2e-http job sets required: Docker is on its runner by construction, so a
// case that could not start says something went wrong (an image that could
// not be pulled, a container that never came up), and a skip there is a line
// in a log nobody reads, with the job reporting the proxy layer green whether
// or not it ran. race.yml's transport job sets optional: it gates a release,
// which a registry outage would otherwise block, so there the case skips and
// the job prints the skip under a warning (dockerNoticesVar). Unset, on a
// developer machine and on the macOS and Windows legs of CI, which have no
// Docker for Linux images, the case skips with its reason. Any other value is
// refused rather than read as either, since a misspelled required would skip.
func dockerMustRun(policy string) (bool, error) {
	switch policy {
	case dockerRequired:
		return true, nil
	case "", dockerOptional:
		return false, nil
	}
	return false, fmt.Errorf("%s=%q: it takes %s or %s", dockerPolicyVar, policy, dockerRequired, dockerOptional)
}

// environmentUnavailablef ends a Docker case that cannot run, failing it where
// dockerMustRun says Docker is expected and skipping it everywhere else, with
// the skip recorded where the job asked for it.
func environmentUnavailablef(t *testing.T, format string, args ...any) {
	t.Helper()
	reason := fmt.Sprintf(format, args...)
	mustRun, err := dockerMustRun(os.Getenv(dockerPolicyVar))
	if err != nil {
		t.Fatalf("%s\n(%v)", reason, err)
	}
	if mustRun {
		t.Fatalf("%s\n(%s=%s: Docker is expected here, so this fails rather than skips)", reason, dockerPolicyVar, dockerRequired)
	}
	if recordErr := recordSkip(os.Getenv(dockerNoticesVar), t.Name(), reason); recordErr != nil {
		t.Fatalf("%s\n(the skip could not be recorded in %s: %v)", reason, dockerNoticesVar, recordErr)
	}
	t.Skip(reason)
}

// skipNoticesMu keeps the lines parallel cases append to the notices file
// whole.
var skipNoticesMu sync.Mutex

// recordSkip appends a skipped case's line to the notices file at path, and
// does nothing when path is empty.
func recordSkip(path, test, reason string) error {
	if path == "" {
		return nil
	}
	skipNoticesMu.Lock()
	defer skipNoticesMu.Unlock()
	//#nosec G703 -- the path is E2E_HTTP_DOCKER_NOTICES, a file the job running these tests names for them to append to
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(skipNotice(test, reason) + "\n")
	return errors.Join(err, f.Close())
}

// skipNotice is the notices file's line for a skipped case: the test's name
// and its reason with every run of white space folded to one space. A
// container's log in the reason then stays on the one line, which starts with
// the test's name and so never with the "::" a workflow reads as a command of
// its own.
func skipNotice(test, reason string) string {
	return test + ": " + strings.Join(strings.Fields(reason), " ")
}

// requireDocker ends the calling test unless Docker is present and usable:
// a skip, or a failure where dockerMustRun says Docker is expected.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := dockerUnusable(t.Context()); err != nil {
		environmentUnavailablef(t, "%v", err)
	}
}

// dockerUnusable says why Docker cannot run a case here, or returns nil when
// it can.
func dockerUnusable(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker not available; a proxy or a balancer is run rather than modeled, so this case cannot run without it")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		return fmt.Errorf("docker is installed but not usable (%w), so this case cannot run", err)
	}
	return nil
}

// containerDir returns a directory a container's own user can read files
// from, which t.TempDir is not: its per-test parent is created 0700 and never
// handed out, and a bind mount below it stops another user at that component.
func containerDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mcp-proxy") //nolint:usetesting // t.TempDir's own parent is 0700 and not ours to open; see above
	if err != nil {
		t.Fatalf("creating a directory for the container: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err = os.Chmod(dir, 0o755); err != nil { //#nosec G302 -- a throwaway directory a container must read
		t.Fatalf("opening the container directory: %v", err)
	}
	return dir
}

// writeContainerFile writes one file a container reads, creating the
// directories on its path, and returns the file's path.
func writeContainerFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	for parent := filepath.Dir(path); parent != dir; parent = filepath.Dir(parent) {
		if err := os.MkdirAll(parent, 0o755); err != nil { //#nosec G301 -- read by a container as another user
			t.Fatalf("creating %s: %v", parent, err)
		}
		if err := os.Chmod(parent, 0o755); err != nil { //#nosec G302 -- read by a container as another user
			t.Fatalf("opening %s: %v", parent, err)
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil { //#nosec G306 -- read by a container as another user
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

// startContainer runs image on the host network with the given bind mounts
// and arguments, and removes it when the test ends.
func startContainer(t *testing.T, name, image string, mounts []string, args ...string) {
	t.Helper()
	requireDocker(t)
	if out, err := runContainer(t, name, image, mounts, args...); err != nil {
		environmentUnavailablef(t, "could not start %s (%v):\n%s", image, err, out)
	}
}

// runContainer is startContainer without the decision: it returns what docker
// run printed and why it failed, for a case that does not skip, and removes
// the container when the test ends once it has started.
//
// Only once it has started: a run refused because the name is taken leaves
// the name to the container that holds it, which another case started and
// still needs.
func runContainer(t *testing.T, name, image string, mounts []string, args ...string) ([]byte, error) {
	t.Helper()
	runArgs := []string{"run", "-d", "--name", name, "--network", "host"}
	for _, mount := range mounts {
		runArgs = append(runArgs, "-v", mount)
	}
	runArgs = append(runArgs, image)
	runArgs = append(runArgs, args...)
	runCtx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(runCtx, "docker", runArgs...).CombinedOutput()
	if err != nil {
		return out, err
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		// Logged rather than failed, so a daemon slow to remove a container
		// does not fail a case that passed; named, because the container a
		// failed removal leaves behind can be the quick tunnel's, and its
		// public address stays open until it is removed.
		if rmOut, rmErr := exec.CommandContext(rmCtx, "docker", "rm", "-f", name).CombinedOutput(); rmErr != nil {
			t.Logf("docker rm -f %s: %v: %s; remove it by hand", name, rmErr, strings.TrimSpace(string(rmOut)))
		}
	})
	return out, nil
}

// containerLogs returns what a container has written, for a failure message.
func containerLogs(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "docker", "logs", name).CombinedOutput()
	return string(out)
}

// publicCert is what a TLS block of the guide is served with: a root the
// test's clients trust, and a certificate for publicHost it signed, written
// where a container can read them.
type publicCert struct {
	// dir holds ca.crt and ca.key (the root, for a proxy that issues its own
	// certificates from it) and fullchain.pem and privkey.pem (the leaf, for
	// a proxy that is handed one, under the names Let's Encrypt gives them).
	dir  string
	pool *x509.CertPool
	// chain and key are the leaf's PEM files' contents.
	chain, key string
}

// combinedPEM is the leaf, its chain and its key in one file, which is how
// HAProxy reads a certificate.
func (c *publicCert) combinedPEM() string {
	return c.chain + c.key
}

// newPublicCert issues a fresh root and a certificate for publicHost.
//
// The root lives thirty days rather than the leaf's one: Caddy refuses to run
// an internal CA whose root expires before the intermediate it would sign,
// and its intermediates live a week.
func newPublicCert(t *testing.T) *publicCert {
	t.Helper()
	caKey := newKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "gitlab-mcp-server e2e root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("creating the root: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parsing the root: %v", err)
	}

	leafKey := newKey(t)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: publicHost},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{publicHost},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("creating the certificate for %s: %v", publicHost, err)
	}

	dir := containerDir(t)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	chain := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})) + string(caPEM)
	key := string(keyPEM(t, leafKey))
	writeContainerFile(t, dir, "ca.crt", string(caPEM))
	writeContainerFile(t, dir, "ca.key", string(keyPEM(t, caKey)))
	writeContainerFile(t, dir, "fullchain.pem", chain)
	writeContainerFile(t, dir, "privkey.pem", key)

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &publicCert{dir: dir, pool: pool, chain: chain, key: key}
}

// newKey generates a P-256 key.
func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	return key
}

// keyPEM encodes a key the way every proxy here reads one.
func keyPEM(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling a key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// letsEncryptMount is the bind mount that puts a public certificate where the
// guide's nginx and Apache blocks read it, so their certificate lines are
// served as the page prints them.
func (c *publicCert) letsEncryptMount() string {
	return c.dir + ":/etc/letsencrypt/live/" + publicHost + ":ro"
}

// clientAddress is a loopback address a case's client connects from.
//
// Every proxy here runs on the host network and connects to the server from
// 127.0.0.1, so a client that also connected from 127.0.0.1 would leave the
// server unable to tell the real client from the proxy, and an assertion that
// the real client was charged could not fail. Linux routes the whole of
// 127.0.0.0/8 to the loopback interface, so a client can be any of them.
type clientAddress string

// The addresses the requirement assertions connect from, one per role so no
// assertion spends another's failure budget.
const (
	clientForHeaders clientAddress = "127.0.0.3"
	clientCharged    clientAddress = "127.0.0.4"
	clientBystander  clientAddress = "127.0.0.5"
)

// throughProxy returns a handle that reaches a server the way a client
// reaching the guide's deployment does: the request goes to the proxy
// listening on port, from address, naming publicHost in Host and, over TLS,
// in the server name, with the certificate checked against the case's root.
func throughProxy(t *testing.T, scheme string, port int, roots *x509.CertPool, from clientAddress, logs func() string) *server {
	t.Helper()
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		LocalAddr: &net.TCPAddr{IP: net.ParseIP(string(from))},
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, fmt.Sprintf("127.0.0.1:%d", port))
		},
		// A custom dialer turns HTTP/2 off unless asked for, and a browser
		// reaching nginx's "http2 on" speaks it.
		ForceAttemptHTTP2: true,
		DisableKeepAlives: true,
	}
	if roots != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}
	t.Cleanup(client.CloseIdleConnections)
	return &server{baseURL: scheme + "://" + publicHost, logs: logs, client: client}
}

// serverCSP is the Content-Security-Policy the server sets on every response
// it writes, a refusal included, and that no documented proxy sets: a response
// carrying it came from the server.
const serverCSP = "default-src 'none'; frame-ancestors 'none'"

// awaitForwarding polls a proxy until a request through it reaches the server
// behind it, whatever the server answers.
//
// A proxy that never answers is a container that did not come up, which is
// the environment, unless it exited. One that answers and never with the
// server's own headers, for as long as the server behind it has already been
// serving, is the block this case served, and that fails: the configuration
// does not reach the server it names.
//
// It returns errPortRace, for the caller to try another port, when the
// container exited because something else bound its port first; every other
// outcome but forwarding ends the test.
func awaitForwarding(t *testing.T, through *server, path, container string) error {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var lastErr error
	lastStatus := 0
	for poll := 0; time.Now().Before(deadline); poll++ {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, through.baseURL+path, http.NoBody)
		if err != nil {
			t.Fatalf("building the readiness request: %v", err)
		}
		resp, err := through.httpClient().Do(req)
		if err != nil {
			lastErr = err
		} else {
			_ = resp.Body.Close()
			if resp.Header.Get("Content-Security-Policy") == serverCSP {
				return nil
			}
			lastStatus = resp.StatusCode
		}
		// Asked once a second rather than on every poll: an exit is
		// rare, and docker inspect is not free.
		if poll%4 == 3 {
			if exitErr := exitedContainer(t, container); exitErr != nil {
				return exitErr
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastStatus != 0 {
		t.Fatalf("%s answered GET %s and never with the server's answer (last status %d), with the server behind it already serving: the block served does not reach the upstream it names.\n%s",
			container, path, lastStatus, containerLogs(container))
	}
	if err := exitedContainer(t, container); err != nil {
		return err
	}
	environmentUnavailablef(t, "%s never answered a request (%v); the container did not come up:\n%s", container, lastErr, containerLogs(container))
	return nil
}

// errPortRace is a container that exited because another listener bound the
// port the case had picked for it, in the moment between freePort releasing
// the port and the container binding it. Every test here picks ports that
// way and runs beside others, so it happens, and another port answers it.
var errPortRace = errors.New("another listener took the port before the container bound it")

// addressInUse is how the proxies here log that race.
var addressInUse = regexp.MustCompile(`(?i)address (already )?in use|cannot bind socket`)

// portRaceAttempts is how many ports a case tries before the race is taken
// for something else.
const portRaceAttempts = 3

// loopbackBind is the address every proxy here listens on, where the
// documented block listens on every interface.
//
// A wildcard listener and one on 127.0.0.1 can both hold a port when both
// ask for address reuse, which Go's listeners and these proxies do, and the
// one on 127.0.0.1 then takes every connection the case's client makes. A
// server that took the case's port in the race above would answer the case
// instead of the proxy, and the case would read a plain HTTP answer where it
// expected TLS. Two listeners on one address cannot share the port, so the
// proxy fails to bind and the race is seen for what it is.
const loopbackBind = "127.0.0.1"

// exitedContainer ends the test when a container has exited, unless it exited
// because its port was taken, which it returns as errPortRace.
//
// An image that could not be pulled fails before the container exists, and
// a container that runs and does not answer is the machine's to explain. One
// that started and stopped for any other reason refused the configuration it
// was given, which on a page that changed is the page's doing, and that is a
// failure wherever the case runs.
func exitedContainer(t *testing.T, container string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}} {{.State.ExitCode}}", container).Output()
	if err != nil {
		return nil
	}
	state := strings.Fields(string(out))
	if len(state) != 2 || state[0] != "false" {
		return nil
	}
	logs := containerLogs(container)
	if addressInUse.MatchString(logs) {
		return fmt.Errorf("%s: %w", container, errPortRace)
	}
	t.Fatalf("%s exited with status %s: it refused the configuration this case served.\n%s", container, state[1], logs)
	return nil
}

// requireStillRunning fails the test when a container that never answered
// has exited, whatever the reason.
func requireStillRunning(t *testing.T, container string) {
	t.Helper()
	if err := exitedContainer(t, container); err != nil {
		t.Fatalf("%v; this container's port is not the case's to change", err)
	}
}

// startServerOnFreePort starts a server on a port the case picks, as a proxy
// in front of it has to name the port before the server starts, and picks
// another when a listener took the first in the meantime.
func startServerOnFreePort(t *testing.T, flags ...string) (int, *server) {
	t.Helper()
	for attempt := 1; ; attempt++ {
		port := freePort(t)
		srv, err := tryStartServerOnPort(t, port, nil, flags...)
		if err == nil {
			return port, srv
		}
		if !errors.Is(err, errPortTaken) || attempt == portRaceAttempts {
			t.Fatalf("starting the server on port %d: %v", port, err)
		}
	}
}

// runOnce runs image to completion with no network and returns what it
// printed and whether it exited zero. The image is pulled first, so a pull
// failure is the environment's and a nonzero exit is the command's.
func runOnce(t *testing.T, image string, mounts []string, args ...string) (string, error) {
	t.Helper()
	requireDocker(t)
	pullCtx, pullCancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer pullCancel()
	if exec.CommandContext(pullCtx, "docker", "image", "inspect", image).Run() != nil {
		if out, err := exec.CommandContext(pullCtx, "docker", "pull", "-q", image).CombinedOutput(); err != nil {
			environmentUnavailablef(t, "could not pull %s (%v):\n%s", image, err, out)
		}
	}
	runArgs := []string{"run", "--rm", "--network", "none"}
	for _, mount := range mounts {
		runArgs = append(runArgs, "-v", mount)
	}
	runArgs = append(runArgs, image)
	runArgs = append(runArgs, args...)
	runCtx, runCancel := context.WithTimeout(t.Context(), time.Minute)
	defer runCancel()
	out, err := exec.CommandContext(runCtx, "docker", runArgs...).CombinedOutput()
	return string(out), err
}

// guideProse returns the one paragraph of an English page that contains
// marker, with its line breaks folded, so a case can hold what the page says
// in a sentence (a flag to pair, a module list, a rule) to what the case does.
func guideProse(t *testing.T, page, marker string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(docsDir(), filepath.FromSlash(page)))
	if err != nil {
		t.Fatalf("reading %s: %v", page, err)
	}
	var found []string
	for paragraph := range strings.SplitSeq(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n\n") {
		if folded := strings.Join(strings.Fields(paragraph), " "); strings.Contains(folded, marker) {
			found = append(found, folded)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s has %d paragraphs saying %q, want exactly one: the case reads what that sentence says, so the page and the case have to change together", page, len(found), marker)
	}
	return found[0]
}

// tokenGitLab is a GitLab that accepts the tokens it was given and refuses
// every other, and counts the calls that carried an accepted one, so a test
// can tell which instance a request reached and with which credential.
type tokenGitLab struct {
	url string
	mu  sync.Mutex
	// accepted counts the user lookups made with each accepted token, in
	// whichever header the server sent it: the header a client used is not
	// the one the server forwards it in.
	accepted map[string]int
}

// startTokenGitLab serves a GitLab that accepts tokens, in either header.
func startTokenGitLab(t *testing.T, tokens ...string) *tokenGitLab {
	t.Helper()
	fake := &tokenGitLab{accepted: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			token = bearer
		}
		if !slices.Contains(tokens, token) {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		fake.mu.Lock()
		fake.accepted[token]++
		fake.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	return fake
}

// asked returns how many user lookups carried token.
func (g *tokenGitLab) asked(token string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.accepted[token]
}

// documentedProxy is one proxy configuration a page prints, as a case serves
// it, and what the guide says to pair it with.
type documentedProxy struct {
	// name names the containers the case starts.
	name string
	// start runs the proxy in front of the server listening on upstream and
	// returns the container's name and the port the proxy listens on.
	start func(t *testing.T, upstream int) (container string, port int)
	// roots verifies the proxy's certificate; nil for a block served over
	// plain HTTP.
	roots *x509.CertPool
	// clientHeader is the header the guide pairs with --trusted-proxy-header
	// for this proxy, with --trusted-proxies=127.0.0.1.
	clientHeader string
	// ready is the path polled until the proxy forwards it; /health when
	// empty, for a route that does not reach the server's root.
	ready string
}

// scheme is how a client reaches the proxy.
func (p documentedProxy) scheme() string {
	if p.roots == nil {
		return "http"
	}
	return "https"
}

// proxied is a server behind one running copy of a documented proxy.
type proxied struct {
	proxy documentedProxy
	// port is the one the proxy listens on.
	port int
	logs func() string
}

// behind starts a server with the given flags and the proxy in front of it,
// and waits until the proxy forwards to it.
func (p documentedProxy) behind(t *testing.T, flags ...string) proxied {
	t.Helper()
	upstream, srv := startServerOnFreePort(t, flags...)
	ready := p.ready
	if ready == "" {
		ready = "/health"
	}
	for attempt := 1; ; attempt++ {
		container, port := p.start(t, upstream)
		running := proxied{proxy: p, port: port, logs: srv.logs}
		err := awaitForwarding(t, running.from(t, clientForHeaders), ready, container)
		if err == nil {
			return running
		}
		if attempt == portRaceAttempts {
			t.Fatalf("%v, on %d ports in a row", err, attempt)
		}
		t.Logf("%v; starting it again on another port", err)
	}
}

// from returns a handle that reaches the server through the proxy from
// address.
func (r proxied) from(t *testing.T, address clientAddress) *server {
	t.Helper()
	return throughProxy(t, r.proxy.scheme(), r.port, r.proxy.roots, address, r.logs)
}

// The credentials the instance a request selects accepts, one per header:
// the server keeps a verified credential in its pool, so a second request
// with the same token would be answered without asking GitLab, and the count
// that shows the header arrived would not move.
const (
	acceptedPrivateToken = "glpat-reaches-the-server-as-private-token"
	acceptedBearerToken  = "glpat-reaches-the-server-as-bearer"
)

// assertProxyRequirements holds a proxy to the requirement table of the
// remote deployment guide, row by row, through three servers behind three
// copies of it: one started as the guide's TLS deployment declares itself
// (OAuth, --public-url=https://mcp.example.com), one with the trusted-proxy
// pairing the guide names for this proxy, and one with neither, which is the
// control that shows the proxy forwarded the client's Host rather than a
// loopback one the server would serve anyway.
func assertProxyRequirements(t *testing.T, p documentedProxy) {
	t.Helper()

	t.Run("an undeclared forwarded host is refused", func(t *testing.T) {
		t.Parallel()
		gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
		undeclared := p.behind(t, "--gitlab-url="+gitlab.url).from(t, clientForHeaders)
		got := undeclared.do(t, request{method: http.MethodGet, path: "/health"})
		if got.status != http.StatusForbidden || !strings.Contains(got.body, "--public-url") {
			t.Errorf("GET /health for %s through a server that declares no host = %d, want the server's 403 naming --public-url: "+
				"a proxy that forwards a loopback Host instead of the client's is served by every server, and the guide's --public-url would then be doing nothing: %s",
				publicHost, got.status, truncate(got.body))
		}
	})

	t.Run("the declared deployment", func(t *testing.T) {
		t.Parallel()
		gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
		declared := p.behind(t,
			"--auth-mode=oauth",
			"--gitlab-url="+gitlab.url,
			"--public-url=https://"+publicHost,
			"--trusted-origins="+trustedOrigin,
		)
		assertDeclaredDeployment(t, declared.from(t, clientForHeaders))
	})

	t.Run("the trusted-proxy pairing", func(t *testing.T) {
		t.Parallel()
		refusing := startTokenGitLab(t, "glpat-accepted-elsewhere")
		accepting := startTokenGitLab(t, acceptedPrivateToken, acceptedBearerToken)
		flags := []string{
			"--gitlab-url=" + refusing.url,
			"--gitlab-url=" + accepting.url,
			"--trusted-proxy-header=" + p.clientHeader,
			"--trusted-proxies=127.0.0.1",
			"--auth-failure-limit=3",
		}
		paired := p.behind(t, flags...)
		assertCredentialHeadersReachTheServer(t, paired.from(t, clientForHeaders), accepting)
		assertTheRealClientIsCharged(t, paired, accepting)
	})
}

// assertDeclaredDeployment holds the rows a server started as the guide
// declares it can answer: the forwarded host served, the stream not held, the
// bearer forwarded, the server alone speaking CORS, and the metadata at the
// host root routed.
func assertDeclaredDeployment(t *testing.T, declared *server) {
	t.Helper()

	t.Run("the declared host is served", func(t *testing.T) {
		if got := declared.do(t, request{method: http.MethodGet, path: "/health"}); got.status != http.StatusOK {
			t.Errorf("GET /health for %s with --public-url=https://%s = %d, want 200: %s", publicHost, publicHost, got.status, truncate(got.body))
		}
	})

	t.Run("Authorization reaches the server", func(t *testing.T) {
		got := declared.do(t, mcpPOST(map[string]string{"Authorization": "Bearer gloas-through-the-proxy"}))
		if got.status != http.StatusOK || !strings.Contains(got.body, "gitlab_find_action") {
			t.Errorf("tools/list with a bearer token through the proxy = %d, want 200 and the tool list: the token was stripped or the call never reached the server: %s",
				got.status, truncate(got.body))
		}
	})

	t.Run("a stream is delivered as it is written", func(t *testing.T) {
		assertStreamIsNotHeld(t, declared, map[string]string{"Authorization": "Bearer gloas-through-the-proxy"})
	})

	t.Run("the server answers the preflight and is the only one to speak CORS", func(t *testing.T) {
		preflight := declared.do(t, request{
			method: http.MethodOptions, path: "/mcp",
			headers: map[string]string{
				"Origin":                         trustedOrigin,
				"Access-Control-Request-Method":  http.MethodPost,
				"Access-Control-Request-Headers": "authorization, content-type, mcp-protocol-version",
			},
		})
		if preflight.status != http.StatusNoContent {
			t.Errorf("the preflight through the proxy = %d, want the server's 204", preflight.status)
		}
		if got := preflight.header.Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != trustedOrigin {
			t.Errorf("the preflight carries Access-Control-Allow-Origin %q, want exactly [%q]", got, trustedOrigin)
		}
		call := declared.do(t, mcpPOST(map[string]string{
			"Authorization":  "Bearer gloas-through-the-proxy",
			"Origin":         trustedOrigin,
			"Sec-Fetch-Site": "cross-site",
		}))
		if got := call.header.Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != trustedOrigin {
			t.Errorf("a cross-origin call carries Access-Control-Allow-Origin %q, want exactly [%q]: two are a CORS failure, not a merge", got, trustedOrigin)
		}
	})

	t.Run("the metadata at the host root is routed", func(t *testing.T) {
		got := declared.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"})
		if got.status != http.StatusOK {
			t.Fatalf("GET /.well-known/oauth-protected-resource through the proxy = %d, want 200: every OAuth client fetches it first: %s", got.status, truncate(got.body))
		}
		var doc struct {
			Resource string `json:"resource"`
		}
		if err := json.Unmarshal([]byte(got.body), &doc); err != nil || doc.Resource != "https://"+publicHost {
			t.Errorf("the metadata through the proxy names resource %q (%v), want %q: %s", doc.Resource, err, "https://"+publicHost, truncate(got.body))
		}
	})
}

// listenToolsChanged is a 2026-07-28 subscriptions/listen for list changes
// alone: the server acknowledges it at once and then holds the stream open,
// which makes it the stream a buffering proxy shows itself on.
const listenToolsChanged = `{"jsonrpc":"2.0","id":11,"method":"subscriptions/listen","params":{"notifications":{"toolsListChanged":true},` +
	`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},` +
	`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}}}`

// streamGrace is how long the acknowledgement of a listen may take to cross a
// proxy. The server writes it at once; a proxy that holds the body holds it
// until the stream ends, which a listen does not.
const streamGrace = 10 * time.Second

// errStreamHeld is a stream whose first event did not cross the proxy while
// the stream was open.
var errStreamHeld = errors.New("the acknowledgement did not arrive while the stream was open")

// firstListenFrame opens a listen through srv and returns the first data
// frame, or errStreamHeld when none arrives within wait.
//
// The wait covers the response headers as well as the frame: a proxy that
// compresses the body sends neither until it has enough to compress, and a
// client waiting for headers is held just the same.
//
// It does not cover the credential's first request. That one verifies the
// token with the instance and creates the credential's pool entry, which
// took more than ten seconds under -race and more than heldWait on a loaded
// host: a stream that crossed the proxy at once was reported held, and the
// control that must see one held could pass on that time alone. A tools/list
// with the same headers pays for it before the wait starts.
func firstListenFrame(t *testing.T, srv *server, headers map[string]string, wait time.Duration) (string, error) {
	t.Helper()
	if warm := srv.do(t, mcpPOST(headers)); warm.status != http.StatusOK {
		return "", fmt.Errorf("tools/list with the listen's headers answered %d before the listen was sent, want 200: %s", warm.status, truncate(warm.body))
	}
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/mcp", strings.NewReader(listenToolsChanged))
	if err != nil {
		t.Fatalf("building the listen: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "subscriptions/listen")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	// The listen's own client: the shared one's 30-second timeout covers the
	// body, and this body is meant to stay open.
	client := *srv.httpClient()
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("no response headers after %v: %w", wait, errStreamHeld)
		}
		return "", fmt.Errorf("sending the listen: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return "", fmt.Errorf("the listen answered %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		_ = resp.Body.Close()
		return "", fmt.Errorf("the listen answered %q, want text/event-stream", ct)
	}
	// The reader ends the frames when the wait does, since the deadline
	// cancels the body it reads.
	stream := readStream(ctx, resp, cancel)
	select {
	case frame, open := <-stream.frames:
		if open {
			return frame, nil
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("headers and no event after %v: %w", wait, errStreamHeld)
		}
		return "", errors.New("the stream ended before any event, which a listen does not do")
	case readErr := <-stream.failure:
		return "", fmt.Errorf("reading the stream: %w", readErr)
	}
}

// assertStreamIsNotHeld holds the "no response buffering" row: the
// acknowledgement of a listen crosses the proxy while the stream it opens is
// still open.
func assertStreamIsNotHeld(t *testing.T, srv *server, headers map[string]string) {
	t.Helper()
	frame, err := firstListenFrame(t, srv, headers, streamGrace)
	if err != nil {
		t.Fatalf("a subscriptions/listen through the proxy: %v; a proxy that buffers turns a live stream into one delivery at the end, and this stream does not end", err)
	}
	if !strings.Contains(frame, "notifications/subscriptions/acknowledged") {
		t.Errorf("the first event through the proxy is %s, want the listen's acknowledgement", truncate(frame))
	}
}

// assertCredentialHeadersReachTheServer holds the three forwarding rows:
// PRIVATE-TOKEN and Authorization carry the credential, and GITLAB-URL picks
// the instance when several are published. The server is given two
// instances, and only the one the header names accepts the token, so a call
// is answered only when all of it arrived.
func assertCredentialHeadersReachTheServer(t *testing.T, paired *server, accepting *tokenGitLab) {
	t.Helper()
	for _, header := range []struct{ name, value, token string }{
		{"PRIVATE-TOKEN", acceptedPrivateToken, acceptedPrivateToken},
		{"Authorization", "Bearer " + acceptedBearerToken, acceptedBearerToken},
	} {
		t.Run(header.name+" and GITLAB-URL reach the server", func(t *testing.T) {
			got := paired.do(t, mcpPOST(map[string]string{header.name: header.value, "GITLAB-URL": accepting.url}))
			if got.status != http.StatusOK {
				t.Errorf("tools/list with %s and GITLAB-URL through the proxy = %d, want 200: %s", header.name, got.status, truncate(got.body))
			}
			if accepting.asked(header.token) == 0 {
				t.Errorf("the instance GITLAB-URL names was never asked about the credential sent in %s: one of the two headers did not reach the server", header.name)
			}
		})
	}
}

// assertTheRealClientIsCharged holds the real-client-address row: with the
// header the guide pairs with this proxy, a client that keeps failing is
// blocked, whatever it writes in that header itself, and a second client
// behind the same proxy is not.
func assertTheRealClientIsCharged(t *testing.T, paired proxied, accepting *tokenGitLab) {
	t.Helper()
	header := paired.proxy.clientHeader
	t.Run("the real client is charged in the failure limiter", func(t *testing.T) {
		charged := paired.from(t, clientCharged)
		var blocked bool
		for i := range 8 {
			got := charged.do(t, mcpPOST(map[string]string{
				"PRIVATE-TOKEN": fmt.Sprintf("glpat-wrong-%d", i),
				"GITLAB-URL":    accepting.url,
				// What a client writes in the header it is charged by:
				// the proxy replaces it or appends the address it heard
				// from, and the server reads that one.
				header: fmt.Sprintf("198.51.100.%d", i+1),
			}))
			if got.status == http.StatusTooManyRequests {
				blocked = true
				break
			}
			if got.status != http.StatusUnauthorized {
				t.Fatalf("failure %d through the proxy = %d, want 401 or 429: %s", i, got.status, truncate(got.body))
			}
		}
		if !blocked {
			t.Fatalf("eight failures through the proxy were never blocked with a limit of three: each was charged to the address the client wrote in %s rather than to the one the proxy heard from", header)
		}

		bystander := paired.from(t, clientBystander)
		if got := bystander.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-wrong-bystander", "GITLAB-URL": accepting.url})); got.status != http.StatusUnauthorized {
			t.Errorf("another client behind the same proxy = %d, want its own 401: the block was charged to the proxy, which every client shares", got.status)
		}
	})
}

// TestDockerMustRun_PolicyTheJobStates_DecidesFailOrSkip holds the Docker
// cases' policy to the value a job states, and to nothing it does not: a
// value that is neither policy is refused rather than read as a skip.
func TestDockerMustRun_PolicyTheJobStates_DecidesFailOrSkip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		policy  string
		want    bool
		refused bool
	}{
		{name: "unset, as on a developer machine", policy: "", want: false},
		{name: "required, as ci.yml's e2e-http job states", policy: dockerRequired, want: true},
		{name: "optional, as race.yml's transport job states", policy: dockerOptional, want: false},
		{name: "a value that is neither", policy: "true", refused: true},
		{name: "required in another case", policy: "Required", refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dockerMustRun(tt.policy)
			if tt.refused {
				if err == nil || !strings.Contains(err.Error(), dockerPolicyVar) {
					t.Errorf("dockerMustRun(%q) = %v, %v; want an error naming %s", tt.policy, got, err, dockerPolicyVar)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("dockerMustRun(%q) = %v, %v; want %v", tt.policy, got, err, tt.want)
			}
		})
	}
}

// TestRecordSkip_SkippedCases_AppendOneLineEach holds the notices file a job
// prints: each skip is one line naming its test, a reason that spans lines
// stays on its own, and no path records nothing.
func TestRecordSkip_SkippedCases_AppendOneLineEach(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "skips.txt")
	if err := recordSkip(path, "TestProxyGuide_Caddy", "could not start caddy:2.11-alpine (exit status 125):\n::error::from the log\n"); err != nil {
		t.Fatalf("recordSkip: %v", err)
	}
	if err := recordSkip(path, "TestBalancer_HAProxy", "docker not available"); err != nil {
		t.Fatalf("recordSkip: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the notices: %v", err)
	}
	want := "TestProxyGuide_Caddy: could not start caddy:2.11-alpine (exit status 125): ::error::from the log\n" +
		"TestBalancer_HAProxy: docker not available\n"
	if string(got) != want {
		t.Errorf("the notices file holds %q, want %q", got, want)
	}

	t.Run("no path records nothing", func(t *testing.T) {
		t.Parallel()
		if noPathErr := recordSkip("", "TestProxyGuide_Caddy", "docker not available"); noPathErr != nil {
			t.Errorf("recordSkip with no path = %v, want nil", noPathErr)
		}
	})
}
