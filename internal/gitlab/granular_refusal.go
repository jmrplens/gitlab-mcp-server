package gitlab

import "strings"

// GranularRefusalKind names which of the sentences GitLab's fine-grained
// authorization writes a refusal is.
//
// GitLab refuses a fine-grained token in one service,
// Authz::Tokens::AuthorizeGranularScopesService
// (app/services/authz/tokens/authorize_granular_scopes_service.rb at
// v19.4.1-ee), and the service has four answers. Three are sentences, which
// the REST API guard sends as the error_description of a 403 carrying
// [GranularScopeRefusalCode] (lib/api/api_guard.rb) and a GraphQL mutation
// sends as one errors[] entry (lib/gitlab/graphql/authz/
// granular_scope_authorization.rb, authorize!). They differ in what the caller
// can do about them, which is why a reader tells them apart rather than
// answering all three with one sentence. The fourth is [GranularNotFound].
type GranularRefusalKind uint8

const (
	// GranularRefusalUnrecognized is a sentence none of the three below is.
	// It is the zero value, so a sentence nobody parsed is never read as one
	// GitLab wrote.
	GranularRefusalUnrecognized GranularRefusalKind = iota
	// GranularRefusalMissingPermissions is access_denied_error: "Access
	// denied: This operation requires a fine-grained <token type> with the
	// following <boundary> permissions: [<Resource>: <Action>, ...]." The
	// token lacks the permissions it lists, and a token's grant cannot be
	// changed after it is created.
	GranularRefusalMissingPermissions
	// GranularRefusalUnsupported is missing_inputs_error: "Access denied: This
	// operation doesn't support fine-grained <token type>s." GitLab declares
	// no fine-grained permission for the operation, which is what every route
	// without a declaration, and every one GitLab marks as still to do,
	// answers (lib/api/helpers.rb resolves no permission for either), so no
	// fine-grained token reaches it on this instance.
	GranularRefusalUnsupported
	// GranularRefusalDisabled is disabled_error: "Access denied: Fine-grained
	// <token type>s are not yet supported." The feature flag
	// granular_personal_access_tokens is off for the token's user, so every
	// call the token makes is refused.
	GranularRefusalDisabled
	// GranularRefusalNotFound is the service's fourth answer,
	// [GranularNotFound], read from a GraphQL errors[] entry: the object the
	// operation names was not found, or is outside what the token may see.
	// [ParseGranularRefusal] never answers it, because the text is GitLab's
	// ordinary 404 everywhere else; [ParseGraphQLGranularRefusal] does.
	GranularRefusalNotFound
)

// GranularNotFound is the fourth answer of GitLab's fine-grained
// authorization, its NOT_FOUND_MESSAGE: the boundary the operation names did
// not resolve, or resolved to one the token may not see.
//
// Over REST GitLab turns it into its ordinary 404 (not_found! in
// lib/api/helpers.rb), which says nothing a 404 does not already say. Over
// GraphQL a mutation raises it as the message of one errors[] entry and runs
// nothing, and a read discards it and answers null. The literal is written by
// nothing else in GitLab's GraphQL layer at v19.4.1-ee, so an errors[] entry
// that is exactly this is the service's, whichever token was refused; the
// same text anywhere else, client-go's own 404 sentinel among them, is not.
const GranularNotFound = "404 Not Found"

// GranularRefusal is one of GitLab's fine-grained refusal sentences, read into
// its parts.
type GranularRefusal struct {
	// Kind is which sentence it is.
	Kind GranularRefusalKind
	// TokenType is the token class as the sentence names it: singular in a
	// [GranularRefusalMissingPermissions] sentence ("personal access token")
	// and plural in the other two ("personal access tokens"), as GitLab
	// writes it.
	TokenType string
	// Boundary is the kind of boundary a [GranularRefusalMissingPermissions]
	// sentence names the permissions at ("project", "group", "user",
	// "instance", "personal projects"), and empty for the other kinds.
	Boundary string
	// Permissions are the permissions a [GranularRefusalMissingPermissions]
	// sentence lists, as GitLab wrote each ("Merge Request: Approve"): the
	// resource and action of the first assignable permission that expands to
	// the raw one refused, in GitLab's file order and deprecated ones
	// included, so a name here is not always one the token creation page
	// offers. Empty for the other kinds.
	Permissions []string
}

// The fixed parts of the three sentences, as
// authorize_granular_scopes_service.rb writes them at v19.4.1-ee.
const (
	granularMissingPrefix     = "Access denied: This operation requires a fine-grained "
	granularMissingBoundary   = " with the following "
	granularMissingList       = " permissions: ["
	granularMissingSuffix     = "]."
	granularUnsupportedPrefix = "Access denied: This operation doesn't support fine-grained "
	granularDisabledPrefix    = "Access denied: Fine-grained "
	granularDisabledSuffix    = " are not yet supported."
	granularSentenceEnd       = "."
)

// granularLabelMaxBytes bounds the two words of a sentence that name a class
// rather than list anything, the token type and the boundary. GitLab's are
// under thirty bytes; a longer one is not a sentence GitLab wrote, and it is
// the instance's text, which under --allow-any-gitlab-url is the caller's.
const granularLabelMaxBytes = 64

// ParseGranularRefusal reads sentence as one of GitLab's fine-grained refusals
// and reports which, with its parts. A sentence that is not one of them in
// full answers [GranularRefusalUnrecognized] and nothing else.
//
// It reads a whole message and never searches inside one: a sentence quoted
// within a longer text, a validation error echoing a title among them, is not
// GitLab refusing the call. Leading and trailing white space is ignored. The
// token type and the boundary must each be lower-case words separated by
// single spaces, which is what ActiveModel's human name and GitLab's boundary
// labels are, and a listed permission may be anything but empty.
//
// Nothing here bounds what it returns beyond the two labels: the permissions
// are the instance's text, and a reader that quotes them bounds and filters
// them first.
func ParseGranularRefusal(sentence string) GranularRefusal {
	sentence = strings.TrimSpace(sentence)
	if rest, ok := strings.CutPrefix(sentence, granularMissingPrefix); ok {
		return parseMissingPermissions(rest)
	}
	if rest, ok := strings.CutPrefix(sentence, granularUnsupportedPrefix); ok {
		if tokenType, ended := strings.CutSuffix(rest, granularSentenceEnd); ended && granularLabel(tokenType) {
			return GranularRefusal{Kind: GranularRefusalUnsupported, TokenType: tokenType}
		}
	}
	if rest, ok := strings.CutPrefix(sentence, granularDisabledPrefix); ok {
		if tokenType, ended := strings.CutSuffix(rest, granularDisabledSuffix); ended && granularLabel(tokenType) {
			return GranularRefusal{Kind: GranularRefusalDisabled, TokenType: tokenType}
		}
	}
	return GranularRefusal{}
}

// ParseGraphQLGranularRefusal is [ParseGranularRefusal] for the message of one
// GraphQL errors[] entry, which is where the service's fourth answer can be
// read too: a message that is exactly [GranularNotFound] answers
// [GranularRefusalNotFound]. The caller has established that message is one
// entry's whole message and nothing else.
func ParseGraphQLGranularRefusal(message string) GranularRefusal {
	if strings.TrimSpace(message) == GranularNotFound {
		return GranularRefusal{Kind: GranularRefusalNotFound}
	}
	return ParseGranularRefusal(message)
}

// parseMissingPermissions reads what follows the missing-permissions
// sentence's opening: "<token type> with the following <boundary> permissions:
// [<list>]."
func parseMissingPermissions(rest string) GranularRefusal {
	tokenType, rest, ok := strings.Cut(rest, granularMissingBoundary)
	if !ok || !granularLabel(tokenType) {
		return GranularRefusal{}
	}
	boundary, rest, ok := strings.Cut(rest, granularMissingList)
	if !ok || !granularLabel(boundary) {
		return GranularRefusal{}
	}
	list, ok := strings.CutSuffix(rest, granularMissingSuffix)
	if !ok {
		return GranularRefusal{}
	}
	permissions := strings.Split(list, ", ")
	for _, permission := range permissions {
		if strings.TrimSpace(permission) == "" {
			return GranularRefusal{}
		}
	}
	return GranularRefusal{
		Kind:        GranularRefusalMissingPermissions,
		TokenType:   tokenType,
		Boundary:    boundary,
		Permissions: permissions,
	}
}

// granularLabel reports whether s can be one of the two words a refusal names
// a class by: lower-case ASCII words separated by single spaces, at most
// [granularLabelMaxBytes] long.
func granularLabel(s string) bool {
	if s == "" || len(s) > granularLabelMaxBytes {
		return false
	}
	for word := range strings.SplitSeq(s, " ") {
		if word == "" {
			return false
		}
		for _, c := range []byte(word) {
			if c < 'a' || c > 'z' {
				return false
			}
		}
	}
	return true
}
