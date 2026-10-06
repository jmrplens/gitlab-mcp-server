package grantwords

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestDenialText_WordsEveryCause verifies the wording says, for each reason
// no fine-grained token reaches an action, what GitLab declares and what it
// does to the request: refused before anything runs, or written and answered
// null. Each cause is worded in both languages, with the element GitLab names
// left as GitLab names it.
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
	english, spanish := English(), Spanish()
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
// rather than left blank. The Spanish wording writes a boundary as the Spanish
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
	english, spanish := English(), Spanish()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := english.NeedsText(&testCase.description); got != testCase.en {
				t.Errorf("English NeedsText = %q, want %q", got, testCase.en)
			}
			if got := spanish.NeedsText(&testCase.description); got != testCase.es {
				t.Errorf("Spanish NeedsText = %q, want %q", got, testCase.es)
			}
		})
	}
}

// TestNeedText_WritesEachBoundaryInTheLanguagesWords verifies every boundary
// GitLab declares is written in the Spanish guide's word for it, and that a
// boundary the wording has no word for is written as GitLab names it rather
// than dropped. The English wording writes every boundary as GitLab names it.
func TestNeedText_WritesEachBoundaryInTheLanguagesWords(t *testing.T) {
	need := finegrained.Need{Permissions: []string{"Metadata: Read"}, At: []string{"project", "group", "user", "instance", "namespace"}}
	if got, want := English().needText(need), "Metadata: Read at project or group or user or instance or namespace"; got != want {
		t.Errorf("English needText = %q, want %q", got, want)
	}
	if got, want := Spanish().needText(need), "Metadata: Read en proyecto o grupo o usuario o instancia o namespace"; got != want {
		t.Errorf("Spanish needText = %q, want %q", got, want)
	}
}

// TestServedEmptyText_NamesWhatAGrantStillLeavesOut verifies the parts of an
// answer a fine-grained token is served empty are named, each as the GraphQL
// selection the description carries: always, or unless the grant also holds
// what follows them, in each language, and nothing for an answer served
// whole.
func TestServedEmptyText_NamesWhatAGrantStillLeavesOut(t *testing.T) {
	description := finegrained.Description{
		AlwaysEmpty: []string{"project { branchRules { nodes } }", "vulnerability"},
		EmptyWithout: []finegrained.Position{{
			Selection: "issue { author }", Needs: []finegrained.Need{{Permissions: []string{"User: Read"}, At: []string{"user"}}},
		}},
	}
	cases := []struct {
		name  string
		words *Words
		want  string
	}{
		{"English", English(), "`project { branchRules { nodes } }` (always), `vulnerability` (always), `issue { author }` (without User: Read at user)"},
		{"Spanish", Spanish(), "`project { branchRules { nodes } }` (siempre), `vulnerability` (siempre), `issue { author }` (sin User: Read en usuario)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.words.ServedEmptyText(&description); got != testCase.want {
				t.Errorf("ServedEmptyText = %q, want %q", got, testCase.want)
			}
			if got := testCase.words.ServedEmptyText(&finegrained.Description{}); got != "" {
				t.Errorf("ServedEmptyText of nothing = %q, want empty", got)
			}
		})
	}
}

// TestWords_HeadTheTwoColumnsInEachLanguage verifies the two column headings
// a requirement table carries, which the permissions page lays out and the
// tool reference does not, so a heading left empty would go unseen there.
func TestWords_HeadTheTwoColumnsInEachLanguage(t *testing.T) {
	cases := []struct {
		name                 string
		words                *Words
		needs, servedEmptyHd string
	}{
		{"English", English(), "Needs", "Served empty"},
		{"Spanish", Spanish(), "Necesita", "Se sirve vacío"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.words.Needs != testCase.needs || testCase.words.ServedEmpty != testCase.servedEmptyHd {
				t.Errorf("headings = %q and %q, want %q and %q", testCase.words.Needs, testCase.words.ServedEmpty, testCase.needs, testCase.servedEmptyHd)
			}
		})
	}
}
