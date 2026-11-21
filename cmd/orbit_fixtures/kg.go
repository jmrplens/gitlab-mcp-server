// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// fixtureLabels are the labels of kg-fixtures, each with its color.
var fixtureLabels = []struct{ name, color string }{
	{"bug", "#d73a4a"},
	{"enhancement", "#a2eeef"},
	{"documentation", "#0075ca"},
	{"good first issue", "#7057ff"},
	{"ci", "#fbca04"},
	{"security", "#b60205"},
	{"fixture", "#0e8a16"},
}

// milestoneTitle is the title of the kg-fixtures milestone, written with the
// em dash GitLab.com holds it under, which a lookup by title has to match. The
// dash is spelled as its UTF-8 bytes, since make check-em-dash refuses the
// character in a line of source, and in one literal, so no operator joins
// constant parts that a mutation run would mutate and no test could reach.
const milestoneTitle = "v0.2 \xe2\x80\x94 KG coverage milestone"

// fixtureIssues are the issues of kg-fixtures, each with its labels.
var fixtureIssues = []struct{ title, labels string }{
	{"Add reserve_stock seed helper", "enhancement,good first issue"},
	{"Index additional entity types in graph_status", "fixture"},
	{"Document the Orbit query DSL", "documentation,fixture"},
	{"Trigger security scanning for security domain", "security,ci"},
	{"Tighten ruff rules in CI", "ci,enhancement"},
}

// fixtureEnvironments are the environments of kg-fixtures, each with its URL.
var fixtureEnvironments = []struct{ name, url string }{
	{"staging", "https://staging.example.com"},
	{"production", "https://example.com"},
}

// kgResources creates the labels, the milestone, the issues and the
// environments of kg-fixtures that do not exist yet, each found by its name
// or title first so a rerun appends nothing.
func (p *provisioner) kgResources(ctx context.Context) error {
	fmt.Fprintln(p.out, "    → labels")
	for _, label := range fixtureLabels {
		p.ensureLabel(ctx, label.name, label.color)
	}

	fmt.Fprintln(p.out, "    → milestone")
	milestoneID, err := p.ensureResource(ctx, fmt.Sprintf("/projects/%d/milestones?title=%s", p.kgID, url.PathEscape(milestoneTitle)), map[string]string{
		"title":       milestoneTitle,
		"description": "Track the fixtures that populate each Orbit entity type. Closes when the namespace reports coverage for ci, plan-Milestone, and security domains.",
		"due_date":    "2026-07-01",
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(p.out, "    ✓ milestone id=%d\n", milestoneID)

	fmt.Fprintln(p.out, "    → issues")
	for _, issue := range fixtureIssues {
		if _, err = p.ensureResource(ctx, fmt.Sprintf("/projects/%d/issues?search=%s", p.kgID, url.PathEscape(issue.title)), map[string]string{
			"title":       issue.title,
			"description": "Fixture issue for Knowledge Graph coverage. Used by gitlab-mcp-server Orbit tools to exercise the plan and ci domains.",
			"labels":      issue.labels,
		}); err != nil {
			return err
		}
	}

	fmt.Fprintln(p.out, "    → environments")
	for _, environment := range fixtureEnvironments {
		if _, err = p.ensureResource(ctx, fmt.Sprintf("/projects/%d/environments?name=%s", p.kgID, url.QueryEscape(environment.name)), map[string]string{
			"name":         environment.name,
			"external_url": environment.url,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ensureLabel creates the label unless one of exactly that name exists. What
// GitLab answers the creation is not read, as it never was: a label missing
// after the run costs a fixture filter, and the next run creates it.
func (p *provisioner) ensureLabel(ctx context.Context, name, color string) {
	var labels []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if p.api.do(ctx, "GET", fmt.Sprintf("/projects/%d/labels?search=%s", p.kgID, url.PathEscape(name)), nil).decode(&labels) {
		for _, label := range labels {
			if label.Name == name && label.ID > 0 {
				return
			}
		}
	}
	p.api.do(ctx, "POST", fmt.Sprintf("/projects/%d/labels", p.kgID), map[string]string{
		"name":        name,
		"color":       color,
		"description": "Fixture label for " + name + " testing.",
	})
}

// ensureResource returns the id of the first resource getPath finds, an
// object or the first of a list, and otherwise creates one by posting body to
// the collection in front of the query string: the query is what finds an
// existing resource, the collection is where a new one is made. A creation
// GitLab answers without an id stops the run, since going on would report a
// resource that was never created.
func (p *provisioner) ensureResource(ctx context.Context, getPath string, body any) (int64, error) {
	if id := firstID(p.api.do(ctx, "GET", getPath, nil)); id > 0 {
		return id, nil
	}
	postPath, _, _ := strings.Cut(getPath, "?")
	created := p.api.do(ctx, "POST", postPath, body)
	var object struct {
		ID int64 `json:"id"`
	}
	if !created.decode(&object) || object.ID <= 0 {
		return 0, fmt.Errorf("    ✗ POST %s did not create anything: %s", postPath, created)
	}
	return object.ID, nil
}

// firstID is the id an answer names: an object's, or the first element's of
// a list, and 0 when it names none.
func firstID(got answer) int64 {
	var raw json.RawMessage
	if !got.decode(&raw) {
		return 0
	}
	var object struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.ID
	}
	var list []struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return list[0].ID
	}
	return 0
}
