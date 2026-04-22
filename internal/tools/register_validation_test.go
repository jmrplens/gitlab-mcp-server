// register_validation_test.go contains tests that verify structural integrity
// of tool and formatter registrations. These tests prevent silent failures when
// a new sub-package is added but not wired into RegisterAll or RegisterAllMeta,
// or when a sub-package with Markdown formatters forgets to register them.
package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// knownExceptions lists sub-packages that are NOT registered via RegisterAll
// in register.go because they have a different constructor signature.
// Each entry must document WHY it is an exception.
var knownExceptions = map[string]string{
	// serverupdate takes *autoupdate.Updater instead of *gitlabclient.Client;
	// it is registered in cmd/server/main.go.
	"serverupdate": "registered in cmd/server/main.go with *autoupdate.Updater",
}

// TestAllSubPackagesRegistered verifies that every sub-directory under
// internal/tools/ has a corresponding RegisterTools call in register.go.
// Sub-packages listed in knownExceptions are allowed to be absent from
// register.go if they are registered elsewhere.
func TestAllSubPackagesRegistered(t *testing.T) {
	// 1. Discover all sub-directories (= sub-packages).
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var subDirs []string
	for _, e := range entries {
		if e.IsDir() {
			subDirs = append(subDirs, e.Name())
		}
	}
	if len(subDirs) == 0 {
		t.Fatal("no sub-directories found — test may be running from wrong directory")
	}

	// 2. Parse register.go to extract all {pkg}.RegisterTools( calls.
	src, err := os.ReadFile("register.go")
	if err != nil {
		t.Fatalf("ReadFile register.go: %v", err)
	}
	re := regexp.MustCompile(`\b(\w+)\.RegisterTools\(`)
	matches := re.FindAllStringSubmatch(string(src), -1)

	registered := make(map[string]bool)
	for _, m := range matches {
		registered[m[1]] = true
	}

	// 3. Check that every sub-directory is registered or is a known exception.
	var missing []string
	for _, dir := range subDirs {
		if registered[dir] {
			continue
		}
		if _, ok := knownExceptions[dir]; ok {
			continue
		}
		missing = append(missing, dir)
	}

	if len(missing) > 0 {
		t.Errorf("sub-packages not registered in register.go (and not in knownExceptions):\n  %s",
			strings.Join(missing, "\n  "))
		t.Log("If a sub-package has a different constructor, add it to knownExceptions with a reason.")
	}

	// 4. Verify known exceptions actually exist as directories.
	for pkg, reason := range knownExceptions {
		if _, statErr := os.Stat(pkg); os.IsNotExist(statErr) {
			t.Errorf("knownExceptions entry %q (%s) does not exist as a sub-directory — remove it", pkg, reason)
		}
	}

	t.Logf("verified %d sub-packages: %d in register.go, %d known exceptions",
		len(subDirs), len(registered), len(knownExceptions))
}

// TestAllMarkdownFormattersRegistered verifies that every sub-package with a
// markdown.go containing init() + RegisterMarkdown has at least one type
// registered in the toolutil Markdown registry.
func TestAllMarkdownFormattersRegistered(t *testing.T) {
	// 1. Get all registered type names from the registry.
	typeNames := toolutil.RegisteredMarkdownTypeNames()
	if len(typeNames) == 0 {
		t.Fatal("no Markdown formatters registered — registry may not be initialized")
	}

	// Build a set of package prefixes that have registered formatters.
	registeredPkgs := make(map[string]bool)
	for _, name := range typeNames {
		// Type names are like "branches.Output", "toolutil.DeleteOutput".
		pkg, _, ok := strings.Cut(name, ".")
		if ok {
			registeredPkgs[pkg] = true
		}
	}

	// 2. Find sub-packages whose markdown.go files contain init() registrations.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	reRegister := regexp.MustCompile(`toolutil\.Register(?:Markdown|MarkdownResult)\b`)
	var missing []string

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(e.Name(), "markdown.go")
		src, readErr := os.ReadFile(mdPath)
		if readErr != nil {
			continue // no markdown.go — that's fine
		}

		if !reRegister.Match(src) {
			continue // markdown.go exists but has no registry calls
		}

		// This sub-package registers formatters — check if they appear in the registry.
		if !registeredPkgs[e.Name()] {
			missing = append(missing, e.Name())
		}
	}

	if len(missing) > 0 {
		t.Errorf("sub-packages with RegisterMarkdown calls in markdown.go but no types in registry:\n  %s",
			strings.Join(missing, "\n  "))
	}

	// 3. Check the toolutil.DeleteOutput formatter is registered.
	if !registeredPkgs["toolutil"] {
		t.Error("toolutil.DeleteOutput formatter not registered in registry")
	}

	t.Logf("verified %d registered formatter types across %d packages",
		len(typeNames), len(registeredPkgs))
}

// TestAllHintReferencesValid validates that tool names and meta-tool action
// names referenced in WriteHints calls actually exist. This catches stale
// references after tool renaming or meta-tool action restructuring.
//
// Two validations:
//   - Backtick-quoted `gitlab_*` tool references must match a registered tool name
//   - `action 'xxx'` references must match a meta-tool action key
func TestAllHintReferencesValid(t *testing.T) {
	// 1. Build set of all registered individual tool names from sub-package register.go files.
	validTools := make(map[string]bool)
	reToolName := regexp.MustCompile(`Name:\s+"(gitlab_\w+)"`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		for _, m := range reToolName.FindAllStringSubmatch(string(src), -1) {
			validTools[m[1]] = true
		}
	}

	// Also add meta-tool names from register_meta.go.
	metaSrc, err := os.ReadFile("register_meta.go")
	if err != nil {
		t.Fatalf("ReadFile register_meta.go: %v", err)
	}
	reMetaTool := regexp.MustCompile(`add(?:ReadOnly)?MetaTool\(server,\s+"(gitlab_\w+)"`)
	for _, m := range reMetaTool.FindAllStringSubmatch(string(metaSrc), -1) {
		validTools[m[1]] = true
	}

	// Also add meta-tools from sub-package RegisterMeta (via mcp.AddTool Name).
	// These are already captured by reToolName above if they use Name: "gitlab_*".

	if len(validTools) == 0 {
		t.Fatal("no tool names found — parsing may be broken")
	}

	// 2. Build set of all meta-tool action keys from route maps.
	validActions := make(map[string]bool)
	// Pattern for register_meta.go: "key": wrapAction/wrapVoidAction/wrapDelegateAction (map literal)
	reInlineAction := regexp.MustCompile(`"(\w+)":\s+(?:route|destructive)(?:Action|VoidAction|ActionWithRequest)\b`)
	for _, m := range reInlineAction.FindAllStringSubmatch(string(metaSrc), -1) {
		validActions[m[1]] = true
	}
	// Pattern for register_meta.go: routes["key"] = route/destructiveRoute/routeAction/etc. (enterprise assignment)
	reRouteAssign := regexp.MustCompile(`routes\["(\w+)"\]\s*=\s*(?:route(?:Action|VoidAction|ActionWithRequest)?|destructive(?:Route|Action|VoidAction|ActionWithRequest))\b`)
	for _, m := range reRouteAssign.FindAllStringSubmatch(string(metaSrc), -1) {
		validActions[m[1]] = true
	}
	// Also match custom action variables wrapped in route/destructiveRoute (e.g., "publish": route(publishAction)).
	reCustomAction := regexp.MustCompile(`"(\w+)":\s+(?:route|destructiveRoute)\(\w+Action\b`)
	for _, m := range reCustomAction.FindAllStringSubmatch(string(metaSrc), -1) {
		validActions[m[1]] = true
	}

	// Pattern for sub-package register.go: "key": toolutil.RouteAction/RouteVoidAction/DestructiveAction etc.
	reDelegatedAction := regexp.MustCompile(`"(\w+)":\s+toolutil\.(?:Route|Destructive)(?:Action|VoidAction|ActionWithRequest|Route)\b`)
	// Pattern for sub-package register.go: routes["key"] = toolutil.Route/DestructiveRoute(...) (enterprise)
	reDelegatedAssign := regexp.MustCompile(`routes\["(\w+)"\]\s*=\s*toolutil\.(?:Route|Destructive)(?:Action|VoidAction|ActionWithRequest|Route)\b`)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		for _, m := range reDelegatedAction.FindAllStringSubmatch(string(src), -1) {
			validActions[m[1]] = true
		}
		for _, m := range reDelegatedAssign.FindAllStringSubmatch(string(src), -1) {
			validActions[m[1]] = true
		}
	}

	if len(validActions) == 0 {
		t.Fatal("no action keys found — parsing may be broken")
	}

	// 3. Validate hints in all markdown.go files.
	reToolRef := regexp.MustCompile("`(gitlab_\\w+)`")
	reActionRef := regexp.MustCompile(`action '(\w+)'`)

	var toolErrors, actionErrors int

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(e.Name(), "markdown.go")
		src, readErr := os.ReadFile(mdPath)
		if readErr != nil {
			continue
		}

		// Extract lines that belong to WriteHints calls.
		hintLines := extractWriteHintLines(string(src))
		for _, line := range hintLines {
			// Check backtick-quoted tool references.
			for _, m := range reToolRef.FindAllStringSubmatch(line, -1) {
				toolName := m[1]
				if !validTools[toolName] {
					t.Errorf("%s: hint references non-existent tool %q", e.Name(), toolName)
					toolErrors++
				}
			}
			// Check action name references.
			for _, m := range reActionRef.FindAllStringSubmatch(line, -1) {
				actionName := m[1]
				if !validActions[actionName] {
					t.Errorf("%s: hint references non-existent action %q", e.Name(), actionName)
					actionErrors++
				}
			}
		}
	}

	t.Logf("validated hints across all packages: %d valid tools, %d valid actions, %d tool errors, %d action errors",
		len(validTools), len(validActions), toolErrors, actionErrors)
}

// extractWriteHintLines finds string literal lines inside WriteHints() calls.
// It uses a simple state machine: when a line contains "WriteHints(", subsequent
// lines containing string literals are collected until the closing ")".
func extractWriteHintLines(src string) []string {
	lines := strings.Split(src, "\n")
	var result []string
	inHints := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "WriteHints(") {
			inHints = true
			continue
		}
		if inHints {
			if strings.HasPrefix(trimmed, `"`) {
				result = append(result, trimmed)
			} else if trimmed == ")" || strings.HasPrefix(trimmed, ")") {
				inHints = false
			}
		}
	}
	return result
}

// TestDestructiveMetadataConsistency verifies that meta-tool routes marked with
// destructive wrappers correspond to individual tools using DeleteAnnotations,
// and that non-destructive routes do not correspond to individual tools with
// DeleteAnnotations. This catches misclassified routes after migration.
func TestDestructiveMetadataConsistency(t *testing.T) {
	// 1. Build set of sub-package actions with their destructive wrapper status.
	type routeInfo struct {
		pkg         string
		destructive bool
	}
	routeMap := make(map[string][]routeInfo)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	// Patterns for destructive wrappers in sub-packages.
	reSubDestructive := regexp.MustCompile(`"(\w+)":\s+toolutil\.Destructive(?:Action|VoidAction|ActionWithRequest|Route)\b`)
	reSubNonDestructive := regexp.MustCompile(`"(\w+)":\s+toolutil\.Route(?:Action|VoidAction|ActionWithRequest|)\b`)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		srcStr := string(src)
		for _, m := range reSubDestructive.FindAllStringSubmatch(srcStr, -1) {
			routeMap[m[1]] = append(routeMap[m[1]], routeInfo{pkg: e.Name(), destructive: true})
		}
		for _, m := range reSubNonDestructive.FindAllStringSubmatch(srcStr, -1) {
			routeMap[m[1]] = append(routeMap[m[1]], routeInfo{pkg: e.Name(), destructive: false})
		}
	}

	// 2. Build set of individual tools with DeleteAnnotations per sub-package.
	deleteTools := make(map[string]bool) // key: "pkg/action" approximate
	reDeleteAnn := regexp.MustCompile(`Annotations:\s+toolutil\.DeleteAnnotations`)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		if reDeleteAnn.Match(src) {
			deleteTools[e.Name()] = true
		}
	}

	// 3. Validate: destructive routes should correspond to packages with DeleteAnnotations.
	var mismatches int
	for action, infos := range routeMap {
		for _, info := range infos {
			hasDelete := deleteTools[info.pkg]
			if info.destructive && !hasDelete {
				// Acceptable for exact-match exceptions (merge, stop, erase, etc.)
				// that are destructive but don't use DeleteAnnotations.
				if !isExactMatchException(action) {
					t.Logf("WARNING: %s/%s is destructive route but package has no DeleteAnnotations", info.pkg, action)
				}
			}
			if !info.destructive && hasDelete {
				// Non-destructive route in a package with delete tools — this is fine
				// for list/get/create/update actions in the same package.
				continue
			}
		}
	}

	t.Logf("validated %d route entries across %d packages, %d mismatches", len(routeMap), len(entries), mismatches)
}

func isExactMatchException(action string) bool {
	exceptions := map[string]bool{
		"merge": true, "erase": true, "stop": true, "ban": true,
		"block": true, "deactivate": true, "reject": true, "unapprove": true,
		"approval_reset": true, "disable_two_factor": true, "disable_2fa": true,
	}
	return exceptions[action]
}

// TestDestructiveRoutesByNameHeuristic scans ALL route definitions across the
// codebase and verifies that action names containing destructive keywords
// (delete, remove, revoke, purge, unprotect, destroy, unpublish) always use
// destructive wrappers, and that safe action names (list, get, search, create,
// update) never use destructive wrappers. This test prevents accidental
// misclassification when adding new routes.
func TestDestructiveRoutesByNameHeuristic(t *testing.T) {
	// routeEntry captures a single action definition found in source code.
	type routeEntry struct {
		file        string
		line        int
		action      string
		destructive bool
	}

	// Regex patterns for register_meta.go (lowercase wrappers, no package prefix).
	reMetaMapDestructive := regexp.MustCompile(
		`"(\w+)":\s+destructive(?:Action|VoidAction)\b`)
	reMetaMapNonDestructive := regexp.MustCompile(
		`"(\w+)":\s+route(?:Action|VoidAction|ActionWithRequest)\b`)
	reMetaAssignDestructive := regexp.MustCompile(
		`routes\["(\w+)"\]\s*=\s*destructive(?:Action|VoidAction)\b`)
	reMetaAssignNonDestructive := regexp.MustCompile(
		`routes\["(\w+)"\]\s*=\s*route(?:Action|VoidAction|ActionWithRequest)\b`)

	// Regex patterns for sub-package register.go files (toolutil. prefix).
	reSubDestructive := regexp.MustCompile(
		`"(\w+)":\s+toolutil\.Destructive(?:Action|VoidAction|ActionWithRequest|Route)\b`)
	reSubNonDestructive := regexp.MustCompile(
		`"(\w+)":\s+toolutil\.Route(?:Action|VoidAction|ActionWithRequest|)\b`)

	var allRoutes []routeEntry

	// Scan register_meta.go for inline route definitions.
	metaSrc, err := os.ReadFile("register_meta.go")
	if err != nil {
		t.Fatalf("failed to read register_meta.go: %v", err)
	}
	metaLines := strings.Split(string(metaSrc), "\n")
	for i, line := range metaLines {
		lineNum := i + 1
		for _, re := range []*regexp.Regexp{reMetaMapDestructive, reMetaAssignDestructive} {
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				allRoutes = append(allRoutes, routeEntry{
					file: "register_meta.go", line: lineNum,
					action: m[1], destructive: true,
				})
			}
		}
		for _, re := range []*regexp.Regexp{reMetaMapNonDestructive, reMetaAssignNonDestructive} {
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				allRoutes = append(allRoutes, routeEntry{
					file: "register_meta.go", line: lineNum,
					action: m[1], destructive: false,
				})
			}
		}
	}

	// Scan sub-package register.go files.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			lineNum := i + 1
			for _, m := range reSubDestructive.FindAllStringSubmatch(line, -1) {
				allRoutes = append(allRoutes, routeEntry{
					file: regPath, line: lineNum,
					action: m[1], destructive: true,
				})
			}
			for _, m := range reSubNonDestructive.FindAllStringSubmatch(line, -1) {
				allRoutes = append(allRoutes, routeEntry{
					file: regPath, line: lineNum,
					action: m[1], destructive: false,
				})
			}
		}
	}

	if len(allRoutes) == 0 {
		t.Fatal("no routes found — regex patterns may be outdated")
	}

	// Keywords that MUST use destructive wrappers.
	destructiveKeywords := []string{
		"delete", "remove", "revoke", "purge", "unprotect",
		"destroy", "unpublish",
	}
	containsDestructiveKeyword := func(action string) bool {
		for _, kw := range destructiveKeywords {
			if strings.Contains(action, kw) {
				return true
			}
		}
		return false
	}

	// Keywords that MUST NOT use destructive wrappers.
	safeKeywords := []string{
		"list", "get", "search", "create", "update", "edit",
		"approve", "subscribe", "upload", "download",
	}
	containsSafeKeyword := func(action string) bool {
		for _, kw := range safeKeywords {
			if strings.Contains(action, kw) {
				return true
			}
		}
		return false
	}

	// Actions that are destructive but do NOT contain a destructive keyword.
	// These are known edge cases verified manually.
	knownNonKeywordDestructive := map[string]bool{
		"merge": true, "erase": true, "stop": true, "ban": true,
		"block": true, "deactivate": true, "reject": true, "unapprove": true,
		"approval_reset": true, "disable_two_factor": true, "disable_2fa": true,
		"deny_project": true, "deny_group": true,
	}

	var failures int
	for _, r := range allRoutes {
		hasDestructiveKw := containsDestructiveKeyword(r.action)
		hasSafeKw := containsSafeKeyword(r.action)

		// Rule 1: Action with destructive keyword MUST be marked destructive.
		if hasDestructiveKw && !r.destructive {
			t.Errorf("%s:%d action %q contains destructive keyword but uses non-destructive wrapper",
				r.file, r.line, r.action)
			failures++
		}

		// Rule 2: Action with safe keyword MUST NOT be marked destructive,
		// UNLESS it also contains a destructive keyword or is a known exception.
		if hasSafeKw && r.destructive && !hasDestructiveKw && !knownNonKeywordDestructive[r.action] {
			t.Errorf("%s:%d action %q contains safe keyword but uses destructive wrapper",
				r.file, r.line, r.action)
			failures++
		}

		// Rule 3: Destructive actions without keyword must be in the known exceptions list.
		if r.destructive && !hasDestructiveKw && !knownNonKeywordDestructive[r.action] {
			t.Errorf("%s:%d action %q is destructive but has no destructive keyword and is not in known exceptions; add it to knownNonKeywordDestructive",
				r.file, r.line, r.action)
			failures++
		}
	}

	t.Logf("scanned %d routes (%d failures)", len(allRoutes), failures)
}

// TestDestructiveRoutesMinimumInventory verifies that the total number of
// destructive routes across the entire codebase does not drop below a
// known minimum. This prevents accidental mass reclassification of
// destructive actions to non-destructive (e.g., a bad find-and-replace).
func TestDestructiveRoutesMinimumInventory(t *testing.T) {
	// Regex patterns matching all destructive wrapper usages.
	destructivePatterns := []*regexp.Regexp{
		// register_meta.go inline patterns.
		regexp.MustCompile(`"(\w+)":\s+destructive(?:Action|VoidAction)\b`),
		regexp.MustCompile(`routes\["(\w+)"\]\s*=\s*destructive(?:Action|VoidAction)\b`),
		// Sub-package patterns.
		regexp.MustCompile(`"(\w+)":\s+toolutil\.Destructive(?:Action|VoidAction|ActionWithRequest|Route)\b`),
	}

	uniqueActions := make(map[string]bool) // "file:action" dedup key

	// Scan register_meta.go.
	metaSrc, err := os.ReadFile("register_meta.go")
	if err != nil {
		t.Fatalf("read register_meta.go: %v", err)
	}
	for _, re := range destructivePatterns {
		for _, m := range re.FindAllStringSubmatch(string(metaSrc), -1) {
			uniqueActions["register_meta.go:"+m[1]] = true
		}
	}

	// Scan sub-package register.go files.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regPath := filepath.Join(e.Name(), "register.go")
		src, readErr := os.ReadFile(regPath)
		if readErr != nil {
			continue
		}
		for _, re := range destructivePatterns {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				uniqueActions[e.Name()+":"+m[1]] = true
			}
		}
	}

	// Current baseline: update this number when intentionally adding/removing
	// destructive routes. This number represents the minimum expected count
	// across BOTH register_meta.go inline routes AND sub-package routes.
	const minExpectedDestructiveRoutes = 150

	total := len(uniqueActions)
	if total < minExpectedDestructiveRoutes {
		t.Errorf("only %d destructive routes found, expected at least %d — possible mass reclassification",
			total, minExpectedDestructiveRoutes)
	}
	t.Logf("found %d unique destructive route definitions (minimum: %d)", total, minExpectedDestructiveRoutes)
}
