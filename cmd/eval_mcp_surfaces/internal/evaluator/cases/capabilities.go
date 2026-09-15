package cases

func capabilityDiscoveryEvalCases() []Case {
	return []Case{
		capabilityEvalCase(
			"MS-039", "Inspect the MCP capability bridge for this GitLab MCP server, list MCP resources, then read the unified tools manifest resource `gitlab://tools`.",
			optionalStep(readStep(capabilityListTool, "", nil, nil)),
			readStep(resourceListTool, "", nil, nil),
			readStep(resourceReadTool, "", params("uri"), nil),
		),
		capabilityEvalCase(
			"MS-040", "Discover MCP resources, read the project get schema resource `gitlab://tools/project.get`, then fetch project `my-org/tools/gitlab-mcp-server`.",
			readStep(resourceListTool, "", nil, nil),
			readStep(resourceReadTool, "", params("uri"), nil),
			readStep("gitlab_project", "get", params("project_id"), nil),
		),
		capabilityEvalCase(
			"MS-041", "List MCP prompt templates, then render prompt `my_open_mrs`.",
			readStep(promptListTool, "", nil, nil),
			readStep(promptGetTool, "", params("name"), nil),
		),
		capabilityEvalCase(
			"MS-042", "Request MCP completion for prompt `summarize_open_mrs` argument `project_id` with partial value `my-org`, then render `summarize_open_mrs` for project `my-org/tools/gitlab-mcp-server`.",
			readStep(completionTool, "", params("ref_type", "name", "argument_name", "argument_value"), nil),
			readStep(promptGetTool, "", params("name", "arguments"), nil),
		),

		// MS-ENT-DYN-10 — Enterprise + dynamic. Capability-driven
		// discovery: ask the model to surface Enterprise-only tools
		// via the MCP capability bridge and read the unified manifest
		// for EE-specific entries, then read the project get schema
		// resource to confirm EE action inputs are exposed.
		enterpriseCapabilityEvalCase(
			"MS-ENT-DYN-10", "Surface the Enterprise-only GitLab MCP capabilities: list MCP resources, read the unified tools manifest `gitlab://tools`, and read the project get schema resource `gitlab://tools/project.get` so I can confirm EE action inputs are exposed.",
			readStep(resourceListTool, "", nil, nil),
			readStep(resourceReadTool, "", params("uri"), nil),
			readStep(resourceReadTool, "", params("uri"), nil),
		),
	}
}

func optionalStep(step Step) Step {
	step.OptionalStep = true
	return step
}

// capabilityEvalCase builds a capability-bridge case for any instance.
func capabilityEvalCase(id, prompt string, steps ...Step) Case {
	return capabilityEvalCaseForEdition(editionCE, id, prompt, steps...)
}

// enterpriseCapabilityEvalCase builds one that only a licensed instance can
// run, so the suite gates it when filtering by edition.
//
// Which edition a case needs is a property of what it asks GitLab for, so the
// constructor states it. It used to be read off the case ID, which made the
// identifier load-bearing: renaming a case would have silently moved it to
// another runtime, and the allocation rule in this package's doc comment says
// an ID means nothing but itself.
func enterpriseCapabilityEvalCase(id, prompt string, steps ...Step) Case {
	return capabilityEvalCaseForEdition(editionEnterprise, id, prompt, steps...)
}

func capabilityEvalCaseForEdition(edition, id, prompt string, steps ...Step) Case {
	return Case{
		ID:               id,
		Prompt:           prompt,
		Steps:            steps,
		Edition:          edition,
		Presets:          []string{presetDockerCapabilityDiscovery},
		Partition:        partitionCapabilityFallback,
		CapabilityBridge: true,
		ReportGroup:      partitionCapabilityFallback,
	}
}
