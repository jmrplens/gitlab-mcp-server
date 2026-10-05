package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// testPages renders the synthetic reference and data file.
func testPages(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	for _, p := range renderPages(testReference(t), mustDomains(t)) {
		pages[p.path] = string(p.content)
	}
	return pages
}

// mustContain fails the test for each of wants text does not contain.
func mustContain(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("text does not contain %q:\n%s", want, text)
		}
	}
}

func TestRenderPages_SyntheticReference_WritesAnIndexAndOnePagePerGroupInEachLanguage(t *testing.T) {
	pages := renderPages(testReference(t), mustDomains(t))
	var paths []string
	for _, p := range pages {
		paths = append(paths, p.path)
	}
	want := []string{
		english.dir + "/index.mdx", english.dir + "/helper.mdx", english.dir + "/widget.mdx",
		spanish.dir + "/index.mdx", spanish.dir + "/helper.mdx", spanish.dir + "/widget.mdx",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Errorf("paths = %v, want %v", paths, want)
	}
	for _, p := range pages {
		if !strings.HasSuffix(string(p.content), ".\n") && !strings.HasSuffix(string(p.content), "|\n") {
			t.Errorf("%s does not end in one newline after its last block", p.path)
		}
	}
}

func TestRenderIndex_SyntheticReference_ListsTheGroupsByCategory(t *testing.T) {
	pages := testPages(t)
	index := pages[english.dir+"/index.mdx"]
	mustContain(t, index,
		"---\ntitle: \"Tools by domain\"\n",
		"sidebar:\n  order: 0\n  label: \"Overview\"\n---\n\n",
		english.generatedNote+"\n\n",
		"On GitLab.com at Ultimate the catalog holds 4 actions; a self-managed Ultimate instance serves 3, a Premium one 3 and a Free one 2.",
		"## Work\n\n- [Widgets](/gitlab-mcp-server/reference/tools/widget/): Widgets described.\n\n## Tools\n\n- [Helpers](/gitlab-mcp-server/reference/tools/helper/): Helpers described.\n",
	)
	if strings.Contains(index, "tableOfContents") {
		t.Error("the index page limits its table of contents")
	}
	mustContain(t, pages[spanish.dir+"/index.mdx"], "label: \"Descripción general\"", "## Trabajo\n\n- [Widgets ES](")
}

func TestSidebarOrder_Titles_AreOrderedAfterTheIndex(t *testing.T) {
	data := mustDomains(t)
	data.Groups["gitlab_widget"].Title["es"] = "aaa"
	if got := sidebarOrder(data, english); got["gitlab_helper"] != 1 || got["gitlab_widget"] != 2 {
		t.Errorf("English order = %v, want Helpers before Widgets", got)
	}
	if got := sidebarOrder(data, spanish); got["gitlab_widget"] != 1 || got["gitlab_helper"] != 2 {
		t.Errorf("Spanish order = %v, want the lower-cased title aaa first", got)
	}
}

func TestRenderGroup_SyntheticWidget_WritesEverySection(t *testing.T) {
	page := testPages(t)[english.dir+"/widget.mdx"]
	mustContain(t, page,
		"---\ntitle: \"Widgets\"\ndescription: \"Widgets described.\"\nsidebar:\n  order: 2\ntableOfContents:\n  maxHeadingLevel: 2\n---\n\n",
		"About Widgets.\n\n## Sample questions\n\n- \"Ask Widgets\"\n\n## How to call it\n\n",
		"such as `widget.create`, and its parameters",
		"call `gitlab_widget` with `action` set to the action's name, such as `create`",
		"such as `gitlab_widget_create`, with its parameters as the arguments.",
		"## Availability\n\nHow many of these actions an instance serves at each tier, out of a total of 3:\n\n- **Free**: 1\n- **Premium**: 2 on a self-managed instance, 3 on GitLab.com\n- **Ultimate**: 2 on a self-managed instance, 3 on GitLab.com\n\n",
		"Listed only for a token that carries `admin_mode`:",
		"Read-only actions: 2 of 3, the ones a deployment in read-only mode keeps.\n\n## Actions\n\n",
		english.destructiveNote, english.paramTierNote,
		"| Action                           | Tier                 | Individual             |",
		"| [`widget.graph`](#widgetgraph)   | Premium (GitLab.com) | `gitlab_widget_graph`  |",
		"### `widget.create`\n\n> Does create.\n\n- **Meta-tool**: `gitlab_widget`, action `create`\n- **Individual tool**: `gitlab_widget_create`\n- **Tier**: Premium\n- **Behavior**: writes, destructive (needs confirmation), not idempotent\n\n",
		"| `weight` (Ultimate) | `string` | no        | weight value     |",
		"- **Tier**: Premium, GitLab.com only\n",
		"> List widgets. See also: [`widget.create`](#widgetcreate).",
		"- **Behavior**: read-only, idempotent\n",
	)
}

func TestRenderGroup_SyntheticHelper_DescribesAStandaloneGroup(t *testing.T) {
	pages := testPages(t)
	page := pages[english.dir+"/helper.mdx"]
	mustContain(t, page,
		"- **Meta** and **individual** (`GITLAB_MCP_TOOL_SURFACE=meta` or `individual`): each action is a tool of its own, such as `gitlab_helper_resolve`",
		"Every tier serves the whole group, on self-managed instances and on GitLab.com alike.\n\n",
		english.capabilities["elicitation"],
		"- **Meta-tool**: `gitlab_helper_resolve`, a tool of its own\n",
		"| Action                             | Individual              |",
		"No parameters.",
	)
	for _, absent := range []string{"call `gitlab_helper`", "Listed only for a token", english.destructiveNote, english.paramTierNote, "| Tier", "Also needs"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(page, absent) {
				t.Errorf("the helper page contains %q", absent)
			}
		})
	}
	mustContain(t, pages[spanish.dir+"/helper.mdx"], "- **Meta-herramienta**: `gitlab_helper_resolve`, una herramienta propia\n", "Sin parámetros.")
}

func TestExample_NoActionHasAnIndividualTool_FallsBackToTheFirst(t *testing.T) {
	group := &refGroup{actions: []*refAction{{id: "x.a"}, {id: "x.b"}}}
	if got := example(group); got.id != "x.a" {
		t.Errorf("example() = %s, want x.a", got.id)
	}
	group.actions[1].individual = "gitlab_x_b"
	if got := example(group); got.id != "x.b" {
		t.Errorf("example() = %s, want the first with an individual tool", got.id)
	}
}

func TestAvailability_ScopesAndCapabilities_AreSaid(t *testing.T) {
	group := &refGroup{
		scopes:       []string{"admin_mode", "sudo"},
		capabilities: []string{"elicitation"},
		served:       [2][3]int{{0, 0, 1}, {0, 0, 1}},
		actions:      []*refAction{{readOnly: false}},
	}
	got := strings.Join(availability(group, spanish), "\n\n")
	mustContain(t, got,
		"Cuántas de estas acciones sirve una instancia en cada nivel, de un total de 1:\n\n- **Free**: 0\n- **Premium**: 0\n- **Ultimate**: 1",
		"tenga `admin_mode` y `sudo`:",
		spanish.capabilities["elicitation"],
		"Acciones de solo lectura: 0 de 1,",
	)
}

func TestEverywhere_AnyBuildShort_IsFalse(t *testing.T) {
	group := &refGroup{actions: []*refAction{{}, {}}, served: [2][3]int{{2, 2, 2}, {2, 2, 2}}}
	if !everywhere(group) {
		t.Error("everywhere() = false for a group every build serves whole")
	}
	group.served[1][2] = 1
	if everywhere(group) {
		t.Error("everywhere() = true with one build short")
	}
}

func TestActionsIntro_Notes_FollowTheGroupsActions(t *testing.T) {
	plain := &refGroup{actions: []*refAction{{tier: edition.Premium, params: []param{{tier: edition.Premium}}}}}
	if got := actionsIntro(plain, english); got != english.actionsIntro {
		t.Errorf("actionsIntro() = %q, want the intro alone", got)
	}
	notes := &refGroup{actions: []*refAction{{destructive: true}, {tier: edition.Free, params: []param{{tier: edition.Ultimate}}}}}
	if got := actionsIntro(notes, english); got != english.actionsIntro+" "+english.destructiveNote+" "+english.paramTierNote {
		t.Errorf("actionsIntro() = %q, want both notes", got)
	}
}

func TestBehavior_AnnotationCombinations_AreNamed(t *testing.T) {
	tests := []struct {
		name   string
		action refAction
		want   string
	}{
		{name: "read", action: refAction{readOnly: true, idempotent: true}, want: "read-only, idempotent"},
		{name: "write", action: refAction{}, want: "writes, not idempotent"},
		{name: "destructive", action: refAction{destructive: true, idempotent: true}, want: "writes, destructive (needs confirmation), idempotent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := behavior(&tt.action, english); got != tt.want {
				t.Errorf("behavior() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteAction_NoIndividualToolAndAlternatives_AreSaid(t *testing.T) {
	var b strings.Builder
	action := &refAction{
		id: "server.health_check", name: "health_check", metaTool: "gitlab_server",
		params: []param{{name: "a", typ: "`string`", required: true, description: "A | B"}},
		oneOf:  [][]string{{"a"}, {"b", "c"}},
	}
	writeAction(&b, action, english, func(id string) string { return "#" + anchor(id) })
	mustContain(t, b.String(),
		"### `server.health_check`\n\n>\n\n",
		english.factNoIndividual,
		"| `a`       | `string` | yes       | A \\| B      |",
		"Also needs at least one of: `a`; `b` and `c`.",
	)
}

func TestQuote_ListsAndClauses_AreSetApart(t *testing.T) {
	linkTo := func(id string) string { return "/x/#" + anchor(id) }
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "list after text", in: "Steps:\n- one\n- two\nDone.", want: "> Steps:\n>\n> - one\n> - two\n>\n> Done."},
		{name: "list first", in: "- one\n\nAfter.", want: "> - one\n>\n> After."},
		{name: "blank line kept", in: "Intro.\n\n- one", want: "> Intro.\n>\n> - one"},
		{name: "see also linked", in: "Get. See also: a.b, c.d.", want: "> Get. See also: [`a.b`](/x/#ab), [`c.d`](/x/#cd)."},
		{name: "every clause linked", in: "Get. See also: a.b. Then. See also: c.d.", want: "> Get. See also: [`a.b`](/x/#ab). Then. See also: [`c.d`](/x/#cd)."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quote(tt.in, linkTo); got != tt.want {
				t.Errorf("quote() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestLink_TargetPage_IsRelativeOnlyOnItsOwnPage(t *testing.T) {
	targets := map[string]string{"widget.list": "widget"}
	if got := link("widget.list", "widget", targets); got != "#widgetlist" {
		t.Errorf("link() on the page = %q", got)
	}
	if got := link("widget.list", "helper", targets); got != "/gitlab-mcp-server/reference/tools/widget/#widgetlist" {
		t.Errorf("link() from another page = %q", got)
	}
}

func TestInline_ServedText_RendersAsTheCharactersItHolds(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		table bool
		want  string
	}{
		{name: "mdx and markdown", text: `a {b} <c> *d* [e] #f |g ~h \i`, want: `a \{b\} \<c\> \*d\* \[e\] \#f \|g \~h \\i`},
		{name: "code kept", text: "use `x|{y}` here {z}", want: "use `x|{y}` here \\{z\\}"},
		{name: "code pipe in a table", text: "use `x|y`", table: true, want: "use `x\\|y`"},
		{name: "unmatched backtick", text: "a ` b {c}", want: "a \\` b \\{c\\}"},
		{name: "web address", text: "See https://docs.gitlab.com/api/x#y.", want: "See [`https://docs.gitlab.com/api/x#y`](https://docs.gitlab.com/api/x#y)."},
		{name: "other scheme", text: "or ssh://git@host/a.git", want: "or `ssh://git@host/a.git`"},
		{name: "every address", text: "https://a.example/x, https://b.example/y", want: "[`https://a.example/x`](https://a.example/x), [`https://b.example/y`](https://b.example/y)"},
		{name: "ssh remote", text: "git@gitlab.example.com:group/p.git, then", want: "`git@gitlab.example.com:group/p.git`, then"},
		{name: "bare user", text: "the git@ prefix", want: "the git@ prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inline(tt.text, tt.table); got != tt.want {
				t.Errorf("inline() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckLabels_UnknownCapability_IsRefused(t *testing.T) {
	if err := checkLabels([]*refGroup{{tool: "gitlab_x", capabilities: []string{"elicitation"}}}); err != nil {
		t.Errorf("checkLabels() = %v for a capability both languages name", err)
	}
	err := checkLabels([]*refGroup{{tool: "gitlab_x", capabilities: []string{"sampling"}}})
	if err == nil || !strings.Contains(err.Error(), "gitlab_x needs the sampling client capability, and the en pages have no sentence for it") {
		t.Errorf("checkLabels() = %v, want the capability named", err)
	}
}

func TestYAMLString_QuotesAndBackslashes_AreEscaped(t *testing.T) {
	if got := yamlString(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Errorf("yamlString() = %s", got)
	}
}
