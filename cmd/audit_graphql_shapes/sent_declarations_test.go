package main

import (
	"strings"
	"testing"
)

// TestSentDeclaration_Covers_AnswersOnlyItsOwnPackageTypeAndField verifies the
// three-way key the adjudication table is read by, one leg at a time.
//
// Each leg is what stops one declaration answering a question nobody asked of
// it: the same schema type is reached from several packages and each is a
// separate decision, the same package decodes several objects, and a
// declaration naming one field must not silence the rest of the object. Only
// the star does that, and it says so.
func TestSentDeclaration_Covers_AnswersOnlyItsOwnPackageTypeAndField(t *testing.T) {
	finding := sentField{Package: "fixture/sent", SchemaType: "Pipeline", Field: "startedAt"}
	segment := sentDeclaration{Package: finding.Package, SchemaType: finding.SchemaType, Field: declaredSegment}

	for _, testCase := range []struct {
		name        string
		declaration sentDeclaration
		want        bool
	}{
		{name: "the star covers every field of that object in that package", declaration: segment, want: true},
		{
			name:        "a declaration naming the field covers it",
			declaration: sentDeclaration{Package: finding.Package, SchemaType: finding.SchemaType, Field: finding.Field},
			want:        true,
		},
		{
			name:        "a declaration naming another field of the same object does not",
			declaration: sentDeclaration{Package: finding.Package, SchemaType: finding.SchemaType, Field: "status"},
			want:        false,
		},
		{
			name:        "a declaration about another object of the same package does not",
			declaration: sentDeclaration{Package: finding.Package, SchemaType: "Label", Field: declaredSegment},
			want:        false,
		},
		{
			name:        "a declaration about the same object in another package does not",
			declaration: sentDeclaration{Package: "fixture/other", SchemaType: finding.SchemaType, Field: declaredSegment},
			want:        false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.declaration.covers(finding); got != testCase.want {
				t.Errorf("%s.covers(%s.%s.%s) = %t, want %t",
					testCase.declaration.key(), finding.Package, finding.SchemaType, finding.Field, got, testCase.want)
			}
		})
	}
}

// TestClassifySent_SortsBothSidesOfTheTable verifies the two claims a
// declaration table makes at once, over one call: a finding a declaration
// covers carries that declaration's category and reason and counts as
// answered, a finding none covers is handed back untouched, and a declaration
// that answered nothing is named so an excuse cannot outlive what it excused.
func TestClassifySent_SortsBothSidesOfTheTable(t *testing.T) {
	answered := sentField{Package: "fixture/sent", SchemaType: "Pipeline", Field: "startedAt"}
	unanswered := sentField{Package: "fixture/sent", SchemaType: "Label", Field: "color"}
	answers := sentDeclaration{
		Package:    answered.Package,
		SchemaType: answered.SchemaType,
		Field:      answered.Field,
		Category:   categoryNotThisResponse,
		Reason:     "the pipelines domain publishes a pipeline, and this document names one to say which",
	}
	stale := sentDeclaration{
		Package:    answered.Package,
		SchemaType: "Widget",
		Field:      declaredSegment,
		Category:   categoryDeprecated,
		Reason:     "no document of this fixture reaches a widget",
	}

	classified, unused := classifySent([]sentDeclaration{answers, stale}, []sentField{answered, unanswered})

	if len(classified) != 2 {
		t.Fatalf("classifySent() returned %d finding(s), want the 2 it was given", len(classified))
	}
	t.Run("a covered finding carries the declaration's words", func(t *testing.T) {
		if classified[0].Category != answers.Category || classified[0].Reason != answers.Reason {
			t.Errorf("the covered finding reads category %q, reason %q; want the declaration's",
				classified[0].Category, classified[0].Reason)
		}
		if !classified[0].declared() {
			t.Errorf("the covered finding does not count as declared")
		}
	})
	t.Run("a finding no declaration covers is left as it was", func(t *testing.T) {
		if classified[1].Category != "" || classified[1].Reason != "" {
			t.Errorf("the uncovered finding was annotated with category %q, reason %q",
				classified[1].Category, classified[1].Reason)
		}
		if classified[1].declared() {
			t.Errorf("a finding no declaration answers counts as declared")
		}
	})
	t.Run("a declaration that answered nothing is named and one that answered is not", func(t *testing.T) {
		if len(unused) != 1 || unused[0] != stale.key() {
			t.Errorf("classifySent() reports %v stale, want only %s", unused, stale.key())
		}
	})
}

// TestClassifySent_NothingStale_ReportsNilRatherThanAnEmptyList verifies the
// distinction the report rests on: a check with no stale declaration and a
// check with no declarations at all must read the same in JSON, so the list is
// left nil rather than allocated empty.
func TestClassifySent_NothingStale_ReportsNilRatherThanAnEmptyList(t *testing.T) {
	finding := sentField{Package: "fixture/sent", SchemaType: "Pipeline", Field: "startedAt"}
	covering := sentDeclaration{
		Package:    finding.Package,
		SchemaType: finding.SchemaType,
		Field:      declaredSegment,
		Category:   categoryLookup,
		Reason:     "the document resolves an identifier and answers nobody",
	}

	if _, unused := classifySent([]sentDeclaration{covering}, []sentField{finding}); unused != nil {
		t.Errorf("classifySent() with nothing stale returns %#v, want nil", unused)
	}
	if _, unused := classifySent(nil, []sentField{finding}); unused != nil {
		t.Errorf("classifySent() with no declarations returns %#v, want nil", unused)
	}
}

// TestDeclaredSent_EveryEntryMeetsTheBarTheTableSetsItself verifies the real
// adjudication table rather than a fixture's, which is the one thing a run
// over a fixture module can never do: the fixtures carry their own
// declarations so that a synthetic tree is not asked to explain this tree's
// decisions, and the committed table would otherwise be read by nothing but
// main.
//
// The bar is the one the type's own comment sets. Every entry names a package,
// an object and a field or the star; the category comes from the closed set,
// since a category invented at the call site is a vocabulary rather than a
// decision; the reason is prose a reviewer can judge rather than a word; and
// no two entries share a key, because the second would be reported stale
// forever while answering exactly what the first answers.
func TestDeclaredSent_EveryEntryMeetsTheBarTheTableSetsItself(t *testing.T) {
	categories := map[string]bool{
		categoryNotThisResponse: true,
		categoryLookup:          true,
		categoryDeprecated:      true,
	}
	seen := map[string]bool{}

	for _, declaration := range declaredSent {
		t.Run(declaration.key(), func(t *testing.T) {
			if declaration.Package == "" || declaration.SchemaType == "" || declaration.Field == "" {
				t.Errorf("the declaration leaves part of its key empty: %+v", declaration)
			}
			if !strings.HasPrefix(declaration.Package, toolsDir+"/") {
				t.Errorf("package = %q, want a package under %s", declaration.Package, toolsDir)
			}
			if !categories[declaration.Category] {
				t.Errorf("category = %q, want one of the three this table defines", declaration.Category)
			}
			if len(declaration.Reason) < 80 {
				t.Errorf("reason = %q, which is too short to judge whether it still holds", declaration.Reason)
			}
			if seen[declaration.key()] {
				t.Errorf("a second declaration carries the key %s, so one of them can only ever read stale", declaration.key())
			}
			seen[declaration.key()] = true
		})
	}
	if len(declaredSent) == 0 {
		t.Error("the adjudication table is empty, so nothing holds it to this bar")
	}
}
