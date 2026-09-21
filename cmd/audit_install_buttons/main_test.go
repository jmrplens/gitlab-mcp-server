package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// dockerEntry is the client entry every install button in this repository
// carries, as JSON, so a test can encode it the way each client expects.
const dockerEntry = `{"command":"docker","args":["run","-i","--rm","-e","GITLAB_TOKEN","ghcr.io/jmrplens/gitlab-mcp-server:latest"],"env":{"GITLAB_TOKEN":"YOUR_GITLAB_TOKEN"}}`

// dockerEntryWithFlag is the same entry as it looked before the flag was
// retired. It is the payload this audit exists to catch.
const dockerEntryWithFlag = `{"command":"docker","args":["run","-i","--rm","-e","GITLAB_TOKEN","ghcr.io/jmrplens/gitlab-mcp-server:latest","--http=false"],"env":{"GITLAB_TOKEN":"YOUR_GITLAB_TOKEN"}}`

// writeButtons builds a repository containing one Markdown file whose buttons
// carry the given payloads, each encoded the way the named client encodes it.
func writeButtons(t *testing.T, links ...string) string {
	t.Helper()
	dir := t.TempDir()
	body := "# Install\n\n" + strings.Join(links, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	return dir
}

func base64Link(host, entry string) string {
	return `<a href="https://` + host + `/install-mcp?name=gitlab&amp;config=` +
		base64.StdEncoding.EncodeToString([]byte(entry)) + `">install</a>`
}

func percentLink(host, entry string) string {
	return `<a href="https://` + host + `/redirect/mcp/install?name=gitlab&amp;config=` +
		url.QueryEscape(entry) + `">install</a>`
}

// TestCollect_ReadsEveryEncodingTheClientsUse verifies that a payload is
// decoded whether it arrives base64 or as percent-encoded JSON.
//
// The two live side by side in this repository: VS Code takes the JSON
// directly, Cursor and LM Studio take it base64. A sweep that understood only
// one of them left the other carrying a retired flag, twice, which is the
// reason this command exists at all.
func TestCollect_ReadsEveryEncodingTheClientsUse(t *testing.T) {
	dir := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		percentLink("insiders.vscode.dev", dockerEntry),
	)

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if len(buttons) != 2 {
		t.Fatalf("collect() found %d buttons, want both encodings", len(buttons))
	}
	for _, b := range buttons {
		if b.Config.Command != "docker" {
			t.Errorf("%s button decoded to command %q, want docker", b.Host, b.Config.Command)
		}
	}
}

// TestCheck_ButtonsThatDisagree_AreReportedWithBothArgumentLists verifies the
// failure this audit is for: one button carrying an argument the others do not.
//
// The report has to name the file, the line and both argument lists, because
// the payload is unreadable in the source and a reader cannot otherwise tell
// what the difference is.
func TestCheck_ButtonsThatDisagree_AreReportedWithBothArgumentLists(t *testing.T) {
	dir := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		base64Link("lmstudio.ai", dockerEntryWithFlag),
	)

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	problems := check(buttons)
	if len(problems) != 1 {
		t.Fatalf("check() reported %d problems, want the one disagreeing button: %v", len(problems), problems)
	}
	for _, want := range []string{"README.md", "lmstudio.ai", "--http=false"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(problems[0], want) {
				t.Errorf("the report does not mention %q: %s", want, problems[0])
			}
		})
	}
}

// TestCheck_ButtonsForDifferentCommands_AreNotComparedWithEachOther verifies
// that a docker button and an npx button are allowed to differ.
//
// They are different installation methods, not a disagreement, and comparing
// them would make the audit fail on a repository that documents both.
func TestCheck_ButtonsForDifferentCommands_AreNotComparedWithEachOther(t *testing.T) {
	npxEntry := `{"command":"npx","args":["-y","@jmrp.io/gitlab-mcp-server"],"env":{}}`
	dir := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		base64Link("lmstudio.ai", npxEntry),
	)

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if problems := check(buttons); len(problems) != 0 {
		t.Errorf("check() reported %v, but the two buttons launch different commands", problems)
	}
}

// TestCollect_APayloadThatIsNotAClientEntry_IsAnErrorRatherThanASkip verifies
// that an unreadable payload fails the audit.
//
// Skipping it would be the worst of both worlds: the button is exactly as
// broken as one carrying a wrong argument, and a silent skip would report a
// clean run over a link nobody can use.
func TestCollect_APayloadThatIsNotAClientEntry_IsAnErrorRatherThanASkip(t *testing.T) {
	dir := writeButtons(t, base64Link("cursor.com", "this is not JSON at all, but it is long enough to match"))

	if _, err := collect(dir); err == nil {
		t.Fatal("collect() accepted a payload that is not a client entry")
	}
}

// TestDecodePayload_ReadsEachFormWithoutMistakingItForAnother covers the
// decoder on its own, including the padding-free and URL-alphabet forms a
// client may produce.
func TestDecodePayload_ReadsEachFormWithoutMistakingItForAnother(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "standard base64", payload: base64.StdEncoding.EncodeToString([]byte(dockerEntry))},
		{name: "base64 without padding", payload: base64.RawStdEncoding.EncodeToString([]byte(dockerEntry))},
		{name: "url-alphabet base64", payload: base64.URLEncoding.EncodeToString([]byte(dockerEntry))},
		{name: "percent-encoded json", payload: url.QueryEscape(dockerEntry)},
		{name: "percent-encoded base64", payload: url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(dockerEntry)))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := decodePayload(tt.payload)
			if err != nil {
				t.Fatalf("decodePayload() error = %v", err)
			}
			if decoded != dockerEntry {
				t.Errorf("decodePayload() = %q, want the entry back unchanged", decoded)
			}
		})
	}
}

// TestConfigParam_DoesNotRunFromOneURLIntoTheNext verifies that a badge image
// wrapped in a link does not make the audit read the badge's URL and the
// button's payload as one match.
//
// Every button in the documentation is written as [![badge](shields.io/...)](install-url),
// so a pattern that crossed the closing parenthesis would decode a truncated
// payload and report a parse failure that has nothing to do with the button.
func TestConfigParam_DoesNotRunFromOneURLIntoTheNext(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(dockerEntry))
	markdown := "[![Install](https://img.shields.io/badge/Install-blue)](https://cursor.com/install-mcp?name=gitlab&config=" + encoded + ")"

	matches := configParam.FindAllStringSubmatch(markdown, -1)
	if len(matches) != 1 {
		t.Fatalf("matched %d times, want the install URL only", len(matches))
	}
	if host := matches[0][1]; host != "cursor.com" {
		t.Errorf("matched host %q, want the install host rather than the badge's", host)
	}
	if got := matches[0][2]; got != encoded {
		t.Errorf("captured %q, want the payload without the trailing parenthesis", got)
	}
}

// writeButtonsIn is [writeButtons] for a file at an arbitrary path under the
// repository root, so a test can exercise the directory walk rather than only
// the single-file root.
func writeButtonsIn(t *testing.T, rel string, links ...string) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(full), err)
	}
	body := "# Install\n\n" + strings.Join(links, "\n") + "\n"
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", full, err)
	}
	return dir
}

// TestCollectRoot_WalksDirectoriesAndSkipsGeneratedTrees verifies that buttons
// are found below a directory root, and that a copy of one under a generated
// tree is not reported a second time.
//
// site/dist and node_modules hold copies of the same pages; counting them
// would report every button twice and make a real disagreement harder to see
// rather than easier.
func TestCollectRoot_WalksDirectoriesAndSkipsGeneratedTrees(t *testing.T) {
	dir := writeButtonsIn(t, "docs/getting-started.md", base64Link("cursor.com", dockerEntry))

	generated := filepath.Join(dir, "docs", "node_modules")
	if err := os.MkdirAll(generated, 0o750); err != nil {
		t.Fatalf("creating the generated tree: %v", err)
	}
	copyOf := filepath.Join(generated, "copy.md")
	if err := os.WriteFile(copyOf, []byte(base64Link("cursor.com", dockerEntryWithFlag)), 0o600); err != nil {
		t.Fatalf("writing the copy: %v", err)
	}

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if len(buttons) != 1 {
		t.Fatalf("collect() found %d buttons, want only the one outside the generated tree: %+v", len(buttons), buttons)
	}
	if buttons[0].File != "docs/getting-started.md" {
		t.Errorf("collect() reported %q, want the documented page", buttons[0].File)
	}
}

// TestCollect_IgnoresFilesWhoseExtensionCarriesNoButtons verifies that a
// payload inside a file the documentation never renders is not audited.
//
// The scan is by extension because a button only matters where a reader can
// click it; a fixture or a log that happens to contain an install URL is not a
// button, and holding it to the same rule would fail the audit over nothing.
func TestCollect_IgnoresFilesWhoseExtensionCarriesNoButtons(t *testing.T) {
	dir := writeButtonsIn(t, "docs/page.md", base64Link("cursor.com", dockerEntry))
	stray := filepath.Join(dir, "docs", "capture.log")
	if err := os.WriteFile(stray, []byte(base64Link("cursor.com", dockerEntryWithFlag)), 0o600); err != nil {
		t.Fatalf("writing the stray file: %v", err)
	}

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if len(buttons) != 1 {
		t.Errorf("collect() found %d buttons, want only the one in a rendered page", len(buttons))
	}
}

// TestRun_AgreeingButtons_ReportTheCountAndSucceed covers the passing path,
// including the -v listing, which is the only way to see what a payload
// actually decodes to.
func TestRun_AgreeingButtons_ReportTheCountAndSucceed(t *testing.T) {
	dir := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		base64Link("lmstudio.ai", dockerEntry),
	)

	var out, errOut strings.Builder
	if code := run(dir, true, &out, &errOut); code != 0 {
		t.Fatalf("run() = %d, want success; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "2 buttons decode cleanly") {
		t.Errorf("run() did not report the button count: %s", out.String())
	}
	if !strings.Contains(out.String(), "cursor.com") {
		t.Errorf("-v did not list the buttons it checked: %s", out.String())
	}
	// The listing is the only place a reader sees what a payload holds, so the
	// column has to carry the decoded entry and not the encoded one it was read
	// from, which is unreadable and is what the audit exists to look past.
	if !strings.Contains(out.String(), dockerEntry) {
		t.Errorf("-v listed the buttons without what they decode to: %s", out.String())
	}
}

// TestRun_DisagreeingButtons_FailAndSayWhichAndHowMany covers the failing path.
func TestRun_DisagreeingButtons_FailAndSayWhichAndHowMany(t *testing.T) {
	dir := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		base64Link("lmstudio.ai", dockerEntryWithFlag),
	)

	var out, errOut strings.Builder
	if code := run(dir, false, &out, &errOut); code != 1 {
		t.Fatalf("run() = %d, want failure", code)
	}
	for _, want := range []string{"lmstudio.ai", "1 problem(s)"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errOut.String(), want) {
				t.Errorf("the report does not mention %q: %s", want, errOut.String())
			}
		})
	}
}

// TestRun_ARepositoryWithNoButtons_FailsRatherThanPassingVacuously verifies
// that finding nothing is treated as the audit looking in the wrong place.
//
// A rename of README.md or of the docs directory would otherwise turn this
// gate into one that passes on every commit while checking nothing, which is
// the failure mode a gate must not have.
func TestRun_ARepositoryWithNoButtons_FailsRatherThanPassingVacuously(t *testing.T) {
	var out, errOut strings.Builder
	if code := run(t.TempDir(), false, &out, &errOut); code != 1 {
		t.Fatalf("run() = %d over a repository with no buttons, want failure", code)
	}
	if !strings.Contains(errOut.String(), "looking in the wrong place") {
		t.Errorf("run() did not say why an empty result is a failure: %s", errOut.String())
	}
}

// TestRun_AnUnreadablePayload_FailsWithTheFileAndLine verifies that a decode
// failure names where to look.
func TestRun_AnUnreadablePayload_FailsWithTheFileAndLine(t *testing.T) {
	dir := writeButtons(t, base64Link("cursor.com", "not a client entry, but long enough to be matched as one"))

	var out, errOut strings.Builder
	if code := run(dir, false, &out, &errOut); code != 1 {
		t.Fatalf("run() = %d over an unreadable payload, want failure", code)
	}
	if !strings.Contains(errOut.String(), "README.md:") {
		t.Errorf("the failure does not name the file and line: %s", errOut.String())
	}
}

// unencodablePayload is long enough for the pattern to match and is in none of
// the encodings the decoder tries: the dot is outside both base64 alphabets,
// and with no percent escape in it there is no second candidate to fall back
// on. It is the one payload shape that reaches the decoder's own failure rather
// than decoding into something that is not a client entry.
const unencodablePayload = "not-base64...not-base64...not-base64...not-base64...not-base64..."

// runMainCapturing runs main with a command line, an exit seam and output files
// of its own, so a test drives the real entry point without parsing the test
// binary's flags or terminating it. It returns the status main exited with and
// what the run wrote to standard output and standard error.
func runMainCapturing(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	previousArgs, previousFlags, previousExit := os.Args, flag.CommandLine, osExit
	previousStdout, previousStderr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		os.Args, flag.CommandLine, osExit = previousArgs, previousFlags, previousExit
		os.Stdout, os.Stderr = previousStdout, previousStderr
	})

	capture := t.TempDir()
	outFile := createCapture(t, filepath.Join(capture, "stdout"))
	errFile := createCapture(t, filepath.Join(capture, "stderr"))

	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = args
	os.Stdout, os.Stderr = outFile, errFile

	exited := false
	osExit = func(status int) { code, exited = status, true }

	main()

	if !exited {
		t.Fatal("main() returned without exiting through osExit")
	}
	return code, readCapture(t, outFile.Name()), readCapture(t, errFile.Name())
}

func createCapture(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating the capture file %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func readCapture(t *testing.T, path string) string {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the capture file %s: %v", path, err)
	}
	return string(written)
}

// TestMainEntry_ParsesItsFlagsAndExitsWithTheStatusRunReturned verifies that
// -dir and -v reach run and that main exits with the status run decided rather
// than one of its own.
//
// The flags are all main adds over run, and both halves matter: a command that
// took -dir and then audited the working directory would pass on this
// repository while checking something else, and one that exited 0 whatever run
// returned would be a gate that never fails.
func TestMainEntry_ParsesItsFlagsAndExitsWithTheStatusRunReturned(t *testing.T) {
	agreeing := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		base64Link("lmstudio.ai", dockerEntry),
	)
	disagreeing := writeButtons(t,
		base64Link("cursor.com", dockerEntry),
		base64Link("lmstudio.ai", dockerEntryWithFlag),
	)

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "agreeing buttons, listed by -v",
			args:       []string{"-dir", agreeing, "-v"},
			wantCode:   0,
			wantStdout: "cursor.com",
		},
		{
			name:       "disagreeing buttons",
			args:       []string{"-dir", disagreeing},
			wantCode:   1,
			wantStderr: "lmstudio.ai",
		},
		{
			name:       "a directory holding no buttons at all",
			args:       []string{"-dir", t.TempDir()},
			wantCode:   1,
			wantStderr: "looking in the wrong place",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runMainCapturing(t, append([]string{"audit_install_buttons"}, tt.args...)...)
			if code != tt.wantCode {
				t.Errorf("main() exited %d, want %d; stdout %q, stderr %q", code, tt.wantCode, stdout, stderr)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("main() wrote %q to stdout, want it to mention %q", stdout, tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("main() wrote %q to stderr, want it to mention %q", stderr, tt.wantStderr)
			}
		})
	}
}

// TestCollect_ButtonsAcrossRoots_AreOrderedByFileThenLine verifies the listing
// is ordered by name rather than by the order the roots happen to be walked in.
//
// It is load-bearing twice over: the -v listing is what a reader diffs between
// runs, and check measures every button of a command against the first one in
// this order, so an order that followed the walk could report the same
// repository's disagreement against either of two buttons.
func TestCollect_ButtonsAcrossRoots_AreOrderedByFileThenLine(t *testing.T) {
	dir := writeButtonsIn(t, "docs/page.md",
		base64Link("lmstudio.ai", dockerEntry),
		base64Link("cursor.com", dockerEntry),
	)
	settings := filepath.Join(dir, ".vscode", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o750); err != nil {
		t.Fatalf("creating .vscode: %v", err)
	}
	entry := `{"install":"https://cursor.com/install-mcp?name=gitlab&config=` +
		base64.StdEncoding.EncodeToString([]byte(dockerEntry)) + `"}`
	if err := os.WriteFile(settings, []byte(entry), 0o600); err != nil {
		t.Fatalf("writing .vscode/mcp.json: %v", err)
	}

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	got := make([]string, 0, len(buttons))
	for _, b := range buttons {
		got = append(got, fmt.Sprintf("%s:%d", b.File, b.Line))
	}
	want := []string{".vscode/mcp.json:1", "docs/page.md:3", "docs/page.md:4"}
	if !slices.Equal(got, want) {
		t.Errorf("collect() listed %v, want %v: .vscode is walked after docs and has to sort before it anyway", got, want)
	}
}

// TestCollect_ARootThatCannotBeStatted_FailsRatherThanPassingAsAbsent verifies
// that only "does not exist" is read as a root this repository does not have.
//
// The absent root is deliberate, because the audit also runs over temporary
// directories holding one file. A root that exists and cannot be read is a
// different thing, and swallowing it would let the audit report a clean run
// over a tree it never looked at.
func TestCollect_ARootThatCannotBeStatted_FailsRatherThanPassingAsAbsent(t *testing.T) {
	dir := writeButtons(t, base64Link("cursor.com", dockerEntry))
	// site/src is one of the scanned roots, so a regular file named site makes
	// stating it fail with something that is not "does not exist".
	if err := os.WriteFile(filepath.Join(dir, "site"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "site", "src")); errors.Is(statErr, fs.ErrNotExist) {
		t.Skipf("this platform reports a path below a regular file as absent: %v", statErr)
	}

	_, err := collect(dir)
	if err == nil {
		t.Fatal("collect() read a root it could not stat as one this repository does not have")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("collect() error = %v, want the stat failure rather than a not-exist", err)
	}
}

// TestCollect_AnUndecodablePayloadBelowADirectoryRoot_StopsTheWalkAndNamesThePage
// verifies that a scan failure inside a walked tree reaches the caller.
//
// A single-file root reports its own failure directly; a page the walk found
// has to carry its error back out through filepath.WalkDir, and a walk that
// returned nil there would report a clean audit over a broken button.
func TestCollect_AnUndecodablePayloadBelowADirectoryRoot_StopsTheWalkAndNamesThePage(t *testing.T) {
	dir := writeButtonsIn(t, "docs/guides/install.md",
		base64Link("cursor.com", "this is not JSON at all, but it is long enough to match"))

	_, err := collect(dir)
	if err == nil {
		t.Fatal("collect() walked past a page whose button does not decode")
	}
	if want := filepath.Join("docs", "guides", "install.md"); !strings.Contains(err.Error(), want) {
		t.Errorf("collect() error = %v, want it to name %s", err, want)
	}
}

// TestCollect_APageThatCannotBeRead_FailsRatherThanBeingSkipped verifies that a
// file the walk offers and the scan cannot open fails the audit.
//
// Skipping it is the same failure as skipping an undecodable payload: the audit
// would report a clean run over a page it never read.
func TestCollect_APageThatCannotBeRead_FailsRatherThanBeingSkipped(t *testing.T) {
	dir := writeButtonsIn(t, "docs/page.md", base64Link("cursor.com", dockerEntry))
	dangling := filepath.Join(dir, "docs", "dangling.md")
	if err := os.Symlink(filepath.Join(dir, "docs", "nothing-here.md"), dangling); err != nil {
		t.Skipf("this platform does not let the test create a symlink: %v", err)
	}

	_, err := collect(dir)
	if err == nil {
		t.Fatal("collect() skipped a page it could not read")
	}
	if !strings.Contains(err.Error(), "dangling.md") {
		t.Errorf("collect() error = %v, want it to name the page it could not read", err)
	}
}

// TestCollect_ADirectoryThatCannotBeListed_FailsRatherThanBeingSkipped
// verifies that a failure to list a subtree reaches the caller instead of being
// read as a subtree with nothing in it.
//
// It is the third shape of the rule the two tests above assert: the audit fails
// on what it could not look at rather than reporting a clean run over it. This
// one is a directory rather than a file, so the failure arrives as the walk's
// error argument and not as a scan that returned one.
func TestCollect_ADirectoryThatCannotBeListed_FailsRatherThanBeingSkipped(t *testing.T) {
	dir := writeButtonsIn(t, "docs/page.md", base64Link("cursor.com", dockerEntry))
	closed := filepath.Join(dir, "docs", "closed")
	if err := os.Mkdir(closed, 0o750); err != nil {
		t.Fatalf("creating the directory: %v", err)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Skipf("this platform does not let the test close a directory: %v", err)
	}
	// Remove it before the temporary directory is, since RemoveAll cannot
	// descend into it. Cleanups run in reverse and t.TempDir registered its own
	// first, so this one goes before it; the directory is empty, and removing
	// one needs permission on the parent rather than on the directory itself.
	t.Cleanup(func() { _ = os.Remove(closed) })
	if _, err := os.ReadDir(closed); err == nil {
		t.Skip("this process lists a directory with no permissions on it, so the walk cannot be made to fail")
	}

	_, err := collect(dir)
	if err == nil {
		t.Fatal("collect() walked past a directory it could not list")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("collect() error = %v, want the permission failure the walk reported", err)
	}
}

// TestScanFile_APathThatCannotBeMadeRelative_IsReportedInFull verifies the
// fallback keeps naming the file when it cannot be expressed relative to the
// root it was given.
//
// Nothing in this command's own scan produces that pair, since every path is
// joined onto the root it is later compared against; the fallback is what keeps
// a failure naming a file rather than an empty string if that stops being true.
func TestScanFile_APathThatCannotBeMadeRelative_IsReportedInFull(t *testing.T) {
	page := filepath.Join(writeButtons(t, base64Link("cursor.com", dockerEntry)), "README.md")

	buttons, err := scanFile("a-relative-root", page)
	if err != nil {
		t.Fatalf("scanFile() error = %v", err)
	}
	if len(buttons) != 1 {
		t.Fatalf("scanFile() found %d buttons, want the one in the page", len(buttons))
	}
	if want := filepath.ToSlash(page); buttons[0].File != want {
		t.Errorf("scanFile() named the button %q, want the full path %q it was given", buttons[0].File, want)
	}
}

// TestDecodePayload_APayloadInNoEncodingItKnows_ReturnsTheBase64Failure
// verifies the decoder hands back the failure of the last encoding it tried
// rather than a message of its own.
//
// That error is a [base64.CorruptInputError], which carries the offset of the
// byte that stopped it, and the offset is the only thing in the report that
// points at where an otherwise unreadable payload goes wrong.
func TestDecodePayload_APayloadInNoEncodingItKnows_ReturnsTheBase64Failure(t *testing.T) {
	decoded, err := decodePayload(unencodablePayload)
	if err == nil {
		t.Fatalf("decodePayload() = %q, want a failure: the payload is in none of the encodings it tries", decoded)
	}
	if _, ok := errors.AsType[base64.CorruptInputError](err); !ok {
		t.Errorf("decodePayload() error = %v (%T), want the decoder's own failure, which names the offending byte", err, err)
	}
}

// TestCollect_APayloadThatDecodesInNoEncoding_NamesTheFileTheLineAndTheHost
// verifies the report distinguishes a payload that never decoded from one that
// decoded into something that is not a client entry.
//
// They are different repairs: the first is a truncated or mangled URL, the
// second a link carrying a configuration this project no longer publishes, and
// a reader who cannot tell them apart starts in the wrong place.
func TestCollect_APayloadThatDecodesInNoEncoding_NamesTheFileTheLineAndTheHost(t *testing.T) {
	dir := writeButtons(t, `<a href="https://cursor.com/install-mcp?name=gitlab&amp;config=`+
		unencodablePayload+`">install</a>`)

	_, err := collect(dir)
	if err == nil {
		t.Fatal("collect() accepted a button whose payload decodes in no encoding")
	}
	for _, want := range []string{"README.md:3", "cursor.com", "does not decode"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("collect() error = %v, want it to mention %q", err, want)
			}
		})
	}
}

// writePageIn writes one file of arbitrary content below dir, creating the
// directories above it, so a test can plant something that is not a page of
// install buttons beside one that is.
func writePageIn(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", full, err)
	}
}

// fileNames is the listing a reader of -v sees, reduced to the paths it names.
func fileNames(buttons []button) []string {
	names := make([]string, 0, len(buttons))
	for _, b := range buttons {
		names = append(names, b.File)
	}
	return names
}

// TestCollect_ARootWalkedFirstThatAlsoSortsFirst_IsLeftWhereItIs verifies the
// listing leaves a pair alone when the walk already produced it in name order.
//
// [TestCollect_ButtonsAcrossRoots_AreOrderedByFileThenLine] exercises only the
// pair the sort has to move, so a comparator that reversed every pair handed to
// it would satisfy that test as well. README.md is scanned before docs and
// sorts before it too, which is the pair that says the order is the files' own.
func TestCollect_ARootWalkedFirstThatAlsoSortsFirst_IsLeftWhereItIs(t *testing.T) {
	dir := writeButtons(t, base64Link("cursor.com", dockerEntry))
	writePageIn(t, dir, "docs/page.md", base64Link("lmstudio.ai", dockerEntry))

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	want := []string{"README.md", "docs/page.md"}
	if got := fileNames(buttons); !slices.Equal(got, want) {
		t.Errorf("collect() listed %v, want %v", got, want)
	}
}

// TestCollect_APageCarryingManyButtons_IsStillListedInLineOrder verifies that
// the order is the comparator's rather than the sort's.
//
// Every other ordering test here hands sort.Slice a handful of buttons, which
// it orders by walking the slice once and comparing each element with the one
// before it; a dozen on one page is past the size at which it stops doing that
// and starts comparing elements with a pivot instead, so the same two buttons
// are asked about in the other direction. The answer has to be the same either
// way, and the -v listing a reader diffs between runs is what depends on it.
func TestCollect_APageCarryingManyButtons_IsStillListedInLineOrder(t *testing.T) {
	const onThePage = 12
	links := make([]string, 0, onThePage)
	for i := range onThePage {
		host := "cursor.com"
		if i%2 == 1 {
			host = "lmstudio.ai"
		}
		links = append(links, base64Link(host, dockerEntry))
	}
	dir := writeButtonsIn(t, "docs/page.md", links...)
	writePageIn(t, dir, ".vscode/mcp.json", `{"install":"https://cursor.com/install-mcp?name=gitlab&config=`+
		base64.StdEncoding.EncodeToString([]byte(dockerEntry))+`"}`)

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	got := make([]string, 0, len(buttons))
	for _, b := range buttons {
		got = append(got, fmt.Sprintf("%s:%d", b.File, b.Line))
	}
	want := []string{".vscode/mcp.json:1"}
	for line := 3; line < 3+onThePage; line++ {
		want = append(want, fmt.Sprintf("docs/page.md:%d", line))
	}
	if !slices.Equal(got, want) {
		t.Errorf("collect() listed %v, want %v", got, want)
	}
}

// TestCollect_ADirectoryThatIsNotThisTree_IsNotWalked verifies that the two
// shapes [sourcewalk.SkipDirBelowRoot] prunes are pruned here too.
//
// The parallel-agent tooling puts a complete worktree of this repository under
// a dot-directory, so a walk that read one would audit another branch's buttons
// as this tree's and report a disagreement nobody can fix from here.
func TestCollect_ADirectoryThatIsNotThisTree_IsNotWalked(t *testing.T) {
	tests := []struct {
		name  string
		plant func(t *testing.T, dir string)
	}{
		{
			name: "a checkout below a scanned root",
			plant: func(t *testing.T, dir string) {
				t.Helper()
				writePageIn(t, dir, "docs/worktree/.git", "gitdir: /elsewhere/.git/worktrees/one\n")
				writePageIn(t, dir, "docs/worktree/page.md", base64Link("cursor.com", dockerEntryWithFlag))
			},
		},
		{
			name: "a dot-directory below a scanned root",
			plant: func(t *testing.T, dir string) {
				t.Helper()
				writePageIn(t, dir, "docs/.cache/page.md", base64Link("cursor.com", dockerEntryWithFlag))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeButtonsIn(t, "docs/page.md", base64Link("cursor.com", dockerEntry))
			tt.plant(t, dir)

			buttons, err := collect(dir)
			if err != nil {
				t.Fatalf("collect() error = %v", err)
			}
			want := []string{"docs/page.md"}
			if got := fileNames(buttons); !slices.Equal(got, want) {
				t.Errorf("collect() listed %v, want %v: the planted tree is not this one", got, want)
			}
		})
	}
}

// malformedEscapePayload is long enough for the pattern to match and carries a
// percent sign that begins no escape, so unescaping it fails rather than
// producing a second candidate to try.
const malformedEscapePayload = "cfg%zzcfg%zzcfg%zzcfg%zzcfg%zzcfg%zzcfg%zzcfg%zzcfg%zz"

// TestCollect_APayloadWhoseEscapesAreMalformed_IsReportedAsNotDecoding
// verifies that a failed unescape leaves the payload alone instead of adding
// what it returned to the candidates.
//
// url.QueryUnescape answers an empty string with its error, and an empty string
// is valid base64 for nothing at all: taking it as a candidate would decode the
// button to "", report it as a configuration that is not a client entry, and
// send a reader looking for a retired flag in a link that is simply mangled.
func TestCollect_APayloadWhoseEscapesAreMalformed_IsReportedAsNotDecoding(t *testing.T) {
	dir := writeButtons(t, `<a href="https://cursor.com/install-mcp?name=gitlab&amp;config=`+
		malformedEscapePayload+`">install</a>`)

	_, err := collect(dir)
	if err == nil {
		t.Fatal("collect() accepted a button whose payload cannot be unescaped")
	}
	if !strings.Contains(err.Error(), "does not decode") {
		t.Errorf("collect() error = %v, want the decode failure", err)
	}
	if strings.Contains(err.Error(), "not a client entry") {
		t.Errorf("collect() error = %v, want the mangled URL named rather than the configuration", err)
	}
}

// reportLine returns the part of a disagreement report introduced by label,
// so a test can hold each argument list to the side of the report it belongs on.
func reportLine(t *testing.T, report, label string) string {
	t.Helper()
	for line := range strings.SplitSeq(report, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, label) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, label))
		}
	}
	t.Fatalf("the report has no %q line: %s", label, report)
	return ""
}

// TestCheck_ADisagreement_NamesTheOffendingButtonAndPutsEachListOnItsOwnSide
// verifies that the report says which button is the odd one and which argument
// list is whose.
//
// [TestCheck_ButtonsThatDisagree_AreReportedWithBothArgumentLists] asks only
// that the retired flag appear somewhere in the report, which a report that
// named the button it agrees with, or that put the majority's arguments under
// "this button", would also satisfy. Both halves are the whole repair: a reader
// edits the file the first line names and makes it look like the second list.
func TestCheck_ADisagreement_NamesTheOffendingButtonAndPutsEachListOnItsOwnSide(t *testing.T) {
	dir := writeButtons(t, base64Link("cursor.com", dockerEntry))
	writePageIn(t, dir, "docs/page.md", base64Link("lmstudio.ai", dockerEntryWithFlag))

	buttons, err := collect(dir)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	problems := check(buttons)
	if len(problems) != 1 {
		t.Fatalf("check() reported %d problems, want the one disagreeing button: %v", len(problems), problems)
	}

	if want := "docs/page.md:1:"; !strings.HasPrefix(problems[0], want) {
		t.Errorf("the report opens with %q, want the offending button at %q", problems[0], want)
	}
	if want := "README.md:3"; !strings.Contains(problems[0], want) {
		t.Errorf("the report does not name %q, the button the others agree with: %s", want, problems[0])
	}
	if got := reportLine(t, problems[0], "this button:"); !strings.Contains(got, "--http=false") {
		t.Errorf("the offending button's arguments read %q, want the retired flag on this side", got)
	}
	if got := reportLine(t, problems[0], "the others:"); strings.Contains(got, "--http=false") {
		t.Errorf("the agreed arguments read %q, want the retired flag off this side", got)
	}
}

// TestCollect_ASubtreeTheWalkCannotList_FailsInAProcessAllowedToReadEverything
// verifies the rule
// [TestCollect_ADirectoryThatCannotBeListed_FailsRatherThanBeingSkipped] states
// through a refusal no privilege lifts.
//
// That test closes the directory with permissions, which a process running as
// root ignores, so it skips wherever CI and the sweep run and the branch it
// covers is held by nothing there. A path longer than the operating system will
// open is refused for everyone; os.Root resolves one component at a time, so
// such a tree can be created although it cannot be walked.
func TestCollect_ASubtreeTheWalkCannotList_FailsInAProcessAllowedToReadEverything(t *testing.T) {
	dir := writeButtonsIn(t, "docs/page.md", base64Link("cursor.com", dockerEntry))

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("opening the repository root: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	segment := strings.Repeat("d", 200)
	deep := "docs"
	for len(filepath.Join(dir, filepath.FromSlash(deep))) < 5000 {
		deep = path.Join(deep, segment)
	}
	if mkdirErr := root.MkdirAll(deep, 0o750); mkdirErr != nil {
		t.Skipf("this platform will not create a path it cannot open: %v", mkdirErr)
	}
	// Remove it before the temporary directory is: cleanups run in reverse and
	// t.TempDir registered its own first, so this one goes before it.
	t.Cleanup(func() { _ = root.RemoveAll(path.Join("docs", segment)) })

	if _, readErr := os.ReadDir(filepath.Join(dir, filepath.FromSlash(deep))); readErr == nil {
		t.Skip("this platform lists a path longer than open(2) is supposed to accept")
	}

	_, collectErr := collect(dir)
	if collectErr == nil {
		t.Fatal("collect() walked past a subtree it could not list")
	}
	if errors.Is(collectErr, fs.ErrNotExist) {
		t.Errorf("collect() error = %v, want the failure to list rather than a not-exist", collectErr)
	}
}
