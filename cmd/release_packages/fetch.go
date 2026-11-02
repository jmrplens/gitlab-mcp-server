// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"time"
)

// Every request here talks to a third-party registry, so each one is
// bounded: a stalled response would otherwise hang the check until the
// workflow's own timeout fires, with no indication of which host went quiet.
// A download is given a longer window than a metadata call, since a bundle is
// tens of megabytes.
// The bounds are written without arithmetic, since a constant expression is
// not code a test can cover; TestBounds_AreCurlsBounds holds each to its
// value in seconds.
const (
	connectTimeout  time.Duration = 10e9
	metadataTimeout time.Duration = 60e9
	downloadTimeout time.Duration = 300e9
	// fetchRetries is how many times a request that failed in a way that may
	// pass is sent again, the two curl's --retry 2 gave every request.
	fetchRetries = 2
)

// retriedStatuses are the answers curl --retry sends a request again on.
var retriedStatuses = []int{
	http.StatusRequestTimeout, http.StatusTooManyRequests,
	http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout,
}

// fetcher sends the check's requests: each bounded, retried as curl
// --retry 2 --retry-connrefused retried it, and failed on any answer of 400
// or above, as curl -f failed it.
type fetcher struct {
	client *http.Client
	// retryDelay is the wait before the first retry, doubled before each
	// one after it; curl waits a second.
	retryDelay time.Duration
	// diagnostics is where a failed request is described, the line curl -S
	// printed.
	diagnostics io.Writer
	// spoolDir is the directory each download is written into, the system's
	// temporary directory when empty, where the script's mktemp -d put the
	// files its curl -o wrote.
	spoolDir string
}

// newFetcher returns a fetcher over the network, through the proxy the
// environment names as curl's was.
func newFetcher(diagnostics io.Writer) *fetcher {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: connectTimeout}).DialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: connectTimeout,
	}
	return &fetcher{client: &http.Client{Transport: transport}, retryDelay: time.Second, diagnostics: diagnostics}
}

// request is one request: its method, URL and headers, its time bound,
// whether it follows redirects (curl -L), and for a download the file its
// body is written into rather than held in memory (curl -o).
type request struct {
	method  string
	url     string
	headers map[string]string
	timeout time.Duration
	follow  bool
	into    *os.File
}

// response is what a request was answered with: its body, or for a request
// written into a file the size and SHA-256 of the body written there.
type response struct {
	body   []byte
	size   int64
	digest string
	header http.Header
}

// artifact is a download held in a file of its own. The answer's bytes are
// the file's first size bytes, and digest is their SHA-256 in lowercase hex,
// as releaseassets.Digest writes it.
type artifact struct {
	file   *os.File
	size   int64
	digest string
}

// reader reads the answer's bytes.
func (a *artifact) reader() *io.SectionReader { return io.NewSectionReader(a.file, 0, a.size) }

// close closes the file and removes it.
func (a *artifact) close() {
	_ = a.file.Close()
	_ = os.Remove(a.file.Name()) //#nosec G703 -- the file os.CreateTemp made for this download, named by it
}

// errNotAnswered is a request whose answer was a failure: the reason is on
// the diagnostics writer.
var errNotAnswered = errors.New("the request was not answered")

// fetch sends a request until it is answered or has failed fetchRetries
// times more, and describes a failure on the diagnostics writer.
func (f *fetcher) fetch(req request) (response, error) {
	delay := f.retryDelay
	for attempt := 0; ; attempt++ {
		got, status, err := f.once(req)
		if err == nil && status < http.StatusBadRequest {
			return got, nil
		}
		if attempt < fetchRetries && (err != nil || slices.Contains(retriedStatuses, status)) {
			time.Sleep(delay)
			delay *= 2
			continue
		}
		if err != nil {
			fmt.Fprintf(f.diagnostics, "  %s %s: %v\n", req.method, req.url, err)
		} else {
			fmt.Fprintf(f.diagnostics, "  %s %s: the server answered %d\n", req.method, req.url, status)
		}
		return response{}, errNotAnswered
	}
}

// once sends a request one time. A request with a file writes the body there
// from the file's start, so the answer's bytes are its first size bytes
// whatever an attempt before it left past them.
func (f *fetcher) once(req request) (response, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), req.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, req.method, req.url, http.NoBody)
	if err != nil {
		return response{}, 0, err
	}
	// What curl accepts unless told otherwise.
	httpReq.Header.Set("Accept", "*/*")
	for key, value := range req.headers {
		httpReq.Header.Set(key, value)
	}
	client := *f.client
	if !req.follow {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return response{}, 0, err
	}
	defer resp.Body.Close()
	if req.into != nil {
		hash := sha256.New()
		var size int64
		if size, err = io.Copy(io.MultiWriter(io.NewOffsetWriter(req.into, 0), hash), resp.Body); err != nil {
			return response{}, 0, err
		}
		return response{size: size, digest: hex.EncodeToString(hash.Sum(nil)), header: resp.Header}, resp.StatusCode, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return response{}, 0, err
	}
	return response{body: body, header: resp.Header}, resp.StatusCode, nil
}

// get fetches a metadata document.
func (f *fetcher) get(url string, follow bool, headers map[string]string) (response, error) {
	return f.fetch(request{method: http.MethodGet, url: url, headers: headers, timeout: metadataTimeout, follow: follow})
}

// download fetches an artifact, following redirects, into a file of its own
// in spoolDir, as curl -o wrote it, so an artifact is held on disk however
// large it is and never in memory. The caller closes what it returns, which
// removes the file; a download that fails leaves none.
func (f *fetcher) download(url string) (*artifact, error) {
	file, err := os.CreateTemp(f.spoolDir, "download-*")
	if err != nil {
		fmt.Fprintf(f.diagnostics, "  GET %s: %v\n", url, err)
		return nil, err
	}
	got, err := f.fetch(request{method: http.MethodGet, url: url, timeout: downloadTimeout, follow: true, into: file})
	downloaded := &artifact{file: file, size: got.size, digest: got.digest}
	if err != nil {
		downloaded.close()
		return nil, err
	}
	return downloaded, nil
}
