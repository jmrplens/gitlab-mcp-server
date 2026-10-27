// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// TestLanguageMiddleware_EveryMethodRunsUnderTheLanguage pins what the
// middleware hands the handler on every method a person-facing text can be
// written from: a context derived from the request's own, so the deadline and
// the credential the outer middlewares installed still reach the handler,
// carrying the deployment's language. English is the only language this layer
// ships, so the language read back is English either way; what tells the
// middleware's context from the request's is that it is a new one, and the
// second language makes the value itself observable.
func TestLanguageMiddleware_EveryMethodRunsUnderTheLanguage(t *testing.T) {
	type credentialKey struct{}
	for _, method := range []string{"tools/call", "resources/read", "prompts/get", "initialize"} {
		t.Run(method, func(t *testing.T) {
			parent := context.WithValue(context.Background(), credentialKey{}, "credential")
			var (
				called     string
				sameAsReq  bool
				credential any
				language   i18n.Language
			)
			handler := languageMiddleware(i18n.English)(func(ctx context.Context, got string, _ mcp.Request) (mcp.Result, error) {
				called = got
				sameAsReq = ctx == parent
				credential = ctx.Value(credentialKey{})
				language = i18n.FromContext(ctx)
				return &mcp.CallToolResult{}, nil
			})

			if _, err := handler(parent, method, nil); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if called != method {
				t.Errorf("the handler was called for %q, want %q", called, method)
			}
			if sameAsReq {
				t.Error("the handler ran under the request's own context, so no language was put on it")
			}
			if credential != "credential" {
				t.Errorf("the handler's context lost what the request carried: %v", credential)
			}
			if language != i18n.English {
				t.Errorf("the handler reads %q, want %q", language, i18n.English)
			}
		})
	}
}

// TestLanguageMiddleware_HandsBackWhatTheHandlerAnswered pins that the
// middleware changes the context and nothing else: the result and the error
// reach the caller as the handler returned them.
func TestLanguageMiddleware_HandsBackWhatTheHandlerAnswered(t *testing.T) {
	want := &mcp.CallToolResult{IsError: true}
	wantErr := errors.New("refused")
	handler := languageMiddleware(i18n.English)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return want, wantErr
	})

	got, err := handler(context.Background(), "tools/call", nil)
	if got != want || !errors.Is(err, wantErr) {
		t.Errorf("handler() = %v, %v; want %v, %v", got, err, want, wantErr)
	}
}

// TestCreateServer_AToolHandlerRunsUnderTheConfiguredLanguage pins that the
// server shell installs the middleware, which the tests above call by hand: a
// tool added to a server createServer built is called through a session and
// reads the language off its context, the effective one for an unset setting
// and the configured one for en. The probe reads what was stored rather than
// what a render would use, because a context carrying no language renders in
// English all the same, so only the stored value tells an installed
// middleware from a missing one while English is the only language.
func TestCreateServer_AToolHandlerRunsUnderTheConfiguredLanguage(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang i18n.Language
	}{
		{name: "unset", lang: ""},
		{name: "en", lang: i18n.English},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := createServerWithin(t, sharedCreateServerClient(t), &config.ServerConfig{
				ToolSurface:       config.ToolSurfaceDynamic,
				CapabilitySurface: config.CapabilitySurfaceMinimal,
				Tier:              edition.Free,
				TierExplicit:      true,
				Language:          tc.lang,
			})
			type stored struct {
				lang i18n.Language
				ok   bool
			}
			// The handler runs on the server's goroutine, so it records what it
			// read and the test goroutine asserts on it.
			seen := make(chan stored, 1)
			mcp.AddTool(server, &mcp.Tool{Name: "language_probe"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				lang, ok := i18n.Requested(ctx)
				seen <- stored{lang: lang, ok: ok}
				return &mcp.CallToolResult{}, nil, nil
			})

			session := newInMemorySession(t, server)
			if _, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "language_probe", Arguments: map[string]any{}}); err != nil {
				t.Fatalf("tools/call language_probe: %v", err)
			}
			select {
			case got := <-seen:
				if !got.ok || got.lang != i18n.English {
					t.Errorf("the handler's context carries %q, %v; want %q, true from the shell's middleware", got.lang, got.ok, i18n.English)
				}
			default:
				t.Fatal("tools/call returned without running the probe")
			}
		})
	}
}

// TestCreateServer_AnUnsetLanguageAndEnglish_ListTheSameCatalog pins the
// promise this layer makes: a deployment that sets nothing, and one that sets
// en, are served exactly what the server served before the setting existed.
// The four listings a strict gateway validates are compared byte for byte on
// each tool surface between a snapshot with no language, which every
// generator and every pool entry built by hand carries, and one configured
// for English; the language middleware and the shape key change nothing
// either of them lists.
func TestCreateServer_AnUnsetLanguageAndEnglish_ListTheSameCatalog(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual} {
		t.Run(surface, func(t *testing.T) {
			listed := map[i18n.Language]map[string][]byte{}
			for _, lang := range []i18n.Language{"", i18n.English} {
				server := createServerWithin(t, sharedCreateServerClient(t), &config.ServerConfig{
					ToolSurface:       surface,
					CapabilitySurface: config.CapabilitySurfaceFull,
					Tier:              edition.Ultimate,
					TierExplicit:      true,
					Language:          lang,
				})
				listed[lang] = catalogListings(t, newInMemorySession(t, server))
			}
			for method, unset := range listed[""] {
				t.Run(method, func(t *testing.T) {
					if english := listed[i18n.English][method]; !bytes.Equal(unset, english) {
						t.Errorf("%s differs between an unset language and en:\nunset: %.400s\nen:    %.400s", method, unset, english)
					}
				})
			}
		})
	}
}

// catalogListings returns the four listings a client reads the catalog from,
// each as the JSON a client receives.
func catalogListings(t *testing.T, session *mcp.ClientSession) map[string][]byte {
	t.Helper()
	ctx := t.Context()
	listings := map[string]any{}
	var err error
	if listings["tools/list"], err = session.ListTools(ctx, nil); err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if listings["prompts/list"], err = session.ListPrompts(ctx, nil); err != nil {
		t.Fatalf("prompts/list: %v", err)
	}
	if listings["resources/list"], err = session.ListResources(ctx, nil); err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if listings["resources/templates/list"], err = session.ListResourceTemplates(ctx, nil); err != nil {
		t.Fatalf("resources/templates/list: %v", err)
	}
	encoded := make(map[string][]byte, len(listings))
	for method, result := range listings {
		body, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			t.Fatalf("encoding %s: %v", method, marshalErr)
		}
		encoded[method] = body
	}
	return encoded
}

// TestLanguageMiddleware_WithoutIt_AHandlerReadsEnglish pins the path the
// middleware does not cover, an internal inspection or a goroutine started
// from a background context: it reads English, which is what every such
// path was served before the setting existed.
func TestLanguageMiddleware_WithoutIt_AHandlerReadsEnglish(t *testing.T) {
	if got := i18n.FromContext(context.Background()); got != i18n.English {
		t.Errorf("a context the middleware never saw reads %q, want %q", got, i18n.English)
	}
}
