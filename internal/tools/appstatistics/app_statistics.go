package appstatistics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GetInput is the input (no params).
type GetInput struct{}

// GetOutput is the output for application statistics.
type GetOutput struct {
	toolutil.HintableOutput
	Forks         int64 `json:"forks"`
	Issues        int64 `json:"issues"`
	MergeRequests int64 `json:"merge_requests"`
	Notes         int64 `json:"notes"`
	Snippets      int64 `json:"snippets"`
	SSHKeys       int64 `json:"ssh_keys"`
	Milestones    int64 `json:"milestones"`
	Users         int64 `json:"users"`
	Groups        int64 `json:"groups"`
	Projects      int64 `json:"projects"`
	ActiveUsers   int64 `json:"active_users"`
}

// rawStatistics is the answer of GET /application/statistics as GitLab sends
// it, each count a [delimitedCount].
type rawStatistics struct {
	Forks         delimitedCount `json:"forks"`
	Issues        delimitedCount `json:"issues"`
	MergeRequests delimitedCount `json:"merge_requests"`
	Notes         delimitedCount `json:"notes"`
	Snippets      delimitedCount `json:"snippets"`
	SSHKeys       delimitedCount `json:"ssh_keys"`
	Milestones    delimitedCount `json:"milestones"`
	Users         delimitedCount `json:"users"`
	Groups        delimitedCount `json:"groups"`
	Projects      delimitedCount `json:"projects"`
	ActiveUsers   delimitedCount `json:"active_users"`
}

// delimitedCount is one count of the statistics answer, which GitLab sends as
// a string rather than as a number: API::Entities::ApplicationStatistics
// renders every count through Rails' number_with_delimiter, and the API sets
// the locale to the caller's preferred_language first, so the digits are
// grouped the way that language groups them: "1,234", "1.234" or "1 234"
// among the languages GitLab offers (docs/development/upstream-bugs.md,
// entry 7, names which language gives which). A count below a thousand
// carries no separator, and a JSON number is read as well.
//
// It mirrors the UnmarshalJSON client-go's ApplicationStatistics gains in
// gitlab-org/api/client-go!3063, so that switching this handler to
// GetApplicationStatistics once a release carries it changes nothing a
// caller reads.
type delimitedCount int64

// UnmarshalJSON reads a count sent as a JSON number or as a string grouped by
// a separator, and refuses one it cannot read rather than leaving it at zero,
// which would read as a fact about the instance. A JSON null reads as zero, as
// it does in client-go's decoding; GitLab never sends one, since its count
// helper raises rather than rendering a missing count.
func (c *delimitedCount) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(data, []byte(`"`)) {
		var n int64
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		*c = delimitedCount(n)
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	n, err := parseDelimitedCount(s)
	if err != nil {
		return err
	}
	*c = delimitedCount(n)
	return nil
}

// parseDelimitedCount parses an integer whose digit groups are separated by a
// language's delimiter. Any space or punctuation character is taken as that
// delimiter, except a minus sign in the first position: the fork count is
// fork network members less fork networks, both approximated, and Rails
// writes a negative number with its sign in front of the first group. Any
// other character refuses the count.
func parseDelimitedCount(s string) (int64, error) {
	var digits strings.Builder
	for i, r := range s {
		if (r >= '0' && r <= '9') || (r == '-' && i == 0) {
			digits.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue // A group delimiter.
		}
		return 0, fmt.Errorf("invalid count %q", s)
	}

	n, err := strconv.ParseInt(digits.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid count %q: %w", s, err)
	}
	return n, nil
}

// statisticsPath is the route [Get] reads.
const statisticsPath = "application/statistics"

// Get retrieves current application statistics (admin).
//
// It builds its own request rather than calling client-go's
// GetApplicationStatistics, whose ApplicationStatistics declares int64
// fields and so cannot decode the strings GitLab sends for every count (see
// [delimitedCount]). Reported as gitlab-org/api/client-go!3063 and tracked in
// docs/development/upstream-bugs.md, entry 7; once a release carries that
// change, this handler calls the SDK method directly.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	raw, err := fetchStatistics(ctx, client, statisticsPath)
	if err != nil {
		return GetOutput{}, err
	}

	return GetOutput{
		Forks:         int64(raw.Forks),
		Issues:        int64(raw.Issues),
		MergeRequests: int64(raw.MergeRequests),
		Notes:         int64(raw.Notes),
		Snippets:      int64(raw.Snippets),
		SSHKeys:       int64(raw.SSHKeys),
		Milestones:    int64(raw.Milestones),
		Users:         int64(raw.Users),
		Groups:        int64(raw.Groups),
		Projects:      int64(raw.Projects),
		ActiveUsers:   int64(raw.ActiveUsers),
	}, nil
}

// fetchStatistics sends GET path and decodes the answer into [rawStatistics].
// The path is a parameter so a test can hand it one NewRequest refuses, which
// is the only way to reach that branch: the route [Get] passes is a constant
// NewRequest always accepts.
func fetchStatistics(ctx context.Context, client *gitlabclient.Client, path string) (rawStatistics, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, nil)
	if err != nil {
		return rawStatistics{}, toolutil.WrapErrWithMessage("get_application_statistics", err)
	}
	req = req.WithContext(ctx)

	var raw rawStatistics
	if _, err = client.GL().Do(req, &raw); err != nil {
		// The hint names a role, so it follows a permission refusal rather
		// than a status: authenticated_as_admin! answers 403, and a 403
		// naming an RFC 6750 code refuses the token's scope, not its owner.
		if toolutil.IsPermissionRefusal(err) {
			return rawStatistics{}, toolutil.WrapErrWithHint("get_application_statistics", err, "application statistics require administrator access")
		}
		return rawStatistics{}, toolutil.WrapErrWithMessage("get_application_statistics", err)
	}
	return raw, nil
}
