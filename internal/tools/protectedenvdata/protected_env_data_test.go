// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// protected_env_data_test.go tests the entry shapes the project and the group
// protected environment tools share: what each hands the checks GitLab's
// rules are held to before a protect or an update is sent.
package protectedenvdata

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestEntryShapes_AnswerEveryFieldTheChecksRead verifies that each of the
// four entry shapes hands the checks every field it carries, under the field
// it is: a shape that answered a user as a group would pass an entry naming a
// user and a group, which GitLab refuses, and one that dropped the id would
// refuse an existing entry named by it alone. The project and the group
// protected environment tools both send these shapes, so one test holds both.
func TestEntryShapes_AnswerEveryFieldTheChecksRead(t *testing.T) {
	id, user, group, inheritance, approvals, destroy := new(int64(1)), new(int64(2)), new(int64(3)), new(int64(1)), new(int64(4)), new(true)
	level := new(30)
	cases := []struct {
		name string
		got  toolutil.EnvironmentAccess
		want toolutil.EnvironmentAccess
	}{
		{
			name: "a deploy access level to protect with",
			got:  DeployAccessLevelInput{AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance}.EnvironmentAccess(),
			want: toolutil.EnvironmentAccess{AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance},
		},
		{
			name: "an approval rule to protect with",
			got:  ApprovalRuleInput{AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance, RequiredApprovalCount: approvals}.EnvironmentAccess(),
			want: toolutil.EnvironmentAccess{AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance, RequiredApprovals: approvals},
		},
		{
			name: "a deploy access level to update with",
			got:  UpdateDeployAccessLevelInput{ID: id, AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance, Destroy: destroy}.EnvironmentAccess(),
			want: toolutil.EnvironmentAccess{ID: id, AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance, Destroy: destroy},
		},
		{
			name: "an approval rule to update with",
			got:  UpdateApprovalRuleInput{ID: id, AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance, RequiredApprovalCount: approvals, Destroy: destroy}.EnvironmentAccess(),
			want: toolutil.EnvironmentAccess{ID: id, AccessLevel: level, UserID: user, GroupID: group, GroupInheritanceType: inheritance, RequiredApprovals: approvals, Destroy: destroy},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("EnvironmentAccess() = %+v, want %+v", tc.got, tc.want)
			}
		})
	}
}

// TestEntryShapes_AreReadByTheChecks verifies that the checks accept each
// pair of shapes a protect and an update send, and refuse through them what
// they refuse in an entry: the shapes are what both packages hand
// [toolutil.CheckEnvironmentProtect] and [toolutil.CheckEnvironmentUpdate].
func TestEntryShapes_AreReadByTheChecks(t *testing.T) {
	const op = "protect environment"
	maintainers := []DeployAccessLevelInput{{AccessLevel: new(40)}}
	if err := toolutil.CheckEnvironmentProtect(op, maintainers, []ApprovalRuleInput{{UserID: new(int64(5))}}, nil); err != nil {
		t.Errorf("CheckEnvironmentProtect(a role and a user rule) = %v, want nil", err)
	}
	if err := toolutil.CheckEnvironmentProtect(op, maintainers, []ApprovalRuleInput{{}}, nil); err == nil {
		t.Error("CheckEnvironmentProtect(a rule naming nothing) = nil, want a refusal")
	}
	if err := toolutil.CheckEnvironmentUpdate(op, []UpdateDeployAccessLevelInput{{ID: new(int64(11))}}, []UpdateApprovalRuleInput{{ID: new(int64(21)), Destroy: new(true)}}, nil); err != nil {
		t.Errorf("CheckEnvironmentUpdate(an existing entry and a removed rule) = %v, want nil", err)
	}
	if err := toolutil.CheckEnvironmentUpdate(op, []UpdateDeployAccessLevelInput{{AccessLevel: new(40), UserID: new(int64(5))}}, []UpdateApprovalRuleInput{}, nil); err == nil {
		t.Error("CheckEnvironmentUpdate(an entry naming two grantees) = nil, want a refusal")
	}
}
