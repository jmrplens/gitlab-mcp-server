package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
)

// decodeDiscoveryCard builds the card for cfg and returns it as a map, failing
// the test if it cannot be built or is not JSON.
func decodeDiscoveryCard(t *testing.T, cfg *config.Config) map[string]any {
	t.Helper()

	raw, buildErr := buildDiscoveryCard(cfg)
	if buildErr != nil {
		t.Fatalf("buildDiscoveryCard: %v", buildErr)
	}
	var card map[string]any
	if decodeErr := json.Unmarshal(raw, &card); decodeErr != nil {
		t.Fatalf("the card is not JSON: %v\n%s", decodeErr, raw)
	}
	return card
}

// TestDiscoveryCard_HoldsTheConstraintsTheExtensionDeclares checks the card
// against the field rules in the extension's schema.ts, which are the ones a
// conformance checker applies and which no Go type enforces.
//
// $schema is pinned to one exact string by a @pattern, so a well-meant change
// to a dated URL or to a private mirror would be a conformance failure rather
// than an improvement. description is capped at 100 characters, which is why
// the card cannot simply reuse projectDescription. name must be reverse-DNS
// with exactly one slash, which is why it cannot reuse the handshake's
// Implementation.Name either.
func TestDiscoveryCard_HoldsTheConstraintsTheExtensionDeclares(t *testing.T) {
	t.Parallel()

	card := decodeDiscoveryCard(t, &config.Config{})

	// The pattern is the extension's own, copied from schema.ts, so the
	// constant is held to what a checker accepts rather than to itself.
	schemaPattern := regexp.MustCompile(`^https://static\.modelcontextprotocol\.io/schemas/v1/server-card\.schema\.json$`)
	if got, _ := card["$schema"].(string); !schemaPattern.MatchString(got) {
		t.Errorf("$schema = %q, want the one string the extension's pattern allows", got)
	}
	// The binary's own version, not the manifest's: a card answers for the
	// process serving it.
	if got, _ := card["version"].(string); got != version {
		t.Errorf("version = %q, want this binary's %q", got, version)
	}

	name, _ := card["name"].(string)
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9.-]+/[a-zA-Z0-9._-]+$`)
	if !namePattern.MatchString(name) {
		t.Errorf("name = %q, want reverse-DNS with exactly one slash", name)
	}

	description, _ := card["description"].(string)
	if n := len(description); n < 1 || n > 100 {
		t.Errorf("description is %d characters, want 1 to 100", n)
	}
	if title, found := card["title"].(string); found && len(title) > 100 {
		t.Errorf("title is %d characters, want at most 100", len(title))
	}

	if got, _ := card["version"].(string); got == "" {
		t.Error("version is empty; it is required and is this binary's own version")
	}

	// A card states identity and connection metadata. Anything that varies
	// per authenticated user belongs to runtime listing, which is the reason
	// the extension omits primitives, so finding one here means the two
	// documents have been confused again.
	for _, key := range []string{"tools", "resources", "resourceTemplates", "prompts", "capabilities", "authentication"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			if _, found := card[key]; found {
				t.Errorf("the card carries %q, which SEP-2127 omits on purpose", key)
			}
		})
	}
}

// TestDiscoveryCard_AgreesWithServerJSON is the drift gate between the two
// documents this project publishes about its own identity.
//
// server.json is the MCP Registry manifest and the card is the SEP-2127
// document, and they are maintained in different places: one is a committed
// JSON file stamped at release, the other is constants compiled into the
// binary. Nothing but this test stops them describing different servers, and
// the description is the field most likely to drift, since the card borrows
// server.json's text purely because it fits the 100-character cap.
//
// isRequired is deliberately NOT compared. The two files are governed by
// different schemas that define that field differently; see
// discoveryCardCredentialHeader.
func TestDiscoveryCard_AgreesWithServerJSON(t *testing.T) {
	t.Parallel()

	data, readErr := os.ReadFile(filepath.Join("..", "..", "server.json"))
	if readErr != nil {
		t.Fatalf("reading server.json: %v", readErr)
	}
	var manifest struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		WebsiteURL  string `json:"websiteUrl"`
		Repository  struct {
			URL    string `json:"url"`
			Source string `json:"source"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("server.json is not JSON: %v", err)
	}

	card := decodeDiscoveryCard(t, &config.Config{})
	repository, _ := card["repository"].(map[string]any)

	for _, tt := range []struct {
		field string
		card  any
		want  string
	}{
		{field: "name", card: card["name"], want: manifest.Name},
		{field: "title", card: card["title"], want: manifest.Title},
		{field: "description", card: card["description"], want: manifest.Description},
		{field: "websiteUrl", card: card["websiteUrl"], want: manifest.WebsiteURL},
		{field: "repository.url", card: repository["url"], want: manifest.Repository.URL},
		{field: "repository.source", card: repository["source"], want: manifest.Repository.Source},
	} {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			if got, _ := tt.card.(string); got != tt.want {
				t.Errorf("the card says %q and server.json says %q", got, tt.want)
			}
		})
	}
}

// TestDiscoveryCard_RemoteIsPublishedOnlyWhenTheDeploymentNamesOne covers the
// one field whose absence is the correct answer.
//
// A card describes a REMOTE server. The only address this process knows to be
// reachable from outside is the one --public-url states: a listen address is
// routinely a loopback address or a unix socket behind a proxy, and publishing
// that would send a client somewhere it cannot go. remotes is optional, so
// omitting it is truthful where guessing would not be.
func TestDiscoveryCard_RemoteIsPublishedOnlyWhenTheDeploymentNamesOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       *config.Config
		wantURL   string
		wantOnly1 bool
	}{
		{
			name: "no public url means no remote",
			cfg:  &config.Config{},
		},
		{
			name:      "a public url is published as the remote",
			cfg:       &config.Config{PublicURL: "https://mcp.example.com/gitlab"},
			wantURL:   "https://mcp.example.com/gitlab",
			wantOnly1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			card := decodeDiscoveryCard(t, tt.cfg)
			remotes, found := card["remotes"].([]any)
			if !tt.wantOnly1 {
				if found {
					t.Errorf("remotes = %v, want none when the deployment names no public URL", remotes)
				}
				return
			}
			if !found || len(remotes) != 1 {
				t.Fatalf("remotes = %v, want exactly one", card["remotes"])
			}
			remote, _ := remotes[0].(map[string]any)
			if got, _ := remote["url"].(string); got != tt.wantURL {
				t.Errorf("remote url = %q, want %q", got, tt.wantURL)
			}
			if got, _ := remote["type"].(string); got != "streamable-http" {
				t.Errorf("remote type = %q, want streamable-http", got)
			}
		})
	}
}

// TestDiscoveryCard_CredentialHeaderFollowsTheAuthMode pins the header a
// client is told to send, which differs by mode and is the card's equivalent
// of the enumerating document's authentication block.
//
// Getting this wrong is not cosmetic: a client that sends PRIVATE-TOKEN to an
// oauth deployment is refused, and one that sends Authorization to a legacy
// deployment is too.
func TestDiscoveryCard_CredentialHeaderFollowsTheAuthMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        *config.Config
		wantHeader string
		// wantBearer is whether the placeholder shows the Bearer scheme,
		// which is the value's shape in one header and wrong in the other.
		wantBearer bool
	}{
		{
			name:       "oauth mode asks for Authorization",
			cfg:        &config.Config{PublicURL: "https://mcp.example.com", AuthMode: config.AuthModeOAuth},
			wantHeader: "Authorization",
			wantBearer: true,
		},
		{
			name:       "legacy mode asks for PRIVATE-TOKEN",
			cfg:        &config.Config{PublicURL: "https://mcp.example.com"},
			wantHeader: "PRIVATE-TOKEN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			header := onlyCredentialHeader(t, decodeDiscoveryCard(t, tt.cfg))
			if got, _ := header["name"].(string); got != tt.wantHeader {
				t.Errorf("header name = %q, want %q", got, tt.wantHeader)
			}
			// The card schema defines isRequired as whether the input must be
			// supplied for the CONNECTION to succeed, and both modes answer a
			// request carrying no credential with 401.
			if required, _ := header["isRequired"].(bool); !required {
				t.Error("isRequired = false, but a connection with no credential is refused")
			}
			if secret, _ := header["isSecret"].(bool); !secret {
				t.Error("isSecret = false for a credential header")
			}
			placeholder, _ := header["placeholder"].(string)
			if got := strings.HasPrefix(placeholder, "Bearer "); got != tt.wantBearer {
				t.Errorf("placeholder = %q, shows the Bearer scheme = %v, want %v", placeholder, got, tt.wantBearer)
			}
			if !strings.Contains(placeholder, "glpat-") {
				t.Errorf("placeholder = %q, want a token's shape a user can recognize", placeholder)
			}
			description, _ := header["description"].(string)
			assertNamesEachScopeForWhatItUnlocks(t, description)
		})
	}
}

// onlyCredentialHeader returns the one header of the card's one remote, and
// fails the test when the card declares any other number of either.
func onlyCredentialHeader(t *testing.T, card map[string]any) map[string]any {
	t.Helper()
	remotes, _ := card["remotes"].([]any)
	if len(remotes) != 1 {
		t.Fatalf("remotes = %v, want exactly one", card["remotes"])
	}
	remote, _ := remotes[0].(map[string]any)
	headers, _ := remote["headers"].([]any)
	if len(headers) != 1 {
		t.Fatalf("headers = %v, want exactly one", remote["headers"])
	}
	header, _ := headers[0].(map[string]any)
	return header
}

// assertNamesEachScopeForWhatItUnlocks holds the credential header's
// description to the two scopes it names. The door admits the minimum scope
// and writes need api, so each is named for the thing it unlocks; told the
// other way round, a user would mint a write token to read.
func assertNamesEachScopeForWhatItUnlocks(t *testing.T, description string) {
	t.Helper()
	for _, want := range []string{
		"Scope " + oauth.MinimumScope + " is enough to",
		oauth.ScopeAPI + " is needed to write",
	} {
		if !strings.Contains(description, want) {
			t.Errorf("description = %q, want it to say %q", description, want)
		}
	}
}

// TestDiscoveryCard_DeclaresTheVersionsTheDeploymentServes pins the remote's
// supportedProtocolVersions to what the protocol version middleware accepts
// for the same --stateless, which is what the extension asks a card to
// reflect. A stateful deployment answers 2026-07-28 with 400, so a card
// declaring it there would send a client to a revision it is then refused.
func TestDiscoveryCard_DeclaresTheVersionsTheDeploymentServes(t *testing.T) {
	t.Parallel()

	for _, stateless := range []bool{true, false} {
		t.Run(fmt.Sprintf("stateless=%v", stateless), func(t *testing.T) {
			t.Parallel()

			card := decodeDiscoveryCard(t, &config.Config{PublicURL: "https://mcp.example.com", Stateless: stateless})
			remotes, _ := card["remotes"].([]any)
			if len(remotes) != 1 {
				t.Fatalf("remotes = %v, want exactly one", card["remotes"])
			}
			remote, _ := remotes[0].(map[string]any)
			declared, _ := remote["supportedProtocolVersions"].([]any)
			var got []string
			for _, v := range declared {
				s, _ := v.(string)
				got = append(got, s)
			}
			if want := supportedProtocolVersionsFor(stateless); !slices.Equal(got, want) {
				t.Errorf("supportedProtocolVersions = %v, want %v", got, want)
			}
			if slices.Contains(got, protocolVersionStatelessOnly) != stateless {
				t.Errorf("supportedProtocolVersions = %v; %s must be declared exactly when the deployment is stateless",
					got, protocolVersionStatelessOnly)
			}
		})
	}
}

// TestDiscoveryCard_AnEncodingFailure_IsReturnedWrapped drives the one error
// buildDiscoveryCard can return. Nothing the card is built from makes
// encoding/json refuse it, so the encoder is swapped for one that fails after
// writing part of a document. What is held is that the refusal reaches the
// caller, which answers the route with a 503, wrapped with what was being
// encoded, and that no half-rendered card travels beside it.
func TestDiscoveryCard_AnEncodingFailure_IsReturnedWrapped(t *testing.T) {
	refused := errors.New("the encoder refused the card")
	original := marshalDiscoveryCard
	marshalDiscoveryCard = func(any, string, string) ([]byte, error) { return []byte("{"), refused }
	t.Cleanup(func() { marshalDiscoveryCard = original })

	out, err := buildDiscoveryCard(&config.Config{PublicURL: "https://mcp.example.com"})
	if !errors.Is(err, refused) || !strings.Contains(err.Error(), "marshaling the server card") {
		t.Errorf("buildDiscoveryCard() error = %v, want the encoder's refusal wrapped as a card marshaling failure", err)
	}
	if out != nil {
		t.Errorf("buildDiscoveryCard() = %q beside its error, want no card", out)
	}
}
