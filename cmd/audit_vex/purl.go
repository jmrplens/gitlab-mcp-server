// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// purl is a package URL (https://github.com/package-url/purl-spec) read into
// the parts this command compares, each percent-decoded.
//
// It is a reader of the subset this repository writes, not a validator of
// the whole specification: the type is lowercased and otherwise taken as it
// is, and the namespace and name are kept together as path, since for a Go
// module the two are one module path.
type purl struct {
	kind       string
	path       string
	version    string
	qualifiers map[string]string
}

// parsePURL reads one package URL.
//
// A subpath is refused rather than dropped: no product of this repository is
// a part of a package, and one that claimed to be would be compared as the
// whole of it.
func parsePURL(s string) (purl, error) {
	rest, ok := strings.CutPrefix(s, "pkg:")
	if !ok {
		return purl{}, fmt.Errorf("%q is not a package URL, which starts with pkg", s)
	}
	if strings.Contains(rest, "#") {
		return purl{}, fmt.Errorf("%q carries a subpath, which no product of this repository has", s)
	}
	rest, query, _ := strings.Cut(rest, "?")
	kind, name, _ := strings.Cut(rest, "/")
	if kind == "" || strings.Trim(name, "/") == "" {
		return purl{}, fmt.Errorf("%q names no type and name", s)
	}
	p := purl{kind: strings.ToLower(kind), qualifiers: map[string]string{}}
	if at := strings.LastIndex(name, "@"); at >= 0 {
		version, err := url.PathUnescape(name[at+1:])
		if err != nil || version == "" {
			return purl{}, errors.Join(fmt.Errorf("%q names no version after its @", s), err)
		}
		name, p.version = name[:at], version
	}
	segments := strings.Split(strings.Trim(name, "/"), "/")
	for i, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "" {
			return purl{}, errors.Join(fmt.Errorf("%q has a namespace or name segment that does not decode", s), err)
		}
		segments[i] = decoded
	}
	p.path = strings.Join(segments, "/")
	if query == "" {
		return p, nil
	}
	for pair := range strings.SplitSeq(query, "&") {
		key, value, _ := strings.Cut(pair, "=")
		decoded, err := url.PathUnescape(value)
		if err != nil || key == "" || decoded == "" {
			return purl{}, errors.Join(fmt.Errorf("%q has a qualifier that is not a key and a value", s), err)
		}
		p.qualifiers[strings.ToLower(key)] = decoded
	}
	return p, nil
}
