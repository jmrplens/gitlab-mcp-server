// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package tokengrantdata contains what the token create actions that take a
// fine-grained grant share: the input shape of one granular scope, the rules
// GitLab holds a grant to, checked before anything is sent, the schema
// overrides that publish them, and the hint for the 404 a granular call is
// answered with when the feature flag is off.
//
// GitLab declares the grant once (granular_scope_params in
// lib/api/helpers/personal_access_tokens_helpers.rb) for the current-user
// and the impersonation token routes, and client-go sends it as
// GranularScopeOptions on both, so the shape is declared once here. Each
// package keeps its handler and the literal that converts the grant into its
// own client-go options.
package tokengrantdata
