// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package completions

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// searchPerPage is intentionally larger than maxCompletionResults (10) so
// that toResultWithTotal can compute an accurate HasMore flag after truncation.
const searchPerPage = 20

// totalFromResponse extracts the total match count from a GitLab REST
// pagination response. GitLab populates [gitlab.Response.TotalItems] from
// the X-Total header for offset-paginated endpoints; it is 0 for keyset
// pagination or when the server omits the header (e.g. on collections
// large enough to skip counting).
func totalFromResponse(resp *gl.Response) int {
	if resp == nil {
		return 0
	}
	return int(resp.TotalItems)
}

// partialMatchLength is the fewest characters a search term needs for GitLab
// to match it in part (MIN_CHARS_FOR_PARTIAL_MATCHING in
// Gitlab::SQL::Pattern). A query with no term this long is compared with each
// column whole.
const partialMatchLength = 3

// searchProjects returns project entries matching the query plus the total
// match count from the GitLab pagination header (X-Total) when available.
//
// What is typed is matched with search_namespaces set whenever GitLab matches
// some term of it in part. Without the flag GitLab's Project.search matches a
// project's path, name and description, and the value this completion offers
// is the full path, so typing it back, or any part reaching into the
// namespace, matched nothing. With it GitLab matches the route's full path and
// full name instead, which hold the project's own path and name, so a term
// matched in part still finds a bare project name. A query with no term of
// partialMatchLength characters is compared whole, and no full path or full
// name equals a bare name, so that query goes without the flag and a project
// named p1 is still found by typing p1.
func searchProjects(ctx context.Context, client *gitlabclient.Client, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListProjectsOptions{
		Membership: new(true),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
		if matchesInPart(query) {
			opts.SearchNamespaces = new(true)
		}
	}
	projects, resp, err := client.GL().Projects.ListProjects(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search projects: %w", err)
	}
	values = make([]string, 0, len(projects))
	for _, p := range projects {
		values = append(values, formatProjectEntry(p.ID, p.PathWithNamespace))
	}
	return values, totalFromResponse(resp), nil
}

// matchesInPart reports whether GitLab matches some term of query in part,
// which is the only case in which search_namespaces widens what it finds.
func matchesInPart(query string) bool {
	for _, term := range searchTerms(query) {
		if len([]rune(term)) >= partialMatchLength {
			return true
		}
	}
	return false
}

// searchTerms splits query into the terms GitLab's fuzzy search matches,
// after collapsing its whitespace as String#squish does
// (Gitlab::SQL::Pattern.split_query_to_search_terms): a double-quoted phrase
// standing on its own is one term without its quotes, and every other word is
// a term.
func searchTerms(query string) []string {
	query = strings.Join(strings.Fields(query), " ")
	var phrases []string
	var rest strings.Builder
	for i := 0; i < len(query); i++ {
		if phrase, end, ok := quotedPhrase(query, i); ok {
			phrases = append(phrases, phrase)
			i = end
			continue
		}
		rest.WriteByte(query[i])
	}
	return append(strings.Fields(rest.String()), phrases...)
}

// quotedPhrase reports the phrase a double quote at i opens, as GitLab reads
// one: the quote starts the query or follows a space, at least one character
// stands before the next quote, and that closing quote ends the query or
// precedes a space. end is the index of the closing quote.
func quotedPhrase(query string, i int) (phrase string, end int, ok bool) {
	if query[i] != '"' || (i > 0 && query[i-1] != ' ') {
		return "", 0, false
	}
	length := strings.IndexByte(query[i+1:], '"')
	end = i + 1 + length
	if length < 1 || (end+1 < len(query) && query[end+1] != ' ') {
		return "", 0, false
	}
	return query[i+1 : end], end, true
}

// searchGroups returns group entries matching the query plus the total
// match count from the GitLab pagination header.
func searchGroups(ctx context.Context, client *gitlabclient.Client, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListGroupsOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	groups, resp, err := client.GL().Groups.ListGroups(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search groups: %w", err)
	}
	values = make([]string, 0, len(groups))
	for _, g := range groups {
		values = append(values, formatGroupEntry(g.ID, g.FullPath))
	}
	return values, totalFromResponse(resp), nil
}

// searchUsers returns usernames matching the query plus the total match
// count from the GitLab pagination header.
func searchUsers(ctx context.Context, client *gitlabclient.Client, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListUsersOptions{
		Active: new(true),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	users, resp, err := client.GL().Users.ListUsers(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search users: %w", err)
	}
	values = make([]string, 0, len(users))
	for _, u := range users {
		values = append(values, u.Username)
	}
	return values, totalFromResponse(resp), nil
}

// iidExtraDigits is how far past what was typed an IID completion looks: what
// was typed, then each IID one digit longer.
//
// It stops at one digit because GitLab writes the request's query string into
// every link of the Link header it answers a listing with, and the nginx a
// GitLab package bundles refuses response headers over its 4 KiB buffer with a
// 502. Looking two digits past a typed 1 named 111 IIDs, and CE 19.4.2 refused
// that request so, which left the completion empty; the 11 IIDs of one digit
// stay under 400 bytes whatever was typed. Completion runs again on every
// keystroke, so the next digit typed asks about the IIDs a second digit would
// have named.
const iidExtraDigits = 1

// iidSuffixes are the endings appended to what was typed to name the IIDs it
// begins: none, then every single digit in ascending order, 11 in all.
var iidSuffixes = func() []string {
	suffixes := []string{""}
	for digit := range 10 {
		suffixes = append(suffixes, strconv.Itoa(digit))
	}
	return suffixes
}()

// iidCandidates returns the IIDs that begin with query and run at most
// [iidExtraDigits] digits past it, or none when query cannot begin an IID: it
// holds something other than digits, or begins with a zero, which no IID does.
//
// The candidates ascend, so the first one too long for an int64 ends the list:
// every one after it is longer still.
func iidCandidates(query string) []int64 {
	if !beginsANumber(query) {
		return nil
	}
	candidates := make([]int64, 0, len(iidSuffixes))
	for _, suffix := range iidSuffixes {
		iid, err := strconv.ParseInt(query+suffix, 10, 64)
		if err != nil {
			break
		}
		candidates = append(candidates, iid)
	}
	return candidates
}

// beginsANumber reports whether query can begin an IID or an ID: digits, the
// first of them not a zero, which no identifier GitLab assigns begins with.
func beginsANumber(query string) bool {
	return query != "" && query[0] != '0' && strings.Trim(query, "0123456789") == ""
}

// iidEntry is one merge request or issue a listing answered: its IID, and the
// value the completion offers for it.
type iidEntry struct {
	iid   int64
	value string
}

// iidLister reads one page of a project's merge requests or issues in any
// state. Given IIDs, it asks for those alone, oldest first; given none, it asks
// for the newest.
type iidLister func(ctx context.Context, iids []int64) ([]iidEntry, *gl.Response, error)

// iidPage is the request an IID listing sends, in the fields the merge request
// and issue listing options share: one page, restricted to the IIDs given when
// there are any.
type iidPage struct {
	list    gl.ListOptions
	iids    *[]int64
	orderBy *string
	sort    *string
}

// newIIDPage builds the request for iids. Naming IIDs, it reads them oldest
// first, so that when more candidates match than a page holds, the page holds
// the shortest of them, which are the ones a completion shows first; naming
// none, it keeps GitLab's own order, the newest first.
func newIIDPage(iids []int64) iidPage {
	page := iidPage{list: gl.ListOptions{PerPage: searchPerPage}}
	if iids != nil {
		page.iids = &iids
		page.orderBy = new("created_at")
		page.sort = new("asc")
	}
	return page
}

// listIIDs adapts a merge request or issue listing to an [iidLister]: list
// sends the request, and entry reads one item it answered.
func listIIDs[T any](list func(ctx context.Context, page iidPage) ([]T, *gl.Response, error), entry func(T) iidEntry) iidLister {
	return func(ctx context.Context, iids []int64) ([]iidEntry, *gl.Response, error) {
		items, resp, err := list(ctx, newIIDPage(iids))
		entries := make([]iidEntry, 0, len(items))
		for _, item := range items {
			entries = append(entries, entry(item))
		}
		return entries, resp, err
	}
}

// searchIIDs completes a merge request or issue IID through list, and reports
// the total GitLab counted and whether more may match than it answered.
//
// What was typed is sent to GitLab as the IIDs it begins (see
// [iidCandidates]) in one request with no state filter, so an item is offered
// whatever its age and whether it is open, closed or merged. Reading the 20
// newest open items and filtering them here found none of them. More may match
// when GitLab named a next page, or when an IID [iidExtraDigits] digits longer
// than what was typed came back: the request asked about nothing longer, and
// the project may hold it. A total is reported only for an empty query, where
// GitLab's count is every item; for a typed one it would count only the
// candidates the request named.
func searchIIDs(ctx context.Context, query, operation string, list iidLister) (values []string, total int, more bool, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, false, ctxErr
	}
	var candidates []int64
	if query != "" {
		if candidates = iidCandidates(query); len(candidates) == 0 {
			return []string{}, 0, false, nil
		}
	}
	entries, resp, err := list(ctx, candidates)
	if err != nil {
		return nil, 0, false, fmt.Errorf("%s: %w", operation, err)
	}
	if query == "" {
		return iidValues(entries), totalFromResponse(resp), namesNextPage(resp), nil
	}
	slices.SortFunc(entries, func(a, b iidEntry) int { return cmp.Compare(a.iid, b.iid) })
	n := len(entries)
	reachedTheLongest := n > 0 && len(strconv.FormatInt(entries[n-1].iid, 10)) == len(query)+iidExtraDigits
	return iidValues(entries), 0, namesNextPage(resp) || reachedTheLongest, nil
}

// iidValues returns the value each entry offers, in the entries' order.
func iidValues(entries []iidEntry) []string {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.value)
	}
	return values
}

// namesNextPage reports whether GitLab said a listing goes on past the page it
// answered. It does so in X-Next-Page whether or not it counted the items, which
// it stops doing on a large enough collection.
func namesNextPage(resp *gl.Response) bool {
	return resp != nil && resp.NextPage > 0
}

// searchMRs completes a merge request IID in a project. See [searchIIDs].
func searchMRs(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, more bool, err error) {
	return searchIIDs(ctx, query, "search merge requests", listIIDs(
		func(ctx context.Context, page iidPage) ([]*gl.BasicMergeRequest, *gl.Response, error) {
			opts := &gl.ListProjectMergeRequestsOptions{ListOptions: page.list, IIDs: page.iids, OrderBy: page.orderBy, Sort: page.sort}
			return client.GL().MergeRequests.ListProjectMergeRequests(projectID, opts, gl.WithContext(ctx))
		},
		func(mr *gl.BasicMergeRequest) iidEntry {
			return iidEntry{iid: mr.IID, value: formatMREntry(mr.IID, mr.Title)}
		},
	))
}

// searchIssues completes an issue IID in a project. See [searchIIDs].
func searchIssues(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, more bool, err error) {
	return searchIIDs(ctx, query, "search issues", listIIDs(
		func(ctx context.Context, page iidPage) ([]*gl.Issue, *gl.Response, error) {
			opts := &gl.ListProjectIssuesOptions{ListOptions: page.list, IIDs: page.iids, OrderBy: page.orderBy, Sort: page.sort}
			return client.GL().Issues.ListProjectIssues(projectID, opts, gl.WithContext(ctx))
		},
		func(issue *gl.Issue) iidEntry {
			return iidEntry{iid: issue.IID, value: formatIssueEntry(issue.IID, issue.Title)}
		},
	))
}

// searchBranches returns branch names matching the query plus the total
// match count from the GitLab pagination header.
func searchBranches(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListBranchesOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	branches, resp, err := client.GL().Branches.ListBranches(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search branches: %w", err)
	}
	values = make([]string, 0, len(branches))
	for _, b := range branches {
		values = append(values, b.Name)
	}
	return values, totalFromResponse(resp), nil
}

// searchTags returns tag names matching the query plus the total match
// count from the GitLab pagination header.
func searchTags(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListTagsOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	tags, resp, err := client.GL().Tags.ListTags(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search tags: %w", err)
	}
	values = make([]string, 0, len(tags))
	for _, t := range tags {
		values = append(values, t.Name)
	}
	return values, totalFromResponse(resp), nil
}

// idHalves says which of an ID completer's two requests the operator left in
// place: the lookup of what was typed and the listing of the newest. Each is
// asked about on its own, as the branch and tag halves of
// [Handler.completeBranchOrTag] are, so removing one action keeps the other.
type idHalves struct {
	lookup bool
	list   bool
}

// candidate is one item an ID completer read: the identifier what was typed is
// matched against, and the value the completion offers for it.
type candidate struct {
	key   string
	value string
}

// idSearch is one pipeline, commit or job completer.
//
// What was typed is looked up directly, so an item typed whole is found
// whatever its age, and the newest items are listed beside it and kept when
// their identifier begins with what was typed, the found one first. More may
// match when the listing names a next page: the items past it were not read.
type idSearch struct {
	// operation names the search in an error.
	operation string
	// matchable is false when no identifier of the kind can begin with what
	// was typed, which is answered empty without asking GitLab.
	matchable bool
	// lookup reads the item what was typed names, and reports false when
	// there is none. It is nil when what was typed cannot name one.
	lookup func(ctx context.Context) (candidate, bool, error)
	// list reads one page of the newest items.
	list func(ctx context.Context) ([]candidate, *gl.Response, error)
}

// search runs the completer for what was typed, with the halves the operator
// left.
func (s idSearch) search(ctx context.Context, query string, halves idHalves) (values []string, more bool, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, false, ctxErr
	}
	values = []string{}
	if !s.matchable {
		return values, false, nil
	}
	var found candidate
	var ok bool
	if halves.lookup && s.lookup != nil {
		if found, ok, err = s.lookup(ctx); err != nil {
			return nil, false, fmt.Errorf("%s: look up %q: %w", s.operation, query, err)
		}
		// A lookup can resolve to an item whose identifier does not begin
		// with what was typed, as a commit lookup does for a branch named in
		// hexadecimal, and that item is no completion of it.
		if ok = ok && hasPrefixFold(found.key, query); ok {
			values = append(values, found.value)
		}
	}
	if !halves.list {
		return values, false, nil
	}
	listed, resp, err := s.list(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", s.operation, err)
	}
	for _, item := range listed {
		if hasPrefixFold(item.key, query) && (!ok || item.key != found.key) {
			values = append(values, item.value)
		}
	}
	return values, namesNextPage(resp), nil
}

// hasPrefixFold reports whether key begins with prefix, ignoring case: a SHA
// is the same commit in either case, and a decimal ID has none.
func hasPrefixFold(key, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(key), strings.ToLower(prefix))
}

// missing reads a failed lookup: GitLab's 404 says nothing has that
// identifier, which is an answer rather than a failure.
func missing(err error) (candidate, bool, error) {
	if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return candidate{}, false, nil
	}
	return candidate{}, false, err
}

// idLookup returns the lookup of the ID what was typed spells, or nil when it
// spells none: nothing was typed, or the number is too long for an int64.
func idLookup(query string, get func(ctx context.Context, id int64) (candidate, bool, error)) func(context.Context) (candidate, bool, error) {
	id, err := strconv.ParseInt(query, 10, 64)
	if err != nil {
		return nil
	}
	return func(ctx context.Context) (candidate, bool, error) { return get(ctx, id) }
}

// searchPipelines completes a pipeline ID in a project. See [idSearch].
func searchPipelines(ctx context.Context, client *gitlabclient.Client, projectID, query string, halves idHalves) (values []string, more bool, err error) {
	return idSearch{
		operation: "search pipelines",
		matchable: query == "" || beginsANumber(query),
		lookup: idLookup(query, func(ctx context.Context, id int64) (candidate, bool, error) {
			pipeline, _, getErr := client.GL().Pipelines.GetPipeline(projectID, id, gl.WithContext(ctx))
			if getErr != nil {
				return missing(getErr)
			}
			return candidate{key: strconv.FormatInt(pipeline.ID, 10), value: formatPipelineEntry(pipeline.ID, pipeline.Ref, pipeline.Status)}, true, nil
		}),
		list: func(ctx context.Context) ([]candidate, *gl.Response, error) {
			opts := &gl.ListProjectPipelinesOptions{}
			opts.PerPage = searchPerPage
			opts.OrderBy = new("id")
			opts.Sort = new("desc")
			pipelines, resp, listErr := client.GL().Pipelines.ListProjectPipelines(projectID, opts, gl.WithContext(ctx))
			listed := make([]candidate, 0, len(pipelines))
			for _, p := range pipelines {
				listed = append(listed, candidate{key: strconv.FormatInt(p.ID, 10), value: formatPipelineEntry(p.ID, p.Ref, p.Status)})
			}
			return listed, resp, listErr
		},
	}.search(ctx, query, halves)
}

// shortestAbbreviation is the fewest hexadecimal digits git reads as an
// abbreviated object name. Fewer are read as a ref name, so a shorter SHA
// prefix cannot name a commit and is not looked up.
const shortestAbbreviation = 4

// isHexadecimal reports whether s holds nothing but hexadecimal digits, in
// either case.
func isHexadecimal(s string) bool {
	return strings.Trim(s, "0123456789abcdefABCDEF") == ""
}

// searchCommits completes a commit SHA in a project. See [idSearch].
//
// What was typed is looked up as a commit of the project on any branch, so a
// SHA older than the listing or off the default branch is found; the listing
// is the default branch's newest commits. Each is matched against the full
// SHA, which decides on its own: GitLab's short_id is an abbreviation of id,
// so every prefix that matches the short SHA matches the full one too, and the
// full one also matches a query longer than the abbreviation, which is what
// somebody pasting a whole SHA types. The value offered is the short SHA.
func searchCommits(ctx context.Context, client *gitlabclient.Client, projectID, query string, halves idHalves) (values []string, more bool, err error) {
	var lookup func(context.Context) (candidate, bool, error)
	if len(query) >= shortestAbbreviation && isHexadecimal(query) {
		lookup = func(ctx context.Context) (candidate, bool, error) {
			// stats=false: GitLab computes a commit's diff stats unless told
			// not to, and a completion reads only the SHA and the title.
			commit, _, getErr := client.GL().Commits.GetCommit(projectID, strings.ToLower(query), &gl.GetCommitOptions{Stats: new(false)}, gl.WithContext(ctx))
			if getErr != nil {
				return missing(getErr)
			}
			return candidate{key: commit.ID, value: formatCommitEntry(commit.ShortID, commit.Title)}, true, nil
		}
	}
	return idSearch{
		operation: "search commits",
		matchable: isHexadecimal(query),
		lookup:    lookup,
		list: func(ctx context.Context) ([]candidate, *gl.Response, error) {
			opts := &gl.ListCommitsOptions{}
			opts.PerPage = searchPerPage
			commits, resp, listErr := client.GL().Commits.ListCommits(projectID, opts, gl.WithContext(ctx))
			listed := make([]candidate, 0, len(commits))
			for _, c := range commits {
				listed = append(listed, candidate{key: c.ID, value: formatCommitEntry(c.ShortID, c.Title)})
			}
			return listed, resp, listErr
		},
	}.search(ctx, query, halves)
}

// searchLabels returns label names for a project matching the query plus
// the total match count from the GitLab pagination header.
func searchLabels(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListLabelsOptions{}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	labels, resp, err := client.GL().Labels.ListLabels(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search labels: %w", err)
	}
	values = make([]string, 0, len(labels))
	for _, l := range labels {
		values = append(values, l.Name)
	}
	return values, totalFromResponse(resp), nil
}

// searchMilestones returns milestone entries for a project plus the total
// match count from the GitLab pagination header.
func searchMilestones(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, err error) {
	return searchMilestonesWithFormatter(ctx, client, projectID, query, "search milestones", func(m *gl.Milestone) string {
		return formatMilestoneEntry(m.ID, m.Title)
	})
}

func searchMilestonesWithFormatter(ctx context.Context, client *gitlabclient.Client, projectID, query, operation string, format func(*gl.Milestone) string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListMilestonesOptions{
		State: new("active"),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	milestones, resp, err := client.GL().Milestones.ListMilestones(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", operation, err)
	}
	values = make([]string, 0, len(milestones))
	for _, m := range milestones {
		values = append(values, format(m))
	}
	return values, totalFromResponse(resp), nil
}

// searchMilestoneTitles returns active milestone titles for a project, filtered by query.
// Unlike [searchMilestones], it returns plain titles (not "id: title") for use as
// completion values for prompt arguments that accept a milestone title.
func searchMilestoneTitles(ctx context.Context, client *gitlabclient.Client, projectID, query string) (values []string, total int, err error) {
	return searchMilestonesWithFormatter(ctx, client, projectID, query, "search milestone titles", func(m *gl.Milestone) string {
		return m.Title
	})
}

// searchGroupMilestoneTitles returns active group milestone titles, filtered by query.
// Used by prompts that scope milestones to a group rather than a project (for
// example group_milestone_progress).
func searchGroupMilestoneTitles(ctx context.Context, client *gitlabclient.Client, groupID, query string) (values []string, total int, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	opts := &gl.ListGroupMilestonesOptions{
		State: new("active"),
	}
	opts.PerPage = searchPerPage
	if query != "" {
		opts.Search = new(query)
	}
	milestones, resp, err := client.GL().GroupMilestones.ListGroupMilestones(groupID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("search group milestone titles: %w", err)
	}
	values = make([]string, 0, len(milestones))
	for _, m := range milestones {
		values = append(values, m.Title)
	}
	return values, totalFromResponse(resp), nil
}

// searchJobs completes a job ID in a project, within one pipeline when
// pipelineID is positive and across the project's jobs when it is zero, which
// is what the job resource template asks, since it names no pipeline. A job
// the lookup finds in another pipeline than the one asked for is not offered.
// See [idSearch].
func searchJobs(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64, query string, halves idHalves) (values []string, more bool, err error) {
	return idSearch{
		operation: "search jobs",
		matchable: query == "" || beginsANumber(query),
		lookup: idLookup(query, func(ctx context.Context, id int64) (candidate, bool, error) {
			job, _, getErr := client.GL().Jobs.GetJob(projectID, id, gl.WithContext(ctx))
			if getErr != nil {
				return missing(getErr)
			}
			if pipelineID > 0 && job.Pipeline.ID != pipelineID {
				return candidate{}, false, nil
			}
			return candidate{key: strconv.FormatInt(job.ID, 10), value: formatJobEntry(job.ID, job.Name, job.Status)}, true, nil
		}),
		list: func(ctx context.Context) ([]candidate, *gl.Response, error) {
			opts := &gl.ListJobsOptions{}
			opts.PerPage = searchPerPage
			var jobs []*gl.Job
			var resp *gl.Response
			var listErr error
			if pipelineID > 0 {
				jobs, resp, listErr = client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, opts, gl.WithContext(ctx))
			} else {
				jobs, resp, listErr = client.GL().Jobs.ListProjectJobs(projectID, opts, gl.WithContext(ctx))
			}
			listed := make([]candidate, 0, len(jobs))
			for _, j := range jobs {
				listed = append(listed, candidate{key: strconv.FormatInt(j.ID, 10), value: formatJobEntry(j.ID, j.Name, j.Status)})
			}
			return listed, resp, listErr
		},
	}.search(ctx, query, halves)
}
