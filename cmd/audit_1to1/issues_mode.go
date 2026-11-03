// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/routes"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
)

// githubTimeout bounds one question to GitHub, so a network that takes the
// request and never answers fails the check as unable to judge rather than
// hanging it.
const githubTimeout = time.Minute

// runCheckIssuesMode asks GitHub about the issues of the tracked rows, with a
// context a Ctrl+C cancels, and ends through fatalf when the check fails.
func runCheckIssuesMode(outputPath string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	failOn(checkTrackedIssues(ctx, outputPath))
}

// checkTrackedIssues reads the rows of the routes and sdk reports that rest on
// an issue, asks GitHub about each issue once, and writes the answers to
// outputPath, failing on an issue that is not open and on one it could not
// ask about. Both reports are read whole, since -gaps-only drops the rows
// that answer a finding, which are the ones that name an issue.
func checkTrackedIssues(ctx context.Context, outputPath string) error {
	root, err := repositoryRoot(".")
	if err != nil {
		return fmt.Errorf("find repository root: %w", err)
	}
	routesReport, _, err := routesRun(root, routes.Options{})
	if err != nil {
		return fmt.Errorf("routes report: %w", err)
	}
	sdkReport, _, err := sdkRun(root, false)
	if err != nil {
		return fmt.Errorf("sdk report: %w", err)
	}
	rows, err := issues.Rows(routesReport, sdkReport)
	if err != nil {
		return err
	}
	report := issues.Check(ctx, issues.Client{
		API:        githubAPI,
		Repository: issues.DefaultRepository,
		Token:      githubToken(),
		HTTP:       &http.Client{Timeout: githubTimeout},
	}, rows)
	if writeErr := docgen.WriteReport(outputPath, report.Marshal()); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	if !report.Clean() {
		return errors.New(report.Failure())
	}
	return nil
}

// githubToken is the token GitHub is asked with: GITHUB_TOKEN, which a
// workflow sets, or GH_TOKEN, which the gh CLI reads; empty asks without one.
func githubToken() string {
	if token := getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	return getenv("GH_TOKEN")
}
