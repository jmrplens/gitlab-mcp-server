package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// A pinned reference is owner/repo[/subpath]@<40 hex>, optionally followed by a
// comment naming the human-readable version Dependabot keeps current.
var (
	pinnedUses = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
	usesLine   = regexp.MustCompile(`^\s*(?:-\s*)?uses:\s*(\S+)`)
)

// envExpression matches the one indirection a version pin is allowed to take:
// a ${{ env.NAME }} reference into the workflow's top-level env block.
var envExpression = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// exactVersion is the Go spelling of Python's re.fullmatch of v<major>.<minor>.<patch>.
// Go's $ is end-of-text outside multiline mode, so ^…$ is a full match.
var exactVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// supportedMajor picks the `N.x` cell out of a SECURITY.md table row.
var supportedMajor = regexp.MustCompile("`(\\d+)\\.x`")

// pipInstall is the positive half of the pip rule. RE2 has no lookahead, so the
// original pattern's negative lookahead is applied separately by
// [pipInstallWithoutHashes] against the remainder of the matched line.
var pipInstall = regexp.MustCompile(`\bpip\s+install\b`)

// credentialedPermissions are the permissions that make a job worth attacking:
// one can rewrite the repository, one mints the repository's signing and
// publishing identity, and one pushes the image server.json pins to ghcr.io.
var credentialedPermissions = []string{"contents", "id-token", "packages"}

// cooldownEcosystems are the ecosystems where an explicit cooldown is
// meaningful. docker-compose is exempt: its images are :latest test fixtures
// that open no pull request.
var cooldownEcosystems = []string{"gomod", "npm", "github-actions", "docker"}

// minCooldownDays is the shortest release window this repository accepts before
// Dependabot may propose a dependency.
const minCooldownDays = 3

// signatureTools are the invocations that prove an installer reaches for a
// signature rather than only a sibling checksum file.
var signatureTools = []string{"cosign verify-blob", "gh attestation verify", "Invoke-Signature"}

// dependabotPath is the repository-relative label the dependabot findings carry.
const dependabotPath = ".github/dependabot.yml"

// workflowDir is the repository-relative directory every workflow is read from.
const workflowDir = ".github/workflows"

// unlockedRule names one shape of code resolved at run time: bytes that nothing
// committed in this repository fixes.
//
// display is the finding's rendering of the rule, kept byte-identical to the
// Python auditor's repr of the original regular expression so a message this
// program prints is the message that program printed. why explains the finding
// to whoever has to act on it, match decides it, and name is how a declaration
// in [declaredRunTimeCode] names the rule it excuses.
type unlockedRule struct {
	match   func(text string) bool
	name    string
	display string
	why     string
}

// unlockedCode is the rule set applied to every run: block of a credentialed
// job and to every script and make recipe such a block reaches.
var unlockedCode = []unlockedRule{
	{
		match:   regexpMatcher(regexp.MustCompile(`\bnpx\b`)),
		name:    "npx",
		display: `'\\bnpx\\b'`,
		why:     "npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)",
	},
	{
		match:   regexpMatcher(regexp.MustCompile(`@latest\b`)),
		name:    "latest",
		display: `'@latest\\b'`,
		why:     "@latest is whatever the registry serves at that moment",
	},
	{
		match:   regexpMatcher(regexp.MustCompile(`curl[^\n|]*\|\s*(?:ba)?sh\b`)),
		name:    "curl-pipe-shell",
		display: `'curl[^\\n|]*\\|\\s*(?:ba)?sh\\b'`,
		why:     "piping a download into a shell runs unreviewed code",
	},
	{
		match:   pipInstallWithoutHashes,
		name:    "pip-unhashed",
		display: `'\\bpip\\s+install\\b(?![^\\n]*--require-hashes)'`,
		why:     "pip install without --require-hashes resolves at run time",
	},
	{
		match:   regexpMatcher(cargoBinstall),
		name:    "cargo-binstall",
		display: `'\\bcargo[ -]binstall\\b'`,
		why: "cargo binstall installs a prebuilt binary from a release asset or QuickInstall whatever version " +
			"it names, which nothing committed here fixes (download the binary and hold it to a recorded SHA-256)",
	},
	{
		match:   cargoInstallUnlocked,
		name:    "cargo-install",
		display: `'\\bcargo\\s+install\\b'`,
		why: "cargo install resolves a crate and its dependencies at run time unless its command carries " +
			"--locked and an exact version or a --path",
	},
	{
		match:   goInstallUnpinned,
		name:    "go-install",
		display: `'\\bgo\\s+install\\b'`,
		why: "go install at anything but an exact version or a full commit (a branch, a version query, " +
			"a value only the run knows) resolves at run time",
	},
	{
		match:   regexpMatcher(downloadInvokeExpression),
		name:    "invoke-expression",
		display: `'(?i)\\b(?:iex|invoke-expression)\\b'`,
		why: "handing a download (iwr, irm, Invoke-WebRequest, Invoke-RestMethod, DownloadString) to " +
			"Invoke-Expression on the same line runs unreviewed code",
	},
}

// cargoBinstall is cargo-binstall run as cargo's subcommand or by its own
// name, and not the name inside a URL or an archive that downloads it.
var cargoBinstall = regexp.MustCompile(`(?m)\bcargo(?:[ \t]+binstall|-binstall(?:\.exe)?)(?:[ \t]|$)`)

// downloadInvokeExpression is a download PowerShell runs as code on the same
// line: piped into Invoke-Expression, or handed to it as its argument.
// PowerShell's names are case-insensitive, and so is the match.
var downloadInvokeExpression = regexp.MustCompile(
	`(?i)\b(?:iwr|irm|invoke-webrequest|invoke-restmethod)\b[^\n|]*\|\s*(?:iex|invoke-expression)\b` +
		`|\b(?:iex|invoke-expression)\b[^\n]*\b(?:iwr|irm|invoke-webrequest|invoke-restmethod|downloadstring)\b`,
)

// cargoInstall and goInstall find an install and the rest of its line, which
// installCommand cuts at the first shell separator so a word of the next
// command is not read as an argument of this one.
var (
	cargoInstall = regexp.MustCompile(`(?m)\bcargo[ \t]+install\b(.*)$`)
	goInstall    = regexp.MustCompile(`(?m)\bgo[ \t]+install\b(.*)$`)
)

// exactCrateVersion is a version cargo install reads as exactly one release:
// MAJOR.MINOR.PATCH with an optional pre-release and build, bare or behind the
// = requirement operator, since cargo reads a bare version as exact there.
var exactCrateVersion = regexp.MustCompile(`^=?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// exactModuleVersion is a version go install fetches exactly one module
// content for: a semantic version, a pseudo-version included, or a full
// commit, all of which the checksum database fixes.
var exactModuleVersion = regexp.MustCompile(`^(?:v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?|[0-9a-f]{40})$`)

// installArguments returns the words of every install expression finds in
// text, each cut at the first shell separator and stripped of the quotes and
// parentheses around it.
func installArguments(expression *regexp.Regexp, text string) [][]string {
	var commands [][]string
	for _, match := range expression.FindAllStringSubmatch(text, -1) {
		rest := match[1]
		if end := strings.IndexAny(rest, ";&|"); end >= 0 {
			rest = rest[:end]
		}
		var words []string
		for field := range strings.FieldsSeq(rest) {
			words = append(words, strings.Trim(field, `"'()`))
		}
		commands = append(commands, words)
	}
	return commands
}

// cargoInstallUnlocked reports whether text runs a cargo install that does not
// carry both --locked and either an exact version or a --path on its command.
//
// crates.io never lets a version be republished, and --locked builds it with
// the dependency versions it was published with, so the two together fix what
// is built; a --path builds the checkout's own crate. Anything else, a newest
// release or a requirement such as ^2.16, is resolved when the job runs.
func cargoInstallUnlocked(text string) bool {
	for _, words := range installArguments(cargoInstall, text) {
		locked, fixed := false, false
		for index, word := range words {
			if word == "--locked" {
				locked = true
				continue
			}
			if fixesCrate(words, index) {
				fixed = true
			}
		}
		if !locked || !fixed {
			return true
		}
	}
	return false
}

// fixesCrate reports whether the word at index of a cargo install's words
// fixes what it builds: a --path, or an exact version after a crate's @,
// after --version= or --vers=, or as the word after --version or --vers.
func fixesCrate(words []string, index int) bool {
	word := words[index]
	if word == "--path" || strings.HasPrefix(word, "--path=") || exactCrateVersion.MatchString(crateVersion(word)) {
		return true
	}
	return (word == "--version" || word == "--vers") && index+1 < len(words) && exactCrateVersion.MatchString(words[index+1])
}

// crateVersion is the version a word of a cargo install names: the value of
// --version= or --vers=, or what follows a crate's @, and nothing otherwise.
func crateVersion(word string) string {
	for _, option := range []string{"--version=", "--vers="} {
		if value, found := strings.CutPrefix(word, option); found {
			return value
		}
	}
	_, version, _ := strings.Cut(word, "@")
	return version
}

// goInstallUnpinned reports whether text runs a go install of a module at
// anything but an exact version or a full commit: a branch, a version query
// such as v1, or a variable whose value only the run knows. A package named
// without a version is built at the version go.mod requires, which go.sum
// fixes, and @latest is left to the latest rule so one install is one
// finding.
func goInstallUnpinned(text string) bool {
	for _, words := range installArguments(goInstall, text) {
		for _, word := range words {
			_, version, versioned := strings.Cut(word, "@")
			if versioned && version != "latest" && !exactModuleVersion.MatchString(version) {
				return true
			}
		}
	}
	return false
}

// regexpMatcher adapts a compiled expression to [unlockedRule].match.
func regexpMatcher(expression *regexp.Regexp) func(string) bool {
	return expression.MatchString
}

// pipInstallWithoutHashes reports whether text runs pip install on a line that
// never says --require-hashes.
//
// This is the RE2 rendering of the original lookahead. Python asked for
// \bpip\s+install\b(?![^\n]*--require-hashes), whose negative lookahead scans
// the remainder of the line the match ends on; Go's regexp package has no
// lookahead, so that remainder is sliced out and searched directly. The two
// halves must stay together: dropping the second one would flag every hashed
// install, and dropping the first would flag nothing.
func pipInstallWithoutHashes(text string) bool {
	for _, location := range pipInstall.FindAllStringIndex(text, -1) {
		rest := text[location[1]:]
		if end := strings.IndexByte(rest, '\n'); end >= 0 {
			rest = rest[:end]
		}
		if !strings.Contains(rest, "--require-hashes") {
			return true
		}
	}
	return false
}

// workflow is one workflow file, kept in both of the forms the audit needs: the
// raw text the pinning rule reads and the parsed document the job rules read.
//
// jobOrder records the document order of the jobs mapping's keys. Go map
// iteration is randomized, so without it two runs over a workflow with two
// offending jobs would print the same findings in a different order.
type workflow struct {
	doc      map[string]any
	path     string
	text     string
	jobOrder []string
}

// loadWorkflows reads every workflow file under .github/workflows, in filename
// order, and returns each one's text and parsed document.
//
// A file that does not parse to a mapping keeps a nil doc: only the pinning
// rule, which works on text, applies to it.
func loadWorkflows(root string) ([]workflow, error) {
	directory := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", directory, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isWorkflowName(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	workflows := make([]workflow, 0, len(names))
	for _, name := range names {
		full := filepath.Join(directory, name)
		text, readErr := readTextFile(full)
		if readErr != nil {
			return nil, readErr
		}
		doc, order, parseErr := parseWorkflow(text)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", full, parseErr)
		}
		workflows = append(workflows, workflow{
			doc:      doc,
			path:     workflowDir + "/" + name,
			text:     text,
			jobOrder: order,
		})
	}
	return workflows, nil
}

// isWorkflowName reports whether a directory entry is a workflow file.
func isWorkflowName(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

// parseWorkflow decodes one workflow into a generic mapping plus the document
// order of its jobs keys.
func parseWorkflow(text string) (doc map[string]any, jobOrder []string, err error) {
	var root yaml.Node
	if unmarshalErr := yaml.Unmarshal([]byte(text), &root); unmarshalErr != nil {
		return nil, nil, unmarshalErr
	}
	if root.Kind == 0 {
		// An empty file parses to no node at all. That is not a failure, and
		// it leaves the job rules nothing to read.
		return nil, nil, nil
	}
	var generic any
	if decodeErr := root.Decode(&generic); decodeErr != nil {
		return nil, nil, decodeErr
	}
	mapping, _ := generic.(map[string]any)
	return mapping, jobsKeyOrder(&root), nil
}

// jobsKeyOrder walks the parsed nodes for the top-level jobs mapping and
// returns its keys in the order the file wrote them.
func jobsKeyOrder(root *yaml.Node) []string {
	mapping := documentMapping(root)
	if mapping == nil {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != "jobs" {
			continue
		}
		jobs := mapping.Content[index+1]
		if jobs.Kind != yaml.MappingNode {
			return nil
		}
		var order []string
		for key := 0; key+1 < len(jobs.Content); key += 2 {
			order = append(order, jobs.Content[key].Value)
		}
		return order
	}
	return nil
}

// documentMapping unwraps a document node down to the mapping it carries.
func documentMapping(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

// checkPinnedUses reports every action reference that is a tag, a branch or a
// short SHA rather than a full commit SHA.
//
// It reads the file text rather than the parsed document on purpose: a uses:
// line inside a comment, or in a region a parser skipped, is still a line a
// future edit can uncomment, and the rule that catches a hijacked tag is worth
// nothing if it can be hidden behind a #.
func checkPinnedUses(pathLabel, text string) []string {
	var problems []string
	for index, line := range splitLines(text) {
		groups := usesLine.FindStringSubmatch(line)
		if groups == nil {
			continue
		}
		reference := strings.Trim(strings.TrimSpace(groups[1]), "\"'")
		if strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "docker://") {
			continue
		}
		if !pinnedUses.MatchString(reference) {
			problems = append(problems, fmt.Sprintf(
				"%s:%d: uses: %s is not pinned to a 40-character commit SHA", pathLabel, index+1, reference,
			))
		}
	}
	return problems
}

// jobPermissions returns the effective permissions for a job: its own if
// present, else the workflow's.
//
// An empty mapping written on the job is a decision, not an absence, so it does
// not fall back to the workflow's block. GitHub's own default is a read-only
// token, so a job with no permissions anywhere is uncredentialed.
func jobPermissions(doc, job map[string]any) map[string]any {
	permissions := job["permissions"]
	if permissions == nil {
		permissions = doc["permissions"]
	}
	if permissions == nil {
		return map[string]any{}
	}
	if shorthand, ok := permissions.(string); ok {
		// "write-all" / "read-all" shorthand.
		if shorthand == "write-all" {
			return map[string]any{"contents": "write", "id-token": "write", "packages": "write"}
		}
		return map[string]any{}
	}
	if mapping, ok := permissions.(map[string]any); ok {
		return mapping
	}
	return map[string]any{}
}

// isCredentialed reports whether a job's effective permissions grant one of the
// three writes the hardening rules exist for. A secret that can write makes a
// job credentialed too; [supplyChainAudit.checkWorkflowJobs] adds that.
func isCredentialed(doc, job map[string]any) bool {
	permissions := jobPermissions(doc, job)
	return slices.ContainsFunc(credentialedPermissions, func(name string) bool {
		return permissions[name] == "write"
	})
}

// resolveEnv substitutes an env expression from the workflow's top-level env
// block.
//
// Version pins are declared once in env: and referenced from the steps that
// download the tool, so the checker has to see through that one indirection or
// it reads every pin as unpinned. A name the block does not define is left
// standing, which then fails the exact-version rule, as it should.
func resolveEnv(value any, doc map[string]any) string {
	environment, _ := doc["env"].(map[string]any)
	return envExpression.ReplaceAllStringFunc(pythonStr(value), func(match string) string {
		name := envExpression.FindStringSubmatch(match)[1]
		replacement, ok := environment[name]
		if !ok {
			return match
		}
		return pythonStr(replacement)
	})
}

// stripComments drops whole-line comments so a rationale about npx is not read
// as npx.
func stripComments(text string) string {
	lines := splitLines(text)
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// tables are the declarations an audit holds the repository to.
type tables struct {
	secrets      map[string]secretDeclaration
	declarations map[string]declaration
	unjudged     map[string]declaration
	guarded      map[string]guardedMatch
}

// supplyChainAudit is one audit of one repository root: the tables it judges
// by and what it learns about them across every workflow.
//
// secretsRead, excused, unjudgedUsed and guardedUsed are what the table rules
// are judged on once every workflow has been read: a secret no workflow reads
// and a declaration that excused nothing are both stale. makefiles holds each
// Makefile parsed once, keyed by its repository-relative path, since every job
// that runs make reads the same rules. store answers what a pinned action's
// commit holds, and is nil in an audit built for one job rule's unit test;
// reported holds the findings about an action that are made once however many
// steps reach it.
type supplyChainAudit struct {
	tables

	store        *actionStore
	secretsRead  map[string]bool
	excused      map[string]bool
	unjudgedUsed map[string]bool
	guardedUsed  map[string]bool
	reported     map[string]bool
	makefiles    map[string]map[string]*makeRule
	root         string
}

// newAudit starts an audit of root against the given tables.
func newAudit(root string, declared tables) *supplyChainAudit {
	return &supplyChainAudit{
		tables:       declared,
		secretsRead:  map[string]bool{},
		excused:      map[string]bool{},
		unjudgedUsed: map[string]bool{},
		guardedUsed:  map[string]bool{},
		reported:     map[string]bool{},
		makefiles:    map[string]map[string]*makeRule{},
		root:         root,
	}
}

// checkCredentialedJob reports everything a job holding a write credential runs
// that this repository does not pin: in its own steps, and in every script and
// make recipe those steps reach, however deep.
func (a *supplyChainAudit) checkCredentialedJob(pathLabel, jobID string, job, doc map[string]any) []string {
	return a.judgeCredentialedJob(pathLabel, jobID, job, doc, map[string]bool{})
}

// judgeCredentialedJob is [supplyChainAudit.checkCredentialedJob] with what
// the job has read already, which the walk through its actions shares.
func (a *supplyChainAudit) judgeCredentialedJob(pathLabel, jobID string, job, doc map[string]any, seen map[string]bool) []string {
	var problems []string
	steps, _ := job["steps"].([]any)
	jobLabel := pathLabel + ": job " + jobID
	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		rawUses, _ := step["uses"].(string)
		uses, _, _ := strings.Cut(rawUses, "@")
		with, _ := step["with"].(map[string]any)
		where := fmt.Sprintf("%s: step %d", jobLabel, index)

		problems = append(problems, checkStepAction(where, uses, with, doc)...)

		runSource, _ := step["run"].(string)
		runText := stripComments(runSource)
		for _, rule := range unlockedCode {
			if rule.match(runText) {
				problems = append(problems, fmt.Sprintf("%s: run block matches %s: %s", where, rule.display, rule.why))
			}
		}
		reached := reach{job: jobLabel, origin: where, directory: runDirectory(doc, job, step)}
		problems = append(problems, a.followReferences(reached, runText, seen)...)
	}
	return problems
}

// runDirectory is the repository-relative directory a step's run block runs
// in: its own working-directory, else the job's default, else the workflow's,
// else the checkout itself, which is the order GitHub applies them in.
func runDirectory(doc, job, step map[string]any) string {
	if directory, ok := step["working-directory"].(string); ok {
		return directory
	}
	if directory, ok := defaultRunDirectory(job); ok {
		return directory
	}
	directory, _ := defaultRunDirectory(doc)
	return directory
}

// defaultRunDirectory reads defaults.run.working-directory off a job or a
// workflow.
func defaultRunDirectory(holder map[string]any) (string, bool) {
	defaults, _ := holder["defaults"].(map[string]any)
	run, _ := defaults["run"].(map[string]any)
	directory, ok := run["working-directory"].(string)
	return directory, ok
}

// checkStepAction applies the per-action rules: the checkout that must not
// persist a credential, the GoReleaser binary that must be pinned, and the SBOM
// action that must not be here at all.
func checkStepAction(where, uses string, with, doc map[string]any) []string {
	var problems []string
	if uses == "actions/checkout" && !persistCredentialsDisabled(with) {
		problems = append(problems, where+": actions/checkout must set persist-credentials: false — "+
			"the job's write-capable token would otherwise sit in .git/config "+
			"while the rest of the steps run")
	}
	if uses == "goreleaser/goreleaser-action" {
		raw, ok := with["version"]
		if !ok {
			raw = ""
		}
		version := resolveEnv(raw, doc)
		if !exactVersion.MatchString(version) {
			problems = append(problems, fmt.Sprintf(
				"%s: goreleaser-action version %s is not an exact vX.Y.Z — "+
					"pinning the action does not pin the binary it downloads", where, pythonRepr(version),
			))
		}
	}
	if strings.HasPrefix(uses, "anchore/sbom-action") {
		problems = append(problems, where+": anchore/sbom-action must not run in a credentialed job. Even SHA-pinned, "+
			"on Linux it downloads raw.githubusercontent.com/anchore/syft/main/install.sh and "+
			"runs it with sh, so the code executing here comes from a branch head. syft-version "+
			"only selects the tarball that mutable script fetches. Download the release tarball "+
			"directly, pinned by SHA256 and verified with cosign, as the Install syft step does")
	}
	return problems
}

// persistCredentialsDisabled reports whether a checkout step turned the
// credential off, accepting either the boolean or the string GitHub allows.
func persistCredentialsDisabled(with map[string]any) bool {
	switch value := with["persist-credentials"].(type) {
	case bool:
		return !value
	case string:
		return value == "false"
	default:
		return false
	}
}

// checkWorkflowJobs applies the credentialed-job rules to every job in a
// workflow that holds a write credential, through its permissions or through a
// secret it reads, and holds every secret any job reads to the secret table.
//
// A job reading a secret the table does not declare, or every secret at once,
// is judged as credentialed: the audit cannot say what it holds, and the
// finding that names the read is what gets it declared.
//
// jobOrder carries the document order when the caller parsed a file; a document
// built in a test carries none, and the keys are then sorted so the findings
// are still deterministic.
func (a *supplyChainAudit) checkWorkflowJobs(pathLabel string, doc map[string]any, jobOrder []string) []string {
	jobs, _ := doc["jobs"].(map[string]any)
	var problems []string
	for _, jobID := range orderedKeys(jobs, jobOrder) {
		job, ok := jobs[jobID].(map[string]any)
		if !ok {
			continue
		}
		names, wholesale := jobSecrets(doc, job)
		credentialed := isCredentialed(doc, job) || len(wholesale) > 0
		for _, name := range names {
			a.secretsRead[name] = true
			declared, known := a.secrets[name]
			if !known {
				problems = append(problems, fmt.Sprintf(
					"%s: job %s: reads secrets.%s, which the secret table does not declare: say whether it can write and where",
					pathLabel, jobID, name,
				))
			}
			credentialed = credentialed || !known || declared.writes
		}
		for _, expression := range wholesale {
			problems = append(problems, fmt.Sprintf(
				"%s: job %s: reads ${{ %s }}, which hands over every secret at once and cannot be judged against the secret table",
				pathLabel, jobID, expression,
			))
		}
		seen := map[string]bool{}
		if credentialed {
			problems = append(problems, a.judgeCredentialedJob(pathLabel, jobID, job, doc, seen)...)
		}
		problems = append(problems, a.checkJobActions(pathLabel, jobID, job, doc, credentialed, seen)...)
	}
	return problems
}

// orderedKeys returns the keys of jobs, preferring the document order in order
// and sorting whatever that order does not name.
func orderedKeys(jobs map[string]any, order []string) []string {
	keys := make([]string, 0, len(jobs))
	seen := map[string]bool{}
	for _, key := range order {
		if _, ok := jobs[key]; ok && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	rest := make([]string, 0, len(jobs))
	for key := range jobs {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

// checkDependabot reports every cooldown-capable ecosystem that does not state
// a window of its own.
func checkDependabot(doc map[string]any) []string {
	updates, _ := doc["updates"].([]any)
	var problems []string
	for _, rawEntry := range updates {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		problems = append(problems, checkDependabotEntry(entry)...)
	}
	return problems
}

// checkDependabotEntry applies the cooldown rules to one updates entry.
func checkDependabotEntry(entry map[string]any) []string {
	ecosystem, _ := entry["package-ecosystem"].(string)
	if !slices.Contains(cooldownEcosystems, ecosystem) {
		return nil
	}
	directory := "?"
	if raw, ok := entry["directory"]; ok {
		directory = pythonStr(raw)
	}
	label := fmt.Sprintf("%s: %s (%s)", dependabotPath, ecosystem, directory)

	cooldown, ok := entry["cooldown"].(map[string]any)
	if !ok {
		return []string{label + ": no cooldown — the release window is whatever GitHub defaults to today"}
	}

	var problems []string
	days, isInteger := pythonInt(cooldown[cooldownDefaultDaysKey])
	if !isInteger || days < minCooldownDays {
		problems = append(problems, fmt.Sprintf("%s: cooldown.default-days is %s, want an integer >= %d",
			label, pythonRepr(cooldown[cooldownDefaultDaysKey]), minCooldownDays))
	}
	if extra := semVerCooldownKeys(ecosystem, cooldown); len(extra) > 0 {
		problems = append(problems, fmt.Sprintf(
			"%s: cooldown carries SemVer keys %s — Dependabot rejects them for this "+
				"ecosystem and a rejected configuration stops its updates entirely", label, pythonList(extra),
		))
	}
	return problems
}

// cooldownDefaultDaysKey is the one cooldown sub-key every ecosystem accepts.
// It is named because the docker check is defined against it: every other key
// under cooldown is a SemVer key Dependabot refuses there.
const cooldownDefaultDaysKey = "default-days"

// semVerCooldownKeys returns the cooldown sub-keys Dependabot rejects for the
// docker ecosystems, and nothing for any other ecosystem.
func semVerCooldownKeys(ecosystem string, cooldown map[string]any) []string {
	if ecosystem != "docker" && ecosystem != "docker-compose" {
		return nil
	}
	extra := make([]string, 0, len(cooldown))
	for key := range cooldown {
		if key != cooldownDefaultDaysKey {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return extra
}

// checkSecurityPolicy reports a supported-versions table that has drifted from
// the version the repository ships.
func checkSecurityPolicy(version, securityMD string) []string {
	major, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	var rows []string
	for _, line := range splitLines(securityMD) {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		}
	}
	if len(rows) == 0 {
		return []string{"SECURITY.md: no supported-versions table found"}
	}

	var problems []string
	if !strings.Contains(strings.Join(rows, "\n"), "`"+major+".x`") {
		problems = append(problems, fmt.Sprintf(
			"SECURITY.md: the supported-versions table never names `%s.x`, "+
				"but VERSION says %s", major, strings.TrimSpace(version),
		))
	}
	for _, row := range rows {
		groups := supportedMajor.FindStringSubmatch(row)
		if len(groups) < 2 || groups[1] == major {
			continue
		}
		if strings.Contains(row, ":white_check_mark:") {
			problems = append(problems, fmt.Sprintf(
				"SECURITY.md: `%s.x` is still marked supported while the shipping major is %s", groups[1], major,
			))
		}
	}
	return problems
}

// checkInstallers reports an installer that trusts checksums.txt without ever
// checking the signature published beside it.
func checkInstallers(installSh, installPS1 string) []string {
	installers := []struct {
		name string
		body string
	}{
		{name: "scripts/install.sh", body: installSh},
		{name: "scripts/install.ps1", body: installPS1},
	}
	var problems []string
	for _, installer := range installers {
		verifies := slices.ContainsFunc(signatureTools, func(tool string) bool {
			return strings.Contains(installer.body, tool)
		})
		if !verifies {
			problems = append(problems, installer.name+
				": verifies no signature; checksums.txt comes from the same mutable release "+
				"as the binary, so a consistent replacement of both files is accepted")
		}
		if !strings.Contains(installer.body, "checksums.txt.sigstore.json") {
			problems = append(problems, installer.name+
				": never fetches checksums.txt.sigstore.json, which every release publishes")
		}
	}
	return problems
}

// audit runs every check against a repository root, judging the actions its
// workflows pin from the committed record, and returns the findings in a
// stable order.
func audit(root string, declared tables) ([]string, error) {
	record, err := loadActionRecord(root)
	if err != nil {
		return nil, err
	}
	return auditWith(root, declared, newOfflineStore(record))
}

// auditWith runs every check against a repository root, reading what its
// pinned actions hold through store.
func auditWith(root string, declared tables, store *actionStore) ([]string, error) {
	problems, err := auditWorkflows(root, declared, store)
	if err != nil {
		return nil, err
	}

	dependabot, err := readYAMLMapping(filepath.Join(root, ".github", "dependabot.yml"))
	if err != nil {
		return nil, err
	}
	problems = append(problems, checkDependabot(dependabot)...)

	version, err := readTextFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return nil, err
	}
	securityMD, err := readTextFile(filepath.Join(root, "SECURITY.md"))
	if err != nil {
		return nil, err
	}
	problems = append(problems, checkSecurityPolicy(version, securityMD)...)

	installSh, err := readTextFile(filepath.Join(root, "scripts", "install.sh"))
	if err != nil {
		return nil, err
	}
	installPS1, err := readTextFile(filepath.Join(root, "scripts", "install.ps1"))
	if err != nil {
		return nil, err
	}
	return append(problems, checkInstallers(installSh, installPS1)...), nil
}

// auditWorkflows applies the pinning rule and the credentialed-job rules to
// every workflow file and to the actions their steps name, then holds the
// tables and the record to what was read.
func auditWorkflows(root string, declared tables, store *actionStore) ([]string, error) {
	workflows, err := loadWorkflows(root)
	if err != nil {
		return nil, err
	}
	auditor := newAudit(root, declared)
	auditor.store = store
	var problems []string
	for _, file := range workflows {
		problems = append(problems, checkPinnedUses(file.path, file.text)...)
		if file.doc != nil {
			problems = append(problems, auditor.checkWorkflowJobs(file.path, file.doc, file.jobOrder)...)
		}
	}
	problems = append(problems, auditor.tableProblems()...)
	return append(problems, store.staleProblems()...), nil
}

// recordAndAudit refreshes the record from the network and audits against
// it: every action the workflows reach is fetched at its commit, and every file
// a credentialed job's steps read from an action's directory, so the record
// written is exactly what the audit read. A fetch that fails stops the run
// before anything is written, since a record built on a guess would be judged
// as the truth by every later run.
func recordAndAudit(root string, declared tables, stdout io.Writer) ([]string, error) {
	store := newRecordingStore(newRecordFetcher())
	problems, err := auditWith(root, declared, store)
	if err != nil {
		return nil, err
	}
	if store.err != nil {
		return nil, fmt.Errorf("record: %w", store.err)
	}
	target := filepath.Join(root, filepath.FromSlash(actionRecordPath))
	if writeErr := docgen.WriteOrCheck(target, renderActionRecord(store.record), false, recordCommand); writeErr != nil {
		return nil, writeErr
	}
	fmt.Fprintf(stdout, "recorded %d actions in %s\n", len(store.record.Actions), actionRecordPath)
	return problems, nil
}

// rawContentBase is where -record reads a pinned commit's files: GitHub serves
// every file of a public repository at a commit there, with no token.
const rawContentBase = "https://raw.githubusercontent.com"

// recordClient is the HTTP client -record fetches with, bounded so a host
// that stops answering ends the run rather than hanging it.
func recordClient() *http.Client {
	return &http.Client{Timeout: recordFetchTimeout}
}

// newRecordFetcher is a seam over how -record reaches the network, so a test
// can serve a commit's files itself.
var newRecordFetcher = func() fetchFunc {
	return rawFetcher(recordClient(), rawContentBase)
}

// readTextFile reads one of the repository's own configuration files.
func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- an audit tool reading this repository's own configuration.
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

// readYAMLMapping reads a YAML file that must decode to a mapping.
func readYAMLMapping(path string) (map[string]any, error) {
	text, err := readTextFile(path)
	if err != nil {
		return nil, err
	}
	var mapping map[string]any
	if unmarshalErr := yaml.Unmarshal([]byte(text), &mapping); unmarshalErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, unmarshalErr)
	}
	return mapping, nil
}

// splitLines splits text into lines the way Python's str.splitlines does for
// the line endings a file in this repository can carry: no trailing empty
// element for a final newline, and no carriage return left on a CRLF line.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	normalized := strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	return strings.Split(normalized, "\n")
}

// pythonStr renders a decoded YAML scalar the way Python's str would, so a
// value substituted into a finding reads as it did before the port.
func pythonStr(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case string:
		return typed
	case bool:
		if typed {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(typed)
	}
}

// pythonRepr renders a decoded YAML scalar the way Python's repr would.
//
// Three findings embedded a repr, so reproducing it is what makes this program
// and the Python auditor it replaces print the same bytes for the same
// repository.
func pythonRepr(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return pythonReprString(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		return pythonReprFloat(typed)
	default:
		return fmt.Sprint(typed)
	}
}

// pythonReprString quotes a string the way Python does: single quotes unless
// that would need escaping and double quotes would not.
func pythonReprString(value string) string {
	quote := '\''
	if strings.Contains(value, "'") && !strings.Contains(value, "\"") {
		quote = '"'
	}
	var builder strings.Builder
	builder.WriteRune(quote)
	for _, character := range value {
		switch character {
		case '\\':
			builder.WriteString(`\\`)
		case quote:
			builder.WriteRune('\\')
			builder.WriteRune(character)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			builder.WriteRune(character)
		}
	}
	builder.WriteRune(quote)
	return builder.String()
}

// pythonReprFloat renders a float the way Python does, which always shows a
// decimal point.
func pythonReprFloat(value float64) string {
	rendered := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(rendered, ".eEn") {
		rendered += ".0"
	}
	return rendered
}

// pythonInt reports the integer a decoded YAML scalar carries, applying
// Python's rule that a bool is an int.
func pythonInt(value any) (days int, ok bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case uint64:
		return int(typed), true //#nosec G115 -- a Dependabot cooldown window, compared against a 3-day floor.
	case bool:
		if typed {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// pythonList renders a slice of strings the way Python prints a list of them.
func pythonList(items []string) string {
	rendered := make([]string, len(items))
	for index, item := range items {
		rendered[index] = pythonReprString(item)
	}
	return "[" + strings.Join(rendered, ", ") + "]"
}

// osExit is a seam over os.Exit, so a test can observe the status main exits
// with instead of the test process terminating on it.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr, repositoryTables()))
}

// run parses the command line, audits the repository and returns the process
// exit code: 0 when every invariant holds, 1 when any of them does not or when
// the audit could not be performed at all.
//
// One non-zero code covers both outcomes because that is what the Python
// auditor exposed: it exited 1 on findings and, on a missing file or a broken
// parse, died of an uncaught exception, which is also 1.
func run(args []string, stdout, stderr io.Writer, declared tables) int {
	flags := flag.NewFlagSet("audit_supply_chain", flag.ContinueOnError)
	flags.SetOutput(stderr)
	rootFlag := flags.String("root", "", "repository root (default: the module root at or above the working directory)")
	recordFlag := flags.Bool("record", false, "fetch what every pinned action the workflows reach holds at its commit, "+
		"rewrite "+actionRecordPath+" with it, then audit against it (needs the network; the default run reads the record offline)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		fmt.Fprintf(stderr, "audit_supply_chain: %v\n", err)
		return 1
	}
	var problems []string
	if *recordFlag {
		problems, err = recordAndAudit(root, declared, stdout)
	} else {
		problems, err = audit(root, declared)
	}
	if err != nil {
		fmt.Fprintf(stderr, "audit_supply_chain: %v\n", err)
		return 1
	}
	if len(problems) > 0 {
		fmt.Fprintf(stdout, "supply-chain audit FAILED (%d problems):\n", len(problems))
		for _, problem := range problems {
			fmt.Fprintf(stdout, "  x %s\n", problem)
		}
		return 1
	}
	fmt.Fprint(stdout, "supply-chain audit passed: pinned actions, locked release jobs, "+
		"stated cooldowns, current security policy, signature-verifying installers\n")
	return 0
}

// resolveRoot turns the --root flag into an absolute repository root, falling
// back to the module root at or above the working directory.
func resolveRoot(flagValue string) (string, error) {
	if flagValue == "" {
		return cmdutil.RepositoryRoot(".")
	}
	return filepath.Abs(flagValue)
}
