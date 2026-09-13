//go:build e2e

// iteration.go builds an iteration, which GitLab offers no REST API to
// create: a cadence and an iteration under it are made over GraphQL, and an
// issue is put into the iteration the same way.
//
// Iterations are a licensed feature of a group namespace, so the builder
// takes a group and skips on an instance that will not create a cadence,
// naming the reason GitLab gave. The suite this replaces built the same
// fixture inside one test body, returned false on every failure, and let the
// test decide; here a fixture that cannot be built ends the test where it
// fails.

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Iteration is an iteration a builder created in a group's cadence.
type Iteration struct {
	// GID is the GraphQL global ID, which the assignment mutation takes.
	GID string
	// IID is the number the REST listings report it under.
	IID int64
	// Title is what it was created as.
	Title string
	// GroupID is the group whose cadence holds it.
	GroupID int64
}

// String names the iteration in a message.
func (i Iteration) String() string {
	return fmt.Sprintf("%s (iid %d, %s)", i.GID, i.IID, i.Title)
}

// iterationLength is how long a fixture iteration lasts. It starts today so
// that it is current, which is what an issue assignment needs.
const iterationLength = 7 * 24 * time.Hour

// cadenceMutation creates a manual cadence, so the fixture controls the
// iteration's dates rather than GitLab's scheduler.
const cadenceMutation = `mutation($groupPath: ID!, $title: String!) {
  iterationCadenceCreate(input: {
    groupPath: $groupPath, title: $title, automatic: false, active: true,
    durationInWeeks: 1, iterationsInAdvance: 1, rollOver: false
  }) {
    iterationCadence { id }
    errors
  }
}`

// iterationMutation creates one iteration in a cadence.
const iterationMutation = `mutation($cadenceId: IterationsCadenceID!, $groupPath: ID!, $title: String!, $startDate: String!, $dueDate: String!) {
  iterationCreate(input: {
    iterationsCadenceId: $cadenceId, groupPath: $groupPath, title: $title,
    startDate: $startDate, dueDate: $dueDate
  }) {
    iteration { id iid }
    errors
  }
}`

// issueIterationMutation puts an issue into an iteration. The mutation is
// issueSetIteration, addressed by project path and issue iid rather than by
// a global ID, which is the shape GitLab documents for it.
const issueIterationMutation = `mutation($projectPath: ID!, $iid: String!, $iterationId: IterationID!) {
  issueSetIteration(input: { projectPath: $projectPath, iid: $iid, iterationId: $iterationId }) {
    issue { iid iteration { id } }
    errors
  }
}`

// NewIteration creates a manual cadence in the group and one current
// iteration under it. Both go with the group, so nothing is registered. The
// test is skipped when the instance refuses to create a cadence, since that
// is the license or the feature flag rather than the test.
func NewIteration(e *harness.Env, group Group) Iteration {
	e.T.Helper()

	var cadence struct {
		IterationCadenceCreate struct {
			IterationCadence struct {
				ID string `json:"id"`
			} `json:"iterationCadence"`
			Errors []string `json:"errors"`
		} `json:"iterationCadenceCreate"`
	}
	err := mutate(e, cadenceMutation, map[string]any{"groupPath": group.Path, "title": e.Name("cadence")}, &cadence)
	if err != nil {
		e.Skipf("creating an iteration cadence in group %s: %v", group.Path, err)
	}
	if errs := cadence.IterationCadenceCreate.Errors; len(errs) > 0 {
		e.Skipf("GitLab refused an iteration cadence in group %s: %s", group.Path, strings.Join(errs, "; "))
	}
	cadenceGID := cadence.IterationCadenceCreate.IterationCadence.ID
	if cadenceGID == "" {
		e.T.Fatalf("the iteration cadence in group %s was created with no ID", group.Path)
	}

	title := e.Name("iteration")
	start := time.Now().UTC()
	var created struct {
		IterationCreate struct {
			Iteration struct {
				ID  string `json:"id"`
				IID string `json:"iid"`
			} `json:"iteration"`
			Errors []string `json:"errors"`
		} `json:"iterationCreate"`
	}
	err = mutate(e, iterationMutation, map[string]any{
		"cadenceId": cadenceGID,
		"groupPath": group.Path,
		"title":     title,
		"startDate": start.Format(time.DateOnly),
		"dueDate":   start.Add(iterationLength).Format(time.DateOnly),
	}, &created)
	if err != nil {
		e.T.Fatalf("creating an iteration in cadence %s: %v", cadenceGID, err)
	}
	if errs := created.IterationCreate.Errors; len(errs) > 0 {
		e.T.Fatalf("GitLab refused the iteration in cadence %s: %s", cadenceGID, strings.Join(errs, "; "))
	}
	iteration := created.IterationCreate.Iteration
	if iteration.ID == "" {
		e.T.Fatalf("the iteration in cadence %s was created with no ID", cadenceGID)
	}
	iid, err := strconv.ParseInt(iteration.IID, 10, 64)
	if err != nil {
		e.T.Fatalf("the iteration %s reports the iid %q, which is not a number: %v", iteration.ID, iteration.IID, err)
	}
	return Iteration{GID: iteration.ID, IID: iid, Title: title, GroupID: group.ID}
}

// AssignIssueIteration puts an issue into an iteration, which is what makes
// GitLab record an iteration event on the issue.
func AssignIssueIteration(e *harness.Env, project Project, issue Issue, iteration Iteration) {
	e.T.Helper()

	var assigned struct {
		IssueSetIteration struct {
			Issue *struct {
				Iteration *struct {
					ID string `json:"id"`
				} `json:"iteration"`
			} `json:"issue"`
			Errors []string `json:"errors"`
		} `json:"issueSetIteration"`
	}
	err := mutate(e, issueIterationMutation, map[string]any{
		"projectPath": project.Path,
		"iid":         strconv.FormatInt(issue.IID, 10),
		"iterationId": iteration.GID,
	}, &assigned)
	if err != nil {
		e.T.Fatalf("assigning issue #%d of %s to iteration %s: %v", issue.IID, project.Path, iteration.GID, err)
	}
	if errs := assigned.IssueSetIteration.Errors; len(errs) > 0 {
		e.T.Fatalf("GitLab refused to put issue #%d of %s into iteration %s: %s", issue.IID, project.Path, iteration.GID, strings.Join(errs, "; "))
	}
	result := assigned.IssueSetIteration.Issue
	if result == nil || result.Iteration == nil || result.Iteration.ID != iteration.GID {
		e.T.Fatalf("issue #%d of %s is not in iteration %s after the assignment: %+v", issue.IID, project.Path, iteration.GID, result)
	}
}

// graphQLEnvelope is the whole of a GraphQL answer: the data half, kept raw
// for the caller's own shape, and the top-level errors GitLab reports
// inside a 200 for a document it refused.
type graphQLEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// mutate sends one document on the test's own context and client.
func mutate(e *harness.Env, query string, variables map[string]any, into any) error {
	e.T.Helper()
	return runGraphQL(e.Ctx, e.Client(), query, variables, into)
}

// errGraphQLNoData is a document GitLab answered with neither data nor
// errors, which no GraphQL server does and a proxy in front of one might.
var errGraphQLNoData = errors.New("GitLab answered the document with no data")

// runGraphQL sends one document once and decodes its data into the caller's
// shape, turning the document-level errors into an error of its own: a
// refused document is otherwise a 200 with an empty data half, which reads
// as a mutation that silently did nothing.
func runGraphQL(ctx context.Context, client *gitlabclient.Client, query string, variables map[string]any, into any) error {
	var envelope graphQLEnvelope
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: query, Variables: variables}, &envelope, gl.WithContext(ctx)); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, refusal := range envelope.Errors {
			messages = append(messages, refusal.Message)
		}
		return fmt.Errorf("GitLab refused the document: %s", strings.Join(messages, "; "))
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errGraphQLNoData
	}
	if err := json.Unmarshal(envelope.Data, into); err != nil {
		return fmt.Errorf("decoding the document's data into %T: %w", into, err)
	}
	return nil
}
