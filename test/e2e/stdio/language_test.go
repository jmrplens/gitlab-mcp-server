// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// language_test.go drives a stdio server through the language setting
// (issue 1547): what a deployment that sets nothing, English or Spanish lists,
// the same catalog byte for byte; the questions and progress a guided flow
// sends a person, in English as before and in Spanish under es, inside the
// result on 2026-07-28 and as an elicitation/create request on 2025-11-25,
// with the property keys and the options a client answers with untouched;
// the refusal the model reads when the client cannot ask, English in every
// language; what a machine's own locale does to it; the precedence of the
// flag over the variable; and the refusal of a language the server has no
// catalog for, through the variable and through the flag that writes it.
package stdioe2e

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// languageRefusal is what the server refuses a language without a catalog
// with, spelled here because this module cannot import internal/config.
const languageRefusal = `invalid GITLAB_MCP_LANGUAGE value: expected one of en, es, got "fr"`

// listedTools starts a session under env and args, has it serve one tool call
// that reaches GitLab, and returns the tools/list response as it crossed the
// pipe.
func listedTools(t *testing.T, env map[string]string, args ...string) (string, *session) {
	t.Helper()
	s := startSessionWithArgs(t, env, args...)
	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("the server did not serve a tool call: %v\nstderr: %s", got, s.stderrText())
	}
	line, got := s.callRaw(t, request(2, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("tools/list was refused: %v", got)
	}
	return line, s
}

// languageSetting is one way an operator configures the language: the
// variable, the flag, or both.
type languageSetting struct {
	name  string
	value string
	args  []string
}

// envFor returns the environment a session under setting starts with.
func (setting languageSetting) envFor(gitlabURL string) map[string]string {
	env := baseEnv(gitlabURL)
	if setting.value != "" {
		env["GITLAB_MCP_LANGUAGE"] = setting.value
	}
	return env
}

// spanishSettings are the ways an operator asks for Spanish: the variable,
// with a region, and the flag, alone and over an English variable.
var spanishSettings = []languageSetting{
	{name: "es through the variable", value: "es"},
	{name: "es-ES through the variable", value: "es-ES"},
	{name: "es through the flag", args: []string{"--language=es"}},
	{name: "es through the flag over an English variable", value: "en", args: []string{"--language=es"}},
}

// TestLanguage_AnyLanguage_ListsWhatWasListedBefore pins the promise the
// setting makes to every deployment: unset, blank, en in any case and with a
// region, en written through the flag, and es through the variable or the
// flag, the server starts, serves, and lists the same tools byte for byte.
// What a gateway validates carries nothing translated, in any language.
func TestLanguage_AnyLanguage_ListsWhatWasListedBefore(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	baseline, s := listedTools(t, baseEnv(gitlabURL))
	s.terminate(t, 10*time.Second)

	settings := []languageSetting{
		{name: "blank", value: "   "},
		{name: "en", value: "en"},
		{name: "en in upper case with a region", value: "EN-GB"},
		{name: "en through the flag", args: []string{"--language=en"}},
	}
	for _, setting := range append(settings, spanishSettings...) {
		t.Run(setting.name, func(t *testing.T) {
			got, configured := listedTools(t, setting.envFor(gitlabURL), setting.args...)
			if got != baseline {
				t.Errorf("tools/list differs from the one served with no language set:\n got: %.300s\nwant: %.300s", got, baseline)
			}
			configured.terminate(t, 10*time.Second)
		})
	}
}

// TestLanguage_AMachinesSpanishLocale_IsNeverRead pins that the language is
// the operator's setting and nothing else. A Linux desktop exports gettext's
// LANGUAGE (es_ES:es) beside LANG, LC_ALL and LC_MESSAGES, and a server
// started there with no GITLAB_MCP_LANGUAGE starts, serves the same tools as
// one started anywhere else, asks a person in English, and warns about none
// of the four, neither as a retired spelling nor as anything else: read bare,
// the gettext list would have refused the start. The warnings the fake GitLab
// causes on every start (it answers no scope probe) are not about the locale
// and are let through.
func TestLanguage_AMachinesSpanishLocale_IsNeverRead(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	baseline, s := listedTools(t, baseEnv(gitlabURL))
	s.terminate(t, 10*time.Second)

	env := baseEnv(gitlabURL)
	env["LANGUAGE"] = "es_ES:es"
	env["LANG"] = "es_ES.UTF-8"
	env["LC_ALL"] = "es_ES.UTF-8"
	env["LC_MESSAGES"] = "es_ES.UTF-8"
	got, s := listedTools(t, env)
	if got != baseline {
		t.Errorf("tools/list under a Spanish locale differs from the one served without it:\n got: %.300s\nwant: %.300s", got, baseline)
	}
	if asked := readFirstQuestion(t, s.call(t, issueQuestionCall(t, 3)), "title"); asked != englishIssueTitleQuestion {
		t.Errorf("under a Spanish locale the first question =\n%+v\nwant the English\n%+v", asked, englishIssueTitleQuestion)
	}
	if about := localeWarnings(s.stderrText()); len(about) != 0 {
		t.Errorf("a server under a Spanish locale warned about it:\n%s", strings.Join(about, "\n"))
	}
	s.terminate(t, 10*time.Second)
}

// localeNames matches what a warning about the machine's locale would name:
// one of its four variables, or the value they carry here.
var localeNames = regexp.MustCompile(`LANGUAGE|LANG|LC_ALL|LC_MESSAGES|es_ES`)

// localeWarnings returns the warnings in logs that name the locale.
func localeWarnings(logs string) []string {
	var about []string
	for _, line := range warnedLines(logs) {
		if localeNames.MatchString(line) {
			about = append(about, line)
		}
	}
	return about
}

// warnedLines returns the JSON log records written at WARN or above, each as
// the line it was written as.
func warnedLines(logs string) []string {
	var warned []string
	for line := range strings.SplitSeq(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if level, _ := record["level"].(string); level == "WARN" || level == "ERROR" {
			warned = append(warned, line)
		}
	}
	return warned
}

// TestLanguage_ALanguageWithoutACatalog_RefusesToStart pins the refusal: a
// language the server has no catalog for ends the start at ERROR, naming the
// variable and the languages the server accepts, before anything is written
// on stdout, whether it came from the variable or from the flag that writes
// it.
func TestLanguage_ALanguageWithoutACatalog_RefusesToStart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		args  []string
	}{
		{name: "the variable", value: "fr"},
		{name: "the flag", args: []string{"--language=fr"}},
		{name: "the flag over an English variable", value: "en", args: []string{"--language=fr"}},
		{name: "the flag over a Spanish variable", value: "es", args: []string{"--language=fr"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			if tc.value != "" {
				env["GITLAB_MCP_LANGUAGE"] = tc.value
			}
			s := startSessionWithArgs(t, env, tc.args...)
			code, exited := s.waitExit(t, 20*time.Second)
			if !exited {
				t.Fatalf("a stdio server given a language without a catalog started instead of refusing\nstderr: %s", s.stderrText())
			}
			if code == 0 {
				t.Errorf("the refusal exited 0\nstderr: %s", s.stderrText())
			}
			refused := awaitLogRecord(t, s, "server exited with error", 5*time.Second)
			if level, _ := refused["level"].(string); level != "ERROR" {
				t.Errorf("the refusal is at %s, want ERROR", level)
			}
			if said, _ := refused["error"].(string); !strings.Contains(said, languageRefusal) {
				t.Errorf("the refusal says %q, want it to carry %q", said, languageRefusal)
			}
			if logs := s.stderrText(); strings.Contains(logs, stdioStartedLine) {
				t.Errorf("the refused run started the server:\n%s", logs)
			}
			if b, err := s.stdout.ReadByte(); err == nil {
				t.Errorf("the refused run wrote %q on stdout, which only JSON-RPC may use", b)
			}
		})
	}
}

// guidedFlowCall is a 2026-07-28 call of a guided flow through the dynamic
// surface, from a client that declares what capabilities says in the
// request's own _meta, where that revision carries them, and asks for
// progress. responses and state are a later round's answers and the state
// the previous round returned, and are left out of the first round.
func guidedFlowCall(t *testing.T, id int, action, params, capabilities, responses, state string) string {
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
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"gitlab_execute_action",`+
		`"arguments":{"action":%q,"params":%s}%s,`+
		`"_meta":{"progressToken":"language-progress","io.modelcontextprotocol/protocolVersion":%q,`+
		`"io.modelcontextprotocol/clientCapabilities":%s,`+
		`"io.modelcontextprotocol/clientInfo":{"name":"stdio-e2e","version":"1"}}}}`,
		id, action, params, retry, protocolVersion, capabilities)
}

// formElicitation is the capabilities of a client that renders a form.
const formElicitation = `{"elicitation":{"form":{}}}`

// issueQuestionCall is the first call of the guided issue creation from a
// client that renders a form.
func issueQuestionCall(t *testing.T, id int) string {
	t.Helper()
	return guidedFlowCall(t, id, "interactive.issue_create", `{"project_id":"42"}`, formElicitation, "", "")
}

// askedQuestion is the first question of a guided flow as it crossed the
// pipe: the message a person reads, and the key, title and description of the
// one field its form asks for.
type askedQuestion struct {
	message     string
	property    string
	title       string
	description string
}

// englishIssueTitleQuestion is the guided issue creation's first question as
// it was asked before the catalog existed, and as English still asks it.
var englishIssueTitleQuestion = askedQuestion{
	message:     "Enter the issue title",
	property:    "title",
	title:       "title",
	description: "Enter the issue title",
}

// spanishIssueTitleQuestion is the same question under es: the message and
// the field's title in Spanish, and the property key the answer carries back
// as it is in every language.
var spanishIssueTitleQuestion = askedQuestion{
	message:     "Escribe el título del issue",
	property:    "title",
	title:       "Título",
	description: "Escribe el título del issue",
}

// askedForm reads the form of the input_required result answer carries under
// id, failing on any other shape of answer: the message, and the one field
// the form asks for with its property key.
func askedForm(t *testing.T, answer map[string]any, id string) (message, property string, field map[string]any) {
	t.Helper()
	result, _ := answer["result"].(map[string]any)
	if result["resultType"] != "input_required" {
		t.Fatalf("the guided flow answered %v, want resultType input_required asking for %q", answer, id)
	}
	inputs, _ := result["inputRequests"].(map[string]any)
	request, _ := inputs[id].(map[string]any)
	if request["method"] != "elicitation/create" {
		t.Fatalf("inputRequests[%q] = %v, want an elicitation/create request", id, inputs[id])
	}
	params, _ := request["params"].(map[string]any)
	return formOf(t, params)
}

// formOf reads the form the params of an elicitation/create request carry,
// failing unless it asks for one field: the message, and that field with its
// property key.
func formOf(t *testing.T, params map[string]any) (message, property string, field map[string]any) {
	t.Helper()
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

// readFirstQuestion reads the question of the input_required result answer
// carries under id.
func readFirstQuestion(t *testing.T, answer map[string]any, id string) askedQuestion {
	t.Helper()
	return questionIn(askedForm(t, answer, id))
}

// questionIn is the question a form with one field asks: its message, and
// the key, title and description of the field.
func questionIn(message, property string, field map[string]any) askedQuestion {
	asked := askedQuestion{message: message, property: property}
	asked.title, _ = field["title"].(string)
	asked.description, _ = field["description"].(string)
	return asked
}

// requestStateOf returns the state an input_required result hands the next
// round.
func requestStateOf(answer map[string]any) string {
	result, _ := answer["result"].(map[string]any)
	state, _ := result["requestState"].(string)
	return state
}

// progressMessages returns the messages of the progress notifications a
// session read past while waiting for answers, in order.
func progressMessages(s *session) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var messages []string
	for _, note := range s.notifications {
		if note["method"] != "notifications/progress" {
			continue
		}
		params, _ := note["params"].(map[string]any)
		if message, ok := params["message"].(string); ok {
			messages = append(messages, message)
		}
	}
	return messages
}

// TestLanguage_UnsetOrEnglish_AsksTheQuestionsAskedBefore pins what moving the
// person-facing texts into the message catalog leaves a deployment that sets
// no language, or sets English: the guided issue creation's first question
// reaches the client through the real binary as it did before the catalog
// existed, its message, its field's key and the field's title, which is that
// key, and the progress message sent ahead of it is the one sent before. An
// English flag over a Spanish variable is English, since the flag the
// operator typed wins.
func TestLanguage_UnsetOrEnglish_AsksTheQuestionsAskedBefore(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	for _, setting := range []languageSetting{
		{name: "unset"},
		{name: "en", value: "en"},
		{name: "the flag", args: []string{"--language=en"}},
		{name: "the flag over a Spanish variable", value: "es", args: []string{"--language=en"}},
	} {
		t.Run(setting.name, func(t *testing.T) {
			s := startSessionWithArgs(t, setting.envFor(gitlabURL), setting.args...)
			if got := readFirstQuestion(t, s.call(t, issueQuestionCall(t, 1)), "title"); got != englishIssueTitleQuestion {
				t.Errorf("the first question =\n%+v\nwant\n%+v", got, englishIssueTitleQuestion)
			}
			if got := progressMessages(s); len(got) == 0 || got[0] != "Collecting issue details..." {
				t.Errorf("the progress messages sent ahead of it = %q, want them to open with %q", got, "Collecting issue details...")
			}
			s.terminate(t, 10*time.Second)
		})
	}
}

// TestLanguage_Spanish_AsksThePersonInSpanish pins what es changes, through
// the real binary: the guided issue creation's first question is asked in
// Spanish, its field titled in Spanish and its property key the English one
// the answer carries back, and the progress message sent under the call's
// progressToken ahead of it is Spanish too. It holds for the variable, with
// and without a region, and for the flag, alone and over an English
// variable.
func TestLanguage_Spanish_AsksThePersonInSpanish(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	for _, setting := range spanishSettings {
		t.Run(setting.name, func(t *testing.T) {
			s := startSessionWithArgs(t, setting.envFor(gitlabURL), setting.args...)
			if got := readFirstQuestion(t, s.call(t, issueQuestionCall(t, 1)), "title"); got != spanishIssueTitleQuestion {
				t.Errorf("the first question =\n%+v\nwant\n%+v", got, spanishIssueTitleQuestion)
			}
			if got := progressMessages(s); len(got) == 0 || got[0] != "Recopilando los datos del issue..." {
				t.Errorf("the progress messages sent ahead of it = %q, want them to open with %q", got, "Recopilando los datos del issue...")
			}
			s.terminate(t, 10*time.Second)
		})
	}
}

// TestLanguage_Spanish_KeepsTheOptionsAPersonChoosesFrom pins what es must not
// change: the values a client answers with. The guided project creation is
// walked through its rounds, answering the name and declining the
// description and echoing the state each round hands the next, to the
// question that offers the visibility, which is asked in Spanish while its
// options stay private, internal and public, the values GitLab takes and the
// flow compares the answer with.
func TestLanguage_Spanish_KeepsTheOptionsAPersonChoosesFrom(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	for _, setting := range []languageSetting{spanishSettings[0], spanishSettings[2]} {
		t.Run(setting.name, func(t *testing.T) {
			s := startSessionWithArgs(t, setting.envFor(gitlabURL), setting.args...)
			const action, params = "interactive.project_create", "{}"

			first := s.call(t, guidedFlowCall(t, 1, action, params, formElicitation, "", ""))
			if message, property, _ := askedForm(t, first, "name"); message != "Escribe el nombre del proyecto" || property != "name" {
				t.Fatalf("the first round asks %q for %q, want the project name in Spanish", message, property)
			}
			second := s.call(t, guidedFlowCall(t, 2, action, params, formElicitation,
				`{"name":{"action":"accept","content":{"name":"demo"}}}`, requestStateOf(first)))
			if message, property, _ := askedForm(t, second, "description"); property != "description" {
				t.Fatalf("the second round asks %q for %q, want the description", message, property)
			}
			third := s.call(t, guidedFlowCall(t, 3, action, params, formElicitation,
				`{"description":{"action":"decline"}}`, requestStateOf(second)))

			message, property, field := askedForm(t, third, "visibility")
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
			s.terminate(t, 10*time.Second)
		})
	}
}

// earlierRevision is the latest revision before 2026-07-28, which every
// client that predates it speaks, and on which a question reaches the client
// as an elicitation/create request the server sends itself.
const earlierRevision = "2025-11-25"

// TestLanguage_AnEarlierRevision_IsAskedThroughElicitationCreate pins the path
// the cases above do not take: on a session initialized at 2025-11-25 the
// guided issue creation, here on the meta surface, sends its first question
// as an elicitation/create request of the server's own and waits for the
// answer, rather than returning it inside the result. Under es that request
// asks in Spanish, its property key title titled in Spanish, and the progress
// sent under the call's progressToken ahead of it is Spanish too; under en
// both are what they were before the catalog existed. The client cancels and
// the call is answered.
func TestLanguage_AnEarlierRevision_IsAskedThroughElicitationCreate(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	for _, tc := range []struct {
		setting  languageSetting
		want     askedQuestion
		progress string
	}{
		{setting: languageSetting{name: "en", value: "en"}, want: englishIssueTitleQuestion, progress: "Collecting issue details..."},
		{setting: spanishSettings[0], want: spanishIssueTitleQuestion, progress: "Recopilando los datos del issue..."},
		{setting: spanishSettings[2], want: spanishIssueTitleQuestion, progress: "Recopilando los datos del issue..."},
	} {
		t.Run(tc.setting.name, func(t *testing.T) {
			env := tc.setting.envFor(gitlabURL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "meta"
			s := startSessionWithArgs(t, env, tc.setting.args...)
			initialized := s.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+earlierRevision+`",`+
				`"capabilities":{"elicitation":{"form":{}}},"clientInfo":{"name":"stdio-e2e","version":"1"}}}`)
			result, _ := initialized["result"].(map[string]any)
			if negotiated := result["protocolVersion"]; negotiated != earlierRevision {
				t.Fatalf("initialize asking for %s negotiated %v", earlierRevision, negotiated)
			}
			s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

			const callID = 2
			s.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_interactive_issue_create",`+
				`"arguments":{"project_id":"42"},"_meta":{"progressToken":"language-progress"}}}`)
			request := awaitServerRequest(t, s, "elicitation/create", callID)
			params, _ := request["params"].(map[string]any)
			if got := questionIn(formOf(t, params)); got != tc.want {
				t.Errorf("the elicitation/create request asks\n%+v\nwant\n%+v", got, tc.want)
			}
			if got := progressMessages(s); len(got) == 0 || got[0] != tc.progress {
				t.Errorf("the progress messages sent ahead of it = %q, want them to open with %q", got, tc.progress)
			}

			cancelled, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"action": "cancel"}})
			if err != nil {
				t.Fatalf("encoding the answer to the elicitation: %v", err)
			}
			s.send(t, string(cancelled))
			if answer := awaitResponse(t, s, callID); answer["result"] == nil {
				t.Errorf("the cancelled flow was answered %v, want a result", answer)
			}
			s.terminate(t, 10*time.Second)
		})
	}
}

// awaitServerRequest reads the session's output until the server sends a
// request for method, and returns it. Notifications read past are kept in the
// session's notifications; an answer to callID first means the call ended
// without asking, and any other message is one this exchange does not
// expect, so both fail.
func awaitServerRequest(t *testing.T, s *session, method string, callID int) map[string]any {
	t.Helper()
	for {
		got := s.readMessage(t, callTimeout)
		switch name, isCall := got["method"].(string); {
		case isCall && got["id"] == nil:
			s.mu.Lock()
			s.notifications = append(s.notifications, got)
			s.mu.Unlock()
		case isCall && name == method:
			return got
		case sameID(got["id"], callID):
			t.Fatalf("the call was answered before the server sent %s: %v\nstderr: %s", method, got, s.stderrText())
		default:
			t.Fatalf("waiting for %s, read %v\nstderr: %s", method, got, s.stderrText())
		}
	}
}

// awaitResponse reads the session's output until the response to callID,
// keeping the notifications read past and failing on anything else.
func awaitResponse(t *testing.T, s *session, callID int) map[string]any {
	t.Helper()
	for {
		got := s.readMessage(t, callTimeout)
		_, isCall := got["method"]
		switch {
		case isCall && got["id"] == nil:
			s.mu.Lock()
			s.notifications = append(s.notifications, got)
			s.mu.Unlock()
		case !isCall && sameID(got["id"], callID):
			return got
		default:
			t.Fatalf("waiting for the response to id %d, read %v\nstderr: %s", callID, got, s.stderrText())
		}
	}
}

// TestLanguage_Spanish_TheModelIsRefusedInEnglish pins the refusal a client
// without elicitation is answered with, which the model reads and not a
// person: the guided flow cannot ask, and says so in English under es, word
// for word what it says under en.
func TestLanguage_Spanish_TheModelIsRefusedInEnglish(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	english := refusalUnder(t, gitlabURL, languageSetting{name: "en", value: "en"})
	if !strings.Contains(english, "Your MCP client does not support elicitation.") {
		t.Fatalf("the refusal under en = %s, want the English one", english)
	}
	for _, setting := range []languageSetting{spanishSettings[0], spanishSettings[2]} {
		t.Run(setting.name, func(t *testing.T) {
			if got := refusalUnder(t, gitlabURL, setting); got != english {
				t.Errorf("the refusal under es =\n%s\nwant what en says\n%s", got, english)
			}
		})
	}
}

// refusalUnder starts a session under setting and returns, as JSON, the
// content of the result the guided issue creation is answered with when the
// client declares no elicitation, failing unless that result is an error.
func refusalUnder(t *testing.T, gitlabURL string, setting languageSetting) string {
	t.Helper()
	s := startSessionWithArgs(t, setting.envFor(gitlabURL), setting.args...)
	defer s.terminate(t, 10*time.Second)
	answer := s.call(t, guidedFlowCall(t, 1, "interactive.issue_create", `{"project_id":"42"}`, `{}`, "", ""))
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

// TestLanguage_TheFlagBeatsTheVariable pins the precedence the flag shares
// with every setting: a --language the operator typed wins over an exported
// GITLAB_MCP_LANGUAGE, so a value the server would refuse in the environment
// is overridden rather than stopping the start.
func TestLanguage_TheFlagBeatsTheVariable(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["GITLAB_MCP_LANGUAGE"] = "fr"
	s := startSessionWithArgs(t, env, "--language=en")
	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("a stdio server given --language=en over GITLAB_MCP_LANGUAGE=fr did not serve: %v\nstderr: %s", got, s.stderrText())
	}
	s.terminate(t, 10*time.Second)
}
