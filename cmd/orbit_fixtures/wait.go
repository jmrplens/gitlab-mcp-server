// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// waitConfig is one configured indexer wait.
type waitConfig struct {
	token, gitlabURL, namespace string
	// timeoutSeconds is how long the wait may take: one read every interval,
	// so timeoutSeconds/15 reads at the 15 seconds a real run waits.
	timeoutSeconds int
	interval       time.Duration
	client         *http.Client
	// sleep waits between two reads; a test makes it return at once.
	sleep func(context.Context, time.Duration) error
}

// waitQuery reads the Orbit indexing state of a group. It replaced
// GET /api/v4/orbit/graph_status when GitLab.com removed that route on
// 2026-10-28 (issue 1516).
const waitQuery = "query OrbitWaitIndexer($fullPath: ID!) { group(fullPath: $fullPath) { orbit { indexing { state projects { indexed notIndexed } } } } }"

// waitRequest is the GraphQL request a read sends.
type waitRequest struct {
	Query     string        `json:"query"`
	Variables waitVariables `json:"variables"`
}

// waitVariables are the variables of [waitQuery].
type waitVariables struct {
	FullPath string `json:"fullPath"`
}

// waitFromEnv reads the wait's settings from the environment.
func waitFromEnv(getenv func(string) string) (waitConfig, error) {
	timeout := firstNonEmpty(getenv("ORBIT_FIXTURES_INDEXER_TIMEOUT"), "600")
	seconds, err := strconv.Atoi(timeout)
	if err != nil || seconds < 0 {
		return waitConfig{}, fmt.Errorf("ORBIT_FIXTURES_INDEXER_TIMEOUT=%q is not a whole number of seconds", timeout)
	}
	return waitConfig{
		token:          getenv("GITLAB_COM_TOKEN"),
		gitlabURL:      strings.TrimSuffix(firstNonEmpty(getenv("ORBIT_FIXTURES_GITLAB_URL"), defaultGitLabURL), "/"),
		namespace:      firstNonEmpty(getenv("ORBIT_FIXTURES_NAMESPACE"), defaultNamespace),
		timeoutSeconds: seconds,
		interval:       15 * time.Second,
		client:         httpClient(),
		sleep:          sleepContext,
	}, nil
}

// indexing is the progress one read reports: the state GitLab answered, or
// "null" when it answered the index null, and the projects indexed and left
// out. A read that got no answer it could read is "?".
type indexing struct {
	state               string
	indexed, notIndexed int
}

// runWait reads the indexing state of the namespace until it is READY with no
// project left out and at least the two fixture projects indexed, which is
// done on the first read for a namespace indexed on an earlier run (issue
// 1534), or until the timeout allows no further read, and then proceeds with
// a warning: the live suite skips what it cannot ask and tolerates partial
// indexing. A null index, GitLab's answer when Orbit is not served to the
// token there, is said and read again like any other state.
func runWait(ctx context.Context, cfg waitConfig, stdout, stderr io.Writer) int {
	if cfg.token == "" {
		fmt.Fprintf(stderr, "%s wait-indexer: GITLAB_COM_TOKEN is required\n", toolName)
		return 1
	}
	fmt.Fprintf(stdout, "\n=== Waiting for Orbit indexer to catch up (timeout: %ds) ===\n", cfg.timeoutSeconds)
	attempts := cfg.timeoutSeconds / 15
	for index := range attempts {
		attempt := index + 1
		progress := readIndexing(ctx, cfg)
		fmt.Fprintf(stdout, "  [%d/%d] state=%s indexed=%d not_indexed=%d\n", attempt, attempts, progress.state, progress.indexed, progress.notIndexed)
		if progress.state == "READY" && progress.notIndexed == 0 && progress.indexed >= 2 {
			fmt.Fprintf(stdout, "  ✓ indexer caught up (%d projects indexed)\n", progress.indexed)
			return 0
		}
		if progress.state == "null" {
			fmt.Fprintf(stderr, "    GitLab answered the index null: Orbit is not served for %s to this token\n", cfg.namespace)
		}
		if err := cfg.sleep(ctx, cfg.interval); err != nil {
			fmt.Fprintf(stderr, "%s wait-indexer: %v\n", toolName, err)
			return 1
		}
	}
	fmt.Fprintf(stderr, "  ⚠ indexer did not catch up within %ds\n", cfg.timeoutSeconds)
	fmt.Fprintln(stderr, "    Proceeding anyway: live test assertions are tolerant of")
	fmt.Fprintln(stderr, "    partial indexing (row_count > 0 rather than strict equality)")
	return 0
}

// readIndexing asks GitLab for the namespace's indexing state once.
func readIndexing(ctx context.Context, cfg waitConfig) indexing {
	unread := indexing{state: "?"}
	// Strings alone, which encoding/json cannot fail on.
	body, _ := json.Marshal(waitRequest{Query: waitQuery, Variables: waitVariables{FullPath: cfg.namespace}}) //nolint:errchkjson // see above
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.gitlabURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return unread
	}
	request.Header.Set("Authorization", "Bearer "+cfg.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := cfg.client.Do(request)
	if err != nil {
		return unread
	}
	defer response.Body.Close()
	var answer struct {
		Data struct {
			Group struct {
				Orbit struct {
					Indexing *struct {
						State    string `json:"state"`
						Projects struct {
							Indexed    int `json:"indexed"`
							NotIndexed int `json:"notIndexed"`
						} `json:"projects"`
					} `json:"indexing"`
				} `json:"orbit"`
			} `json:"group"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, maxAnswerBytes)).Decode(&answer) != nil {
		return unread
	}
	index := answer.Data.Group.Orbit.Indexing
	if index == nil || index.State == "" {
		return indexing{state: "null"}
	}
	return indexing{state: index.State, indexed: index.Projects.Indexed, notIndexed: index.Projects.NotIndexed}
}
