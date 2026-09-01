# Templates — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Templates
> **Individual tools**: 12
> **Meta-tool**: `gitlab_template` (`TOOL_SURFACE=meta` catalog — also includes CI lint actions from the `cilint` sub-package)
> **Dynamic IDs**: `ci_catalog.*`, `pipeline.*`, `repository.*`, `template.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [CI YAML Templates](https://docs.gitlab.com/ee/api/templates/gitlab_ci_ymls.html) · [CI Lint API](https://docs.gitlab.com/ee/api/lint.html) · [Dockerfile Templates](https://docs.gitlab.com/ee/api/templates/dockerfiles.html) · [Gitignore Templates](https://docs.gitlab.com/ee/api/templates/gitignores.html) · [License Templates](https://docs.gitlab.com/ee/api/templates/licenses.html) · [Project Templates](https://docs.gitlab.com/ee/api/project_templates.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The templates domain provides access to GitLab's built-in template libraries for CI YAML, Dockerfiles, gitignore files, open-source licenses, and project-level templates. The domain also exposes CI lint actions that validate `.gitlab-ci.yml` content (inline or committed) without executing pipelines. All tools are read-only.

On the default dynamic surface, these operations are the `ci_catalog.*`, `pipeline.*`, `repository.*`, `template.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `TOOL_SURFACE=meta`, all 12 template tools are consolidated into a single `gitlab_template` meta-tool. The meta-tool also includes CI lint actions (`lint`, `lint_project`) from the `cilint` sub-package for convenience.

### Common Questions

> "List available CI/CD templates"
> "Show the Docker gitignore template"
> "What Dockerfile templates are available?"
> "Validate this .gitlab-ci.yml snippet against project 42"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description              |
| ---------- | :------: | :---------: | :--------: | ------------------------ |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation |

---

## CI YAML Templates

### `gitlab_list_ci_yml_templates`

List all available GitLab CI YAML templates. Returns key and name for each template.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_ci_yml_template`

Get a single GitLab CI YAML template by key. Returns the template name and content.

| Annotation | **Read** |
| ---------- | -------- |

---

## CI Lint

Validate `.gitlab-ci.yml` content against a project's namespace without committing it, or validate a project's already-committed CI configuration at a branch or tag. Both tools are read-only and return the validity flag, errors, warnings, the merged YAML, and the resolved includes. Set `dry_run` to simulate pipeline creation, and `include_jobs` to include the expanded job list in the response.

### `gitlab_ci_lint`

Validate inline `.gitlab-ci.yml` content within a project namespace. Required parameters: `project_id` (project ID or URL-encoded path used as namespace context), `content` (the CI/CD YAML to validate), `ref` (branch or tag used to resolve CI includes), `dry_run` (boolean; run pipeline creation simulation), and `include_jobs` (boolean; include the expanded job list in the response). Returns: `valid`, `errors`, `warnings`, `merged_yaml`, `includes`, and `next_steps`. See also: `gitlab_ci_lint_project`, `gitlab_pipeline_create`, `gitlab_list_catalog_resources`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_ci_lint_project`

Validate a project's committed `.gitlab-ci.yml` at a branch or tag. Required parameters: `project_id` (project ID or URL-encoded path), `content_ref` (branch or tag whose committed CI configuration is validated), `ref` (branch or tag used to resolve CI includes), `dry_run_ref` (branch or tag used as the context for the dry run), `dry_run` (boolean; run pipeline creation simulation), and `include_jobs` (boolean; include the expanded job list in the response). Returns: `valid`, `errors`, `warnings`, `merged_yaml`, `includes`, and `next_steps`. See also: `gitlab_ci_lint`, `gitlab_pipeline_create`, `gitlab_file_get`.

| Annotation | **Read** |
| ---------- | -------- |

---

## Dockerfile Templates

### `gitlab_list_dockerfile_templates`

List all available Dockerfile templates.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_dockerfile_template`

Get a single Dockerfile template by key.

| Annotation | **Read** |
| ---------- | -------- |

---

## Gitignore Templates

### `gitlab_list_gitignore_templates`

List all available gitignore templates.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_gitignore_template`

Get a single gitignore template by key.

| Annotation | **Read** |
| ---------- | -------- |

---

## License Templates

### `gitlab_list_license_templates`

List all available open-source license templates. Optionally filter by popular.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_license_template`

Get a single license template by key. Optionally substitute project name and full name.

| Annotation | **Read** |
| ---------- | -------- |

---

## Project Templates

### `gitlab_list_project_templates`

List project templates of a given type (dockerfiles, gitignores, gitlab_ci_ymls, licenses).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_template`

Get a single project template by type and key.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_ci_yml_templates` | CI YAML | Read |
| 2 | `gitlab_get_ci_yml_template` | CI YAML | Read |
| 3 | `gitlab_ci_lint` | CI Lint | Read |
| 4 | `gitlab_ci_lint_project` | CI Lint | Read |
| 5 | `gitlab_list_dockerfile_templates` | Dockerfile | Read |
| 6 | `gitlab_get_dockerfile_template` | Dockerfile | Read |
| 7 | `gitlab_list_gitignore_templates` | Gitignore | Read |
| 8 | `gitlab_get_gitignore_template` | Gitignore | Read |
| 9 | `gitlab_list_license_templates` | License | Read |
| 10 | `gitlab_get_license_template` | License | Read |
| 11 | `gitlab_list_project_templates` | Project | Read |
| 12 | `gitlab_get_project_template` | Project | Read |

### Destructive Tools

None — all template tools are read-only.

---

## Related

- [GitLab CI YAML Templates API](https://docs.gitlab.com/ee/api/templates/gitlab_ci_ymls.html)
- [GitLab CI Lint API](https://docs.gitlab.com/ee/api/lint.html)
- [GitLab Dockerfile Templates API](https://docs.gitlab.com/ee/api/templates/dockerfiles.html)
- [GitLab Gitignore Templates API](https://docs.gitlab.com/ee/api/templates/gitignores.html)
- [GitLab License Templates API](https://docs.gitlab.com/ee/api/templates/licenses.html)
- [GitLab Project Templates API](https://docs.gitlab.com/ee/api/project_templates.html)
