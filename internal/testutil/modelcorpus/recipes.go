package modelcorpus

// Recipe names the world a case runs in. The builders live in the end-to-end
// fixture library, behind the e2e tag; what lives here is the name a case
// refers to and the fact keys the builder promises to produce.
//
// The facts are the join between the two halves of a case. A prompt may
// interpolate a promised key and nothing else, and an argument may bind its
// truth to a promised key and nothing else, so a case cannot ask about a value
// its world does not build, and a value the world builds is written into the
// stimulus by the run rather than into the corpus by an author.
type Recipe string

// The worlds the Free corpus runs in.
//
// RecipeWorld is the one shared, read-only world: a project with a README and
// a default branch, inside a group, which any number of attempts may read at
// once. Every other recipe builds state of the attempt's own, which is what
// lets a mutating case name a literal ("Evaluation Sprint", "EVAL_TOKEN")
// without two attempts colliding over it.
const (
	// RecipeWorld is the shared read-only project and its group.
	RecipeWorld Recipe = "world"
	// RecipeProject is a project of this attempt's own, with a README and a
	// CI configuration, so a case may create a pipeline in it.
	RecipeProject Recipe = "project"
	// RecipeGroup is a group of this attempt's own, for the cases that
	// create something directly under a group.
	RecipeGroup Recipe = "group"
	// RecipeBranch is a project with a branch beside the default one.
	RecipeBranch Recipe = "branch"
	// RecipeIssue is a project with an issue in it.
	RecipeIssue Recipe = "issue"
	// RecipeMergeRequest is a project with an open merge request.
	RecipeMergeRequest Recipe = "merge_request"
	// RecipeMergeRequestDiscussion is that merge request with an unresolved
	// discussion on it.
	RecipeMergeRequestDiscussion Recipe = "merge_request_discussion"
	// RecipeMergeRequestSource is a project with a branch ahead of the
	// default one, ready for a merge request that does not exist yet.
	RecipeMergeRequestSource Recipe = "merge_request_source"
	// RecipePipelineJob is a project with a pipeline and its jobs. A case
	// that needs the job to have finished says so with Needs.Runner; a case
	// that needs it unfinished, such as the cancel one, does not.
	RecipePipelineJob Recipe = "pipeline_job"
	// RecipeFailedJob is a project with a pipeline whose job failed and left
	// an artifact behind.
	RecipeFailedJob Recipe = "failed_job_artifact"
	// RecipeEnvironment is a project with an environment and a deployment
	// recorded against it.
	RecipeEnvironment Recipe = "environment_deployment"
	// RecipeRelease is a project with a release.
	RecipeRelease Recipe = "release"
	// RecipeSnippet is a personal snippet.
	RecipeSnippet Recipe = "snippet"
	// RecipeMember is a project with another user added to it.
	RecipeMember Recipe = "member"
	// RecipeRunner is a project with a runner registered for it and a job
	// that runner has run.
	RecipeRunner Recipe = "runner"
	// RecipeCIVariable is a project with a CI variable already in it, for
	// the cases that change one rather than create one.
	RecipeCIVariable Recipe = "ci_variable"
	// RecipeInstanceVariable is a CI variable name no other attempt uses.
	// Instance scope is shared by every attempt on the instance, so unlike a
	// project-scoped name this one cannot be a literal.
	RecipeInstanceVariable Recipe = "instance_ci_variable"
	// RecipePackageFiles is a project and local files to publish to it.
	RecipePackageFiles Recipe = "package_files"
)

// The fact keys the recipes promise. They are spelled as the evaluator this
// replaces spelled them, so a reader comparing the two corpora is comparing
// cases rather than vocabularies.
const (
	// FactProjectPath is the project a case works in, rendered as its full
	// path. A recipe accepts its numeric ID as a second spelling, so an
	// argument that takes either is satisfied by either.
	FactProjectPath = "project_path"
	// FactDefaultBranch is that project's default branch.
	FactDefaultBranch = "default_branch"
	// FactRemoteURL is that project's git remote URL, which the project
	// discovery tool resolves.
	FactRemoteURL = "remote_url"
	// FactGroupPath is the group a case works in, rendered as its path and
	// accepting its numeric ID.
	FactGroupPath = "group_path"
	// FactGroupID is that same group rendered as its numeric ID.
	//
	// It is a fact of its own rather than a second spelling of
	// [FactGroupPath], because what a prompt hands the model is the value the
	// fact renders as, not the set of spellings the scorer would accept: an
	// argument typed as an integer refuses a path, so a case binding one has
	// to give the model the number.
	FactGroupID = "group_id"

	FactArtifactPath          = "artifact_path"
	FactBranchName            = "branch_name"
	FactCIVariableKey         = "ci_variable_key"
	FactDeploymentID          = "deployment_id"
	FactDiscussionID          = "discussion_id"
	FactEnvironmentID         = "environment_id"
	FactEnvironmentName       = "environment_name"
	FactInstanceCIVariableKey = "instance_ci_variable_key"
	FactIssueIID              = "issue_iid"
	FactJobID                 = "job_id"
	FactMergeRequestIID       = "merge_request_iid"
	FactMergeRequestSource    = "mr_source_branch"
	FactPackageDir            = "package_dir"
	FactPackageFilesDisplay   = "package_files_display"
	FactPackageName           = "package_name"
	FactPackageTag            = "package_tag"
	FactPackageVersion        = "package_version"
	FactPipelineID            = "pipeline_id"
	FactReleaseName           = "release_name"
	FactReleaseTagName        = "release_tag_name"
	FactRunnerID              = "runner_id"
	FactSnippetID             = "snippet_id"
)

// withProjectFacts prefixes the three facts every recipe that builds a project
// promises. A project has a path, a default branch and a clone URL whatever
// else the recipe puts in it, and spelling that out per recipe is how one of
// them ends up missing one.
//
// It builds a slice of its own rather than appending to a shared one, so no
// two recipes can ever come to share a backing array.
func withProjectFacts(extra ...string) []string {
	facts := make([]string, 0, 3+len(extra))
	facts = append(facts, FactProjectPath, FactDefaultBranch, FactRemoteURL)
	return append(facts, extra...)
}

// recipeFacts is what each recipe promises to produce. types_test.go holds
// every Truth.Fact and every {{ .Facts.<key> }} in a prompt to the promises of
// that case's own recipe, so a case cannot bind to a value nothing builds.
var recipeFacts = map[Recipe][]string{
	RecipeWorld:                  withProjectFacts(FactGroupPath),
	RecipeProject:                withProjectFacts(),
	RecipeGroup:                  {FactGroupPath, FactGroupID},
	RecipeBranch:                 withProjectFacts(FactBranchName),
	RecipeIssue:                  withProjectFacts(FactIssueIID),
	RecipeMergeRequest:           withProjectFacts(FactMergeRequestIID),
	RecipeMergeRequestDiscussion: withProjectFacts(FactMergeRequestIID, FactDiscussionID),
	RecipeMergeRequestSource:     withProjectFacts(FactMergeRequestSource),
	RecipePipelineJob:            withProjectFacts(FactPipelineID, FactJobID),
	RecipeFailedJob:              withProjectFacts(FactPipelineID, FactJobID, FactArtifactPath),
	RecipeEnvironment:            withProjectFacts(FactEnvironmentID, FactEnvironmentName, FactDeploymentID),
	RecipeRelease:                withProjectFacts(FactReleaseTagName, FactReleaseName),
	RecipeSnippet:                {FactSnippetID},
	// The member recipe adds a person to the project rather than a value to
	// the prompt, so what it promises is the project it added them to.
	RecipeMember:           withProjectFacts(),
	RecipeRunner:           withProjectFacts(FactRunnerID, FactJobID, FactPipelineID),
	RecipeCIVariable:       withProjectFacts(FactCIVariableKey),
	RecipeInstanceVariable: {FactInstanceCIVariableKey},
	RecipePackageFiles: withProjectFacts(
		FactPackageDir, FactPackageFilesDisplay, FactPackageName, FactPackageVersion, FactPackageTag,
	),
}

// Recipes returns every recipe the corpus names, in declaration order, which
// is what the fixture library's own gate enumerates to check that each has a
// builder.
func Recipes() []Recipe {
	return []Recipe{
		RecipeWorld, RecipeProject, RecipeGroup, RecipeBranch, RecipeIssue,
		RecipeMergeRequest, RecipeMergeRequestDiscussion, RecipeMergeRequestSource,
		RecipePipelineJob, RecipeFailedJob, RecipeEnvironment, RecipeRelease,
		RecipeSnippet, RecipeMember, RecipeRunner, RecipeCIVariable,
		RecipeInstanceVariable, RecipePackageFiles,
	}
}

// Facts returns the fact keys a recipe promises, and whether the recipe is one
// this corpus knows at all.
func Facts(recipe Recipe) ([]string, bool) {
	facts, known := recipeFacts[recipe]
	if !known {
		return nil, false
	}
	return append([]string(nil), facts...), true
}
