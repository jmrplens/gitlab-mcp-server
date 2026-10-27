// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flagged

import (
	"net/http"
	"net/http/httptest"
)

// NewServer starts a server the way a repository helper does, so the gate has
// to know the value it returns by its type rather than by the call that made
// it.
func NewServer() *httptest.Server {
	return httptest.NewServer(http.NotFoundHandler())
}

// Client is bound to an address when it is built, as a GitLab client is.
type Client struct{ base string }

// NewClient binds a client to base.
func NewClient(base string) *Client { return &Client{base: base} }

// Dial binds a client to base the way a constructor that can fail does.
func Dial(base string) (*Client, error) { return &Client{base: base}, nil }

// Get stands for a request sent to the address the client was built on.
func (c *Client) Get() string { return c.base }

// closedURL is the helper that hands out a closed server's address: the close
// and the use after it are both here, so this is where it is reported.
func closedURL() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return url
}

// closedLater is the same helper written as a function literal at package
// scope, reported under the variable's name.
var closedLater = func() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

// fakeGitLab is what a helper hands a test: a server and its address.
type fakeGitLab struct {
	srv *httptest.Server
	url string
}

// newFakeGitLab builds the pair in one literal, so the address reaches the
// test through a field the test's own function never assigns.
func newFakeGitLab() *fakeGitLab {
	s := httptest.NewServer(http.NotFoundHandler())
	return &fakeGitLab{srv: s, url: s.URL}
}

// deadURL is closedURL with the close deferred: the address it returns is
// handed out as the close runs.
func deadURL() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	return srv.URL
}

// deadURLLater is deadURL written as a function literal at package scope.
var deadURLLater = func() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	return srv.URL
}

// deadClient hands out a client built on the server whose close it defers,
// beside an error that carries nothing.
func deadClient() (*Client, error) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client, err := Dial(srv.URL)
	return client, err
}

// deadDial returns what a constructor built on the server whose close it
// defers, both of its results in one call.
func deadDial() (*Client, error) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	return Dial(srv.URL)
}

// deadNamedURL gives the address to its named result and returns it bare.
func deadNamedURL() (url string) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	url = srv.URL
	return
}
