// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/baseline"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/grants"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/params"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/paths"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/required"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// baselineReports runs the seven scopes the baseline reads, each in the mode
// whose lists hold every finding of its streams: structs, required and enums
// without -gaps-only, which shortens their lists, and the other four with it,
// which leaves theirs whole and drops the context. A scope's gate is not
// consulted, since what the baseline holds is what the gates let through.
func baselineReports(root string) (baseline.Reports, error) {
	var reports baseline.Reports
	for _, scope := range []struct {
		name string
		into *[]byte
		run  func() ([]byte, error)
	}{
		{name: "structs", into: &reports.Structs, run: func() ([]byte, error) { return structsRun(root, false) }},
		{name: "actions", into: &reports.Actions, run: func() ([]byte, error) { return actionsRun(root, true) }},
		{name: "params", into: &reports.Params, run: func() ([]byte, error) {
			content, _, err := paramsRun(root, params.Options{GapsOnly: true})
			return content, err
		}},
		{name: scopePaths, into: &reports.Paths, run: func() ([]byte, error) {
			content, _, err := pathsRun(root, paths.Options{GapsOnly: true})
			return content, err
		}},
		{name: scopeRequired, into: &reports.Required, run: func() ([]byte, error) {
			content, _, err := requiredRun(root, required.Options{})
			return content, err
		}},
		{name: scopeGrants, into: &reports.Grants, run: func() ([]byte, error) {
			content, _, err := grantsRun(root, grants.Options{GapsOnly: true})
			return content, err
		}},
		{name: "enums", into: &reports.Enums, run: func() ([]byte, error) {
			content, _, err := enumsRun(root, false)
			return content, err
		}},
	} {
		cmdutil.Progressf("audit_1to1: [baseline] reading the %s report...", scope.name)
		content, err := scope.run()
		if err != nil {
			return baseline.Reports{}, fmt.Errorf("%s report: %w", scope.name, err)
		}
		*scope.into = content
	}
	return reports, nil
}

// treeBaseline reads the report-only findings of the tree at root.
func treeBaseline(root string) (baseline.Baseline, error) {
	reports, err := baselineReports(root)
	if err != nil {
		return baseline.Baseline{}, err
	}
	return baseline.Build(reports)
}

// writeBaselineFile writes the tree's report-only findings to outputPath,
// which make gen-audit-1to1-baseline names as the committed baseline. Nothing
// is written when a report could not be read.
func writeBaselineFile(outputPath string) error {
	root, err := repositoryRoot(".")
	if err != nil {
		return fmt.Errorf("find repository root: %w", err)
	}
	current, err := treeBaseline(root)
	if err != nil {
		return err
	}
	if writeErr := docgen.WriteReport(outputPath, current.Marshal()); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	return nil
}

// checkBaselineFile holds the tree's report-only findings to the committed
// baseline and writes the comparison to outputPath, failing on a finding the
// baseline does not hold and on a key of it no finding answers. The baseline
// is read first, so a missing one fails before the scopes run, and the
// comparison is written before the failure, so whoever reads the failure has
// every difference.
func checkBaselineFile(outputPath string) error {
	root, err := repositoryRoot(".")
	if err != nil {
		return fmt.Errorf("find repository root: %w", err)
	}
	committed, err := baseline.Read(filepath.Join(root, filepath.FromSlash(baseline.File)))
	if err != nil {
		return err
	}
	current, err := treeBaseline(root)
	if err != nil {
		return err
	}
	comparison := baseline.Compare(committed, current)
	if writeErr := docgen.WriteReport(outputPath, comparison.Marshal()); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	if !comparison.Clean() {
		return errors.New(comparison.Failure())
	}
	return nil
}
