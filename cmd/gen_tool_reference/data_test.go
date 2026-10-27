// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// testDomains is a valid data file for the synthetic builds, as JSON.
func testDomains() map[string]any {
	group := func(category, title string) map[string]any {
		return map[string]any{
			"category":    category,
			"title":       map[string]any{"en": title, "es": title + " ES"},
			"description": map[string]any{"en": title + " described.", "es": title + " descrito."},
			"overview":    map[string]any{"en": []any{"About " + title + "."}, "es": []any{"Sobre " + title + "."}},
			"questions":   map[string]any{"en": []any{"Ask " + title}, "es": []any{"Pregunta " + title}},
		}
	}
	return map[string]any{
		"categories": []any{
			map[string]any{"id": "work", "title": map[string]any{"en": "Work", "es": "Trabajo"}},
			map[string]any{"id": "tools", "title": map[string]any{"en": "Tools", "es": "Herramientas"}},
		},
		"groups": map[string]any{
			"gitlab_widget": group("work", "Widgets"),
			"gitlab_helper": group("tools", "Helpers"),
		},
	}
}

// encode renders a data file as JSON, failing the test on an error.
func encode(t *testing.T, data map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

// mustDomains decodes the synthetic data file.
func mustDomains(t *testing.T) domains {
	t.Helper()
	data, err := decodeDomains(encode(t, testDomains()))
	if err != nil {
		t.Fatalf("decodeDomains() error = %v", err)
	}
	return data
}

func TestDecodeDomains_ValidFile_DecodesEveryGroup(t *testing.T) {
	data := mustDomains(t)
	if got := data.sortedTools(); strings.Join(got, ",") != "gitlab_helper,gitlab_widget" {
		t.Errorf("sortedTools() = %v", got)
	}
	if data.Groups["gitlab_widget"].Title["es"] != "Widgets ES" {
		t.Errorf("Spanish title = %q", data.Groups["gitlab_widget"].Title["es"])
	}
}

func TestDecodeDomains_InvalidFile_IsRefused(t *testing.T) {
	widget := func(d map[string]any) map[string]any {
		return d["groups"].(map[string]any)["gitlab_widget"].(map[string]any)
	}
	tests := []struct {
		name string
		// raw replaces the file when set; otherwise edit changes the valid
		// one and suffix follows it.
		raw    string
		edit   func(map[string]any)
		suffix string
		want   string
	}{
		{name: "not JSON", raw: "{", want: "parse domains.json"},
		{name: "unknown field", raw: `{"extra": 1}`, want: `unknown field "extra"`},
		{name: "trailing value", suffix: " {}", want: "unexpected content after the object"},
		{name: "unknown category", edit: func(d map[string]any) { widget(d)["category"] = "nowhere" }, want: `gitlab_widget: unknown category "nowhere"`},
		{name: "unused category", edit: func(d map[string]any) { widget(d)["category"] = "tools" }, want: "category work lists no group"},
		{name: "blank category title", edit: func(d map[string]any) {
			d["categories"].([]any)[0].(map[string]any)["title"] = map[string]any{"en": "Work", "es": " "}
		}, want: "category work title has no es text"},
		{name: "missing title", edit: func(d map[string]any) { widget(d)["title"] = map[string]any{"es": "Solo"} }, want: "gitlab_widget title has no en text"},
		{name: "missing description", edit: func(d map[string]any) { delete(widget(d), "description") }, want: "gitlab_widget description has no en text"},
		{name: "empty overview", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{}}
		}, want: "gitlab_widget overview has no es entries"},
		{name: "uneven questions", edit: func(d map[string]any) {
			widget(d)["questions"] = map[string]any{"en": []any{"One"}, "es": []any{"Una", "Dos"}}
		}, want: "gitlab_widget questions has 2 es entries and 1 en ones"},
		{name: "empty English list", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{}, "es": []any{""}}
		}, want: "gitlab_widget overview has a blank es entry"},
		{name: "blank question", edit: func(d map[string]any) {
			widget(d)["questions"] = map[string]any{"en": []any{"One"}, "es": []any{""}}
		}, want: "gitlab_widget questions has a blank es entry"},
		{name: "Spanish overview links an English page", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{
				"en": []any{"See [Orbit](/gitlab-mcp-server/tools/orbit/)."},
				"es": []any{"Mira [Orbit](/gitlab-mcp-server/tools/orbit/)."},
			}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/tools/orbit/ from its es text, a page of another language"},
		{name: "English description links a Spanish page", edit: func(d map[string]any) {
			widget(d)["description"] = map[string]any{"en": "[x](/gitlab-mcp-server/es/x/)", "es": "[x](/gitlab-mcp-server/es/x/)"}
		}, want: "gitlab_widget description links /gitlab-mcp-server/es/x/ from its en text, a page of another language"},
		{name: "Spanish question links an English page", edit: func(d map[string]any) {
			widget(d)["questions"] = map[string]any{"en": []any{"One"}, "es": []any{"[Una](/gitlab-mcp-server/x/)"}}
		}, want: "gitlab_widget questions links /gitlab-mcp-server/x/ from its es text, a page of another language"},
		{name: "Spanish title links an English page", edit: func(d map[string]any) {
			widget(d)["title"] = map[string]any{"en": "Widgets", "es": "[W](/gitlab-mcp-server/x/)"}
		}, want: "gitlab_widget title links /gitlab-mcp-server/x/ from its es text, a page of another language"},
		{name: "every stray link of a text is named", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{
				"en": []any{"One."},
				"es": []any{"[A](/gitlab-mcp-server/a/) y [B](/gitlab-mcp-server/b/)."},
			}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text, a page of another language; " +
			"gitlab_widget overview links /gitlab-mcp-server/b/ from its es text, a page of another language"},
		{name: "a link with a title", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{`[A](/gitlab-mcp-server/a/ "Título")`}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "a link with whitespace before its destination", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"Mira [Orbit]( /gitlab-mcp-server/tools/orbit/)."}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/tools/orbit/ from its es text"},
		{name: "a link whose destination starts the next line", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"Mira [Orbit](\n  /gitlab-mcp-server/tools/orbit/)."}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/tools/orbit/ from its es text"},
		{name: "an absolute link on the site's host", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"[A](https://docs.jmrp.io/gitlab-mcp-server/a/)"}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "a bare URL on the site's host", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"Ver https://docs.jmrp.io/gitlab-mcp-server/a/ ahora."}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "an autolink", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"<https://docs.jmrp.io/gitlab-mcp-server/a/>"}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "a reference definition", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"[A][a]\n\n[a]: /gitlab-mcp-server/a/"}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "a reference definition whose destination starts the next line", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{"[A][a]\n\n[a]:\n  /gitlab-mcp-server/a/"}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "an href", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"One."}, "es": []any{`<a href="/gitlab-mcp-server/a/">A</a>`}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/a/ from its es text"},
		{name: "an English link in angle brackets", edit: func(d map[string]any) {
			widget(d)["overview"] = map[string]any{"en": []any{"[A](</gitlab-mcp-server/es/a/>)"}, "es": []any{"Uno."}}
		}, want: "gitlab_widget overview links /gitlab-mcp-server/es/a/ from its en text"},
		{name: "category title links another language", edit: func(d map[string]any) {
			d["categories"].([]any)[0].(map[string]any)["title"] = map[string]any{"en": "Work", "es": "[T](/gitlab-mcp-server/x/)"}
		}, want: "category work title links /gitlab-mcp-server/x/ from its es text, a page of another language"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(tt.raw)
			if tt.raw == "" {
				d := testDomains()
				if tt.edit != nil {
					tt.edit(d)
				}
				raw = append(encode(t, d), tt.suffix...)
			}
			_, err := decodeDomains(raw)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("decodeDomains() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestDecodeDomains_LinksIntoTheirOwnLanguage_AreAccepted verifies a text that
// links the pages of its own language decodes: an English one the site's
// root, a Spanish one the pages under /gitlab-mcp-server/es/, and both may
// link outside the site.
func TestDecodeDomains_LinksIntoTheirOwnLanguage_AreAccepted(t *testing.T) {
	d := testDomains()
	d["groups"].(map[string]any)["gitlab_widget"].(map[string]any)["overview"] = map[string]any{
		"en": []any{"See [Orbit](/gitlab-mcp-server/tools/orbit/ \"Orbit\"), https://docs.jmrp.io/gitlab-mcp-server/ and [GitLab](https://docs.gitlab.com/)."},
		"es": []any{"Mira [Orbit](/gitlab-mcp-server/es/tools/orbit/ \"Orbit\"), https://docs.jmrp.io/gitlab-mcp-server/es/ y [GitLab](https://docs.gitlab.com/)."},
	}
	if _, err := decodeDomains(encode(t, d)); err != nil {
		t.Errorf("decodeDomains() error = %v, want links into their own language accepted", err)
	}
}

func TestCovers_DataAndCatalogDisagree_NamesEachSide(t *testing.T) {
	data := mustDomains(t)
	groups := []*refGroup{{tool: "gitlab_widget"}, {tool: "gitlab_newcomer"}}
	err := data.covers(groups)
	if err == nil {
		t.Fatal("covers() error = nil, want both disagreements named")
	}
	for _, want := range []string{
		"the catalog builds gitlab_newcomer and domains.json does not describe it",
		"domains.json describes gitlab_helper and the catalog builds no such group",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("covers() error = %v, want it to contain %q", err, want)
			}
		})
	}
	if sameErr := data.covers([]*refGroup{{tool: "gitlab_widget"}, {tool: "gitlab_helper"}}); sameErr != nil {
		t.Errorf("covers() of the same groups = %v, want nil", sameErr)
	}
}

func TestDomainsJSON_EmbeddedFile_DescribesExactlyTheGroupsTheCatalogBuilds(t *testing.T) {
	data, err := decodeDomains(domainsJSON)
	if err != nil {
		t.Fatalf("decodeDomains(domains.json) error = %v", err)
	}
	if coverErr := data.covers(mustRealReference(t).groups); coverErr != nil {
		t.Errorf("domains.json and the catalog disagree: %v", coverErr)
	}
}
