package sdkroutes

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureDir is the stand-in for client-go's root package the reading is held
// to, one shape per method.
const fixtureDir = "testdata/sdk"

// brokenFile and brokenSource are the one file of the stand-in that does not
// parse, which the reading has to pass over. It is written beside a copy of
// [fixtureDir] when the fixture is read rather than committed into it: every
// walker of this repository's source parses the Go files it finds, testdata
// included, and a committed file that does not parse stops each of them.
const (
	brokenFile   = "broken.go"
	brokenSource = "package gitlab\n\n// A file that does not parse contributes nothing.\nfunc (s *IssuesService) Broken( {\n"
)

// fixtureSDK is the reading of the stand-in, parsed once: it is read-only for
// every test that asks of it.
var fixtureSDK *SDK

// fixture returns the reading of the stand-in package: [fixtureDir] copied
// whole, subdirectory and non-Go file included, with [brokenFile] added. The
// copy is read at once, so the temporary directory it sits in may go when the
// test that made it ends.
func fixture(t *testing.T) *SDK {
	t.Helper()
	if fixtureSDK == nil {
		dir := t.TempDir()
		if err := os.CopyFS(dir, os.DirFS(fixtureDir)); err != nil {
			t.Fatalf("copy %s: %v", fixtureDir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, brokenFile), []byte(brokenSource), 0o600); err != nil {
			t.Fatalf("write %s: %v", brokenFile, err)
		}
		fixtureSDK = Read(dir)
	}
	return fixtureSDK
}

// TestRead_FixturePackage_ResolvesEveryMethodShape holds each method of the
// stand-in to the requests it sends: the option form, delegation through the
// receiver, through a field and through a generic package function with the
// constant each caller hands over, the legacy form folded through Sprintf, a
// helper and a reassigned verb, a collection picked through a branched local
// (in a template, handed to a helper, formatted and returned by a helper), and
// the GraphQL transport.
func TestRead_FixturePackage_ResolvesEveryMethodShape(t *testing.T) {
	sdk := fixture(t)
	get := func(method, path string) Route { return Route{Method: method, Path: path} }
	cases := map[string]Method{
		"Issues.GetIssue": {
			Service: "Issues", Name: "GetIssue", Answers: "Issue",
			Routes: []Route{get("GET", "/projects/:/issues/:")},
		},
		"Issues.ListIssues": {
			Service: "Issues", Name: "ListIssues", Answers: "Issue", Many: true,
			Options: []string{"ListIssuesOptions"},
			Routes:  []Route{get("GET", "/projects/:/issues")},
		},
		"Issues.DeleteIssue": {
			Service: "Issues", Name: "DeleteIssue",
			Routes: []Route{get("DELETE", "/projects/:/issues/:")},
		},
		"Issues.AddSpentTime": {
			Service: "Issues", Name: "AddSpentTime", Answers: "TimeStats",
			Options: []string{"AddSpentTimeOptions"},
			Routes:  []Route{get("POST", "/projects/:/issues/:/add_spent_time")},
		},
		"Issues.ListNotes": {
			Service: "Issues", Name: "ListNotes", Answers: "Issue", Many: true,
			Routes: []Route{get("GET", "/projects/:/issues/notes"), get("GET", "/projects/:/merge_requests/notes")},
		},
		"Issues.ListDiscussions": {
			Service: "Issues", Name: "ListDiscussions", Answers: "Issue", Many: true,
			Routes: []Route{get("GET", "/projects/:/issues/discussions"), get("GET", "/projects/:/merge_requests/discussions")},
		},
		"Repositories.Stats": {
			Service: "Repositories", Name: "Stats", Answers: "Upload",
			Routes: []Route{
				get("GET", "/projects/:/issues/time_stats"), get("GET", "/projects/:/merge_requests/time_stats"),
				get("POST", "/stats/issues"), get("POST", "/stats/merge_requests"),
			},
		},
		"AwardEmoji.GetIssueAwardEmoji": {
			Service: "AwardEmoji", Name: "GetIssueAwardEmoji", Answers: "AwardEmoji",
			Routes: []Route{get("GET", "/projects/:/issues/:/award_emoji/:")},
		},
		"AwardEmoji.GetSnippetAwardEmoji": {
			Service: "AwardEmoji", Name: "GetSnippetAwardEmoji", Answers: "AwardEmoji",
			Routes: []Route{get("GET", "/projects/:/snippets/:/award_emoji/:")},
		},
		"ProjectUploads.ListUploads": {
			Service: "ProjectUploads", Name: "ListUploads", Answers: "Upload", Many: true,
			Routes: []Route{get("GET", "/projects/:/uploads")},
		},
		"Issues.Search": {
			Service: "Issues", Name: "Search", Answers: "Issue",
			Routes: []Route{get("GET", "/literal/:/path"), get("GET", "/search")},
		},
		"Issues.Package": {
			Service: "Issues", Name: "Package", Answers: "Issue",
			Routes: []Route{get("GET", "/projects/:/packages/file")},
		},
		"Issues.Opaque":       {Service: "Issues", Name: "Opaque", Answers: "Issue"},
		"Issues.EmptyResults": {Service: "Issues", Name: "EmptyResults"},
		"Issues.Loop":         {Service: "Issues", Name: "Loop", Answers: "Issue"},
		"Issues.Unnamed":      {Service: "Issues", Name: "Unnamed", Answers: "Issue", Routes: []Route{get("GET", "/projects/:/issues")}},
		"Issues.NoResults":    {Service: "Issues", Name: "NoResults"},
		"Issues.Scalar":       {Service: "Issues", Name: "Scalar", Answers: "int"},
		"Issues.Qualified":    {Service: "Issues", Name: "Qualified"},
		"Issues.Bytes":        {Service: "Issues", Name: "Bytes", Answers: "byte", Many: true},
		"Issues.Instantiated": {Service: "Issues", Name: "Instantiated", Answers: "Generic"},
		"Issues.Paired":       {Service: "Issues", Name: "Paired", Answers: "Pair"},
		"Repositories.StreamArchive": {
			Service: "Repositories", Name: "StreamArchive",
			Options: []string{"ArchiveOptions"},
			Routes:  []Route{get("GET", "/projects/:/repository/archive")},
		},
		"GenericPackages.FormatPackageURL": {Service: "GenericPackages", Name: "FormatPackageURL", Answers: "string"},
		"GenericPackages.PublishPackageFile": {
			Service: "GenericPackages", Name: "PublishPackageFile", Answers: "Upload",
			Routes: []Route{get("PUT", "/projects/:/packages/generic/:/:/:")},
		},
		"Repositories.Fetch": {
			Service: "Repositories", Name: "Fetch", Answers: "Upload",
			Routes: []Route{get("DELETE", "/two/args"), get("POST", "/projects/uploads")},
			Unresolved: []string{
				"RepositoriesService.Fetch: the path of a request it builds folds to no static segment",
				"RepositoriesService.Fetch: the verb of a request it builds is not a net/http constant",
			},
		},
		"Repositories.Verbs": {
			Service: "Repositories", Name: "Verbs", Answers: "Upload",
			Routes: []Route{get("GET", "/verbs")},
		},
		"Achievements.DescribeAchievements": {
			Service: "Achievements", Name: "DescribeAchievements", Answers: "Upload",
			Routes: []Route{get("GET", "/achievements")},
		},
		"GraphQL.Do":                     {Service: "GraphQL", Name: "Do", Answers: ""},
		"Achievements.ListAchievements":  {Service: "Achievements", Name: "ListAchievements", Answers: "Upload", Many: true, GraphQL: true},
		"Achievements.InlineAchievement": {Service: "Achievements", Name: "InlineAchievement", Answers: "Upload", GraphQL: true},
		"WorkItems.GetWorkItem":          {Service: "WorkItems", Name: "GetWorkItem", Answers: "Upload", GraphQL: true},
		"WorkItems.NotGraphQL":           {Service: "WorkItems", Name: "NotGraphQL", Answers: "Upload"},
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			got, ok := sdk.Method(key)
			if !ok {
				t.Fatalf("Method(%q) not found", key)
			}
			if got.Key() != key {
				t.Errorf("Key() = %q, want %q", got.Key(), key)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Method(%q) =\n%+v\nwant\n%+v", key, got, want)
			}
		})
	}
}

// TestRead_FoldsMethod_ReadsEveryExpressionShape holds the legacy path folding
// to every expression it reads: concatenation, Sprintf with an escaped percent,
// flags, a non-string argument and arguments that run out, a self-calling
// helper, one with no return, one whose only return is bare, a function
// literal called in place, a local assigned a literal, the second result of a
// call that returns two, a method of the receiver, a constant, and a choice
// that would multiply past the bound.
func TestRead_FoldsMethod_ReadsEveryExpressionShape(t *testing.T) {
	method, ok := fixture(t).Method("Repositories.Folds")
	if !ok {
		t.Fatal(`Method("Repositories.Folds") not found`)
	}
	var plain, many []string
	for _, route := range method.Routes {
		if route.Method != http.MethodGet {
			t.Errorf("route %v carries a verb other than GET", route)
		}
		if strings.HasPrefix(route.Path, "/many/") {
			many = append(many, route.Path)
			continue
		}
		plain = append(plain, route.Path)
	}
	wantPlain := []string{
		"/%/groups/:/:", "/answers", "/ends/:", "/pairs/two/:", "/prefixed/:", "/products/:",
		"/projects/:", "/recursive/:/:/:", "/relative/:",
	}
	if !reflect.DeepEqual(plain, wantPlain) {
		t.Errorf("routes = %v, want %v", plain, wantPlain)
	}
	if len(many) != maxFolds {
		t.Errorf("a path with five four-way choices folded to %d spellings, want the bound %d", len(many), maxFolds)
	}
	wantUnresolved := []string{
		"RepositoriesService.Folds: the path of a request it builds folds to more spellings than the reading keeps",
		"RepositoriesService.Folds: the path of a request it builds folds to no static segment",
	}
	if !reflect.DeepEqual(method.Unresolved, wantUnresolved) {
		t.Errorf("Unresolved = %v, want %v", method.Unresolved, wantUnresolved)
	}
}

// TestRead_ManyChoicesMethod_ReportsTheTemplatePastTheBound verifies a
// template whose arguments multiply past the bound keeps the routes the bound
// allows and reports the rest as unresolved, rather than reading the routes
// kept as all the method sends.
func TestRead_ManyChoicesMethod_ReportsTheTemplatePastTheBound(t *testing.T) {
	method, ok := fixture(t).Method("Issues.ManyChoices")
	if !ok {
		t.Fatal(`Method("Issues.ManyChoices") not found`)
	}
	if len(method.Routes) != maxFolds {
		t.Errorf("a template with five four-way arguments read %d routes, want the bound %d", len(method.Routes), maxFolds)
	}
	want := []string{"IssuesService.ManyChoices: a path it formats folds to more spellings than the reading keeps"}
	if !reflect.DeepEqual(method.Unresolved, want) {
		t.Errorf("Unresolved = %v, want %v", method.Unresolved, want)
	}
}

// TestRead_FixturePackage_ReadsOnlyExportedServiceMethods verifies that a
// method a handler cannot call is no entry: an unexported one, one of an
// unexported type, of the bare Service type or of a type that is no service,
// one declared without a body, and one in a test file, a subdirectory, a file
// that does not parse or a file that parses but is not named as Go source.
func TestRead_FixturePackage_ReadsOnlyExportedServiceMethods(t *testing.T) {
	sdk := fixture(t)
	for _, key := range []string{
		"Issues.unexported", "internal.Exported", ".Exported", "Helper.Exported",
		"Issues.Declared", "Issues.FromATest", "Issues.Nested", "Issues.Broken",
		"Issues.FromText",
	} {
		t.Run(key, func(t *testing.T) {
			if method, ok := sdk.Method(key); ok {
				t.Errorf("Method(%q) = %+v, want no such method", key, method)
			}
		})
	}
}

// TestSDK_Methods_ListsEveryEntryInKeyOrder verifies the listing is complete
// and sorted.
func TestSDK_Methods_ListsEveryEntryInKeyOrder(t *testing.T) {
	methods := fixture(t).Methods()
	if len(methods) != 35 {
		t.Errorf("Methods() listed %d, want 35", len(methods))
	}
	for i := 1; i < len(methods); i++ {
		if methods[i-1].Key() >= methods[i].Key() {
			t.Errorf("Methods() not in key order at %q, %q", methods[i-1].Key(), methods[i].Key())
		}
	}
}

// TestRead_DirectoryItCannotRead_YieldsNoMethods verifies that a directory that
// is not there, and no directory at all, yield an empty reading rather than a
// failure.
func TestRead_DirectoryItCannotRead_YieldsNoMethods(t *testing.T) {
	for _, dir := range []string{"", "testdata/absent"} {
		t.Run(dir, func(t *testing.T) {
			if methods := Read(dir).Methods(); len(methods) != 0 {
				t.Errorf("Read(%q).Methods() = %v, want none", dir, methods)
			}
		})
	}
}

// TestRoute_String_SpellsVerbAndPath verifies the spelling a report uses.
func TestRoute_String_SpellsVerbAndPath(t *testing.T) {
	if got := (Route{Method: "GET", Path: "/projects"}).String(); got != "GET /projects" {
		t.Errorf("String() = %q, want %q", got, "GET /projects")
	}
}

// TestSortedUnique_DropsRepeatsAndEmpties verifies the list helper's two ends.
func TestSortedUnique_DropsRepeatsAndEmpties(t *testing.T) {
	if got := sortedUnique(nil); got != nil {
		t.Errorf("sortedUnique(nil) = %v, want nil", got)
	}
	if got := sortedUnique([]string{"b", "a", "b", "a"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("sortedUnique() = %v, want [a b]", got)
	}
}

// TestSortedRoutes_DropsRepeatsAndEmpties verifies the route list helper's two
// ends.
func TestSortedRoutes_DropsRepeatsAndEmpties(t *testing.T) {
	if got := sortedRoutes(nil); got != nil {
		t.Errorf("sortedRoutes(nil) = %v, want nil", got)
	}
	a := Route{Method: "GET", Path: "/a"}
	b := Route{Method: "GET", Path: "/b"}
	if got := sortedRoutes([]Route{b, a, b}); !reflect.DeepEqual(got, []Route{a, b}) {
		t.Errorf("sortedRoutes() = %v, want [%v %v]", got, a, b)
	}
}
