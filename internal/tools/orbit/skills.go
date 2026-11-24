// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// skillsPath is where GitLab serves the deployed Orbit skills, below
// /api/v4. client-go has no method for it or for the two routes under it.
const skillsPath = "orbit/skills"

// The operations a skill read is reported under.
const (
	orbitSkillListOp = "orbit_skill_list"
	orbitSkillGetOp  = "orbit_skill_get"
	orbitSkillFileOp = "orbit_skill_file"
)

// skillNamePattern is the name GitLab's route takes for a skill
// (ee/lib/api/orbit/data.rb on master, 2026-10-29), checked here so a name
// GitLab would refuse with 400 "name is invalid" is refused before the
// request, naming the parameter, and so the name is always one path segment.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// SkillListInput takes nothing: GitLab lists every skill it has deployed.
type SkillListInput struct{}

// SkillGetInput names one deployed skill.
type SkillGetInput struct {
	// Name is the skill's name as orbit.skill_list lists it.
	Name string `json:"name" jsonschema:"Skill name as orbit.skill_list lists it: lowercase letters, digits and dashes, starting with a letter or digit (orbit, orbit-start),required"`
}

// SkillFileInput names one file of one deployed skill.
type SkillFileInput struct {
	// Name is the skill's name as orbit.skill_list lists it.
	Name string `json:"name" jsonschema:"Skill name as orbit.skill_list lists it,required"`
	// Path is the file's path relative to the skill's root, as orbit.skill_get
	// lists it.
	Path string `json:"path" jsonschema:"File path relative to the skill root, as orbit.skill_get lists it (SKILL.md, references/gql.md),required"`
}

// skillFileInput is what orbit.skill_file says about its parameters: that its
// path is one of the skill's files. The catalog fills in for a path with no
// guidance the one of a path in a repository, which a skill is not.
func skillFileInput() orbitInput {
	return orbitInput{guidance: map[string]toolutil.ParameterGuidance{
		"path": {
			SemanticRole:   "skill_file_path",
			ValueSource:    "A file's path relative to the skill root, as orbit.skill_get lists it for the skill named.",
			ExampleBinding: "SKILL.md",
		},
	}}
}

// Skill is one deployed skill as GitLab lists it.
type Skill struct {
	// Name is the skill's name, what orbit.skill_get and orbit.skill_file take.
	Name string `json:"name"`
	// Version is the skill's version.
	Version string `json:"version,omitempty"`
	// Compatibility says what the skill needs to run.
	Compatibility string `json:"compatibility,omitempty"`
	// Description says what the skill is for.
	Description string `json:"description,omitempty"`
}

// SkillListOutput is the list of deployed skills.
type SkillListOutput struct {
	toolutil.HintableOutput
	// ServerVersion is the Orbit service version that serves the skills.
	ServerVersion string `json:"server_version,omitempty"`
	// Skills are the deployed skills.
	Skills []Skill `json:"skills"`
}

// SkillFileEntry is one file of a skill, without its content.
type SkillFileEntry struct {
	// Path is the file's path relative to the skill's root, what
	// orbit.skill_file takes.
	Path string `json:"path"`
	// SHA256 is the digest of the file's content.
	SHA256 string `json:"sha256,omitempty"`
	// Size is the length in bytes of the content GitLab answered.
	Size int `json:"size"`
}

// SkillGetOutput is one skill and the files it holds. The content of each
// file is left out: GitLab answers every file inline, 64 KB across the three
// skills of 2026-10-29, so the output lists them for orbit.skill_file to read
// one at a time.
type SkillGetOutput struct {
	toolutil.HintableOutput
	// Name is the skill's name.
	Name string `json:"name"`
	// Version is the skill's version.
	Version string `json:"version,omitempty"`
	// Compatibility says what the skill needs to run.
	Compatibility string `json:"compatibility,omitempty"`
	// ServerVersion is the Orbit service version that serves the skill.
	ServerVersion string `json:"server_version,omitempty"`
	// Files are the skill's files, without their content.
	Files []SkillFileEntry `json:"files"`
}

// SkillFileOutput is one file of a skill, as GitLab serves it.
type SkillFileOutput struct {
	toolutil.HintableOutput
	// Name is the skill the file belongs to.
	Name string `json:"name"`
	// Path is the file's path relative to the skill's root.
	Path string `json:"path"`
	// ContentType is the media type GitLab answered the file with.
	ContentType string `json:"content_type,omitempty"`
	// Content is the file as GitLab served it.
	Content string `json:"content"`
}

// skillListAnswer is the answer of GET /orbit/skills.
type skillListAnswer struct {
	ServerVersion string  `json:"server_version"`
	Skills        []Skill `json:"skills"`
}

// skillAnswer is the answer of GET /orbit/skills/:name.
type skillAnswer struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Compatibility string `json:"compatibility"`
	ServerVersion string `json:"server_version"`
	Files         []struct {
		Path    string `json:"path"`
		SHA256  string `json:"sha256"`
		Content string `json:"content"`
	} `json:"files"`
}

// SkillList retrieves the Orbit skills GitLab.com has deployed.
//
// Source: GET /api/v4/orbit/skills, which client-go has no method for.
// GitLab skips the Orbit license check on the skill routes, yet answers them
// only to a user with an Orbit-enabled namespace, with 404 "Skills are not
// available" otherwise (measured 2026-10-29 at 09:22 UTC), which the action's
// route answers as a not-found result.
func SkillList(ctx context.Context, client *gitlabclient.Client, _ SkillListInput) (SkillListOutput, error) {
	if err := ctx.Err(); err != nil {
		return SkillListOutput{}, err
	}
	var answer skillListAnswer
	if err := getJSON(ctx, client, skillsPath, &answer); err != nil {
		return SkillListOutput{}, wrapOrbitErr(orbitSkillListOp, err)
	}
	if answer.Skills == nil {
		answer.Skills = []Skill{}
	}
	return SkillListOutput{ServerVersion: answer.ServerVersion, Skills: answer.Skills}, nil
}

// SkillGet retrieves one deployed Orbit skill and the files it holds.
//
// Source: GET /api/v4/orbit/skills/:name, which client-go has no method for.
// GitLab answers every file with its content; the output keeps the path, the
// digest and the size of each and leaves the content to [SkillFile].
func SkillGet(ctx context.Context, client *gitlabclient.Client, input SkillGetInput) (SkillGetOutput, error) {
	if err := ctx.Err(); err != nil {
		return SkillGetOutput{}, err
	}
	name, err := skillName(orbitSkillGetOp, input.Name)
	if err != nil {
		return SkillGetOutput{}, err
	}
	captureCtx, capture := gitlabclient.WithResponseCapture(ctx)
	var answer skillAnswer
	if err = getJSON(captureCtx, client, skillsPath+"/"+toolutil.EscapePathSegment(name), &answer); err != nil {
		return SkillGetOutput{}, wrapSkillErr(orbitSkillGetOp, err, capture)
	}
	out := SkillGetOutput{
		Name: answer.Name, Version: answer.Version, Compatibility: answer.Compatibility,
		ServerVersion: answer.ServerVersion, Files: make([]SkillFileEntry, 0, len(answer.Files)),
	}
	for _, file := range answer.Files {
		out.Files = append(out.Files, SkillFileEntry{Path: file.Path, SHA256: file.SHA256, Size: len(file.Content)})
	}
	return out, nil
}

// SkillFile retrieves one file of one deployed Orbit skill, as text.
//
// Source: GET /api/v4/orbit/skills/:name/*path, which client-go has no method
// for. GitLab answers the file itself rather than JSON, text/markdown for a
// Markdown file, and serves only the paths the skill lists.
func SkillFile(ctx context.Context, client *gitlabclient.Client, input SkillFileInput) (SkillFileOutput, error) {
	if err := ctx.Err(); err != nil {
		return SkillFileOutput{}, err
	}
	name, err := skillName(orbitSkillFileOp, input.Name)
	if err != nil {
		return SkillFileOutput{}, err
	}
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return SkillFileOutput{}, toolutil.ErrRequiredString(orbitSkillFileOp, "path")
	}
	if strings.HasPrefix(path, "/") {
		return SkillFileOutput{}, errSkillFileAbsolute
	}
	if slices.Contains(strings.Split(path, "/"), "") {
		return SkillFileOutput{}, errSkillFileEmptySegment
	}
	if traversal := toolutil.RejectTraversalArg(orbitSkillFileOp, "path", path); traversal != nil {
		return SkillFileOutput{}, traversal
	}
	captureCtx, capture := gitlabclient.WithResponseCapture(ctx)
	var body bytes.Buffer
	resp, err := restResponse(captureCtx, client, http.MethodGet, skillsPath+"/"+toolutil.EscapePathSegment(name)+"/"+toolutil.EscapePathSegments(path), nil, &body)
	if err != nil {
		return SkillFileOutput{}, wrapSkillErr(orbitSkillFileOp, err, capture)
	}
	return SkillFileOutput{Name: name, Path: path, ContentType: resp.Header.Get("Content-Type"), Content: body.String()}, nil
}

// skillsUnavailable is the message GitLab answers every skill route with, 404,
// for a user it serves no skills (measured 2026-10-29), which is what tells
// that answer from a skill or a file it does not hold.
const skillsUnavailable = "Skills are not available"

// errSkillsUnavailable marks a failed skill read GitLab answered with
// [skillsUnavailable].
var errSkillsUnavailable = errors.New("GitLab serves this user no Orbit skills")

// skillUnknownHint follows a skill or a skill file GitLab does not hold.
const skillUnknownHint = "orbit.skill_list lists the skills GitLab has deployed by name, and orbit.skill_get the files of one by path"

// wrapSkillErr classifies a failed read of a skill. A 404 is one of two
// answers, which only its body tells apart, and client-go answers every 404
// with a sentinel that carries none, so the body is read off the captured
// response: GitLab's message that it serves the user no skills is the failure
// every Orbit read can meet, marked with [errSkillsUnavailable], and anything
// else is an unknown skill or file, followed by [skillUnknownHint]. A failure
// that is no 404 is classified the way every Orbit failure is, by
// [wrapOrbitErr].
func wrapSkillErr(op string, err error, capture *gitlabclient.ResponseCapture) error {
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return wrapOrbitErr(op, err)
	}
	if servesNoSkills(capture) {
		return fmt.Errorf("%w: %w", wrapOrbitErr(op, err), errSkillsUnavailable)
	}
	return toolutil.WrapErrWithHint(op, err, skillUnknownHint)
}

// servesNoSkills reports whether a captured 404 is GitLab's answer to a user
// it serves no skills. A body that is not JSON, or that says anything else, is
// not.
func servesNoSkills(capture *gitlabclient.ResponseCapture) bool {
	var body map[string]any
	return capture.Decode(&body) == nil && body["message"] == skillsUnavailable
}

// answerSkillsUnavailable decorates the handler of a route that reads one
// skill or one of its files: GitLab's 404 for a user it serves no skills is
// answered by the result that names what Orbit needs, and every other 404 is
// left to the route's own, which names the skill and the file asked for.
func answerSkillsUnavailable(next toolutil.ActionFunc) toolutil.ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		result, err := next(ctx, params)
		if errors.Is(err, errSkillsUnavailable) {
			return orbitNotFoundOutput{Resource: "GitLab Orbit Skills", Identifier: "deployed skills"}, nil
		}
		return result, err
	}
}

// skillNotFound is what orbit.skill_get answers for a skill GitLab does not
// hold: the name the caller sent and the action that lists the names.
func skillNotFound(params map[string]any) any {
	return orbitNotFoundOutput{
		Resource:   "GitLab Orbit Skill",
		Identifier: toolutil.ParamText(params["name"]),
		Hints:      []string{toolutil.HintAction(orbitActionSkillList, "list the skills GitLab has deployed by name")},
	}
}

// skillFileNotFound is what orbit.skill_file answers for a file or a skill
// GitLab does not hold: the path and the name the caller sent, and the
// actions that list a skill's files and the skills' names.
func skillFileNotFound(params map[string]any) any {
	return orbitNotFoundOutput{
		Resource:   "GitLab Orbit Skill File",
		Identifier: fmt.Sprintf("%s of skill %s", toolutil.ParamText(params["path"]), toolutil.ParamText(params["name"])),
		Hints: []string{
			toolutil.HintAction(orbitActionSkillGet, "list the files of the skill by path"),
			toolutil.HintAction(orbitActionSkillList, "list the skills GitLab has deployed by name"),
		},
	}
}

// errSkillFileAbsolute is what [SkillFile] answers for a path that starts at
// a root, which no skill lists: a skill's paths are relative to its root.
var errSkillFileAbsolute = errors.New(`orbit_skill_file: path is relative to the skill root, as orbit.skill_get lists it; pass it without a leading "/"`)

// errSkillFileEmptySegment is what [SkillFile] answers for a path with an
// empty segment, which a doubled or a trailing slash makes and the pattern of
// GitLab's route refuses: a 400 of a skill route would otherwise carry the
// hint of a refused query, whose parameters no skill read takes.
var errSkillFileEmptySegment = errors.New(`orbit_skill_file: path has an empty segment; pass it as orbit.skill_get lists it, with no doubled or trailing "/"`)

// skillName checks a skill name against the pattern GitLab's route takes and
// returns it trimmed.
func skillName(op, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", toolutil.ErrRequiredString(op, "name")
	}
	if !skillNamePattern.MatchString(name) {
		return "", errors.New(op + ": name is lowercase letters, digits and dashes, starting with a letter or digit, as orbit.skill_list lists it")
	}
	return name, nil
}

// getJSON sends one GET to an Orbit route client-go has no method for and
// decodes its JSON answer into out.
func getJSON(ctx context.Context, client *gitlabclient.Client, path string, out any) error {
	return restRequest(ctx, client, http.MethodGet, path, nil, out)
}

// restRequest sends one request to an Orbit REST route client-go has no
// method for, with body as its JSON body when there is one, and decodes the
// answer into out: JSON into a value, or the bytes as they came into an
// io.Writer.
func restRequest(ctx context.Context, client *gitlabclient.Client, method, path string, body, out any) error {
	_, err := restResponse(ctx, client, method, path, body, out)
	return err
}

// restResponse is [restRequest] for a caller that reads the answer's headers
// as well.
func restResponse(ctx context.Context, client *gitlabclient.Client, method, path string, body, out any) (*gl.Response, error) {
	req, err := client.GL().NewRequest(method, path, body, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	return client.GL().Do(req, out)
}
