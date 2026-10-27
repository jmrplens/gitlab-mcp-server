// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"strings"
	"testing"
)

// sdkCategories is the closed set an entry of the client-go tables may name:
// every category the tables over our own documents use, and the ones only a
// dependency's document can need.
var sdkCategories = map[string]bool{
	categoryNotThisResponse:        true,
	categoryLookup:                 true,
	categoryDeprecated:             true,
	categoryExperiment:             true,
	categoryNewerThanFloor:         true,
	categorySeparateAction:         true,
	categoryPublishedElsewhere:     true,
	categoryAffordance:             true,
	categoryUnusedIdentifier:       true,
	categoryRecursiveShape:         true,
	categoryWebRendering:           true,
	categoryViewer:                 true,
	categoryNeverSentHere:          true,
	categoryOutsideSurface:         true,
	categoryRestated:               true,
	categoryTierAboveDomain:        true,
	categoryRecordedUpstream:       true,
	categoryDecodedNeverRead:       true,
	categoryResolvedOncePerRequest: true,
	categoryBuiltPerCall:           true,
}

// methodName is how a declaration names the function holding a send, as
// [shortName] spells it: a client-go service type and a method of it.
var methodName = regexp.MustCompile(`^[A-Z][A-Za-z]*Service\.[A-Za-z]+$`)

// upstreamRow matches the register row a recorded-upstream answer names.
var upstreamRow = regexp.MustCompile(`[Uu]pstream-bugs row \d+`)

// TestDeclaredSDK_EveryEntryMeetsTheBarTheTablesSetThemselves verifies the
// committed client-go tables rather than a fixture's, which no run over a
// stand-in module can do: every entry names what it answers in the spelling
// the run reports, from the closed set of categories, with a reason a
// reviewer can judge; a recorded-upstream answer names the register row that
// records it, which is what a reader follows to see the state of the fix; and
// no two entries of a table share a key, since the second could only ever read
// stale.
func TestDeclaredSDK_EveryEntryMeetsTheBarTheTablesSetThemselves(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range declaredSDK.shapes {
		t.Run(declaration.key(), func(t *testing.T) {
			if !methodName.MatchString(declaration.Method) || !strings.HasPrefix(declaration.Path, "data.") {
				t.Errorf("the declaration names %q at %q, want a service method and a response path from data", declaration.Method, declaration.Path)
			}
			checkSDKDeclaration(t, seen, declaration.key(), declaration.Category, declaration.Reason)
		})
	}
	for _, declaration := range declaredSDK.sends {
		t.Run(declaration.key(), func(t *testing.T) {
			if !methodName.MatchString(declaration.Method) {
				t.Errorf("the declaration names %q, want a service method", declaration.Method)
			}
			checkSDKDeclaration(t, seen, declaration.key(), declaration.Category, declaration.Reason)
		})
	}
	for _, declaration := range declaredSDK.sent {
		t.Run(declaration.key(), func(t *testing.T) {
			if !strings.HasPrefix(declaration.Package, sdkModule+".") || !strings.HasSuffix(declaration.Package, "Service") {
				t.Errorf("package = %q, want a client-go service as sdkService spells it", declaration.Package)
			}
			if declaration.SchemaType == "" || declaration.Field == "" {
				t.Errorf("the declaration leaves part of its key empty: %+v", declaration)
			}
			checkSDKDeclaration(t, seen, declaration.key(), declaration.Category, declaration.Reason)
		})
	}
}

// checkSDKDeclaration holds one entry of the client-go tables to the bar every
// entry meets, whichever table it is in, and records its key in seen.
func checkSDKDeclaration(t *testing.T, seen map[string]bool, key, category, reason string) {
	t.Helper()
	if !sdkCategories[category] {
		t.Errorf("category = %q, want one of the %d the tables define", category, len(sdkCategories))
	}
	if len(reason) < 80 {
		t.Errorf("reason = %q, which is too short to judge whether it still holds", reason)
	}
	if category == categoryRecordedUpstream && !upstreamRow.MatchString(reason) {
		t.Errorf("reason = %q records the gap upstream and names no register row", reason)
	}
	if seen[key] {
		t.Errorf("a second declaration carries the key %s, so one of them can only ever read stale", key)
	}
	seen[key] = true
}

// TestDeclaredSDKSent_ANamedAnswerComesBeforeTheStarOverItsObject verifies the
// ordering the sent table relies on: classification takes the first entry that
// covers a finding, so a field answered by name and covered by a star over its
// object must be answered by its own entry, the one that goes stale when
// client-go starts selecting the field. The star coming first would make the
// named entry stale on every run.
func TestDeclaredSDKSent_ANamedAnswerComesBeforeTheStarOverItsObject(t *testing.T) {
	for _, finding := range []sentField{
		{Package: achievementsService, SchemaType: "Namespace", Field: "fullPath"},
		{Package: achievementsService, SchemaType: "UserAchievement", Field: "awardMessageHtml"},
	} {
		t.Run(finding.SchemaType+"."+finding.Field, func(t *testing.T) {
			classified, _ := classifySent(declaredSDK.sent, []sentField{finding})
			if classified[0].Category != categoryRecordedUpstream {
				t.Errorf("classifySent(%s.%s) = %q, want it answered by its own %s entry", finding.SchemaType, finding.Field, classified[0].Category, categoryRecordedUpstream)
			}
		})
	}
}

// TestClassifyAgainst_SortsBothSidesOfTheTable verifies the client-go
// tables' classification on its own: a finding a declaration answers carries
// its words, one none answers is handed back as it was, and a declaration that
// answered nothing is named, sorted, while a table with nothing stale reports
// nil.
func TestClassifyAgainst_SortsBothSidesOfTheTable(t *testing.T) {
	answered := sdkShapeFinding{Method: "ProjectsService.Get", Path: "data.project.color"}
	unanswered := sdkShapeFinding{Method: "ProjectsService.Get", Path: "data.project.size"}
	declarations := []sdkShapeDeclaration{
		{Method: "ProjectsService.Zeta", Path: "data.z", Category: categoryDecodedNeverRead, Reason: "z"},
		{Method: answered.Method, Path: answered.Path, Category: categoryDecodedNeverRead, Reason: "the color is never read"},
		{Method: "ProjectsService.Alpha", Path: "data.a", Category: categoryDecodedNeverRead, Reason: "a"},
		{Method: answered.Method, Path: "data.project.other", Category: categoryDecodedNeverRead, Reason: "same method, another path"},
	}

	classified, unused := classifyShapes(declarations, []sdkShapeFinding{answered, unanswered})

	t.Run("an answered finding carries the declaration's words", func(t *testing.T) {
		if classified[0].Reason != "the color is never read" || !classified[0].declared() {
			t.Errorf("classifyShapes() = %+v, want the first finding answered", classified[0])
		}
	})
	t.Run("an unanswered one is left as it was", func(t *testing.T) {
		if classified[1].declared() {
			t.Errorf("classifyShapes() = %+v, want the second finding unanswered", classified[1])
		}
	})
	t.Run("the stale ones are named in order", func(t *testing.T) {
		if strings.Join(unused, "|") != "ProjectsService.Alpha data.a|ProjectsService.Get data.project.other|ProjectsService.Zeta data.z" {
			t.Errorf("classifyShapes() stale = %v", unused)
		}
	})
	t.Run("nothing stale is nil", func(t *testing.T) {
		if _, none := classifySends([]sdkSendDeclaration{{Method: "A.B"}}, []sdkUnjudgedSend{{Method: "A.B"}}); none != nil {
			t.Errorf("classifySends() with nothing stale = %#v, want nil", none)
		}
	})
}
