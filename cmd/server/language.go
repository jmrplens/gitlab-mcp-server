// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// languageMiddleware puts the deployment's language on the context of every
// request, so the texts a handler sends a person (an elicitation's question
// and form, a progress message) are rendered in it.
//
// The language is the operator's, per process: no client sends the person's
// own today, and a credential carries none, so every credential of an HTTP
// deployment is served the same, as it is served the same --client-compat.
// When the per-request language proposal (SEP-2792) lands, this is the one
// place that changes: it reads the language a request declares, falls back to
// this one, and still keeps nothing on the session.
func languageMiddleware(lang i18n.Language) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			return next(i18n.WithLanguage(ctx, lang), method, req)
		}
	}
}
