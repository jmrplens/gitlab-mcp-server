package gitlab

import (
	"fmt"
	"net/http"
)

// GetIssue is the ordinary option form: a route template and nothing else.
func (s *IssuesService) GetIssue(pid any, issue int64, options ...RequestOptionFunc) (*Issue, *Response, error) {
	return do[*Issue](s.client,
		withPath(routeProjectsIDIssuesID, ProjectID{pid}, issue),
		withRequestOpts(options...),
	)
}

// ListIssues answers with many, takes one option struct twice and a variadic
// option tail the reading passes over.
func (s *IssuesService) ListIssues(pid any, opt *ListIssuesOptions, again *ListIssuesOptions, extra ...ExtraOptions) ([]*Issue, *Response, error) {
	return do[[]*Issue](s.client,
		withMethod(http.MethodGet),
		withPath(routeProjectsIDIssues, ProjectID{pid}),
		withAPIOpts(opt),
	)
}

// Opaque formats a template with no literal segment, which names no endpoint
// anybody could tell from another, and so contributes no route.
func (s *IssuesService) Opaque(pid any, kind string) (*Issue, *Response, error) {
	return do[*Issue](s.client,
		withPath(routeOnlyPlaceholders, ProjectID{pid}, kind),
	)
}

// EmptyResults writes its result list as an empty pair of parentheses, which
// the parser keeps as a list holding nothing.
func (s *IssuesService) EmptyResults() () {}

// DeleteIssue answers with the pagination wrapper alone, and is kept.
func (s *IssuesService) DeleteIssue(pid any, issue int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath(routeProjectsIDIssuesID, ProjectID{pid}, issue),
	)
	return resp, err
}

// AddSpentTime delegates to a helper held in a field, handing it a literal.
func (s *IssuesService) AddSpentTime(pid any, issue int64, opt *AddSpentTimeOptions, options ...RequestOptionFunc) (*TimeStats, *Response, error) {
	return s.timeStats.addSpentTime(pid, "issues", issue, opt, options...)
}

func (s *timeStatsService) addSpentTime(pid any, entity string, issue int64, opt *AddSpentTimeOptions, options ...RequestOptionFunc) (*TimeStats, *Response, error) {
	return do[*TimeStats](s.client,
		withMethod(http.MethodPost),
		withPath(routeProjectsIDIDIDAddSpentTime, ProjectID{pid}, entity, issue),
	)
}

// GetIssueAwardEmoji and GetSnippetAwardEmoji delegate to one helper of their
// own service with a constant each, which is what keeps their routes apart.
func (s *AwardEmojiService) GetIssueAwardEmoji(pid any, issue, award int64, options ...RequestOptionFunc) (*AwardEmoji, *Response, error) {
	return s.getAwardEmoji(pid, awardIssue, issue, award, options...)
}

func (s *AwardEmojiService) GetSnippetAwardEmoji(pid any, snippet, award int64, options ...RequestOptionFunc) (*AwardEmoji, *Response, error) {
	return s.getAwardEmoji(pid, (awardSnippets), snippet, award)
}

func (s *AwardEmojiService) getAwardEmoji(pid any, resource string, id, award int64, options ...RequestOptionFunc) (*AwardEmoji, *Response, error) {
	return do[*AwardEmoji](s.client,
		withPath(routeProjectsIDIDIDAwardEmojiID, ProjectID{pid}, resource, id, award),
	)
}

// ListNotes picks the collection its template names through a branched local,
// so it sends to one route per collection.
func (s *IssuesService) ListNotes(pid any, mergeRequest bool) ([]*Issue, *Response, error) {
	collection := "issues"
	if mergeRequest {
		collection = "merge_requests"
	}
	return do[[]*Issue](s.client, withPath(routeProjectsIDIDNotes, ProjectID{pid}, collection))
}

// ManyChoices formats a template whose five arguments each fold four ways, so
// its combinations multiply past the bound.
func (s *IssuesService) ManyChoices() ([]*Issue, *Response, error) {
	return do[[]*Issue](s.client, withPath(routeFiveChoices, choice(), choice(), choice(), choice(), choice()))
}

// ListDiscussions hands a collection picked the same way to a helper, which
// is entered once per collection.
func (s *IssuesService) ListDiscussions(pid any, mergeRequest bool) ([]*Issue, *Response, error) {
	collection := awardIssue
	if mergeRequest {
		collection = "merge_requests"
	}
	return s.listDiscussions(pid, collection)
}

func (s *IssuesService) listDiscussions(pid any, collection string) ([]*Issue, *Response, error) {
	return do[[]*Issue](s.client, withPath(routeProjectsIDIDDiscussions, ProjectID{pid}, collection))
}

// ListUploads delegates to a generic package function with a typed constant.
func (s *ProjectUploadsService) ListUploads(pid any, options ...RequestOptionFunc) ([]*Upload, *Response, error) {
	return listUploads[Upload](s.client, ProjectResource, ProjectID{pid})
}

func listUploads[T any](client *Client, resource ResourceType, id Pather) ([]*T, *Response, error) {
	return do[[]*T](client, withPath(routeIDIDUploads, resource, id))
}

// Search names its template inline, carries a query string in it and names a
// verb through something that is not a net/http constant.
func (s *IssuesService) Search(scope string, verb string) (*Issue, *Response, error) {
	return do[*Issue](s.client,
		withMethod(verb),
		withMethod(),
		withMethod(s.verb),
		withMethod(a.b.MethodGet),
		withMethod(http.StatusOK),
		withPath(routeSearch, scope),
		withPath("literal/%s/path", scope),
		withPath(),
		withPath(unknownRoute),
		withPath(42),
	)
}

// Package formats a template whose verbs are joined by a separator.
func (s *IssuesService) Package(pid any) (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeProjectsIDPackagesID, ProjectID{pid}, "file", 1))
}

// Loop and loopBack call each other, which the walk must not follow forever.
func (s *IssuesService) Loop() (*Issue, *Response, error) {
	s.missing()
	s.other.Do()
	s.client.Do(nil, nil)
	newThing().Run()
	other.timeStats.addSpentTime(nil, "issues", 1, nil)
	s.callResult().timeStats.addSpentTime(nil, "issues", 1, nil)
	func() {}()
	helper.Run()
	helper.Do()
	pair[int, string]()
	return s.loopBack()
}

func (s *IssuesService) loopBack() (*Issue, *Response, error) {
	return s.Loop()
}

// Unnamed has no receiver name, so nothing in it can delegate through one.
func (*IssuesService) Unnamed() (*Issue, *Response, error) {
	helper.Run()
	return do[*Issue](nil, withPath(routeProjectsIDIssues, nil))
}

// Answers spell the results a method answers with.
func (s *IssuesService) NoResults()                                     {}
func (s *IssuesService) Scalar() (int, error)                           { return 0, nil }
func (s *IssuesService) Qualified() (bytes.Buffer, error)               { return bytes.Buffer{}, nil }
func (s *IssuesService) Bytes() ([]byte, *Response, error)              { return nil, nil, nil }
func (s *IssuesService) Instantiated() (*Generic[int], *Response, error) { return nil, nil, nil }
func (s *IssuesService) Paired() (*Pair[string, int], *Response, error) { return nil, nil, nil }
func (s *IssuesService) unexported() (*Issue, *Response, error)         { return nil, nil, nil }
func (s *internalService) Exported() (*Issue, *Response, error)         { return nil, nil, nil }
func (s *Service) Exported() (*Issue, *Response, error)                 { return nil, nil, nil }
func (h Helper) Exported() (*Issue, *Response, error)                   { return nil, nil, nil }
func (s *IssuesService) Declared() (*Issue, *Response, error)

func blank(int, string) {}
