package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcewalk"
)

// replacesPrefix starts the comment line a new test carries to name the old
// tests it replaces: "// Replaces: TestMeta_Issues, TestIndividual_Issues".
const replacesPrefix = "Replaces:"

// dropDeclaration records why an old Test function has no successor.
type dropDeclaration struct {
	// Category says what kind of drop this is.
	Category string
	// Reason says why, in the words a reviewer needs.
	Reason string
}

// Drop categories.
const (
	// dropCoveredElsewhere is a test whose subject another module already
	// covers on the wire: the transport modules under test/e2e/http and
	// test/e2e/stdio.
	dropCoveredElsewhere = "covered-elsewhere"
	// dropCopiedProduction is a test that reproduced production wiring in
	// the test process and so tested its copy, which the binary-driven
	// suite has no equivalent of by construction.
	dropCopiedProduction = "copied-production"
	// dropSuperseded is a test whose subject the rebuild makes moot.
	dropSuperseded = "superseded"
)

// declaredDrops holds every old Test function that no new test replaces,
// each with a category and a reason.
//
// It fills as the port proceeds. A drop naming a test the old suite does
// not have is a finding, and so is one naming a test that a Replaces line
// also claims: the two are different answers to where a scenario went, and
// both cannot be true.
var declaredDrops = map[string]dropDeclaration{
	// The old suite's own baseline recorder, instrumented in S09 to record
	// the baseline the port is compared with. Its stack-walking attribution,
	// its source index of subtest literals, its outcome classifier and its
	// argument reader exist only because that suite drove an in-process
	// transport where no test frame was on the handler's stack; the harness
	// attributes every call on the caller's goroutine and asserts the
	// dispatched route on every call at flush, and its recorder is tested in
	// test/e2e/internal/harness against the real binary. The one test of
	// the ten that reached GitLab, the join of user.current, is replaced
	// rather than dropped.
	"TestBaseline_Attribution_NamesTheRunningSubtest":                  baselineRecorderDrop,
	"TestBaseline_Attribution_ReadsTheContextFirst":                    baselineRecorderDrop,
	"TestBaseline_SubtestIndex_ParsesThisFile":                         baselineRecorderDrop,
	"TestBaseline_DeclaredFunctionName_StripsWhatTheRuntimeAppends":    baselineRecorderDrop,
	"TestBaseline_ParseGoroutineDump_ReadsFramesAndCreators":           baselineRecorderDrop,
	"TestBaseline_MergeNameParts_DropsTheSharedLiteral":                baselineRecorderDrop,
	"TestBaseline_RewriteSubtestName_SpellsNamesLikeTheTestingPackage": baselineRecorderDrop,
	"TestBaseline_Outcome_ClassifiesLikeTheHarness":                    baselineRecorderDrop,
	"TestBaseline_Arguments_NamesTypedAndUntypedInputs":                baselineRecorderDrop,

	// S17 B2: users, access, tokens and to-dos.
	// The one test of the batch that reached no GitLab: it read tools/list
	// off the in-process individual session and checked six user management
	// tools for their required fields, their destructive hint and whether
	// they carry a confirm property. That is the catalog's projection of a
	// tool schema, which the golden snapshots under internal/tools pin for
	// every tool on every surface, and the harness's served-set check holds
	// the binary's own tools/list to the assemblers on every session start.
	// Nothing in it was about GitLab, and the user actions those tools
	// project are driven end to end by the useradmin and useraccount tests.
	"TestIndividual_UserManagementCatalogProjection": {
		Category: dropCoveredElsewhere,
		Reason: "a check of six user tools' projected schemas and annotations read off an in-process tools/list; " +
			"the golden snapshot tests of internal/tools pin every tool's schema and annotations, and the harness's " +
			"served-set check compares the binary's tools/list with the assemblers on every session start, so the " +
			"projection is held in both places and the suite keeps only the calls that reach GitLab",
	},
	// S17 B4: repository, CI, environments, releases and packages.
	"TestMeta_EnvironmentsProtected": {
		Category: dropSuperseded,
		Reason: "returned before its first call on an unlicensed runtime, so the CE baseline holds nothing of it; " +
			"its licensed branch (environment.protected_list, protected_protect, protected_get and protected_unprotect) " +
			"is the scenario test/e2e/gitlab/ee/protectedenvs_test.go drives on every surface of a licensed runtime",
	},
	// S17 B6: the old suite's own infrastructure tests and the MCP-level cases
	// the plan resolves rather than ports. The suite's own helper tests are
	// superseded by the harness and fixture that replaced those helpers; the
	// HTTP-transport cases are covered on the wire by test/e2e/http, which the
	// stdio harness cannot reach; the in-process capability tests are replaced
	// by binary-driven ports (mcp_dynamic_find, mcp_annotations, mcp_schema,
	// mcp_manifest, mcp_confirm_guard, mcp_elicitation, mcp_wait) or dropped to
	// a sweep that already proves them.
	"TestPoll_ImmediateSuccess":                                b6PollDrop,
	"TestPoll_RetrySuccess":                                    b6PollDrop,
	"TestPoll_ReturnsContextCancellation":                      b6PollDrop,
	"TestPoll_ReturnsTimeoutWithLastState":                     b6PollDrop,
	"TestPoll_ReturnsConditionError":                           b6PollDrop,
	"TestRetryWithBackoffInterval_RetrySuccess":                b6PollDrop,
	"TestRetryWithBackoffInterval_ReturnsNonRetryableError":    b6PollDrop,
	"TestRetryWithBackoffInterval_RespectsContextCancellation": b6PollDrop,
	"TestShortStableHash_ReturnsStableLowercaseHex":            b6NamesDrop,
	"TestSanitizeTestName_ConvertsGoTestNameToSlug":            b6NamesDrop,
	"TestSanitizeTestName_TruncatesToFortyCharacters":          b6NamesDrop,
	"TestNewE2ERunID_UsesUTCStampAndHashSuffix":                b6NamesDrop,
	"TestConfiguredE2ERunID_UsesEnvironmentOverride":           b6NamesDrop,
	"TestUniqueName_IncludesRunIDHashAndCounter":               b6NamesDrop,
	"TestUniqueName_UsesDefaultPrefixForEmptyInput":            b6NamesDrop,
	"TestResourceLedger_CleansInReverseRegistrationOrder":      b6LedgerDrop,
	"TestResourceLedger_RecordsReturnsCopy":                    b6LedgerDrop,
	"TestResourceLedger_RegisterIsConcurrentSafe":              b6LedgerDrop,
	"TestResourceLedger_CleanupAllReportsFailures":             b6LedgerDrop,
	"TestResourceLedger_CleanupAllIsIdempotent":                b6LedgerDrop,
	"TestResourceLedger_RegisterAfterCleanupReturnsError":      b6LedgerDrop,
	"TestMain":               b6MainDrop,
	"TestGitLabURLHeaderE2E": b6HTTPHeaderDrop,
	"TestHTTPStatelessBinary_FullFlow_NoSessionTracking": b6HTTPStatelessDrop,
	"TestOAuthE2E":               b6OAuthDrop,
	"TestIdentityE2E":            b6IdentityDrop,
	"TestCapability_Progress":    b6ProgressDrop,
	"TestCapability_Completions": b6CompletionsDrop,
	"TestResources_ReadAll":      b6ResourcesReadDrop,
}

// baselineRecorderDrop is the one reason the old suite's recorder tests
// share: they test the instrument that produced the baseline, which goes
// with the suite it instrumented.
var baselineRecorderDrop = dropDeclaration{
	Category: dropSuperseded,
	Reason: "a unit test of the old suite's baseline recorder, whose stack-walking attribution and " +
		"in-process dispatch join the harness's own recorder replaces (test/e2e/internal/harness/record.go " +
		"and otlp.go, tested there against the real binary); the recorder is deleted with the suite it instrumented",
}

// The reasons the S17 B6 drops share, one per superseding successor. B6 holds
// the old suite's own infrastructure tests and the MCP-level cases that either
// drove a server the suite assembled in its own process, or exercise a fact a
// transport module already proves against the real binary.
var (
	b6PollDrop = dropDeclaration{
		Category: dropSuperseded,
		Reason: "a unit test of the old suite's own Poll and RetryWithBackoffInterval helpers, which the " +
			"harness replaced with test/e2e/internal/harness/poll.go, tested there by TestPoll_* and TestRetry_*",
	}
	b6NamesDrop = dropDeclaration{
		Category: dropSuperseded,
		Reason: "a unit test of the old suite's own run-id and name helpers, which the harness replaced with " +
			"test/e2e/internal/harness/names.go, tested there by TestShortStableHash_*, TestSanitizeTestName_*, " +
			"TestNewRunID_*, TestConfiguredRunID_* and TestUniqueName_*",
	}
	b6LedgerDrop = dropDeclaration{
		Category: dropSuperseded,
		Reason: "a unit test of the old suite's own cleanup ledger, which the harness replaced with the ledger in " +
			"test/e2e/internal/harness/env.go, tested there by TestLedger_*",
	}
	b6MainDrop = dropDeclaration{
		Category: dropSuperseded,
		Reason: "the old suite's TestMain, which assembled the in-process sessions the whole suite drove; the " +
			"harness starts the real binary from harness.Main, called by each runtime package's own main_test.go",
	}
	b6HTTPHeaderDrop = dropDeclaration{
		Category: dropCoveredElsewhere,
		Reason: "reproduced the cmd/server GITLAB-URL selector closure over httptest servers in the test process; " +
			"the real binary's per-request instance selection and pool keying are driven on the wire by " +
			"test/e2e/http (TestGate_PublishedInstances_HeaderCannotRedirectTheCredential, " +
			"TestGate_SinglePublishedInstance_IgnoresTheHeader, TestGate_MalformedGitLabURLHeader_IsRejectedWithDetail, " +
			"TestClient_DistinctCredentialsGetDistinctServers)",
	}
	b6HTTPStatelessDrop = dropDeclaration{
		Category: dropCoveredElsewhere,
		Reason: "drove a hand-built binary over sessionless JSON-RPC POSTs; the harness starts the binary over stdio " +
			"only, and the stateless HTTP flow it tested is driven against the real binary by test/e2e/http " +
			"(TestClient_JSONResponseMode, TestGate_StatefulServesEveryRevisionExceptTheStatelessOne, " +
			"TestGate_NonPostMethodsReachTheSDK)",
	}
	b6OAuthDrop = dropDeclaration{
		Category: dropCoveredElsewhere,
		Reason: "reassembled the OAuth bearer middleware and RFC 9728 metadata handler in the test process; the real " +
			"binary's OAuth admission, metadata and identity propagation are driven on the wire by test/e2e/http " +
			"(the TestOAuth_* family and TestCollectorSurfaces_OAuthModeStillRecords)",
	}
	b6IdentityDrop = dropDeclaration{
		Category: dropCopiedProduction,
		Reason: "built an in-memory server and mock GitLab to exercise toolutil.IdentityToContext/ResolveIdentity and " +
			"the oauth caching verifier, which are unit-tested in internal/toolutil and internal/oauth; HTTP OAuth " +
			"identity against the real binary is covered by test/e2e/http (TestCollectorSurfaces_OAuthModeStillRecords)",
	}
	b6ProgressDrop = dropDeclaration{
		Category: dropCopiedProduction,
		Reason: "built an in-process server matching cmd/server to observe a progress notification, which the harness " +
			"client does not wire; the progress tracker is unit-tested in internal/progress and " +
			"internal/tools/uploads (TestProjectUpload_WithProgressToken), and progress is not a coverage capability",
	}
	b6CompletionsDrop = dropDeclaration{
		Category: dropSuperseded,
		Reason: "drove completions against an in-process server; the harness completion verbs drive the real binary and " +
			"TestCompletions_Sweep in test/e2e/gitlab/common walks every served completion reference and argument",
	}
	b6ResourcesReadDrop = dropDeclaration{
		Category: dropSuperseded,
		Reason: "read every registered resource URI against the in-process individual session; TestResources_Sweep in " +
			"test/e2e/gitlab/common reads every static resource and every bindable template off the real binary, and " +
			"its fixture-building calls (project snippet, board, deploy key and the group objects) are covered by the " +
			"snippets, boards and group family ports",
	}
)

// declaredDropCategories is the set a category must belong to.
var declaredDropCategories = map[string]bool{
	dropCoveredElsewhere: true,
	dropCopiedProduction: true,
	dropSuperseded:       true,
}

// retiredTests lists the old Test functions whose files are gone: the 74 of
// the old suite's EE half, deleted with the build constraint that used to
// select it once every one of them had a Replaces successor under
// test/e2e/gitlab/ee.
//
// The port map reads the old suite's Test functions from its files, and a
// deleted file declares nothing, so without this list every Replaces line
// naming one of these would read as naming a test the old suite never had,
// and the map could no longer say that the EE half was ported. Each is held
// to the rule a live one is: replaced or dropped, or unresolved. One found
// declared in the old suite is a finding, since retiring it is the claim
// that its file is gone; the list goes with the old suite when the suite
// goes.
var retiredTests = []string{
	"TestEE_MetaGroupEnterpriseOperations",
	"TestEE_MetaRunnerManagement",
	"TestEE_MetaUserServiceAccounts",
	"TestEnterpriseOrbit_NotRegisteredOnSelfManaged",
	"TestGroupDatadogIntegration",
	"TestIndividual_GroupCreateUltimateFields",
	"TestIndividual_MRDependenciesList",
	"TestIndividual_PushRules",
	"TestIndividual_Vulnerabilities",
	"TestMeta_AdminLicenseLifecycle",
	"TestMeta_Attestations",
	"TestMeta_AuditEventListActions",
	"TestMeta_AuditEvents",
	"TestMeta_CompliancePolicy",
	"TestMeta_DORAMetrics",
	"TestMeta_Dependencies",
	"TestMeta_DeploymentApproveOrReject",
	"TestMeta_EnterpriseUsers",
	"TestMeta_EpicBoards",
	"TestMeta_EpicDiscussions",
	"TestMeta_EpicIssues",
	"TestMeta_EpicLinks",
	"TestMeta_EpicNotes",
	"TestMeta_Epics",
	"TestMeta_ExternalStatusChecks",
	"TestMeta_Geo",
	"TestMeta_GroupBillableMembers",
	"TestMeta_GroupBoardListColumns",
	"TestMeta_GroupEpicBoards",
	"TestMeta_GroupEpicLabelEvents",
	"TestMeta_GroupHookExtras",
	"TestMeta_GroupIterations",
	"TestMeta_GroupLDAPLinks",
	"TestMeta_GroupLDAPSync",
	"TestMeta_GroupLabelArchive",
	"TestMeta_GroupMRApprovalSettings",
	"TestMeta_GroupProtectedBranchesEE",
	"TestMeta_GroupProtectedEnvironmentsEE",
	"TestMeta_GroupProvisionedUsers",
	"TestMeta_GroupPushRules",
	"TestMeta_GroupSAML",
	"TestMeta_GroupSAMLUsers",
	"TestMeta_GroupSCIM",
	"TestMeta_GroupSSHCerts",
	"TestMeta_GroupServiceAccounts",
	"TestMeta_GroupStorageMoves_Graceful404",
	"TestMeta_GroupWikis",
	"TestMeta_IssueWeightEvents",
	"TestMeta_IssueWorkItems",
	"TestMeta_MRApprovalSettings",
	"TestMeta_MRBlockingDependencies",
	"TestMeta_MemberRoles",
	"TestMeta_MergeTrainGet",
	"TestMeta_MergeTrains",
	"TestMeta_ProjectAliases",
	"TestMeta_ProjectIterations",
	"TestMeta_ProjectMirroring",
	"TestMeta_ProjectSecuritySettings",
	"TestMeta_ProjectServiceAccounts",
	"TestMeta_ProjectStorageMoves_Graceful404",
	"TestMeta_ProtectedEnvUpdate",
	"TestMeta_ProtectedEnvs",
	"TestMeta_PushRules",
	"TestMeta_RunnerControllerLifecycle",
	"TestMeta_SecurityAttributes",
	"TestMeta_SecurityCategories",
	"TestMeta_SecurityClassifications",
	"TestMeta_SecurityFindings",
	"TestMeta_SecurityScanProfiles",
	"TestMeta_SnippetStorageMoves_Graceful404",
	"TestMeta_StorageMoves",
	"TestMeta_TargetBranchRules",
	"TestMeta_Vulnerabilities",
	"TestMeta_VulnerabilityLifecycle",
}

// portMap is the resolution of every old Test function.
type portMap struct {
	// Old lists every Test function of the old suite, sorted: the ones its
	// files declare and the retired ones.
	Old []string `json:"old"`
	// Retired lists the old tests on the map by declaration rather than by
	// file, sorted.
	Retired []string `json:"retired"`
	// Replaced maps an old test to the new tests that name it.
	Replaced map[string][]string `json:"replaced"`
	// Dropped maps an old test to its drop declaration.
	Dropped map[string]dropDeclaration `json:"dropped"`
	// Unresolved lists the old tests neither replaced nor dropped.
	Unresolved []string `json:"unresolved"`
	// Findings lists what is wrong with the map itself: a Replaces line or a
	// drop naming a test the old suite lacks, a test both replaced and
	// dropped, a drop with an unknown category.
	Findings []string `json:"findings"`
}

// complete reports whether every old test is resolved and nothing about the
// map is wrong.
func (m *portMap) complete() bool {
	return len(m.Unresolved) == 0 && len(m.Findings) == 0
}

// buildPortMap reads the old suite's Test functions and the new suite's
// Replaces lines, and resolves each old test against the retired list and
// the declared drops it is given.
func buildPortMap(oldDir, newDir string, retired []string, drops map[string]dropDeclaration) (*portMap, error) {
	oldTests, err := testFunctions(oldDir)
	if err != nil {
		return nil, fmt.Errorf("old suite: %w", err)
	}
	if len(oldTests) == 0 {
		return nil, fmt.Errorf("old suite: no Test function under %s", oldDir)
	}
	replaces, err := replacesLines(newDir)
	if err != nil {
		return nil, fmt.Errorf("new suite: %w", err)
	}
	return resolvePortMap(oldTests, retired, replaces, drops), nil
}

// resolvePortMap is [buildPortMap] once the two suites have been read, so a
// test can hand it lists instead of directories.
//
// declared are the Test functions the old suite's files hold and retired the
// ones on the map by declaration alone; together they are the old tests. A
// retired test the files still declare is a finding, and a name retired
// twice is one too.
func resolvePortMap(declared, retired []string, replaces map[string][]string, drops map[string]dropDeclaration) *portMap {
	known := map[string]bool{}
	for _, name := range declared {
		known[name] = true
	}
	m := &portMap{Replaced: map[string][]string{}, Dropped: map[string]dropDeclaration{}}
	oldTests := slices.Clone(declared)
	seenRetired := map[string]bool{}
	for _, name := range retired {
		switch {
		case seenRetired[name]:
			m.Findings = append(m.Findings, name+" is retired twice")
		case known[name]:
			m.Findings = append(m.Findings, name+" is retired and still declared in the old suite")
		default:
			known[name] = true
			seenRetired[name] = true
			oldTests = append(oldTests, name)
			m.Retired = append(m.Retired, name)
		}
	}
	sort.Strings(oldTests)
	sort.Strings(m.Retired)
	m.Old = oldTests
	for newTest, olds := range replaces {
		for _, old := range olds {
			if !known[old] {
				m.Findings = append(m.Findings, fmt.Sprintf("%s replaces %s, which the old suite does not have", newTest, old))
				continue
			}
			m.Replaced[old] = append(m.Replaced[old], newTest)
		}
	}
	for old, replacements := range m.Replaced {
		sort.Strings(replacements)
		m.Replaced[old] = replacements
	}
	for old, drop := range drops {
		m.noteDrop(old, drop, known)
	}
	for _, name := range oldTests {
		if _, replaced := m.Replaced[name]; replaced {
			continue
		}
		if _, dropped := m.Dropped[name]; dropped {
			continue
		}
		m.Unresolved = append(m.Unresolved, name)
	}
	sort.Strings(m.Unresolved)
	sort.Strings(m.Findings)
	return m
}

// noteDrop records one declared drop, or what is wrong with it.
func (m *portMap) noteDrop(old string, drop dropDeclaration, known map[string]bool) {
	switch {
	case !known[old]:
		m.Findings = append(m.Findings, fmt.Sprintf("drop declared for %s, which the old suite does not have", old))
	case !declaredDropCategories[drop.Category]:
		m.Findings = append(m.Findings, fmt.Sprintf("drop declared for %s under the unknown category %q", old, drop.Category))
	case len(m.Replaced[old]) > 0:
		m.Findings = append(m.Findings, fmt.Sprintf("%s is both dropped and replaced by %s", old, strings.Join(m.Replaced[old], ", ")))
	default:
		m.Dropped[old] = drop
	}
}

// testFunctions lists the Test functions declared in the _test.go files
// directly under dir, sorted.
//
// It does not descend: the old suite is one flat package, and a Test function
// one directory down would belong to another package the map is not about.
func testFunctions(dir string) ([]string, error) {
	var names []string
	err := walkTestFiles(dir, false, func(file *ast.File) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Recv == nil && isTestFunc(fn.Name.Name) {
				names = append(names, fn.Name.Name)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// isTestFunc reports whether a function name is one the test binary runs as
// a test, TestMain included, since the old suite's TestMain is on the port
// map too.
//
// The rule is go test's own: Test, followed by nothing or by a character
// that is not a lowercase letter. Testhelper is a helper and not a test, and
// a prefix check alone would put it on the port map, credit its ids to a
// test nothing runs, and let a Replaces line on it retire a real one.
func isTestFunc(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	rest := strings.TrimPrefix(name, "Test")
	if rest == "" {
		return true
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(first)
}

// replacesLines reads every Replaces line off the Test functions under dir,
// recursively, as a map from the new test to the old tests it names.
//
// A directory that does not exist yields an empty map rather than an error:
// before the new suite is created every old test is unresolved, which is
// the truthful answer, and not a broken command.
func replacesLines(dir string) (map[string][]string, error) {
	replaces := map[string][]string{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return replaces, nil
	}
	err := walkTestFiles(dir, true, func(file *ast.File) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil || !isTestFunc(fn.Name.Name) || fn.Doc == nil {
				continue
			}
			if olds := parseReplaces(fn.Doc); len(olds) > 0 {
				replaces[fn.Name.Name] = append(replaces[fn.Name.Name], olds...)
			}
		}
	})
	return replaces, err
}

// parseReplaces reads the old test names off a doc comment's Replaces lines.
func parseReplaces(doc *ast.CommentGroup) []string {
	var olds []string
	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		if !strings.HasPrefix(text, replacesPrefix) {
			continue
		}
		for name := range strings.SplitSeq(strings.TrimPrefix(text, replacesPrefix), ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// A subtest reference names its parent's function.
			olds = append(olds, topLevelTest(name))
		}
	}
	return olds
}

// walkTestFiles parses every _test.go file under dir and hands each to visit.
func walkTestFiles(dir string, recursive bool, visit func(*ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", dir, err)
		}
		if entry.IsDir() {
			// A recursive walk still stops at anything that is not this
			// repository's source: a nested worktree under the suite would
			// otherwise contribute its own copy of every Test function.
			if path != dir && (!recursive || sourcewalk.SkipDirBelowRoot(path)) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		visit(file)
		return nil
	})
}
