// probe.go answers the container HEALTHCHECK from inside the binary.
//
// The image used to probe http://localhost:8080/health with wget, which is
// right for the default command and wrong for every other listener a
// deployment can choose: another port, a unix socket, or TLS terminated by the
// server itself all reported unhealthy while serving perfectly, and an
// orchestrator then restarted a container whose restart changed nothing.
//
// --probe reads the listener off the running server's own command line instead.
// It finds the other instances of this binary, takes --http-addr, --tls-cert,
// --transport and --http from their arguments, derives where /health is served,
// and asks. A target may also be given outright, for a probe run from outside
// the container or for a deployment whose process list cannot be read.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// probeTimeout bounds one probe attempt. The image's HEALTHCHECK allows five
// seconds, and an attempt that has not answered in three is not going to.
const probeTimeout = 3 * time.Second

// probeBudget bounds the whole run, however many instances discovery finds.
// Attempts are sequential, so two unreachable listeners at probeTimeout each
// would already exceed the image's five-second HEALTHCHECK and be killed
// without a verdict; under this deadline the run answers instead, and a
// caller that imposed a shorter one of its own keeps it.
const probeBudget = 4 * time.Second

// Probe exit codes.
const (
	probeHealthy   = 0
	probeUnhealthy = 1
	probeUsage     = 2
)

// probeHealthPath is what a probe asks for unless a given target named a path.
const probeHealthPath = "/health"

// probeTarget is where a probe connects.
type probeTarget struct {
	// scheme is "http", "https" or "unix".
	scheme string
	// addr is host:port, or the socket path for a unix target.
	addr string
	// path is the request path, /health unless a given target named one.
	path string
	// certFile is the PEM certificate an https listener is verified against,
	// as the only trusted root: the server's own --tls-cert. Empty means the
	// system roots.
	certFile string
}

func (t probeTarget) String() string {
	if t.scheme == "unix" {
		return "unix:" + t.addr + t.path
	}
	return t.scheme + "://" + t.addr + t.path
}

// listenerFlags are the flags the probe reads off a peer's command line. They
// are the ones that decide where, and whether, HTTP is served.
type listenerFlags struct {
	addr      string
	tlsCert   string
	transport string
	http      bool
	httpSet   bool
	// utility is set when the command line is a probe, a shutdown, a version
	// or a help invocation rather than a server.
	utility bool
}

// parseListenerFlags reads the listener flags out of a command line, argv[0]
// excluded. It accepts the spellings the flag package accepts, -name=value and
// -name value with one or two dashes, and ignores every other flag and every
// positional argument rather than refusing them: the command line belongs to a
// process that already parsed it successfully.
//
// A bare -- ends the scan, because it ended the peer's own: everything after
// it was a positional argument to that process, so reading `-- --http` as an
// HTTP listener would send the probe to a port a stdio server never opened.
func parseListenerFlags(args []string) listenerFlags {
	f := listenerFlags{addr: ":8080"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return f
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value, hasValue := "", false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, value, hasValue = name[:eq], name[eq+1:], true
		}
		// takeValue reads a string flag's argument, from the same token or the
		// next one.
		takeValue := func() string {
			if hasValue {
				return value
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch name {
		case "http-addr":
			f.addr = takeValue()
		case "tls-cert":
			f.tlsCert = takeValue()
		case "transport":
			f.transport = takeValue()
		case "http":
			f.httpSet = true
			f.http = !hasValue || !strings.EqualFold(value, "false")
		case "probe", "shutdown", "version", "h", "help":
			f.utility = true
		}
	}
	return f
}

// peerServesHTTP decides whether a peer with these flags has an HTTP listener,
// the way resolveTransport decides it for this process. stdinIsNull answers
// what the peer's --transport auto observed: whether its file descriptor 0 is
// the null device. When that cannot be read, HTTP is assumed and the probe
// itself settles it, which errs toward reporting a stdio instance unhealthy
// rather than an HTTP instance healthy unprobed.
func peerServesHTTP(f listenerFlags, stdinIsNull func() (bool, error)) (serves bool, reason string) {
	switch strings.TrimSpace(strings.ToLower(f.transport)) {
	case transportStdio:
		return false, "--transport=stdio"
	case transportHTTP:
		return true, "--transport=http"
	case transportAuto:
		isNull, err := stdinIsNull()
		if err != nil {
			return true, "--transport=auto and stdin could not be examined (" + err.Error() + "), so HTTP is assumed"
		}
		if isNull {
			return true, "--transport=auto and stdin is " + os.DevNull
		}
		return false, "--transport=auto and stdin is not " + os.DevNull
	}
	if f.http {
		return true, "--http"
	}
	if f.httpSet {
		return false, "--http=false"
	}
	return false, "no transport flag, so stdio"
}

// probeTargetFor derives where a listener with these flags serves /health.
// An unspecified host, which is what :8080 and 0.0.0.0:8080 mean, is reached
// on loopback.
func probeTargetFor(addr, tlsCert string) probeTarget {
	if isUnixSocketAddr(addr) {
		return probeTarget{scheme: "unix", addr: addr, path: probeHealthPath}
	}
	scheme := "http"
	if tlsCert != "" {
		scheme = "https"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return probeTarget{scheme: scheme, addr: addr, path: probeHealthPath, certFile: tlsCert}
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return probeTarget{scheme: scheme, addr: net.JoinHostPort(host, port), path: probeHealthPath, certFile: tlsCert}
}

// parseProbeTarget reads a target given on the command line: an http or https
// URL, unix:<path> or a bare path for a socket, or host:port for plain HTTP.
func parseProbeTarget(s string) (probeTarget, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return probeTarget{}, errors.New("empty target")
	case strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"):
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return probeTarget{}, fmt.Errorf("%q is not a URL with a host", s)
		}
		path := u.Path
		if path == "" {
			path = probeHealthPath
		}
		return probeTarget{scheme: u.Scheme, addr: u.Host, path: path}, nil
	case strings.HasPrefix(s, "unix:"):
		return parseProbeTarget(strings.TrimPrefix(s, "unix:"))
	case isUnixSocketAddr(s):
		return probeTarget{scheme: "unix", addr: s, path: probeHealthPath}, nil
	}
	if _, _, err := net.SplitHostPort(s); err != nil {
		return probeTarget{}, fmt.Errorf("%q is not a URL, a socket path or host:port", s)
	}
	return probeTargetFor(s, ""), nil
}

// probe asks the target for /health and returns nil when it answers 200.
func probe(ctx context.Context, target probeTarget) error {
	tlsConfig, err := probeTLSConfig(target)
	if err != nil {
		return err
	}
	transport := &http.Transport{
		// The proxy environment must not redirect a loopback probe.
		Proxy:           nil,
		TLSClientConfig: tlsConfig,
	}
	requestURL := target.scheme + "://" + target.addr + target.path
	if target.scheme == "unix" {
		socketPath := target.addr
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}
		requestURL = "http://unix" + target.path
	}
	// The request context carries the run's overall deadline, so this timeout
	// is the per-attempt ceiling and whichever is nearer ends the attempt.
	client := &http.Client{Transport: transport, Timeout: probeTimeout}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", target, resp.Status)
	}
	return nil
}

// probeTLSConfig decides how an https listener is verified.
//
// A listener the probe derived from the server's flags is verified against
// the certificate --tls-cert names, read from the same file the server read:
// it is the only root the probe trusts, and the name the probe expects is one
// the certificate itself carries. So a self-signed certificate on a loopback
// address verifies the standard way, with no chain it never had and no host
// name it was never issued for, and nothing else is trusted. Without a
// certificate file, a given https target gets the system roots and the host
// it named.
func probeTLSConfig(target probeTarget) (*tls.Config, error) {
	if target.scheme != "https" || target.certFile == "" {
		return &tls.Config{MinVersion: tls.VersionTLS12}, nil
	}
	cert, err := loadServerCertificate(target.certFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: certificateName(cert),
	}, nil
}

// certificateName is a name the certificate was issued for, which is what the
// probe asks the listener to answer to: a DNS name, else an IP address, else
// the common name.
func certificateName(cert *x509.Certificate) string {
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	if len(cert.IPAddresses) > 0 {
		return cert.IPAddresses[0].String()
	}
	return cert.Subject.CommonName
}

// loadServerCertificate reads the first certificate of a PEM file.
func loadServerCertificate(path string) (*x509.Certificate, error) {
	rest, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the certificate to trust: %w", err)
	}
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, fmt.Errorf("%s holds no CERTIFICATE block", path)
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing the certificate in %s: %w", path, parseErr)
		}
		return cert, nil
	}
}

// probePeer is a running instance of this binary as the probe sees it.
type probePeer struct {
	pid  int32
	args []string
}

// probeDeps are the two things about the machine a discovery probe reads,
// injectable so the decision logic is testable without live processes.
type probeDeps struct {
	// peers lists the other instances of this binary, with their command
	// lines.
	peers func() ([]probePeer, error)
	// stdinIsNull reports whether a peer's file descriptor 0 is the null
	// device.
	stdinIsNull func(pid int32) (bool, error)
}

// runProbe implements --probe and returns the process exit code: 0 when a
// listener answered (or the only instance serves stdio, which has nothing to
// probe and is alive), 1 when none did, 2 for a target that does not parse.
// certFile is the probe's own --tls-cert, the pin for a given https target.
func runProbe(ctx context.Context, args []string, certFile string, deps probeDeps, stderr io.Writer) int {
	// One deadline for the whole run, so a peer that never answers cannot
	// spend the next peer's time. A caller with an earlier deadline of its
	// own keeps it: the context already carries the tighter one.
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > probeBudget {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, probeBudget)
		defer cancel()
	}

	if len(args) > 0 {
		target, err := parseProbeTarget(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "probe: %v\n", err)
			return probeUsage
		}
		target.certFile = certFile
		if probeErr := probe(ctx, target); probeErr != nil {
			fmt.Fprintf(stderr, "probe: %s: %v\n", target, probeErr)
			return probeUnhealthy
		}
		fmt.Fprintf(stderr, "probe: %s answered\n", target)
		return probeHealthy
	}

	peers, err := deps.peers()
	if err != nil {
		fmt.Fprintf(stderr, "probe: listing processes: %v\n", err)
		return probeUnhealthy
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].pid < peers[j].pid })

	var failures []string
	servers := 0
	for _, peer := range peers {
		if len(peer.args) == 0 {
			continue
		}
		flags := parseListenerFlags(peer.args[1:])
		if flags.utility {
			continue
		}
		servers++
		serves, why := peerServesHTTP(flags, func() (bool, error) { return deps.stdinIsNull(peer.pid) })
		if !serves {
			fmt.Fprintf(stderr, "probe: pid %d serves stdio (%s) and is running\n", peer.pid, why)
			return probeHealthy
		}
		target := probeTargetFor(flags.addr, flags.tlsCert)
		if probeErr := probe(ctx, target); probeErr != nil {
			failures = append(failures, fmt.Sprintf("pid %d at %s: %v", peer.pid, target, probeErr))
			continue
		}
		fmt.Fprintf(stderr, "probe: pid %d at %s answered\n", peer.pid, target)
		return probeHealthy
	}
	if servers == 0 {
		fmt.Fprintf(stderr, "probe: no running instance of %s\n", canonicalBinaryName(filepath.Base(os.Args[0])))
		return probeUnhealthy
	}
	fmt.Fprintf(stderr, "probe: %s\n", strings.Join(failures, "; "))
	return probeUnhealthy
}

// livePeers lists the other instances of this binary with their command lines,
// through the same lookup --shutdown uses.
func livePeers() ([]probePeer, error) {
	found, err := findPeers()
	if err != nil {
		return nil, err
	}
	peers := make([]probePeer, 0, len(found))
	for _, p := range found {
		args, argsErr := p.CmdlineSlice()
		if argsErr != nil || len(args) == 0 {
			// A process that vanished between the listing and the read, or
			// one whose command line this user may not see.
			continue
		}
		peers = append(peers, probePeer{pid: p.Pid, args: args})
	}
	return peers, nil
}

// stdinIsNullUnder reports whether the process's file descriptor 0, as
// published under procRoot (/proc on Linux), is the null device.
func stdinIsNullUnder(procRoot string, pid int32) (bool, error) {
	link, err := os.Readlink(filepath.Join(procRoot, strconv.Itoa(int(pid)), "fd", "0"))
	if err != nil {
		return false, err
	}
	return link == os.DevNull, nil
}
