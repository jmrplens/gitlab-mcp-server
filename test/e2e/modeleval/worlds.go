//go:build e2e

// worlds.go is the join between the corpus and the fixture library: for every
// recipe a case names, the builder that raises that world on an Env and says
// what it built.
//
// It is worlds.go and not facts.go because test/e2e/internal/fixture/facts.go
// already exists and holds facts about a GitLab release rather than about an
// attempt's world; two files of that name under test/e2e with two meanings
// would be read wrongly.
//
// Three things come out of a build, and they are three because they are read
// at three different moments:
//
//   - Facts are read before the model is asked: the prompt renders them and
//     the scorer compares against them, each key carrying every spelling the
//     scorer accepts for it and rendering as the first.
//   - Verify is read after the attempt: it asks GitLab whether the change the
//     case was for actually happened. It is present only where every case
//     naming the recipe leaves the world the same way, which is a judgement
//     made here, per recipe, against the corpus as it stands; the two tables
//     below hold both halves of it, and worlds_test.go refuses a recipe in
//     neither or in both.
//   - Respond is read during the attempt, by the harness's scripted
//     elicitation client, for the recipes whose cases drive an interactive
//     flow.
//
// **What a recipe does when GitLab cannot hold the object.** A handful of the
// licensed worlds name something a Docker instance cannot raise: an
// attestation is published by a CI job that attests, a SCIM identity by an
// identity provider, an enterprise user by a verified domain. Those recipes
// build everything around the object and reserve an identifier nothing holds,
// so the case still dispatches the action it is about and GitLab answers the
// not-found. That is the "dispatched, GitLab refused" class the scoring
// already has, and it is the same evidence the end-to-end suite's own
// scenarios rest on: each of them asserts exactly that refusal. Each such
// recipe says so where it is built.

package modeleval

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// World is the state one attempt runs in.
type World struct {
	// Facts is every value the attempt's stimulus may render and its key may
	// compare, by fact key. The first spelling is what a prompt renders; the
	// rest are the other ways the scorer accepts the same thing.
	Facts map[string][]string
	// Verify asks GitLab whether the change the case was for happened, after
	// the attempt has ended. Nil for a world no case leaves in one state.
	Verify func(*harness.Env) error
	// Respond answers an elicitation request, for the worlds whose cases
	// drive an interactive flow. Nil elsewhere.
	Respond func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
}

// builder raises one recipe's world.
type builder struct {
	// build is the builder itself.
	build func(*harness.Env) World
	// interactive says the world answers elicitation. The runner reads it
	// before it opens an Env, to declare the scripted-session lock, so it
	// cannot be read off a world that has not been built yet.
	interactive bool
}

// BuildWorld raises the world a recipe names, and reports whether the
// recipe is one this package can build at all.
//
// The facts a builder produced are held to the promises the corpus made for
// that recipe, here rather than in a test, because producing a fact needs a
// GitLab and a test that runs on every push has none. A missing key would
// otherwise reach a stimulus as a render error and a surplus one would be a
// value no case could bind, so both fail the test that asked for the world.
func BuildWorld(e *harness.Env, recipe modelcorpus.Recipe) (World, bool) {
	e.T.Helper()

	known, ok := builders[recipe]
	if !ok {
		return World{}, false
	}
	world := known.build(e)
	if problem := factsProblem(recipe, world.Facts); problem != "" {
		e.T.Fatalf("the world built for recipe %q does not match what the corpus promises: %s", recipe, problem)
	}
	if want, got := verifies(recipe), world.Verify != nil; want != got {
		e.T.Fatalf("recipe %q is declared verified=%t and built a world with Verify=%t", recipe, want, got)
	}
	if known.interactive != (world.Respond != nil) {
		e.T.Fatalf("recipe %q is declared interactive=%t and built a world with Respond=%t",
			recipe, known.interactive, world.Respond != nil)
	}
	return world, true
}

// Interactive reports whether a recipe's world answers elicitation.
//
// The runner reads it to decide whether the case's Env declares the scripted
// session lock, which it has to do before the Env exists. It is a property of
// the recipe rather than of the key, so the runner learns that a case is
// interactive without reading its answer.
func Interactive(recipe modelcorpus.Recipe) bool {
	return builders[recipe].interactive
}

// Recipes returns every recipe this package can build, which is what
// worlds_test.go holds the corpus's own enumeration to.
func Recipes() []modelcorpus.Recipe {
	built := make([]modelcorpus.Recipe, 0, len(builders))
	for recipe := range builders {
		built = append(built, recipe)
	}
	return built
}

// factsProblem reports what is wrong with the facts a builder produced,
// against the promises the corpus made, or the empty string when they agree.
//
// It is a function of its own so that the comparison can be driven offline,
// which the worlds it judges cannot be.
func factsProblem(recipe modelcorpus.Recipe, built map[string][]string) string {
	promised, known := modelcorpus.Facts(recipe)
	if !known {
		return "the corpus names no such recipe"
	}
	wanted := map[string]bool{}
	var missing []string
	for _, key := range promised {
		wanted[key] = true
		if len(built[key]) == 0 {
			missing = append(missing, key)
		}
	}
	var surplus []string
	for key := range built {
		if !wanted[key] {
			surplus = append(surplus, key)
		}
	}
	switch {
	case len(missing) > 0 && len(surplus) > 0:
		return "produces nothing for " + strings.Join(sorted(missing), ", ") +
			" and produces " + strings.Join(sorted(surplus), ", ") + ", which it does not promise"
	case len(missing) > 0:
		return "produces nothing for " + strings.Join(sorted(missing), ", ")
	case len(surplus) > 0:
		return "produces " + strings.Join(sorted(surplus), ", ") + ", which it does not promise"
	default:
		return ""
	}
}

// sorted returns the strings in order, for a message that reads the same way
// twice.
func sorted(values []string) []string {
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	return ordered
}

// The fact helpers. A project and a group are named by two spellings each, and
// spelling that out per recipe is how one of them ends up carrying only one.

// projectFacts is what every recipe that builds a project produces.
func projectFacts(project fixture.Project) map[string][]string {
	return map[string][]string{
		modelcorpus.FactProjectPath:   {project.Path, project.IDParam()},
		modelcorpus.FactProjectID:     {project.IDParam(), project.Path},
		modelcorpus.FactDefaultBranch: {project.DefaultBranch},
		modelcorpus.FactRemoteURL:     {project.HTTPURLToRepo},
	}
}

// groupFacts is the same for a group.
func groupFacts(group fixture.Group) map[string][]string {
	return map[string][]string{
		modelcorpus.FactGroupPath: {group.Path, number(group.ID)},
		modelcorpus.FactGroupID:   {number(group.ID), group.Path},
	}
}

// with merges extra facts into a base, and is what a recipe composes its
// world out of. A key written twice is a builder contradicting itself, so the
// last write wins and the run-time comparison in BuildWorld is what catches a
// key that should not be there at all.
func with(base, extra map[string][]string) map[string][]string {
	merged := make(map[string][]string, len(base)+len(extra))
	maps.Copy(merged, base)
	maps.Copy(merged, extra)
	return merged
}

// number spells an identifier the way a prompt shows it and a scorer reads it.
func number(id int64) string { return strconv.FormatInt(id, 10) }

// stillThere turns a fixture library predicate into the verdict a Verify
// wants: the object the case was asked to remove must be gone.
func stillThere(what string, present bool, err error) error {
	if err != nil {
		return err
	}
	if present {
		return fmt.Errorf("%s is still there", what)
	}
	return nil
}

// removed is the same verdict read off a plain GitLab read rather than off a
// predicate: a 404 is the case having done its work, no error is the object
// still being there, and anything else is a world that came apart.
func removed(what string, err error) error {
	if fixture.IsStatus(err, http.StatusNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s back: %w", what, err)
	}
	return fmt.Errorf("%s is still there", what)
}

// The elicitation responder.
//
// One responder serves every interactive world, because most of what an
// elicitation asks for is decided by the flow rather than by the world: the
// flows ask for a title, a description, a name, a confirmation or a choice,
// and the answer to each is the same whichever world the attempt runs in.
// What the world contributes is two things. One is the value that has to be
// unique per attempt, the name a creation flow will give the object, which is
// generated when the world is built and closed over here. The other is every
// answer that has to name something GitLab already holds, which is the world's
// own and nothing a schema can supply: the merge request flow asks for a
// source and a target branch and refuses a request whose source does not
// exist, and the release flow asks for a tag that "must already exist". Both
// were answered from the unique name until this took them from the world, and
// neither case could succeed for any model on any surface.
func elicitationResponder(
	unique string, fromTheWorld map[string]string,
) func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	return func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: elicitedContent(req, unique, fromTheWorld)}, nil
	}
}

// elicitedContent fills every property the requested schema names, from the
// world where the world has an answer for it and from the schema otherwise.
func elicitedContent(req *mcp.ElicitRequest, unique string, fromTheWorld map[string]string) map[string]any {
	content := map[string]any{}
	if req == nil || req.Params == nil {
		return content
	}
	schema, isObject := req.Params.RequestedSchema.(map[string]any)
	if !isObject {
		return content
	}
	properties, hasProperties := schema["properties"].(map[string]any)
	if !hasProperties {
		return content
	}
	for key, raw := range properties {
		property, isProperty := raw.(map[string]any)
		if !isProperty {
			continue
		}
		if answer, known := fromTheWorld[key]; known {
			content[key] = answer
			continue
		}
		content[key] = elicitedValue(key, property, unique)
	}
	return content
}

// elicitedValue picks one value for one elicited property.
//
// A confirmation is accepted and a choice takes the first option the server
// offered, which is the one thing a responder can do without an opinion about
// the flow. Everything else is text, and the fields that name the object being
// created take the attempt's own unique name: two attempts of one model share
// a session, so a constant there would have the second refused for a name the
// first took.
//
// A tag name is deliberately not one of them, though it reads like one: the
// only flow that asks for one is the release flow, which asks for a tag that
// "must already exist", so it names something the world holds rather than
// something the flow creates, and the world is what answers it.
func elicitedValue(key string, property map[string]any, unique string) any {
	switch key {
	case "confirmed":
		return true
	case "selection":
		if options, ok := property["enum"].([]any); ok && len(options) > 0 {
			return options[0]
		}
		return "default"
	case "name", "path", "title":
		return unique
	case "description":
		return "Created by the model evaluation's scripted elicitation responder"
	default:
		return "e2e-" + key
	}
}

// missingID is an identifier nothing on a fresh instance holds. It is what a
// world reserves when the object its case is about cannot be raised on a
// Docker instance at all, so that the case still dispatches its action and
// GitLab answers the not-found.
const missingID = "999999999"

// builders is the whole join: one entry per recipe the corpus names.
//
// It is assembled from three tables rather than written as one literal,
// because the corpus declares its recipes in three blocks and a reader with
// modelcorpus/recipes.go open beside this file should be reading the same
// list twice rather than two lists.
var builders = mergedBuilders(freeWorlds(), destructiveWorlds(), licensedWorlds())

// mergedBuilders joins the three tables into the one the package reads. A
// recipe declared twice is a contradiction rather than a merge, and it is
// refused at package initialization rather than resolved by order.
func mergedBuilders(tables ...map[modelcorpus.Recipe]builder) map[modelcorpus.Recipe]builder {
	merged := map[modelcorpus.Recipe]builder{}
	for _, table := range tables {
		for recipe, known := range table {
			if _, twice := merged[recipe]; twice {
				panic("modeleval: recipe " + string(recipe) + " has two builders")
			}
			merged[recipe] = known
		}
	}
	return merged
}

// freeWorlds are the worlds the Free corpus runs in.
func freeWorlds() map[modelcorpus.Recipe]builder {
	return map[modelcorpus.Recipe]builder{
		modelcorpus.RecipeWorld: {interactive: true, build: func(e *harness.Env) World {
			shared := fixture.SharedWorld(e)
			return World{
				Facts: with(projectFacts(shared.Project), groupFacts(shared.Group)),
				// Its interactive case creates a project, so every answer the
				// flow wants is the name of something that does not exist yet.
				Respond: elicitationResponder(e.Name("world"), nil),
			}
		}},

		modelcorpus.RecipeProject: {interactive: true, build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("eval"))
			// The CI configuration is what lets a case create a pipeline in the
			// project it was given, which a project with no configuration
			// answers with a refusal about the configuration rather than about
			// the call.
			ciConfiguration(e, project)
			// The tag is for the guided release flow, which asks for one that
			// "must already exist" and is refused by GitLab when it does not.
			// It is not a fact: no case names it, the flow asks the responder
			// for it, and a fact nothing promises is a world the entry point
			// refuses.
			tag := fixture.NewTag(e, project)
			return World{
				Facts:   projectFacts(project),
				Respond: elicitationResponder(e.Name("project"), map[string]string{"tag_name": tag.Name}),
			}
		}},

		modelcorpus.RecipeGroup: {build: func(e *harness.Env) World {
			return World{Facts: groupFacts(fixture.NewGroup(e, fixture.WithGroupNamePrefix("eval")))}
		}},

		modelcorpus.RecipeBranch: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("branch"))
			branch := fixture.NewBranch(e, project, e.Name("feature"))
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactBranchName: {branch.Name},
			})}
		}},

		modelcorpus.RecipeIssue: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("issue"))
			issue := fixture.NewIssue(e, project, e.Name("issue"))
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactIssueIID: {number(issue.IID)},
			})}
		}},

		modelcorpus.RecipeMergeRequest: {build: func(e *harness.Env) World {
			project, mergeRequest := newMergeRequestWorld(e, "mr")
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactMergeRequestIID: {number(mergeRequest.IID)},
			})}
		}},

		modelcorpus.RecipeMergeRequestDiscussion: {build: func(e *harness.Env) World {
			project, mergeRequest := newMergeRequestWorld(e, "mrdisc")
			discussion := fixture.NewMergeRequestDiscussion(e, project, mergeRequest.IID)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactMergeRequestIID: {number(mergeRequest.IID)},
					modelcorpus.FactDiscussionID:    {discussion.ID},
				}),
				Verify: func(e *harness.Env) error {
					return discussionResolved(e, project, mergeRequest.IID, discussion.ID)
				},
			}
		}},

		modelcorpus.RecipeMergeRequestSource: {interactive: true, build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("mrsource"))
			branch := fixture.NewBranch(e, project, e.Name("source"))
			fixture.CommitFile(e, project, branch.Name, "merge-request.txt",
				"the change a merge request is opened for\n", "add the merge request fixture")
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactMergeRequestSource: {branch.Name},
				}),
				// The guided merge request flow asks for both branches by
				// name, and GitLab refuses a request whose source branch does
				// not exist. The world built the branch for exactly this case,
				// so the responder hands it over rather than inventing one.
				Respond: elicitationResponder(e.Name("mrsource"), map[string]string{
					"source_branch": branch.Name,
					"target_branch": project.DefaultBranch,
				}),
			}
		}},

		modelcorpus.RecipePipelineJob: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("pipeline"))
			// The configuration with a manual job in it, because one of this
			// world's cases plays one. The plain configuration declares a
			// single job that runs by itself, and GitLab answers a play of it
			// with "Unplayable Job" whatever the model sends.
			manualJobConfiguration(e, project)
			// A case that cancels or deletes a pipeline declares no runner and
			// is about a pipeline that has not finished, so the world is built
			// without waiting wherever it can be: waiting would finish the very
			// pipeline the case is asked to stop.
			pipeline := fixture.NewPipelineNoWait(e, project, project.DefaultBranch)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactPipelineID: {number(pipeline.ID)},
				modelcorpus.FactJobID:      {number(fixture.ManualPipelineJobID(e, project, pipeline.ID))},
			})}
		}},

		modelcorpus.RecipeFailedJob: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("failedjob"))
			failed := fixture.NewFailedJob(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactPipelineID:   {number(failed.PipelineID)},
				modelcorpus.FactJobID:        {number(failed.JobID)},
				modelcorpus.FactArtifactPath: {failed.ArtifactPath},
			})}
		}},

		modelcorpus.RecipeEnvironment: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("environment"))
			environment := fixture.NewEnvironment(e, project, "eval")
			commit := fixture.CommitFile(e, project, project.DefaultBranch,
				e.Name("deploy")+".txt", "deployment fixture\n", "add the deployment fixture")
			deployment := fixture.NewDeployment(e, project, environment, commit.SHA)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactEnvironmentID:   {number(environment.ID)},
				modelcorpus.FactEnvironmentName: {environment.Name},
				modelcorpus.FactDeploymentID:    {number(deployment.ID)},
			})}
		}},

		modelcorpus.RecipeRelease: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("release"))
			release := fixture.NewRelease(e, project, "eval")
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactReleaseTagName: {release.TagName},
				modelcorpus.FactReleaseName:    {release.Name},
			})}
		}},

		modelcorpus.RecipeSnippet: {build: func(e *harness.Env) World {
			snippet := fixture.NewSnippet(e)
			return World{Facts: map[string][]string{modelcorpus.FactSnippetID: {number(snippet.ID)}}}
		}},

		modelcorpus.RecipeMember: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("member"))
			member := fixture.NewUser(e, "member")
			fixture.AddProjectMember(e, project, member, gl.DeveloperPermissions)
			return World{Facts: projectFacts(project)}
		}},

		modelcorpus.RecipeRunner: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("runner"))
			ciConfiguration(e, project)
			pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
			runner := fixture.NewProjectRunner(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactRunnerID:   {number(runner.ID)},
				modelcorpus.FactJobID:      {number(fixture.FirstPipelineJobID(e, project, pipeline.ID))},
				modelcorpus.FactPipelineID: {number(pipeline.ID)},
			})}
		}},

		modelcorpus.RecipeCIVariable: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("civar"))
			variable := fixture.NewProjectCIVariable(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactCIVariableKey: {variable.Key},
			})}
		}},

		modelcorpus.RecipeInstanceVariable: {build: func(e *harness.Env) World {
			return World{Facts: map[string][]string{
				modelcorpus.FactInstanceCIVariableKey: {fixture.ReserveInstanceCIVariableKey(e)},
			}}
		}},

		modelcorpus.RecipePackageFiles: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("packagefiles"))
			files := fixture.NewPackageFiles(e)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactPackageDir:          {files.Dir},
				modelcorpus.FactPackageFilesDisplay: {files.Display()},
				modelcorpus.FactPackageName:         {files.Name},
				modelcorpus.FactPackageVersion:      {files.Version},
				modelcorpus.FactPackageTag:          {files.Tag},
			})}
		}},
	}
}

// destructiveWorlds are the worlds the destructive partition runs in: almost
// every one of them is a project of the attempt's own with one thing already
// in it, because a case that removes something needs that something to exist
// and needs it to belong to nobody else.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: see freeWorlds.
func destructiveWorlds() map[modelcorpus.Recipe]builder {
	return map[modelcorpus.Recipe]builder{
		modelcorpus.RecipeMergeableMergeRequest: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("mergeable"))
			branch := fixture.NewBranch(e, project, e.Name("merge"))
			fixture.CommitFile(e, project, branch.Name, "mergeable.txt",
				"the change a merge request carries\n", "add the mergeable fixture")
			mergeRequest := fixture.NewMergeableMergeRequest(e, project, branch.Name, project.DefaultBranch, "mergeable fixture")
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactMergeRequestIID: {number(mergeRequest.IID)},
			})}
		}},

		modelcorpus.RecipeFile: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("file"))
			branch := fixture.NewBranch(e, project, e.Name("filebranch"))
			path := e.Name("file") + ".txt"
			fixture.CommitFile(e, project, branch.Name, path, "the file a case removes\n", "add the file fixture")
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactBranchName: {branch.Name},
					modelcorpus.FactFilePath:   {path},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().RepositoryFiles.GetFileMetaData(project.ID, path,
						&gl.GetFileMetaDataOptions{Ref: new(branch.Name)}, gl.WithContext(e.Ctx))
					return removed("file "+path, err)
				},
			}
		}},

		modelcorpus.RecipeMilestone: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("milestone"))
			milestone := fixture.NewMilestone(e, project, "eval")
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactMilestoneIID: {number(milestone.IID), number(milestone.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().Milestones.GetMilestone(project.ID, milestone.ID, gl.WithContext(e.Ctx))
					return removed("milestone "+number(milestone.IID), err)
				},
			}
		}},

		modelcorpus.RecipeProjectAccessToken: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("projecttoken"))
			token := fixture.NewProjectAccessToken(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactProjectAccessTokenID: {number(token.ID)},
				}),
				Verify: func(e *harness.Env) error {
					return tokenRevoked(e, project, token.ID)
				},
			}
		}},

		modelcorpus.RecipePackage: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("package"))
			published := fixture.NewPackage(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactPackageID: {number(published.ID)},
				}),
				Verify: func(e *harness.Env) error {
					// GitLab offers no read of a single package through
					// client-go, so the registry listing is what answers.
					packages, _, err := e.Client().GL().Packages.ListProjectPackages(project.ID,
						&gl.ListProjectPackagesOptions{}, gl.WithContext(e.Ctx))
					if err != nil {
						return fmt.Errorf("listing the packages of project %d back: %w", project.ID, err)
					}
					for _, one := range packages {
						if one != nil && one.ID == published.ID {
							return fmt.Errorf("package %d is still in the registry", published.ID)
						}
					}
					return nil
				},
			}
		}},

		modelcorpus.RecipeBroadcastMessage: {build: func(e *harness.Env) World {
			message := fixture.NewBroadcastMessage(e)
			return World{
				Facts: map[string][]string{modelcorpus.FactBroadcastMessageID: {number(message.ID)}},
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().BroadcastMessage.GetBroadcastMessage(message.ID, gl.WithContext(e.Ctx))
					return removed("broadcast message "+number(message.ID), err)
				},
			}
		}},

		modelcorpus.RecipeProjectHook: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("hook"))
			hook := fixture.NewProjectHook(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactHookID: {number(hook.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().Projects.GetProjectHook(project.ID, hook.ID, gl.WithContext(e.Ctx))
					return removed("project hook "+number(hook.ID), err)
				},
			}
		}},

		modelcorpus.RecipeProjectBadge: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("badge"))
			badge := fixture.NewProjectBadge(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactBadgeID: {number(badge.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().ProjectBadges.GetProjectBadge(project.ID, badge.ID, gl.WithContext(e.Ctx))
					return removed("project badge "+number(badge.ID), err)
				},
			}
		}},

		modelcorpus.RecipeDraftNote: {build: func(e *harness.Env) World {
			project, mergeRequest := newMergeRequestWorld(e, "draftnote")
			draft := fixture.NewDraftNote(e, project, mergeRequest.IID)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactMergeRequestIID: {number(mergeRequest.IID)},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.DraftNoteExists(e.Ctx, e.Client(), project.ID, mergeRequest.IID, draft.ID)
					return stillThere("draft note "+number(draft.ID), present, err)
				},
			}
		}},

		modelcorpus.RecipeJobTokenScope: {build: func(e *harness.Env) World {
			source := fixture.NewProject(e, fixture.WithNamePrefix("jobtokensource"))
			target := fixture.NewProject(e, fixture.WithNamePrefix("jobtokentarget"))
			scope := fixture.NewJobTokenScope(e, source, target)
			return World{
				Facts: with(projectFacts(source), map[string][]string{
					modelcorpus.FactJobTokenTargetProjectID: {number(scope.TargetProjectID)},
				}),
				Verify: func(e *harness.Env) error {
					return jobTokenScopeCleared(e, source, target)
				},
			}
		}},

		modelcorpus.RecipeInstanceVariableSeeded: {build: func(e *harness.Env) World {
			variable := fixture.NewInstanceCIVariable(e)
			return World{
				Facts: map[string][]string{modelcorpus.FactInstanceCIVariableKey: {variable.Key}},
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().InstanceVariables.GetVariable(variable.Key, gl.WithContext(e.Ctx))
					return removed("instance CI variable "+variable.Key, err)
				},
			}
		}},

		modelcorpus.RecipeTag: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("tag"))
			tag := fixture.NewTag(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactTagName: {tag.Name},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.TagExists(e.Ctx, e.Client(), project.ID, tag.Name)
					return stillThere("tag "+tag.Name, present, err)
				},
			}
		}},

		modelcorpus.RecipePipelineTrigger: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("trigger"))
			trigger := fixture.NewPipelineTrigger(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactPipelineTriggerID: {number(trigger.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().PipelineTriggers.GetPipelineTrigger(project.ID, trigger.ID, gl.WithContext(e.Ctx))
					return removed("pipeline trigger "+number(trigger.ID), err)
				},
			}
		}},

		modelcorpus.RecipePipelineSchedule: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("schedule"))
			schedule := fixture.NewPipelineSchedule(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactPipelineScheduleID: {number(schedule.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().PipelineSchedules.GetPipelineSchedule(project.ID, schedule.ID, gl.WithContext(e.Ctx))
					return removed("pipeline schedule "+number(schedule.ID), err)
				},
			}
		}},

		modelcorpus.RecipeUser: {build: func(e *harness.Env) World {
			// A user of the attempt's own, which is what the skip reason of the
			// two-factor case was about: it ran against the shared evaluator
			// user, where blocking or disabling two-factor authentication
			// changes what every other attempt is running as.
			user := fixture.NewUser(e, "subject")
			return World{Facts: map[string][]string{modelcorpus.FactUserID: {number(user.ID)}}}
		}},

		modelcorpus.RecipeMemberCandidate: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("candidate"))
			candidate := fixture.NewUser(e, "candidate")
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactUserID: {number(candidate.ID)},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.ProjectHasMember(e.Ctx, e.Client(), project.ID, candidate.ID)
					return stillThere("user "+number(candidate.ID)+" as a member", present, err)
				},
			}
		}},

		modelcorpus.RecipeFeatureFlag: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("flag"))
			flag := fixture.NewProjectFeatureFlag(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactFeatureFlagName: {flag.Name},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().ProjectFeatureFlags.GetProjectFeatureFlag(project.ID, flag.Name, gl.WithContext(e.Ctx))
					return removed("feature flag "+flag.Name, err)
				},
			}
		}},

		modelcorpus.RecipeCustomEmoji: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("emoji"))
			emoji := fixture.NewCustomEmoji(e, group)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactCustomEmojiID: {emoji.ID},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.CustomEmojiExists(e.Ctx, e.Client(), group.Path, emoji.ID)
					return stillThere("custom emoji "+emoji.ID, present, err)
				},
			}
		}},

		modelcorpus.RecipeWikiPage: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("wiki"))
			page := fixture.NewWikiPage(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactWikiSlug: {page.Slug},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().Wikis.GetWikiPage(project.ID, page.Slug, &gl.GetWikiPageOptions{}, gl.WithContext(e.Ctx))
					return removed("wiki page "+page.Slug, err)
				},
			}
		}},

		modelcorpus.RecipeMergeRequestAward: {build: func(e *harness.Env) World {
			project, mergeRequest := newMergeRequestWorld(e, "mraward")
			award := fixture.NewMergeRequestAward(e, project, mergeRequest.IID)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactMergeRequestIID: {number(mergeRequest.IID)},
					modelcorpus.FactAwardID:         {number(award.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().AwardEmoji.GetMergeRequestAwardEmoji(project.ID, mergeRequest.IID, award.ID, gl.WithContext(e.Ctx))
					return removed("merge request award "+number(award.ID), err)
				},
			}
		}},

		modelcorpus.RecipeIssueAward: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("issueaward"))
			issue := fixture.NewIssue(e, project, e.Name("awarded"))
			award := fixture.NewIssueAward(e, project, issue.IID)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactIssueIID: {number(issue.IID)},
					modelcorpus.FactAwardID:  {number(award.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().AwardEmoji.GetIssueAwardEmoji(project.ID, issue.IID, award.ID, gl.WithContext(e.Ctx))
					return removed("issue award "+number(award.ID), err)
				},
			}
		}},

		modelcorpus.RecipeDeployKey: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("deploykey"))
			key := fixture.NewDeployKey(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactDeployKeyID: {number(key.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().DeployKeys.GetDeployKey(project.ID, key.ID, gl.WithContext(e.Ctx))
					return removed("deploy key "+number(key.ID), err)
				},
			}
		}},

		modelcorpus.RecipeDeployToken: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("deploytoken"))
			token := fixture.NewProjectDeployToken(e, project)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactDeployTokenID: {number(token.ID)},
				}),
				Verify: func(e *harness.Env) error {
					_, _, err := e.Client().GL().DeployTokens.GetProjectDeployToken(project.ID, token.ID, gl.WithContext(e.Ctx))
					return removed("deploy token "+number(token.ID), err)
				},
			}
		}},

		modelcorpus.RecipeCommitDiscussion: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("commitdisc"))
			commit := fixture.CommitFile(e, project, project.DefaultBranch,
				e.Name("commented")+".txt", "the commit a discussion hangs off\n", "add the commit discussion fixture")
			discussion := fixture.NewCommitDiscussion(e, project, commit.SHA)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactCommitSHA:          {commit.SHA, commit.ShortID},
					modelcorpus.FactCommitDiscussionID: {discussion.ID},
					modelcorpus.FactCommitNoteID:       {number(discussion.NoteID)},
				}),
				Verify: func(e *harness.Env) error {
					return commitNoteRemoved(e, project, commit.SHA, discussion.ID, discussion.NoteID)
				},
			}
		}},

		modelcorpus.RecipeTerraformState: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("terraform"))
			name := e.Name("state")
			fixture.PushTerraformState(e, project, name, 1)
			fixture.LockTerraformState(e, project, name)
			return World{
				Facts: with(projectFacts(project), map[string][]string{
					modelcorpus.FactTerraformStateName: {name},
				}),
				Verify: func(e *harness.Env) error {
					locked, err := fixture.TerraformStateIsLocked(e.Ctx, e.Client(), project.Path, name)
					return stillThere("the lock on Terraform state "+name, locked, err)
				},
			}
		}},

		modelcorpus.RecipeDatabaseMigration: {build: func(e *harness.Env) World {
			return World{Facts: map[string][]string{
				modelcorpus.FactDatabaseMigration: {fixture.DBMigrationVersion(e)},
			}}
		}},

		modelcorpus.RecipeProjectMirror: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("mirror"))
			mirror := fixture.NewProjectMirror(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactMirrorID: {number(mirror.ID)},
			})}
		}},
	}
}

// licensedWorlds are the worlds the licensed partitions run in.
//
// A licensed case says what it needs of the instance through its own tier; a
// recipe here says what it needs of the world, and the two are separate
// questions. A group holding an epic is a world any tier could describe; only
// GitLab's license decides whether the epic can exist.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: see freeWorlds.
func licensedWorlds() map[modelcorpus.Recipe]builder {
	return map[modelcorpus.Recipe]builder{
		modelcorpus.RecipeEpic: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("epic"))
			epic := fixture.NewEpic(e, group, e.Name("epic"))
			note := fixture.NewEpicNote(e, epic)
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactEpicIID:          {number(epic.IID)},
				modelcorpus.FactEpicNoteID:       {number(note.ID)},
				modelcorpus.FactEpicDiscussionID: {note.DiscussionID},
			})}
		}},

		modelcorpus.RecipeEpicIssue: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("epicissue"))
			project := fixture.NewProject(e, fixture.WithNamePrefix("epicissue"), fixture.InGroup(group))
			epic := fixture.NewEpic(e, group, e.Name("parent"))
			issue := fixture.NewIssue(e, project, e.Name("child"))
			fixture.AssignIssueToEpic(e, epic, project, issue)
			return World{Facts: with(with(projectFacts(project), groupFacts(group)), map[string][]string{
				modelcorpus.FactEpicIID:  {number(epic.IID)},
				modelcorpus.FactIssueIID: {number(issue.IID)},
			})}
		}},

		modelcorpus.RecipePushRule: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("pushrule"))
			fixture.NewProjectPushRule(e, project)
			return World{Facts: projectFacts(project)}
		}},

		modelcorpus.RecipeProjectServiceAccount: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("projectsvc"))
			account := fixture.NewProjectServiceAccount(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactServiceAccountID:      {number(account.ID)},
				modelcorpus.FactServiceAccountTokenID: {number(account.TokenID)},
			})}
		}},

		modelcorpus.RecipeGroupServiceAccount: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("groupsvc"))
			account := fixture.NewGroupServiceAccount(e, group)
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactServiceAccountID:      {number(account.ID)},
				modelcorpus.FactServiceAccountTokenID: {number(account.TokenID)},
			})}
		}},

		modelcorpus.RecipeServiceAccountName: {build: func(e *harness.Env) World {
			return World{Facts: map[string][]string{
				modelcorpus.FactServiceAccountUsername: {fixture.ReserveServiceAccountUsername(e)},
			}}
		}},

		modelcorpus.RecipeGroupProtectedBranch: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("groupbranch"))
			rule := fixture.NewGroupProtectedBranch(e, group)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactProtectedBranchName: {rule.Name},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.GroupBranchIsProtected(e.Ctx, e.Client(), group.ID, rule.Name)
					return stillThere("the protection of branch "+rule.Name, present, err)
				},
			}
		}},

		modelcorpus.RecipeGroupProtectedEnvironment: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("groupenv"))
			protection := fixture.NewGroupProtectedEnvironment(e, group)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactProtectedEnvironment: {protection.Name},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.GroupEnvironmentIsProtected(e.Ctx, e.Client(), group.ID, protection.Name)
					return stillThere("the protection of environment "+protection.Name, present, err)
				},
			}
		}},

		modelcorpus.RecipeGeoSite: {build: func(e *harness.Env) World {
			site := fixture.NewGeoSite(e)
			return World{Facts: map[string][]string{modelcorpus.FactGeoSiteID: {number(site.ID)}}}
		}},

		modelcorpus.RecipeGeoSiteName: {build: func(e *harness.Env) World {
			reserved := fixture.ReserveGeoSiteName(e)
			return World{Facts: map[string][]string{
				modelcorpus.FactGeoSiteName: {reserved.Name},
				modelcorpus.FactGeoSiteURL:  {reserved.URL},
			}}
		}},

		// A SCIM identity is provisioned by an identity provider through the SCIM
		// endpoint, and GitLab offers no way to create one: the API reads,
		// updates and deletes. A Docker instance has no provider, so the world is
		// the group and a uid nothing provisioned, and the case is answered by
		// GitLab's own not-found. The end-to-end suite asserts exactly that
		// refusal, which is the evidence this rests on.
		modelcorpus.RecipeScimIdentity: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("scim"))
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactScimUID: {e.Name("scimuid")},
			})}
		}},

		modelcorpus.RecipeLDAPLink: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("ldap"))
			link := fixture.NewGroupLDAPLink(e, group)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactLDAPProvider: {link.Provider},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.GroupHasLDAPLink(e.Ctx, e.Client(), group.ID, link.Provider)
					return stillThere("the LDAP link for "+link.Provider, present, err)
				},
			}
		}},

		// A SAML group link needs the group to have single sign-on configured,
		// which needs an identity provider the Docker stack has none of: GitLab
		// refuses every SAML link action on such a group. The world is the group
		// and a link name nothing holds, and the case is answered by that
		// refusal, which the end-to-end suite asserts on its own.
		modelcorpus.RecipeSAMLLink: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("saml"))
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactSAMLGroupName: {e.Name("samlgroup")},
			})}
		}},

		modelcorpus.RecipeGroupSSHCertificate: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("sshcert"))
			certificate := fixture.NewGroupSSHCertificate(e, group)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactSSHCertificateID: {number(certificate.ID)},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.GroupHasSSHCertificate(e.Ctx, e.Client(), group.ID, certificate.ID)
					return stillThere("SSH certificate "+number(certificate.ID), present, err)
				},
			}
		}},

		modelcorpus.RecipeGroupWikiPage: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("groupwiki"))
			page := fixture.NewGroupWikiPage(e, group)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactWikiSlug: {page.Slug},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.GroupWikiPageExists(e.Ctx, e.Client(), group.ID, page.Slug)
					return stillThere("group wiki page "+page.Slug, present, err)
				},
			}
		}},

		// The role is an instance role rather than a group one: a self-managed
		// Ultimate instance refuses the group-level create as deprecated and
		// points at the instance level. The group is what the case's own prompt
		// names, and the builder raises both.
		modelcorpus.RecipeMemberRole: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("memberrole"))
			role := fixture.NewMemberRole(e)
			return World{
				Facts: with(groupFacts(group), map[string][]string{
					modelcorpus.FactMemberRoleID: {number(role.ID)},
				}),
				Verify: func(e *harness.Env) error {
					present, err := fixture.InstanceHasMemberRole(e.Ctx, e.Client(), role.ID)
					return stillThere("instance member role "+number(role.ID), present, err)
				},
			}
		}},

		modelcorpus.RecipeProjectAlias: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("alias"))
			alias := fixture.NewProjectAlias(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactProjectAliasName: {alias.Name},
			})}
		}},

		modelcorpus.RecipeProjectAliasName: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("aliasname"))
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactProjectAliasName: {fixture.ReserveProjectAliasName(e)},
			})}
		}},

		modelcorpus.RecipeExternalStatusCheck: {build: func(e *harness.Env) World {
			project, mergeRequest := newMergeRequestWorld(e, "statuscheck")
			check := fixture.NewExternalStatusCheck(e, project)
			sha := mergeRequestSHA(e, project, mergeRequest.IID)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactMergeRequestIID:       {number(mergeRequest.IID)},
				modelcorpus.FactExternalStatusCheckID: {number(check.ID)},
				modelcorpus.FactCommitSHA:             {sha},
			})}
		}},

		// A merge request with no pipeline of its own cannot board a train, and a
		// project whose pipelines nothing runs cannot give it one, so the world is
		// the train-enabled project and a request that has not boarded. GitLab
		// refuses the add and answers the entry read with a not-found, which is
		// what the end-to-end suite asserts, on a fixture of this same shape and
		// for this same reason. The project is in a group because GitLab keeps
		// the train switches only on a project whose namespace carries the
		// licensed feature.
		modelcorpus.RecipeMergeTrainEntry: {build: func(e *harness.Env) World {
			train := fixture.NewMergeTrain(e)
			return World{Facts: with(projectFacts(train.Project), map[string][]string{
				modelcorpus.FactMergeRequestIID: {number(train.MergeRequest.IID)},
			})}
		}},

		// A repository storage move needs a second Gitaly storage to move to,
		// and the Docker stack configures a single one, which is what the
		// end-to-end suite's own group storage move scenario rests on. The
		// world is the group and a move identifier nobody scheduled.
		modelcorpus.RecipeStorageMove: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("storagemove"))
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactStorageMoveID: {missingID},
			})}
		}},

		// A dependency list export is made from a pipeline that ran a dependency
		// scan, and it is the case itself that creates one. The world is the
		// scanned project and an export identifier nothing created.
		modelcorpus.RecipeDependencyExport: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("depexport"))
			ciConfiguration(e, project)
			pipeline := fixture.NewPipelineNoWait(e, project, project.DefaultBranch)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactPipelineID:         {number(pipeline.ID)},
				modelcorpus.FactDependencyExportID: {missingID},
			})}
		}},

		// An audit event is written by GitLab as a side effect of something
		// else, so the world causes one rather than creating it: it edits the
		// project and waits for the record to appear, first in the project's
		// own log and then in the instance's. Reserving an identifier nothing
		// holds would have been wrong here, because the end-to-end suite's own
		// audit event scenario asserts the read succeeds rather than that it
		// is refused, and it causes its events the same way.
		modelcorpus.RecipeInstanceAuditEvent: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("auditevent"))
			event := fixture.NewInstanceAuditEvent(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactAuditEventID: {number(event.ID)},
			})}
		}},

		// A build attestation is published by a CI job that attests, through a
		// runner configured for it. The world is a project that has published
		// none and an attestation number nothing holds, which is the state the
		// end-to-end suite asserts the listing and the download against.
		modelcorpus.RecipeAttestation: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("attestation"))
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactAttestationIID: {missingID},
			})}
		}},

		modelcorpus.RecipeVulnerability: {build: func(e *harness.Env) World {
			scanned := fixture.NewVulnerableProject(e)
			// The two numbers of one pipeline are two facts, and each case
			// renders the one its action takes: the security finding and
			// pipeline summary actions take the project's number, the
			// dependency export the instance's. They were one fact carrying
			// both spellings, which rendered the project's number to the
			// export case and let GitLab act on whichever pipeline on the
			// instance happens to carry it.
			return World{Facts: with(projectFacts(scanned.Project), map[string][]string{
				modelcorpus.FactPipelineIID:     {number(scanned.PipelineIID)},
				modelcorpus.FactPipelineID:      {number(scanned.Pipeline.ID)},
				modelcorpus.FactVulnerabilityID: {scanned.Vulnerabilities[0].ID},
			})}
		}},

		// An enterprise user belongs to a group with a verified domain, which
		// needs DNS the Docker stack cannot have. The world is the group and a
		// real user it does not manage, which is the subject the end-to-end suite
		// asserts the same refusals against.
		modelcorpus.RecipeEnterpriseUser: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("entuser"))
			user := fixture.NewUser(e, "enterprise")
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactUserID: {number(user.ID)},
			})}
		}},

		modelcorpus.RecipeGroupAccessToken: {build: func(e *harness.Env) World {
			group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("grouptoken"))
			token := fixture.NewGroupToken(e, group, gl.MaintainerPermissions)
			return World{Facts: with(groupFacts(group), map[string][]string{
				modelcorpus.FactGroupAccessTokenID: {number(token.ID)},
			})}
		}},

		// A model version is published through the MLflow-compatible API, which
		// client-go does not model: its model registry service only downloads.
		// The world is a project and a version, path and file name nothing
		// published.
		modelcorpus.RecipeModelVersion: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("modelversion"))
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactModelVersionID: {missingID},
				modelcorpus.FactModelFilePath:  {"models"},
				modelcorpus.FactModelFileName:  {"model.onnx"},
			})}
		}},

		modelcorpus.RecipeDeploymentApproval: {build: func(e *harness.Env) World {
			project := fixture.NewProject(e, fixture.WithNamePrefix("deployapproval"))
			approval := fixture.NewDeploymentApproval(e, project)
			return World{Facts: with(projectFacts(project), map[string][]string{
				modelcorpus.FactProtectedEnvironment: {approval.Protected.Name},
				modelcorpus.FactEnvironmentName:      {approval.Environment.Name},
				modelcorpus.FactDeploymentID:         {number(approval.Deployment.ID)},
			})}
		}},
	}
}

// The shared pieces of a world, and the read-backs a Verify is made of.

// ciConfiguration commits the pipeline configuration without starting a
// pipeline on the push.
//
// The worlds that want a pipeline create one themselves, and the push the
// configuration arrives on would start another that nobody reads. On a Docker
// instance that is one runner, and thirty-seven attempts whose world is a
// plain project would each queue a job behind the attempts that are about a
// pipeline.
func ciConfiguration(e *harness.Env, project fixture.Project) {
	e.T.Helper()
	commitCIConfiguration(e, project, fixture.CIYAML)
}

// manualJobConfiguration is the same for the world whose case plays a job: a
// configuration that declares a manual one, which the plain one does not.
func manualJobConfiguration(e *harness.Env, project fixture.Project) {
	e.T.Helper()
	commitCIConfiguration(e, project, fixture.ManualJobCIYAML)
}

// commitCIConfiguration commits one pipeline configuration, telling GitLab not
// to start a pipeline on the push for the reason above.
func commitCIConfiguration(e *harness.Env, project fixture.Project, yaml string) {
	e.T.Helper()
	fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, yaml,
		"ci: add the e2e pipeline configuration [skip ci]")
}

// newMergeRequestWorld raises the project, the branch ahead of its default
// one and the open merge request that five recipes all start from.
func newMergeRequestWorld(e *harness.Env, prefix string) (fixture.Project, fixture.MergeRequest) {
	e.T.Helper()

	project := fixture.NewProject(e, fixture.WithNamePrefix(prefix))
	branch := fixture.NewBranch(e, project, e.Name(prefix))
	fixture.CommitFile(e, project, branch.Name, prefix+".txt",
		"the change this merge request carries\n", "add the merge request fixture")
	return project, fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, prefix+" fixture")
}

// mergeRequestSHA reads the head commit of a merge request, which is what an
// external status check is reported against.
func mergeRequestSHA(e *harness.Env, project fixture.Project, iid int64) string {
	e.T.Helper()

	mergeRequest, _, err := e.Client().GL().MergeRequests.GetMergeRequest(project.ID, iid,
		&gl.GetMergeRequestsOptions{}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading merge request !%d of project %d for its head commit: %v", iid, project.ID, err)
	}
	if mergeRequest.SHA == "" {
		e.T.Fatalf("merge request !%d of project %d has no head commit", iid, project.ID)
	}
	return mergeRequest.SHA
}

// discussionResolved reports whether the case resolved the thread it was
// given, which is what its world is verified against.
func discussionResolved(e *harness.Env, project fixture.Project, iid int64, discussionID string) error {
	discussion, _, err := e.Client().GL().Discussions.GetMergeRequestDiscussion(project.ID, iid, discussionID, gl.WithContext(e.Ctx))
	if err != nil {
		return fmt.Errorf("reading discussion %s of merge request !%d back: %w", discussionID, iid, err)
	}
	for _, note := range discussion.Notes {
		if note != nil && note.Resolvable && !note.Resolved {
			return fmt.Errorf("discussion %s of merge request !%d is still unresolved", discussionID, iid)
		}
	}
	return nil
}

// tokenRevoked reports whether the case revoked the project access token it
// was given. GitLab keeps a revoked token readable and marks it, so the state
// is what answers rather than a not-found.
func tokenRevoked(e *harness.Env, project fixture.Project, tokenID int64) error {
	token, _, err := e.Client().GL().ProjectAccessTokens.GetProjectAccessToken(project.ID, tokenID, gl.WithContext(e.Ctx))
	if fixture.IsStatus(err, http.StatusNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading project access token %d back: %w", tokenID, err)
	}
	if token.Active && !token.Revoked {
		return fmt.Errorf("project access token %d is still active", tokenID)
	}
	return nil
}

// jobTokenScopeCleared reports whether the case took the target project off
// the source project's inbound job token allowlist.
func jobTokenScopeCleared(e *harness.Env, source, target fixture.Project) error {
	allowed, _, err := e.Client().GL().JobTokenScope.GetProjectJobTokenInboundAllowList(source.ID,
		&gl.GetJobTokenInboundAllowListOptions{}, gl.WithContext(e.Ctx))
	if err != nil {
		return fmt.Errorf("reading the job token allowlist of project %d back: %w", source.ID, err)
	}
	for _, project := range allowed {
		if project != nil && project.ID == target.ID {
			return fmt.Errorf("project %d is still on the job token allowlist of project %d", target.ID, source.ID)
		}
	}
	return nil
}

// commitNoteRemoved reports whether the case deleted the note it was given
// out of the commit's discussion.
func commitNoteRemoved(e *harness.Env, project fixture.Project, sha, discussionID string, noteID int64) error {
	discussion, _, err := e.Client().GL().Discussions.GetCommitDiscussion(project.ID, sha, discussionID, gl.WithContext(e.Ctx))
	if fixture.IsStatus(err, http.StatusNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading commit discussion %s back: %w", discussionID, err)
	}
	for _, note := range discussion.Notes {
		if note != nil && note.ID == noteID {
			return fmt.Errorf("note %d of commit discussion %s is still there", noteID, discussionID)
		}
	}
	return nil
}

// The two halves of the verification judgement.
//
// A Verify asks GitLab whether the change a case was for actually happened,
// and it belongs to the recipe rather than to the case, so it can only exist
// where every case naming that recipe leaves the world in the same state. A
// recipe named by one case that deletes its object has one; a recipe named by
// a read and a delete cannot, because either ending would be right and a
// Verify that accepts both says nothing.
//
// The two tables below partition every recipe the corpus names, and
// worlds_test.go refuses a recipe in neither or in both. That is the gate:
// a recipe added to the corpus cannot reach a run without somebody deciding
// which half it is in, and a recipe whose cases change cannot keep a Verify
// that has stopped being true without the reason here being edited to say so.

// verifiedRecipes are the recipes whose builder returns a Verify, with what
// that Verify asserts.
var verifiedRecipes = map[modelcorpus.Recipe]string{
	modelcorpus.RecipeMergeRequestDiscussion:    "the discussion the world opened is resolved",
	modelcorpus.RecipeFile:                      "the file is gone from the branch",
	modelcorpus.RecipeMilestone:                 "the milestone is gone",
	modelcorpus.RecipeProjectAccessToken:        "the project access token is revoked",
	modelcorpus.RecipePackage:                   "the package is gone from the registry",
	modelcorpus.RecipeBroadcastMessage:          "the broadcast message is gone",
	modelcorpus.RecipeProjectHook:               "the project hook is gone",
	modelcorpus.RecipeProjectBadge:              "the badge is gone",
	modelcorpus.RecipeDraftNote:                 "the draft note is gone, published or deleted",
	modelcorpus.RecipeJobTokenScope:             "the target project is off the inbound allowlist",
	modelcorpus.RecipeInstanceVariableSeeded:    "the instance CI variable is gone",
	modelcorpus.RecipeTag:                       "the tag is gone",
	modelcorpus.RecipePipelineTrigger:           "the pipeline trigger is gone",
	modelcorpus.RecipePipelineSchedule:          "the pipeline schedule is gone",
	modelcorpus.RecipeMemberCandidate:           "the candidate is not a member of the project",
	modelcorpus.RecipeFeatureFlag:               "the feature flag is gone",
	modelcorpus.RecipeCustomEmoji:               "the custom emoji is gone from the group",
	modelcorpus.RecipeWikiPage:                  "the wiki page is gone",
	modelcorpus.RecipeMergeRequestAward:         "the award is off the merge request",
	modelcorpus.RecipeIssueAward:                "the award is off the issue",
	modelcorpus.RecipeDeployKey:                 "the deploy key is gone",
	modelcorpus.RecipeDeployToken:               "the deploy token is gone",
	modelcorpus.RecipeCommitDiscussion:          "the note is gone from the commit's discussion",
	modelcorpus.RecipeTerraformState:            "the Terraform state carries no lock",
	modelcorpus.RecipeGroupProtectedBranch:      "the branch pattern is no longer protected",
	modelcorpus.RecipeGroupProtectedEnvironment: "the environment is no longer protected at group scope",
	modelcorpus.RecipeLDAPLink:                  "the group holds no link for the provider",
	modelcorpus.RecipeGroupSSHCertificate:       "the SSH certificate is gone",
	modelcorpus.RecipeGroupWikiPage:             "the group wiki page is gone",
	modelcorpus.RecipeMemberRole:                "the instance member role is gone",
}

// unverifiedRecipes are the recipes whose builder returns none, with why.
var unverifiedRecipes = map[modelcorpus.Recipe]string{
	modelcorpus.RecipeWorld:                 "the shared world is read-only by contract, and the fixture library's own digest is what holds a test to that",
	modelcorpus.RecipeProject:               "its cases create things in the project rather than change the project, and what they create is named by the model",
	modelcorpus.RecipeGroup:                 "the group half of the same: its cases create in it rather than change it",
	modelcorpus.RecipeBranch:                "one case deletes the branch and two commit into it, which are two endings",
	modelcorpus.RecipeIssue:                 "one case deletes the issue and two update it, which are two endings",
	modelcorpus.RecipeMergeRequest:          "its cases write notes, discussions and drafts on the request rather than changing the request",
	modelcorpus.RecipeMergeRequestSource:    "its cases open a merge request from the branch, and the branch is where the world left it",
	modelcorpus.RecipeEnvironment:           "reading the environment, reading its deployment and stopping the environment are three endings",
	modelcorpus.RecipeRelease:               "listing the releases and deleting the release are two endings",
	modelcorpus.RecipeSnippet:               "reading the snippet's content, deleting it and scheduling a storage move for it are three endings",
	modelcorpus.RecipePipelineJob:           "canceling, playing and deleting the pipeline are three endings",
	modelcorpus.RecipeFailedJob:             "retrying the job and deleting its artifacts are two endings",
	modelcorpus.RecipeMember:                "its case reads the membership the world added",
	modelcorpus.RecipeRunner:                "updating the runner, removing it and reading its jobs are three endings",
	modelcorpus.RecipeCIVariable:            "updating the variable and deleting it are two endings",
	modelcorpus.RecipeInstanceVariable:      "the world reserves a name and creates nothing, so the only thing to read back is what the case made",
	modelcorpus.RecipePackageFiles:          "the world is local files, and nothing of it is on GitLab to read back",
	modelcorpus.RecipeMergeableMergeRequest: "its case asks for a merge once the pipeline passes, and a request with no pipeline is either merged straight away or has the scheduled merge refused, which are two endings",
	modelcorpus.RecipeUser:                  "blocking the user and turning two-factor authentication off are two endings",
	modelcorpus.RecipeDatabaseMigration:     "the world is a version the setup script made pending, and whether GitLab marked it applied is not a read the API offers",
	modelcorpus.RecipeProjectMirror:         "its case forces a push, which leaves nothing a read can tell from a mirror that never pushed",
	modelcorpus.RecipeEpic:                  "closing the epic, deleting it, adding a note and deleting a note are four endings",
	modelcorpus.RecipeEpicIssue:             "assigning the issue and taking it off are two endings",
	modelcorpus.RecipePushRule:              "editing the rule and deleting it are two endings",
	modelcorpus.RecipeProjectServiceAccount: "deleting the account, renaming it, and creating, rotating or revoking its token are five endings",
	modelcorpus.RecipeGroupServiceAccount:   "deleting the account and revoking its token are two endings",
	modelcorpus.RecipeServiceAccountName:    "the world reserves a username and creates nothing",
	modelcorpus.RecipeGeoSite:               "reading the site and deleting it are two endings",
	modelcorpus.RecipeGeoSiteName:           "the world reserves a name and a URL and registers nothing",
	modelcorpus.RecipeScimIdentity:          "nothing provisioned an identity, so there is nothing to read back",
	modelcorpus.RecipeSAMLLink:              "nothing linked a SAML group, so there is nothing to read back",
	modelcorpus.RecipeProjectAlias:          "reading the alias and deleting it are two endings",
	modelcorpus.RecipeProjectAliasName:      "the world reserves an alias name and creates nothing",
	modelcorpus.RecipeExternalStatusCheck:   "reporting a status against the check and deleting the check are two endings",
	modelcorpus.RecipeMergeTrainEntry:       "GitLab refuses to board a request with no pipeline, so nothing is on the train whether a case asked for a place or read one",
	modelcorpus.RecipeStorageMove:           "nothing scheduled a move, so there is no move to read back",
	modelcorpus.RecipeDependencyExport:      "nothing exported a dependency list, and the export a case creates is named by GitLab in the answer",
	modelcorpus.RecipeInstanceAuditEvent:    "its case reads the event the world caused, and an audit log is append-only anyway",
	modelcorpus.RecipeAttestation:           "nothing published an attestation, so there is none to read back",
	modelcorpus.RecipeVulnerability:         "reading a vulnerability, dismissing one and creating an export are three endings",
	modelcorpus.RecipeEnterpriseUser:        "the group manages nobody, so there is no enterprise user to read back",
	modelcorpus.RecipeGroupAccessToken:      "listing the tokens and revoking one are two endings",
	modelcorpus.RecipeModelVersion:          "nothing published a model version, so there is none to read back",
	modelcorpus.RecipeDeploymentApproval:    "GitLab refuses the self-approval the world is built for, so the deployment is where the world left it whatever the model did",
}

// verifies reports whether a recipe's world carries a Verify, read off the
// table rather than off a built world, which is what lets BuildWorld hold a
// builder to the judgement recorded here.
func verifies(recipe modelcorpus.Recipe) bool {
	_, declared := verifiedRecipes[recipe]
	return declared
}
