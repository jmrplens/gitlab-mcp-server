package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
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

	if got, _ := card["$schema"].(string); got != discoveryCardSchema {
		t.Errorf("$schema = %q, want the one string the extension's pattern allows, %q", got, discoveryCardSchema)
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
		if _, found := card[key]; found {
			t.Errorf("the card carries %q, which SEP-2127 omits on purpose", key)
		}
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
	}{
		{
			name:       "oauth mode asks for Authorization",
			cfg:        &config.Config{PublicURL: "https://mcp.example.com", AuthMode: config.AuthModeOAuth},
			wantHeader: "Authorization",
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

			card := decodeDiscoveryCard(t, tt.cfg)
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
		})
	}
}
