// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
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

// siteLinkTarget finds the target of a link to a page of the documentation
// site, which every page of it is served under, in any of the forms
// TestSiteLink_EveryFormOfALinkIntoTheSite_IsFound lists.
var siteLinkTarget = regexp.MustCompile(`(?:(?:\]\([ \t]*(?:\r?\n[ \t]*)?<?|\]:[ \t]*(?:\r?\n[ \t]*)?|<|href=["'])(?:https://docs\.jmrp\.io)?|https://docs\.jmrp\.io)(/gitlab-mcp-server/[^\s)"'>]*)`)

// localeLinks splits the site links of the pages lang writes into the ones
// that stay in lang and the ones that leave it. The Spanish pages are the ones
// served under /gitlab-mcp-server/es/, and every other page is English; the
// rule is written out here rather than read from the generator, so a language
// whose links the generator spells wrong cannot pass by agreeing with itself.
func localeLinks(pages map[string]string, lang language) (inside int, outside []string) {
	for path, page := range pages {
		if !strings.HasPrefix(path, lang.dir+"/") {
			continue
		}
		for _, match := range siteLinkTarget.FindAllStringSubmatch(page, -1) {
			spanish := strings.HasPrefix(match[1], "/gitlab-mcp-server/es/")
			if spanish == (lang.code == "es") {
				inside++
				continue
			}
			outside = append(outside, path+": "+match[1])
		}
	}
	return inside, outside
}

// TestRenderPages_EachLanguage_LinksOnlyThePagesOfItsLanguage holds every link
// a page writes into the site to the pages of the page's own language (issue
// 1285): the index entries, the permissions page each action line names, the
// note on destructive actions and the client capability a group needs, all of
// which the synthetic pages carry. The site would rewrite an English path on a
// Spanish page at build time, so this is about what the page says rather than
// where the built site sends a reader, and the source is what every other
// reader of it, the link checks among them, reads.
func TestRenderPages_EachLanguage_LinksOnlyThePagesOfItsLanguage(t *testing.T) {
	pages := testPages(t)
	for _, lang := range languages {
		t.Run(lang.code, func(t *testing.T) {
			inside, outside := localeLinks(pages, lang)
			for _, link := range outside {
				t.Errorf("a %s page links outside its language: %s", lang.code, link)
			}
			// Two index entries, a permissions link on each of the four
			// action lines, the destructive note and the elicitation note.
			if want := 2 + 4 + 1 + 1; inside+len(outside) != want {
				t.Errorf("the %s pages carry %d site links, want %d", lang.code, inside+len(outside), want)
			}
		})
	}
	mustContain(t, pages[spanish.dir+"/widget.mdx"],
		"([página de permisos](/gitlab-mcp-server/es/reference/fine-grained-permissions/#widget))",
		"([Acciones destructivas](/gitlab-mcp-server/es/operations/security/#acciones-destructivas))")
	mustContain(t, pages[spanish.dir+"/helper.mdx"], "[elicitación](/gitlab-mcp-server/es/capabilities/elicitation/)")
	mustContain(t, pages[spanish.dir+"/index.mdx"], "- [Widgets ES](/gitlab-mcp-server/es/reference/tools/widget/): Widgets descrito.")
}

// TestRenderPages_RealCatalog_EachLanguageLinksOnlyThePagesOfItsLanguage is
// the same rule over the pages the command writes, where the links to the
// other groups' pages and the hand-written overviews of domains.json are read
// too.
func TestRenderPages_RealCatalog_EachLanguageLinksOnlyThePagesOfItsLanguage(t *testing.T) {
	data, err := decodeDomains(domainsJSON)
	if err != nil {
		t.Fatalf("decodeDomains() error = %v", err)
	}
	pages := map[string]string{}
	for _, p := range renderPages(mustRealReference(t), data) {
		pages[p.path] = string(p.content)
	}
	for _, lang := range languages {
		t.Run(lang.code, func(t *testing.T) {
			inside, outside := localeLinks(pages, lang)
			if len(outside) > 0 {
				t.Errorf("%d links of the %s pages leave their language, the first %v", len(outside), lang.code, outside[:min(len(outside), 5)])
			}
			if inside < 1000 {
				t.Errorf("the %s pages carry %d links into their language, want the real catalog's", lang.code, inside)
			}
		})
	}
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
		"Read-only actions: 2 of 3, the ones a deployment in read-only mode keeps. A token with `read_api` and not `api` is served 2 of the 3.\n\n## Actions\n\n",
		"The token lines say what GitLab 19.4.1 requires of a token for the requests each action sends; on an instance with Admin Mode turned on, an action only an administrator may run also needs `admin_mode`.",
		english.destructiveNote, english.paramTierNote,
		"| Action                           | Tier                 | Individual             |",
		"| [`widget.graph`](#widgetgraph)   | Premium (GitLab.com) | `gitlab_widget_graph`  |",
		"### `widget.create`\n\n> Does create.\n\n- **Meta-tool**: `gitlab_widget`, action `create`\n- **Individual tool**: `gitlab_widget_create`\n- **Tier**: Premium\n"+
			"- **Classic or OAuth token**: `api`; this server lists the group only to a token that also carries `admin_mode`\n"+
			"- **Fine-grained token**: Widget: Read at project ([permissions page](/gitlab-mcp-server/reference/fine-grained-permissions/#widget))\n"+
			"- **Behavior**: writes, destructive (needs confirmation), not idempotent\n\n",
		"- **Classic or OAuth token**: `read_api` (or `api`); this server lists the group only to a token that also carries `admin_mode`\n",
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
		"- **Classic or OAuth token**: `read_api` (or `api`)\n",
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
	mustContain(t, pages[spanish.dir+"/helper.mdx"],
		"- **Meta-herramienta**: `gitlab_helper_resolve`, una herramienta propia\n",
		"- **Token clásico u OAuth**: `read_api` (o `api`)\n",
		"- **Token de grano fino**: Widget: Read en proyecto ([página de permisos](/gitlab-mcp-server/es/reference/fine-grained-permissions/#helper))\n",
		"Las líneas de token dicen lo que exige GitLab 19.4.1 a un token para las peticiones que envía cada acción; en una instancia con Admin Mode activado, una acción que solo puede ejecutar un administrador necesita además `admin_mode`.",
		"Sin parámetros.")
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
		"Acciones de solo lectura: 0 de 1, las que conserva un despliegue en modo de solo lectura. A un token con `read_api` y sin `api` se le sirven 0 de las 1.",
	)
}

// TestAvailability_ReadAPICount_FollowsEachActionsClassicScope verifies the
// count of actions a read_api token is served is read from each action's
// classic scope and not from its read-only annotation: a write GitLab
// authenticates by another credential and an action that sends nothing are
// served, a read sent as a POST GitLab answers only from api is not.
func TestAvailability_ReadAPICount_FollowsEachActionsClassicScope(t *testing.T) {
	group := &refGroup{
		served: [2][3]int{{4, 4, 4}, {4, 4, 4}},
		actions: []*refAction{
			{readOnly: true, classic: finegrained.ClassicAPI},
			{classic: finegrained.ClassicOtherCredential},
			{readOnly: true, classic: finegrained.ClassicNoRequest},
			{classic: finegrained.ClassicReadAPI},
		},
	}
	blocks := availability(group, english)
	if got, want := blocks[len(blocks)-1], "Read-only actions: 2 of 4, the ones a deployment in read-only mode keeps. A token with `read_api` and not `api` is served 3 of the 4."; got != want {
		t.Errorf("availability() last block = %q, want %q", got, want)
	}
}

// TestClassicText_EachScope_IsWorded verifies the classic or OAuth token line
// for every scope an action can need, with and without the scopes this server
// demands of its group, which the line says are the server's and not GitLab's,
// in both languages: api alone, read_api with api named as the token that
// carries it too, the two kinds whose own requests GitLab does not judge by
// the token's scope, which say why, and the routes GitLab refuses to an OAuth
// token, on every way the action runs or on some.
func TestClassicText_EachScope_IsWorded(t *testing.T) {
	tests := []struct {
		name   string
		action refAction
		en, es string
	}{
		{name: "api", action: refAction{classic: finegrained.ClassicAPI}, en: "`api`", es: "`api`"},
		{name: "read_api", action: refAction{classic: finegrained.ClassicReadAPI}, en: "`read_api` (or `api`)", es: "`read_api` (o `api`)"},
		{
			name: "api and admin_mode", action: refAction{classic: finegrained.ClassicAPI, scopes: []string{"admin_mode"}},
			en: "`api`; this server lists the group only to a token that also carries `admin_mode`",
			es: "`api`; este servidor solo lista el grupo a un token que lleve además `admin_mode`",
		},
		{
			name: "read_api and two group scopes", action: refAction{classic: finegrained.ClassicReadAPI, scopes: []string{"admin_mode", "sudo"}},
			en: "`read_api` (or `api`); this server lists the group only to a token that also carries `admin_mode` and `sudo`",
			es: "`read_api` (o `api`); este servidor solo lista el grupo a un token que lleve además `admin_mode` y `sudo`",
		},
		{
			name: "refused to OAuth on every way", action: refAction{classic: finegrained.ClassicReadAPI, oauthRefused: []string{"DELETE /tokens/self"}, oauthRefusedEveryWay: true},
			en: "`read_api` (or `api`); not an OAuth token, which GitLab refuses on `DELETE /tokens/self`",
			es: "`read_api` (o `api`); no un token OAuth, que GitLab rechaza en `DELETE /tokens/self`",
		},
		{
			name: "refused to OAuth on some ways", action: refAction{classic: finegrained.ClassicReadAPI, oauthRefused: []string{"GET /tokens/self", "GET /tokens/mine"}},
			en: "`read_api` (or `api`); GitLab refuses an OAuth token on `GET /tokens/self` and `GET /tokens/mine`, which this action sends for some inputs",
			es: "`read_api` (o `api`); GitLab rechaza un token OAuth en `GET /tokens/self` y `GET /tokens/mine`, que esta acción envía con algunas entradas",
		},
		{
			name: "another credential", action: refAction{classic: finegrained.ClassicOtherCredential},
			en: "`read_api` (or `api`); GitLab authenticates the token this action sends as a parameter and does not judge this one's scope",
			es: "`read_api` (o `api`); GitLab autentica el token que esta acción envía como parámetro y no juzga el scope de este",
		},
		{
			name: "no request", action: refAction{classic: finegrained.ClassicNoRequest},
			en: "`read_api` (or `api`); the action sends GitLab no request",
			es: "`read_api` (o `api`); la acción no envía ninguna petición a GitLab",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classicText(&tt.action, english); got != tt.en {
				t.Errorf("English classicText() = %q, want %q", got, tt.en)
			}
			if got := classicText(&tt.action, spanish); got != tt.es {
				t.Errorf("Spanish classicText() = %q, want %q", got, tt.es)
			}
		})
	}
}

// TestFineGrainedText_NamesWhatIsServedEmptyAndLinksTheDomain verifies the
// fine-grained line is the permissions page's sentence for the action, then
// the parts of its answer a fine-grained token may be served empty when there
// are any, then a link to its domain on that page, in both languages.
func TestFineGrainedText_NamesWhatIsServedEmptyAndLinksTheDomain(t *testing.T) {
	read := finegrained.Need{Permissions: []string{"Issue: Read"}, At: []string{"project"}}
	tests := []struct {
		name        string
		description finegrained.Description
		en, es      string
	}{
		{
			name: "served whole", description: finegrained.Description{AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read}}}},
			en: "Issue: Read at project ([permissions page](/gitlab-mcp-server/reference/fine-grained-permissions/#issue))",
			es: "Issue: Read en proyecto ([página de permisos](/gitlab-mcp-server/es/reference/fine-grained-permissions/#issue))",
		},
		{
			name: "served empty in part",
			description: finegrained.Description{
				AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read}}}, AlwaysEmpty: []string{"issue { epic }"},
			},
			en: "Issue: Read at project; served empty: `issue { epic }` (always) ([permissions page](/gitlab-mcp-server/reference/fine-grained-permissions/#issue))",
			es: "Issue: Read en proyecto; se sirve vacío: `issue { epic }` (siempre) ([página de permisos](/gitlab-mcp-server/es/reference/fine-grained-permissions/#issue))",
		},
		{
			name:        "not reachable",
			description: finegrained.Description{Denied: &finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"}},
			en:          "Not reachable at this release: GitLab declares no fine-grained permission for `GET /x` ([permissions page](/gitlab-mcp-server/reference/fine-grained-permissions/#issue))",
			es:          "No alcanzable en esta versión: GitLab no declara ningún permiso de grano fino para `GET /x` ([página de permisos](/gitlab-mcp-server/es/reference/fine-grained-permissions/#issue))",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &refAction{domain: "issue", fineGrained: &tt.description}
			if got := fineGrainedText(action, english); got != tt.en {
				t.Errorf("English fineGrainedText() = %q, want %q", got, tt.en)
			}
			if got := fineGrainedText(action, spanish); got != tt.es {
				t.Errorf("Spanish fineGrainedText() = %q, want %q", got, tt.es)
			}
		})
	}
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
	intro := english.actionsIntro + " The token lines say what GitLab 19.4.1 requires of a token for the requests each action sends; " +
		"on an instance with Admin Mode turned on, an action only an administrator may run also needs `admin_mode`."
	plain := &refGroup{actions: []*refAction{{tier: edition.Premium, params: []param{{tier: edition.Premium}}}}}
	if got := actionsIntro(plain, english, "19.4.1"); got != intro {
		t.Errorf("actionsIntro() = %q, want the intro and the release alone", got)
	}
	notes := &refGroup{actions: []*refAction{{destructive: true}, {tier: edition.Free, params: []param{{tier: edition.Ultimate}}}}}
	if got := actionsIntro(notes, english, "19.4.1"); got != intro+" "+english.destructiveNote+" "+english.paramTierNote {
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
		params:  []param{{name: "a", typ: "`string`", required: true, description: "A | B"}},
		oneOf:   [][]string{{"a"}, {"b", "c"}},
		domain:  "server",
		classic: finegrained.ClassicReadAPI, fineGrained: &finegrained.Description{AnyOf: []finegrained.Way{{}}},
	}
	writeAction(&b, action, english, func(id string) string { return "#" + anchor(id) })
	mustContain(t, b.String(),
		"### `server.health_check`\n\n>\n\n",
		english.factNoIndividual,
		"- **Fine-grained token**: no permission ([permissions page](/gitlab-mcp-server/reference/fine-grained-permissions/#server))\n",
		"| `a`       | `string` | yes       | A \\| B      |",
		"Also needs at least one of: `a`; `b` and `c`.",
	)
}

// TestWriteAction_ExclusiveAlternatives_SayExactlyOne verifies the sentence
// an action whose alternatives are exclusive is written with, in each
// language: exactly one of them, where the alternatives of an anyOf read at
// least one. The page wrote at least one for both, so the reference told a
// reader that an LDAP group link could be added by a common name and a filter
// together, and GitLab refuses that call.
func TestWriteAction_ExclusiveAlternatives_SayExactlyOne(t *testing.T) {
	tests := []struct {
		lang language
		want string
	}{
		{lang: english, want: "Also needs exactly one of: `cn`; `filter`."},
		{lang: spanish, want: "Además necesita exactamente uno de estos: `cn`; `filter`."},
	}
	for _, tc := range tests {
		t.Run(tc.lang.code, func(t *testing.T) {
			var b strings.Builder
			action := &refAction{
				id: "group.ldap_link_add", name: "ldap_link_add", metaTool: "gitlab_group", individual: "gitlab_group_ldap_link_add",
				params:    []param{{name: "cn", typ: "`string`"}, {name: "filter", typ: "`string`"}},
				oneOf:     [][]string{{"cn"}, {"filter"}},
				exclusive: true,
				domain:    "group",
				classic:   finegrained.ClassicAPI, fineGrained: &finegrained.Description{AnyOf: []finegrained.Way{{}}},
			}
			writeAction(&b, action, tc.lang, func(id string) string { return "#" + anchor(id) })
			mustContain(t, b.String(), tc.want)
			if strings.Contains(b.String(), tc.lang.oneOf[:strings.Index(tc.lang.oneOf, "%s")]) {
				t.Errorf("page = %q, want no at-least-one sentence for exclusive alternatives", b.String())
			}
		})
	}
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
	tests := []struct {
		lang  language
		other string
	}{
		{lang: english, other: "/gitlab-mcp-server/reference/tools/widget/#widgetlist"},
		{lang: spanish, other: "/gitlab-mcp-server/es/reference/tools/widget/#widgetlist"},
	}
	for _, tt := range tests {
		t.Run(tt.lang.code, func(t *testing.T) {
			if got := link("widget.list", "widget", targets, tt.lang); got != "#widgetlist" {
				t.Errorf("link() on the page = %q", got)
			}
			if got := link("widget.list", "helper", targets, tt.lang); got != tt.other {
				t.Errorf("link() from another page = %q, want %q", got, tt.other)
			}
		})
	}
}

// TestSiteLink_LanguagePaths_AreUnderTheSiteRoot holds the literals the link
// finder and the language table spell to siteRoot: every language serves its
// pages below it, English at it, and the finder reads exactly the links that
// begin with it.
func TestSiteLink_LanguagePaths_AreUnderTheSiteRoot(t *testing.T) {
	if english.site != siteRoot {
		t.Errorf("english.site = %q, want the site root %q", english.site, siteRoot)
	}
	for _, lang := range languages {
		t.Run(lang.code, func(t *testing.T) {
			if !strings.HasPrefix(lang.site, siteRoot) || !strings.HasSuffix(lang.site, "/") {
				t.Errorf("%s.site = %q, want a directory below %q", lang.code, lang.site, siteRoot)
			}
		})
	}
	text := "[a](" + siteRoot + "x/) [b](/elsewhere/) [c](https://example.com" + siteRoot + ")"
	got := siteLink.FindAllStringSubmatch(text, -1)
	if len(got) != 1 || got[0][1] != siteRoot+"x/" {
		t.Errorf("siteLink found %q in %q, want only %q", got, text, siteRoot+"x/")
	}
}

// TestSiteLink_EveryFormOfALinkIntoTheSite_IsFound verifies both link
// finders, the generator's and this test file's own, read every way a page
// can link into the site, root-relative or on the site's host, and nothing
// that only names the path: a Markdown link with or without a title, one in
// angle brackets, one whose destination follows spaces, tabs or a line ending
// after the opening parenthesis (CommonMark allows up to one line ending
// there), a reference definition, one whose destination starts the next line,
// an autolink, a bare URL the site's Markdown turns into a link, and an href.
func TestSiteLink_EveryFormOfALinkIntoTheSite_IsFound(t *testing.T) {
	text := strings.Join([]string{
		"[a](/gitlab-mcp-server/a/)",
		`[b](/gitlab-mcp-server/b/ "a title")`,
		"[c](</gitlab-mcp-server/c/>)",
		"[d](https://docs.jmrp.io/gitlab-mcp-server/d/)",
		"[e]: /gitlab-mcp-server/e/",
		"[f]:\thttps://docs.jmrp.io/gitlab-mcp-server/f/",
		"<https://docs.jmrp.io/gitlab-mcp-server/g/>",
		"see https://docs.jmrp.io/gitlab-mcp-server/h/ there",
		`<a href="/gitlab-mcp-server/i/">i</a> <a href='https://docs.jmrp.io/gitlab-mcp-server/j/'>j</a>`,
		"the site is served under `/gitlab-mcp-server/k/`, [l](https://example.com/gitlab-mcp-server/l/)",
		"[m]( /gitlab-mcp-server/m/) [n](\t https://docs.jmrp.io/gitlab-mcp-server/n/)",
		"[o](\n  /gitlab-mcp-server/o/)",
		"[p]:\n  /gitlab-mcp-server/p/",
	}, "\n")
	want := []string{
		"/gitlab-mcp-server/a/", "/gitlab-mcp-server/b/", "/gitlab-mcp-server/c/", "/gitlab-mcp-server/d/",
		"/gitlab-mcp-server/e/", "/gitlab-mcp-server/f/", "/gitlab-mcp-server/g/", "/gitlab-mcp-server/h/",
		"/gitlab-mcp-server/i/", "/gitlab-mcp-server/j/", "/gitlab-mcp-server/m/", "/gitlab-mcp-server/n/",
		"/gitlab-mcp-server/o/", "/gitlab-mcp-server/p/",
	}
	for _, finder := range []struct {
		name string
		re   *regexp.Regexp
	}{
		{name: "siteLink", re: siteLink},
		{name: "siteLinkTarget", re: siteLinkTarget},
	} {
		t.Run(finder.name, func(t *testing.T) {
			var got []string
			for _, match := range finder.re.FindAllStringSubmatch(text, -1) {
				got = append(got, match[1])
			}
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("%s found\n%q\nwant\n%q", finder.name, got, want)
			}
		})
	}
}

// TestLanguageOf_SiteLinks_BelongToTheLanguageWhosePrefixTheyBegin verifies
// a link into the site is English unless it begins with another language's
// prefix, which the English root is a prefix of.
func TestLanguageOf_SiteLinks_BelongToTheLanguageWhosePrefixTheyBegin(t *testing.T) {
	tests := []struct{ target, want string }{
		{target: "/gitlab-mcp-server/", want: "en"},
		{target: "/gitlab-mcp-server/reference/tools/", want: "en"},
		{target: "/gitlab-mcp-server/es/", want: "es"},
		{target: "/gitlab-mcp-server/es/reference/tools/#x", want: "es"},
		{target: "/gitlab-mcp-server/estimates/", want: "en"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := languageOf(tt.target).code; got != tt.want {
				t.Errorf("languageOf(%q) = %s, want %s", tt.target, got, tt.want)
			}
		})
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
