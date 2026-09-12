// Package elicitationtools implements interactive MCP tools that gather missing
// values through the MCP elicitation capability before calling GitLab actions.
//
// The package orchestrates guided issue, merge request, project, and release
// creation over the same GitLab APIs used by the corresponding domain packages.
//
// The "**Label**: value" lines the wizards compose are not card rows and do not
// move to toolutil.Card. They are the message of an elicitation request — the
// question a person is shown in a dialog before they allow the creation — and
// not a tool result about a GitLab object, so there is no object whose fields
// they could be the rows of. Their containment is
// [github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil.EscapeConsentValue],
// which is stricter than a card row's escaper and specific to this channel: the
// specification says a form-mode elicitation request should carry no clickable
// URL in any field, so the value's line breaks collapse, its URL schemes are
// defanged, and it is fenced in backticks rather than entity-escaped. The
// rendered tool results of these wizards are cards, written by the domain
// package whose Output each wizard returns.
package elicitationtools
