// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// docs_test.go is the live run: every page of the site that marks a block,
// run in a reader of its own against the GitLab run-docker-e2e.sh booted.
// make test-e2e-docs starts it; any other go test of the package skips it and
// runs the controls alone.
package docse2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// The windows of the live run. A block is a reader's command, so two minutes
// is a hang rather than a slow step; a server gets three seconds to fail on
// its flags, and a minute to answer its first probe, which covers its tier
// and scope detection against a busy GitLab.
const (
	liveBlockTimeout = 2 * time.Minute
	liveSettle       = 3 * time.Second
	liveReadiness    = time.Minute
	livePoll         = time.Second
)

// TestDocs_MarkedBlocks_RunInAReaderAgainstTheE2EGitLab runs the marked
// blocks of every page, one reader per page, and fails a page at its first
// block that fails. A tree in which no page marks a block fails as well,
// since a run that ran nothing would pass whatever the pages say.
func TestDocs_MarkedBlocks_RunInAReaderAgainstTheE2EGitLab(t *testing.T) {
	if os.Getenv("E2E_MODE") != "docker" {
		t.Skip("the live run needs the Docker GitLab of run-docker-e2e.sh; make test-e2e-docs starts it")
	}
	binary := requireEnv(t, "E2E_SERVER_BINARY")
	token := requireEnv(t, "GITLAB_TOKEN")
	gitlabURL := requireEnv(t, "E2E_GITLAB_INTERNAL_URL")

	repo := repoRoot(t)
	version, err := releaseVersion(repo)
	if err != nil {
		t.Fatalf("reading the release the site renders: %v", err)
	}
	pages, err := markedPages(filepath.Join(repo, "site", "src", "content", "docs"))
	if err != nil {
		t.Fatalf("reading the marked blocks of the site: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no page of the site marks a block to run, so this run would check nothing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	const bin = "docker"
	image, err := buildReader(ctx, bin)
	if err != nil {
		t.Fatalf("building the reader: %v", err)
	}
	network, err := gitlabNetwork(ctx, bin, filepath.Join(repo, "test", "e2e", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("finding the e2e GitLab's network: %v", err)
	}
	subs := substitutions{version: version, gitlabURL: gitlabURL, token: token}
	asset := "gitlab-mcp-server-linux-" + runtime.GOARCH

	for i, page := range pages {
		t.Run(page.rel, func(t *testing.T) {
			name := fmt.Sprintf("gitlab-mcp-docse2e-%d-%d", os.Getpid(), i)
			t.Logf("%d marked blocks in the reader %s; if this run is interrupted: docker rm -f %s", len(page.blocks), name, name)
			rd, startErr := startReader(ctx, bin, image, network, name, binary, asset)
			if rd != nil {
				t.Cleanup(func() {
					if removeErr := rd.remove(context.WithoutCancel(ctx)); removeErr != nil {
						t.Errorf("removing the reader: %v", removeErr)
					}
				})
			}
			if startErr != nil {
				t.Fatalf("starting the reader: %v", startErr)
			}
			r := &runner{
				shell:        rd,
				subs:         subs,
				blockTimeout: liveBlockTimeout,
				settle:       liveSettle,
				readiness:    liveReadiness,
				poll:         livePoll,
			}
			if runErr := r.runPage(ctx, page.rel, page.blocks); runErr != nil {
				t.Error(runErr)
			}
		})
	}
}

// requireEnv returns a variable run-docker-e2e.sh sets, failing the test when
// it is empty.
func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is empty; make test-e2e-docs sets it, from the e2e environment run-docker-e2e.sh writes", name)
	}
	return value
}

// repoRoot returns the repository root, the nearest directory above the test
// holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("finding the working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's directory")
		}
		dir = parent
	}
}
