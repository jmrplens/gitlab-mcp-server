// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// presenter.go presents the credentials a verification bound is reached by.
//
// A bound on verification cannot be reached by a lane's own credential, which
// admission verified before the phase began and which the server answers from
// memory from then on. What reaches it is a credential the server has never
// seen: a new one a legitimate client was just issued, and a token nobody
// issued at all. This file hands out both, and sends the second from where a
// flood of them would come from.

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
)

// The token prefixes the stand-in GitLab tells apart. It accepts every token
// but an invented one, so a lane's own credential and every new one are valid,
// and an invented one is refused whatever it asks for.
const (
	newTokenPrefix      = "bench-new-"      //#nosec G101 -- not a credential, a stand-in the stub instance accepts
	inventedTokenPrefix = "bench-invented-" //#nosec G101 -- not a credential, a stand-in the stub instance refuses
)

// credentialCaller is a client that can send one request presenting a
// credential other than its own.
type credentialCaller interface {
	callAs(ctx context.Context, method string, params map[string]any, cred credential) ([]byte, error)
}

// floodSource is one transport source the flood sends from.
type floodSource interface {
	credentialCaller
	close()
}

// The ways a verb's request cannot be made, each a harness wired wrong rather
// than anything the server did, and filed as failures of the run.
var (
	errNoPresenter   = errors.New("this arm presents no credential but its lanes' own")
	errCannotPresent = errors.New("this client cannot present a credential other than its own")
	errCannotFetch   = errors.New("this client cannot fetch a document beside the endpoint")
	errNoTag         = errors.New("no tag was read for the document this verb revalidates")
)

// presenter hands out the credentials one arm presents in place of its lanes'
// own: a new valid token per call, and an invented token per call from a
// forwarded address and a transport source of its own. It also holds the tag
// each revalidated document was first served with.
type presenter struct {
	sources  []floodSource
	minted   atomic.Uint64
	invented atomic.Uint64
	// tags are written once, by learnTags before the arm is driven, and only
	// read after that.
	tags map[string]string
}

// newPresenter builds an arm's presenter against the endpoint it drives, with
// as many flood sources as the plan needs.
//
// One source is the default route, which is the quiet population's too: a
// flood small enough to need one stays under the transport-source budget on
// its own. More leave from addresses of their own in 127.2.0.0/16, the flood's
// alone. Linux answers every address in 127.0.0.0/8 on its loopback, so no
// setup is needed there; a platform that answers 127.0.0.1 alone refuses the
// dial, and every flood request fails and says why.
func newPresenter(plan fairnessPlan, endpoint string, bearer bool) *presenter {
	count := plan.floodSources()
	p := &presenter{tags: map[string]string{}}
	if count == 1 {
		shared := newHTTPRPC(endpoint, "")
		shared.bearer = bearer
		p.sources = append(p.sources, shared)
		return p
	}
	for index := range count {
		p.sources = append(p.sources, newSourcedHTTPRPC(endpoint, sourceAddress(index), bearer))
	}
	return p
}

// maxFloodSources is how many sources 127.2.0.0/16 holds once its network and
// broadcast addresses are set aside: 1<<16 - 2, which the test of
// sourceAddress holds by the last address it names.
const maxFloodSources = 65534

// sourceAddress is the loopback address the flood's source at index leaves
// from.
func sourceAddress(index int) string {
	host := index + 1
	return fmt.Sprintf("127.2.%d.%d", host>>8&0xff, host&0xff)
}

// forwardedAddress is the client address a trusted proxy names for the nth
// invented token, distinct for each of the first sixteen million, so no
// address fails twice and no per-address budget sees more than one.
func forwardedAddress(n uint64) string {
	return fmt.Sprintf("10.%d.%d.%d", n>>16&0xff, n>>8&0xff, n&0xff)
}

// mint is a new valid credential, one the server has never been shown.
func (p *presenter) mint() (string, error) {
	if p == nil {
		return "", errNoPresenter
	}
	return newTokenPrefix + strconv.FormatUint(p.minted.Add(1), 10), nil
}

// invent is the next invented token, the forwarded address it claims to come
// from, and the source it is sent through, taken in turn.
func (p *presenter) invent() (floodSource, credential, error) {
	if p == nil || len(p.sources) == 0 {
		return nil, credential{}, errNoPresenter
	}
	n := p.invented.Add(1)
	cred := credential{token: inventedTokenPrefix + strconv.FormatUint(n, 10), forwardedFor: forwardedAddress(n)}
	return p.sources[n%uint64(len(p.sources))], cred, nil
}

// learnTags reads, once, the entity tag of every document a verb revalidates,
// from the fetcher's own address, so a revalidation carries a tag the server
// issued and is answered 304, and no lane's bucket pays for the reading. A
// plan that revalidates nothing reads nothing.
func (p *presenter) learnTags(ctx context.Context, client rpcClient, verbIDs []string) error {
	for _, id := range verbIDs {
		verb := verbs[id]
		if !verb.Revalidate {
			continue
		}
		if _, read := p.tags[verb.Document]; read {
			continue
		}
		fetcher, ok := client.(documentFetcher)
		if !ok {
			return errCannotFetch
		}
		tag, err := fetcher.fetch(ctx, verb.Document, documentRequest{})
		if err != nil {
			return fmt.Errorf("reading the tag of %s: %w", verb.Document, err)
		}
		p.tags[verb.Document] = tag
	}
	return nil
}

// tag is the entity tag the document at path was first served with.
func (p *presenter) tag(path string) (string, error) {
	if p == nil {
		return "", errNoPresenter
	}
	tag, read := p.tags[path]
	if !read {
		return "", fmt.Errorf("%w: %s", errNoTag, path)
	}
	return tag, nil
}

// close releases every source's idle connections.
func (p *presenter) close() {
	for _, source := range p.sources {
		source.close()
	}
}

// present sends one request with the credential its verb names, and has a
// lane adopt a new credential once it was served.
//
// A request that presents the lane's own credential goes out as it always
// has, unless the lane adopted a new one, which it then presents instead: that
// is a client that was issued a token, presented it once and uses it from then
// on, and the request is one the server answers from memory either way.
func present(ctx context.Context, in paceInput, verb verbSpec) error {
	if verb.Document != "" {
		return fetchDocument(ctx, in, verb)
	}
	params := verb.params(in.call)
	switch verb.Credential {
	case credentialInvented:
		source, cred, err := in.present.invent()
		if err != nil {
			return err
		}
		_, err = source.callAs(ctx, verb.Method, params, cred)
		return err
	case credentialNew:
		caller, ok := in.conn.rpc.(credentialCaller)
		if !ok {
			return errCannotPresent
		}
		token, err := in.present.mint()
		if err != nil {
			return err
		}
		if _, err = caller.callAs(ctx, verb.Method, params, credential{token: token}); err != nil {
			return err
		}
		in.conn.adopt(token)
		return nil
	}
	if token := in.conn.current(); token != "" {
		// Only a credentialCaller ever adopts one, so the assertion holds.
		caller, _ := in.conn.rpc.(credentialCaller)
		_, err := caller.callAs(ctx, verb.Method, params, credential{token: token})
		return err
	}
	_, err := in.conn.rpc.call(ctx, verb.Method, params)
	return err
}

// fetchDocument sends one document verb's GET over the lane's own connections,
// from the address the lane arrives from, with the tag the document was first
// served with when the verb revalidates it. It presents no credential: these
// are the documents a caller reaches without one.
func fetchDocument(ctx context.Context, in paceInput, verb verbSpec) error {
	fetcher, ok := in.conn.rpc.(documentFetcher)
	if !ok {
		return errCannotFetch
	}
	request := documentRequest{forwardedFor: in.address}
	if verb.Revalidate {
		tag, err := in.present.tag(verb.Document)
		if err != nil {
			return err
		}
		request.ifNoneMatch = tag
	}
	_, err := fetcher.fetch(ctx, verb.Document, request)
	return err
}
