// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// setupConfig is one configured setup run.
type setupConfig struct {
	// token is the GitLab.com credential every request and git call carries.
	token string
	// gitlabURL is the instance, with no trailing slash.
	gitlabURL string
	// namespace is the full path of the group the fixtures live under.
	namespace string
	// visibility is what every fixture project is created with or set to.
	visibility string
	// mirror adds the gitlab-org/cli mirror.
	mirror bool
	// fixtures is the directory holding kg-fixtures and security-fixtures.
	fixtures string
	// api makes the REST requests.
	api gitlabAPI
	// git runs git, for the pushes and for the fixture files HEAD holds.
	git gitFunc
	// mergePolls and mergeInterval bound the wait for GitLab to finish
	// checking the fixture merge request: so many reads, so far apart.
	mergePolls    int
	mergeInterval time.Duration
	// sleep waits between two reads; a test makes it return at once.
	sleep func(context.Context, time.Duration) error
}

// The fixture merge request: its branch, its title and the change it makes.
const (
	mrBranch    = "feature/restock-helper"
	mrBranchEnc = "feature%2Frestock-helper"
	mrTitle     = "Add seed_stock helper for fixture test runs"
	// mirrorUpstream is what -mirror-cli mirrors.
	mirrorUpstream = "https://gitlab.com/gitlab-org/cli.git"
	// botEmail and botName sign the commits a run pushes into a fixture
	// project: the fixture's own history, never this repository's.
	botEmail = "orbit-fixtures@local"
	botName  = "Orbit Fixtures Bot"
	// maxListPages is how many pages of a hundred entries a paged read, a
	// project's tree or the merge requests from the fixture branch, is read
	// over at most.
	maxListPages = 100
	// readmeFile is the README GitLab creates with a new project, which the
	// fixture's own replaces.
	readmeFile = "README.md"
)

// errRefused is a setting refused before it could do harm, which the process
// reports with exit code 2.
var errRefused = errors.New("refused")

// setupFromArgs reads the setup flags and the environment, and returns the
// configuration and true, or false and the code to exit with: 0 after -h,
// which printed the flags, and 2 for a usage error.
func setupFromArgs(args []string, getenv func(string) string, stderr io.Writer) (setupConfig, int, bool) {
	flags := flag.NewFlagSet(toolName+" setup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mirror := flags.Bool("mirror-cli", false, "also mirror gitlab-org/cli as <namespace>/glab-mirror")
	skipMirror := flags.Bool("skip-mirror", false, "do not mirror gitlab-org/cli, whatever -mirror-cli says")
	root := flags.String("root", ".", "the repository root, which holds "+fixturesDir)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return setupConfig{}, 0, false
		}
		return setupConfig{}, 2, false
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s setup: unknown argument %q\n", toolName, flags.Arg(0))
		return setupConfig{}, 2, false
	}
	mergeInterval, err := mergePollInterval(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "%s setup: %v\n", toolName, err)
		return setupConfig{}, 2, false
	}
	token := getenv("GITLAB_COM_TOKEN")
	gitlabURL := strings.TrimSuffix(firstNonEmpty(getenv("ORBIT_FIXTURES_GITLAB_URL"), defaultGitLabURL), "/")
	return setupConfig{
		token:         token,
		gitlabURL:     gitlabURL,
		namespace:     firstNonEmpty(getenv("ORBIT_FIXTURES_NAMESPACE"), defaultNamespace),
		visibility:    firstNonEmpty(getenv("ORBIT_FIXTURES_VISIBILITY"), "public"),
		mirror:        *mirror && !*skipMirror,
		fixtures:      filepath.Join(*root, filepath.FromSlash(fixturesDir)),
		api:           gitlabAPI{root: gitlabURL + "/api/v4", token: token, client: httpClient()},
		git:           runGit,
		mergePolls:    60,
		mergeInterval: mergeInterval,
		sleep:         sleepContext,
	}, 0, true
}

// mergePollInterval is the wait between two reads of the fixture merge
// request while GitLab checks it: ORBIT_FIXTURES_MERGE_POLL_SECONDS seconds, 2
// by default, read as the script's sleep read it, so a fraction counts and 0
// reads again at once. A value that is not a number of seconds, zero or more,
// or that a duration cannot hold, is refused rather than waited on.
func mergePollInterval(getenv func(string) string) (time.Duration, error) {
	value := firstNonEmpty(getenv("ORBIT_FIXTURES_MERGE_POLL_SECONDS"), "2")
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || !(seconds >= 0) || seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return 0, fmt.Errorf("ORBIT_FIXTURES_MERGE_POLL_SECONDS=%q is not a number of seconds, zero or more", value)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// runSetup provisions the fixtures and returns the exit code: 0 when every
// fixture is in place, 2 when a setting was refused, and 1 for anything else
// that stopped the run.
func runSetup(ctx context.Context, cfg setupConfig, stdout, stderr io.Writer) int {
	if cfg.token == "" {
		fmt.Fprintf(stderr, "%s setup: GITLAB_COM_TOKEN is required\n", toolName)
		return 1
	}
	run := &provisioner{setupConfig: cfg, out: stdout, errOut: stderr}
	if err := run.provision(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, errRefused) {
			return 2
		}
		return 1
	}
	return 0
}

// provisioner is one setup run in progress.
type provisioner struct {
	setupConfig
	out, errOut io.Writer
	// namespaceEnc is the namespace with every character but the unreserved
	// ones percent-encoded, its slashes included, as GitLab finds a group or
	// a project by its full path.
	namespaceEnc string
	namespaceID  int64
	kgID         int64
}

// refuseNonPublic refuses a visibility other than public under jmrp-oss
// (issue 1533): that group allows one private project and these fixtures are
// public, and a run sets the visibility of projects it did not create, so a
// typo would flip both. GitLab resolves paths whatever their case, so the
// comparison is too.
func (p *provisioner) refuseNonPublic(groupPath string) error {
	lower := strings.ToLower(groupPath)
	if (lower == "jmrp-oss" || strings.HasPrefix(lower, "jmrp-oss/")) && p.visibility != "public" {
		return fmt.Errorf("error: ORBIT_FIXTURES_VISIBILITY=%s is not allowed under %s:\n       jmrp-oss allows one private project, and these fixtures must be public: %w", p.visibility, groupPath, errRefused)
	}
	return nil
}

// provision is the whole run, in the order the fixtures depend on each other.
func (p *provisioner) provision(ctx context.Context) error {
	// Up front, on the namespace as written, so that a refusal sends nothing.
	// The group GitLab resolves it to is checked again before anything is
	// written, since a numeric group id names jmrp-oss without spelling it.
	if err := p.refuseNonPublic(p.namespace); err != nil {
		return err
	}
	if err := p.resolveNamespace(ctx); err != nil {
		return err
	}

	fmt.Fprintf(p.out, "\n>>> fixture: %s/kg-fixtures\n", p.namespace)
	kgID, err := p.createOrReconcile(ctx, "kg-fixtures", filepath.Join(p.fixtures, "kg-fixtures"),
		"Knowledge Graph fixture project: source code with imports, CI pipelines, issues, milestones, merge requests. Designed to exercise every entity type in the Orbit ontology.", "")
	if err != nil {
		return err
	}
	p.kgID = kgID
	if resourcesErr := p.kgResources(ctx); resourcesErr != nil {
		return resourcesErr
	}
	if mergeErr := p.fixtureMergeRequest(ctx); mergeErr != nil {
		return mergeErr
	}

	fmt.Fprintf(p.out, "\n>>> fixture: %s/security-fixtures\n", p.namespace)
	secID, err := p.createOrReconcile(ctx, "security-fixtures", filepath.Join(p.fixtures, "security-fixtures"),
		"Security scan fixture: intentional vulnerable patterns so SAST, Secret Detection, and Dependency Scanning report findings.", "")
	if err != nil {
		return err
	}

	var mirrorID int64
	if p.mirror {
		fmt.Fprintf(p.out, "\n>>> mirror: %s/glab-mirror (from gitlab-org/cli)\n", p.namespace)
		mirrorID, err = p.createOrReconcile(ctx, "glab-mirror", "",
			"Mirror of gitlab-org/cli (the glab CLI). Brings real-world data: real MRs, real CI, real code, real branches. Used as a stable, large-scale fixture for Orbit knowledge graph queries.",
			mirrorUpstream)
		if err != nil {
			return err
		}
		fmt.Fprintf(p.out, "    ✓ glab-mirror id=%d\n", mirrorID)
	}

	fmt.Fprintf(p.out, "\nDone. Fixtures provisioned under %s on %s\n", p.namespace, p.gitlabURL)
	fmt.Fprintf(p.out, "  %s/kg-fixtures        (id=%d): code, CI, milestone, MR\n", p.namespace, p.kgID)
	fmt.Fprintf(p.out, "  %s/security-fixtures  (id=%d): SAST/secret-detection findings\n", p.namespace, secID)
	if p.mirror {
		fmt.Fprintf(p.out, "  %s/glab-mirror       (id=%d): glab CLI mirror\n", p.namespace, mirrorID)
	}
	fmt.Fprintln(p.out, "\nThe Orbit knowledge graph indexer picks up new content within a few minutes.\nRe-run at any time: an existing project is brought into line, and any other\nexisting resource is left as it is.")
	return nil
}

// resolveNamespace finds the group the namespace names and checks the full
// path GitLab answers with against jmrp-oss again. An answer without an id or
// a full path stops the run: the second check must never be skipped because
// the path was not there to read.
func (p *provisioner) resolveNamespace(ctx context.Context) error {
	fmt.Fprintf(p.out, ">>> resolving namespace '%s' on %s\n", p.namespace, p.gitlabURL)
	p.namespaceEnc = url.PathEscape(p.namespace)
	got := p.api.do(ctx, "GET", "/groups/"+p.namespaceEnc, nil)
	var group struct {
		ID       int64  `json:"id"`
		FullPath string `json:"full_path"`
	}
	if !got.decode(&group) || group.ID == 0 || group.FullPath == "" {
		return fmt.Errorf("error: the namespace did not resolve to a group with an id and a full_path: %s", got)
	}
	p.namespaceID = group.ID
	fmt.Fprintf(p.out, "    namespace id: %d (%s)\n", group.ID, group.FullPath)
	return p.refuseNonPublic(group.FullPath)
}

// createOrReconcile returns the id of the project path under the namespace,
// reconciling it when it exists and creating it when it does not. A fixture
// directory of "" is the mirror, whose content comes from mirrorFrom.
func (p *provisioner) createOrReconcile(ctx context.Context, path, fixtureDir, description, mirrorFrom string) (int64, error) {
	var existing struct {
		ID int64 `json:"id"`
	}
	if p.api.do(ctx, "GET", "/projects/"+p.namespaceEnc+"%2F"+path, nil).decode(&existing) && existing.ID > 0 {
		fmt.Fprintf(p.errOut, "    ✓ project exists: %s/%s (id=%d)\n", p.namespace, path, existing.ID)
		return existing.ID, p.reconcile(ctx, existing.ID, fixtureDir)
	}
	return p.create(ctx, path, fixtureDir, description, mirrorFrom)
}

// reconcile brings a fixture project that already exists into line (issue
// 1533): it adds the fixture files the project lacks, then sets the
// visibility, so that a run that stops halfway has not made public a project
// still without its LICENSE. A read that failed stops it rather than reading
// as a visibility to change.
func (p *provisioner) reconcile(ctx context.Context, id int64, fixtureDir string) error {
	got := p.api.do(ctx, "GET", fmt.Sprintf("/projects/%d", id), nil)
	if got.status != 200 {
		return fmt.Errorf("    ✗ cannot read project %d (HTTP %d); not reconciling", id, got.status)
	}
	var project struct {
		Visibility string `json:"visibility"`
	}
	got.decode(&project)
	if !slices.Contains([]string{"public", "internal", "private"}, project.Visibility) {
		return fmt.Errorf("    ✗ project %d has no readable visibility; not reconciling", id)
	}
	// A mirror has no fixture files to compare, and its tree is the upstream
	// project's: thousands of entries read for nothing.
	if fixtureDir != "" {
		if err := p.addMissingFiles(ctx, id, fixtureDir); err != nil {
			return err
		}
	}
	if project.Visibility == p.visibility {
		return nil
	}
	set := p.api.do(ctx, "PUT", fmt.Sprintf("/projects/%d", id), map[string]string{"visibility": p.visibility})
	if !set.ok() {
		return fmt.Errorf("    ✗ could not set visibility (HTTP %d): %s", set.status, set)
	}
	fmt.Fprintf(p.errOut, "    → visibility %s → %s\n", project.Visibility, p.visibility)
	return nil
}

// fileAction is one file of a commit made through the commits API.
type fileAction struct {
	Action   string `json:"action"`
	FilePath string `json:"file_path"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// addMissingFiles creates, in one commit on main, the files git tracks in the
// fixture that the project lacks. One commit and one request: the fixture's
// .gitlab-ci.yml would start a pipeline per commit, each on a partial tree. A
// project holding GitLab's README alone is one whose first push failed, the
// push that replaces that README, so there it is replaced.
func (p *provisioner) addMissingFiles(ctx context.Context, id int64, fixtureDir string) error {
	present, err := p.repositoryFiles(ctx, id)
	if err != nil {
		return err
	}
	tracked, err := trackedFiles(ctx, p.git, fixtureDir)
	if err != nil {
		return fmt.Errorf("    ✗ cannot compare the project's files with the fixture; not reconciling: %w", err)
	}
	onlyReadme := len(present) == 1 && present[readmeFile]
	var actions []fileAction
	for _, rel := range tracked {
		replace := onlyReadme && rel == readmeFile
		if present[rel] && !replace {
			continue
		}
		content, showErr := committedBytes(ctx, p.git, fixtureDir, rel)
		if showErr != nil {
			return fmt.Errorf("    ✗ cannot compare the project's files with the fixture; not reconciling: %w", showErr)
		}
		action := "create"
		if replace {
			action = "update"
		}
		actions = append(actions, fileAction{Action: action, FilePath: rel, Encoding: "base64", Content: base64.StdEncoding.EncodeToString(content)})
	}
	if len(actions) == 0 {
		fmt.Fprintln(p.errOut, "    ✓ project files are in line")
		return nil
	}
	commit := p.api.do(ctx, "POST", fmt.Sprintf("/projects/%d/repository/commits", id), map[string]any{
		"branch":         "main",
		"commit_message": "Add the fixture files this project lacks",
		"actions":        actions,
	})
	if !commit.ok() {
		return fmt.Errorf("    ✗ could not add the missing fixture files (HTTP %d): %s", commit.status, commit)
	}
	for _, action := range actions {
		verb := "added"
		if action.Action == "update" {
			verb = "replaced"
		}
		fmt.Fprintf(p.errOut, "    → %s %s\n", verb, action.FilePath)
	}
	return nil
}

// repositoryFiles is the set of paths branch main of the project holds, read
// over every page of the recursive tree: a fixture path past the first page
// would otherwise be created again on every run. A directory does not stand
// in for a file of the same name. A tree that runs past maxListPages full
// pages stops the run rather than paging on for ever: no fixture comes near
// it, and a project that does is not one this command made.
func (p *provisioner) repositoryFiles(ctx context.Context, id int64) (map[string]bool, error) {
	present := map[string]bool{}
	for index := range maxListPages {
		page := index + 1
		got := p.api.do(ctx, "GET", fmt.Sprintf("/projects/%d/repository/tree?ref=main&recursive=true&per_page=100&page=%d", id, page), nil)
		if got.status != 200 {
			return nil, fmt.Errorf("    ✗ cannot list the project's files (HTTP %d); not reconciling", got.status)
		}
		var entries []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		}
		if !got.decode(&entries) || entries == nil {
			return nil, errors.New("    ✗ the project's file list is unreadable; not reconciling")
		}
		for _, entry := range entries {
			if entry.Type != "tree" {
				present[entry.Path] = true
			}
		}
		if len(entries) < 100 {
			return present, nil
		}
	}
	return nil, fmt.Errorf("    ✗ the project's file list runs past %d pages; not reconciling", maxListPages)
}

// create makes the project and pushes its content: the fixture files HEAD
// holds, or with mirrorFrom every branch and tag of that repository.
func (p *provisioner) create(ctx context.Context, path, fixtureDir, description, mirrorFrom string) (int64, error) {
	fmt.Fprintf(p.errOut, "    → creating project %s/%s\n", p.namespace, path)
	got := p.api.do(ctx, "POST", "/projects", map[string]any{
		"name":                   path,
		"path":                   path,
		"namespace_id":           p.namespaceID,
		"description":            description,
		"visibility":             p.visibility,
		"initialize_with_readme": true,
		"default_branch":         "main",
		"issues_enabled":         true,
		"merge_requests_enabled": true,
		"wiki_enabled":           false,
	})
	var created struct {
		ID int64 `json:"id"`
	}
	if !got.decode(&created) || created.ID <= 0 {
		return 0, fmt.Errorf("    ✗ failed to create project: %s", got)
	}
	fmt.Fprintf(p.errOut, "    ✓ created project id=%d\n", created.ID)
	remote := p.gitlabURL + "/" + p.namespace + "/" + path + ".git"
	if mirrorFrom != "" {
		return created.ID, p.pushMirror(ctx, mirrorFrom, remote)
	}
	return created.ID, p.pushFixture(ctx, fixtureDir, remote)
}

// authHeader is the git configuration that authenticates a clone or a push.
// It is handed to git as a value, never in a URL git might echo on a failure.
func (p *provisioner) authHeader() string {
	return "http.extraHeader=PRIVATE-TOKEN: " + p.token
}

// pushFixture clones the new project, replaces GitLab's README with the
// fixture files HEAD holds, and pushes them to main in one commit. The work
// directory is a temporary one, removed when the push is done, so the token
// git was configured with goes with it.
func (p *provisioner) pushFixture(ctx context.Context, fixtureDir, remote string) error {
	work, err := os.MkdirTemp("", "orbit-fixture-push-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	clone := filepath.Join(work, "repository")
	steps := [][]string{{"-c", p.authHeader(), "clone", gitQuiet, remote, clone}}
	if cloneErr := p.gitSteps(ctx, work, steps); cloneErr != nil {
		return cloneErr
	}
	if removeErr := os.Remove(filepath.Join(clone, readmeFile)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	if copyErr := p.copyTrackedFiles(ctx, fixtureDir, clone); copyErr != nil {
		return copyErr
	}
	return p.gitSteps(ctx, clone, [][]string{
		{"config", "--local", "http.extraHeader", "PRIVATE-TOKEN: " + p.token},
		{"-c", "user.email=" + botEmail, "-c", "user.name=" + botName, "add", "-A"},
		{"-c", "user.email=" + botEmail, "-c", "user.name=" + botName, "commit", "-m", "Initial fixture content", gitQuiet},
		{"push", gitQuiet, "-u", "origin", "main"},
	})
}

// copyTrackedFiles writes every file git tracks in fixtureDir, with the bytes
// HEAD holds, into dir: the projects are public, and an untracked file of the
// working tree, a local .env among them, must not reach one.
func (p *provisioner) copyTrackedFiles(ctx context.Context, fixtureDir, dir string) error {
	tracked, err := trackedFiles(ctx, p.git, fixtureDir)
	if err != nil {
		return err
	}
	for _, rel := range tracked {
		content, showErr := committedBytes(ctx, p.git, fixtureDir, rel)
		if showErr != nil {
			return showErr
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if dirErr := os.MkdirAll(filepath.Dir(target), 0o750); dirErr != nil {
			return dirErr
		}
		if writeErr := os.WriteFile(target, content, 0o600); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// pushMirror mirrors upstream into the new project: every branch and tag.
func (p *provisioner) pushMirror(ctx context.Context, upstream, remote string) error {
	fmt.Fprintf(p.errOut, "    → mirroring from %s (this may take several minutes)\n", upstream)
	work, err := os.MkdirTemp("", "orbit-fixture-mirror-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	mirror := filepath.Join(work, "mirror.git")
	if cloneErr := p.gitSteps(ctx, work, [][]string{{"clone", gitQuiet, "--mirror", upstream, mirror}}); cloneErr != nil {
		return cloneErr
	}
	return p.gitSteps(ctx, mirror, [][]string{
		{"config", "--local", "http.extraHeader", "PRIVATE-TOKEN: " + p.token},
		{"push", gitQuiet, "--force", remote, "refs/heads/*:refs/heads/*", "refs/tags/*:refs/tags/*"},
	})
}

// gitSteps runs each git command in dir, stopping at the first that fails.
func (p *provisioner) gitSteps(ctx context.Context, dir string, steps [][]string) error {
	for _, args := range steps {
		if _, err := p.git(ctx, dir, args...); err != nil {
			return fmt.Errorf("    ✗ %w", err)
		}
	}
	return nil
}
