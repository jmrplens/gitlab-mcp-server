// tls_reload.go serves the certificate that is on disk now, not the one that
// was there when the process started.
//
// A certificate expires, and a deployment that terminates TLS on the listener
// itself has to replace it without a gap. Loading the pair once at startup
// makes that a restart: every in-flight call is cut, every pooled credential
// is rebuilt on the instances that come back, and a fleet has to be rolled for
// something that changed no configuration. With the pair behind
// [tls.Config.GetCertificate] the rotation is two file writes, and the next
// handshake presents the new certificate while the connections already open
// keep the old one until they are replaced.
//
// The check is a stat of both files on the handshake path rather than a timer
// or a signal. A timer needs an interval to explain and leaves a window; a
// signal needs the operator to have somewhere to send it, which a container
// whose renewal is an updated mounted secret does not. Two stats cost
// nothing next to a handshake, and nothing is read again while the files are
// unchanged.

package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// certReloader holds the certificate pair a TLS listener presents, and
// re-reads it when the files it was read from change.
type certReloader struct {
	certFile string
	keyFile  string

	mu sync.Mutex
	// cert is the pair currently presented. It is never nil after
	// newCertReloader returns, and a failed reload leaves it in place.
	cert *tls.Certificate
	// source is what the two files looked like when cert was read.
	source certStamp
	// reported is the stamp of the last pair that failed to load, so a pair
	// that stays broken is reported once rather than once per handshake.
	reported certStamp
}

// certStamp is the observable identity of a certificate pair on disk: the size
// and modification time of each file.
//
// It is a value so that "unchanged" is one comparison. The modification time
// is kept as nanoseconds since the epoch rather than as a [time.Time], whose
// equality also compares a monotonic reading and a location pointer that have
// nothing to do with the file.
//
// Size and time together are what every rotation tool moves: certbot, Vault's
// agent and a Kubernetes secret projection all write a new file, and a
// filesystem this server can run on records that write to the nanosecond. A
// replacement that kept both the byte count and the modification timestamp
// would not be noticed, and there is no such rotation.
type certStamp struct {
	certSize int64
	certTime int64
	keySize  int64
	keyTime  int64
}

// stampOf reads the current identity of the pair.
func stampOf(certFile, keyFile string) (certStamp, error) {
	certInfo, err := os.Stat(certFile)
	if err != nil {
		return certStamp{}, fmt.Errorf("stat %q: %w", certFile, err)
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		return certStamp{}, fmt.Errorf("stat %q: %w", keyFile, err)
	}
	return certStamp{
		certSize: certInfo.Size(),
		certTime: certInfo.ModTime().UnixNano(),
		keySize:  keyInfo.Size(),
		keyTime:  keyInfo.ModTime().UnixNano(),
	}, nil
}

// newCertReloader loads the pair once and returns the reloader that will keep
// it current.
//
// The first load is the operator's error to see: a path that does not exist or
// a key that does not match its certificate stops startup, which is what
// validateTLSFiles already promises. Every later load is a rotation, and a
// rotation that fails must not stop a running server.
func newCertReloader(certFile, keyFile string) (*certReloader, error) {
	reloader := &certReloader{certFile: certFile, keyFile: keyFile}
	cert, err := loadTLSKeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading the TLS certificate and key: %w", err)
	}
	reloader.cert = &cert
	// A stat failure here is not fatal: the pair loaded, so the server can
	// serve. The zero stamp then differs from whatever the next handshake
	// reads, which reloads once and settles.
	reloader.source, _ = stampOf(certFile, keyFile)
	return reloader, nil
}

// GetCertificate answers a TLS handshake with the current pair, reloading it
// first when the files have changed.
//
// It never returns an error once the server is running. A pair that cannot be
// read or does not parse leaves the previous one in place, because refusing
// the handshake would turn a half-written rotation into an outage, and the
// certificate already loaded is still valid until it expires. The failure is
// logged at WARN with the reason, once per distinct state of the files.
func (r *certReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stamp, err := stampOf(r.certFile, r.keyFile)
	if err != nil {
		r.reportOnce(certStamp{}, "the TLS certificate files could not be read; serving the certificate already loaded", err)
		return r.cert, nil
	}
	if stamp == r.source {
		return r.cert, nil
	}

	cert, err := loadTLSKeyPair(r.certFile, r.keyFile)
	if err != nil {
		// The stamp is deliberately not recorded as the source, so the next
		// handshake tries again: a rotation that writes the certificate before
		// its key is a pair that is broken for as long as it takes to write
		// the second file.
		r.reportOnce(stamp, "the TLS certificate on disk could not be loaded; serving the previous one", err)
		return r.cert, nil
	}
	r.cert = &cert
	r.source = stamp
	r.reported = certStamp{}
	slog.Info("reloaded the TLS certificate", "cert_file", r.certFile)
	return r.cert, nil
}

// reportOnce logs a reload failure the first time the files are seen in that
// state.
//
// Without it a pair that stays unreadable writes a line per handshake, which
// buries the one line that says what is wrong under the traffic of every
// client retrying.
func (r *certReloader) reportOnce(stamp certStamp, msg string, err error) {
	if r.reported == stamp && stamp != (certStamp{}) {
		return
	}
	r.reported = stamp
	slog.Warn(msg, "cert_file", r.certFile, "key_file", r.keyFile, "error", err)
}

// tlsConfigFor builds the listener's TLS configuration, or nil when the
// operator configured no certificate.
//
// The floor is stated rather than inherited: the standard library's default
// has moved before and may move again, and a deployment that turned TLS on to
// satisfy an auditor should be able to read the floor off this line. No
// maximum is set, so a current client negotiates TLS 1.3.
func tlsConfigFor(certFile, keyFile string) (*tls.Config, error) {
	if certFile == "" {
		return nil, nil //nolint:nilnil // no certificate configured is not a failure
	}
	reloader, err := newCertReloader(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: reloader.GetCertificate,
	}, nil
}
