package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestDenialText_WordsEveryCause verifies the page says, for each reason no
// fine-grained token reaches an action, what GitLab declares and what it does
// to the request: refused before anything runs, or written and answered null.
// Each cause is worded in both languages, with the element GitLab names left
// as GitLab names it.
func TestDenialText_WordsEveryCause(t *testing.T) {
	cases := []struct {
		denial finegrained.Denial
		en, es string
	}{
		{
			denial: finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: "attach"},
			en:     "GitLab declares no fine-grained permission for the mutation `attach`, and refuses it",
			es:     "GitLab no declara ningún permiso de grano fino para la mutación `attach`, y la rechaza",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace"},
			en:     "GitLab declares no fine-grained permission for `Namespace`, which the answer is made of",
			es:     "GitLab no declara ningún permiso de grano fino para `Namespace`, del que está hecha la respuesta",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CausePayloadUndeclared, Element: "Label"},
			en:     "GitLab declares no fine-grained permission for `Label` in the answer, so the write commits and its answer is lost",
			es:     "GitLab no declara ningún permiso de grano fino para `Label` en la respuesta, así que la escritura se confirma y su respuesta se pierde",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "workItemUpdate", Effect: finegrained.EffectRefused},
			en:     "`workItemUpdate` is declared at a boundary the object the action reaches never resolves to, so GitLab refuses the write before it runs",
			es:     "`workItemUpdate` se declara en un ámbito al que nunca se resuelve el objeto que alcanza la acción, así que GitLab rechaza la escritura antes de que se ejecute",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "WorkItem", Effect: finegrained.EffectCommittedThenNull},
			en:     "`WorkItem` is declared at a boundary the object the action reaches never resolves to, so the write commits and its answer is lost",
			es:     "`WorkItem` se declara en un ámbito al que nunca se resuelve el objeto que alcanza la acción, así que la escritura se confirma y su respuesta se pierde",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "WorkItem", Effect: finegrained.EffectNullOrEmpty},
			en:     "`WorkItem` is declared at a boundary the object the action reaches never resolves to, so GitLab answers it as null or leaves it out of the list",
			es:     "`WorkItem` se declara en un ámbito al que nunca se resuelve el objeto que alcanza la acción, así que GitLab lo responde como null o lo deja fuera de la lista",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "POST /policies"},
			en:     "GitLab marks `POST /policies` as not yet supported for fine-grained tokens",
			es:     "GitLab marca `POST /policies` como aún no admitido para tokens de grano fino",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"},
			en:     "GitLab declares no fine-grained permission for `GET /x`",
			es:     "GitLab no declara ningún permiso de grano fino para `GET /x`",
		},
	}
	english, spanish := englishReference(), spanishReference()
	for _, testCase := range cases {
		t.Run(string(testCase.denial.Cause)+" "+string(testCase.denial.Effect), func(t *testing.T) {
			if got := english.denialText(&testCase.denial); got != testCase.en {
				t.Errorf("English denialText = %q, want %q", got, testCase.en)
			}
			if got := spanish.denialText(&testCase.denial); got != testCase.es {
				t.Errorf("Spanish denialText = %q, want %q", got, testCase.es)
			}
		})
	}
}

// TestNeedsText_WordsEachWayOfRunningTheAction verifies what an action needs
// is worded way by way: one line for one way, "one of" for several, a request
// GitLab leaves to itself named as such, and a way that needs nothing said so
// rather than left blank. The Spanish page writes a boundary as the Spanish
// guide names it and leaves the permission names in GitLab's words.
func TestNeedsText_WordsEachWayOfRunningTheAction(t *testing.T) {
	read := finegrained.Need{Permissions: []string{"Project: Read"}, At: []string{"project", "group"}}
	write := finegrained.Need{Permissions: []string{"Issue: Create", "Label: Create"}, At: []string{"project"}}
	cases := []struct {
		name        string
		description finegrained.Description
		en, es      string
	}{
		{
			name: "denied", description: finegrained.Description{Denied: &finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"}},
			en: "Not reachable at this release: GitLab declares no fine-grained permission for `GET /x`",
			es: "No alcanzable en esta versión: GitLab no declara ningún permiso de grano fino para `GET /x`",
		},
		{
			name: "one way", description: finegrained.Description{AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read, write}}}},
			en: "Project: Read at project or group; Issue: Create, Label: Create at project",
			es: "Project: Read en proyecto o grupo; Issue: Create, Label: Create en proyecto",
		},
		{
			name: "several ways", description: finegrained.Description{AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read}}, {NotJudged: true}}},
			en: "one of: Project: Read at project or group **or** a request GitLab does not judge by the grant",
			es: "una de: Project: Read en proyecto o grupo **o** una petición que GitLab no juzga por la concesión",
		},
		{name: "nothing needed", description: finegrained.Description{AnyOf: []finegrained.Way{{}}}, en: "no permission", es: "ningún permiso"},
		{
			name: "a way no token takes",
			description: finegrained.Description{
				AnyOf:      []finegrained.Way{{Needs: []finegrained.Need{read}}},
				DeniedWays: []finegrained.Denial{{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace"}},
			},
			en: "Project: Read at project or group. With an input that sends another request instead, not reachable at this release: " +
				"GitLab declares no fine-grained permission for `Namespace`, which the answer is made of",
			es: "Project: Read en proyecto o grupo. Con una entrada que envía otra petición en su lugar, no alcanzable en esta versión: " +
				"GitLab no declara ningún permiso de grano fino para `Namespace`, del que está hecha la respuesta",
		},
		{
			name: "two ways no token takes",
			description: finegrained.Description{
				AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read}}, {Needs: []finegrained.Need{write}}},
				DeniedWays: []finegrained.Denial{
					{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace"},
					{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"},
				},
			},
			en: "one of: Project: Read at project or group **or** Issue: Create, Label: Create at project. " +
				"With an input that sends another request instead, not reachable at this release: " +
				"GitLab declares no fine-grained permission for `Namespace`, which the answer is made of **or** " +
				"GitLab declares no fine-grained permission for `GET /x`",
			es: "una de: Project: Read en proyecto o grupo **o** Issue: Create, Label: Create en proyecto. " +
				"Con una entrada que envía otra petición en su lugar, no alcanzable en esta versión: " +
				"GitLab no declara ningún permiso de grano fino para `Namespace`, del que está hecha la respuesta **o** " +
				"GitLab no declara ningún permiso de grano fino para `GET /x`",
		},
	}
	english, spanish := englishReference(), spanishReference()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := english.needsText(&testCase.description); got != testCase.en {
				t.Errorf("English needsText = %q, want %q", got, testCase.en)
			}
			if got := spanish.needsText(&testCase.description); got != testCase.es {
				t.Errorf("Spanish needsText = %q, want %q", got, testCase.es)
			}
		})
	}
}

// TestNeedText_WritesEachBoundaryInThePagesWords verifies every boundary GitLab
// declares is written in the Spanish guide's word for it, and that a boundary
// the page has no word for is written as GitLab names it rather than dropped.
// The English page writes every boundary as GitLab names it.
func TestNeedText_WritesEachBoundaryInThePagesWords(t *testing.T) {
	need := finegrained.Need{Permissions: []string{"Metadata: Read"}, At: []string{"project", "group", "user", "instance", "namespace"}}
	if got, want := englishReference().needText(need), "Metadata: Read at project or group or user or instance or namespace"; got != want {
		t.Errorf("English needText = %q, want %q", got, want)
	}
	if got, want := spanishReference().needText(need), "Metadata: Read en proyecto o grupo o usuario o instancia o namespace"; got != want {
		t.Errorf("Spanish needText = %q, want %q", got, want)
	}
}

// TestServedEmptyText_NamesWhatAGrantStillLeavesOut verifies the parts of an
// answer a fine-grained token is served empty are named, each as the GraphQL
// selection the description carries: always, or unless the grant also holds
// what follows them, in each language.
func TestServedEmptyText_NamesWhatAGrantStillLeavesOut(t *testing.T) {
	description := finegrained.Description{
		AlwaysEmpty: []string{"project { branchRules { nodes } }", "vulnerability"},
		EmptyWithout: []finegrained.Position{{
			Selection: "issue { author }", Needs: []finegrained.Need{{Permissions: []string{"User: Read"}, At: []string{"user"}}},
		}},
	}
	cases := []struct {
		language *referenceLanguage
		want     string
	}{
		{englishReference(), "`project { branchRules { nodes } }` (always), `vulnerability` (always), `issue { author }` (without User: Read at user)"},
		{spanishReference(), "`project { branchRules { nodes } }` (siempre), `vulnerability` (siempre), `issue { author }` (sin User: Read en usuario)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.language.title, func(t *testing.T) {
			if got := testCase.language.servedEmptyText(&description); got != testCase.want {
				t.Errorf("servedEmptyText = %q, want %q", got, testCase.want)
			}
			if got := testCase.language.servedEmptyText(&finegrained.Description{}); got != "" {
				t.Errorf("servedEmptyText of nothing = %q, want empty", got)
			}
		})
	}
}

// referenceTable is a small table the page tests render: two actions of one
// domain that read an issue, and one of another that no token reaches.
func referenceTable() *finegrained.Table {
	return &finegrained.Table{
		Version:     "19.4.1-ee",
		Permissions: []string{"read_issue"},
		Displays:    []string{"", "Issue: Read"},
		Display:     []uint16{1},
		Groups:      []finegrained.Group{{Perms: []uint16{0}, Any: finegrained.BoundaryProject}},
		Operations:  []finegrained.Operation{{Name: "GET /projects/:id/issues", Groups: []uint32{0}}},
		Actions: []finegrained.Requirement{
			{ID: "issue.get", Paths: [][]uint32{{0}}},
			{ID: "issue.list", Paths: [][]uint32{{0}}},
			{ID: "admin.thing", Denied: &finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"}},
		},
	}
}

// TestRenderReference_OneTablePerDomain verifies each language's page names
// the release it describes and holds one table per domain, in order, under
// that language's column headings, each action on its own row.
func TestRenderReference_OneTablePerDomain(t *testing.T) {
	cases := []struct {
		language *referenceLanguage
		release  string
		rows     []string
	}{
		{
			language: englishReference(),
			release:  "as GitLab 19.4.1 declares it",
			rows: []string{
				"| Action       | Needs                  | Served empty |",
				"| `issue.get`  | Issue: Read at project |", "| `issue.list` | Issue: Read at project |",
				"Not reachable at this release",
			},
		},
		{
			language: spanishReference(),
			release:  "tal como lo declara GitLab 19.4.1",
			rows: []string{
				"| Acción       | Necesita                | Se sirve vacío |",
				"| `issue.get`  | Issue: Read en proyecto |", "| `issue.list` | Issue: Read en proyecto |",
				"No alcanzable en esta versión",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.language.title, func(t *testing.T) {
			page := string(testCase.language.render(referenceTable()))
			if !strings.Contains(page, testCase.release) {
				t.Error("the page does not name the release it describes")
			}
			admin, issue := strings.Index(page, "## `admin`"), strings.Index(page, "## `issue`")
			if admin < 0 || issue < admin || strings.Count(page, "## `") != 2 {
				t.Errorf("the domains are not one table each in order:\n%s", page)
			}
			for _, row := range testCase.rows {
				if !strings.Contains(page, row) {
					t.Errorf("the page holds no %q:\n%s", row, page)
				}
			}
		})
	}
}

// TestRenderReference_IsASitePage verifies each page is one the documentation
// site takes: frontmatter whose title and description are one line each, the
// description naming the release, no heading of its own above the domains
// (the site makes the title the page's only top-level heading), and the
// generated-file note as an MDX comment, since MDX refuses an HTML one.
func TestRenderReference_IsASitePage(t *testing.T) {
	cases := []struct {
		language    *referenceLanguage
		title       string
		description string
	}{
		{englishReference(), "Fine-grained permissions", "as GitLab 19.4.1 declares it: the permissions"},
		{spanishReference(), "Permisos de grano fino", "tal como lo declara GitLab 19.4.1: los permisos"},
	}
	frontmatter := regexp.MustCompile(`\A---\ntitle: ([^\n]+)\ndescription: "([^"\n]+)"\n---\n\n`)
	for _, testCase := range cases {
		t.Run(testCase.title, func(t *testing.T) {
			page := string(testCase.language.render(referenceTable()))
			match := frontmatter.FindStringSubmatch(page)
			if match == nil || match[1] != testCase.title || !strings.Contains(match[2], testCase.description) {
				t.Fatalf("the page opens with no frontmatter naming %q and %q:\n%s", testCase.title, testCase.description, page)
			}
			body := page[len(match[0]):]
			if !strings.HasPrefix(body, "{/*Generated by cmd/gen_action_grants (make gen-action-grants); do not edit.*/}\n\n") {
				t.Errorf("the body does not open with the generated-file note as an MDX comment:\n%s", body)
			}
			if strings.Contains(page, "<!--") || strings.HasPrefix(body, "# ") || strings.Contains(body, "\n# ") {
				t.Errorf("the page carries an HTML comment or a heading of its own:\n%s", page)
			}
			if strings.Count(page, "%") != 0 {
				t.Errorf("the page carries a format verb left unfilled:\n%s", page)
			}
		})
	}
}

// TestReferenceLanguages_WriteTheSitePageAndItsSpanishTwin verifies the pages
// are the site's English reference page and its Spanish twin at the same path
// under es/, in that order, and that both lay out the same tables: the same
// domains, the same rows and the same action on each, which is what lets the
// site's checks hold the two pages to one layout.
func TestReferenceLanguages_WriteTheSitePageAndItsSpanishTwin(t *testing.T) {
	languages := referenceLanguages()
	if len(languages) != 2 {
		t.Fatalf("referenceLanguages = %d pages, want English and Spanish", len(languages))
	}
	english, spanish := languages[0], languages[1]
	if english.path != "site/src/content/docs/reference/fine-grained-permissions.mdx" ||
		spanish.path != "site/src/content/docs/es/reference/fine-grained-permissions.mdx" {
		t.Errorf("the pages are written to %q and %q", english.path, spanish.path)
	}
	// firstColumn is every domain heading and every row's action, in order.
	firstColumn := func(page []byte) []string {
		var rows []string
		for line := range strings.SplitSeq(string(page), "\n") {
			switch {
			case strings.HasPrefix(line, "## "):
				rows = append(rows, line)
			case strings.HasPrefix(line, "| `"):
				cell, _, _ := strings.Cut(line[1:], "|")
				rows = append(rows, strings.TrimSpace(cell))
			}
		}
		return rows
	}
	table := referenceTable()
	en, es := firstColumn(english.render(table)), firstColumn(spanish.render(table))
	if strings.Join(en, "\n") != strings.Join(es, "\n") || len(en) != 5 {
		t.Errorf("the pages lay out different tables:\nEnglish %q\nSpanish %q", en, es)
	}
}
