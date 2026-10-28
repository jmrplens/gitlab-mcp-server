// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// language_test.go drives the real binary in HTTP mode through the language
// setting (issue 1547): a deployment that sets nothing, English or Spanish
// lists the same catalog byte for byte; the questions and progress a guided
// flow sends a person are English as before and Spanish under es, with the
// property keys and the options a client answers with untouched; the refusal
// the model reads when the client cannot ask is English in every language; a
// language the server has no catalog for refuses the start before the
// listener opens, as it refuses a stdio start; and the flag wins over the
// variable.
package httpe2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// listedToolsOverHTTP starts a server publishing a fake GitLab under env and
// flags and returns its stateless tools/list answer.
func listedToolsOverHTTP(t *testing.T, gitlabURL string, env map[string]string, flags ...string) string {
	t.Helper()
	srv := startServer(t, env, append([]string{"--gitlab-url=" + gitlabURL}, flags...)...)
	got := srv.do(t, request{
		body:    toolsListBody,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-language"},
	})
	if got.status != http.StatusOK {
		t.Fatalf("tools/list = %d: %s", got.status, truncate(got.body))
	}
	return jsonRPCPayload(t, got.body)
}

// httpLanguageSetting is one way an operator configures the language of an
// HTTP deployment: the variable, the flag, or both.
type httpLanguageSetting struct {
	name  string
	env   map[string]string
	flags []string
}

// start starts a server under setting publishing gitlabURL.
func (setting httpLanguageSetting) start(t *testing.T, gitlabURL string) *server {
	t.Helper()
	return startServer(t, setting.env, append([]string{"--gitlab-url=" + gitlabURL}, setting.flags...)...)
}

// spanishOverHTTP are the ways an operator asks for Spanish: the variable,
// with a region, and the flag, alone and over an English variable.
var spanishOverHTTP = []httpLanguageSetting{
	{name: "es through the variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "es"}},
	{name: "es-419 through the variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "es-419"}},
	{name: "es through the flag", flags: []string{"--language=es"}},
	{name: "es through the flag over an English variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}, flags: []string{"--language=es"}},
}

// TestLanguage_AnyLanguage_ListsWhatWasListedBeforeOverHTTP pins the promise
// on the transport a hosted deployment uses: unset, blank, en in any case and
// with a region, en through the flag, en through the flag over a variable the
// server would refuse, and es through the variable or the flag, each start
// serves and lists the same tools byte for byte. What a gateway validates
// carries nothing translated, in any language.
func TestLanguage_AnyLanguage_ListsWhatWasListedBeforeOverHTTP(t *testing.T) {
	gitlab := startFakeGitLabServingProjects(t)
	baseline := listedToolsOverHTTP(t, gitlab.URL, nil)

	settings := []httpLanguageSetting{
		{name: "blank", env: map[string]string{"GITLAB_MCP_LANGUAGE": "   "}},
		{name: "en", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}},
		{name: "en in upper case with a region", env: map[string]string{"GITLAB_MCP_LANGUAGE": "EN-GB"}},
		{name: "en through the flag", flags: []string{"--language=en"}},
		{name: "the flag beats the variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "fr"}, flags: []string{"--language=en"}},
	}
	for _, setting := range append(settings, spanishOverHTTP...) {
		t.Run(setting.name, func(t *testing.T) {
			if got := listedToolsOverHTTP(t, gitlab.URL, setting.env, setting.flags...); got != baseline {
				t.Errorf("tools/list differs from the one served with no language set:\n got: %.300s\nwant: %.300s", got, baseline)
			}
		})
	}
}

// guidedFlowBody is a stateless 2026-07-28 POST of a guided flow through the
// dynamic surface, from a client that declares what capabilities says in the
// request's own _meta, where that revision carries them. responses and state
// are a later round's answers and the state the previous round returned, and
// are left out of the first round; progress asks for progress notifications.
func guidedFlowBody(t *testing.T, action, params, capabilities, responses, state string, progress bool) string {
	t.Helper()
	retry := ""
	if responses != "" {
		retry += `,"inputResponses":` + responses
	}
	if state != "" {
		quoted, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("encoding the request state: %v", err)
		}
		retry += `,"requestState":` + string(quoted)
	}
	token := ""
	if progress {
		token = `"progressToken":"language-progress",`
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action",`+
		`"arguments":{"action":%q,"params":%s}%s,`+
		`"_meta":{%s"io.modelcontextprotocol/protocolVersion":"2026-07-28",`+
		`"io.modelcontextprotocol/clientCapabilities":%s}}}`,
		action, params, retry, token, capabilities)
}

// formElicitation is the capabilities of a client that renders a form.
const formElicitation = `{"elicitation":{"form":{}}}`

// postMessages posts body, a call of action through gitlab_execute_action, to
// srv and returns every JSON-RPC message the answer carries, in order: each
// frame of an event stream, or the body itself when it is one JSON document.
// The action is mirrored in the Mcp-Param-Action header, as protocol
// 2026-07-28 requires of a client and the SDK enforces once the server's
// tools are registered, which a fresh server's first POST is answered before.
func postMessages(t *testing.T, srv *server, action, body string) []map[string]any {
	t.Helper()
	got := srv.do(t, request{
		body:    body,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-language", "Mcp-Param-Action": action},
	})
	if got.status != http.StatusOK {
		t.Fatalf("the guided flow = %d: %s", got.status, truncate(got.body))
	}
	var frames []string
	for line := range strings.SplitSeq(got.body, "\n") {
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			frames = append(frames, strings.TrimSpace(after))
		}
	}
	if len(frames) == 0 {
		frames = []string{got.body}
	}
	messages := make([]map[string]any, 0, len(frames))
	for _, frame := range frames {
		var message map[string]any
		if err := json.Unmarshal([]byte(frame), &message); err != nil {
			t.Fatalf("a frame of the answer is not JSON-RPC: %v\n%s", err, truncate(got.body))
		}
		messages = append(messages, message)
	}
	return messages
}

// answerOf returns the response among messages, the one message that carries
// the request's id, failing when there is none.
func answerOf(t *testing.T, messages []map[string]any) map[string]any {
	t.Helper()
	for _, message := range messages {
		if _, isResponse := message["id"]; isResponse {
			return message
		}
	}
	t.Fatalf("the answer carries no response, only %v", messages)
	return nil
}

// progressOf returns the messages of the progress notifications among
// messages, in order.
func progressOf(messages []map[string]any) []string {
	var progress []string
	for _, message := range messages {
		if message["method"] != "notifications/progress" {
			continue
		}
		params, _ := message["params"].(map[string]any)
		if text, ok := params["message"].(string); ok {
			progress = append(progress, text)
		}
	}
	return progress
}

// httpQuestion is the first question of a guided flow as it crossed the
// wire: the message a person reads, and the key, title and description of the
// one field its form asks for.
type httpQuestion struct {
	message     string
	property    string
	title       string
	description string
}

// formOf reads the form of the input_required result answer carries under
// id, failing on any other shape of answer: the message, and the one field
// the form asks for with its property key.
func formOf(t *testing.T, answer map[string]any, id string) (message, property string, field map[string]any) {
	t.Helper()
	result, _ := answer["result"].(map[string]any)
	if result["resultType"] != "input_required" {
		t.Fatalf("the guided flow answered %v, want resultType input_required asking for %q", answer, id)
	}
	inputs, _ := result["inputRequests"].(map[string]any)
	asked, _ := inputs[id].(map[string]any)
	if asked["method"] != "elicitation/create" {
		t.Fatalf("inputRequests[%q] = %v, want an elicitation/create request", id, inputs[id])
	}
	params, _ := asked["params"].(map[string]any)
	schema, _ := params["requestedSchema"].(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) != 1 {
		t.Fatalf("the form asks for %v, want one field", properties)
	}
	message, _ = params["message"].(string)
	for key, raw := range properties {
		property = key
		field, _ = raw.(map[string]any)
	}
	return message, property, field
}

// requestStateOf returns the state an input_required result hands the next
// round.
func requestStateOf(answer map[string]any) string {
	result, _ := answer["result"].(map[string]any)
	state, _ := result["requestState"].(string)
	return state
}

// firstQuestionOverHTTP posts the guided issue creation to srv, asking for
// progress, and returns the first form its input_required answer carries and
// the progress messages sent ahead of it.
func firstQuestionOverHTTP(t *testing.T, srv *server) (httpQuestion, []string) {
	t.Helper()
	const action = "interactive.issue_create"
	messages := postMessages(t, srv, action, guidedFlowBody(t, action, `{"project_id":"42"}`, formElicitation, "", "", true))
	message, property, field := formOf(t, answerOf(t, messages), "title")
	question := httpQuestion{message: message, property: property}
	question.title, _ = field["title"].(string)
	question.description, _ = field["description"].(string)
	return question, progressOf(messages)
}

// TestLanguage_UnsetOrEnglish_AsksTheQuestionsAskedBeforeOverHTTP pins, on
// the transport a hosted deployment uses, what moving the person-facing texts
// into the message catalog leaves a deployment that sets no language, or sets
// English: the guided issue creation's first question, answered to a
// stateless POST, is the one asked before the catalog existed, its message,
// its field's key and the field's title, which is that key, and the progress
// sent under the call's progressToken ahead of it is the one sent before. An
// English flag over a Spanish variable is English, since the flag the
// operator typed wins.
func TestLanguage_UnsetOrEnglish_AsksTheQuestionsAskedBeforeOverHTTP(t *testing.T) {
	gitlab := startFakeGitLabServingProjects(t)
	want := httpQuestion{
		message:     "Enter the issue title",
		property:    "title",
		title:       "title",
		description: "Enter the issue title",
	}
	for _, setting := range []httpLanguageSetting{
		{name: "unset"},
		{name: "en", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}},
		{name: "en through the flag", flags: []string{"--language=en"}},
		{name: "en through the flag over a Spanish variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "es"}, flags: []string{"--language=en"}},
	} {
		t.Run(setting.name, func(t *testing.T) {
			got, progress := firstQuestionOverHTTP(t, setting.start(t, gitlab.URL))
			if got != want {
				t.Errorf("the first question =\n%+v\nwant\n%+v", got, want)
			}
			if len(progress) == 0 || progress[0] != "Collecting issue details..." {
				t.Errorf("the progress sent ahead of it = %q, want it to open with %q", progress, "Collecting issue details...")
			}
		})
	}
}

// TestLanguage_Spanish_AsksThePersonInSpanishOverHTTP pins what es changes on
// the HTTP transport: the guided issue creation's first question is asked in
// Spanish, its field titled in Spanish and its property key the English one
// the answer carries back, and the progress sent under the call's
// progressToken ahead of it is Spanish too.
func TestLanguage_Spanish_AsksThePersonInSpanishOverHTTP(t *testing.T) {
	gitlab := startFakeGitLabServingProjects(t)
	want := httpQuestion{
		message:     "Escribe el título del issue",
		property:    "title",
		title:       "Título",
		description: "Escribe el título del issue",
	}
	for _, setting := range spanishOverHTTP {
		t.Run(setting.name, func(t *testing.T) {
			got, progress := firstQuestionOverHTTP(t, setting.start(t, gitlab.URL))
			if got != want {
				t.Errorf("the first question =\n%+v\nwant\n%+v", got, want)
			}
			if len(progress) == 0 || progress[0] != "Recopilando los datos del issue..." {
				t.Errorf("the progress sent ahead of it = %q, want it to open with %q", progress, "Recopilando los datos del issue...")
			}
		})
	}
}

// TestLanguage_Spanish_KeepsTheOptionsAPersonChoosesFromOverHTTP pins what es
// must not change: the values a client answers with. The guided project
// creation is walked through its rounds in stateless POSTs, answering the
// name and declining the description and echoing the state each round hands
// the next, to the question that offers the visibility, which is asked in
// Spanish while its options stay private, internal and public.
func TestLanguage_Spanish_KeepsTheOptionsAPersonChoosesFromOverHTTP(t *testing.T) {
	gitlab := startFakeGitLabServingProjects(t)
	for _, setting := range []httpLanguageSetting{spanishOverHTTP[0], spanishOverHTTP[2]} {
		t.Run(setting.name, func(t *testing.T) {
			srv := setting.start(t, gitlab.URL)
			const action, params = "interactive.project_create", "{}"

			first := answerOf(t, postMessages(t, srv, action, guidedFlowBody(t, action, params, formElicitation, "", "", false)))
			if message, property, _ := formOf(t, first, "name"); message != "Escribe el nombre del proyecto" || property != "name" {
				t.Fatalf("the first round asks %q for %q, want the project name in Spanish", message, property)
			}
			second := answerOf(t, postMessages(t, srv, action, guidedFlowBody(t, action, params, formElicitation,
				`{"name":{"action":"accept","content":{"name":"demo"}}}`, requestStateOf(first), false)))
			if message, property, _ := formOf(t, second, "description"); property != "description" {
				t.Fatalf("the second round asks %q for %q, want the description", message, property)
			}
			third := answerOf(t, postMessages(t, srv, action, guidedFlowBody(t, action, params, formElicitation,
				`{"description":{"action":"decline"}}`, requestStateOf(second), false)))

			message, property, field := formOf(t, third, "visibility")
			if message != "Selecciona la visibilidad del proyecto" {
				t.Errorf("the visibility question = %q, want it in Spanish", message)
			}
			if title, _ := field["title"].(string); property != "selection" || title != "Selección" {
				t.Errorf("the visibility field is %q titled %q, want the key selection titled in Spanish", property, title)
			}
			var options []string
			raw, _ := field["enum"].([]any)
			for _, option := range raw {
				value, _ := option.(string)
				options = append(options, value)
			}
			if want := []string{"private", "internal", "public"}; !slices.Equal(options, want) {
				t.Errorf("the visibility options under es = %q, want GitLab's own %q", options, want)
			}
		})
	}
}

// TestLanguage_Spanish_TheModelIsRefusedInEnglishOverHTTP pins the refusal a
// client without elicitation is answered with, which the model reads and not
// a person: the guided flow cannot ask, and says so in English under es, word
// for word what it says under en.
func TestLanguage_Spanish_TheModelIsRefusedInEnglishOverHTTP(t *testing.T) {
	gitlab := startFakeGitLabServingProjects(t)
	english := refusalOverHTTP(t, httpLanguageSetting{name: "en", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}}, gitlab.URL)
	if !strings.Contains(english, "Your MCP client does not support elicitation.") {
		t.Fatalf("the refusal under en = %s, want the English one", english)
	}
	for _, setting := range []httpLanguageSetting{spanishOverHTTP[0], spanishOverHTTP[2]} {
		t.Run(setting.name, func(t *testing.T) {
			if got := refusalOverHTTP(t, setting, gitlab.URL); got != english {
				t.Errorf("the refusal under es =\n%s\nwant what en says\n%s", got, english)
			}
		})
	}
}

// refusalOverHTTP starts a server under setting and returns, as JSON, the
// content of the result the guided issue creation is answered with when the
// client declares no elicitation, failing unless that result is an error.
func refusalOverHTTP(t *testing.T, setting httpLanguageSetting, gitlabURL string) string {
	t.Helper()
	srv := setting.start(t, gitlabURL)
	const action = "interactive.issue_create"
	answer := answerOf(t, postMessages(t, srv, action, guidedFlowBody(t, action, `{"project_id":"42"}`, `{}`, "", "", false)))
	result, _ := answer["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("a client without elicitation was answered %v, want the refusal", answer)
	}
	body, err := json.Marshal(result["content"])
	if err != nil {
		t.Fatalf("encoding the refusal: %v", err)
	}
	return string(body)
}

// TestLanguage_ALanguageWithoutACatalog_RefusesToStartOverHTTP pins the
// refusal before the listener opens, through the variable and through the
// flag, which writes the variable, with the words a stdio start is refused
// with. The output is the JSON log, where the quotes around the value are
// escaped, so the refusal is matched up to the value.
func TestLanguage_ALanguageWithoutACatalog_RefusesToStartOverHTTP(t *testing.T) {
	base := []string{"--http", "--http-addr=127.0.0.1:0", "--gitlab-url=https://gitlab.example.com"}
	const says = "invalid GITLAB_MCP_LANGUAGE value: expected one of en, es, got "
	for _, tc := range []struct {
		name  string
		env   map[string]string
		flags []string
	}{
		{name: "the variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "fr"}},
		{name: "the flag", flags: []string{"--language=fr"}},
		{name: "the flag over an English variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}, flags: []string{"--language=fr"}},
		{name: "the flag over a Spanish variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "es"}, flags: []string{"--language=fr"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runServerWithEnvExpectingExit(t, serverBinary(t), tc.env, append(slices.Clone(base), tc.flags...)...)
			if err == nil {
				t.Fatalf("the server started and exited cleanly; output:\n%s", out)
			}
			if !strings.Contains(out, says) || !strings.Contains(out, "fr") {
				t.Errorf("the start was not refused with %q; output:\n%s", says, out)
			}
			if strings.Contains(out, "listening on tcp") {
				t.Errorf("the server listened under a language it refuses; output:\n%s", out)
			}
		})
	}
}
