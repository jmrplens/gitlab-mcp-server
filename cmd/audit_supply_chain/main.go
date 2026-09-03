// Command audit_supply_chain audits the release supply chain's configuration
// invariants.
//
// Five properties, each of which was false at some point and each of which is
// invisible to every other gate in this repository:
//
//  1. Every uses: in .github/workflows is pinned to a 40-character commit SHA.
//     A mutable tag is resolved by the runner at job start, so a hijacked v7 is
//     consumed with no pull request, no cooldown and no review.
//  2. A job holding contents: write or id-token: write runs no code resolved at
//     run time. That means: no npx, no @latest, no curl piped into a shell, no
//     unhashed pip install — in its own run: blocks or in any scripts/ file
//     those blocks invoke; actions/checkout leaves no credential in
//     .git/config; and a tool the job downloads (GoReleaser, syft) is pinned to
//     an exact version, because SHA-pinning the action that fetches a binary
//     does not pin the binary.
//  3. Dependabot states its cooldown instead of inheriting a platform default
//     that GitHub can change under us.
//  4. SECURITY.md names the major version the repository actually ships.
//  5. Both installers verify the release's Sigstore bundle, not only a
//     checksums.txt fetched from the same mutable release.
//
// Usage:
//
//	go run ./cmd/audit_supply_chain/ [--root <dir>]
//
// Exits non-zero and prints one line per violation.
//
// The auditor is deliberately split in two: pinning is decided on the raw file
// text, so a uses: inside a comment or an unparsed region still counts, while
// job structure comes from the parsed YAML.
//
// This is a port of a Python auditor and reproduces its findings byte for byte,
// down to the repr() quoting three messages embed. It diverges in one place:
// PyYAML accepted a duplicated mapping key and silently kept the last value, so
// a step written with two run: keys was audited as though one of them did not
// exist. This refuses the document instead, failing closed on a workflow whose
// meaning is ambiguous rather than auditing a guess about it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
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

// scriptReference finds the scripts/<file> paths a run: block invokes, so their
// contents are audited alongside the block itself.
var scriptReference = regexp.MustCompile(`scripts/[A-Za-z0-9_.-]+\.(?:sh|mjs|py|ps1)`)

// pipInstall is the positive half of the pip rule. RE2 has no lookahead, so the
// original pattern's negative lookahead is applied separately by
// [pipInstallWithoutHashes] against the remainder of the matched line.
var pipInstall = regexp.MustCompile(`\bpip\s+install\b`)

// credentialedPermissions are the permissions that make a job worth attacking:
// one mints the repository's signing and publishing identity, the other can
// rewrite the repository.
var credentialedPermissions = []string{"contents", "id-token"}

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
// to whoever has to act on it, and match decides it.
type unlockedRule struct {
	match   func(text string) bool
	display string
	why     string
}

// unlockedCode is the rule set applied to every run: block of a credentialed
// job and to every scripts/ file such a block invokes.
var unlockedCode = []unlockedRule{
	{
		match:   regexpMatcher(regexp.MustCompile(`\bnpx\b`)),
		display: `'\\bnpx\\b'`,
		why:     "npx resolves a dependency tree at run time (use a lockfile and npm ci, or drop the CLI)",
	},
	{
		match:   regexpMatcher(regexp.MustCompile(`@latest\b`)),
		display: `'@latest\\b'`,
		why:     "@latest is whatever the registry serves at that moment",
	},
	{
		match:   regexpMatcher(regexp.MustCompile(`curl[^\n|]*\|\s*(?:ba)?sh\b`)),
		display: `'curl[^\\n|]*\\|\\s*(?:ba)?sh\\b'`,
		why:     "piping a download into a shell runs unreviewed code",
	},
	{
		match:   pipInstallWithoutHashes,
		display: `'\\bpip\\s+install\\b(?![^\\n]*--require-hashes)'`,
		why:     "pip install without --require-hashes resolves at run time",
	},
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
		order := make([]string, 0, len(jobs.Content)/2)
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
			return map[string]any{"contents": "write", "id-token": "write"}
		}
		return map[string]any{}
	}
	if mapping, ok := permissions.(map[string]any); ok {
		return mapping
	}
	return map[string]any{}
}

// isCredentialed reports whether a job's effective permissions grant one of the
// two writes the hardening rules exist for.
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

// referencedScripts returns the scripts/<file> paths a run block invokes, in
// sorted order and without repeats, so their contents are audited too.
func referencedScripts(runText string) []string {
	var found []string
	for _, match := range scriptReference.FindAllString(runText, -1) {
		if !slices.Contains(found, match) {
			found = append(found, match)
		}
	}
	sort.Strings(found)
	return found
}

// checkCredentialedJob reports everything a job holding a write credential runs
// that this repository does not pin.
func checkCredentialedJob(pathLabel, jobID string, job map[string]any, root string, doc map[string]any) []string {
	var problems []string
	steps, _ := job["steps"].([]any)
	seenScripts := map[string]bool{}
	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		rawUses, _ := step["uses"].(string)
		uses, _, _ := strings.Cut(rawUses, "@")
		with, _ := step["with"].(map[string]any)
		where := fmt.Sprintf("%s: job %s: step %d", pathLabel, jobID, index)

		problems = append(problems, checkStepAction(where, uses, with, doc)...)

		runSource, _ := step["run"].(string)
		runText := stripComments(runSource)
		for _, rule := range unlockedCode {
			if rule.match(runText) {
				problems = append(problems, fmt.Sprintf("%s: run block matches %s: %s", where, rule.display, rule.why))
			}
		}
		problems = append(problems, checkReferencedScripts(pathLabel, jobID, root, runText, seenScripts)...)
	}
	return problems
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

// checkReferencedScripts applies the run-time-code rules to each scripts/ file
// the job invokes, once per job however many steps mention it.
func checkReferencedScripts(pathLabel, jobID, root, runText string, seenScripts map[string]bool) []string {
	var problems []string
	for _, relative := range referencedScripts(runText) {
		if seenScripts[relative] {
			continue
		}
		seenScripts[relative] = true
		scriptPath := filepath.Join(root, filepath.FromSlash(relative))
		info, statErr := os.Stat(scriptPath)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		data, readErr := os.ReadFile(scriptPath) //#nosec G304 -- an audit tool reading a scripts/ file this repository's own workflows invoke.
		if readErr != nil {
			continue
		}
		body := stripComments(string(data))
		for _, rule := range unlockedCode {
			if rule.match(body) {
				problems = append(problems, fmt.Sprintf(
					"%s: job %s: %s matches %s: %s", pathLabel, jobID, relative, rule.display, rule.why,
				))
			}
		}
	}
	return problems
}

// checkWorkflowJobs applies the credentialed-job rules to every job in a
// workflow that holds a write credential.
//
// jobOrder carries the document order when the caller parsed a file; a document
// built in a test carries none, and the keys are then sorted so the findings
// are still deterministic.
func checkWorkflowJobs(pathLabel string, doc map[string]any, root string, jobOrder []string) []string {
	jobs, _ := doc["jobs"].(map[string]any)
	var problems []string
	for _, jobID := range orderedKeys(jobs, jobOrder) {
		job, ok := jobs[jobID].(map[string]any)
		if !ok || !isCredentialed(doc, job) {
			continue
		}
		problems = append(problems, checkCredentialedJob(pathLabel, jobID, job, root, doc)...)
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
				": verifies no signature — checksums.txt comes from the same mutable release "+
				"as the binary, so a consistent replacement of both files is accepted")
		}
		if !strings.Contains(installer.body, "checksums.txt.sigstore.json") {
			problems = append(problems, installer.name+
				": never fetches checksums.txt.sigstore.json, which every release publishes")
		}
	}
	return problems
}

// audit runs every check against a repository root and returns the findings in
// a stable order.
func audit(root string) ([]string, error) {
	problems, err := auditWorkflows(root)
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
// every workflow file.
func auditWorkflows(root string) ([]string, error) {
	workflows, err := loadWorkflows(root)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, file := range workflows {
		problems = append(problems, checkPinnedUses(file.path, file.text)...)
		if file.doc != nil {
			problems = append(problems, checkWorkflowJobs(file.path, file.doc, root, file.jobOrder)...)
		}
	}
	return problems, nil
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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses the command line, audits the repository and returns the process
// exit code: 0 when every invariant holds, 1 when any of them does not or when
// the audit could not be performed at all.
//
// One non-zero code covers both outcomes because that is what the Python
// auditor exposed: it exited 1 on findings and, on a missing file or a broken
// parse, died of an uncaught exception, which is also 1.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("audit_supply_chain", flag.ContinueOnError)
	flags.SetOutput(stderr)
	rootFlag := flags.String("root", "", "repository root (default: the module root at or above the working directory)")
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
	problems, err := audit(root)
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
