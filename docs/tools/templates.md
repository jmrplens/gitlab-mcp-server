# Templates — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Templates
> **Individual tools**: 10
> **Meta-tool**: `gitlab_template` (when `META_TOOLS=true`, default — also includes CI lint actions from the `cilint` sub-package)
> **GitLab API**: [CI YAML Templates](https://docs.gitlab.com/ee/api/templates/gitlab_ci_ymls.html) · [Dockerfile Templates](https://docs.gitlab.com/ee/api/templates/dockerfiles.html) · [Gitignore Templates](https://docs.gitlab.com/ee/api/templates/gitignores.html) · [License Templates](https://docs.gitlab.com/ee/api/templates/licenses.html) · [Project Templates](https://docs.gitlab.com/ee/api/project_templates.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The templates domain provides access to GitLab's built-in template libraries for CI YAML, Dockerfiles, gitignore files, open-source licenses, and project-level templates. All tools are read-only.

When `META_TOOLS=true` (the default), all 10 template tools are consolidated into a single `gitlab_template` meta-tool. The meta-tool also includes CI lint actions (`lint`, `lint_project`) from the `cilint` sub-package for convenience.

### Common Questions

> "List available CI/CD templates"
> "Show the Docker gitignore template"
> "What Dockerfile templates are available?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

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
| 3 | `gitlab_list_dockerfile_templates` | Dockerfile | Read |
| 4 | `gitlab_get_dockerfile_template` | Dockerfile | Read |
| 5 | `gitlab_list_gitignore_templates` | Gitignore | Read |
| 6 | `gitlab_get_gitignore_template` | Gitignore | Read |
| 7 | `gitlab_list_license_templates` | License | Read |
| 8 | `gitlab_get_license_template` | License | Read |
| 9 | `gitlab_list_project_templates` | Project | Read |
| 10 | `gitlab_get_project_template` | Project | Read |

### Destructive Tools

None — all template tools are read-only.

---

## Related

- [GitLab CI YAML Templates API](https://docs.gitlab.com/ee/api/templates/gitlab_ci_ymls.html)
- [GitLab Dockerfile Templates API](https://docs.gitlab.com/ee/api/templates/dockerfiles.html)
- [GitLab Gitignore Templates API](https://docs.gitlab.com/ee/api/templates/gitignores.html)
- [GitLab License Templates API](https://docs.gitlab.com/ee/api/templates/licenses.html)
- [GitLab Project Templates API](https://docs.gitlab.com/ee/api/project_templates.html)
