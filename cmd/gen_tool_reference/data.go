package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// localized is one text in every language of the reference, keyed by
// language code.
type localized map[string]string

// localizedList is one list of texts in every language, keyed by language
// code. Both languages hold the same number of entries, the nth of each
// being the same text.
type localizedList map[string][]string

// domains is the hand-written half of the reference, decoded from
// domains.json.
type domains struct {
	// Categories are the sections of the index page, in the order it shows
	// them.
	Categories []category `json:"categories"`
	// Groups is keyed by the catalog group's tool name.
	Groups map[string]domain `json:"groups"`
}

// category is one section of the index page.
type category struct {
	ID    string    `json:"id"`
	Title localized `json:"title"`
}

// domain is what the catalog cannot say about one group.
type domain struct {
	// Category is the ID of the index section the group is listed in.
	Category string `json:"category"`
	// Title is the page title and its sidebar label.
	Title localized `json:"title"`
	// Description is one sentence, the page's frontmatter description and
	// its line on the index.
	Description localized `json:"description"`
	// Overview is the page's opening, one Markdown paragraph per entry.
	Overview localizedList `json:"overview"`
	// Questions are requests a reader would make of the group.
	Questions localizedList `json:"questions"`
}

// decodeDomains parses the data file, refusing a field it does not know and a
// value after the object, and holds every text to being present and
// non-empty in each language.
func decodeDomains(raw []byte) (domains, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var data domains
	if err := decoder.Decode(&data); err != nil {
		return domains{}, fmt.Errorf("parse domains.json: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return domains{}, errors.New("parse domains.json: unexpected content after the object")
	}
	var problems []string
	categories := map[string]bool{}
	for _, c := range data.Categories {
		categories[c.ID] = true
		problems = append(problems, c.Title.missing("category "+c.ID+" title")...)
	}
	used := map[string]bool{}
	for _, tool := range data.sortedTools() {
		d := data.Groups[tool]
		used[d.Category] = true
		if !categories[d.Category] {
			problems = append(problems, fmt.Sprintf("%s: unknown category %q", tool, d.Category))
		}
		problems = append(problems, d.Title.missing(tool+" title")...)
		problems = append(problems, d.Description.missing(tool+" description")...)
		problems = append(problems, d.Overview.missing(tool+" overview")...)
		problems = append(problems, d.Questions.missing(tool+" questions")...)
	}
	for _, c := range data.Categories {
		if !used[c.ID] {
			problems = append(problems, fmt.Sprintf("category %s lists no group", c.ID))
		}
	}
	if len(problems) > 0 {
		return domains{}, fmt.Errorf("domains.json: %s", strings.Join(problems, "; "))
	}
	return data, nil
}

// sortedTools is the data file's group names in order.
func (d domains) sortedTools() []string {
	tools := make([]string, 0, len(d.Groups))
	for tool := range d.Groups {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

// missing names each language the text is absent or blank in.
func (l localized) missing(what string) []string {
	var problems []string
	for _, language := range languages {
		if strings.TrimSpace(l[language.code]) == "" {
			problems = append(problems, fmt.Sprintf("%s has no %s text", what, language.code))
		}
	}
	return problems
}

// missing names each language the list is empty in, holds a blank entry in,
// or holds a different number of entries in than English.
func (l localizedList) missing(what string) []string {
	var problems []string
	for _, language := range languages {
		entries := l[language.code]
		if len(entries) == 0 {
			problems = append(problems, fmt.Sprintf("%s has no %s entries", what, language.code))
			continue
		}
		if len(entries) != len(l[languages[0].code]) {
			problems = append(problems, fmt.Sprintf("%s has %d %s entries and %d %s ones", what, len(entries), language.code, len(l[languages[0].code]), languages[0].code))
		}
		for _, entry := range entries {
			if strings.TrimSpace(entry) == "" {
				problems = append(problems, fmt.Sprintf("%s has a blank %s entry", what, language.code))
			}
		}
	}
	return problems
}

// covers fails unless the data file describes exactly the groups the catalog
// builds, naming each group it lacks and each entry no group answers to.
func (d domains) covers(groups []*refGroup) error {
	built := map[string]bool{}
	var problems []string
	for _, group := range groups {
		built[group.tool] = true
		if _, ok := d.Groups[group.tool]; !ok {
			problems = append(problems, fmt.Sprintf("the catalog builds %s and domains.json does not describe it", group.tool))
		}
	}
	for _, tool := range d.sortedTools() {
		if !built[tool] {
			problems = append(problems, fmt.Sprintf("domains.json describes %s and the catalog builds no such group", tool))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
