// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package sdkoptions is the one reader of what client-go's option structs
// put on a request: the key each field is written under and whether it is
// written whatever the field holds, walked through the option structs a
// field nests and named the way GitLab's record declares a nested param.
//
// Two checks ask it. R-PATH's always-sent check
// (cmd/audit_1to1/internal/paths) asks which keys a request carries on every
// call, and R-PARAM (cmd/audit_1to1/internal/params) asks which params a
// route declares that no option struct of the requests reaching it can
// carry. Each would otherwise read the struct tags on its own, and two
// readers of one fact are how one of them ends up answering a narrower
// question than the other.
//
// What a field writes depends on the encoder, and the encoder on the
// constructor of the request: encoding/json reads the json tag for the body
// NewRequest builds for a PATCH, POST or PUT, and go-querystring reads the url
// tag for the query of every other verb and for every multipart field of an
// UploadRequest ([Encoders]). A field is walked into whenever it carries a
// struct client-go declares, not only an option struct, and a type defined
// on an option struct is read as that struct.
//
// It parses client-go's root package rather than type-checking it, as
// cmd/internal/sdkroutes does, whose reading of each service method it joins
// to the structs: which option structs a method takes and which routes it
// sends them to.
package sdkoptions
