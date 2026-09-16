//go:build e2e

// ci_variable.go builds the three scopes a CI variable lives in, and reserves
// a name at the one scope that cannot be named by a case.
//
// A project or group variable is scoped to an object the run created, so its
// key may be a literal the prompt states. An instance variable is not: one
// instance holds one `EVAL_TOKEN`, so two attempts naming it would be the
// same variable and the second create would be refused as taken. The reserved
// name is therefore a fact the recipe produces, and both halves live here so
// the difference is read in one place.

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

// The value every fixture variable carries. It is not a secret and is not
// masked: a masked value GitLab refuses to echo would make a read case
// unable to assert anything about what it read.
const ciVariableValue = "e2e-fixture-value"

// CIVariable is a CI variable a builder created, at whatever scope.
type CIVariable struct {
	// Key is how every variable action addresses it.
	Key string
	// Value is what it holds.
	Value string
}

// NewProjectCIVariable creates a variable on the project and registers its
// removal.
//
//nolint:dupl // ProjectVariables and GroupVariables are distinct SDK services with no shared interface, each taking its own option type; everything the two builders do share is already in newScopedCIVariable below, and what is left is the two calls.
func NewProjectCIVariable(e *harness.Env, project Project) CIVariable {
	e.T.Helper()

	return newScopedCIVariable(e, "project", ciVariableKey(e, "PROJECT"),
		func(ctx context.Context, key string) (CIVariable, error) {
			created, _, err := e.Client().GL().ProjectVariables.CreateVariable(project.ID,
				&gl.CreateProjectVariableOptions{Key: new(key), Value: new(ciVariableValue)}, gl.WithContext(ctx))
			if err != nil {
				return CIVariable{}, err
			}
			return CIVariable{Key: created.Key, Value: created.Value}, nil
		},
		func(ctx context.Context, key string) error {
			_, err := e.Client().GL().ProjectVariables.RemoveVariable(project.ID, key, nil, gl.WithContext(ctx))
			return err
		})
}

// NewGroupCIVariable creates a variable on the group and registers its
// removal.
//
//nolint:dupl // see NewProjectCIVariable: the same two calls around a different SDK service and option type.
func NewGroupCIVariable(e *harness.Env, group Group) CIVariable {
	e.T.Helper()

	return newScopedCIVariable(e, "group", ciVariableKey(e, "GROUP"),
		func(ctx context.Context, key string) (CIVariable, error) {
			created, _, err := e.Client().GL().GroupVariables.CreateVariable(group.ID,
				&gl.CreateGroupVariableOptions{Key: new(key), Value: new(ciVariableValue)}, gl.WithContext(ctx))
			if err != nil {
				return CIVariable{}, err
			}
			return CIVariable{Key: created.Key, Value: created.Value}, nil
		},
		func(ctx context.Context, key string) error {
			_, err := e.Client().GL().GroupVariables.RemoveVariable(group.ID, key, nil, gl.WithContext(ctx))
			return err
		})
}

// newScopedCIVariable is what a project and a group variable have in common:
// the retry, the message a failure carries and the removal registered on the
// Env. Only the two calls differ, and they are the two arguments, so the
// shared half cannot drift between the scopes.
func newScopedCIVariable(e *harness.Env, scope, key string,
	create func(context.Context, string) (CIVariable, error),
	remove func(context.Context, string) error,
) CIVariable {
	e.T.Helper()

	variable, err := retryTransient(e, "create "+scope+" CI variable "+key, createRetries, func() (CIVariable, error) {
		return create(e.Ctx, key)
	})
	if err != nil {
		e.T.Fatalf("creating %s CI variable %q: %v", scope, key, err)
	}

	e.Defer(scope+" CI variable "+key, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return toleratingGoneVariable(remove(ctx, key), scope+" variable "+key)
	})
	return variable
}

// ReserveInstanceCIVariableKey returns an instance-wide variable name nothing
// else on the instance uses, and removes it at the end of the test in case a
// case created it.
//
// Nothing is created here: the case under test is the one that creates it, and
// a fixture that made it first would have that case refused as a duplicate.
// The cleanup runs either way, because a reservation whose case never ran
// leaves nothing and a reservation whose case ran leaves a variable the next
// run would collide with.
func ReserveInstanceCIVariableKey(e *harness.Env) string {
	e.T.Helper()
	requireAdmin(e, "an instance CI variable")

	key := ciVariableKey(e, "INSTANCE")
	e.Defer("instance CI variable "+key, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return removeInstanceCIVariable(ctx, e.Client(), key)
	})
	return key
}

// NewInstanceCIVariable creates an instance-wide variable under a reserved
// name, for the cases that change or delete one rather than create it.
func NewInstanceCIVariable(e *harness.Env) CIVariable {
	e.T.Helper()

	key := ReserveInstanceCIVariableKey(e)
	variable, err := retryTransient(e, "create instance CI variable "+key, createRetries, func() (CIVariable, error) {
		return createInstanceCIVariable(e.Ctx, e.Client(), key)
	})
	if err != nil {
		e.T.Fatalf("creating instance CI variable %q: %v", key, err)
	}
	return variable
}

// createInstanceCIVariable asks GitLab for the variable, reading back one a
// previous attempt of the same retry already created.
func createInstanceCIVariable(ctx context.Context, client *gitlabclient.Client, key string) (CIVariable, error) {
	created, _, err := client.GL().InstanceVariables.CreateVariable(&gl.CreateInstanceVariableOptions{
		Key:   new(key),
		Value: new(ciVariableValue),
	}, gl.WithContext(ctx))
	// An attempt that created the variable and then lost the answer is
	// retried, and GitLab refuses the second one with "has already been
	// taken". The variable is what the caller asked for, so it is read back.
	if err != nil && strings.Contains(err.Error(), "already been taken") {
		existing, _, getErr := client.GL().InstanceVariables.GetVariable(key, gl.WithContext(ctx))
		if getErr != nil {
			return CIVariable{}, fmt.Errorf("reading the instance variable %q that already exists: %w", key, getErr)
		}
		return CIVariable{Key: existing.Key, Value: existing.Value}, nil
	}
	if err != nil {
		return CIVariable{}, err
	}
	return CIVariable{Key: created.Key, Value: created.Value}, nil
}

// removeInstanceCIVariable removes the variable and tolerates one that is not
// there, which is both a case that deleted it and a reservation nothing used.
func removeInstanceCIVariable(ctx context.Context, client *gitlabclient.Client, key string) error {
	_, err := client.GL().InstanceVariables.RemoveVariable(key, gl.WithContext(ctx))
	return toleratingGoneVariable(err, "instance variable "+key)
}

// toleratingGoneVariable reports a removal failure unless the variable is
// already gone, which is the ordinary ending of a case that deleted it.
func toleratingGoneVariable(err error, what string) error {
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("removing %s: %w", what, err)
	}
	return nil
}

// ciVariableKey names a variable for this test at the given scope.
func ciVariableKey(e *harness.Env, scope string) string {
	return ciVariableKeyFrom(scope, e.Name("var"))
}

// ciVariableKeyFrom spells a variable name GitLab accepts out of a run-scoped
// name: letters, digits and underscores only, so the scoping's dashes and dots
// become underscores and the whole name is upper-cased the way a CI variable
// is written.
//
// GitLab refuses anything else outright, and the run scoping this package
// hands out is full of dashes, so a name passed through unchanged would be
// refused by every create rather than by some of them.
func ciVariableKeyFrom(scope, name string) string {
	return "E2E_" + scope + "_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name))
}
