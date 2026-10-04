package derive

import (
	"crypto/sha256"
	"encoding/hex"
)

// Kind is what a request is on the wire, or that it could not be read.
type Kind string

// The kinds of request.
const (
	// KindREST is a REST route.
	KindREST Kind = "rest"
	// KindGraphQL is one GraphQL document posted to the GraphQL endpoint.
	KindGraphQL Kind = "graphql"
	// KindUnresolved is a request the walk reached and could not read: a path
	// that does not fold, a client-go document assembled at run time. It is
	// carried rather than dropped, so a reader decides what it means and a
	// declaration can answer it.
	KindUnresolved Kind = "unresolved"
)

// Request is one request an action can make.
type Request struct {
	Kind Kind
	// Method and Path are a REST route's verb and path, with every identifier
	// segment collapsed to ":", which is how cmd/internal/sdkroutes spells a
	// client-go route.
	Method string
	Path   string
	// Document is a GraphQL request's text, as GitLab receives it, and Name
	// the constant or client-go variable it is declared as ("" when it is
	// written inline).
	Document string
	Name     string
	// Reason names an unresolved request's class and subject
	// ("raw-path invites.inviteProject"), which is what a declaration
	// answering it names.
	Reason string
}

// Key identifies the request among every action's: the route for a REST one,
// the document's digest for a GraphQL one, the reason for an unresolved one.
func (r Request) Key() string {
	switch r.Kind {
	case KindREST:
		return r.Method + " " + r.Path
	case KindGraphQL:
		return "graphql " + Digest(r.Document)
	default:
		return "unresolved " + r.Reason
	}
}

// Digest names a document by the first twelve hex digits of its SHA-256, which
// is short enough to read and long enough that two documents of this tree do
// not meet.
func Digest(document string) string {
	sum := sha256.Sum256([]byte(document))
	return hex.EncodeToString(sum[:])[:12]
}

// SDK is what the derivation asks of client-go: the requests one service
// method can send.
type SDK interface {
	// Requests answers the requests a method sends on one call, keyed the way
	// a handler names it ("Issues.GetIssue"). Routes are alternatives (a call
	// sends one of them) and documents are all posted; known is false for a
	// method client-go does not declare, and a known method may send nothing
	// (one that only formats a URL).
	Requests(method string) (routes, documents []Request, known bool)
}
