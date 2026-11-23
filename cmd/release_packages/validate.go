// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// validation is one run of a validator: every problem it finds is collected
// and reported, rather than the first ending the run, since a published npm
// version is permanent, a PyPI file name is burned forever and a NuGet
// version can be unlisted but never replaced, so a release engineer wants
// every defect named before the one irreversible step.
type validation struct {
	name    string
	root    string
	version string
	stdout  io.Writer
	stderr  io.Writer
	// minBinary is the size floor of a packed binary, a field so a test can
	// pack stand-ins well below the real binaries' size.
	minBinary int
	// run starts a program and waits for it, a seam so a test can stand in
	// for npm, pip or dotnet.
	run func(dir string, env []string, name string, args ...string) ([]byte, error)
	// npx and dotnet are the programs a handshake starts the installed
	// package through, fields so a test can stand in for them.
	npx, dotnet string

	problems []string
}

// newValidation returns a run named name over the repository at root,
// holding the packages to version.
func newValidation(name, root, version string, minBinary int, stdout, stderr io.Writer) *validation {
	return &validation{name: name, root: root, version: version, stdout: stdout, stderr: stderr, minBinary: minBinary, run: runProgram, npx: "npx", dotnet: "dotnet"}
}

// failf records one problem and says it at once.
func (v *validation) failf(format string, args ...any) {
	problem := fmt.Sprintf(format, args...)
	v.problems = append(v.problems, problem)
	fmt.Fprintln(v.stderr, "FAIL: "+problem)
}

// check records the problem when ok is false, and reports ok.
func (v *validation) check(ok bool, format string, args ...any) bool {
	if !ok {
		v.failf(format, args...)
	}
	return ok
}

// finish reports the outcome and returns the exit code.
func (v *validation) finish(what string) int {
	if len(v.problems) > 0 {
		fmt.Fprintf(v.stderr, "%s %s: %d failure(s)\n", toolName, v.name, len(v.problems))
		return 1
	}
	fmt.Fprintf(v.stdout, "%s %s: %s valid for version %s\n", toolName, v.name, what, v.version)
	return 0
}

// runProgram runs name with args in dir, with env added to the
// environment, and returns its combined output.
func runProgram(dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(context.Background(), name, args...) //#nosec G204 -- the package manager a validator drives, over packages it built
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

// noticesFirstLine is the line the release's notices open with.
var noticesFirstLine = []byte(noticesHeader + "\n")

// repositoryLicense reads the repository's LICENSE, the text every package
// must carry byte for byte.
func (v *validation) repositoryLicense() []byte {
	data, err := os.ReadFile(localPath(v.root, licensePath)) //#nosec G304 -- the repository's own LICENSE
	if err != nil {
		v.failf("the repository's LICENSE could not be read: %v", err)
	}
	return data
}

// checkNotices holds one package's notices to the generator's header and,
// when the builder recorded one, to the digest it checked them against the
// release's signed checksums.txt at.
func (v *validation) checkNotices(label string, data []byte, digest string) {
	v.check(bytes.HasPrefix(data, noticesFirstLine), "%s: %s does not open with the generator's header", label, noticesName)
	if got := releaseassets.Digest(data); digest != "" && got != digest {
		v.failf("%s: %s is sha256 %s, but the release's signed checksums.txt named %s", label, noticesName, got, digest)
	}
}

// verified is what a builder recorded in verified-binaries.json.
type verified struct {
	Version  string            `json:"version"`
	Verified bool              `json:"verified"`
	Binaries map[string]string `json:"binaries"`
	Notices  string            `json:"notices"`
}

// readVerified loads the record a builder left in dir, and with remove takes
// it out of dir once decoded, so the publish step never meets a file in the
// directory it uploads that is not a package. A record that does not decode
// stays where it is, for the operator to read, and the run fails. A missing
// or unreadable record, one written by a build that skipped the checksums,
// one of another version and one without the notices' digest are each
// problems.
func (v *validation) readVerified(dir string, remove bool) *verified {
	file := filepath.Join(dir, verifiedManifest)
	data, err := os.ReadFile(file) //#nosec G304 -- the record a builder wrote beside its packages
	if err != nil {
		v.failf("the packages were assembled without %s: the builder did not check them against the release's checksums.txt", verifiedManifest)
		return nil
	}
	var record verified
	if err = json.Unmarshal(data, &record); err != nil {
		v.failf("%s is not the record a builder writes: %v", verifiedManifest, err)
		return nil
	}
	if remove {
		if err = os.Remove(file); err != nil {
			v.failf("%s could not be removed: %v", verifiedManifest, err)
		}
	}
	v.check(record.Verified, "%s says the binaries were packaged with --allow-unverified", verifiedManifest)
	v.check(record.Version == v.version, "%s records version %s, but this is %s", verifiedManifest, record.Version, v.version)
	v.check(record.Notices != "", "%s records no digest for %s", verifiedManifest, noticesName)
	return &record
}

// checkBinary holds one packed binary to its platform: the size floor, the
// format and the machine its headers name, read with debug/elf, debug/macho
// or debug/pe, and the digest the builder recorded for it under key. A file
// of the right size whose first bytes look right passes every shape check
// but the last, and five of the six platforms are never run by anything.
func (v *validation) checkBinary(label string, p releasePlatform, data []byte, record *verified, key string) {
	v.check(len(data) >= v.minBinary, "%s: binary is %d bytes, below the %d floor", label, len(data), v.minBinary)
	binary, err := releaseassets.Identify(data)
	switch {
	case err != nil:
		v.failf("%s: the binary %v", label, err)
	case binary.Format == releaseassets.FormatUniversal || binary.GOOS != p.goos() || len(binary.Arches) != 1 || binary.Arches[0] != p.goarch():
		v.failf("%s: the binary is a %s file for %s/%s, not for %s", label, binary.Format, binary.GOOS, strings.Join(binary.Arches, "+"), p.key)
	}
	if record == nil {
		return
	}
	if got, want := releaseassets.Digest(data), record.Binaries[key]; got != want {
		v.failf("%s: the packed binary is sha256 %s, but the release's signed checksums.txt named %s", label, got, want)
	}
}

// executableMode reports whether a mode is a regular file with an execute
// bit, which is what pip and the .NET SDK need to install a binary that runs.
func executableMode(mode uint32) bool { return mode&0o170000 == 0o100000 && mode&0o111 != 0 }

// handshakeRequest is the initialize request every install check sends.
const handshakeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"release-packages-validate","version":"1"}}}` + "\n"

// handshakeSettle is how long the server keeps running after its answer,
// so a line it prints right after the answer is read too.
const handshakeSettle = time.Second

// handshakeEnv is the environment the installed server starts with: an
// instance nothing answers on, so nothing reaches a network, and a token
// that would be refused.
var handshakeEnv = []string{
	"GITLAB_URL=http://127.0.0.1:1",
	"GITLAB_TOKEN=glpat-release-packages-validate",
	"GITLAB_MCP_LOG_LEVEL=error",
	// Scope detection against the unreachable instance would retry in the
	// handshake path.
	"GITLAB_MCP_IGNORE_SCOPES=true",
}

// handshake starts argv as an MCP client would, sends initialize with stdin
// held open (closing it tells a stdio server to shut down, and a server told
// that before it answered is allowed to just go), and holds the answer to the
// version: the first line a JSON-RPC 2.0 result naming that server version,
// and every line stdout carried before the server was stopped JSON-RPC 2.0
// too, the property a stray print would break.
func (v *validation) handshake(label, dir string, env []string, timeout time.Duration, argv ...string) {
	lines, err := talk(dir, env, timeout, argv...)
	if err != nil {
		v.failf("handshake (%s): %v", label, err)
		return
	}
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  struct {
			ServerInfo struct {
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err = json.Unmarshal([]byte(lines[0]), &response); err != nil || response.JSONRPC != "2.0" {
		v.failf("handshake (%s): stdout is not pure JSON-RPC: %q", label, lines[0][:min(len(lines[0]), 120)])
		return
	}
	// Only the answer to this handshake's own initialize counts, as the npm
	// script read the version from the response with id 1 alone.
	if string(response.ID) != "1" {
		v.failf("handshake (%s): the first line answers id %s, not the initialize request's 1", label, response.ID)
		return
	}
	for _, line := range lines[1:] {
		var message struct {
			JSONRPC string `json:"jsonrpc"`
		}
		if strings.TrimSpace(line) != "" && (json.Unmarshal([]byte(line), &message) != nil || message.JSONRPC != "2.0") {
			v.failf("handshake (%s): stdout carried a line that is not JSON-RPC 2.0, which would corrupt the MCP stream: %q", label, line[:min(len(line), 120)])
		}
	}
	if v.check(response.Result.ServerInfo.Version == v.version, "handshake (%s): serverInfo.version = %q, want %q", label, response.Result.ServerInfo.Version, v.version) {
		fmt.Fprintf(v.stdout, "handshake: %s answered initialize with version %s\n", label, v.version)
	}
}

// pipeDrainLimit is how long the handshake waits, after the server has
// ended, for its stdout and stderr to reach their end. A process the server
// or its launcher started and did not reap can hold either open for good,
// and the release job must fail rather than wait on it.
const pipeDrainLimit = 3 * time.Second

// talk runs argv, writes the initialize request and returns every line it
// printed on stdout until it was stopped, the first being its answer. A
// process that printed nothing within timeout is an error carrying the tail
// of its stderr, and so is a stdout the scanner could not read to its end
// or one still open pipeDrainLimit after the server ended.
func talk(dir string, env []string, timeout time.Duration, argv ...string) ([]string, error) {
	cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...) //#nosec G204 -- the command the package under validation installed
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), handshakeEnv...), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// Wait copies stderr until its end, so a descendant holding it would
	// hold Wait too: past this delay Wait closes it and returns.
	cmd.WaitDelay = pipeDrainLimit
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// The read end is the validator's own, so the server can be waited for
	// whenever it ends, and stdout read to its end after that.
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	cmd.Stdout = writer
	err = cmd.Start()
	_ = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("%s could not be started: %w", argv[0], err)
	}
	lines := make(chan string, 64)
	// Sent before lines is closed, so it is ready once lines has drained.
	scanned := make(chan error, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanned <- scanner.Err()
	}()
	_, _ = io.WriteString(stdin, handshakeRequest)
	var first string
	var answered bool
	select {
	case first, answered = <-lines:
	case <-time.After(timeout):
	}
	// A stray print right after the answer is what this check exists to
	// catch, so the server keeps running a moment before it is stopped.
	if answered {
		time.Sleep(handshakeSettle)
	}
	_ = stdin.Close()
	stopCommand(cmd)
	out := []string{first}
	drained := time.After(pipeDrainLimit)
drain:
	for {
		select {
		case line, open := <-lines:
			if !open {
				break drain
			}
			out = append(out, line)
		case <-drained:
			// Closing the read end ends the scanner's read.
			_ = reader.Close()
			return nil, fmt.Errorf("stdout was still open %s after the server ended: a process it started holds it", pipeDrainLimit)
		}
	}
	if !answered {
		tail := stderr.Bytes()
		return nil, fmt.Errorf("no answer within %s; stderr tail: %q", timeout, tail[max(0, len(tail)-800):])
	}
	if err = <-scanned; err != nil {
		return nil, fmt.Errorf("stdout could not be read to its end: %w", err)
	}
	return out, nil
}

// stopCommand ends a started command the way a supervisor does, with
// SIGTERM where there is one, kills it if it is still running fifteen seconds
// later, and waits for it.
func stopCommand(cmd *exec.Cmd) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cmd.Wait()
	}()
	if cmd.Process.Signal(syscall.SIGTERM) != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

// validatorFlags is the command line every validator takes.
type validatorFlags struct {
	set       *flag.FlagSet
	version   string
	noInstall bool
}

// newValidatorFlags declares the shared flags.
func newValidatorFlags(name string, stderr io.Writer) *validatorFlags {
	f := &validatorFlags{set: flag.NewFlagSet(toolName+" "+name, flag.ContinueOnError)}
	f.set.SetOutput(stderr)
	f.set.StringVar(&f.version, "version", "", "the release version the packages carry")
	f.set.BoolVar(&f.noInstall, "no-install", false, "skip the install and the MCP handshake")
	return f
}

// parse reads args, refusing extra arguments and a missing version.
func (f *validatorFlags) parse(args []string, stderr io.Writer) bool {
	if err := f.set.Parse(args); err != nil {
		return false
	}
	if f.set.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: unexpected argument %q\n", f.set.Name(), f.set.Arg(0))
		return false
	}
	if f.version == "" {
		fmt.Fprintf(stderr, "%s: --version is required\n", f.set.Name())
		return false
	}
	return true
}

// glibcSymbol is a versioned glibc symbol reference in a Linux binary.
var glibcSymbol = regexp.MustCompile(`GLIBC_(\d+)\.(\d+)`)

// newestGlibc is the newest glibc version a binary's symbol references
// demand, or 0.0 when it names none.
func newestGlibc(data []byte) (major, minor int) {
	for _, match := range glibcSymbol.FindAllSubmatch(data, -1) {
		a, errA := strconv.Atoi(string(match[1]))
		b, errB := strconv.Atoi(string(match[2]))
		if errors.Join(errA, errB) == nil && (a > major || a == major && b > minor) {
			major, minor = a, b
		}
	}
	return major, minor
}
