package gitlab

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ErrUnboundClient is what every request through an unbound client fails
// with. A handler that reports it was called through the shared, unbound
// copy of a catalog instead of through a copy bound to a credential, which
// is a wiring defect and never a user error.
var ErrUnboundClient = errors.New("gitlab client is unbound: the handler belongs to a shared catalog and was not bound to a credential")

// NewUnboundClient returns a client for baseURL that carries no credential
// and refuses every request with [ErrUnboundClient].
//
// It exists for the catalog cached per configuration and shared by every
// pooled server. The catalog is built once, through the same ActionSpecs
// functions a bound client would be handed, and those inspect the client only
// for the instance class (GitLab.com or self-managed), which the URL answers.
// Building it with a real client would capture that client's credential in a
// process-wide cache; building it with this one captures nothing that can
// reach GitLab, so a handler served from the shared copy by mistake fails
// closed instead of running under someone else's token.
//
// It panics when baseURL does not parse: the URL is a constant chosen by the
// caller, so a bad one is a programming error rather than a condition to
// handle.
func NewUnboundClient(baseURL string) *Client {
	c := &Client{
		baseURL:   baseURL,
		healthURL: strings.TrimRight(baseURL, "/") + versionAPIPath,
		unbound:   true,
	}
	c.maxResponse.Store(DefaultMaxResponseBytes)
	httpClient := &http.Client{Transport: unboundTransport{}}
	c.healthClient = httpClient
	inner, err := gl.NewClient("", gl.WithBaseURL(baseURL), gl.WithHTTPClient(httpClient), gl.WithoutRetries())
	if err != nil {
		panic(fmt.Sprintf("gitlab: unbound client for %q: %v", baseURL, err))
	}
	c.inner = inner
	return c
}

// IsUnbound reports whether this client is the credential-less one, which
// refuses every request with [ErrUnboundClient].
//
// It exists so a surface can refuse early and say why. A resource read or a
// completion resolved to this client has not failed at GitLab and never will
// reach it: the request could not be attributed to a credential, which is a
// wiring defect on this side. Asked after [Client.For], it is the question "did
// this request bring a credential", and answering it before the call is what
// keeps the caller from being told their token lacks access to something.
//
// A nil client is not unbound: it is no client at all, which every reader
// already handles.
func (c *Client) IsUnbound() bool {
	return c != nil && c.unbound
}

// unboundTransport refuses every request.
type unboundTransport struct{}

// RoundTrip implements [http.RoundTripper] by failing with [ErrUnboundClient].
func (unboundTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, ErrUnboundClient
}
