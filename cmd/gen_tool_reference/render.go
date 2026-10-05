package main

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// page is one generated file, its path relative to the repository root.
type page struct {
	path    string
	content []byte
}

// checkLabels fails for a client capability a group requires that a language
// has no sentence for, so a capability added to the catalog cannot render as
// an empty paragraph.
func checkLabels(groups []*refGroup) error {
	for _, group := range groups {
		for _, capability := range group.capabilities {
			for _, lang := range languages {
				if lang.capabilities[capability] == "" {
					return fmt.Errorf("%s needs the %s client capability, and the %s pages have no sentence for it", group.tool, capability, lang.code)
				}
			}
		}
	}
	return nil
}

// renderPages renders every page of the reference in every language: the
// index first, then one page per group.
func renderPages(ref reference, data domains) []page {
	targets := actionTargets(ref.groups)
	var pages []page
	for _, lang := range languages {
		order := sidebarOrder(data, lang)
		pages = append(pages, page{path: lang.dir + "/index" + pageExtension, content: []byte(renderIndex(ref, data, lang, order))})
		for _, group := range ref.groups {
			content := renderGroup(group, data.Groups[group.tool], lang, order[group.tool], targets)
			pages = append(pages, page{path: lang.dir + "/" + group.slug + pageExtension, content: []byte(content)})
		}
	}
	return pages
}

// sidebarOrder places the groups in the order of their titles in lang, after
// the index, which is first.
func sidebarOrder(data domains, lang language) map[string]int {
	tools := data.sortedTools()
	slices.SortStableFunc(tools, func(a, b string) int {
		return cmp.Compare(strings.ToLower(data.Groups[a].Title[lang.code]), strings.ToLower(data.Groups[b].Title[lang.code]))
	})
	order := make(map[string]int, len(tools))
	for position, tool := range tools {
		order[tool] = position + 1
	}
	return order
}

// actionTargets is the page each action is described on, by the slug of its
// group.
func actionTargets(groups []*refGroup) map[string]string {
	targets := map[string]string{}
	for _, group := range groups {
		for _, action := range group.actions {
			targets[action.id] = group.slug
		}
	}
	return targets
}

// link is where a page links an action to: the anchor of its heading, on
// the page itself when the action is described there.
func link(id, slug string, targets map[string]string) string {
	if targets[id] == slug {
		return "#" + anchor(id)
	}
	return pagePath + targets[id] + "/#" + anchor(id)
}

// anchor is the fragment the site gives a heading that spells id. The site
// takes it from github-slugger, which keeps letters, digits, underscores and
// hyphens and drops the rest; a canonical ID is lowercase words of those
// joined by one dot, so the dot is all it drops.
func anchor(id string) string {
	return strings.ReplaceAll(id, ".", "")
}

// frontmatter writes a page's YAML header. A group page keeps its table of
// contents to the sections, since a heading per action would list hundreds of
// entries; the index page takes the label its sidebar group gives an overview.
func frontmatter(b *strings.Builder, title, description string, order int, lang language) {
	b.WriteString("---\n")
	fmt.Fprintf(b, "title: %s\n", yamlString(title))
	fmt.Fprintf(b, "description: %s\n", yamlString(description))
	fmt.Fprintf(b, "sidebar:\n  order: %d\n", order)
	if order == 0 {
		fmt.Fprintf(b, "  label: %s\n", yamlString(lang.indexLabel))
	} else {
		b.WriteString("tableOfContents:\n  maxHeadingLevel: 2\n")
	}
	b.WriteString("---\n\n")
}

// yamlString quotes s as a YAML double-quoted scalar.
func yamlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// paragraph writes one block and the blank line after it.
func paragraph(b *strings.Builder, text string) {
	b.WriteString(text)
	b.WriteString("\n\n")
}

// renderIndex renders the page that lists every group, by category.
func renderIndex(ref reference, data domains, lang language, order map[string]int) string {
	var b strings.Builder
	frontmatter(&b, lang.indexTitle, lang.indexDescription, 0, lang)
	paragraph(&b, lang.generatedNote)
	paragraph(&b, lang.indexIntro)
	paragraph(&b, fmt.Sprintf(lang.indexTotals,
		ref.served[1][edition.Ultimate], ref.served[0][edition.Ultimate], ref.served[0][edition.Premium], ref.served[0][edition.Free]))
	tools := data.sortedTools()
	slices.SortStableFunc(tools, func(a, b string) int { return cmp.Compare(order[a], order[b]) })
	slugs := map[string]string{}
	for _, group := range ref.groups {
		slugs[group.tool] = group.slug
	}
	for _, c := range data.Categories {
		paragraph(&b, "## "+c.Title[lang.code])
		var entries []string
		for _, tool := range tools {
			d := data.Groups[tool]
			if d.Category == c.ID {
				entries = append(entries, fmt.Sprintf("- [%s](%s%s/): %s", d.Title[lang.code], pagePath, slugs[tool], d.Description[lang.code]))
			}
		}
		paragraph(&b, strings.Join(entries, "\n"))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// renderGroup renders the page of one group.
func renderGroup(group *refGroup, d domain, lang language, order int, targets map[string]string) string {
	var b strings.Builder
	frontmatter(&b, d.Title[lang.code], d.Description[lang.code], order, lang)
	paragraph(&b, lang.generatedNote)
	for _, text := range d.Overview[lang.code] {
		paragraph(&b, text)
	}
	paragraph(&b, "## "+lang.headingQuestions)
	questions := make([]string, 0, len(d.Questions[lang.code]))
	for _, question := range d.Questions[lang.code] {
		questions = append(questions, `- "`+question+`"`)
	}
	paragraph(&b, strings.Join(questions, "\n"))
	paragraph(&b, "## "+lang.headingCall)
	paragraph(&b, callLines(group, lang))
	paragraph(&b, "## "+lang.headingAvailability)
	for _, text := range availability(group, lang) {
		paragraph(&b, text)
	}
	paragraph(&b, "## "+lang.headingActions)
	paragraph(&b, actionsIntro(group, lang))
	paragraph(&b, summaryTable(group, lang))
	for _, action := range group.actions {
		writeAction(&b, action, lang, func(id string) string { return link(id, group.slug, targets) })
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// example is the action a page's call instructions are illustrated with: the
// first that has an individual tool, so all three surfaces show the same one.
func example(group *refGroup) *refAction {
	for _, action := range group.actions {
		if action.individual != "" {
			return action
		}
	}
	return group.actions[0]
}

// callLines says how a client reaches the group's actions on each surface.
func callLines(group *refGroup, lang language) string {
	sample := example(group)
	lines := []string{fmt.Sprintf(lang.callDynamic, sample.id)}
	if group.standalone {
		lines = append(lines, fmt.Sprintf(lang.callStandaloneSurface, sample.metaTool))
		return strings.Join(lines, "\n")
	}
	lines = append(lines,
		fmt.Sprintf(lang.callMeta, group.tool, sample.name),
		fmt.Sprintf(lang.callIndividual, sample.individual))
	return strings.Join(lines, "\n")
}

// availability says which tiers serve how many of the group's actions, what a
// token or a client needs for them, and how many of them only read.
func availability(group *refGroup, lang language) []string {
	total := len(group.actions)
	var blocks []string
	if everywhere(group) {
		blocks = append(blocks, lang.servedEverywhere)
	} else {
		lines := []string{fmt.Sprintf(lang.servedIntro, total)}
		for _, tier := range tiers {
			selfManaged, dotcom := group.served[0][tier], group.served[1][tier]
			if selfManaged == dotcom {
				lines = append(lines, fmt.Sprintf(lang.servedTier, tierNames[tier], selfManaged))
				continue
			}
			lines = append(lines, fmt.Sprintf(lang.servedSplit, tierNames[tier], selfManaged, dotcom))
		}
		blocks = append(blocks, lines[0], strings.Join(lines[1:], "\n"))
	}
	if len(group.scopes) > 0 {
		quoted := make([]string, 0, len(group.scopes))
		for _, scope := range group.scopes {
			quoted = append(quoted, "`"+scope+"`")
		}
		blocks = append(blocks, fmt.Sprintf(lang.scope, strings.Join(quoted, lang.scopeJoin)))
	}
	for _, capability := range group.capabilities {
		blocks = append(blocks, lang.capabilities[capability])
	}
	readOnly := 0
	for _, action := range group.actions {
		if action.readOnly {
			readOnly++
		}
	}
	return append(blocks, fmt.Sprintf(lang.readOnlyCount, readOnly, total))
}

// everywhere reports whether every build serves every action of the group.
func everywhere(group *refGroup) bool {
	for _, class := range group.served {
		for _, count := range class {
			if count != len(group.actions) {
				return false
			}
		}
	}
	return true
}

// actionsIntro opens the list of actions: whose words the descriptions are,
// and the notes the group's actions call for.
func actionsIntro(group *refGroup, lang language) string {
	text := lang.actionsIntro
	destructive, tiered := false, false
	for _, action := range group.actions {
		destructive = destructive || action.destructive
		for _, p := range action.params {
			tiered = tiered || p.tier > action.tier
		}
	}
	if destructive {
		text += " " + lang.destructiveNote
	}
	if tiered {
		text += " " + lang.paramTierNote
	}
	return text
}

// tierLabel is the tier an action is served from, with the GitLab.com mark a
// table cell carries when no self-managed instance serves it. Both read the
// same in every language.
func tierLabel(action *refAction) string {
	if action.dotcomOnly {
		return tierNames[action.tier] + " (GitLab.com)"
	}
	return tierNames[action.tier]
}

// summaryTable lists the group's actions. The tier column is left out when
// every action carries the same tier, which the availability section already
// says: a column that varies always holds a cell longer than its header, in
// either language.
func summaryTable(group *refGroup, lang language) string {
	tiersSeen := map[string]bool{}
	for _, action := range group.actions {
		tiersSeen[tierLabel(action)] = true
	}
	mixed := len(tiersSeen) > 1
	headers := []string{lang.columnAction, lang.columnIndividual}
	if mixed {
		headers = []string{lang.columnAction, lang.columnTier, lang.columnIndividual}
	}
	rows := make([][]string, 0, len(group.actions))
	for _, action := range group.actions {
		individual := ""
		if action.individual != "" {
			individual = "`" + action.individual + "`"
		}
		link := "[`" + action.id + "`](#" + anchor(action.id) + ")"
		row := []string{link, individual}
		if mixed {
			row = []string{link, tierLabel(action), individual}
		}
		rows = append(rows, row)
	}
	return strings.TrimRight(docgen.RenderMarkdownTable(headers, nil, rows), "\n")
}

// writeAction writes the section of one action: its served description, the
// tools that reach it, its tier and behavior, and its parameters. linkTo is
// where the page links another action to.
func writeAction(b *strings.Builder, action *refAction, lang language, linkTo func(string) string) {
	paragraph(b, "### `"+action.id+"`")
	paragraph(b, quote(action.description, linkTo))
	facts := []string{fmt.Sprintf(lang.factMeta, action.metaTool, action.name)}
	if action.standalone {
		facts = []string{fmt.Sprintf(lang.factMetaStandalone, action.metaTool)}
	}
	if action.individual == "" {
		facts = append(facts, lang.factNoIndividual)
	} else {
		facts = append(facts, fmt.Sprintf(lang.factIndividual, action.individual))
	}
	tier := tierNames[action.tier]
	if action.dotcomOnly {
		tier = fmt.Sprintf(lang.factDotcomOnly, tier)
	}
	facts = append(facts, fmt.Sprintf(lang.factTier, tier), fmt.Sprintf(lang.factBehavior, behavior(action, lang)))
	paragraph(b, strings.Join(facts, "\n"))
	if len(action.params) == 0 {
		paragraph(b, lang.noParameters)
	} else {
		paragraph(b, parameterTable(action, lang))
	}
	if len(action.oneOf) > 0 {
		sets := make([]string, 0, len(action.oneOf))
		for _, set := range action.oneOf {
			names := make([]string, 0, len(set))
			for _, name := range set {
				names = append(names, "`"+name+"`")
			}
			sets = append(sets, strings.Join(names, lang.oneOfAnd))
		}
		paragraph(b, fmt.Sprintf(lang.oneOf, strings.Join(sets, lang.oneOfJoin)))
	}
}

// behavior names an action's annotations in lang.
func behavior(action *refAction, lang language) string {
	parts := []string{lang.writes}
	if action.readOnly {
		parts = []string{lang.readOnly}
	}
	if action.destructive {
		parts = append(parts, lang.destructive)
	}
	if action.idempotent {
		return strings.Join(append(parts, lang.idempotent), ", ")
	}
	return strings.Join(append(parts, lang.notIdempotent), ", ")
}

// parameterTable lists an action's parameters. A parameter served from a
// higher tier than its action carries that tier after its name.
func parameterTable(action *refAction, lang language) string {
	headers := []string{lang.columnParameter, lang.columnType, lang.columnMandatory, lang.columnDesc}
	rows := make([][]string, 0, len(action.params))
	for _, p := range action.params {
		name := "`" + p.name + "`"
		if p.tier > action.tier {
			name += " (" + tierNames[p.tier] + ")"
		}
		mandatory := lang.no
		if p.required {
			mandatory = lang.yes
		}
		rows = append(rows, []string{name, inline(p.typ, true), mandatory, inline(p.description, true)})
	}
	return strings.TrimRight(docgen.RenderMarkdownTable(headers, nil, rows), "\n")
}

// quote renders a served description as a blockquote, every line of it, with
// the IDs of its "See also" clause linked to where each is described.
//
// A list in a description follows the line that introduces it, which
// Markdown reads as the same list either way; the quote sets it apart with a
// blank line on each side, which renders identically and is the form the
// repository's Markdown lint asks for.
func quote(description string, linkTo func(string) string) string {
	var lines []string
	inList := false
	for line := range strings.SplitSeq(description, "\n") {
		item := strings.HasPrefix(line, "- ")
		if item != inList && line != "" && len(lines) > 0 && lines[len(lines)-1] != ">" {
			lines = append(lines, ">")
		}
		inList = item
		lines = append(lines, strings.TrimRight("> "+quoteLine(line, linkTo), " "))
	}
	return strings.Join(lines, "\n")
}

// quoteLine escapes one line of a served description, linking the IDs of a
// "See also" clause in it.
func quoteLine(line string, linkTo func(string) string) string {
	var b strings.Builder
	last := 0
	for _, match := range actioncatalog.SeeAlsoClause.FindAllStringSubmatchIndex(line, -1) {
		b.WriteString(inline(line[last:match[0]], false))
		var links []string
		for id := range strings.SplitSeq(line[match[2]:match[3]], ", ") {
			links = append(links, "[`"+id+"`]("+linkTo(id)+")")
		}
		b.WriteString("See also: " + strings.Join(links, ", ") + ".")
		last = match[1]
	}
	b.WriteString(inline(line[last:], false))
	return b.String()
}

// textEscaper escapes what MDX would read as an expression or a tag, and what
// Markdown would read as emphasis, code, a link, a heading, a table cell
// boundary or a strikethrough, so a served text renders as the characters it
// holds.
var textEscaper = strings.NewReplacer(
	`\`, `\\`, "{", `\{`, "}", `\}`, "<", `\<`, ">", `\>`, "*", `\*`, "`", "\\`",
	"[", `\[`, "]", `\]`, "#", `\#`, "|", `\|`, "~", `\~`,
)

// bareURL and bareAddress find what Markdown would link on its own: an
// address with a scheme, and a mail address or the user@host part of an SSH
// remote. The page writes a web address as a link and the rest as code, so
// that what the reader gets does not depend on autolinking, and an escape
// added inside an address cannot cut it short.
var (
	bareURL     = regexp.MustCompile("[a-z][a-z0-9+.-]*://[^\\s<>()\\[\\]{}|`'\"]+")
	webURL      = regexp.MustCompile("^https?://")
	bareAddress = regexp.MustCompile("[A-Za-z0-9._-]+@[A-Za-z0-9-]+(?:\\.[A-Za-z0-9-]+)+(?:[:/][^\\s<>()\\[\\]{}|`'\"]*)?")
)

// trailingPunctuation ends a sentence rather than an address.
const trailingPunctuation = ".,;:!?"

// inline renders served text for a page, keeping its code spans as they are.
// A backtick without a partner opens no code span, so it is escaped with the
// rest; inside a table, a code span's pipes are escaped too, since the table
// is split into cells before any span is read.
func inline(text string, table bool) string {
	parts := strings.Split(text, "`")
	if len(parts)%2 == 0 {
		return prose(text)
	}
	for i, part := range parts {
		if i%2 == 0 {
			parts[i] = prose(part)
			continue
		}
		if table {
			parts[i] = strings.ReplaceAll(part, "|", `\|`)
		}
	}
	return strings.Join(parts, "`")
}

// prose renders text outside a code span: each web address a link to itself,
// labeled as code so the label is not linked a second time, any other
// address code, and the rest escaped.
func prose(text string) string {
	return replaceMatches(text, bareURL, func(url string) string {
		if webURL.MatchString(url) {
			return "[`" + url + "`](" + url + ")"
		}
		return code(url)
	}, func(rest string) string {
		return replaceMatches(rest, bareAddress, code, textEscaper.Replace)
	})
}

// code is text as a code span.
func code(text string) string {
	return "`" + text + "`"
}

// replaceMatches rewrites each match of pattern in text with wrap, less the
// punctuation that ends a sentence after it, and the text between them with
// rest.
func replaceMatches(text string, pattern *regexp.Regexp, wrap, rest func(string) string) string {
	var b strings.Builder
	last := 0
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		token := strings.TrimRight(text[match[0]:match[1]], trailingPunctuation)
		b.WriteString(rest(text[last:match[0]]))
		b.WriteString(wrap(token))
		last = match[0] + len(token)
	}
	b.WriteString(rest(text[last:]))
	return b.String()
}
