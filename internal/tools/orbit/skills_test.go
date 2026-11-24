// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// readFixture returns a body under testdata: an answer GitLab.com gave on
// 2026-10-29, trimmed to what a test reads and with no account's name or id
// in it. Where GitLab publishes text of its own, the text is this project's,
// since the Knowledge Graph is not under a license this repository could
// publish it under: the description of a command, of its parameters, of a
// skill, of a template and of the response format schema, that schema's title,
// the compatibility of a skill, the content of a skill's file, and the
// statement of a template.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return strings.TrimSuffix(string(body), "\n")
}

// TestSkillList_GitLabsAnswer_ListsEverySkill holds [SkillList] to GET
// /orbit/skills and to the three skills GitLab.com listed on 2026-10-29
// (descriptions and compatibility this project's), with the version of the
// service that serves them.
func TestSkillList_GitLabsAnswer_ListsEverySkill(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/skills")
		testutil.RespondJSON(w, http.StatusOK, readFixture(t, "skills.json"))
	}))
	out, err := SkillList(context.Background(), client, SkillListInput{})
	if err != nil {
		t.Fatalf("SkillList() error: %v", err)
	}
	if out.ServerVersion != "0.163.0" || len(out.Skills) != 3 {
		t.Fatalf("SkillList() = %+v, want three skills served by 0.163.0", out)
	}
	if got := out.Skills[1]; got.Name != "orbit-gql-migration" || got.Version != "0.1.0" || got.Compatibility == "" || got.Description == "" {
		t.Errorf("second skill = %+v, want orbit-gql-migration 0.1.0 with its compatibility and description", got)
	}
}

// TestSkillList_NoSkills_IsAnEmptyList verifies that an answer listing no
// skill is published as an empty list rather than as null.
func TestSkillList_NoSkills_IsAnEmptyList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"server_version":"0.163.0","skills":null}`)
	}))
	out, err := SkillList(context.Background(), client, SkillListInput{})
	if err != nil {
		t.Fatalf("SkillList() error: %v", err)
	}
	if out.Skills == nil || len(out.Skills) != 0 {
		t.Errorf("Skills = %#v, want an empty list", out.Skills)
	}
}

// TestSkillGet_GitLabsAnswer_ListsTheFilesWithoutTheirContent holds
// [SkillGet] to GET /orbit/skills/:name and to GitLab.com's answer for the
// orbit skill on 2026-10-29 (two files, their digests GitLab's, their content
// and the skill's compatibility this project's): each file published with its
// path, digest and the length of the content it came with, the content itself
// left to orbit.skill_file.
func TestSkillGet_GitLabsAnswer_ListsTheFilesWithoutTheirContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/skills/orbit")
		testutil.RespondJSON(w, http.StatusOK, readFixture(t, "skill_orbit.json"))
	}))
	out, err := SkillGet(context.Background(), client, SkillGetInput{Name: " orbit "})
	if err != nil {
		t.Fatalf("SkillGet() error: %v", err)
	}
	want := []SkillFileEntry{
		{Path: "SKILL.md", SHA256: "f3d87659f853749f62448f76704d80925fde13d58e65adbac734575631a9be41", Size: 41},
		{Path: "references/gql.md", SHA256: "fa0c9266668b202bd03db4cf6aa2c6ae10ffb66ce42b03e283b51b57a239b46c", Size: 26},
	}
	if out.Name != "orbit" || out.Version != "0.35.4+gql" || out.ServerVersion != "0.163.0" || !reflect.DeepEqual(out.Files, want) {
		t.Errorf("SkillGet() = %+v, want orbit 0.35.4+gql with files %+v", out, want)
	}
}

// TestSkillFile_GitLabsAnswer_IsTheFileAsText holds [SkillFile] to the route
// it asks, the path's segments each escaped and its slash kept, and to a
// file GitLab answers as text rather than JSON, with its media type. The path
// holds a percent sign and a "%20" of its own, which are another path or none
// unless each segment is escaped first, and the spaces around the path the
// caller gave are no part of it.
func TestSkillFile_GitLabsAnswer_IsTheFileAsText(t *testing.T) {
	const path = "references/100% a%20b.md"
	content := readFixture(t, "skill_file_skill_md.md")
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		if r.URL.EscapedPath() != "/api/v4/orbit/skills/orbit/references/100%25%20a%2520b.md" {
			t.Errorf("request path = %s, want each segment escaped", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	out, err := SkillFile(context.Background(), client, SkillFileInput{Name: "orbit", Path: " " + path + "\n"})
	if err != nil {
		t.Fatalf("SkillFile() error: %v", err)
	}
	want := SkillFileOutput{Name: "orbit", Path: path, ContentType: "text/markdown; charset=utf-8", Content: content}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("SkillFile() = %+v, want %+v", out, want)
	}
}

// TestSkillFileGuidance_ThePathIsRelativeToTheSkill holds what is served about
// the path of orbit.skill_file to the skill: the catalog gives a path with no
// guidance of its own the one of a path in a repository, which a skill is not.
func TestSkillFileGuidance_ThePathIsRelativeToTheSkill(t *testing.T) {
	guidance := servedGuidance(t, "skill_file", "path")
	if guidance.SemanticRole != "skill_file_path" || guidance.ExampleBinding != "SKILL.md" {
		t.Errorf("path guidance = %+v, want the role of a skill's file and SKILL.md as its example", guidance)
	}
	if lacks := missingFrom(guidance.ValueSource, "relative to the skill root", "orbit.skill_get"); len(lacks) > 0 {
		t.Errorf("path guidance lacks %q: %s", lacks, guidance.ValueSource)
	}
	if strings.Contains(guidance.ValueSource, "repository") {
		t.Errorf("path guidance names a repository, and a skill's file is in none: %s", guidance.ValueSource)
	}
}

// TestSkillReads_InvalidInput_IsRefusedBeforeSending covers what the skill
// reads refuse before a request: a missing name, a name GitLab's route would
// refuse with 400 "name is invalid" (49f in the capture), a missing path, a
// path from a root, a path with an empty segment, which the pattern of GitLab's
// route refuses, and a path that leaves the skill.
func TestSkillReads_InvalidInput_IsRefusedBeforeSending(t *testing.T) {
	cases := []struct {
		name string
		call func(context.Context, *testClient) error
		want string
	}{
		{name: "skill_get, no name", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillGet(ctx, c, SkillGetInput{})
			return err
		}, want: "name"},
		{name: "skill_get, a name GitLab refuses", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillGet(ctx, c, SkillGetInput{Name: "Bad_Name"})
			return err
		}, want: "lowercase letters, digits and dashes"},
		{name: "skill_get, a name with a slash", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillGet(ctx, c, SkillGetInput{Name: "orbit/files"})
			return err
		}, want: "lowercase letters, digits and dashes"},
		{name: "skill_file, no path", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "orbit"})
			return err
		}, want: "path"},
		{name: "skill_file, a path from a root", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "orbit", Path: "/etc/passwd"})
			return err
		}, want: "without a leading"},
		{name: "skill_file, an empty segment", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "orbit", Path: "references//gql.md"})
			return err
		}, want: "empty segment"},
		{name: "skill_file, a path that ends in a slash", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "orbit", Path: "references/"})
			return err
		}, want: "empty segment"},
		{name: "skill_file, a path that leaves the skill", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "orbit", Path: "references/../../query"})
			return err
		}, want: `has the segment ".."`},
		{name: "skill_file, a name GitLab refuses", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "-orbit", Path: "SKILL.md"})
			return err
		}, want: "lowercase letters, digits and dashes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("request sent for an input refused before one: %s %s", r.Method, r.URL.Path)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{}`)
			}))
			err := tc.call(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to say %q", err, tc.want)
			}
		})
	}
}

// TestSkillReads_GitLabsRefusals_SayWhichOfTheTwoTheyAre verifies that each
// skill read passes GitLab's 404 on with the hint it calls for: "Skills are
// not available", which GitLab.com answered after the access gate turned on
// (63 in the capture), with what Orbit needs, and an unknown skill (49e) and
// a missing file (49i), with the actions that list the skills and their files
// and without a word about Orbit being served, and so is a 404 whose body is
// not GitLab's JSON.
func TestSkillReads_GitLabsRefusals_SayWhichOfTheTwoTheyAre(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		call   func(context.Context, *testClient) error
		want   []string
		not    string
	}{
		{name: "skill_list, skills not available", answer: "skills_unavailable.json", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillList(ctx, c, SkillListInput{})
			return err
		}, want: []string{orbitUnavailableHint}, not: skillUnknownHint},
		{name: "skill_get, skills not available", answer: "skills_unavailable.json", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillGet(ctx, c, SkillGetInput{Name: "orbit"})
			return err
		}, want: []string{orbitUnavailableHint}, not: skillUnknownHint},
		{name: "skill_get, an unknown skill", answer: "skill_unknown.json", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillGet(ctx, c, SkillGetInput{Name: "no-such-skill"})
			return err
		}, want: []string{skillUnknownHint}, not: orbitUnavailableHint},
		{name: "skill_get, a 404 that is not JSON", answer: "response_format_llm.txt", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillGet(ctx, c, SkillGetInput{Name: "orbit"})
			return err
		}, want: []string{skillUnknownHint}, not: orbitUnavailableHint},
		{name: "skill_file, a missing file", answer: "skill_file_missing.json", call: func(ctx context.Context, c *testClient) error {
			_, err := SkillFile(ctx, c, SkillFileInput{Name: "orbit", Path: "missing.md"})
			return err
		}, want: []string{skillUnknownHint}, not: orbitUnavailableHint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := readFixture(t, tc.answer)
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, body)
			}))
			err := tc.call(context.Background(), client)
			if err == nil || !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				t.Fatalf("error = %v, want GitLab's 404", err)
			}
			if lacks := missingFrom(err.Error(), tc.want...); len(lacks) > 0 {
				t.Errorf("error = %v, lacks %q", err, lacks)
			}
			if strings.Contains(err.Error(), tc.not) {
				t.Errorf("error = %v, says %q of the other 404", err, tc.not)
			}
		})
	}
}

// TestSkillReads_ARefusalThatIsNotA404_IsClassifiedAsEveryOrbitFailure
// verifies that a 403 on a skill route keeps the Orbit availability hint and
// is not read as an unknown skill, whatever its message says.
func TestSkillReads_ARefusalThatIsNotA404_IsClassifiedAsEveryOrbitFailure(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, readFixture(t, "skills_unavailable.json"))
	}))
	_, err := SkillGet(context.Background(), client, SkillGetInput{Name: "orbit"})
	if err == nil || !strings.Contains(err.Error(), orbitUnavailableHint) || strings.Contains(err.Error(), skillUnknownHint) {
		t.Fatalf("SkillGet() error = %v, want the 403 with the Orbit availability hint alone", err)
	}
	if errors.Is(err, errSkillsUnavailable) {
		t.Errorf("SkillGet() error = %v wraps errSkillsUnavailable, want a 403 left alone", err)
	}
}

// TestSkillRoutes_NotFound_NameWhatTheCallerAskedFor holds what the routes of
// the skill reads serve for GitLab's 404s. An unknown skill or file is named
// as the caller sent it, with the actions that list the skills and their files
// and nothing about Orbit being served, since a mistyped name is not Orbit
// turned off. "Skills are not available" is the other answer and keeps the two
// checks that explain it.
func TestSkillRoutes_NotFound_NameWhatTheCallerAskedFor(t *testing.T) {
	const availability = "Verify GitLab Orbit is enabled"
	cases := []struct {
		name   string
		tool   string
		args   map[string]any
		answer string
		want   []string
		not    string
	}{
		{
			name: "an unknown skill", tool: "gitlab_orbit_skill_get", args: map[string]any{"name": "no-such-skill"}, answer: "skill_unknown.json",
			want: []string{"GitLab Orbit Skill Not Found", "**no-such-skill**", toolutil.HintAction("orbit.skill_list", "list the skills GitLab has deployed by name")}, not: availability,
		},
		{
			name: "a missing file", tool: "gitlab_orbit_skill_file", args: map[string]any{"name": "orbit", "path": "missing.md"}, answer: "skill_file_missing.json",
			want: []string{
				"GitLab Orbit Skill File Not Found", "**missing.md of skill orbit**",
				toolutil.HintAction("orbit.skill_get", "list the files of the skill by path"),
				toolutil.HintAction("orbit.skill_list", "list the skills GitLab has deployed by name"),
			}, not: availability,
		},
		{
			name: "no skills for this user, asked for a skill", tool: "gitlab_orbit_skill_get", args: map[string]any{"name": "orbit"}, answer: "skills_unavailable.json",
			want: []string{"GitLab Orbit Skills Not Found", availability}, not: "orbit.skill_list",
		},
		{
			name: "no skills for this user, asked for a file", tool: "gitlab_orbit_skill_file", args: map[string]any{"name": "orbit", "path": "SKILL.md"}, answer: "skills_unavailable.json",
			want: []string{"GitLab Orbit Skills Not Found", availability}, not: "orbit.skill_get",
		},
		{
			name: "no skills for this user, asked for the list", tool: "gitlab_orbit_skill_list", args: map[string]any{}, answer: "skills_unavailable.json",
			want: []string{"GitLab Orbit Skills Not Found", availability}, not: "orbit.skill_get",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := readFixture(t, tc.answer)
			routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, body)
			}))
			result, err := routes[tc.tool].Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("%s error = %v, want the not-found result", tc.tool, err)
			}
			card := toolutil.MarkdownForResult(result)
			if card == nil || !card.IsError || len(card.Content) == 0 {
				t.Fatalf("%s = %+v, want an informational error result", tc.tool, card)
			}
			text, ok := card.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content type = %T, want *mcp.TextContent", card.Content[0])
			}
			if lacks := missingFrom(text.Text, tc.want...); len(lacks) > 0 {
				t.Errorf("card lacks %q:\n%s", lacks, text.Text)
			}
			if strings.Contains(text.Text, tc.not) {
				t.Errorf("card says %q, which is the other 404's:\n%s", tc.not, text.Text)
			}
		})
	}
}

// TestSkillRoutes_AServerFault_StaysAnError verifies that the decoration of
// the skill routes answers the 404s alone: a 500 reaches the caller as the
// error it is.
func TestSkillRoutes_AServerFault_StaysAnError(t *testing.T) {
	routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, readFixture(t, "skills_unavailable.json"))
	}))
	result, err := routes["gitlab_orbit_skill_get"].Handler(t.Context(), map[string]any{"name": "orbit"})
	if !toolutil.IsHTTPStatus(err, http.StatusInternalServerError) {
		t.Fatalf("skill_get = %+v, %v, want GitLab's 500", result, err)
	}
}

// TestRestRequest_ABodyThatDoesNotEncode_IsTheErrorItIs verifies that a
// request whose body cannot be encoded fails before anything is sent.
func TestRestRequest_ABodyThatDoesNotEncode_IsTheErrorItIs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request sent for a body that does not encode: %s %s", r.Method, r.URL.Path)
		testutil.RespondJSON(w, http.StatusInternalServerError, `{}`)
	}))
	var answer map[string]any
	err := restRequest(context.Background(), client, http.MethodPost, skillsPath, map[string]any{"value": func() {}}, &answer)
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("restRequest() error = %v, want the encoding failure", err)
	}
}
