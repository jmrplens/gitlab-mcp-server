// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// RawRequest is one request a function of the tool packages builds with one
// of client-go's request constructors rather than through a service method,
// as the upstream waits walk reads it, so R-PARAM can walk its options the
// way it walks a method's.
type RawRequest struct {
	// Site is the function that builds it, spelled package.Function.
	Site string
	// Constructor is the client-go constructor: NewRequest,
	// NewRequestToURL or UploadRequest.
	Constructor string
	// Verb is the verb when it is a constant, and empty otherwise.
	Verb string
	// Options is the static type of the options the request carries,
	// spelled with each package's name ("*gitlab.ImportFileOptions"), and
	// empty for nil. A function that forwards its own options parameter
	// spells the interface here, and Callers say what each call hands it.
	Options string
	// UploadField is, for an UploadRequest, the form field its file is sent
	// under when the call names it with a constant.
	UploadField string
	// Callers are, for a function that forwards its own verb or options
	// parameter, each call of it with what the request then carries.
	Callers []RawCaller
	// Sends are the structs of a tool package's own the request carries as
	// its options that embed a client-go struct, each with the keys it
	// carries beside that struct.
	Sends []waits.Superset
}

// RawCaller is one call of a function that forwards its verb or options to
// a raw request it builds.
type RawCaller struct {
	Site    string
	Verb    string
	Options string
}

// RawRequests loads the tool packages of the repository at root and returns
// every raw request their functions build, sorted by site, together with the
// directory of the client-go source the packages compile against.
func RawRequests(root string) (requests []RawRequest, clientGoDir string, err error) {
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		return nil, "", err
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		return nil, "", err
	}
	found, _, _ := walkToolWorkarounds(pkgs, clientGo)
	for _, raw := range found {
		if raw.Form != formRawRequest {
			continue
		}
		request := RawRequest{
			Site: raw.Site, Constructor: raw.Constructor, Verb: raw.Verb,
			Options: raw.Options, UploadField: raw.UploadField, Sends: raw.Sends,
		}
		for _, caller := range raw.Callers {
			request.Callers = append(request.Callers, RawCaller(caller))
		}
		requests = append(requests, request)
	}
	slices.SortStableFunc(requests, func(a, b RawRequest) int {
		return cmp.Or(strings.Compare(a.Site, b.Site), strings.Compare(a.Verb, b.Verb))
	})
	return requests, structs.ClientGoDir(pkgs), nil
}
