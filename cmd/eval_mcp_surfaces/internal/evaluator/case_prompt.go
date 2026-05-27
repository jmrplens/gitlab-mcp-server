package evaluator

import (
	"bytes"
	"fmt"
	"reflect"
	"text/template"
)

// PromptData exposes typed fixture outputs to case prompt templates.
type PromptData struct {
	Project      PromptProjectData
	Group        PromptGroupData
	Issue        PromptIssueData
	MergeRequest PromptMergeRequestData
	Pipeline     PromptPipelineData
	Job          PromptJobData
	Branch       PromptBranchData
	Tag          PromptTagData
	Release      PromptReleaseData
	Package      PromptPackageData
	Environment  PromptEnvironmentData
	Runner       PromptRunnerData
	Token        PromptTokenData
	DeployKey    PromptDeployKeyData
	Award        PromptAwardData
	Dates        PromptDateData
	Values       FixtureOutput
}

// PromptProjectData contains project fixture values.
type PromptProjectData struct {
	ID            string
	Path          string
	Name          string
	URL           string
	DefaultBranch string
}

// PromptGroupData contains group fixture values.
type PromptGroupData struct {
	ID   string
	Path string
	Name string
}

// PromptIssueData contains issue fixture values.
type PromptIssueData struct {
	ID    string
	IID   string
	Title string
}

// PromptMergeRequestData contains merge request fixture values.
type PromptMergeRequestData struct {
	ID           string
	IID          string
	Title        string
	SourceBranch string
	TargetBranch string
}

// PromptPipelineData contains pipeline fixture values.
type PromptPipelineData struct {
	ID  string
	IID string
	Ref string
}

// PromptJobData contains job fixture values.
type PromptJobData struct {
	ID     string
	Name   string
	Status string
}

// PromptBranchData contains branch fixture values.
type PromptBranchData struct {
	Name    string
	Default string
}

// PromptTagData contains tag fixture values.
type PromptTagData struct {
	Name string
}

// PromptReleaseData contains release fixture values.
type PromptReleaseData struct {
	TagName string
	Name    string
}

// PromptPackageData contains package fixture values.
type PromptPackageData struct {
	ID   string
	Name string
	Type string
}

// PromptEnvironmentData contains environment fixture values.
type PromptEnvironmentData struct {
	ID   string
	Name string
}

// PromptRunnerData contains runner fixture values.
type PromptRunnerData struct {
	ID string
}

// PromptTokenData contains token fixture values.
type PromptTokenData struct {
	ID   string
	Name string
}

// PromptDeployKeyData contains deploy key fixture values.
type PromptDeployKeyData struct {
	ID    string
	Title string
}

// PromptAwardData contains award emoji fixture values.
type PromptAwardData struct {
	ID   string
	Name string
}

// PromptDateData contains date fixture values.
type PromptDateData struct {
	Today     string
	Tomorrow  string
	ExpiresAt string
}

// RenderCasePrompt renders a typed case prompt from fixture outputs.
func RenderCasePrompt(evalCase EvalCase, output FixtureOutput) (string, error) {
	if evalCase.PromptTemplate.Text == "" {
		return evalCase.Prompt, nil
	}
	tmpl, err := template.New(string(evalCase.ID)).Option("missingkey=error").Parse(evalCase.PromptTemplate.Text)
	if err != nil {
		return "", fmt.Errorf("parse prompt template: %w", err)
	}
	data := promptTemplateDataMap(promptDataFromOutputs(output))
	var rendered bytes.Buffer
	if executeErr := tmpl.Execute(&rendered, data); executeErr != nil {
		return "", fmt.Errorf("render prompt template: %w", executeErr)
	}
	return rendered.String(), nil
}

func promptDataFromOutputs(output FixtureOutput) PromptData {
	return PromptData{
		Project:      PromptProjectData{ID: output["project_id"], Path: output["project_path"], Name: output["project_name"], URL: output["project_url"], DefaultBranch: output["default_branch"]},
		Group:        PromptGroupData{ID: output["group_id"], Path: output["group_path"], Name: output["group_name"]},
		Issue:        PromptIssueData{ID: output["issue_id"], IID: output["issue_iid"], Title: output["issue_title"]},
		MergeRequest: PromptMergeRequestData{ID: output["merge_request_id"], IID: output["merge_request_iid"], Title: output["merge_request_title"], SourceBranch: output["source_branch"], TargetBranch: output["target_branch"]},
		Pipeline:     PromptPipelineData{ID: output["pipeline_id"], IID: output["pipeline_iid"], Ref: output["pipeline_ref"]},
		Job:          PromptJobData{ID: output["job_id"], Name: output["job_name"], Status: output["job_status"]},
		Branch:       PromptBranchData{Name: firstNonEmpty(output["branch_name"], output["feature_branch"]), Default: output["default_branch"]},
		Tag:          PromptTagData{Name: output["tag_name"]},
		Release:      PromptReleaseData{TagName: firstNonEmpty(output["release_tag_name"], output["release_summary_tag"]), Name: output["release_name"]},
		Package:      PromptPackageData{ID: output["package_id"], Name: output["package_name"], Type: output["package_type"]},
		Environment:  PromptEnvironmentData{ID: output["environment_id"], Name: output["environment_name"]},
		Runner:       PromptRunnerData{ID: output["runner_id"]},
		Token:        PromptTokenData{ID: output["token_id"], Name: output["token_name"]},
		DeployKey:    PromptDeployKeyData{ID: output["deploy_key_id"], Title: output["deploy_key_title"]},
		Award:        PromptAwardData{ID: output["award_id"], Name: output["award_name"]},
		Dates:        PromptDateData{Today: output["today"], Tomorrow: output["tomorrow"], ExpiresAt: output["expires_at"]},
		Values:       output,
	}
}

func promptTemplateDataMap(data PromptData) map[string]any {
	out := map[string]any{}
	addPromptData(out, "Project", data.Project)
	addPromptData(out, "Group", data.Group)
	addPromptData(out, "Issue", data.Issue)
	addPromptData(out, "MergeRequest", data.MergeRequest)
	addPromptData(out, "Pipeline", data.Pipeline)
	addPromptData(out, "Job", data.Job)
	addPromptData(out, "Branch", data.Branch)
	addPromptData(out, "Tag", data.Tag)
	addPromptData(out, "Release", data.Release)
	addPromptData(out, "Package", data.Package)
	addPromptData(out, "Environment", data.Environment)
	addPromptData(out, "Runner", data.Runner)
	addPromptData(out, "Token", data.Token)
	addPromptData(out, "DeployKey", data.DeployKey)
	addPromptData(out, "Award", data.Award)
	addPromptData(out, "Dates", data.Dates)
	if len(data.Values) > 0 {
		out["Values"] = map[string]string(data.Values)
	}
	return out
}

func addPromptData(out map[string]any, name string, value any) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return
	}
	for reflected.Kind() == reflect.Pointer {
		if reflected.IsNil() {
			return
		}
		reflected = reflected.Elem()
	}
	if reflected.Kind() != reflect.Struct {
		return
	}
	fields := map[string]string{}
	reflectedType := reflected.Type()
	for i := range reflected.NumField() {
		field := reflected.Field(i)
		if field.Kind() != reflect.String || field.String() == "" {
			continue
		}
		fields[reflectedType.Field(i).Name] = field.String()
	}
	if len(fields) > 0 {
		out[name] = fields
	}
}
