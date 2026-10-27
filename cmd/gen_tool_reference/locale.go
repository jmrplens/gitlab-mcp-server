// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/grantwords"
)

// language is one language the reference is written in: where its pages go,
// where the site serves them, and every word the generator writes in it.
// Every link a page writes into the site points at a page of its own
// language. What a fine-grained token needs is written in the words of
// cmd/internal/grantwords, which the fine-grained permissions page is written
// in too, so a reader meets one sentence for one action on both pages.
//
// The table headers of a language are held to the length of their English
// twins, and a translated table cell to the length of its header, so that a
// table measures the same in both languages (see the package comment). That
// is why the Spanish "Nivel" has an English "Tier" beside it only in a column
// whose cells are always longer, and why the column the English pages head
// "Mandatory" is not headed "Required".
type language struct {
	code string
	dir  string
	// site is the path the site serves this language's pages under, which a
	// page links each of them by.
	site string

	generatedNote string

	indexTitle       string
	indexLabel       string
	indexDescription string
	indexIntro       string
	indexTotals      string

	headingQuestions    string
	headingCall         string
	headingAvailability string
	headingActions      string

	callDynamic           string
	callMeta              string
	callIndividual        string
	callStandaloneSurface string

	servedEverywhere string
	servedIntro      string
	servedTier       string
	servedSplit      string
	scope            string
	scopeJoin        string
	capabilities     map[string]string
	readOnlyCount    string

	actionsIntro     string
	requirementIntro string
	destructiveNote  string
	paramTierNote    string

	columnAction     string
	columnTier       string
	columnIndividual string
	columnParameter  string
	columnType       string
	columnMandatory  string
	columnDesc       string
	yes              string
	no               string

	factMeta           string
	factMetaStandalone string
	factIndividual     string
	factNoIndividual   string
	factTier           string
	factDotcomOnly     string
	factBehavior       string

	// factClassic is the line of the scope a classic or OAuth token needs:
	// classicReadAPI or classicAPI, then classicGroupScopes naming the scopes
	// this server demands of the group's tokens before it lists the group,
	// then classicOtherAuthentication or classicNoRequest for an action whose
	// own requests GitLab does not judge by the token's scope, then
	// classicOAuthRefused, or classicOAuthRefusedSome when only some inputs
	// send them, naming the routes GitLab refuses to an OAuth token.
	factClassic                string
	classicReadAPI             string
	classicAPI                 string
	classicGroupScopes         string
	classicOtherAuthentication string
	classicNoRequest           string
	classicOAuthRefused        string
	classicOAuthRefusedSome    string
	// factFineGrained is the line of what a fine-grained token needs, in the
	// words' sentence, servedEmpty the clause naming the parts of the answer
	// it may be served empty, and permissionsLink the link to the action's
	// domain on the fine-grained permissions page.
	factFineGrained string
	servedEmpty     string
	permissionsLink string
	words           *grantwords.Words
	readOnly        string
	writes          string
	destructive     string
	idempotent      string
	notIdempotent   string
	noParameters    string
	// oneOf is the sentence naming an action's alternative sets, of which a
	// call needs at least one, and exactlyOneOf the one naming exclusive
	// sets, of which it needs exactly one.
	oneOf        string
	exactlyOneOf string
	oneOfJoin    string
	oneOfAnd     string
}

// siteRoot is the path the documentation site is served under. The English
// pages are its root, and the pages of every other language sit below it
// under a prefix of their own.
//
// A Spanish page writes the Spanish path itself (issue 1285). The site's
// build would rewrite an English path on a Spanish page into the Spanish one
// (remarkLocaleLinks in site/astro.config.mjs), so the rendered site was not
// what sent a reader astray; the page source was, which named a page its
// reader never lands on, and nothing held a hand-written overview in
// domains.json to its own language. The fine-grained permissions page that
// cmd/gen_action_grants writes spells its Spanish paths the same way.
const siteRoot = "/gitlab-mcp-server/"

// siteLink finds the target of a link to a page of the site, whose path
// begins with siteRoot: a Markdown link with or without a title, one in angle
// brackets, a reference definition, an autolink, an href, each root-relative
// or on the site's host, and a bare URL on the site's host, which the site's
// Markdown turns into a link. CommonMark lets spaces or tabs, with at most one
// line ending among them, stand between a link's opening parenthesis or a
// definition's colon and its destination, so the pattern does too. A path
// named in prose or in code is not a link and is not read. The pattern spells
// the root and the host as literals so that nothing is computed when the
// package initializes.
var siteLink = regexp.MustCompile(`(?:(?:\]\([ \t]*(?:\r?\n[ \t]*)?<?|\]:[ \t]*(?:\r?\n[ \t]*)?|<|href=["'])(?:https://docs\.jmrp\.io)?|https://docs\.jmrp\.io)(/gitlab-mcp-server/[^\s)"'>]*)`)

// toolsPath is where a page links a page of this reference in l.
func (l language) toolsPath() string {
	return l.site + "reference/tools/"
}

// languageOf is the language whose pages a link into the site points at. The
// English pages are served at the site's root, so the prefix of every other
// language begins with theirs: a link is in another language when it begins
// with that language's prefix, and English otherwise.
func languageOf(target string) language {
	for _, lang := range languages[1:] {
		if strings.HasPrefix(target, lang.site) {
			return lang
		}
	}
	return languages[0]
}

// strays names each link text, the l text of what, makes into the pages of
// another language.
func (l language) strays(what, text string) []string {
	var problems []string
	for _, match := range siteLink.FindAllStringSubmatch(text, -1) {
		if languageOf(match[1]).code != l.code {
			problems = append(problems, fmt.Sprintf("%s links %s from its %s text, a page of another language", what, match[1], l.code))
		}
	}
	return problems
}

// languages are the languages the reference is written in, English first:
// English is what a list in another language is counted against, and the
// language of every link into the site no other language's prefix claims.
var languages = []language{english, spanish}

// english and spanish are one table in two languages: the same fields in the
// same order is what makes them twins.
var english = language{
	code: "en",
	dir:  "site/src/content/docs/reference/tools",
	site: siteRoot,

	generatedNote: "{/*Generated by cmd/gen_tool_reference from the action catalog and cmd/gen_tool_reference/domains.json. Edit those, then run make gen-tool-reference.*/}",

	indexTitle:       "Tools by domain",
	indexLabel:       "Overview",
	indexDescription: "Every action of GitLab MCP Server by domain: its canonical ID, its meta-tool and individual tool, its tier, the token it needs, its annotations and its parameters.",
	indexIntro:       "One page per catalog group, each listing every action of the group with its canonical ID, the tools that reach it on each surface, the tier that serves it, the scope a classic or OAuth token needs for it and what a fine-grained token needs, its annotations and its parameters. The pages are generated from the catalog the server builds and from the requests each action sends, so what they say about an action is what the server serves; only the overview and the sample questions of a page are written by hand.",
	indexTotals:      "On GitLab.com at Ultimate the catalog holds %d actions; a self-managed Ultimate instance serves %d, a Premium one %d and a Free one %d.",

	headingQuestions:    "Sample questions",
	headingCall:         "How to call it",
	headingAvailability: "Availability",
	headingActions:      "Actions",

	callDynamic:           "- **Dynamic**, the default surface: call `gitlab_execute_action` with `action` set to the action's ID, such as `%s`, and its parameters in `params`. `gitlab_find_action` finds an ID from a description of the task.",
	callMeta:              "- **Meta** (`GITLAB_MCP_TOOL_SURFACE=meta`): call `%s` with `action` set to the action's name, such as `%s`, and its parameters in `params`.",
	callIndividual:        "- **Individual** (`GITLAB_MCP_TOOL_SURFACE=individual`): call the action's own tool, such as `%s`, with its parameters as the arguments.",
	callStandaloneSurface: "- **Meta** and **individual** (`GITLAB_MCP_TOOL_SURFACE=meta` or `individual`): each action is a tool of its own, such as `%s`, under the same name on both surfaces, with its parameters as the arguments.",

	servedEverywhere: "Every tier serves the whole group, on self-managed instances and on GitLab.com alike.",
	servedIntro:      "How many of these actions an instance serves at each tier, out of a total of %d:",
	servedTier:       "- **%s**: %d",
	servedSplit:      "- **%s**: %d on a self-managed instance, %d on GitLab.com",
	scope:            "Listed only for a token that carries %s: the server removes the group from every surface for a token whose scopes it knows and that lacks it.",
	scopeJoin:        " and ",
	capabilities: map[string]string{
		"elicitation": "Needs a client that supports MCP [elicitation](/gitlab-mcp-server/capabilities/elicitation/), which each action asks its questions through.",
	},
	readOnlyCount: "Read-only actions: %d of %d, the ones a deployment in read-only mode keeps. A token with `read_api` and not `api` is served %d of the %d.",

	actionsIntro:     "The description of each action, and of each of its parameters, is the text the server serves for it on the default surface, quoted as served.",
	requirementIntro: "The token lines say what GitLab %s requires of a token for the requests each action sends; on an instance with Admin Mode turned on, an action only an administrator may run also needs `admin_mode`.",
	destructiveNote:  "A destructive action runs only once confirmed, unless `GITLAB_MCP_YOLO_MODE` (or `AUTOPILOT`) skips that step: the dynamic surface needs `confirm: true` on `gitlab_execute_action`, and the other two take a `confirm` parameter or the client's prompt ([Destructive actions](/gitlab-mcp-server/operations/security/#destructive-actions)).",
	paramTierNote:    "A parameter followed by a tier in parentheses is served only from that tier on.",

	columnAction:     "Action",
	columnTier:       "Tier",
	columnIndividual: "Individual",
	columnParameter:  "Parameter",
	columnType:       "Type",
	columnMandatory:  "Mandatory",
	columnDesc:       "Description",
	yes:              "yes",
	no:               "no",

	factMeta:           "- **Meta-tool**: `%s`, action `%s`",
	factMetaStandalone: "- **Meta-tool**: `%s`, a tool of its own",
	factIndividual:     "- **Individual tool**: `%s`",
	factNoIndividual:   "- **Individual tool**: none, the individual surface does not serve this action",
	factTier:           "- **Tier**: %s",
	factDotcomOnly:     "%s, GitLab.com only",
	factBehavior:       "- **Behavior**: %s",

	factClassic:                "- **Classic or OAuth token**: %s",
	classicReadAPI:             "`read_api` (or `api`)",
	classicAPI:                 "`api`",
	classicGroupScopes:         "; this server lists the group only to a token that also carries %s",
	classicOtherAuthentication: "; GitLab authenticates the token this action sends as a parameter and does not judge this one's scope",
	classicNoRequest:           "; the action sends GitLab no request",
	classicOAuthRefused:        "; not an OAuth token, which GitLab refuses on %s",
	classicOAuthRefusedSome:    "; GitLab refuses an OAuth token on %s, which this action sends for some inputs",
	factFineGrained:            "- **Fine-grained token**: %s",
	servedEmpty:                "; served empty: %s",
	permissionsLink:            " ([permissions page](/gitlab-mcp-server/reference/fine-grained-permissions/#%s))",
	words:                      grantwords.English(),

	readOnly:      "read-only",
	writes:        "writes",
	destructive:   "destructive (needs confirmation)",
	idempotent:    "idempotent",
	notIdempotent: "not idempotent",
	noParameters:  "No parameters.",
	oneOf:         "Also needs at least one of: %s.",
	exactlyOneOf:  "Also needs exactly one of: %s.",
	oneOfJoin:     "; ",
	oneOfAnd:      " and ",
}

// spanish is the Spanish twin of english; see there.
var spanish = language{
	code: "es",
	dir:  "site/src/content/docs/es/reference/tools",
	site: "/gitlab-mcp-server/es/",

	generatedNote: "{/*Generado por cmd/gen_tool_reference a partir del catálogo de acciones y de cmd/gen_tool_reference/domains.json. Edita esos y ejecuta make gen-tool-reference.*/}",

	indexTitle:       "Herramientas por dominio",
	indexLabel:       "Descripción general",
	indexDescription: "Cada acción de GitLab MCP Server por dominio: su ID canónico, su meta-herramienta y su herramienta individual, su nivel, el token que necesita, sus anotaciones y sus parámetros.",
	indexIntro:       "Una página por grupo del catálogo, cada una con todas las acciones del grupo: su ID canónico, las herramientas que la alcanzan en cada superficie, el nivel que la sirve, el scope que necesita para ella un token clásico u OAuth y lo que necesita un token de grano fino, sus anotaciones y sus parámetros. Las páginas se generan a partir del catálogo que construye el servidor y de las peticiones que envía cada acción, así que lo que dicen de una acción es lo que sirve el servidor; solo la descripción general y las preguntas de ejemplo de cada página están escritas a mano.",
	indexTotals:      "En GitLab.com con Ultimate el catálogo tiene %d acciones; una instancia autogestionada sirve %d con Ultimate, %d con Premium y %d con Free.",

	headingQuestions:    "Preguntas de ejemplo",
	headingCall:         "Cómo llamarlo",
	headingAvailability: "Disponibilidad",
	headingActions:      "Acciones",

	callDynamic:           "- **Dinámica**, la superficie predeterminada: llama a `gitlab_execute_action` con `action` igual al ID de la acción, como `%s`, y sus parámetros en `params`. `gitlab_find_action` encuentra un ID a partir de una descripción de la tarea.",
	callMeta:              "- **Meta** (`GITLAB_MCP_TOOL_SURFACE=meta`): llama a `%s` con `action` igual al nombre de la acción, como `%s`, y sus parámetros en `params`.",
	callIndividual:        "- **Individual** (`GITLAB_MCP_TOOL_SURFACE=individual`): llama a la herramienta propia de la acción, como `%s`, pasándole directamente sus parámetros.",
	callStandaloneSurface: "- **Meta** e **individual** (`GITLAB_MCP_TOOL_SURFACE=meta` o `individual`): cada acción es una herramienta propia, como `%s`, con el mismo nombre en las dos superficies, a la que se le pasan directamente sus parámetros.",

	servedEverywhere: "Todos los niveles sirven el grupo completo, tanto en instancias autogestionadas como en GitLab.com.",
	servedIntro:      "Cuántas de estas acciones sirve una instancia en cada nivel, de un total de %d:",
	servedTier:       "- **%s**: %d",
	servedSplit:      "- **%s**: %d en una instancia autogestionada, %d en GitLab.com",
	scope:            "Solo se lista para un token que tenga %s: el servidor quita el grupo de todas las superficies a un token cuyos ámbitos conoce y que no lo tiene.",
	scopeJoin:        " y ",
	capabilities: map[string]string{
		"elicitation": "Necesita un cliente que admita la [elicitación](/gitlab-mcp-server/es/capabilities/elicitation/) de MCP, por la que cada acción hace sus preguntas.",
	},
	readOnlyCount: "Acciones de solo lectura: %d de %d, las que conserva un despliegue en modo de solo lectura. A un token con `read_api` y sin `api` se le sirven %d de las %d.",

	actionsIntro:     "La descripción de cada acción, y la de cada uno de sus parámetros, es el texto que sirve el servidor para ella en la superficie predeterminada, citado tal cual; por eso está en inglés.",
	requirementIntro: "Las líneas de token dicen lo que exige GitLab %s a un token para las peticiones que envía cada acción; en una instancia con Admin Mode activado, una acción que solo puede ejecutar un administrador necesita además `admin_mode`.",
	destructiveNote:  "Una acción destructiva solo se ejecuta una vez confirmada, salvo que `GITLAB_MCP_YOLO_MODE` (o `AUTOPILOT`) se salte ese paso: la superficie dinámica necesita `confirm: true` en `gitlab_execute_action`, y las otras dos aceptan un parámetro `confirm` o la pregunta del cliente ([Acciones destructivas](/gitlab-mcp-server/es/operations/security/#acciones-destructivas)).",
	paramTierNote:    "Un parámetro seguido de un nivel entre paréntesis solo se sirve a partir de ese nivel.",

	columnAction:     "Acción",
	columnTier:       "Nivel",
	columnIndividual: "Individual",
	columnParameter:  "Parámetro",
	columnType:       "Tipo",
	columnMandatory:  "Requerido",
	columnDesc:       "Descripción",
	yes:              "sí",
	no:               "no",

	factMeta:           "- **Meta-herramienta**: `%s`, acción `%s`",
	factMetaStandalone: "- **Meta-herramienta**: `%s`, una herramienta propia",
	factIndividual:     "- **Herramienta individual**: `%s`",
	factNoIndividual:   "- **Herramienta individual**: ninguna, la superficie individual no sirve esta acción",
	factTier:           "- **Nivel**: %s",
	factDotcomOnly:     "%s, solo GitLab.com",
	factBehavior:       "- **Comportamiento**: %s",

	factClassic:                "- **Token clásico u OAuth**: %s",
	classicReadAPI:             "`read_api` (o `api`)",
	classicAPI:                 "`api`",
	classicGroupScopes:         "; este servidor solo lista el grupo a un token que lleve además %s",
	classicOtherAuthentication: "; GitLab autentica el token que esta acción envía como parámetro y no juzga el scope de este",
	classicNoRequest:           "; la acción no envía ninguna petición a GitLab",
	classicOAuthRefused:        "; no un token OAuth, que GitLab rechaza en %s",
	classicOAuthRefusedSome:    "; GitLab rechaza un token OAuth en %s, que esta acción envía con algunas entradas",
	factFineGrained:            "- **Token de grano fino**: %s",
	servedEmpty:                "; se sirve vacío: %s",
	permissionsLink:            " ([página de permisos](/gitlab-mcp-server/es/reference/fine-grained-permissions/#%s))",
	words:                      grantwords.Spanish(),

	readOnly:      "solo lectura",
	writes:        "escribe",
	destructive:   "destructiva (necesita confirmación)",
	idempotent:    "idempotente",
	notIdempotent: "no idempotente",
	noParameters:  "Sin parámetros.",
	oneOf:         "Además necesita al menos uno de estos: %s.",
	exactlyOneOf:  "Además necesita exactamente uno de estos: %s.",
	oneOfJoin:     "; ",
	oneOfAnd:      " y ",
}
