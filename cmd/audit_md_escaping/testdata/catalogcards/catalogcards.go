// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package catalogcards

import (
	"context"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The fixture keys. Their templates are in the audit's test, which installs
// them in place of the shipped catalog's.
const (
	keySummary i18n.Key = "fixture.summary"
	keyLabels  i18n.Key = "fixture.labels"
	keyPair    i18n.Key = "fixture.pair_row"
	keyHeader  i18n.Key = "fixture.header"
	keyEmoji   i18n.Key = "fixture.emoji_row"
	keyAuthor  i18n.Key = "fixture.author"
	keyIDCell  i18n.Key = "fixture.id_cell"
	keyQuery   i18n.Key = "fixture.question"
)

// Result stands in for a struct filled from a GitLab response.
type Result struct {
	ID    string
	Name  string
	State string
}

//gitlab:allow-card Declared: a consent prompt whose rows a person approves, escaped by EscapeConsentValue.

// Declared writes a consent summary through the catalog, with an optional
// row joined after it; the directive above answers both rows.
func Declared(ctx context.Context, project, title, labels string) i18n.Text {
	return i18n.Concat(
		i18n.New(keySummary, i18n.S("project", toolutil.EscapeConsentValue(project)), i18n.S("title", toolutil.EscapeConsentValue(title))),
		i18n.New(keyLabels, i18n.S("labels", toolutil.EscapeConsentValue(labels))),
	).Text(ctx)
}

// Undeclared writes a two-cell row, the header of a field table and a row
// with an emoji before its label, none of which a directive answers.
func Undeclared(ctx context.Context, result Result) i18n.Text {
	return i18n.Concat(
		i18n.New(keyHeader),
		i18n.New(keyPair, i18n.S("name", toolutil.EscapeMdTableCell(result.Name))),
		i18n.New(keyEmoji, i18n.S("icon", "x"), i18n.S("state", toolutil.EscapeMdTableCell(result.State))),
	).Text(ctx)
}

// Author writes a value in emphasis, which is not a row.
func Author(ctx context.Context, result Result) i18n.Text {
	return i18n.T(ctx, keyAuthor, i18n.S("author", toolutil.EscapeMdTableCell(result.Name)))
}

//gitlab:allow-unescaped result.ID: a canonical catalog ID, compiled in rather than read from GitLab.

// Identifier puts a value a directive declares safe in a table cell.
func Identifier(ctx context.Context, result Result) i18n.Text {
	return i18n.T(ctx, keyIDCell, i18n.S("value", result.ID))
}

//gitlab:allow-card Gone: a consent prompt that used to be written as rows.

// Gone asks its question in prose and writes no row, so the directive above
// answers nothing.
func Gone(ctx context.Context, result Result) i18n.Text {
	return i18n.T(ctx, keyQuery, i18n.S("name", result.Name))
}
