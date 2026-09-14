//go:build e2e

// achievements_test.go covers the achievements domain, which is twelve actions
// and was reached by nothing.
//
// It is one lifecycle rather than twelve tests, because every action here needs
// what the one before it made: a definition to award, an award to revoke or
// reorder, a recipient to count. Twelve independent tests would each rebuild
// that chain, and each would be asserting the same GraphQL mutations a second
// time over.
//
// The whole domain is GraphQL, and the awards have a shape worth knowing before
// reading the assertions: an achievement definition and an award of it are two
// records with two IDs, and revoking, hiding or deleting an award addresses the
// award's, never the definition's. Using the wrong one is the mistake the
// handlers exist to prevent and the reason each assertion below names which ID
// it is holding.
//
// One group of its own, because a definition belongs to a namespace and the
// listing is by namespace: sharing the World's group would make the listing
// assertions depend on what every other scenario happened to leave there.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/achievements"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestAchievement_Lifecycle_CreateAwardReorderRevokeDelete walks the whole
// domain once, on one surface.
//
// One surface rather than three: these are twelve GraphQL mutations and
// queries reached through the same catalog on every surface, and the surfaces
// differ in how a call is named rather than in what it does. Running the chain
// three times would triple the GitLab work to prove the projection again, which
// the served-set check already proves for every action at session start.
//
// The awards go to the run's own user, which every runtime has and no fixture
// has to create. Achievements need only the admin_achievement permission a
// group Owner has, so this runs on an ordinary Docker instance with no license
// and no feature flag.
func TestAchievement_Lifecycle_CreateAwardReorderRevokeDelete(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("achievement"))
	userID := e.Runtime().UserID
	username := e.Runtime().Username

	created := harness.Do[achievements.Output](s, actionAchievementCreate, map[string]any{
		"namespace_id": group.ID,
		"name":         e.Name("first-contribution"),
		"description":  "Awarded by the e2e suite",
	})
	if created.Achievement.ID == 0 {
		t.Fatalf("create answered %+v, want an achievement with an ID", created.Achievement)
	}
	achievementID := created.Achievement.ID

	t.Run("update changes the definition", func(t *testing.T) {
		updated := harness.Do[achievements.Output](s, actionAchievementUpdate, map[string]any{
			"achievement_id": achievementID,
			"description":    "Awarded by the e2e suite, renamed",
		})
		if updated.Achievement.ID != achievementID {
			t.Errorf("update answered achievement %d, want %d", updated.Achievement.ID, achievementID)
		}
		if updated.Achievement.Description != "Awarded by the e2e suite, renamed" {
			t.Errorf("the description is %q, want the updated one", updated.Achievement.Description)
		}
	})

	t.Run("the namespace lists it", func(t *testing.T) {
		listed := harness.Do[achievements.ListOutput](s, actionAchievementList, map[string]any{
			"full_path": group.Path,
		})
		if !listsAchievement(listed.Achievements, achievementID) {
			t.Errorf("the namespace listing does not carry achievement %d: %+v", achievementID, listed.Achievements)
		}
	})

	// Two awards of one definition to one user, because the reorder needs
	// something to order and GitLab allows a repeat award.
	firstAward := awardAchievement(t, s, achievementID, userID, "first")
	secondAward := awardAchievement(t, s, achievementID, userID, "second")

	t.Run("the recipients carry both awards", func(t *testing.T) {
		recipients := harness.Do[achievements.UserAchievementListOutput](s, actionAchievementRecipients, map[string]any{
			"full_path": group.Path, "achievement_id": achievementID,
		})
		if len(recipients.UserAchievements) < 2 {
			t.Errorf("the recipients list carries %d awards, want the two just made", len(recipients.UserAchievements))
		}
	})

	t.Run("the unique users deduplicate the repeat award", func(t *testing.T) {
		// The point of this action rather than the recipients one: two awards
		// to one person are two recipients and one user, and a caller asking
		// how many people hold an achievement wants the second number.
		unique := harness.Do[achievements.UniqueUsersOutput](s, actionAchievementUniqueUsers, map[string]any{
			"full_path": group.Path, "achievement_id": achievementID,
		})
		if len(unique.Users) != 1 {
			t.Errorf("the unique recipients are %d, want one person holding two awards", len(unique.Users))
		}
	})

	t.Run("the user's own awards list", func(t *testing.T) {
		owned := harness.Do[achievements.UserAchievementListOutput](s, actionAchievementUserList, map[string]any{
			"username": username,
		})
		if len(owned.UserAchievements) < 2 {
			t.Errorf("the user holds %d awards by this listing, want at least the two just made",
				len(owned.UserAchievements))
		}
	})

	t.Run("an award is hidden from the profile", func(t *testing.T) {
		// The award's own ID, not the definition's: this changes one award's
		// visibility and leaves the other showing.
		hidden := harness.Do[achievements.UserAchievementMutationOutput](s, actionAchievementUserAchievementUpdate,
			map[string]any{"user_achievement_id": firstAward, "show_on_profile": false})
		if hidden.UserAchievement.ShowOnProfile {
			t.Errorf("award %d still shows on the profile after being hidden", firstAward)
		}
	})

	t.Run("the awards are reordered", func(t *testing.T) {
		reordered := harness.Do[achievements.ReorderOutput](s, actionAchievementUserAchievementReord,
			map[string]any{"user_achievement_ids": []int64{secondAward, firstAward}})
		if len(reordered.UserAchievements) < 2 {
			t.Errorf("the reorder answered %d awards, want the set it was given", len(reordered.UserAchievements))
		}
	})

	t.Run("an award is revoked and kept", func(t *testing.T) {
		// Revoking and deleting are different endings and this asserts the
		// first: the record survives, marked revoked, which is what a history
		// of who held what depends on.
		revoked := harness.Do[achievements.UserAchievementMutationOutput](s, actionAchievementRevoke,
			map[string]any{"user_achievement_id": firstAward})
		if revoked.UserAchievement.ID != firstAward {
			t.Errorf("revoke answered award %d, want %d", revoked.UserAchievement.ID, firstAward)
		}
	})

	t.Run("the other award is deleted outright", func(t *testing.T) {
		deleted := harness.Do[achievements.UserAchievementMutationOutput](s, actionAchievementUserAchievementDelete,
			map[string]any{"user_achievement_id": secondAward})
		if deleted.UserAchievement.ID != secondAward {
			t.Errorf("delete answered award %d, want %d", deleted.UserAchievement.ID, secondAward)
		}
	})

	t.Run("the definition is deleted", func(t *testing.T) {
		deleted := harness.Do[achievements.DeleteOutput](s, actionAchievementDelete,
			map[string]any{"achievement_id": achievementID})
		if deleted.Achievement.ID != achievementID {
			t.Errorf("delete answered achievement %d, want %d", deleted.Achievement.ID, achievementID)
		}

		remaining := harness.Do[achievements.ListOutput](s, actionAchievementList,
			map[string]any{"full_path": group.Path})
		if listsAchievement(remaining.Achievements, achievementID) {
			t.Errorf("achievement %d is still listed after being deleted", achievementID)
		}
	})
}

// awardAchievement hands the achievement to a user and returns the award's own
// ID, which every later call addresses.
func awardAchievement(t *testing.T, s *harness.Session, achievementID, userID int64, message string) int64 {
	t.Helper()

	awarded := harness.Do[achievements.UserAchievementOutput](s, actionAchievementAward, map[string]any{
		"achievement_id": achievementID,
		"user_id":        userID,
		"award_message":  message,
	})
	if awarded.UserAchievement.ID == 0 {
		t.Fatalf("award answered %+v, want an award with an ID of its own", awarded.UserAchievement)
	}
	return awarded.UserAchievement.ID
}

// listsAchievement reports whether a listing carries the given definition.
func listsAchievement(listed []achievements.Achievement, id int64) bool {
	for _, achievement := range listed {
		if achievement.ID == id {
			return true
		}
	}
	return false
}
