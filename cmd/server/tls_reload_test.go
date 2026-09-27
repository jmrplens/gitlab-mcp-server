package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rotatingPair writes a certificate and key into dir under fixed names, and
// returns the serial number that identifies which generation it is.
//
// The two files are always rewritten in place, because that is what a rotation
// does: certbot, a Kubernetes secret projection and Vault's agent all replace
// the contents behind the paths the server was started with, and a reloader
// that only noticed a new path would notice none of them.
//
// The modification times are set explicitly rather than left to the write.
// Two certificates of the same shape can have the same byte count, and a
// filesystem with coarse timestamps could stamp two writes a few milliseconds
// apart with the same time, which would make the test depend on where it runs
// rather than on the code.
func rotatingPair(t *testing.T, dir string, serial int64, when time.Time) (certPath, keyPath string) {
	t.Helper()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	certPEM, keyPEM := mintPair(t, serial)
	writePEM(t, certPath, certPEM, when)
	writePEM(t, keyPath, keyPEM, when)
	return certPath, keyPath
}

// mintPair returns a self-signed certificate for the loopback address and its
// key, both PEM encoded, carrying the serial number the caller asked for.
func mintPair(t *testing.T, serial int64) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "gitlab-mcp-server rotation test"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating a certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling the key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// writePEM writes one PEM file and stamps it with the given time.
func writePEM(t *testing.T, path string, body []byte, when time.Time) {
	t.Helper()
	//#nosec G703 -- both halves of every path given here are the test's own: a t.TempDir and a literal
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("stamping %s: %v", path, err)
	}
}

// servedSerial returns the serial number of the leaf certificate the reloader
// would present to a client right now.
func servedSerial(t *testing.T, r *certReloader) int64 {
	t.Helper()
	cert, err := r.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert == nil || len(cert.Certificate) == 0 {
		t.Fatal("GetCertificate returned no certificate")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parsing the served leaf: %v", err)
	}
	return leaf.SerialNumber.Int64()
}

// TestCertReloader_PresentsTheCertificateThatIsOnDiskNow is the reason this
// type exists: a certificate replaced under a running server is the one the
// next handshake gets, with no restart.
//
// Loading the pair once at startup made rotation a restart, and a restart cuts
// every call in flight and empties the credential pool of the instance being
// restarted. This asserts the whole promise at the seam a handshake reaches.
func TestCertReloader_PresentsTheCertificateThatIsOnDiskNow(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	certPath, keyPath := rotatingPair(t, dir, 1, base)

	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	if got := servedSerial(t, reloader); got != 1 {
		t.Fatalf("served serial before the rotation = %d, want 1", got)
	}

	rotatingPair(t, dir, 2, base.Add(time.Minute))

	if got := servedSerial(t, reloader); got != 2 {
		t.Errorf("served serial after the rotation = %d, want 2: the new pair on disk was not picked up", got)
	}
}

// TestCertReloader_UnchangedFilesAreReadOnce keeps the check off the hot path.
//
// The staleness test is two stats; the load is a parse of two PEM files and a
// key-pair match. Doing the second on every handshake would put a measurable
// cost on every new connection for a file that changes twice a year, so the
// stamp has to be what decides, and this is what says it does.
func TestCertReloader_UnchangedFilesAreReadOnce(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := rotatingPair(t, dir, 7, time.Now().Add(-time.Hour))

	loads := 0
	original := loadTLSKeyPair
	loadTLSKeyPair = func(certFile, keyFile string) (tls.Certificate, error) {
		loads++
		return original(certFile, keyFile)
	}
	t.Cleanup(func() { loadTLSKeyPair = original })

	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	for range 5 {
		servedSerial(t, reloader)
	}

	if loads != 1 {
		t.Errorf("the pair was loaded %d times across five handshakes with no rotation, want 1", loads)
	}
}

// TestStampOf_ReadsEachFilesOwnSizeAndTime pins which file each of the four
// stamp fields comes from. Every rotation test above moves both files at
// once, so a stamp that read the key's time off the certificate, or the
// certificate's size off the key, noticed every one of them; it would miss a
// replacement that changes only the field it lost, such as a file copied in
// with its modification time preserved. The two files here differ in both
// size and time, so no field can stand in for another.
func TestStampOf_ReadsEachFilesOwnSizeAndTime(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	// The sub-second parts are nonzero so a stamp that kept whole seconds
	// would fail, and whole multiples of 100 ns because NTFS records file
	// times at that granularity and rounds anything finer away.
	certTime := time.Date(2026, 9, 1, 10, 0, 0, 11_100, time.UTC)
	keyTime := time.Date(2026, 9, 2, 11, 30, 0, 22_200, time.UTC)
	writePEM(t, certPath, []byte("twenty-five bytes of cert"), certTime)
	writePEM(t, keyPath, []byte("key"), keyTime)

	got, err := stampOf(certPath, keyPath)
	if err != nil {
		t.Fatalf("stampOf: %v", err)
	}
	want := certStamp{certSize: 25, certTime: certTime.UnixNano(), keySize: 3, keyTime: keyTime.UnixNano()}
	if got != want {
		t.Errorf("stampOf = %+v, want %+v", got, want)
	}
}

// TestCertReloader_ARotationIsLoadedOnce covers what a reload leaves behind.
// The new stamp has to become the one later handshakes compare against, or
// every handshake after a rotation parses the pair again and logs a reload,
// which is the hot-path cost the stamp exists to avoid, returning for the
// rest of the process's life after the first renewal.
func TestCertReloader_ARotationIsLoadedOnce(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	certPath, keyPath := rotatingPair(t, dir, 1, base)

	loads := 0
	original := loadTLSKeyPair
	loadTLSKeyPair = func(certFile, keyFile string) (tls.Certificate, error) {
		loads++
		return original(certFile, keyFile)
	}
	t.Cleanup(func() { loadTLSKeyPair = original })

	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	rotatingPair(t, dir, 2, base.Add(time.Minute))
	for range 5 {
		if got := servedSerial(t, reloader); got != 2 {
			t.Fatalf("served serial after the rotation = %d, want 2", got)
		}
	}

	if loads != 2 {
		t.Errorf("the pair was loaded %d times for one rotation and five handshakes, want 2 (startup and the rotation)", loads)
	}
}

// TestCertReloader_ABrokenStateSeenAgainAfterARecoveryIsReportedAgain covers
// the reset that follows a successful reload. A failure is reported once per
// state of the files, and a recovery ends that state: if the same broken pair
// comes back afterwards (a copy that preserves modification times restores
// the exact stamp), it is a new failure and the operator has to hear about
// it, not have it silenced by a report from before the recovery.
//
// Not parallel: it replaces the process-wide default logger.
func TestCertReloader_ABrokenStateSeenAgainAfterARecoveryIsReportedAgain(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	const failure = "the TLS certificate on disk could not be loaded"

	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	certPath, keyPath := rotatingPair(t, dir, 1, base)
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("reading the key: %v", err)
	}
	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	breakPair := func() {
		writePEM(t, certPath, []byte("not a certificate"), base.Add(time.Minute))
		writePEM(t, keyPath, keyPEM, base)
	}

	breakPair()
	servedSerial(t, reloader)
	servedSerial(t, reloader)
	if n := strings.Count(logged.String(), failure); n != 1 {
		t.Fatalf("the broken pair was reported %d times across two handshakes, want once", n)
	}

	rotatingPair(t, dir, 2, base.Add(2*time.Minute))
	if got := servedSerial(t, reloader); got != 2 {
		t.Fatalf("served serial after the recovery = %d, want 2", got)
	}

	breakPair()
	servedSerial(t, reloader)
	if n := strings.Count(logged.String(), failure); n != 2 {
		t.Errorf("the same broken pair returning after a recovery was reported %d times in all, want 2", n)
	}
}

// TestCertReloader_AHalfWrittenRotationKeepsThePreviousCertificate covers the
// window every rotation has.
//
// Writing a certificate and its key is two writes, and between them the pair
// on disk does not match. A reloader that failed the handshake there would
// turn a routine renewal into an outage lasting as long as one file write, on
// whichever instance happened to be asked in that window. The previous
// certificate is still valid, so it is what gets served until the pair is
// whole again.
func TestCertReloader_AHalfWrittenRotationKeepsThePreviousCertificate(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	certPath, keyPath := rotatingPair(t, dir, 1, base)

	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	servedSerial(t, reloader)

	// The certificate of the new generation lands first; its key has not been
	// written yet, so the pair on disk does not match.
	newCert, newKey := mintPair(t, 2)
	writePEM(t, certPath, newCert, base.Add(time.Minute))

	if got := servedSerial(t, reloader); got != 1 {
		t.Errorf("served serial mid-rotation = %d, want the previous 1", got)
	}

	// The key completes the pair, and the next handshake moves on.
	writePEM(t, keyPath, newKey, base.Add(2*time.Minute))

	if got := servedSerial(t, reloader); got != 2 {
		t.Errorf("served serial after the key landed = %d, want 2", got)
	}
}

// TestCertReloader_UnreadableFilesKeepThePreviousCertificate covers the other
// way a rotation can be caught in the middle: the files gone rather than
// mismatched.
//
// A renewal that unlinks before it writes, or a mount that is briefly absent,
// must not take the listener down. The certificate already in memory is
// unaffected by anything happening to the files it was read from.
func TestCertReloader_UnreadableFilesKeepThePreviousCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath, keyFile := rotatingPair(t, dir, 3, time.Now().Add(-time.Hour))

	reloader, err := newCertReloader(certPath, keyFile)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	servedSerial(t, reloader)

	if removeErr := os.Remove(certPath); removeErr != nil {
		t.Fatalf("removing the certificate: %v", removeErr)
	}

	if got := servedSerial(t, reloader); got != 3 {
		t.Errorf("served serial with the certificate file gone = %d, want the loaded 3", got)
	}
}

// TestCertReloader_AnUnreadablePairIsReportedOncePerOutage holds the files
// being gone to the same rule as the files being broken: one WARN for the
// state, not one per handshake. A renewal that unlinks before it writes, or a
// mount that drops, can last long enough for a busy listener to take
// thousands of handshakes, and a line for each buries the one that says what
// is wrong. A recovery ends the state, so the next outage is reported again.
//
// Not parallel: it replaces the process-wide default logger.
func TestCertReloader_AnUnreadablePairIsReportedOncePerOutage(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	const unreadable = "the TLS certificate files could not be read"

	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	certPath, keyPath := rotatingPair(t, dir, 1, base)
	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	if removeErr := os.Remove(certPath); removeErr != nil {
		t.Fatalf("removing the certificate: %v", removeErr)
	}
	for range 3 {
		if got := servedSerial(t, reloader); got != 1 {
			t.Fatalf("served serial with the certificate gone = %d, want the loaded 1", got)
		}
	}
	if n := strings.Count(logged.String(), unreadable); n != 1 {
		t.Errorf("three handshakes against missing files wrote %d reports, want one for the outage", n)
	}

	rotatingPair(t, dir, 2, base.Add(time.Minute))
	if got := servedSerial(t, reloader); got != 2 {
		t.Fatalf("served serial after the files came back = %d, want 2", got)
	}
	if removeErr := os.Remove(keyPath); removeErr != nil {
		t.Fatalf("removing the key: %v", removeErr)
	}
	servedSerial(t, reloader)
	if n := strings.Count(logged.String(), unreadable); n != 2 {
		t.Errorf("a second outage after a recovery brought the reports to %d, want 2", n)
	}
}

// TestCertReloader_EachDistinctBrokenStateIsReported holds the other half of
// "once per state": a state that changes while still broken is a new failure.
// A rotation can fail one way and then another (a truncated certificate, then
// one written whole against a key that is still the old one, then the files
// gone), and each of those says something different about what went wrong.
// A report keyed only on "already failing" would keep the first line and hide
// the rest.
//
// Not parallel: it replaces the process-wide default logger.
func TestCertReloader_EachDistinctBrokenStateIsReported(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	const (
		broken     = "the TLS certificate on disk could not be loaded"
		unreadable = "the TLS certificate files could not be read"
	)

	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	certPath, keyPath := rotatingPair(t, dir, 1, base)
	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	writePEM(t, certPath, []byte("not a certificate"), base.Add(time.Minute))
	servedSerial(t, reloader)
	newCert, _ := mintPair(t, 2)
	writePEM(t, certPath, newCert, base.Add(2*time.Minute))
	servedSerial(t, reloader)
	if n := strings.Count(logged.String(), broken); n != 2 {
		t.Errorf("two different broken pairs in a row were reported %d times, want once each", n)
	}

	if removeErr := os.Remove(certPath); removeErr != nil {
		t.Fatalf("removing the certificate: %v", removeErr)
	}
	if got := servedSerial(t, reloader); got != 1 {
		t.Errorf("served serial with the certificate gone = %d, want the loaded 1", got)
	}
	if n := strings.Count(logged.String(), unreadable); n != 1 {
		t.Errorf("the files going missing after a broken pair wrote %d unreadable reports, want 1", n)
	}
}

// TestNewCertReloader_AnUnloadablePairIsAStartupError keeps the first load
// strict.
//
// Startup is the one moment there is nothing to fall back to and an operator
// is watching, so a path that does not exist or a key that does not match its
// certificate has to be a named error there rather than a handshake failure
// reported later by a client.
func TestNewCertReloader_AnUnloadablePairIsAStartupError(t *testing.T) {
	dir := t.TempDir()
	certPath, _ := rotatingPair(t, dir, 1, time.Now())
	otherDir := t.TempDir()
	_, foreignKey := rotatingPair(t, otherDir, 2, time.Now())

	cases := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{name: "missing files", certFile: filepath.Join(dir, "absent.pem"), keyFile: filepath.Join(dir, "absent.key")},
		{name: "key does not match the certificate", certFile: certPath, keyFile: foreignKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newCertReloader(tc.certFile, tc.keyFile); err == nil {
				t.Error("newCertReloader() = nil error, want the pair refused")
			}
		})
	}
}

// TestTLSConfigFor_WithoutACertificateServesPlain states that TLS stays opt-in.
//
// A nil configuration is what keeps the plain path plain: the caller serves
// without TLS when there is none, and a deployment that configured no
// certificate must not end up demanding one.
func TestTLSConfigFor_WithoutACertificateServesPlain(t *testing.T) {
	cfg, err := tlsConfigFor("", "")
	if err != nil {
		t.Fatalf("tlsConfigFor() error = %v, want none", err)
	}
	if cfg != nil {
		t.Errorf("tlsConfigFor() = %+v, want nil when no certificate is configured", cfg)
	}
}

// TestTLSConfigFor_StatesTheFloorAndReloads pins both properties the listener
// depends on: the version floor is stated rather than inherited, and the
// certificate is served through the callback rather than frozen into
// Certificates, which is what lets net/http leave it alone.
func TestTLSConfigFor_StatesTheFloorAndReloads(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := rotatingPair(t, dir, 5, time.Now().Add(-time.Hour))

	cfg, err := tlsConfigFor(certPath, keyPath)
	if err != nil {
		t.Fatalf("tlsConfigFor() error = %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2 (%#x)", cfg.MinVersion, tls.VersionTLS12)
	}
	if cfg.MaxVersion != 0 {
		t.Errorf("MaxVersion = %#x, want unset so a current client reaches TLS 1.3", cfg.MaxVersion)
	}
	if cfg.GetCertificate == nil {
		t.Fatal("GetCertificate is nil: the certificate would be frozen at startup")
	}
	if len(cfg.Certificates) != 0 {
		t.Errorf("Certificates holds %d entries, want none: net/http replaces a listed certificate", len(cfg.Certificates))
	}
	got, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil || got == nil {
		t.Fatalf("GetCertificate() = %v, %v, want the configured pair", got, err)
	}
}

// TestTLSConfigFor_AnUnloadablePairIsReported keeps the failure on the path
// the caller checks, so a bad pair stops the server instead of producing a
// configuration that cannot complete a handshake.
func TestTLSConfigFor_AnUnloadablePairIsReported(t *testing.T) {
	dir := t.TempDir()
	if _, err := tlsConfigFor(filepath.Join(dir, "nope.pem"), filepath.Join(dir, "nope.key")); err == nil {
		t.Error("tlsConfigFor() = nil error, want the missing pair reported")
	}
}
