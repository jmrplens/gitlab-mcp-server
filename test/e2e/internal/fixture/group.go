//go:build e2e

// group.go builds groups and subgroups, and removes them the way projects are
// removed: marked, re-read, then permanently deleted under the renamed path.

package fixture

import (
	"context"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Group is a group a builder created, as a test refers to it.
type Group struct {
	// ID is the numeric identifier every group-scoped action takes.
	ID int64
	// Path is the full path, parents included.
	Path string
	// Name is the group's display name.
	Name string
	// ParentID is the parent group's ID, zero for a top-level group.
	ParentID int64
}

// IDParam spells the group's ID as a group_id parameter declared a string
// takes it, for the same reason [Project.IDParam] exists: the dispatcher
// surfaces coerce a number into the string, and the individual surface
// refuses one against the schema. An action whose group_id is declared
// numeric takes ID itself.
func (g Group) IDParam() string {
	return strconv.FormatInt(g.ID, 10)
}

// groupSpec is what a builder asks GitLab for.
type groupSpec struct {
	name       string
	visibility gl.VisibilityValue
	parentID   int64
}

// GroupOption adjusts one group builder.
type GroupOption func(*groupSpec)

// WithGroupVisibility sets the group's visibility; private is the default.
func WithGroupVisibility(visibility gl.VisibilityValue) GroupOption {
	return func(spec *groupSpec) { spec.visibility = visibility }
}

// WithGroupNamePrefix changes the prefix the generated name opens with.
func WithGroupNamePrefix(prefix string) GroupOption {
	return func(spec *groupSpec) { spec.name = prefix }
}

// NewGroup creates a private top-level group and registers its permanent
// deletion on the Env. Deleting the group deletes everything under it, so a
// project built with InGroup needs no deletion of its own, though it is given
// one anyway since the ledger runs it first and a project that fails to delete
// says so on its own line.
func NewGroup(e *harness.Env, opts ...GroupOption) Group {
	e.T.Helper()
	return newGroup(e, 0, opts)
}

// NewSubgroup creates a private group under parent and registers its
// deletion on the Env.
func NewSubgroup(e *harness.Env, parent Group, opts ...GroupOption) Group {
	e.T.Helper()
	return newGroup(e, parent.ID, opts)
}

// newGroup is the shared half of the two group builders.
func newGroup(e *harness.Env, parentID int64, opts []GroupOption) Group {
	e.T.Helper()
	armSweep(e)

	spec := groupSpec{name: "grp", visibility: gl.PrivateVisibility, parentID: parentID}
	for _, opt := range opts {
		opt(&spec)
	}

	group, err := createGroup(e, spec, func() string { return e.Name(spec.name) })
	if err != nil {
		e.T.Fatalf("creating the group fixture: %v", err)
	}
	e.Defer("group "+group.Path, func(ctx context.Context) error {
		return DeleteGroup(ctx, e.Client(), group.ID, group.Path)
	})
	return group
}

// createGroup asks GitLab for a group under a fresh name on every attempt,
// retried on the same failures a project creation is.
func createGroup(e *harness.Env, spec groupSpec, nextName func() string) (Group, error) {
	e.T.Helper()

	enterprise := e.Runtime().Tier.IsEnterprise()
	return retryWhen(e, "create group", createRetries,
		func(err error) bool { return CreateRetryable(err, enterprise) },
		func() (Group, error) {
			name := nextName()
			opts := &gl.CreateGroupOptions{
				Name:       new(name),
				Path:       new(name),
				Visibility: new(spec.visibility),
			}
			if spec.parentID != 0 {
				opts.ParentID = new(spec.parentID)
			}
			created, _, err := e.Client().GL().Groups.CreateGroup(opts, gl.WithContext(e.Ctx))
			if err != nil {
				return Group{}, err
			}
			return groupOf(created), nil
		})
}

// groupOf reads what a test needs out of what GitLab returned.
func groupOf(g *gl.Group) Group {
	return Group{ID: g.ID, Path: g.FullPath, Name: g.Name, ParentID: g.ParentID}
}

// AddGroupMember makes user a member of the group at the given access
// level. The membership goes with the group, so nothing is registered.
func AddGroupMember(e *harness.Env, group Group, user User, accessLevel gl.AccessLevelValue) {
	e.T.Helper()

	_, err := retryTransient(e, "add group member "+user.Username, createRetries, func() (struct{}, error) {
		_, _, err := e.Client().GL().GroupMembers.AddGroupMember(group.ID, &gl.AddGroupMemberOptions{
			UserID:      new(user.ID),
			AccessLevel: new(accessLevel),
		}, gl.WithContext(e.Ctx))
		return struct{}{}, err
	})
	if err != nil {
		e.T.Fatalf("adding user %s to group %d: %v", user.Username, group.ID, err)
	}
}

// DeleteGroup removes a group permanently, with everything under it.
//
// It follows the project's two steps for the same reason: an instance with
// delayed deletion marks the group first and renames its path while it waits,
// and the permanent removal has to name that path. An instance without
// delayed deletion removes it on the first step, and the second step's 404
// then says the job is done.
//
//nolint:dupl // DeleteProject is this step for step on a service of its own, and the shared part is deletePermanently
func DeleteGroup(ctx context.Context, client *gitlabclient.Client, groupID int64, path string) error {
	groups := client.GL().Groups
	return deletePermanently(ctx, "group", groupID, path, deletionSteps{
		mark: func(ctx context.Context) error {
			_, err := groups.DeleteGroup(groupID, nil, gl.WithContext(ctx))
			return err
		},
		currentPath: func(ctx context.Context) (string, error) {
			current, _, err := groups.GetGroup(groupID, nil, gl.WithContext(ctx))
			if err != nil {
				return "", err
			}
			return current.FullPath, nil
		},
		remove: func(ctx context.Context, path string) error {
			_, err := groups.DeleteGroup(groupID, &gl.DeleteGroupOptions{
				PermanentlyRemove: new(true),
				FullPath:          new(path),
			}, gl.WithContext(ctx))
			return err
		},
	})
}
