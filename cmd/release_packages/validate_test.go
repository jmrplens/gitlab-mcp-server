// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// validate_test.go holds what the three validators share: the fixtures that
// build a release with stand-in binaries the binary checks can read, the
// verified-binaries record, the binary checks themselves, and the MCP
// handshake, driven against this test binary acting as a stand-in server.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeServerEnv makes this test binary answer as a stand-in MCP server: its
// value names how it answers.
const fakeServerEnv = "RELEASE_PACKAGES_FAKE_SERVER"

// init turns the test binary into the stand-in server when the environment
// asks for it, before any test runs.
func init() {
	if mode := os.Getenv(fakeServerEnv); mode != "" {
		// A stubborn server answers like ok and outlives its parent, the
		// launcher before 3.1.0.
		if stubborn, found := strings.CutPrefix(mode, "stubborn-"); found {
			mode = stubborn
		} else {
			go exitWithParent()
		}
		if addr := httpAddr(os.Args); addr != "" {
			os.Exit(fakeHTTPServer(addr))
		}
		os.Exit(fakeServer(mode, os.Stdin, os.Stdout))
	}
}

// exitWithParent ends the stand-in server when its parent changes, which is
// what the npm launcher's own parent watch does for the real one.
func exitWithParent() {
	parent := os.Getppid()
	for os.Getppid() == parent {
		time.Sleep(50 * time.Millisecond)
	}
	os.Exit(0)
}

// httpAddr is the address after --http-addr, or "".
func httpAddr(args []string) string {
	for i, arg := range args {
		if arg == "--http-addr" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// fakeHTTPServer answers /health on addr until the process ends.
func fakeHTTPServer(addr string) int {
	server := &http.Server{Addr: addr, ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	_ = server.ListenAndServe()
	return 1
}

// fakeServer reads the initialize request and answers it as mode says:
// ok answers with version 9.8.7, stray prints a line that is not JSON-RPC
// after the answer, wrong answers another version, garbage prints a line
// that is not JSON, and silent prints nothing until its stdin closes.
func fakeServer(mode string, in io.Reader, out io.Writer) int {
	line, _ := bufioLine(in)
	var request struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(line, &request)
	answer := func(version string) {
		fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%d,"result":{"serverInfo":{"name":"gitlab-mcp-server","version":%q}}}`+"\n", request.ID, version)
	}
	switch mode {
	case "ok":
		answer("9.8.7")
	case "stray":
		answer("9.8.7")
		fmt.Fprintln(out, "debug: a print on stdout")
	case "wrong":
		answer("1.0.0")
	case "garbage":
		fmt.Fprintln(out, "starting gitlab-mcp-server")
	case "otherid":
		request.ID++
		answer("9.8.7")
	case "longline":
		answer("9.8.7")
		fmt.Fprintln(out, strings.Repeat("x", 5<<20))
	case "holdpipes":
		answer("9.8.7")
		holdPipes()
	case "hold":
		// The descendant holdpipes leaves behind: it keeps the stdout and
		// stderr it inherited until the test that started it kills it.
		time.Sleep(time.Minute)
		return 0
	case "silent":
	}
	_, _ = io.Copy(io.Discard, in)
	return 0
}

// holdPipesPIDEnv names the file holdpipes writes its descendant's process
// ID to, so the test can kill it.
const holdPipesPIDEnv = "RELEASE_PACKAGES_HOLD_PIDFILE"

// holdPipes starts this test binary again as a stubborn hold, which inherits
// stdout and stderr and outlives the server, as a process a launcher started
// and did not reap would, and records its process ID.
func holdPipes() {
	cmd := exec.CommandContext(context.Background(), os.Args[0]) //#nosec G204 -- this test binary
	cmd.Env = append(os.Environ(), fakeServerEnv+"=stubborn-hold")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if cmd.Start() != nil {
		return
	}
	_ = os.WriteFile(os.Getenv(holdPipesPIDEnv), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
}

// bufioLine reads one line without buffering past it.
func bufioLine(in io.Reader) ([]byte, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := in.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return line, nil
			}
			line = append(line, b[0])
		}
		if err != nil {
			return line, err
		}
	}
}

// TestHandshake_HoldsTheAnswerToTheVersionAndPureJSONRPC drives the
// handshake against the stand-in server in each way it answers: a correct
// answer passes and is said, and a stray line after it, another version, a
// first line that is not JSON and no answer at all are each a problem.
func TestHandshake_HoldsTheAnswerToTheVersionAndPureJSONRPC(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode, want string
	}{
		{mode: "ok"},
		{mode: "stray", want: "stdout carried a line that is not JSON-RPC 2.0"},
		{mode: "wrong", want: `serverInfo.version = "1.0.0", want "9.8.7"`},
		{mode: "garbage", want: "stdout is not pure JSON-RPC"},
		{mode: "otherid", want: "answers id 2, not the initialize request's 1"},
		{mode: "longline", want: "stdout could not be read to its end"},
		{mode: "silent", want: "no answer within 500ms"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			v := newValidation("validate-test", t.TempDir(), "9.8.7", 1, &stdout, &stderr)
			v.handshake("stand-in", t.TempDir(), []string{fakeServerEnv + "=" + tc.mode}, 500*time.Millisecond, self)
			if tc.want == "" {
				if len(v.problems) != 0 || !strings.Contains(stdout.String(), "stand-in answered initialize with version 9.8.7") {
					t.Errorf("problems %q, stdout %q, want none and the answer said", v.problems, stdout.String())
				}
				return
			}
			if !slices.ContainsFunc(v.problems, func(p string) bool { return strings.Contains(p, tc.want) }) {
				t.Errorf("problems %q, want one naming %q", v.problems, tc.want)
			}
		})
	}
}

// TestHandshake_ADescendantHoldingThePipes_EndsInAProblem verifies that a
// server which leaves a process holding its stdout and stderr, one a
// launcher started and did not reap, ends the handshake in a problem within
// a bounded time rather than holding the release job until it times out.
func TestHandshake_ADescendantHoldingThePipes_EndsInAProblem(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the descendant is killed by its process ID, read back on POSIX terms")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "hold.pid")
	t.Cleanup(func() {
		data, readErr := os.ReadFile(pidFile)
		if readErr != nil {
			return
		}
		if pid, atoiErr := strconv.Atoi(string(data)); atoiErr == nil {
			if process, findErr := os.FindProcess(pid); findErr == nil {
				_ = process.Kill()
			}
		}
	})
	var stdout, stderr bytes.Buffer
	v := newValidation("validate-test", t.TempDir(), "9.8.7", 1, &stdout, &stderr)
	done := make(chan struct{})
	go func() {
		defer close(done)
		v.handshake("stand-in", t.TempDir(), []string{fakeServerEnv + "=holdpipes", holdPipesPIDEnv + "=" + pidFile}, 5*time.Second, self)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Error("the handshake was still waiting 30s after the server was stopped")
		return
	}
	if !slices.ContainsFunc(v.problems, func(p string) bool { return strings.Contains(p, "still open") }) {
		t.Errorf("problems %q, want one saying a pipe was still open", v.problems)
	}
}

// TestHandshake_AProgramThatCannotStart_IsAProblem covers a command that is
// not there.
func TestHandshake_AProgramThatCannotStart_IsAProblem(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	v := newValidation("validate-test", t.TempDir(), "9.8.7", 1, &stdout, &stderr)
	v.handshake("absent", t.TempDir(), nil, time.Second, filepath.Join(t.TempDir(), "no-such-program"))
	if len(v.problems) != 1 || !strings.Contains(v.problems[0], "could not be started") {
		t.Errorf("problems %q, want one saying the program could not be started", v.problems)
	}
}

// standInBinary is the smallest file of p's format and machine the binary
// checks read, padded with bytes naming the platform so each platform's file
// differs: an ELF header, a thin Mach-O header or a PE stub and file header.
func standInBinary(p releasePlatform, fill string) []byte {
	var data []byte
	switch p.goos() {
	case "linux":
		machine := elf.EM_X86_64
		if p.goarch() == "arm64" {
			machine = elf.EM_AARCH64
		}
		data = make([]byte, 64)
		copy(data, elf.ELFMAG)
		data[elf.EI_CLASS] = byte(elf.ELFCLASS64)
		data[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
		data[elf.EI_VERSION] = byte(elf.EV_CURRENT)
		binary.LittleEndian.PutUint16(data[18:], uint16(machine))
		binary.LittleEndian.PutUint32(data[20:], uint32(elf.EV_CURRENT))
		binary.LittleEndian.PutUint16(data[54:], 56)
	case "darwin":
		cpu := macho.CpuAmd64
		if p.goarch() == "arm64" {
			cpu = macho.CpuArm64
		}
		data = make([]byte, 32)
		binary.LittleEndian.PutUint32(data, macho.Magic64)
		binary.LittleEndian.PutUint32(data[4:], uint32(cpu))
		binary.LittleEndian.PutUint32(data[12:], uint32(macho.TypeExec))
	default:
		machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
		if p.goarch() == "arm64" {
			machine = pe.IMAGE_FILE_MACHINE_ARM64
		}
		data = make([]byte, 128)
		copy(data, "MZ")
		binary.LittleEndian.PutUint32(data[0x3c:], 0x40)
		copy(data[0x40:], "PE\x00\x00")
		binary.LittleEndian.PutUint16(data[0x44:], machine)
	}
	return slices.Concat(data, []byte(strings.Repeat(p.key+" "+fill+"\n", 128)))
}

// writeScript writes an executable shell script, for a stand-in of a
// program the install tier starts.
func writeScript(t *testing.T, file, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"+body), 0o700); err != nil { //#nosec G306 -- a test's own script
		t.Fatal(err)
	}
}

// standInServer is a shell script that runs this test binary as the
// stand-in MCP server answering ok.
func standInServer(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return fakeServerEnv + "=ok exec \"" + self + "\" \"$@\"\n"
}

// testFloor is the size floor the validators are held to over stand-ins.
const testFloor = 1024

// releaseTree lays out a repository root with the repository files the
// builders read, frozen in testdata/build_packages.json, and a dist/ of
// stand-in binaries the binary checks can read, with the notices and a
// checksums.txt naming them all.
func releaseTree(t *testing.T) string {
	t.Helper()
	var record packagesRecord
	readRecord(t, "build_packages.json", &record)
	root := t.TempDir()
	for rel, text := range record.RepoFiles {
		writeFile(t, root, rel, []byte(text))
	}
	writeFile(t, root, iconPath, []byte("stand-in icon"))
	var sums strings.Builder
	write := func(name string, data []byte) {
		writeFile(t, root, "dist/"+name, data)
		fmt.Fprintf(&sums, "%s  %s\n", sha256Hex(data), name)
	}
	for _, p := range releasePlatforms {
		write(p.asset, standInBinary(p, "release"))
	}
	write(noticesName, []byte(noticesHeader+"\n\nstand-in notices\n"))
	writeFile(t, root, "dist/checksums.txt", []byte(sums.String()))
	return root
}

// build runs one builder over a release tree and fails the test unless it
// succeeds.
func build(t *testing.T, root string, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := runIn(root, args, &stdout, &stderr, noEnv); code != 0 {
		t.Fatalf("%q exited %d: %s", args, code, stderr.String())
	}
}

// rewriteZip rewrites the archive at file through edit, which receives every
// entry in order with its bytes and mode and returns the entries to write.
// Each is written with its mode as given, so a test can drop the file type
// bits or the execute bit.
func rewriteZip(t *testing.T, file string, edit func([]zipFixtureEntry) []zipFixtureEntry) {
	t.Helper()
	archive, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	var entries []zipFixtureEntry
	for _, f := range archive.File {
		entries = append(entries, zipFixtureEntry{name: f.Name, data: readEntry(t, f), mode: f.ExternalAttrs >> 16})
	}
	if stepErr := archive.Close(); stepErr != nil {
		t.Fatal(stepErr)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range edit(entries) {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate, CreatorVersion: 3<<8 | 20, ExternalAttrs: entry.mode << 16}
		var w io.Writer
		if entry.streamed {
			w, err = writer.CreateHeader(header)
		} else {
			header.CompressedSize64 = uint64(len(entry.data))
			header.UncompressedSize64 = uint64(len(entry.data))
			header.Method = zip.Store
			header.CRC32 = crc32Of(entry.data)
			header.Extra = entry.extra
			w, err = writer.CreateRaw(header)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, stepErr := w.Write(entry.data); stepErr != nil {
			t.Fatal(stepErr)
		}
	}
	if stepErr := writer.Close(); stepErr != nil {
		t.Fatal(stepErr)
	}
	if stepErr := os.WriteFile(file, out.Bytes(), 0o600); stepErr != nil {
		t.Fatal(stepErr)
	}
}

// zipFixtureEntry is one entry rewriteZip writes: streamed writes it with a
// data descriptor, and extra is its extra field.
type zipFixtureEntry struct {
	name     string
	data     []byte
	mode     uint32
	streamed bool
	extra    []byte
}

// crc32Of is the CRC a zip records for data.
func crc32Of(data []byte) uint32 { return crc32.ChecksumIEEE(data) }

// replaceEntry returns an edit that replaces one entry's bytes.
func replaceEntry(name string, data []byte) func([]zipFixtureEntry) []zipFixtureEntry {
	return func(entries []zipFixtureEntry) []zipFixtureEntry {
		for i := range entries {
			if entries[i].name == name {
				entries[i].data = data
			}
		}
		return entries
	}
}

// editEntry returns an edit that rewrites one entry's text, once.
func editEntry(name, old, replacement string) func([]zipFixtureEntry) []zipFixtureEntry {
	return func(entries []zipFixtureEntry) []zipFixtureEntry {
		for i := range entries {
			if entries[i].name == name {
				entries[i].data = []byte(strings.Replace(string(entries[i].data), old, replacement, 1))
			}
		}
		return entries
	}
}

// dropEntry returns an edit that leaves one entry out.
func dropEntry(name string) func([]zipFixtureEntry) []zipFixtureEntry {
	return func(entries []zipFixtureEntry) []zipFixtureEntry {
		return slices.DeleteFunc(entries, func(e zipFixtureEntry) bool { return e.name == name })
	}
}

// setMode returns an edit that records one entry under another mode.
func setMode(name string, mode uint32) func([]zipFixtureEntry) []zipFixtureEntry {
	return func(entries []zipFixtureEntry) []zipFixtureEntry {
		for i := range entries {
			if entries[i].name == name {
				entries[i].mode = mode
			}
		}
		return entries
	}
}

// wantProblem fails the test unless one problem names want.
func wantProblem(t *testing.T, problems []string, want string) {
	t.Helper()
	if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, want) }) {
		t.Errorf("problems %q, want one naming %q", problems, want)
	}
}

// TestReadVerified_RefusesARecordItCannotTrust covers the record a builder
// leaves: absent, unreadable, unverified, of another version, or without the
// notices' digest, each a problem, and removed once read when asked.
func TestReadVerified_RefusesARecordItCannotTrust(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, record, want string
	}{
		{name: "absent", want: "assembled without verified-binaries.json"},
		{name: "not JSON", record: "{", want: "is not the record a builder writes"},
		{name: "unverified", record: `{"version":"9.8.7","verified":false,"notices":"x"}`, want: "packaged with --allow-unverified"},
		{name: "another version", record: `{"version":"1.0.0","verified":true,"notices":"x"}`, want: "records version 1.0.0, but this is 9.8.7"},
		{name: "no notices", record: `{"version":"9.8.7","verified":true}`, want: "records no digest for THIRD_PARTY_NOTICES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if tc.record != "" {
				writeFile(t, dir, verifiedManifest, []byte(tc.record))
			}
			var stdout, stderr bytes.Buffer
			v := newValidation("validate-test", dir, "9.8.7", 1, &stdout, &stderr)
			v.readVerified(dir, true)
			wantProblem(t, v.problems, tc.want)
			// A record that does not decode stays, for the operator to read;
			// one that decodes is taken out of the publish directory.
			_, err := os.Stat(filepath.Join(dir, verifiedManifest))
			switch {
			case tc.name == "not JSON" && err != nil:
				t.Errorf("a record that does not decode was removed: %v", err)
			case tc.name != "not JSON" && err == nil:
				t.Error("the record is still beside the packages after it was read")
			}
		})
	}
}

// TestCheckBinary_HoldsABinaryToItsPlatform covers the binary checks: a
// stand-in of the right format and machine passes, and one below the floor,
// one that is no executable, one for another machine, a universal Mach-O and
// one whose digest is not the recorded one are each named.
func TestCheckBinary_HoldsABinaryToItsPlatform(t *testing.T) {
	t.Parallel()

	linux, darwin := releasePlatforms[0], releasePlatforms[3]
	good := standInBinary(linux, "x")
	record := &verified{Binaries: map[string]string{"k": sha256Hex(good)}}
	fat := make([]byte, 8+2*20)
	binary.BigEndian.PutUint32(fat, macho.MagicFat)
	for _, tc := range []struct {
		name string
		p    releasePlatform
		data []byte
		want string
	}{
		{name: "the right one", p: linux, data: good},
		{name: "below the floor", p: linux, data: good[:100], want: "below the 1024 floor"},
		{name: "not an executable", p: linux, data: bytes.Repeat([]byte("x"), 2048), want: "is not an ELF, 64-bit Mach-O or PE file"},
		{name: "another machine", p: releasePlatforms[1], data: good, want: "for linux/amd64, not for linux-arm64"},
		{name: "another system", p: darwin, data: good, want: "not for darwin-arm64"},
		{name: "another digest", p: linux, data: standInBinary(linux, "swapped"), want: "but the release's signed checksums.txt named"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			v := newValidation("validate-test", t.TempDir(), "9.8.7", testFloor, &stdout, &stderr)
			v.checkBinary("pkg", tc.p, tc.data, record, "k")
			if tc.want == "" {
				if len(v.problems) != 0 {
					t.Errorf("problems %q, want none", v.problems)
				}
				return
			}
			wantProblem(t, v.problems, tc.want)
		})
	}
}

// TestNewestGlibc_ReadsTheNewestVersionDemanded covers the glibc floor's
// reading of symbol versions.
func TestNewestGlibc_ReadsTheNewestVersionDemanded(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		data         string
		major, minor int
	}{
		{data: "no versions"},
		{data: "GLIBC_2.2.5 GLIBC_2.17 GLIBC_2.3", major: 2, minor: 17},
		{data: "GLIBC_2.34 GLIBC_2.17", major: 2, minor: 34},
		{data: "GLIBC_3.0 GLIBC_2.40", major: 3},
	} {
		t.Run(tc.data, func(t *testing.T) {
			t.Parallel()

			if major, minor := newestGlibc([]byte(tc.data)); major != tc.major || minor != tc.minor {
				t.Errorf("newestGlibc(%q) = %d.%d, want %d.%d", tc.data, major, minor, tc.major, tc.minor)
			}
		})
	}
}

// TestValidators_RefuseACommandLineTheyCannotUse covers the flags the three
// validators share: an argument beside the flags, a flag they do not know and
// a missing version are usage errors.
func TestValidators_RefuseACommandLineTheyCannotUse(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"validate-npm", "validate-pypi", "validate-nuget"} {
		for _, args := range [][]string{{"--version", "1.0.0", "extra"}, {"--nope"}, {}} {
			t.Run(name+" "+strings.Join(args, " "), func(t *testing.T) {
				t.Parallel()

				var stdout, stderr bytes.Buffer
				if code := runIn(t.TempDir(), append([]string{name}, args...), &stdout, &stderr, noEnv); code != 2 {
					t.Errorf("exit %d, want 2 (stderr %q)", code, stderr.String())
				}
			})
		}
	}
}
