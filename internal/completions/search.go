package completions

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// searchPerPage is intentionally larger than maxCompletionResults (10) so
// that toResult can compute an accurate HasMore flag after truncation.
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
	if query == "" || query[0] == '0' || strings.Trim(query, "0123456789") != "" {
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

// searchPipelines returns recent pipelines for a project, filtered by ID prefix.
func searchPipelines(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListProjectPipelinesOptions{}
	opts.PerPage = searchPerPage
	opts.OrderBy = new("id")
	opts.Sort = new("desc")
	pipelines, _, err := client.GL().Pipelines.ListProjectPipelines(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search pipelines: %w", err)
	}
	values := make([]string, 0, len(pipelines))
	for _, p := range pipelines {
		entry := formatPipelineEntry(p.ID, p.Ref, p.Status)
		if query == "" || strings.HasPrefix(strconv.FormatInt(p.ID, 10), query) {
			values = append(values, entry)
		}
	}
	return values, nil
}

// searchCommits returns recent commits for a project, filtered by SHA prefix.
func searchCommits(ctx context.Context, client *gitlabclient.Client, projectID, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListCommitsOptions{}
	opts.PerPage = searchPerPage
	commits, _, err := client.GL().Commits.ListCommits(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search commits: %w", err)
	}
	values := make([]string, 0, len(commits))
	for _, c := range commits {
		entry := formatCommitEntry(c.ShortID, c.Title)
		// The full SHA decides on its own. GitLab's short_id is an abbreviation
		// of id, so every prefix that matches the short SHA matches the full
		// one too, and a second test against the abbreviation could not change
		// an answer; the full one additionally matches a query longer than the
		// abbreviation, which is what somebody pasting a whole SHA types.
		if query == "" || strings.HasPrefix(strings.ToLower(c.ID), strings.ToLower(query)) {
			values = append(values, entry)
		}
	}
	return values, nil
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

// searchJobs returns job entries for a pipeline, filtered by ID prefix.
func searchJobs(ctx context.Context, client *gitlabclient.Client, projectID string, pipelineID int64, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts := &gl.ListJobsOptions{}
	opts.PerPage = searchPerPage
	jobs, _, err := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("search jobs: %w", err)
	}
	values := make([]string, 0, len(jobs))
	for _, j := range jobs {
		entry := formatJobEntry(j.ID, j.Name, j.Status)
		if query == "" || strings.HasPrefix(strconv.FormatInt(j.ID, 10), query) {
			values = append(values, entry)
		}
	}
	return values, nil
}
