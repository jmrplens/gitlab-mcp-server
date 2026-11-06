// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// pullRequestTarget is the trigger that runs a workflow from the base branch
// with a token that can write and with the repository's secrets, for a pull
// request from a fork as much as for one from a branch here.
const pullRequestTarget = "pull_request_target"

// eventNameTest matches a condition comparing github.event_name for equality
// with a quoted event name, the one form a workflow here selects an event by.
var eventNameTest = regexp.MustCompile(`github\.event_name\s*==\s*['"]([A-Za-z_]+)['"]`)

// checkTriggers holds a workflow to the events it is triggered by (rule 9).
//
// A job or step condition that compares github.event_name with an event the
// workflow is not triggered by never holds, so the branch it guards is skipped
// and the run still reports success: auto-assign.yml moved to
// pull_request_target while a condition still named pull_request would assign
// no pull request at all, and nothing would turn red.
//
// A workflow triggered by pull_request_target runs with a write token and the
// repository's secrets on a fork's pull request, so it checks out nothing: the
// event payload is all such a job reads. A checkout of the base branch would be
// safe, but no workflow here needs one, and the rule is kept to what can be
// read off the file without resolving a ref.
func checkTriggers(pathLabel string, doc map[string]any) []string {
	events := workflowEvents(doc["on"])
	jobs, _ := doc["jobs"].(map[string]any)
	ids := make([]string, 0, len(jobs))
	for id := range jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var problems []string
	for _, id := range ids {
		job, _ := jobs[id].(map[string]any)
		where := fmt.Sprintf("%s: job %s", pathLabel, id)
		problems = append(problems, untriggeredEvents(where, job["if"], events)...)
		steps, _ := job["steps"].([]any)
		for index, raw := range steps {
			step, _ := raw.(map[string]any)
			stepWhere := fmt.Sprintf("%s step %d", where, index+1)
			problems = append(problems, untriggeredEvents(stepWhere, step["if"], events)...)
			if !events[pullRequestTarget] {
				continue
			}
			if uses, _ := step["uses"].(string); isCheckout(uses) {
				problems = append(problems, fmt.Sprintf("%s checks out code in a workflow triggered by %s, which runs "+
					"with a write token and the repository's secrets on a fork's pull request; such a workflow reads "+
					"only the event payload", stepWhere, pullRequestTarget))
			}
		}
	}
	return problems
}

// workflowEvents returns the events an on: key names, in any of the three
// spellings GitHub accepts: one event, a list of them, or a mapping keyed by
// them.
func workflowEvents(on any) map[string]bool {
	events := map[string]bool{}
	switch value := on.(type) {
	case string:
		events[value] = true
	case []any:
		for _, item := range value {
			if name, ok := item.(string); ok {
				events[name] = true
			}
		}
	case map[string]any:
		for name := range value {
			events[name] = true
		}
	}
	return events
}

// untriggeredEvents reports each event a condition compares github.event_name
// with that the workflow is not triggered by.
func untriggeredEvents(where string, condition any, events map[string]bool) []string {
	text, _ := condition.(string)
	var problems []string
	for _, match := range eventNameTest.FindAllStringSubmatch(text, -1) {
		if events[match[1]] {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s tests github.event_name == '%s', an event the workflow is not "+
			"triggered by, so that branch of the condition never holds and the run still succeeds", where, match[1]))
	}
	return problems
}

// isCheckout reports whether a step's uses: is actions/checkout at any
// reference.
func isCheckout(uses string) bool {
	name, _, _ := strings.Cut(strings.TrimSpace(uses), "@")
	return strings.EqualFold(name, "actions/checkout")
}
