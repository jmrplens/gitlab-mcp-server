# E2E Capability Inventory

This inventory records the highest-risk E2E test patterns and the capability gates that should protect them during the E2E refactor. It is intentionally focused on tests that touch shared state, runner capacity, or MCP capability infrastructure.

| Test Pattern | Resource Scope | Mutates Shared State | Can Run In Parallel | Required Gate | Notes |
| ------------ | -------------- | -------------------- | ------------------- | ------------- | ----- |
| `admin_meta_test.go::TestMeta_AdminTopics` | `instance-global` | Yes | No, serialize global mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | Creates, updates, and deletes instance topic metadata. |
| `admin_meta_test.go::TestMeta_AdminSettingsAppearance` | `instance-global` | Yes | No, serialize global mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | Updates global appearance/settings and needs snapshot/restore. |
| `admin_meta_test.go::TestMeta_AdminBroadcast` | `instance-global` | Yes | No, serialize global mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | Broadcast messages are visible instance-wide. |
| `admin_meta_test.go::TestMeta_AdminFeatures` | `instance-global` | Yes | No, serialize global mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | Feature flag changes can affect unrelated tests. |
| `admin_meta_test.go::TestMeta_AdminSystemHooks` | `instance-global` | Yes | No, serialize global mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | System hooks are instance-level webhooks. |
| `admin_meta_test.go::TestMeta_AdminSidekiqMetrics` | `instance-global` | No | Yes | `CapabilityAdmin` | Read-only admin metrics. |
| `admin_meta_test.go::TestMeta_AdminPlanLimitsMetadata` | `instance-global` | No | Yes | `CapabilityAdmin` | Read-only plan limits, metadata, and statistics checks. |
| `admin_meta_test.go::TestMeta_AdminApplications` | `instance-global` | Yes | No, serialize global mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | OAuth application lifecycle affects instance-global app registry. |
| `admin_meta_test.go::TestMeta_AdminCustomAttributes` | `user`, `instance-global` | Yes | No, serialize admin mutations | `CapabilityAdmin`, `CapabilityInstanceGlobal` | Mutates custom attributes on admin-scoped resources. |
| `users_meta_test.go::TestMeta_UserSelf` | `current-user` | Yes | No, serialize current-user state | `CapabilityCurrentUserState` | Sets authenticated user status and must restore prior state. |
| `users_meta_test.go::TestMeta_UserTodosEvents` | `current-user` | Yes | No, serialize current-user state | `CapabilityCurrentUserState` | Marks all current-user todos done. |
| `users_meta_test.go::TestMeta_UserNamespacesNotifications` | `current-user`, `project`, `group` | Yes | No, serialize current-user notification changes | `CapabilityCurrentUserState` | Updates global and scoped notification settings; group fixture cleanup is required. |
| `users_meta_test.go::TestMeta_UserSSHKeyLifecycle` | `current-user` | Yes | No, serialize current-user state | `CapabilityCurrentUserState` | Creates and deletes SSH keys for the authenticated user. |
| `users_meta_test.go::TestMeta_UserAdmin` | `user`, `instance-global` | Yes | No, serialize admin user lifecycle | `CapabilityAdmin`, `CapabilityInstanceGlobal` | Creates, updates, blocks, deactivates, bans, and deletes users. |
| `users_meta_test.go::TestMeta_UserServiceAccounts` | `current-user`, `user`, `enterprise` | Yes | No, serialize PAT/service-account creation | `CapabilityCurrentUserState`, `CapabilityEnterprise` | Service accounts are EE-only; current-user PAT creation needs cleanup or documented limitations. |
| `projects_meta_test.go::TestMeta_ProjectHooks` | `external-network`, `project` | Yes | Yes, when public webhook URLs are reachable | `CapabilityExternalNetwork` | Creates project webhooks against an external endpoint and expects GitLab URL validation to accept the endpoint. |
| `projectmirrors_test.go::TestMeta_ProjectRemoteMirrors` | `external-network`, `project` | Yes | Yes, when public Git remotes are reachable | `CapabilityExternalNetwork` | Creates push mirror configuration against an external Git remote. |
| `customemoji_test.go::TestIndividual_CustomEmoji` / `TestMeta_CustomEmoji` | `external-network`, `group` | Yes | Yes, when public image URLs are reachable | `CapabilityExternalNetwork` | Creates custom emoji from a public image URL fetched by GitLab. |
| `notifications_test.go::TestMeta_Notifications` | `current-user`, `project` | No | Yes | None | Read-only notification retrieval today; mutation tests belong behind current-user gates. |
| `todos_test.go::TestIndividual_Todos` | `current-user` | Yes | No, serialize current-user state | `CapabilityCurrentUserState` | Marks all current-user todos done through individual tools. |
| `todos_test.go::TestMeta_Todos` | `current-user` | Yes | No, serialize current-user state | `CapabilityCurrentUserState` | Marks all current-user todos done through meta-tools. |
| `pipelines_test.go::TestPipelines` | `project`, `runner` | No | No, keep runner lifecycle serial | `CapabilityRunner` | Intentionally not parallelized because Docker mode uses a shared runner. |
| `wait_test.go::TestWaitTools` | `project`, `runner` | No | Yes, with runner availability | `CapabilityRunner` | Exercises pipeline/job wait tools and can time out on slow runner hosts. |
| `capabilities_test.go::TestCapability_Logging` | `metadata` | No | Yes | None | Read-only MCP logging capability test. |
| `capabilities_test.go::TestCapability_Progress` | `project` | No | Yes | None | Creates a project and verifies progress notifications during upload. |
| `capabilities_test.go::TestCapability_Roots` | `metadata` | No | Yes | None | Verifies roots discovery through MCP resource reads. |
| `capabilities_test.go::TestCapability_RootsListChanged` | `metadata` | No | Yes | None | Verifies roots/list_changed notification handling. |
| `capabilities_test.go::TestCapability_Completions` | `project`, `metadata` | No | Yes | None | Creates a project to provide completion data, then runs parallel read-only subtests. |
| `meta_schema_resource_test.go::TestMetaSchemaResource_ListsTemplate` | `metadata` | No | Yes | None | Verifies meta-schema resource templates. |
| `meta_schema_resource_test.go::TestMetaSchemaResource_ReadMergeRequestCreate` | `metadata` | No | Yes | None | Reads and validates merge-request create schema. |
| `meta_schema_resource_test.go::TestMetaSchemaResource_NotFound` | `metadata` | No | Yes | None | Verifies invalid schema resources fail cleanly. |
| `meta_schema_resource_test.go::TestMetaSchemaResource_IndexEnumeratesMetaTools` | `metadata` | No | Yes | None | Verifies the schema index includes meta-tools. |

## Capability Rules

- `CapabilityInstanceGlobal` should guard tests that create, update, or delete instance-wide resources.
- `CapabilityCurrentUserState` should guard tests that mutate the authenticated user's status, todos, notification preferences, SSH keys, or personal access tokens.
- `CapabilityRunner` should guard tests that need a registered CI runner or can consume shared runner capacity.
- `CapabilityEnterprise` should guard Premium or Ultimate features and skip cleanly on Community Edition.
- `CapabilityExternalNetwork` should guard tests that require GitLab to fetch public URLs or contact public Git remotes. Set `E2E_EXTERNAL_NETWORK=true` only in environments with deterministic outbound access.
- Project-scoped and group-scoped tests can remain parallel when every created resource is test-owned and cleanup is registered.
- Read-only metadata and MCP capability tests can remain parallel unless they create shared GitLab state.
