// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkroutes

import (
	"reflect"
	"testing"
)

// TestSDK_NilGuards_NamesTheCheckARouteSitsBehind verifies which routes a method
// sends only under a nil check of its own parameter: each side of an if with
// the side it holds on, written either way round, in an else that holds another
// if, behind two checks at once, and the check two ways of sending a route
// share.
func TestSDK_NilGuards_NamesTheCheckARouteSitsBehind(t *testing.T) {
	sdk := fixture(t)
	item := Route{Method: "GET", Path: "/guarded/:/items/:"}
	items := Route{Method: "GET", Path: "/guarded/:/items"}
	lidSet := NilGuard{Param: "lid", Index: 1}
	lidNil := NilGuard{Param: "lid", Index: 1, Nil: true}
	pidSet := NilGuard{Param: "pid", Index: 0}
	otherSet := NilGuard{Param: "other", Index: 2}
	cases := map[string]map[Route][]NilGuard{
		"Guards.GuardedAppend":     {item: {lidSet}, items: {lidNil}},
		"Guards.GuardedEquals":     {items: {lidNil}},
		"Guards.GuardedEqualsElse": {item: {lidSet}, items: {lidNil}},
		"Guards.GuardedElseIf":     {item: {lidSet}, items: {lidNil, pidSet}},
		"Guards.GuardedNested":     {item: {pidSet, lidSet}},
		"Guards.GuardedTwoWays": {
			items: {otherSet},
		},
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			method, _ := sdk.Method(key)
			if !reflect.DeepEqual(method.NilGuards, want) {
				t.Errorf("%s NilGuards = %+v, want %+v", key, method.NilGuards, want)
			}
		})
	}
}

// TestSDK_NilGuards_ReadsNoGuardItCannotRelyOn verifies what guards nothing: a
// route also sent outside its check, whichever way is met first; a parameter
// the body assigns; a name that is no parameter, local or declared by the
// package; a selector; a condition of several terms; nil written first; a
// comparison with another name or a value or by another operator; a condition
// that is no comparison; a route of a helper the method delegates to, behind a
// check in the method or in the helper; and a request built the pre-option way.
func TestSDK_NilGuards_ReadsNoGuardItCannotRelyOn(t *testing.T) {
	sdk := fixture(t)
	for _, name := range []string{
		"GuardedAlsoOutside", "GuardedOutsideFirst", "GuardedAssigned", "GuardedLocal", "GuardedGlobal", "GuardedField", "GuardedTerms",
		"GuardedNilFirst", "GuardedOtherName", "GuardedOtherValue", "GuardedOtherOperator", "GuardedCondition",
		"GuardedDelegate", "GuardedLegacy",
	} {
		t.Run(name, func(t *testing.T) {
			method, known := sdk.Method("Guards." + name)
			if !known || len(method.Routes) == 0 {
				t.Fatalf("Guards.%s = %+v, %t; want a method with a route", name, method, known)
			}
			if method.NilGuards != nil {
				t.Errorf("Guards.%s NilGuards = %+v, want none", name, method.NilGuards)
			}
		})
	}
}

// TestSDK_NilGuards_LeavesEveryOtherMethodWithNone verifies a method with no nil
// check reads as none, whatever else it does.
func TestSDK_NilGuards_LeavesEveryOtherMethodWithNone(t *testing.T) {
	method, known := fixture(t).Method("Issues.GetIssue")
	if !known || method.NilGuards != nil {
		t.Errorf("Issues.GetIssue NilGuards = %+v, %t; want a known method with none", method.NilGuards, known)
	}
}
