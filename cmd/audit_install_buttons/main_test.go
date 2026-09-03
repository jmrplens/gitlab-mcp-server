package main

import (
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
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
