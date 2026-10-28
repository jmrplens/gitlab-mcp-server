// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
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
// carrying the deployment's language, English or Spanish, whichever the
// middleware was built with.
func TestLanguageMiddleware_EveryMethodRunsUnderTheLanguage(t *testing.T) {
	type credentialKey struct{}
	for _, lang := range []i18n.Language{i18n.English, i18n.Spanish} {
		for _, method := range []string{"tools/call", "resources/read", "prompts/get", "initialize"} {
			t.Run(string(lang)+"/"+method, func(t *testing.T) {
				parent := context.WithValue(context.Background(), credentialKey{}, "credential")
				var (
					called     string
					sameAsReq  bool
					credential any
					language   i18n.Language
				)
				handler := languageMiddleware(lang)(func(ctx context.Context, got string, _ mcp.Request) (mcp.Result, error) {
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
				if language != lang {
					t.Errorf("the handler reads %q, want %q", language, lang)
				}
			})
		}
	}
}

// TestLanguageMiddleware_HandsBackWhatTheHandlerAnswered pins that the
// middleware changes the context and nothing else: the result and the error
// reach the caller as the handler returned them.
func TestLanguageMiddleware_HandsBackWhatTheHandlerAnswered(t *testing.T) {
	want := &mcp.CallToolResult{IsError: true}
	wantErr := errors.New("refused")
	handler := languageMiddleware(i18n.Spanish)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
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
// and the configured one for en and es. The probe reads what was stored
// rather than what a render would use, because a context carrying no
// language renders in English all the same, so only the stored value tells an
// installed middleware from a missing one for English.
func TestCreateServer_AToolHandlerRunsUnderTheConfiguredLanguage(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang i18n.Language
		want i18n.Language
	}{
		{name: "unset", lang: "", want: i18n.English},
		{name: "en", lang: i18n.English, want: i18n.English},
		{name: "es", lang: i18n.Spanish, want: i18n.Spanish},
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
				if !got.ok || got.lang != tc.want {
					t.Errorf("the handler's context carries %q, %v; want %q, true from the shell's middleware", got.lang, got.ok, tc.want)
				}
			default:
				t.Fatal("tools/call returned without running the probe")
			}
		})
	}
}

// TestCreateServer_UnsetEnglishAndSpanish_ListTheSameCatalog pins the promise
// the setting makes: a deployment that sets nothing, one that sets en and one
// that sets es are listed exactly what the server listed before the setting
// existed. The four listings a strict gateway validates are compared byte for
// byte on each tool surface between a snapshot with no language, which every
// generator and every pool entry built by hand carries, and one configured
// for each language; the language middleware and the shape key change nothing
// any of them lists, so no translated text reaches what a gateway validates
// and the catalog stays the same public, cacheable document in every
// language.
func TestCreateServer_UnsetEnglishAndSpanish_ListTheSameCatalog(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual} {
		t.Run(surface, func(t *testing.T) {
			listed := map[i18n.Language]map[string][]byte{}
			for _, lang := range []i18n.Language{"", i18n.English, i18n.Spanish} {
				server := createServerWithin(t, sharedCreateServerClient(t), &config.ServerConfig{
					ToolSurface:       surface,
					CapabilitySurface: config.CapabilitySurfaceFull,
					Tier:              edition.Ultimate,
					TierExplicit:      true,
					Language:          lang,
				})
				listed[lang] = catalogListings(t, newInMemorySession(t, server))
			}
			if len(listed[""]) != 4 {
				t.Fatalf("compared %d listings, want the four a gateway validates", len(listed[""]))
			}
			for _, lang := range []i18n.Language{i18n.English, i18n.Spanish} {
				for method, unset := range listed[""] {
					t.Run(string(lang)+"/"+method, func(t *testing.T) {
						if got := listed[lang][method]; !bytes.Equal(unset, got) {
							t.Errorf("%s differs between an unset language and %s:\nunset: %.400s\n%s:    %.400s", method, lang, unset, lang, got)
						}
					})
				}
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

// modelFacingCall is one tool call whose answer the model reads, and an
// English sentence that answer has to carry.
type modelFacingCall struct {
	name    string
	surface string
	tool    string
	args    map[string]any
	// asks is whether the client answers an elicitation, with an answer
	// that carries no content, which fails the confirmation closed.
	asks bool
	// protocol is the revision the client asks for in initialize. Empty is
	// the SDK's latest, 2026-07-28, on which a question travels inside the
	// result as an input request; earlierRevision is the one every client
	// that predates it speaks, on which the server asks with an
	// elicitation/create request of its own and waits for the answer.
	protocol string
	says     string
}

// earlierRevision is the latest revision before 2026-07-28, the one the
// synchronous elicitation path serves.
const earlierRevision = "2025-11-25"

// modelFacingCalls are the answers a person-facing text has a model-facing
// twin in, or that a model reads where a person would have been asked: the
// refusal of a guided flow on a client that cannot ask, on the dynamic and
// the individual surface; the refusal a destructive action is answered with
// when the client cannot confirm it, whose opening sentence is the question
// a person would have been asked; the result of a confirmation the client
// answered without its field, asked as an input request and, on an earlier
// revision, as an elicitation/create request; and the errors of the two
// guards that ask before a write, the pull mirror's and the work item's, on
// both surfaces.
func modelFacingCalls() []modelFacingCall {
	mirror := map[string]any{
		"project_id": "42", "url": "https://github.com/example/repo.git",
		"enabled": true, "mirror_overwrites_diverged_branches": true,
	}
	workItem := map[string]any{"full_path": "group/app", "work_item_iid": 7, "assignee_ids": []any{}}
	return []modelFacingCall{
		{
			name: "a guided flow on a client that cannot ask", surface: config.ToolSurfaceDynamic,
			tool: "gitlab_execute_action", args: map[string]any{"action": "interactive.issue_create", "params": map[string]any{"project_id": "42"}},
			says: "Your MCP client does not support elicitation.",
		},
		{
			name: "a guided flow on a client that cannot ask", surface: config.ToolSurfaceIndividual,
			tool: "gitlab_interactive_issue_create", args: map[string]any{"project_id": "42"},
			says: "Your MCP client does not support elicitation.",
		},
		{
			name: "a destructive action on a client that cannot confirm", surface: config.ToolSurfaceIndividual,
			tool: "gitlab_branch_delete", args: map[string]any{"project_id": "42", "branch_name": "feature"},
			says: "Confirm gitlab_branch_delete? This action may be irreversible. The connected client cannot prompt for confirmation.",
		},
		{
			name: "a confirmation answered without its field", surface: config.ToolSurfaceIndividual,
			tool: "gitlab_branch_delete", args: map[string]any{"project_id": "42", "branch_name": "feature"}, asks: true,
			says: "Confirmation failed: elicitation: the answer did not satisfy the requested schema. The action was not executed.",
		},
		{
			name: "a confirmation answered without its field on " + earlierRevision, surface: config.ToolSurfaceIndividual,
			tool: "gitlab_branch_delete", args: map[string]any{"project_id": "42", "branch_name": "feature"}, asks: true,
			protocol: earlierRevision,
			says:     "Confirmation failed: elicitation: the answer did not satisfy the requested schema. The action was not executed.",
		},
		{
			name: "the pull mirror guard", surface: config.ToolSurfaceDynamic,
			tool: "gitlab_execute_action", args: map[string]any{"action": "project.pull_mirror_configure", "params": mirror},
			says: "Configure project 42 to pull from https://github.com/example/repo.git and overwrite diverged branches?",
		},
		{
			name: "the pull mirror guard", surface: config.ToolSurfaceIndividual,
			tool: "gitlab_project_pull_mirror_configure", args: mirror,
			says: "Configure project 42 to pull from https://github.com/example/repo.git and overwrite diverged branches?",
		},
		{
			name: "the work item guard", surface: config.ToolSurfaceDynamic,
			tool: "gitlab_execute_action", args: map[string]any{"action": "issue.work_item_update", "params": workItem},
			says: "Remove every assignee (the current list could not be read) from work item group/app#7?",
		},
		{
			name: "the work item guard", surface: config.ToolSurfaceIndividual,
			tool: "gitlab_update_work_item", args: workItem,
			says: "Remove every assignee (the current list could not be read) from work item group/app#7?",
		},
	}
}

// TestCreateServer_UnderSpanish_TheModelStillReadsEnglish pins the other half
// of the setting: it changes what a person is asked and nothing the model
// reads. Each call of modelFacingCalls is answered under es exactly as it is
// under en, byte for byte, and the answer carries its English sentence. The
// confirmation the client answers is the control that the server was built
// for Spanish at all: the person is asked in Spanish, the question and the
// title of its field, on the revision that carries it as an input request and
// on the earlier one the server sends it to the client itself on, and the
// model is told in English that the action did not run.
func TestCreateServer_UnderSpanish_TheModelStillReadsEnglish(t *testing.T) {
	for _, call := range modelFacingCalls() {
		t.Run(call.surface+"/"+call.name, func(t *testing.T) {
			english := answerUnder(t, call, i18n.English)
			spanish := answerUnder(t, call, i18n.Spanish)
			if spanish.answer != english.answer {
				t.Errorf("the model reads under es\n%s\nwant what it reads under en\n%s", spanish.answer, english.answer)
			}
			if !strings.Contains(spanish.answer, call.says) {
				t.Errorf("the answer under es is\n%s\nwant it to carry %q", spanish.answer, call.says)
			}
			assertAskedTheConfirmation(t, call, english, spanish)
		})
	}
}

// answerUnder makes call on a server built for lang, on call's surface at
// the Ultimate tier, and returns what callAs returns.
func answerUnder(t *testing.T, call modelFacingCall, lang i18n.Language) conversation {
	t.Helper()
	server := mustCreateServer(t, sharedCreateServerClient(t), &config.ServerConfig{
		ToolSurface:       call.surface,
		CapabilitySurface: config.CapabilitySurfaceMinimal,
		Tier:              edition.Ultimate,
		TierExplicit:      true,
		Language:          lang,
	})
	return callAs(t, server, call)
}

// assertAskedTheConfirmation holds what the client of call was asked in each
// language: nothing at all when it cannot ask, and otherwise the destructive
// confirmation once, its question and the title of its one field in English
// under en and in Spanish under es, with the field's key the same in both.
// The Spanish is the catalog's render, which the catalog's own tests hold to
// the reviewed translation; what this adds is that the server asks it, and
// that it is not the English. It also holds the path the question took: an
// elicitation/create request of the server's own on the earlier revision,
// and none on the latest, where the client's SDK asks the input request the
// result carries.
func assertAskedTheConfirmation(t *testing.T, call modelFacingCall, english, spanish conversation) {
	t.Helper()
	if !call.asks {
		if len(english.asked) != 0 || len(spanish.asked) != 0 {
			t.Errorf("a client that cannot ask was asked %+v under en and %+v under es", english.asked, spanish.asked)
		}
		return
	}
	inEnglish := question{
		message: "Confirm gitlab_branch_delete? This action may be irreversible.",
		fields:  "confirmed=Confirm",
	}
	inSpanish := question{
		message: i18n.New(i18n.ConfirmDestructiveIndividual, i18n.S("tool", "gitlab_branch_delete")).In(i18n.Spanish),
		fields:  "confirmed=" + i18n.New(i18n.ElicitationSchemaConfirmTitle).In(i18n.Spanish),
	}
	if inSpanish.message == inEnglish.message || !strings.HasPrefix(inSpanish.message, "¿") || inSpanish.fields == inEnglish.fields {
		t.Fatalf("the catalog renders the confirmation in Spanish as %+v, which is not a Spanish one", inSpanish)
	}
	if !slices.Equal(english.asked, []question{inEnglish}) {
		t.Errorf("under en the person was asked %+v, want [%+v]", english.asked, inEnglish)
	}
	if !slices.Equal(spanish.asked, []question{inSpanish}) {
		t.Errorf("under es the person was asked %+v, want [%+v]", spanish.asked, inSpanish)
	}
	sent := 0
	if call.protocol == earlierRevision {
		sent = 1
	}
	if english.requests != sent || spanish.requests != sent {
		t.Errorf("the server sent %d elicitation/create request(s) under en and %d under es on %q, want %d",
			english.requests, spanish.requests, call.protocol, sent)
	}
}

// conversation is what one call came to: what the model reads, the result's
// content and whether it is an error, as JSON; every question the client was
// asked, in order; and how many of them the server sent as an
// elicitation/create request of its own.
type conversation struct {
	answer   string
	asked    []question
	requests int
}

// question is one elicitation as the client renders it: the message a person
// reads, and the fields of its form as key=title pairs in key order.
type question struct {
	message string
	fields  string
}

// questionOf reads the question of one elicitation request. It runs on the
// client's goroutine, so a schema it cannot read is recorded in the question
// rather than failing the test there.
func questionOf(params *mcp.ElicitParams) question {
	var schema struct {
		Properties map[string]struct {
			Title string `json:"title"`
		} `json:"properties"`
	}
	encoded, err := json.Marshal(params.RequestedSchema)
	if err == nil {
		err = json.Unmarshal(encoded, &schema)
	}
	if err != nil {
		return question{message: params.Message, fields: "unreadable schema: " + err.Error()}
	}
	fields := make([]string, 0, len(schema.Properties))
	for _, key := range slices.Sorted(maps.Keys(schema.Properties)) {
		fields = append(fields, key+"="+schema.Properties[key].Title)
	}
	return question{message: params.Message, fields: strings.Join(fields, ",")}
}

// callAs calls one tool of server through an in-memory session opened at
// call.protocol, from a client that answers every elicitation it is asked
// with an acceptance that carries no content when call.asks, and from one
// without elicitation otherwise, and returns the conversation. The client
// counts the elicitation/create requests it receives in a middleware, which
// sees what the server sends and nothing the client's SDK asks on its own
// behalf.
func callAs(t *testing.T, server *mcp.Server, call modelFacingCall) conversation {
	t.Helper()
	var options *mcp.ClientOptions
	// The handler and the middleware run on the client's goroutines, so the
	// handler hands each question over a channel the test goroutine drains
	// after the call returns, and the middleware counts with an atomic.
	questions := make(chan question, 8)
	if call.asks {
		options = &mcp.ClientOptions{
			ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				select {
				case questions <- questionOf(req.Params):
				default:
				}
				return &mcp.ElicitResult{Action: "accept"}, nil
			},
		}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, options)
	var requests atomic.Int32
	client.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "elicitation/create" {
				requests.Add(1)
			}
			return next(ctx, method, req)
		}
	})
	var sessionOptions *mcp.ClientSessionOptions
	if call.protocol != "" {
		sessionOptions = &mcp.ClientSessionOptions{ProtocolVersion: call.protocol}
	}
	session := newInMemorySessionWith(t, server, client, sessionOptions)
	if negotiated := session.InitializeResult().ProtocolVersion; call.protocol != "" && negotiated != call.protocol {
		t.Fatalf("a client asking for %s was negotiated %s", call.protocol, negotiated)
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: call.tool, Arguments: call.args})
	if err != nil {
		t.Fatalf("tools/call %s: %v", call.tool, err)
	}
	close(questions)
	var asked []question
	for q := range questions {
		asked = append(asked, q)
	}
	body, err := json.Marshal(struct {
		Content []mcp.Content `json:"content"`
		IsError bool          `json:"isError"`
	}{Content: result.Content, IsError: result.IsError})
	if err != nil {
		t.Fatalf("encoding the result of %s: %v", call.tool, err)
	}
	return conversation{answer: string(body), asked: asked, requests: int(requests.Load())}
}
