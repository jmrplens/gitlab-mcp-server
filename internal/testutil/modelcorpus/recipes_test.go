package modelcorpus

import (
	"slices"
	"testing"
)

// auditFacts is the fixture state every stimulus is rendered with offline: the
// corpus gate renders each prompt to look for the literals its key compares,
// and the prompt audit renders it to look for the answer.
//
// It lives in a test file and nowhere else, on purpose. A run renders a
// stimulus over the facts its recipe actually built on the instance under
// test; a table of plausible values here would let a run send a prompt naming
// a project that does not exist, which is the state the evaluator this
// replaces was in when it sent every case to a mock backend.
//
// The values are the ones that evaluator rendered its mock runs with, so a
// reader comparing an old prompt with a new one is comparing the case rather
// than the fixture.
func auditFacts() map[string]string {
	return map[string]string{
		FactProjectPath:           "my-org/tools/gitlab-mcp-server",
		FactDefaultBranch:         "main",
		FactRemoteURL:             "https://gitlab.example.com/my-org/tools/gitlab-mcp-server.git",
		FactGroupPath:             "my-org",
		FactGroupID:               "123",
		FactArtifactPath:          "coverage/report.xml",
		FactBranchName:            "feature/eval",
		FactCIVariableKey:         "EVAL_CRUD_TOKEN",
		FactDeploymentID:          "77",
		FactDiscussionID:          "abc123",
		FactEnvironmentID:         "7",
		FactEnvironmentName:       "eval-deploy",
		FactInstanceCIVariableKey: "INSTANCE_EVAL_TOKEN",
		FactIssueIID:              "42",
		FactJobID:                 "999",
		FactMergeRequestIID:       "7",
		FactMergeRequestSource:    "feature/eval",
		FactPackageDir:            "/tmp/eval-packages",
		FactPackageFilesDisplay:   "checksums, release-notes",
		FactPackageName:           "eval-package",
		FactPackageTag:            "v0.0.0-eval-package",
		FactPackageVersion:        "0.1.0",
		FactPipelineID:            "12345",
		FactPipelineIID:           "3",
		FactReleaseName:           "v0.0.0-eval",
		FactReleaseTagName:        "v0.0.0-eval",
		FactRunnerID:              "99",
		FactSnippetID:             "33",

		// The destructive and licensed worlds. Where the old evaluator
		// rendered a value for one of these, it is that value; where it
		// spelled the same thing into the prompt by hand, the value is
		// what it spelled.
		FactProjectID:               "123",
		FactAttestationIID:          "5",
		FactAuditEventID:            "77",
		FactAwardID:                 "12",
		FactBadgeID:                 "8",
		FactBroadcastMessageID:      "12",
		FactCommitDiscussionID:      "abc123",
		FactCommitNoteID:            "999",
		FactCommitSHA:               "abc1234",
		FactCustomEmojiID:           "gid://gitlab/CustomEmoji/77",
		FactDatabaseMigration:       "20260101000000",
		FactDependencyExportID:      "987",
		FactDeployKeyID:             "88",
		FactDeployTokenID:           "66",
		FactEpicDiscussionID:        "def456",
		FactEpicIID:                 "12",
		FactEpicNoteID:              "44",
		FactExternalStatusCheckID:   "8",
		FactFeatureFlagName:         "eval_flag",
		FactFilePath:                "tmp/eval.txt",
		FactGeoSiteID:               "3",
		FactGeoSiteName:             "eval-geo",
		FactGeoSiteURL:              "https://geo.example.com",
		FactGroupAccessTokenID:      "77",
		FactHookID:                  "5",
		FactJobTokenTargetProjectID: "456",
		FactLDAPProvider:            "ldapmain",
		FactMemberRoleID:            "44",
		FactMilestoneIID:            "7",
		FactMirrorID:                "9",
		FactModelFileName:           "model.onnx",
		FactModelFilePath:           "models",
		FactModelVersionID:          "candidate:5",
		FactPackageID:               "55",
		FactPipelineScheduleID:      "12",
		FactPipelineTriggerID:       "77",
		FactProjectAccessTokenID:    "77",
		FactProjectAliasName:        "eval-alias",
		FactProtectedBranchName:     "release/*",
		FactProtectedEnvironment:    "production",
		FactSAMLGroupName:           "Engineering",
		FactScimUID:                 "external-123",
		FactServiceAccountID:        "55",
		FactServiceAccountTokenID:   "66",
		FactServiceAccountUsername:  "eval-service-account",
		FactSSHCertificateID:        "44",
		FactStorageMoveID:           "77",
		FactTagName:                 "v0.0.0-eval",
		FactTerraformStateName:      "production",
		FactUserID:                  "55",
		FactVulnerabilityID:         "gid://gitlab/Vulnerability/42",
		FactWikiSlug:                "obsolete-eval",
	}
}

// TestRecipes_EnumerationMatchesThePromises keeps the list a builder gate
// iterates from drifting away from the table a case is checked against: a
// recipe in one and not the other is either a world nothing builds or a
// builder nothing asks for.
func TestRecipes_EnumerationMatchesThePromises(t *testing.T) {
	listed := Recipes()
	if len(listed) != len(recipeFacts) {
		t.Errorf("Recipes() lists %d recipes and the promise table holds %d", len(listed), len(recipeFacts))
	}
	seen := map[Recipe]bool{}
	for _, recipe := range listed {
		if seen[recipe] {
			t.Errorf("Recipes() lists %q twice", recipe)
		}
		seen[recipe] = true
		if _, known := Facts(recipe); !known {
			t.Errorf("Recipes() lists %q, which promises no facts", recipe)
		}
	}
	for recipe := range recipeFacts {
		if !seen[recipe] {
			t.Errorf("recipe %q promises facts and Recipes() does not list it", recipe)
		}
	}
}

// TestRecipes_EveryOneIsNamedByACase checks the other direction of the same
// join. A recipe no case names is a world the fixture library would be asked
// to build for nothing, and the corpus is where that is visible.
func TestRecipes_EveryOneIsNamedByACase(t *testing.T) {
	named := map[Recipe]bool{}
	for _, one := range cases() {
		named[one.Recipe] = true
	}
	for _, recipe := range Recipes() {
		if !named[recipe] {
			t.Errorf("recipe %q is named by no case", recipe)
		}
	}
}

// TestRecipes_EveryPromisedFactHasAnOfflineValue keeps the offline table and
// the promises together: a fact promised with no value here renders as an
// error the moment a case interpolates it, and a value here for a fact nothing
// promises is a value no run could ever produce.
func TestRecipes_EveryPromisedFactHasAnOfflineValue(t *testing.T) {
	values := auditFacts()
	promised := map[string]bool{}
	for _, recipe := range Recipes() {
		facts, _ := Facts(recipe)
		for _, key := range facts {
			promised[key] = true
			if values[key] == "" {
				t.Errorf("recipe %q promises fact %q, which has no offline value", recipe, key)
			}
		}
	}
	for key := range values {
		if !promised[key] {
			t.Errorf("the offline table holds fact %q, which no recipe promises", key)
		}
	}
}

// TestFacts_UnknownRecipeIsReportedAndTheAnswerIsACopy covers both halves of
// the accessor: an unknown recipe is a "no" rather than an empty promise, and
// the promises a caller gets back are its own, so a consumer sorting or
// filtering them cannot reach into the corpus.
func TestFacts_UnknownRecipeIsReportedAndTheAnswerIsACopy(t *testing.T) {
	if facts, known := Facts("no-such-recipe"); known || facts != nil {
		t.Errorf("Facts(unknown) = %v, %t, want nil, false", facts, known)
	}
	first, known := Facts(RecipeWorld)
	if !known {
		t.Fatal("Facts(RecipeWorld) reported the world unknown")
	}
	slices.Sort(first)
	first[0] = "clobbered"
	second, _ := Facts(RecipeWorld)
	if slices.Contains(second, "clobbered") {
		t.Errorf("Facts returned the corpus's own slice: %v", second)
	}
	if !slices.Contains(second, FactProjectPath) {
		t.Errorf("Facts(RecipeWorld) = %v, want the project path among them", second)
	}
}

// TestWithProjectFacts_GivesEachRecipeItsOwnSlice pins the reason the helper
// builds a slice rather than appending to a shared one: two recipes sharing a
// backing array would have one of them overwrite the other's extra fact.
func TestWithProjectFacts_GivesEachRecipeItsOwnSlice(t *testing.T) {
	first := withProjectFacts("one")
	second := withProjectFacts("two")
	if got := first[len(first)-1]; got != "one" {
		t.Errorf("the first slice ends with %q, want one", got)
	}
	if got := second[len(second)-1]; got != "two" {
		t.Errorf("the second slice ends with %q, want two", got)
	}
	if len(first) != 5 || first[0] != FactProjectPath {
		t.Errorf("withProjectFacts(one) = %v, want the four project facts and one more", first)
	}
}

// TestWithGroupFacts_GivesEachRecipeItsOwnSlice is the same rule for the group
// half, which exists for the same reason: a recipe that builds a group
// promises both spellings of it, and spelling that out per recipe is how one
// of them ends up promising only the path.
func TestWithGroupFacts_GivesEachRecipeItsOwnSlice(t *testing.T) {
	first := withGroupFacts("one")
	second := withGroupFacts("two")
	if got := first[len(first)-1]; got != "one" {
		t.Errorf("the first slice ends with %q, want one", got)
	}
	if got := second[len(second)-1]; got != "two" {
		t.Errorf("the second slice ends with %q, want two", got)
	}
	if len(first) != 3 || first[0] != FactGroupPath || first[1] != FactGroupID {
		t.Errorf("withGroupFacts(one) = %v, want both group facts and one more", first)
	}
}
