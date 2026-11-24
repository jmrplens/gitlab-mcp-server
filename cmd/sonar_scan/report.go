// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxAnswer bounds what is read of one answer.
const maxAnswer = 8 << 20

// metricKeys are the measures printed under the gate.
const metricKeys = "new_coverage,new_duplicated_lines_density,new_code_smells,coverage,duplicated_lines_density"

// get asks SonarCloud for address as the token's user, and decodes its answer.
func (s *session) get(ctx context.Context, read settings, address string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return err
	}
	request.SetBasicAuth(read.token, "")
	response, err := s.client.Do(request) //#nosec G704 -- the host the maintainer names, or the one sonar-scanner reports
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s answered %s", request.URL.Path, response.Status)
	}
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer+1))
	if err != nil {
		return fmt.Errorf("%s could not be read: %w", request.URL.Path, err)
	}
	if len(answer) > maxAnswer {
		return fmt.Errorf("%s answered more than %d bytes", request.URL.Path, maxAnswer)
	}
	if err = json.Unmarshal(answer, into); err != nil {
		return fmt.Errorf("%s answered no JSON this reads: %w", request.URL.Path, err)
	}
	return nil
}

// answeredWhereAsked keeps a client from following a redirect, which no script
// request did: the token goes to the address it was meant for and no further.
func answeredWhereAsked(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// api is an address of SonarCloud's web API with its query.
func (read settings) api(path string, query url.Values) string {
	return read.host + path + "?" + query.Encode()
}

// shown is a JSON value as a line of the report shows it: a string's text, and
// any other value, an absent one included, as JSON writes it.
func shown(value json.RawMessage) string {
	var text string
	if len(value) == 0 {
		return "null"
	}
	if json.Unmarshal(value, &text) == nil && value[0] == '"' {
		return text
	}
	return string(value)
}

// given reports whether a JSON value is there and is not null.
func given(value json.RawMessage) bool {
	return len(value) > 0 && string(value) != "null"
}

// taskStatus asks what became of a Compute Engine task.
func (s *session) taskStatus(ctx context.Context, read settings, address string) (string, error) {
	var answer struct {
		Task struct {
			Status json.RawMessage `json:"status"`
		} `json:"task"`
	}
	err := s.get(ctx, read, address, &answer)
	return shown(answer.Task.Status), err
}

// gate is the quality gate of a branch's last analysis.
type gate struct {
	ProjectStatus struct {
		Status     json.RawMessage `json:"status"`
		Conditions []struct {
			Status         json.RawMessage `json:"status"`
			MetricKey      json.RawMessage `json:"metricKey"`
			ActualValue    json.RawMessage `json:"actualValue"`
			ErrorThreshold json.RawMessage `json:"errorThreshold"`
		} `json:"conditions"`
	} `json:"projectStatus"`
}

// measured is a measure of a branch's last analysis. One about new code has
// its value in a period, or in the first of several.
type measured struct {
	Metric json.RawMessage `json:"metric"`
	Value  json.RawMessage `json:"value"`
	Period struct {
		Value json.RawMessage `json:"value"`
	} `json:"period"`
	Periods []struct {
		Value json.RawMessage `json:"value"`
	} `json:"periods"`
}

// value is the measure's value, wherever it is.
func (m measured) value() string {
	for _, value := range []json.RawMessage{m.Value, m.Period.Value} {
		if given(value) {
			return shown(value)
		}
	}
	if len(m.Periods) > 0 {
		return shown(m.Periods[0].Value)
	}
	return "null"
}

// reportGate fetches the gate and the measures of the branch, prints them, and
// returns 0 for a gate that is OK and 1 for any other.
func (s *session) reportGate(ctx context.Context, read settings) (int, error) {
	fmt.Fprintln(s.stdout, "==> [4/4] Fetching quality gate result")
	var answer gate
	if err := s.get(ctx, read, read.api("/api/qualitygates/project_status", url.Values{"projectKey": {read.projectKey}, "branch": {read.branch}}), &answer); err != nil {
		return 0, err
	}
	var measures struct {
		Component struct {
			Measures []measured `json:"measures"`
		} `json:"component"`
	}
	if err := s.get(ctx, read, read.api("/api/measures/component", url.Values{"component": {read.projectKey}, "branch": {read.branch}, "metricKeys": {metricKeys}}), &measures); err != nil {
		return 0, err
	}
	status := shown(answer.ProjectStatus.Status)
	var report strings.Builder
	fmt.Fprintf(&report, "\n=============== SonarCloud: %s @ %s ===============\nQuality gate: %s\n-- new-code conditions --\n", read.projectKey, read.branch, status)
	for _, condition := range answer.ProjectStatus.Conditions {
		fmt.Fprintf(&report, "  [%s] %s: %s (error threshold: %s)\n", shown(condition.Status), shown(condition.MetricKey), shown(condition.ActualValue), shown(condition.ErrorThreshold))
	}
	fmt.Fprintln(&report, "-- measures --")
	for _, measure := range measures.Component.Measures {
		fmt.Fprintf(&report, "  %s: %s\n", shown(measure.Metric), measure.value())
	}
	fmt.Fprintf(&report, "Dashboard: %s/summary/new_code?id=%s&branch=%s\n==================================================================\n", read.host, read.projectKey, read.branch)
	fmt.Fprint(s.stdout, report.String())
	if status != "OK" {
		return 1, nil
	}
	return 0, nil
}
