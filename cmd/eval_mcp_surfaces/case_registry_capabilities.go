package main

func capabilityDiscoveryEvalCases() []EvalCase {
	return []EvalCase{
		capabilityEvalCase("MS-039", "Inspect the MCP capability bridge for this GitLab MCP server, list MCP resources, then read the unified tools manifest resource `gitlab://tools`.",
			readStep(capabilityListTool, "", nil, nil),
			readStep(resourceListTool, "", nil, nil),
			readStep(resourceReadTool, "", params("uri"), nil),
		),
		capabilityEvalCase("MS-040", "Discover MCP resources, read the project get schema resource `gitlab://tools/project.get`, then fetch project `my-org/tools/gitlab-mcp-server`.",
			readStep(resourceListTool, "", nil, nil),
			readStep(resourceReadTool, "", params("uri"), nil),
			readStep("gitlab_project", "get", params("project_id"), nil),
		),
		capabilityEvalCase("MS-041", "List MCP prompt templates, then render prompt `my_open_mrs`.",
			readStep(promptListTool, "", nil, nil),
			readStep(promptGetTool, "", params("name"), nil),
		),
		capabilityEvalCase("MS-042", "Request MCP completion for prompt `summarize_open_mrs` argument `project_id` with partial value `my-org`, then render `summarize_open_mrs` for project `my-org/tools/gitlab-mcp-server`.",
			readStep(completionTool, "", params("ref_type", "name", "argument_name", "argument_value"), nil),
			readStep(promptGetTool, "", params("name", "arguments"), nil),
		),
	}
}

func capabilityEvalCase(id, prompt string, steps ...ExpectedStep) EvalCase {
	return EvalCase{
		ID:               EvalCaseID(id),
		Prompt:           prompt,
		Steps:            steps,
		Edition:          EvalCaseEdition(editionCE),
		Presets:          []EvalPreset{EvalPreset(presetDockerCapabilityDiscovery)},
		Partition:        EvalPartition(partitionCapabilityFallback),
		CapabilityBridge: true,
		ReportGroup:      partitionCapabilityFallback,
	}
}
