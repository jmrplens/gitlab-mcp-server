# Meta-Tool Anthropic Evaluation

Date: 2026-05-02T21:50:37Z
Mode: Anthropic tool-calling
Model: `claude-sonnet-4-6`
Catalog tools: 48
Runs: 1
Task attempts: 5

Trace artifacts: `plan/metatool-token-schema-research/evals/2026-05-02-anthropic-sonnet-4-6-capability-trace-sample.traces`

## Metrics

| Metric | Value |
| --- | ---: |
| Tool-selection accuracy | 100.0% |
| Action-selection accuracy | 100.0% |
| First-call validation pass rate | 100.0% |
| Schema lookup use rate | 0.0% |
| Repair success rate | 100.0% |
| Destructive safety | 100.0% |
| Final task success proxy | 100.0% |

## API Usage

| Metric | Value |
| --- | ---: |
| Anthropic requests | 7 |
| Tool calls emitted | 8 |
| Input tokens | 112 |
| Output tokens | 641 |
| Cache creation input tokens | 0 |
| Cache read input tokens | 272454 |
| Estimated cost | $0.0917 |
| Pricing source | default Claude Sonnet estimate |

## Fixture Tool Coverage

| Metric | Value |
| --- | ---: |
| Catalog tools | 48 |
| Tools covered by expected steps | 5 |
| Missing tools | 43 |
| Catalog action routes | 1007 |
| Action routes covered by expected steps | 7 |
| Missing action routes | 1000 |

Missing: `gitlab_access`, `gitlab_admin`, `gitlab_attestation`, `gitlab_audit_event`, `gitlab_ci_catalog`, `gitlab_ci_variable`, `gitlab_compliance_policy`, `gitlab_custom_emoji`, `gitlab_dependency`, `gitlab_discover_project`, `gitlab_dora_metrics`, `gitlab_enterprise_user`, `gitlab_environment`, `gitlab_external_status_check`, `gitlab_feature_flags`, `gitlab_geo`, `gitlab_group`, `gitlab_group_scim`, `gitlab_interactive_mr_create`, `gitlab_interactive_project_create`, `gitlab_interactive_release_create`, `gitlab_job`, `gitlab_member_role`, `gitlab_merge_request`, `gitlab_merge_train`, `gitlab_model_registry`, `gitlab_mr_review`, `gitlab_package`, `gitlab_project`, `gitlab_project_alias`, `gitlab_release`, `gitlab_repository`, `gitlab_runner`, `gitlab_search`, `gitlab_security_finding`, `gitlab_server`, `gitlab_snippet`, `gitlab_storage_move`, `gitlab_tag`, `gitlab_template`, `gitlab_user`, `gitlab_vulnerability`, `gitlab_wiki`

## Task Results

| Run | Task | Expected | First final call | Steps | Schema lookup | First pass | Repair | Final success | Calls | Tool calls | Notes |
| ---: | --- | --- | --- | ---: | --- | --- | --- | --- | ---: | ---: | --- |
| 1 | MT-093 | `gitlab_analyze` / `mr_changes` | `gitlab_analyze` / `mr_changes` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |
| 1 | MT-099 | `gitlab_branch` / `delete` | `gitlab_branch` / `delete` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |
| 1 | MT-101 | `gitlab_pipeline` / `delete` | `gitlab_pipeline` / `delete` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |
| 1 | MF-004 | `gitlab_analyze` / `issue_summary` -> `gitlab_issue` / `get` -> `gitlab_issue` / `note_list` | `gitlab_analyze` / `issue_summary` | 3/3 | No | Yes | Yes | Yes | 2 | 3 | step 1 simulation sampling_unsupported_continue: simulated sampling capability unsupported |
| 1 | MF-005 | `gitlab_interactive_issue_create` -> `gitlab_issue` / `create` | `gitlab_interactive_issue_create` | 2/2 | No | Yes | Yes | Yes | 2 | 2 | step 1 simulation elicitation_unsupported_continue: simulated elicitation capability unsupported |
