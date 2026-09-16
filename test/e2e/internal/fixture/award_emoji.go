//go:build e2e

// award_emoji.go puts an award emoji on an issue or a merge request, which is
// the object an award case reads or removes.
//
// One emoji name is used everywhere and a repeat award is read back rather
// than reported: GitLab holds one award per user, emoji and object, and
// refuses a second with "has already been taken". That refusal is what a
// retried create sees after an attempt whose answer was lost, and treating it
// as a failure would turn a flaky network into a failed fixture.

package fixture

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// AwardEmojiName is the emoji every fixture award carries, spelled once so a
// case comparing what it read knows the name to expect.
const AwardEmojiName = "eyes"

// awardAlreadyTaken is what GitLab answers an award the user already gave.
const awardAlreadyTaken = "already been taken"

// Award is an award emoji a builder created.
type Award struct {
	// ID is what the award actions take.
	ID int64
	// Name is the emoji itself.
	Name string
}

// NewMergeRequestAward awards the fixture emoji on the merge request and
// registers its removal.
func NewMergeRequestAward(e *harness.Env, project Project, mergeRequestIID int64) Award {
	e.T.Helper()

	award, err := retryTransient(e, "create merge request award", createRetries, func() (Award, error) {
		return createMergeRequestAward(e.Ctx, e.Client(), project.ID, mergeRequestIID)
	})
	if err != nil {
		e.T.Fatalf("awarding %q on merge request !%d of project %d: %v", AwardEmojiName, mergeRequestIID, project.ID, err)
	}

	e.Defer(fmt.Sprintf("merge request award %d", award.ID), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, deleteErr := e.Client().GL().AwardEmoji.DeleteMergeRequestAwardEmoji(project.ID, mergeRequestIID, award.ID, gl.WithContext(ctx))
		return toleratingGoneAward(deleteErr, award.ID)
	})
	return award
}

// NewIssueAward awards the fixture emoji on the issue and registers its
// removal.
func NewIssueAward(e *harness.Env, project Project, issueIID int64) Award {
	e.T.Helper()

	award, err := retryTransient(e, "create issue award", createRetries, func() (Award, error) {
		return createIssueAward(e.Ctx, e.Client(), project.ID, issueIID)
	})
	if err != nil {
		e.T.Fatalf("awarding %q on issue #%d of project %d: %v", AwardEmojiName, issueIID, project.ID, err)
	}

	e.Defer(fmt.Sprintf("issue award %d", award.ID), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, deleteErr := e.Client().GL().AwardEmoji.DeleteIssueAwardEmoji(project.ID, issueIID, award.ID, gl.WithContext(ctx))
		return toleratingGoneAward(deleteErr, award.ID)
	})
	return award
}

// createMergeRequestAward awards the emoji, reading back one this user
// already gave.
func createMergeRequestAward(ctx context.Context, client *gitlabclient.Client, projectID, mergeRequestIID int64) (Award, error) {
	created, _, err := client.GL().AwardEmoji.CreateMergeRequestAwardEmoji(projectID, mergeRequestIID,
		&gl.CreateAwardEmojiOptions{Name: AwardEmojiName}, gl.WithContext(ctx))
	if err == nil {
		return Award{ID: created.ID, Name: created.Name}, nil
	}
	if !strings.Contains(err.Error(), awardAlreadyTaken) {
		return Award{}, err
	}
	existing, _, listErr := client.GL().AwardEmoji.ListMergeRequestAwardEmoji(projectID, mergeRequestIID, nil, gl.WithContext(ctx))
	if listErr != nil {
		return Award{}, fmt.Errorf("listing the awards on merge request !%d that already carries one: %w", mergeRequestIID, listErr)
	}
	return firstAwardNamed(existing, mergeRequestIID)
}

// createIssueAward is its issue half.
func createIssueAward(ctx context.Context, client *gitlabclient.Client, projectID, issueIID int64) (Award, error) {
	created, _, err := client.GL().AwardEmoji.CreateIssueAwardEmoji(projectID, issueIID,
		&gl.CreateAwardEmojiOptions{Name: AwardEmojiName}, gl.WithContext(ctx))
	if err == nil {
		return Award{ID: created.ID, Name: created.Name}, nil
	}
	if !strings.Contains(err.Error(), awardAlreadyTaken) {
		return Award{}, err
	}
	existing, _, listErr := client.GL().AwardEmoji.ListIssueAwardEmoji(projectID, issueIID, nil, gl.WithContext(ctx))
	if listErr != nil {
		return Award{}, fmt.Errorf("listing the awards on issue #%d that already carries one: %w", issueIID, listErr)
	}
	return firstAwardNamed(existing, issueIID)
}

// firstAwardNamed picks the fixture's own emoji out of a listing, and reports
// a listing that does not hold it rather than handing back a zero ID the
// caller would delete nothing with.
func firstAwardNamed(awards []*gl.AwardEmoji, objectIID int64) (Award, error) {
	for _, award := range awards {
		if award != nil && award.Name == AwardEmojiName {
			return Award{ID: award.ID, Name: award.Name}, nil
		}
	}
	return Award{}, fmt.Errorf("the award %q was refused as already given on %d and is not in the listing", AwardEmojiName, objectIID)
}

// toleratingGoneAward reports a removal failure unless the award is already
// gone, which is how a case that removed it ends.
func toleratingGoneAward(err error, awardID int64) error {
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting award %d: %w", awardID, err)
	}
	return nil
}
