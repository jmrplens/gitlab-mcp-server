// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkroutes

import (
	"slices"
	"sort"
	"strings"
)

// The two constructors a client-go method builds a REST request with, which
// decide how its options are encoded: NewRequest writes a PATCH, POST or PUT
// body with encoding/json and every other verb's query with go-querystring,
// and UploadRequest writes every option as a multipart form field with
// go-querystring whatever the verb.
const (
	ConstructorNewRequest = "NewRequest"
	ConstructorUpload     = "UploadRequest"
)

// Route is one REST request a client-go method sends.
type Route struct {
	// Method is the HTTP verb, uppercase.
	Method string
	// Path is the endpoint with a leading slash and every identifier segment
	// collapsed to the placeholder, which is the spelling client-go's own route
	// registry normalizes a template to.
	Path string
}

// String spells the route the way a report names it.
func (r Route) String() string { return r.Method + " " + r.Path }

// Method is one exported method of a client-go service, with what it sends.
type Method struct {
	// Service is the concrete service without its suffix, the way the
	// interface a handler calls it through is named without its own:
	// IssuesService behind IssuesServiceInterface is "Issues".
	Service string
	// Name is the method's own name.
	Name string
	// Answers is the struct the method's first result names, element type of a
	// slice included, and empty for a method answering with something that is
	// not a client-go struct of this package or with the pagination wrapper
	// alone.
	Answers string
	// Many is true when that result is a slice.
	Many bool
	// Options names the option structs the method takes, each once.
	Options []string
	// Routes is every REST request the method can send, sorted.
	Routes []Route
	// Constructors names, sorted, the constructors the method and what it
	// delegates to can build a REST request with: UploadRequest for a call
	// of do handed withUpload and for a legacy UploadRequest, NewRequest for
	// any other call of do and for a legacy NewRequest. A method that hands
	// do withUpload only under a condition (CreateGroup does when an avatar
	// is given) can build either, and names both.
	Constructors []string
	// UploadFields names, sorted, the form fields an upload of the method
	// sends its file under (UploadAvatar is "avatar", UploadFile "file"),
	// which no option struct field carries.
	UploadFields []string
	// GraphQL is true when the method, or something it delegates to, posts to
	// the GraphQL endpoint; [SDK.Documents] names the documents it posts.
	GraphQL bool
	// Unresolved names every legacy request the method can send whose path did
	// not fold to anything static, sorted. It is the reading's own blind spot,
	// kept on the method so a reader can say so rather than miss a request.
	Unresolved []string
}

// Key names the method the way a handler's call names it: "Issues.GetIssue".
func (m Method) Key() string { return m.Service + "." + m.Name }

// SDK is one reading of client-go's root package.
type SDK struct {
	methods map[string]Method
	keys    []string
	reading *reading
	// entries maps a method's key to the body it was resolved from, which
	// [SDK.Placements] reads a second time.
	entries map[string]*function
}

// Read parses the client-go root package in dir and resolves every exported
// service method to what it sends.
//
// It never fails: a directory that cannot be read yields an SDK with no
// methods, and a file that does not parse contributes nothing. Both are facts
// about the module cache rather than about client-go, and every reader here
// reports rather than gates on what it finds missing.
func Read(dir string) *SDK {
	parsed := parseDir(dir)
	sdk := &SDK{methods: map[string]Method{}, reading: parsed, entries: map[string]*function{}}
	for _, entry := range parsed.entries {
		method := parsed.resolve(entry)
		key := method.Key()
		sdk.methods[key] = method
		sdk.entries[key] = entry
		sdk.keys = append(sdk.keys, key)
	}
	sort.Strings(sdk.keys)
	return sdk
}

// Methods returns every exported service method, sorted by key.
func (s *SDK) Methods() []Method {
	out := make([]Method, 0, len(s.keys))
	for _, key := range s.keys {
		out = append(out, s.methods[key])
	}
	return out
}

// Method returns one method by key, and false for a key client-go does not
// declare.
func (s *SDK) Method(key string) (Method, bool) {
	method, ok := s.methods[key]
	return method, ok
}

// sortedUnique sorts a list and drops repeats, returning nil for an empty one
// so an empty result reads the same however it was built.
func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

// sortedRoutes sorts and deduplicates a route list the way [sortedUnique] does
// a string list.
func sortedRoutes(routes []Route) []Route {
	if len(routes) == 0 {
		return nil
	}
	slices.SortFunc(routes, func(a, b Route) int { return strings.Compare(a.String(), b.String()) })
	out := routes[:1]
	for _, route := range routes[1:] {
		if route != out[len(out)-1] {
			out = append(out, route)
		}
	}
	return out
}
