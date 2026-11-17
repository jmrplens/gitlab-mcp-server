// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mergeRequest is what a run reads of a merge request.
type mergeRequest struct {
	IID                 int64  `json:"iid"`
	State               string `json:"state"`
	DetailedMergeStatus string `json:"detailed_merge_status"`
}

// stillChecking are the detailed_merge_status values GitLab answers while it
// has not finished deciding whether the merge request can be merged.
var stillChecking = map[string]bool{"unchecked": true, "checking": true, "preparing": true, "approvals_syncing": true}

// seedStock is the helper the fixture merge request adds to the fulfillment
// module, in front of reserve_stock: a deterministic diff. It is one literal,
// as the milestone title is, so no operator joins constant parts that a
// mutation run would mutate and no test could reach.
const seedStock = "\n\ndef seed_stock(sku: str, quantity: int) -> None:\n    \"\"\"Add `quantity` units of `sku` to the in-memory stock register.\"\"\"\n    _STOCK[sku] = _STOCK.get(sku, 0) + quantity\n\n"

// fixtureMergeRequest makes sure kg-fixtures holds the squash-merged merge
// request from the fixture branch (issue 1428). The merge requests from the
// branch decide what is left to do: one merged is the fixture, one open is
// merged, and one closed without being merged is reopened and merged while
// its branch is there, or replaced by a new one when the branch is gone. With
// none at all, one is opened from the branch, pushed first unless a run that
// failed after pushing it left it there.
func (p *provisioner) fixtureMergeRequest(ctx context.Context) error {
	fmt.Fprintf(p.out, "    → squash-merged MR (%s → main)\n", mrBranch)
	listed := p.api.do(ctx, "GET", fmt.Sprintf("/projects/%d/merge_requests?source_branch=%s&state=all", p.kgID, mrBranch), nil)
	var requests []mergeRequest
	if listed.status != 200 || !listed.decode(&requests) || requests == nil {
		return fmt.Errorf("    ✗ cannot list the merge requests from %s (HTTP %d): %s", mrBranch, listed.status, listed)
	}
	state, iid := pickMergeRequest(requests)
	switch state {
	case "merged":
		fmt.Fprintf(p.out, "    ✓ MR !%d is merged\n", iid)
		return nil
	case "opened":
		return p.merge(ctx, iid)
	case "closed":
		return p.repairClosed(ctx, iid)
	}
	// A run that pushed the branch and then failed to open its merge request
	// leaves the branch behind. A new commit pushed over it is not a
	// fast-forward and GitLab refuses it, so the merge request is opened from
	// the branch as it is.
	exists, err := p.branchExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		fmt.Fprintf(p.out, "    → branch %s is there without a merge request: opening one from it\n", mrBranch)
	}
	return p.openAndMerge(ctx, !exists)
}

// branchExists reports whether the fixture branch is on GitLab, and fails
// when GitLab answers neither that it is nor that it is not.
func (p *provisioner) branchExists(ctx context.Context) (bool, error) {
	branch := p.api.do(ctx, "GET", fmt.Sprintf("/projects/%d/repository/branches/%s", p.kgID, mrBranchEnc), nil)
	switch branch.status {
	case 200:
		return true, nil
	case 404:
		return false, nil
	}
	return false, fmt.Errorf("    ✗ cannot tell whether branch %s exists (HTTP %d): %s", mrBranch, branch.status, branch)
}

// openAndMerge opens the fixture merge request, pushing its branch first when
// push says the branch is not there, and merges it.
func (p *provisioner) openAndMerge(ctx context.Context, push bool) error {
	iid, err := p.openMergeRequest(ctx, push)
	if err != nil {
		return err
	}
	return p.merge(ctx, iid)
}

// pickMergeRequest picks the merge request that decides the run: a merged one
// before an open one before a closed one, the newest of a state, and "" when
// the branch has none in any of the three.
func pickMergeRequest(requests []mergeRequest) (state string, iid int64) {
	for _, candidate := range []string{"merged", "opened", "closed"} {
		for _, request := range requests {
			if request.State == candidate {
				iid = max(iid, request.IID)
			}
		}
		if iid > 0 {
			return candidate, iid
		}
	}
	return "", 0
}

// repairClosed reopens and merges a closed merge request whose branch is
// still there, and opens and merges a new one when the branch is gone.
func (p *provisioner) repairClosed(ctx context.Context, iid int64) error {
	exists, err := p.branchExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintf(p.out, "    → MR !%d was closed and its branch is gone: opening a new one\n", iid)
		return p.openAndMerge(ctx, true)
	}
	reopened := p.api.do(ctx, "PUT", fmt.Sprintf("/projects/%d/merge_requests/%d", p.kgID, iid), map[string]string{"state_event": "reopen"})
	var request mergeRequest
	if !reopened.ok() || !reopened.decode(&request) || request.State != "opened" {
		return fmt.Errorf("    ✗ could not reopen MR !%d (HTTP %d): %s", iid, reopened.status, reopened)
	}
	fmt.Fprintf(p.out, "    ✓ MR !%d reopened\n", iid)
	return p.merge(ctx, iid)
}

// merge squash-merges the merge request once GitLab has checked it, and fails
// unless GitLab answers the merge with the merge request merged.
func (p *provisioner) merge(ctx context.Context, iid int64) error {
	if err := p.waitForMergeability(ctx, iid); err != nil {
		return err
	}
	merged := p.api.do(ctx, "PUT", fmt.Sprintf("/projects/%d/merge_requests/%d/merge", p.kgID, iid), map[string]any{
		"squash":                      true,
		"should_remove_source_branch": true,
		"squash_commit_message":       mrTitle,
	})
	if !merged.ok() {
		return fmt.Errorf("    ✗ GitLab refused to merge MR !%d (HTTP %d): %s", iid, merged.status, merged)
	}
	var request mergeRequest
	merged.decode(&request)
	if request.State != "merged" {
		return fmt.Errorf("    ✗ MR !%d is %s after the merge, not merged: %s", iid, firstNonEmpty(request.State, "in no state"), merged)
	}
	fmt.Fprintf(p.out, "    ✓ MR !%d merged\n", iid)
	return nil
}

// waitForMergeability reads the merge request until detailed_merge_status
// says GitLab has finished checking it, at most mergePolls times, since
// GitLab decides asynchronously after a merge request is opened or reopened
// and refuses a merge asked for before. An answer without the field, from a
// GitLab older than it, leaves the merge itself to say whether it can be made.
func (p *provisioner) waitForMergeability(ctx context.Context, iid int64) error {
	status := ""
	for range p.mergePolls {
		got := p.api.do(ctx, "GET", fmt.Sprintf("/projects/%d/merge_requests/%d", p.kgID, iid), nil)
		if got.status != 200 {
			return fmt.Errorf("    ✗ cannot read MR !%d (HTTP %d): %s", iid, got.status, got)
		}
		var request mergeRequest
		got.decode(&request)
		status = request.DetailedMergeStatus
		if !stillChecking[status] {
			return nil
		}
		if err := p.sleep(ctx, p.mergeInterval); err != nil {
			return err
		}
	}
	return fmt.Errorf("    ✗ MR !%d was still being checked after %d reads (detailed_merge_status %s)", iid, p.mergePolls, status)
}

// openMergeRequest opens the merge request from the fixture branch and
// returns the new merge request's iid. With push, it first pushes the branch
// from main, with the seed_stock helper added to the fulfillment module.
func (p *provisioner) openMergeRequest(ctx context.Context, push bool) (int64, error) {
	if push {
		if err := p.pushMergeRequestBranch(ctx); err != nil {
			return 0, err
		}
	}
	opened := p.api.do(ctx, "POST", fmt.Sprintf("/projects/%d/merge_requests", p.kgID), map[string]any{
		"source_branch":        mrBranch,
		"target_branch":        "main",
		"title":                mrTitle,
		"description":          "Adds a `seed_stock(sku, quantity)` helper to the fulfillment module so test runs can populate the in-memory stock register without mocking.",
		"remove_source_branch": true,
		"squash":               true,
	})
	var request mergeRequest
	if !opened.ok() || !opened.decode(&request) || request.IID <= 0 {
		return 0, fmt.Errorf("    ✗ could not open the fixture MR (HTTP %d): %s", opened.status, opened)
	}
	fmt.Fprintf(p.errOut, "    ✓ MR !%d opened\n", request.IID)
	return request.IID, nil
}

// pushMergeRequestBranch clones kg-fixtures into a temporary directory,
// commits the seed_stock helper on the fixture branch and pushes it.
func (p *provisioner) pushMergeRequestBranch(ctx context.Context) error {
	work, err := os.MkdirTemp("", "orbit-fixture-mr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	clone := filepath.Join(work, "repository")
	remote := p.gitlabURL + "/" + p.namespace + "/kg-fixtures.git"
	if cloneErr := p.gitSteps(ctx, work, [][]string{{"-c", p.authHeader(), "clone", "--quiet", remote, clone}}); cloneErr != nil {
		return cloneErr
	}
	if branchErr := p.gitSteps(ctx, clone, [][]string{
		{"config", "--local", "http.extraHeader", "PRIVATE-TOKEN: " + p.token},
		{"checkout", "-q", "-b", mrBranch},
	}); branchErr != nil {
		return branchErr
	}
	if seedErr := insertSeedStock(filepath.Join(clone, "src", "acme", "orders", "fulfillment.py")); seedErr != nil {
		return seedErr
	}
	return p.gitSteps(ctx, clone, [][]string{
		{"-c", "user.email=" + botEmail, "-c", "user.name=" + botName, "add", "-A"},
		{"-c", "user.email=" + botEmail, "-c", "user.name=" + botName, "commit", "-m", "feat(fulfillment): add seed_stock helper for fixture test runs", "--quiet"},
		{"push", "--quiet", "-u", "origin", mrBranch},
	})
}

// insertSeedStock adds the seed_stock helper in front of reserve_stock in the
// fulfillment module at path, and fails when the module has no reserve_stock
// past its first byte to put it in front of.
func insertSeedStock(path string) error {
	source, err := os.ReadFile(path) //#nosec G304 -- the module of a clone this run made
	if err != nil {
		return err
	}
	text := string(source)
	at := strings.Index(text, "def reserve_stock")
	if at <= 0 {
		return errors.New("    ✗ the fulfillment module has no reserve_stock to add seed_stock in front of")
	}
	//#nosec G703 -- the module of a clone this run made in its own temporary directory
	return os.WriteFile(path, []byte(text[:at]+seedStock+text[at:]), 0o600)
}
