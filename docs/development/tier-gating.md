# Tier Gating

**How the licensing tier decides which actions, tools and schema fields a session is served**: the detection order, the field-level pruning, and what each surface gates at Premium and Ultimate.

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

How the tier is detected is described under `GITLAB_MCP_TIER` in [Environment Variables and Flags](configuration.md), and the tier of every action is printed on its group's page of the generated [tool reference](https://docs.jmrp.io/gitlab-mcp-server/reference/tools/).

---

`GITLAB_MCP_TIER` controls access to GitLab Premium/Ultimate features in stdio mode (Enterprise tools are gated when the resolved tier is Premium or Ultimate). In HTTP mode, the `--tier` flag forces the tier; when omitted, it is detected per token+URL pool entry by the steps `GITLAB_MCP_TIER` describes in [Environment Variables and Flags](configuration.md): the instance license, then the namespace plans, then on GitLab.com the top-level groups the token is a member of, falling back to `free`. The catalog effect is the same in individual and meta-tool modes.

The tier affects tool registration (input/output schemas and tool lists) through `pruneSchemaFieldsByTier` in `internal/tools/action_catalog.go`: every registered action has its input schema pruned strictly (lower-tier clients never see higher-tier input fields, even though the SDK type still carries them) and its output schema pruned leniently (`lenientExtra=true`: higher-tier output fields are kept but omitted from the model-facing schema, so a Premium client reading an Ultimate response sees the data; an Ultimate client reading a Premium response does not). The 3-tier field-level gating is described per-field with `tier:"premium"` / `tier:"ultimate"` struct tags throughout `internal/tools/*/action_specs.go` and `internal/tools/*/shapes.go`.

**Individual mode** (`GITLAB_MCP_TOOL_SURFACE=individual`) gates the same actions through the same catalog metadata, one tool each: every action of the dedicated groups and the paid actions of the seven base groups listed under meta-tool mode below. An action's minimum tier is its `ActionSpec.Edition` (`premium` or `ultimate`), set by the domain package or, for a whole package, by `editionTaggedSpecs` in `internal/tools/action_specs.go`; an empty one is Free. The generated tool reference states the tier of every action on its group's page, measured by building the catalog at each tier. Group releases and group service accounts are Free.

**Meta-tool mode** (`GITLAB_MCP_TOOL_SURFACE=meta`) gates 17 dedicated Enterprise/Premium catalog groups:

- gitlab_merge_train, gitlab_audit_event, gitlab_dora_metrics, gitlab_dependency, gitlab_external_status_check, gitlab_group_scim, gitlab_member_role, gitlab_enterprise_user, gitlab_attestation, gitlab_compliance_policy, gitlab_project_alias, gitlab_geo, gitlab_vulnerability, gitlab_security_attribute, gitlab_security_category, gitlab_security_finding, gitlab_security_scan_profile

and an eighteenth on GitLab.com alone, `gitlab_orbit` (Premium, contributed by `buildOrbitActionSpecs` only for a GitLab.com client).

Plus paid actions that join 7 base meta-tools; each action listed is served from the tier named, and the rest of its group is Free:

- `gitlab_project` → approval_config_*, approval_rule_*, push_rule_*, pull_mirror_*, start_mirroring, target_branch_rule_*, dependency_firewall_evaluate (Premium); security_settings_* (Ultimate). The push mirror routes, mirror_*, are Free
- `gitlab_group` → epics with their issues, notes, discussions, links, boards and label events; wikis; protected branches and environments; webhooks (hook_*); push rules; LDAP and SAML links and SAML users; provisioned users; billable members; SSH certificates; the analytics counts; the milestone burndown; creating and deleting a group board (Premium); credentials (credential_*) and security_settings_update (Ultimate). Group releases and service accounts are Free, and group iterations are served by `gitlab_issue`
- `gitlab_issue` → project and group iterations (iteration_list_project, iteration_list_group) and the iteration and weight resource events (Premium)
- `gitlab_merge_request` → approval rules, approval state and the project and group approval settings, and merge request dependencies (Premium)
- `gitlab_environment` → protected environments, protected_* (Premium)
- `gitlab_storage_move` → group repository storage moves, the *_group actions (Premium); project and snippet moves are Free
- `gitlab_runner` → runner controllers with their scopes and tokens, controller_* (Ultimate)

**Dynamic mode** (default): the same catalog gating applies: `gitlab_find_action` never lists and `gitlab_execute_action` never runs an action above the resolved tier, so the tier changes which `domain.action` IDs exist rather than which tools are registered. Execute answers such an action as withheld rather than unknown, naming the tier it needs and the tier the session was built for, and asks the operator to pin the tier (issue 956); an action the operator excluded by name stays unknown whatever its tier.
