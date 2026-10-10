package actiongrants

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestTable_IsTheSameTableOnEveryCall verifies every reader shares the one
// generated table rather than a copy, which is what lets each catalog action
// point into it for free.
func TestTable_IsTheSameTableOnEveryCall(t *testing.T) {
	first, second := Table(), Table()
	if first != second {
		t.Fatal("Table() answered two different tables")
	}
	if Table().Version == "" {
		t.Error("the generated table names no GitLab version; regenerate it with make gen-action-grants")
	}
}

// TestRequirement_AnswersByCanonicalID verifies a row is found by its action
// ID, as a pointer into the table, and that an ID with no row answers nil.
func TestRequirement_AnswersByCanonicalID(t *testing.T) {
	rows := Table().Actions
	if len(rows) == 0 {
		t.Fatal("the generated table holds no row; regenerate it with make gen-action-grants")
	}
	first := &rows[0]
	if got := Requirement(first.ID); got != first {
		t.Errorf("Requirement(%q) = %p, want the table's row %p", first.ID, got, first)
	}
	if got := Requirement("no.such_action"); got != nil {
		t.Errorf("Requirement(no.such_action) = %+v, want nil", got)
	}
}

// TestBuild_OnlyAFineGrainedTokenGetsAnAuthority verifies a classic credential
// is decided by nothing here, and that a fine-grained one is judged over the
// generated table: phase A with the reason when its grant could not be read,
// and phase B, at the version the table was recorded from, when it was.
func TestBuild_OnlyAFineGrainedTokenGetsAnAuthority(t *testing.T) {
	if got := Build(false, finegrained.Reading{Version: "19.4.1-ee"}); got != nil {
		t.Errorf("Build(false) = %+v, want nil for a classic credential", got)
	}
	unread := Build(true, finegrained.Reading{Fallback: finegrained.FallbackGrantUnreadable})
	if unread == nil || unread.Table() != Table() || unread.Phase() != finegrained.PhaseUnknown ||
		unread.Fallback() != finegrained.FallbackGrantUnreadable {
		t.Errorf("Build(true, unreadable) = %+v, want phase A over the generated table saying why", unread)
	}
	read := Build(true, finegrained.Reading{Grant: finegrained.Grant{Scopes: []finegrained.Scope{}}, Version: Table().Version})
	if read == nil || read.Table() != Table() || read.Phase() != finegrained.PhaseGranted || read.Reported() != Table().Version {
		t.Errorf("Build(true, read) = %+v, want phase B over the generated table at its own version", read)
	}
	if Build(true, finegrained.Reading{Fallback: finegrained.FallbackGrantUnreadable}) == unread {
		t.Error("Build returned the same authority twice; each credential gets its own")
	}
}

// initSymbol matches, in `go tool nm -size` output, the init functions this
// package compiles to: the one the compiler writes for package-level values
// it could not lay out as data, and any declared init function.
var initSymbol = regexp.MustCompile(`^\s*[0-9a-f]+\s+(\d+)\s+T\s+\S*/internal/tools/actiongrants\.init(\.\d+)?$`)

// TestTable_CompilesToDataWithNoInitWork verifies the generated table is laid
// out by the compiler rather than built when the server starts: a value it
// cannot place statically gets code in the package's init function, and every
// start would pay for building a table of more than a thousand rows.
//
// The package is compiled twice and the init functions compared: once as it
// stands, and once with the generator's stand-in, an empty table, in
// table_gen.go's place. The compiler writes a one-instruction init for any
// package importing one with init work of its own, so the empty table is the
// baseline, and a generated table adding nothing to it is one the compiler
// laid out whole. Compiling the stand-in is also what holds it to compiling
// against the current finegrained types, which nothing else does: the
// generator only ever reads it through an overlay.
func TestTable_CompilesToDataWithNoInitWork(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	stub, err := filepath.Abs(filepath.Join(dir, "..", "..", "..", "cmd", "gen_action_grants", "table_stub.go.txt"))
	if err != nil {
		t.Fatalf("stand-in path: %v", err)
	}
	overlay := filepath.Join(t.TempDir(), "overlay.json")
	body, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(dir, "table_gen.go"): stub}})
	if err != nil {
		t.Fatalf("encode overlay: %v", err)
	}
	if err = os.WriteFile(overlay, body, 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	generated := initBytes(t, "go", "list", "-export", "-f", "{{.Export}}", ".")
	empty := initBytes(t, "go", "list", "-overlay="+overlay, "-export", "-f", "{{.Export}}", ".")
	if generated > empty {
		t.Errorf("the generated table compiles to %d bytes of init code against the empty table's %d: a literal in table_gen.go is no longer one the compiler lays out as data", generated, empty)
	}
}

// initBytes compiles the package with the go command given, through
// `go list -export`, and returns the size of its init functions.
func initBytes(t *testing.T, command ...string) int {
	t.Helper()
	exported, err := exec.CommandContext(t.Context(), command[0], command[1:]...).Output() // #nosec G204 -- the go command, with the arguments this test writes
	if err != nil {
		t.Fatalf("%s: %v", strings.Join(command, " "), err)
	}
	object := strings.TrimSpace(string(exported))
	symbols, err := exec.CommandContext(t.Context(), "go", "tool", "nm", "-size", object).Output()
	if err != nil {
		t.Fatalf("go tool nm -size %s: %v", object, err)
	}
	if !strings.Contains(string(symbols), "actiongrants.table") {
		t.Fatalf("the symbols of %s name no table, so they are not this package's", object)
	}
	total := 0
	for line := range strings.SplitSeq(string(symbols), "\n") {
		if match := initSymbol.FindStringSubmatch(line); match != nil {
			size, _ := strconv.Atoi(match[1])
			total += size
		}
	}
	return total
}

// committedWithoutObject names every action whose write GitLab can run and
// answer without the object it returns to a fine-grained token, with how its
// handler answers that (issue 1103). GitLab checks the token against the
// mutation before the write runs and against the object the payload carries
// only after it ran, so the write commits and the object comes back null.
//
// Two kinds are listed. The rows the table denies with that effect are
// withheld from every session carrying an authority, and their handlers answer
// for a table recorded from a later release that stops withholding one. The
// rows it does not deny are served, and reach that answer today: a grant that
// passes the mutation and lacks a permission the payload's object needs, which
// phase A never asks about and phase B judges at any project or group, not the
// one written to.
var committedWithoutObject = map[string]string{
	"achievement.award":                      "achievements: toolutil.UnconfirmedWrite",
	"achievement.create":                     "achievements: toolutil.UnconfirmedWrite",
	"achievement.delete":                     "achievements: toolutil.UnconfirmedWrite",
	"achievement.revoke":                     "achievements: toolutil.UnconfirmedWrite",
	"achievement.update":                     "achievements: toolutil.UnconfirmedWrite",
	"achievement.user_achievement_delete":    "achievements: toolutil.UnconfirmedWrite",
	"achievement.user_achievement_reorder":   "achievements: toolutil.UnconfirmedWrite",
	"achievement.user_achievement_update":    "achievements: toolutil.UnconfirmedWrite",
	"custom_emoji.create":                    "customemoji.Create: the authority at the missing emoji",
	"custom_emoji.delete":                    "customemoji.Delete selects the errors alone and answers success, which a deletion GitLab ran is",
	"group.epic_create":                      "epics.Create: toolutil.UnconfirmedWrite",
	"group.epic_discussion_update_note":      "toolutil.ExecGraphQLNoteMutation: the authority at the missing note",
	"group.epic_note_update":                 "toolutil.ExecGraphQLNoteMutation: the authority at the missing note",
	"issue.work_item_create":                 "workitems.Create: toolutil.UnconfirmedWrite",
	"issue.work_item_saved_view_create":      "workitemsavedviews: toolutil.UnconfirmedWrite",
	"issue.work_item_saved_view_subscribe":   "workitemsavedviews: toolutil.UnconfirmedWrite",
	"issue.work_item_saved_view_unsubscribe": "workitemsavedviews: toolutil.UnconfirmedWrite",
	"issue.work_item_saved_view_update":      "workitemsavedviews: toolutil.UnconfirmedWrite",
	"project.target_branch_rule_create":      "projects: toolutil.UnconfirmedWrite after the captured refusal",
	"security_attribute.create":              "securityattributes: toolutil.UnconfirmedWrite and the authority at the empty list",
	"security_attribute.update":              "securityattributes: toolutil.UnconfirmedWrite and the authority at the missing attribute",
	"security_category.create":               "securitycategories: toolutil.UnconfirmedWrite",
	"security_category.update":               "securitycategories: toolutil.UnconfirmedWrite",
	"vulnerability.confirm":                  "vulnerabilities.runVulnerabilityMutation: the authority at the missing vulnerability",
	"vulnerability.dismiss":                  "vulnerabilities.runVulnerabilityMutation: the authority at the missing vulnerability",
	"vulnerability.resolve":                  "vulnerabilities.runVulnerabilityMutation: the authority at the missing vulnerability",
	"vulnerability.revert":                   "vulnerabilities.runVulnerabilityMutation: the authority at the missing vulnerability",
}

// TestTable_WritesAnsweredWithoutTheirObject_AreEachAnswered verifies that
// every action the table says GitLab can run and answer without its object is
// declared in [committedWithoutObject], so a write a regeneration adds to that
// set fails here until its handler is looked at, and that every declaration
// still names one. The handler tests hold what each answers.
func TestTable_WritesAnsweredWithoutTheirObject_AreEachAnswered(t *testing.T) {
	found := writesAnsweredWithoutTheirObject(Table())
	for id, why := range found {
		if _, declared := committedWithoutObject[id]; !declared {
			t.Errorf("%s: GitLab can run this write and answer without its object (%s); answer that in its handler "+
				"through toolutil.UnconfirmedWrite or the client's authority, and declare it in committedWithoutObject", id, why)
		}
	}
	for id := range committedWithoutObject {
		if _, ok := found[id]; !ok {
			t.Errorf("%s is declared in committedWithoutObject and the table no longer says it can be answered without its object", id)
		}
	}
}

// writesAnsweredWithoutTheirObject returns, keyed by action ID with the reason,
// every action the table denies as committed and then answered null, and every
// action it serves whose mutation's answer spine holds a position needing a
// permission some grant passing the mutation does not hold.
func writesAnsweredWithoutTheirObject(table *finegrained.Table) map[string]string {
	found := map[string]string{}
	for i := range table.Actions {
		row := &table.Actions[i]
		if row.Denied != nil {
			if row.Denied.Effect == finegrained.EffectCommittedThenNull {
				found[row.ID] = "denied: " + row.Denied.Element + " is committed and answered null"
			}
			continue
		}
		for _, path := range row.Paths {
			for _, index := range path {
				if why := mutationObjectWithheld(table, &table.Operations[index]); why != "" {
					found[row.ID] = why
				}
			}
		}
	}
	return found
}

// mutationObjectWithheld names the first position on a mutation's answer
// spine some grant passing the mutation cannot read, or returns "" when there
// is none or the operation is no mutation.
func mutationObjectWithheld(table *finegrained.Table, op *finegrained.Operation) string {
	if !strings.HasPrefix(op.Name, "mutation ") {
		return ""
	}
	held := map[uint16]bool{}
	for _, group := range op.Groups {
		for _, perm := range table.Groups[group].Perms {
			held[perm] = true
		}
	}
	for _, index := range op.Spine {
		element := &table.Elements[index]
		for _, group := range element.Groups {
			for _, perm := range table.Groups[group].Perms {
				if !held[perm] && grantableWithout(table, held, perm) {
					return element.Path + " needs " + table.Permissions[perm]
				}
			}
		}
	}
	return ""
}

// grantableWithout reports whether a grant can hold every permission of
// needed without perm: for each one, some assignable a token may be granted
// carries it and not perm. Assignables are granted whole, so this is exactly
// whether the union of one choice per permission leaves perm out.
func grantableWithout(table *finegrained.Table, needed map[uint16]bool, perm uint16) bool {
	for want := range needed {
		if !slices.ContainsFunc(table.Assignables, func(a finegrained.Assignable) bool {
			return a.Grantable && slices.Contains(a.Permissions, want) && !slices.Contains(a.Permissions, perm)
		}) {
			return false
		}
	}
	return true
}

// TestTable_NamesTheRoutesTheHandlersSend holds the generated table's routes
// to the requests the binary's handlers send, as the client span of each
// names it: an issue list, an issue read by a project's escaped path, a file
// written by an escaped file path, the raw file's HEAD read off its GET (the
// route GitLab mounts only as a GET, whose authorization a HEAD carries, which
// a booted 19.4.1 shows), the GraphQL endpoint, and a route no action calls,
// which no template names.
func TestTable_NamesTheRoutesTheHandlersSend(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{"GET", "/api/v4/projects/7/issues", "/api/v4/projects/:id/issues"},
		{"GET", "/api/v4/projects/group%2Fproject/issues/3", "/api/v4/projects/:id/issues/:issue_iid"},
		{"POST", "/api/v4/projects/7/repository/files/docs%2Fguide.md", "/api/v4/projects/:id/repository/files/:file_path"},
		{"HEAD", "/api/v4/projects/7/repository/files/README.md/raw", "/api/v4/projects/:id/repository/files/:file_path/raw"},
		{"POST", "/api/graphql", "/api/graphql"},
		{"GET", "/api/v4/version", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			got, known := Table().RouteTemplate(tc.method, tc.path)
			if got != tc.want || known != (tc.want != "") {
				t.Errorf("RouteTemplate = %q, %t; want %q", got, known, tc.want)
			}
		})
	}
}
