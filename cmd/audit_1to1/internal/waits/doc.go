// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package waits holds every workaround this repository carries because
// client-go does something wrong or does not do it at all, and the predicate
// on the pinned client-go that says the workaround is still needed.
//
// A workaround outlives its reason when nothing asks for it. Row 6 of
// docs/development/upstream-bugs.md is the case that made this package:
// client-go's SetFeatureFlagOptions gained omitempty in v3.34.0, and
// internal/tools/features.Set and the e2e fixture's setFeature went on
// building the request body by hand with nothing to say so. Each entry here
// names the register row it waits on, the functions that carry it, the
// client-go methods it stands in for and what is true of the SDK while it is
// needed (a field written on every call, a key no struct models, a route no
// method sends), so a pin that makes a predicate false is a finding the day
// it lands.
//
// A raw request that waits on nothing upstream (a slug dispatched from the
// input, a streamed download, a URL built for the caller) carries a category
// in place of a row and a predicate.
//
// The package also reads a [Superset], a struct of a tool package's own that
// a raw request carries in place of client-go's, for the predicate each key
// it carries beside the client-go struct it embeds needs, and the register's
// rows with the text that describes each, which an entry's names are held
// to.
//
// The package is a leaf so that every reader can import it: R-ACTION derives
// its COVERED_RAW methods from the entries rather than declaring them a
// second time, and the sdk scope holds the entries to the tree and to the
// SDK. If the table lived in the sdk scope, the action scope reading it and
// the sdk scope judging the action scope's table would import each other.
package waits
