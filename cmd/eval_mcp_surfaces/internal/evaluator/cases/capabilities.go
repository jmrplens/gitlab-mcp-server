package cases

func capabilityDiscoveryEvalCases() []Case {
	return []Case{
		capabilityEvalCase("MS-039", "Inspect the MCP capability bridge for this GitLab MCP server, list MCP resources, then read the unified tools manifest resource `gitlab://tools`.",
			optionalStep(readStep(capabilityListTool, "", nil, nil)),
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

func optionalStep(step Step) Step {
	step.OptionalStep = true
	return step
}

func capabilityEvalCase(id, prompt string, steps ...Step) Case {
	return Case{
		ID:               id,
		Prompt:           prompt,
		Steps:            steps,
		Edition:          editionCE,
		Presets:          []string{presetDockerCapabilityDiscovery},
		Partition:        partitionCapabilityFallback,
		CapabilityBridge: true,
		ReportGroup:      partitionCapabilityFallback,
	}
}
