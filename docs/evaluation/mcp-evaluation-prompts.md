# MCP Evaluation Prompts — gitlab-mcp-server

> **Purpose**: Collection of 80+ natural language prompts to evaluate how well an LLM navigates
> the MCP tools exposed by `gitlab-mcp-server`. Each prompt tests a specific path or combination
> of tools. Use them to verify the LLM picks the correct tool(s), parameters, and sequencing.
>
> **GitLab instance**: `https://gitlab.example.com` (self-signed TLS, skip verification)
> **User**: `testuser` (id: 184)
> **Test project**: `my-org/tools/gitlab-mcp-server` (id: 1835)

---

## How to Use

1. Configure the MCP server pointing to `https://gitlab.example.com`
2. Send each prompt to the LLM as-is (or adapt slightly)
3. Observe which MCP tool(s) the LLM invokes and in what order
4. Compare with the **Expected Path** to identify mismatches
5. Document findings in the **Result** column

### Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | LLM chose correct path |
| ⚠️ | LLM chose a suboptimal but working path |
| ❌ | LLM chose wrong tool or failed entirely |
| 🔄 | Multi-step: LLM must chain multiple tools |

---

## 1. User & Authentication (5 prompts)

### P-001: Current user info

| Field | Value |
|-------|-------|
| **Prompt** | "¿Quién soy en GitLab? Dame mi información de usuario." |
| **Expected Path** | `gitlab_user` → action `current` |
| **Expected Data** | username=testuser, id=184, email=<testuser@example.com> |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-002: Get another user's profile

| Field | Value |
|-------|-------|
| **Prompt** | "Dame información del usuario `slopez` en GitLab." |
| **Expected Path** | `gitlab_user` → action `list` with search, or `get` by username |
| **Expected Data** | Returns user profile for slopez |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-003: List all users

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame los primeros 10 usuarios registrados en el GitLab." |
| **Expected Path** | `gitlab_user` → action `list` with per_page=10 |
| **Expected Data** | List of users |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-004: Set user status

| Field | Value |
|-------|-------|
| **Prompt** | "Pon mi estado de GitLab como '🔧 Testing MCP Server' con emoji wrench." |
| **Expected Path** | `gitlab_user` → action `set_status` with message and emoji |
| **Expected Data** | Status updated |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-005: Check SSH keys

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tengo alguna clave SSH registrada en GitLab?" |
| **Expected Path** | `gitlab_user` → action `list_ssh_keys` |
| **Expected Data** | List (possibly empty) of SSH keys |
| **Result** | Resutado correcto, mira observaciones |
| **Error / Observaciones** | el LLM le ha costado encontrarlo, su razonamiento: The tool search didn't return a specific SSH key tool. Let me check the user tool more carefully for SSH key actions, or look for other tools. I don't see a specific SSH key tool. Let me check the user tool's available actions more carefully. |

---

## 2. Project Discovery & Navigation (10 prompts)

### P-010: List projects

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los proyectos más recientes a los que tengo acceso en GitLab." |
| **Expected Path** | `gitlab_project` → action `list` ordered by last_activity_at |
| **Expected Data** | Includes management-pe-project-tools, pe-project-tools, my-project, etc. |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-011: Find project by name

| Field | Value |
|-------|-------|
| **Prompt** | "Busca el proyecto 'gitlab-mcp-server' en GitLab." |
| **Expected Path** | `gitlab_search` → search projects, or `gitlab_project` → list with search |
| **Expected Data** | my-org/tools/gitlab-mcp-server, id=1835 |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-012: Get project details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles del proyecto my-org/tools/gitlab-mcp-server." |
| **Expected Path** | `gitlab_project` → action `get` with project path or ID |
| **Expected Data** | Full project info for id=1835 |
| **Result** | |
| **Error / Observaciones** | En el primer intento no ha enviado todo lo necesario, ha usado gitlab_project pero ha usado project en lugar de project_id y ha tenido esta salida: projectGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id . ha reintentado y ya ha funcionado, debe corregirse para que no ocurra el error en la primera ocasion  |

### P-013: List personal projects

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué proyectos tengo en mi espacio personal de GitLab (usuario testuser)?" |
| **Expected Path** | `gitlab_project` → action `list_user_projects` for user testuser |
| **Expected Data** | onboarding-tasks, informes, filesystem-analytics, code-diff-tool, analisis-rendimiento-modbus-tcp |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-014: Project languages

| Field | Value |
|-------|-------|
| **Prompt** | "¿En qué lenguajes de programación está escrito el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_project` → action `languages` for project 1835 |
| **Expected Data** | Go percentage |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-015: Search projects globally

| Field | Value |
|-------|-------|
| **Prompt** | "Busca todos los proyectos que tengan 'charger' en el nombre." |
| **Expected Path** | `gitlab_search` → search projects with query "charger" |
| **Expected Data** | pe-nb-charger-display, nb_combiner, etc. |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-016: List project members

| Field | Value |
|-------|-------|
| **Prompt** | "¿Quiénes son los miembros del proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_project` → `list_users` or member tools for project 1835 |
| **Expected Data** | mlatorre, gdemiguel, slopez, ahurtado, david.sanz, ablasco, pbalaguer, magarcia, pjordan, ocoba, andreupicornell |
| **Result** | |
| **Error / Observaciones** | El LLM ha respndido inicialmente: "No veo la herramienta de miembros directamente en la búsqueda. Voy a buscar específicamente:Voy a buscar la herramienta de miembros de proyecto:La acción es members en gitlab_project:" despues ya ha usado la herramienta y devuelto los datos correctos |

### P-017: Find project by path

| Field | Value |
|-------|-------|
| **Prompt** | "Obtén la información del proyecto en la ruta engineering/embedded/firmware/my-project" |
| **Expected Path** | `gitlab_project` → action `get` with path `engineering/embedded/firmware/my-project` |
| **Expected Data** | my-project project, id=393 |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-018: Project star

| Field | Value |
|-------|-------|
| **Prompt** | "Dale una estrella al proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_project` → action `star` for project 1835 |
| **Expected Data** | Project starred |
| **Result** | |
| **Error / Observaciones** | ha ñadido la estrella (verificado en web) pero despues ha dicho "Parece que ya tenías una estrella en el proyecto (el conteo sigue en 2). Si ya lo habías destacado antes, GitLab no permite duplicar la estrella. El proyecto gitlab-mcp-server tiene actualmente 2 estrellas ⭐." Como en la web he quitado yo la estrella antes de ejecutar el prompt, puede que no haya refrescado los datos en cache |

### P-019: List project forks

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tiene forks el proyecto pe-nb-charger-display (id 251)?" |
| **Expected Path** | `gitlab_project` → action `list_forks` for project 251 |
| **Expected Data** | Fork list (may be empty) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 3. Groups & Subgroups (7 prompts)

### P-020: List top-level groups

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué grupos de nivel superior existen en el GitLab?" |
| **Expected Path** | `gitlab_group` → action `list` with top_level_only |
| **Expected Data** | Ceedling (347), my-org (2246), engineering (229) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-021: Get group details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles del grupo 'engineering'." |
| **Expected Path** | `gitlab_group` → action `get` with group id 229 or path engineering |
| **Expected Data** | Group engineering, visibility=private |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-022: List subgroups

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué subgrupos tiene el grupo 'pe'?" |
| **Expected Path** | `gitlab_group` → action `subgroups_list` for group pe (2246) |
| **Expected Data** | pe/ai (2220), pe/embedded (8), pe/nwt (4), pe/Testing_Group (41), pe/uP-DSP (39) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-023: List group projects

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame todos los proyectos dentro del grupo engineering/embedded-linux." |
| **Expected Path** | `gitlab_group` → action `projects` for group engineering/embedded-linux |
| **Expected Data** | pe-nb-charger-display, pe_ocpp_manager, etc. |
| **Result** | |
| **Error / Observaciones** | ejecuto gitlab_group, dijo "Necesito el ID del grupo engineering/embedded-linux para listar sus proyectos. Voy a buscarlo:" despues ejecuto gitlab_group con el groupd_id, primero sin paginacion y despues indicando per_page, obtiene datos pero el LLM dice: "Ese resultado no parece correcto, probablemente necesito incluir subgrupos:", entonces vuelve a usar gitlab_group añadiendo el include_subgroups y entonces ya devuelve los datos, deberia entender que si decimos dentro de un grupo, lo que debe buscar son subgrupos no? en lugar de tantos intentos |

### P-024: Search within group

| Field | Value |
|-------|-------|
| **Prompt** | "Busca proyectos con 'valans' dentro del grupo engineering." |
| **Expected Path** | `gitlab_group` → action `search` or `gitlab_search` with group scope |
| **Expected Data** | valans_display_ui, valans_linux_package, valans_display_logic, valans_can_logger |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-025: List group members

| Field | Value |
|-------|-------|
| **Prompt** | "¿Quiénes son los miembros del grupo pe?" |
| **Expected Path** | `gitlab_group` → action `members_list` for group 2246 |
| **Expected Data** | List of group members |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-026: Get subgroups of engineering

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuáles son los subgrupos de engineering?" |
| **Expected Path** | `gitlab_group` → action `subgroups_list` for group 229 |
| **Expected Data** | automation_qa (231), deprecated-projects (1899), doc (348), embedded (223), embedded-linux (230), management (410), tools (248) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 4. Issues (10 prompts)

### P-030: List open issues globally

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame las issues abiertas que puedo ver en todo el GitLab." |
| **Expected Path** | `gitlab_issue` → action `list_all` with state=opened |
| **Expected Data** | Issues from management-xmv670, pe-project-tools-gitlab, pcs-260-common-modules, etc. |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-031: Get specific issue

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles de la issue #1 del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_issue` → action `get` with project=1835, iid=1 |
| **Expected Data** | "GFM Spot-Check \| Pipe and Heading Test", state=closed |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-032: List project issues

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué issues abiertas tiene el proyecto management-xmv670?" |
| **Expected Path** | `gitlab_issue` → action `list` for project 1733, state=opened |
| **Expected Data** | Issues 34-38 about internal issues, EIP trips, etc. |
| **Result** | |
| **Error / Observaciones** | Primero ha intentado listar y ha obtenido el error: "issueList: unexpected error: 404 Not Found", despues ha reintentado con search y ha obtenido el error: "searchProjects: query is required" porque ha enviado la clave search con lo que queria buscar, ha vuelto a intentar usando la clave query y entonces ha obtenido que "El proyecto management-xmv670 (ID 1733, engineering/management-project) tiene las issues deshabilitadas (issues_enabled: false), por lo que no es posible listar ni crear issues en él."   |

### P-033: Create an issue 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una issue en mi proyecto personal 'onboarding-tasks' con título 'Test MCP: verificar herramientas de issues' y descripción 'Issue creada automáticamente para validar el MCP server'." |
| **Expected Path** | `gitlab_issue` → action `create` with project=1064 (or resolve by path testuser/onboarding-tasks) |
| **Expected Data** | New issue created with given title and description |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-034: Update an issue 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Actualiza la issue que acabas de crear en onboarding-tasks: añade la etiqueta 'test' y asígnala a testuser." |
| **Expected Path** | `gitlab_issue` → action `update` with labels and assignee |
| **Expected Data** | Issue updated |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-035: Close an issue

| Field | Value |
|-------|-------|
| **Prompt** | "Cierra la issue de test que creamos en onboarding-tasks." |
| **Expected Path** | `gitlab_issue` → action `update` with state_event=close |
| **Expected Data** | Issue closed |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-036: Search issues

| Field | Value |
|-------|-------|
| **Prompt** | "Busca issues que contengan la palabra 'EIP' en todos los proyectos." |
| **Expected Path** | `gitlab_search` → action `issues` with query "EIP" |
| **Expected Data** | Issues from management-xmv670 about EIP faults |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-037: Issue time tracking 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "En la issue #33 del proyecto my-project (id 393), establece una estimación de tiempo de 4 horas." |
| **Expected Path** | `gitlab_issue` → action `time_estimate_set` with project=393, iid=33 |
| **Expected Data** | Time estimate set to 4h |
| **Result** | |
| **Error / Observaciones** | he tenido error indicando "El parámetro time_estimate no está soportado directamente en la acción update de issues. Voy a usar el endpoint específico de time tracking:" despues ha reintentado y lo ha hecho bien, hay que corregir para que lo haga en el primer intento |

### P-038: List issues with filters

| Field | Value |
|-------|-------|
| **Prompt** | "Dame las issues abiertas del proyecto management-xmv670 que contengan 'Internal Issue' en el título." |
| **Expected Path** | `gitlab_issue` → action `list` for project 1733 with search filter |
| **Expected Data** | Issues #38, #36, #35, #34 |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-039: Issue notes/comments 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Añade un comentario a la issue #1 del proyecto gitlab-mcp-server que diga 'Verificación del MCP server completada'." |
| **Expected Path** | `gitlab_issue` → action `note_create` with project=1835, iid=1, body=... |
| **Expected Data** | Comment added |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 5. Labels & Milestones (8 prompts)

### P-040: List project labels

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué etiquetas (labels) tiene el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_issue` → label tools or `gitlab_project` label actions |
| **Expected Data** | type::bug, type::enhancement, type::documentation, priority::critical/high/medium/low, component::tools, status::in-progress, etc. |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-041: Create a label 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una etiqueta llamada 'mcp-test' con color verde (#0e8a16) en el proyecto onboarding-tasks." |
| **Expected Path** | Label create tool for project 1064 |
| **Expected Data** | Label created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-042: List milestones

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué milestones tiene el proyecto gitlab-mcp-server?" |
| **Expected Path** | Milestone list tool for project 1835 |
| **Expected Data** | v1.0.0 (closed, IID 1), Backlog (active, IID 2), v1.1.0 (closed, IID 3) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-043: Get milestone details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles del milestone 'v1.0.0' del proyecto gitlab-mcp-server." |
| **Expected Path** | Milestone get tool with project=1835, milestone_iid=1 |
| **Expected Data** | "Initial stable release — 775 MCP tools, 27 meta-tools, 18 resources, 33 prompts", state=closed |
| **Result** | |
| **Error / Observaciones** | en el primer intento ha enviado milestone_id en lugar de milestone_iid, despues ha reintentado y correcto, hay que corregir lacmomo se expone los parametros necesarios |

### P-044: Create milestone 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea un milestone llamado 'v2.0.0-test' en el proyecto onboarding-tasks con descripción 'Milestone de prueba MCP'." |
| **Expected Path** | Milestone create tool for project 1064 |
| **Expected Data** | Milestone created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-045: Milestone issues

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué issues están asociadas al milestone 'Backlog' (IID 2) del proyecto gitlab-mcp-server?" |
| **Expected Path** | Milestone issues tool for project 1835, milestone_iid=2 |
| **Expected Data** | List of issues (may be empty) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-046: Label with description

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuáles son las etiquetas del tipo 'component::*' en gitlab-mcp-server y qué describe cada una?" |
| **Expected Path** | Label list for project 1835, filtered to component:: labels |
| **Expected Data** | component::tools, component::resources, component::prompts, component::config, etc. with descriptions |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-047: Delete label 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Elimina la etiqueta 'mcp-test' que creamos antes en onboarding-tasks." |
| **Expected Path** | Label delete tool for project 1064, label_name=mcp-test |
| **Expected Data** | Label deleted |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 6. Branches & Tags (8 prompts)

### P-050: List branches

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué ramas tiene el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_branch` → action `list` for project 1835 |
| **Expected Data** | develop (default, protected) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-051: Get branch details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles de la rama 'develop' del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_branch` → action `get` with project=1835, branch=develop |
| **Expected Data** | Latest commit info, protected=true |
| **Result** | |
| **Error / Observaciones** | en el primer intento ha usado la clae brach y project_id, en el segundo la clave name y project_id, en ambas la respuesta ha sido :"branchGet: unexpected error: json: cannot unmarshal array into Go value of type gitlab.Branch". despues ha usado las claves branch_name y project_id y esta vez ya ha obtenido los datos, es necesario revisar porque no sabe que claves usar. |

### P-052: Create branch 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una rama llamada 'feature/mcp-test' en el proyecto onboarding-tasks desde la rama principal." |
| **Expected Path** | `gitlab_branch` → action `create` for project 1064 |
| **Expected Data** | Branch created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-053: List tags

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué tags tiene el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_tag` → action `list` for project 1835 |
| **Expected Data** | v1.1.7, v1.1.6, v1.1.5, v1.1.4, v1.1.3, ... |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-054: Get tag details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame la información del tag v1.1.7 en gitlab-mcp-server." |
| **Expected Path** | `gitlab_tag` → action `get` with project=1835, tag_name=v1.1.7 |
| **Expected Data** | Tag info with commit reference |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-055: List protected branches

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuáles son las ramas protegidas en gitlab-mcp-server?" |
| **Expected Path** | `gitlab_branch` → action `protected_branches_list` for project 1835 |
| **Expected Data** | develop (protected) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-056: Delete branch 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Elimina la rama 'feature/mcp-test' del proyecto onboarding-tasks." |
| **Expected Path** | `gitlab_branch` → action `delete` for project 1064, branch=feature/mcp-test |
| **Expected Data** | Branch deleted |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-057: Create tag 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea un tag llamado 'test-mcp-v0.0.1' en el proyecto onboarding-tasks con el mensaje 'Tag de prueba MCP'." |
| **Expected Path** | `gitlab_tag` → action `create` for project 1064 |
| **Expected Data** | Tag created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 7. Commits & Repository (8 prompts)

### P-060: List recent commits

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame los 5 commits más recientes del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_repository` → commit list, or commit tools for project 1835 |
| **Expected Data** | ddcc2f13 "Merge branch 'feature/sampling-tools-modularization'...", etc. |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-061: Get commit details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles del commit ddcc2f13 en gitlab-mcp-server." |
| **Expected Path** | Commit get tool with project=1835, sha=ddcc2f13 |
| **Expected Data** | Full commit info including author, message, date |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-062: Compare branches/tags

| Field | Value |
|-------|-------|
| **Prompt** | "Compara los tags v1.1.6 y v1.1.7 del proyecto gitlab-mcp-server. ¿Qué cambios hubo?" |
| **Expected Path** | `gitlab_repository` → action `compare` with from=v1.1.6, to=v1.1.7 |
| **Expected Data** | Diff between the two versions |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-063: Repository tree

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame la estructura de archivos del directorio raíz del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_repository` → action `tree` for project 1835 |
| **Expected Data** | cmd/, internal/, docs/, go.mod, README.md, etc. |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-064: Get file content

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame el contenido del archivo VERSION en el proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_repository` → file get/raw for project 1835, path=VERSION |
| **Expected Data** | Version string (e.g., 1.1.7 or similar) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-065: File blame

| Field | Value |
|-------|-------|
| **Prompt** | "¿Quién hizo los últimos cambios en el archivo go.mod del proyecto gitlab-mcp-server?" |
| **Expected Path** | File blame tool for project 1835, path=go.mod |
| **Expected Data** | Blame info showing author(s) and commits |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-066: Repository contributors

| Field | Value |
|-------|-------|
| **Prompt** | "¿Quiénes son los contribuidores del repositorio gitlab-mcp-server?" |
| **Expected Path** | `gitlab_repository` → action `contributors` for project 1835 |
| **Expected Data** | Contributor list with commit counts |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-067: Search code

| Field | Value |
|-------|-------|
| **Prompt** | "Busca en el código del proyecto gitlab-mcp-server donde se define la función 'RegisterAll'." |
| **Expected Path** | `gitlab_search` → action `code` with query "RegisterAll" in project 1835 |
| **Expected Data** | Matches in internal/tools/register.go |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 8. Merge Requests (10 prompts)

### P-070: List merged MRs

| Field | Value |
|-------|-------|
| **Prompt** | "Dame las merge requests ya mergeadas del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_merge_request` → action `list` for project 1835, state=merged |
| **Expected Data** | MR !15 samplingtools modularization, !14 TLS fix, !13 version bump, !12 dot-escape, !11(¡ bump, !10 param naming, !9 milestone IID, !8 staticcheck, !7 position fix, !6 draft notes |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-071: Get MR details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles de la merge request !15 del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_merge_request` → action `get` with project=1835, merge_request_iid=15 |
| **Expected Data** | "feat(samplingtools): modularize sampling tools...", state=merged, source=feature/sampling-tools-modularization |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-072: MR changes/diff

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué archivos se modificaron en la MR !12 del proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_mr_review` → changes/diff for project 1835, merge_request_iid=12 |
| **Expected Data** | Changes in internal/gitlab/client.go and test files |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-073: MR commits

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuántos commits tiene la MR !15 de gitlab-mcp-server?" |
| **Expected Path** | `gitlab_merge_request` → action `commits` for project 1835, merge_request_iid=15 |
| **Expected Data** | List of commits in that MR |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-074: Create MR 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una merge request en el proyecto onboarding-tasks desde la rama 'feature/mcp-test' hacia la rama principal, con título 'Test: MR creada via MCP'." |
| **Expected Path** | `gitlab_merge_request` → action `create` for project 1064 |
| **Expected Data** | MR created |
| **Prerequisite** | Branch feature/mcp-test must exist (see P-052) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-075: List MR pipelines

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tiene pipelines la MR !15 del proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_merge_request` → action `pipelines` for project 1835, merge_request_iid=15 |
| **Expected Data** | Pipeline list (may be empty since project doesn't have CI) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-076: MR related issues

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué issues cierra o está relacionada la MR !10 del proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_merge_request` → action `related_issues` or `issues_closed` for project=1835, merge_request_iid=10 |
| **Expected Data** | Related issues (if any) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-077: MR participants

| Field | Value |
|-------|-------|
| **Prompt** | "¿Quiénes participaron en la MR !14 del proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_merge_request` → action `participants` for project=1835, merge_request_iid=14 |
| **Expected Data** | testuser (author and merger) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-078: MR approval state

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuál es el estado de aprobación de la MR !15 en gitlab-mcp-server?" |
| **Expected Path** | `gitlab_mr_review` → approval actions for project=1835, merge_request_iid=15 |
| **Expected Data** | Approval state info |
| **Result** | |
| **Error / Notes** | The model used `merge_request_iid` and `project_id` with the `approval_state` action and also tried the `approval_rules` action; both returned: "mrApprovalState: unexpected error: 404 Not Found" and "mrApprovalRules: unexpected error: 404 Not Found". The LLM concluded that MR !15 of gitlab-mcp-server has no approval system configured and that MR approvals are a GitLab Premium feature unavailable on this instance. Confirm whether this interpretation is correct, and avoid surfacing a 404 when the MR simply has no enforced approval rules (the MR does have an approval button but no rules requiring approvals before merge). |

### P-079: List global MRs

| Field | Value |
|-------|-------|
| **Prompt** | "Dame las merge requests abiertas asignadas a mí en todo GitLab." |
| **Expected Path** | `gitlab_merge_request` → action `list_global` with state=opened, assignee_id=mine |
| **Expected Data** | List of open MRs assigned to testuser |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 9. Releases (5 prompts)

### P-080: List releases

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué releases tiene el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_release` → action `list` for project 1835 |
| **Expected Data** | v1.1.7, v1.1.6, v1.1.5, v1.1.4, v1.1.3 |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-081: Get latest release

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuál es la última release del proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_release` → action `latest` for project 1835 |
| **Expected Data** | v1.1.7 |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-082: Get release details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles de la release v1.1.5 del proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_release` → action `get` for project 1835, tag_name=v1.1.5 |
| **Expected Data** | Release info with description, assets |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-083: Release links

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué assets tiene la release v1.1.7 de gitlab-mcp-server?" |
| **Expected Path** | `gitlab_release` → action `link_list` for project 1835, tag=v1.1.7 |
| **Expected Data** | Binary links (if any) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-084: Create release 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una release llamada 'v0.0.1-test' en el proyecto onboarding-tasks con tag 'test-mcp-v0.0.1' y descripción 'Release de prueba del MCP server'." |
| **Expected Path** | `gitlab_release` → action `create` for project 1064 |
| **Prerequisite** | Tag test-mcp-v0.0.1 must exist (see P-057) |
| **Expected Data** | Release created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 10. CI/CD - Pipelines & Jobs (6 prompts)

### P-090: List pipelines

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué pipelines tiene el proyecto my-project (id 393)?" |
| **Expected Path** | `gitlab_pipeline` → action `list` for project 393 |
| **Expected Data** | Pipeline 41557 (success), 41556 (canceled), 41554 (canceled) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-091: Get pipeline details

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles del pipeline 41557 del proyecto my-project." |
| **Expected Path** | `gitlab_pipeline` → action `get` for project 393, pipeline_id=41557 |
| **Expected Data** | status=success, ref=refs/merge-requests/2438/head |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-092: Pipeline jobs

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué jobs tiene el pipeline 41557 del proyecto my-project?" |
| **Expected Path** | `gitlab_job` → action `list` or pipeline jobs for project 393, pipeline_id=41557 |
| **Expected Data** | List of jobs in the pipeline |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-093: Latest pipeline

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuál es el último pipeline del proyecto my-project y en qué estado está?" |
| **Expected Path** | `gitlab_pipeline` → action `latest` for project 393 |
| **Expected Data** | Most recent pipeline with status |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-094: Pipeline test report

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tiene resultados de tests el pipeline 41557 del proyecto my-project?" |
| **Expected Path** | `gitlab_pipeline` → action `test_report` for project 393, pipeline_id=41557 |
| **Expected Data** | Test report summary (if available) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-095: Pipeline variables

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué variables tiene el pipeline 41557 de my-project?" |
| **Expected Path** | `gitlab_pipeline` → action `variables` for project 393, pipeline_id=41557 |
| **Expected Data** | Pipeline variables |
| **Result** | |
| **Error / Observaciones** | Hace falta rol maintainer en el repositorio, asi que no se puede obtener pero muestra el json como si se hubiedese prodido un error, mejor si es algo mas semantico, el output del json: "pipelineGetVariables: access denied — your token lacks the required permissions for this operation: GET <https://gitlab.example.com/api/v4/projects/393/pipelines/41557/variables>: 403 {message: 403 Forbidden}" y despues el LLM si que lo entiende y dice: "No se puede acceder a las variables del pipeline #41557 — el token actual no tiene permisos suficientes." y "Error: 403 Forbidden — se requiere rol Maintainer o superior en el proyecto my-project para consultar variables de pipeline. El usuario testuser tiene rol Developer en este proyecto, que no es suficiente para esta operación." |

---

## 11. Project CRUD - Full Lifecycle 🔄 (6 prompts)

### P-100: Create project

| Field | Value |
|-------|-------|
| **Prompt** | "Crea un nuevo proyecto en mi espacio personal (testuser) llamado 'mcp-test-project' con descripción 'Proyecto temporal para testing del MCP server' y visibilidad privada." |
| **Expected Path** | `gitlab_project` → action `create` with name, description, visibility=private in namespace testuser |
| **Expected Data** | Project created in testuser/mcp-test-project |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-101: Update project

| Field | Value |
|-------|-------|
| **Prompt** | "Actualiza la descripción del proyecto mcp-test-project a 'Proyecto de test MCP - actualizado' y activa las issues." |
| **Expected Path** | `gitlab_project` → action `update` with description and issues_enabled |
| **Expected Data** | Project updated |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-102: Create file in project 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea un archivo README.md en el proyecto mcp-test-project con el contenido '# MCP Test Project\n\nProyecto temporal para testing.'." |
| **Expected Path** | `gitlab_repository` → file create for the new project |
| **Expected Data** | File created with commit |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-103: Create wiki page 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una página wiki llamada 'home' en el proyecto mcp-test-project con contenido 'Bienvenido al proyecto de test MCP'." |
| **Expected Path** | `gitlab_wiki` → action `create` for the new project |
| **Expected Data** | Wiki page created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-104: Archive project

| Field | Value |
|-------|-------|
| **Prompt** | "Archiva el proyecto mcp-test-project." |
| **Expected Path** | `gitlab_project` → action `archive` for the created project |
| **Expected Data** | Project archived |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-105: Delete project

| Field | Value |
|-------|-------|
| **Prompt** | "Elimina el proyecto mcp-test-project de mi espacio personal." |
| **Expected Path** | `gitlab_project` → action `delete` for the created project |
| **Expected Data** | Project deleted (or scheduled for deletion) |
| **Result** | |
| **Error / Observaciones** | ha tenido 3 errores consecutivos, primero porque ha intentado delecte sin marcar como para borrar primero, despues lo ha vuelto a intentar dos veces unsando la ruta antigua (aunque tenia tambien el project_id) y como habia cambiado al marcarse como para eliminar, no la encontraba. En cuarto intento lo ha conseguido  |

---

## 12. Search (6 prompts)

### P-110: Search code globally

| Field | Value |
|-------|-------|
| **Prompt** | "Busca en todo el código del GitLab dónde se usa 'RegisterAll'." |
| **Expected Path** | `gitlab_search` → action `code` with query "RegisterAll" |
| **Expected Data** | Results from gitlab-mcp-server and potentially other projects |
| **Result** | |
| **Error / Observaciones** | como en otro caso anterior usando gitlab_search, primero ha usado project_id y search, fallo por que falta query, despues ha usado project_id y query y ya ha obtenido los datos |

### P-111: Search commits

| Field | Value |
|-------|-------|
| **Prompt** | "Busca commits que contengan 'fix' en el mensaje en el proyecto gitlab-mcp-server." |
| **Expected Path** | `gitlab_search` → action `commits` with query "fix" scoped to project 1835 |
| **Expected Data** | Multiple fix commits |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-112: Search merge requests

| Field | Value |
|-------|-------|
| **Prompt** | "Busca merge requests que contengan 'milestone' en todo GitLab." |
| **Expected Path** | `gitlab_search` → action `merge_requests` with query "milestone" |
| **Expected Data** | MR !9 "fix: rename MilestoneID to MilestoneIID..." and others |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-113: Search issues

| Field | Value |
|-------|-------|
| **Prompt** | "Busca issues que contengan 'modbus' en todo el GitLab." |
| **Expected Path** | `gitlab_search` → action `issues` with query "modbus" |
| **Expected Data** | Issues related to modbus from various projects |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-114: Search users

| Field | Value |
|-------|-------|
| **Prompt** | "Busca usuarios que se llamen 'garcia' en el GitLab." |
| **Expected Path** | `gitlab_search` → action `users` or `gitlab_user` → list with search "garcia" |
| **Expected Data** | magarcia and possibly others |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-115: Search projects

| Field | Value |
|-------|-------|
| **Prompt** | "Busca proyectos que contengan 'mcp' en el nombre." |
| **Expected Path** | `gitlab_search` → action `projects` with query "mcp" |
| **Expected Data** | gitlab-mcp-server (1835), redmine-mcp-server (1869) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 13. Multi-Step Workflows 🔄 (10 prompts)

### P-120: Full issue lifecycle

| Field | Value |
|-------|-------|
| **Prompt** | "En mi proyecto onboarding-tasks: (1) crea una issue titulada 'Workflow test', (2) crea un label 'workflow', (3) asigna ese label a la issue, (4) añade un comentario diciendo 'Paso 4 completado', (5) cierra la issue." |
| **Expected Path** | Chain: issue create → label create → issue update (add label) → issue note create → issue update (close) |
| **Expected Data** | Each step completes successfully |
| **Result** | |
| **Error / Observaciones** | solo ha tenido que reintentar a la hora de asignar el label que no ha incluido el issue_iid, el MCP ha devuelto el error: "issueUpdate: issue_iid is required (must be > 0). Ensure you use the exact parameter name 'issue_iid' as documented in the tool description" y al reintentar ya lo ha hecho bien |

### P-121: Branch + Commit + MR workflow

| Field | Value |
|-------|-------|
| **Prompt** | "En el proyecto onboarding-tasks: (1) crea una rama 'feature/workflow-test' desde main, (2) crea un archivo 'test.md' con contenido 'Hello MCP' en esa rama, (3) crea una merge request de esa rama a main con título 'MR de prueba workflow'." |
| **Expected Path** | Chain: branch create → file create (on branch) → MR create |
| **Expected Data** | Branch, file commit, and MR created |
| **Result** | |
| **Error / Observaciones** | Ha tenido error creando la rama porque la rama main no existe, despues ha buscado la defecto que es develop y ya ha podido hacerlo todo (correcto, pero mejor si en lugar de error es algo mas semantico/markdown para el usuario) |

### P-122: Release investigation

| Field | Value |
|-------|-------|
| **Prompt** | "Compara las últimas dos releases de gitlab-mcp-server y dime qué cambios hubo entre ellas. Lista los commits entre ambos tags." |
| **Expected Path** | Chain: release list → get v1.1.7 and v1.1.6 → repository compare v1.1.6..v1.1.7 |
| **Expected Data** | Diff and commit list between v1.1.6 and v1.1.7 |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-123: Project overview report

| Field | Value |
|-------|-------|
| **Prompt** | "Dame un resumen completo del proyecto gitlab-mcp-server: qué lenguajes usa, cuántas branches tiene, cuáles son sus milestones, quiénes son sus contribuidores y cuántas MRs hay mergeadas." |
| **Expected Path** | Chain: project get → languages → branch list → milestone list → contributors → MR list (state=merged) |
| **Expected Data** | Comprehensive project overview |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-124: Cross-project issue search

| Field | Value |
|-------|-------|
| **Prompt** | "Encuentra todas las issues abiertas que tengan que ver con 'charger' en todos los proyectos a los que tengo acceso." |
| **Expected Path** | `gitlab_search` → issues with "charger", or `gitlab_issue` → list_all with search |
| **Expected Data** | Issues from charger-related projects |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-125: MR review preparation

| Field | Value |
|-------|-------|
| **Prompt** | "I need to code-review MR !12 of the gitlab-mcp-server project. Give me everything: description, changed files, diff, commits, and participants." |
| **Expected Path** | Chain: MR get → MR changes → MR commits → MR participants |
| **Expected Data** | Full MR context for review |
| **Result** | |
| **Error / Notes** | Hit the same error in gitlab_merge_request and gitlab_mr_review: the model used `merge_request_id` instead of the correct `merge_request_iid`. Everything else worked. |

### P-126: Milestone tracking

| Field | Value |
|-------|-------|
| **Prompt** | "Para el milestone 'Backlog' del proyecto gitlab-mcp-server, muéstrame las issues asociadas y las MRs vinculadas." |
| **Expected Path** | Chain: milestone get (iid=2) → milestone issues → milestone merge_requests |
| **Expected Data** | Issues and MRs linked to Backlog milestone |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-127: Group exploration deep dive

| Field | Value |
|-------|-------|
| **Prompt** | "Explora el grupo pe/ai: dame sus subgrupos, proyectos, y miembros." |
| **Expected Path** | Chain: group get → subgroups list → group projects → members list for pe/ai (2220) |
| **Expected Data** | Full group hierarchy info |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-128: Pipeline failure analysis

| Field | Value |
|-------|-------|
| **Prompt** | "El pipeline 41556 del proyecto my-project fue cancelado. Dame los detalles del pipeline, sus jobs, y los logs si es posible." |
| **Expected Path** | Chain: pipeline get (393, 41556) → pipeline jobs → job logs/traces |
| **Expected Data** | Pipeline and job details showing canceled state |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-129: Project transfer preparation

| Field | Value |
|-------|-------|
| **Prompt** | "Quiero mover el proyecto 'onboarding-tasks' de mi espacio personal al grupo pe. Primero muéstrame los detalles del proyecto y del grupo destino para confirmar." |
| **Expected Path** | Chain: project get (1064) → group get (pe/2246) → (optional) project transfer |
| **Expected Data** | Project and group details for confirmation |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 14. Wikis (4 prompts)

### P-130: List wiki pages

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tiene páginas wiki el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_wiki` → action `list` for project 1835 |
| **Expected Data** | Empty list (no wikis) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-131: Create wiki 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una página wiki 'Setup Guide' en el proyecto onboarding-tasks con contenido que explique cómo configurar el proyecto." |
| **Expected Path** | `gitlab_wiki` → action `create` for project 1064 |
| **Expected Data** | Wiki page created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-132: Update wiki 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Actualiza la wiki 'Setup Guide' de onboarding-tasks añadiendo una sección de requisitos." |
| **Expected Path** | `gitlab_wiki` → action `update` for project 1064, slug=Setup-Guide |
| **Expected Data** | Wiki updated |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-133: Delete wiki 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Elimina la página wiki 'Setup Guide' del proyecto onboarding-tasks." |
| **Expected Path** | `gitlab_wiki` → action `delete` for project 1064, slug=Setup-Guide |
| **Expected Data** | Wiki deleted |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 15. Templates & Configuration (4 prompts)

### P-140: List gitignore templates

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué plantillas de .gitignore tiene disponibles el GitLab?" |
| **Expected Path** | `gitlab_template` → action list gitignore templates |
| **Expected Data** | List of gitignore templates (Go, Python, C++, etc.) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-141: Get CI YAML template

| Field | Value |
|-------|-------|
| **Prompt** | "Muéstrame la plantilla de CI/CD YAML para Go." |
| **Expected Path** | `gitlab_template` → action get CI YAML template for Go |
| **Expected Data** | .gitlab-ci.yml template for Go projects |
| **Result** | |
| **Error / Observaciones** | En el primer intento ha obtenido el error: "get_ci_yml_template: unexpected error: json: cannot unmarshal array into Go value of type gitlab.CIYMLTemplate" |

### P-142: License templates

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué plantillas de licencia están disponibles? Muéstrame la de MIT." |
| **Expected Path** | `gitlab_template` → list license templates → get MIT template |
| **Expected Data** | MIT license text |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-143: Dockerfile template

| Field | Value |
|-------|-------|
| **Prompt** | "Dame la plantilla de Dockerfile para un proyecto Go." |
| **Expected Path** | `gitlab_template` → action get Dockerfile template |
| **Expected Data** | Go Dockerfile template |
| **Result** | |
| **Error / Observaciones** | He probado dos veces y siempre pone despues de dar la respuesta correcta: "No se devolvió ninguna respuesta.", dice que hay 3 plantillas pero solo muestra la primera, cuando va a mostrar la siguiente parece ese error. Parece que el servidor se habia caido y he tenido que reiniciar VCScode |

---

## 16. Environments & Deployments (3 prompts)

### P-150: List environments

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tiene entornos (environments) configurados el proyecto my-project (393)?" |
| **Expected Path** | `gitlab_environment` → action `list` for project 393 |
| **Expected Data** | Environment list (if any) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-151: List deployments

| Field | Value |
|-------|-------|
| **Prompt** | "¿Hay despliegues (deployments) registrados en el proyecto my-project?" |
| **Expected Path** | `gitlab_environment` → action `deployment_list` for project 393 |
| **Expected Data** | Deployment list (if any) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-152: Environment details

| Field | Value |
|-------|-------|
| **Prompt** | "Si hay entornos en my-project, dame los detalles del primero." |
| **Expected Path** | env list → env get (conditional) |
| **Expected Data** | Environment details or "no environments" |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 17. Todoss & Notifications (3 prompts)

### P-160: List todos

| Field | Value |
|-------|-------|
| **Prompt** | "¿Tengo algún TODO pendiente en GitLab?" |
| **Expected Path** | `gitlab_user` → todo list or todo tools |
| **Expected Data** | Todo list for current user |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-161: Create todo from issue 🔄

| Field | Value |
|-------|-------|
| **Prompt** | "Crea un TODO para mí a partir de la issue #33 del proyecto my-project." |
| **Expected Path** | `gitlab_issue` → action `create_todo` for project 393, iid=33 |
| **Expected Data** | Todo created |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-162: Notification settings

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuáles son mis configuraciones de notificación en GitLab?" |
| **Expected Path** | Notification settings tool |
| **Expected Data** | Current notification level and settings |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 18. Admin & Instance (4 prompts)

### P-170: GitLab version

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué versión de GitLab está corriendo en gitlab.example.com?" |
| **Expected Path** | Health/version tool or metadata tool |
| **Expected Data** | GitLab version info |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-171: Instance statistics

| Field | Value |
|-------|-------|
| **Prompt** | "Dame las estadísticas generales de la instancia de GitLab." |
| **Expected Path** | `gitlab_admin` → statistics or app_statistics tool |
| **Expected Data** | Total projects, users, groups counts |
| **Result** | |
| **Error / Observaciones** | Como el token no es de admin se recibe error, mejor si es algo mas orgnico en lugar de error normal, aparece: "get_application_statistics: access denied — your token lacks the required permissions for this operation: GET <https://gitlab.example.com/api/v4/application/statistics>: 403 {message: 403 Forbidden}" |

### P-172: List namespaces

| Field | Value |
|-------|-------|
| **Prompt** | "¿Qué namespaces existen en el GitLab?" |
| **Expected Path** | `gitlab_admin` → namespace list tool |
| **Expected Data** | List of namespaces (users, groups) |
| **Result** | |
| **Error / Observaciones** | Ahora no ha mostrado error por no ser admin, entiendo porque tiene el contexto edel prompt anterior |

### P-173: Broadcast messages

| Field | Value |
|-------|-------|
| **Prompt** | "¿Hay algún mensaje de broadcast activo en el GitLab?" |
| **Expected Path** | `gitlab_admin` → broadcast messages list |
| **Expected Data** | Active broadcast messages (if any) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 19. Edge Cases & Error Handling (5 prompts)

### P-180: Non-existent project

| Field | Value |
|-------|-------|
| **Prompt** | "Dame los detalles del proyecto 'no-existe/fake-project'." |
| **Expected Path** | `gitlab_project` → get → 404 error |
| **Expected Data** | Clear error message: project not found |
| **Result** | |
| **Error / Observaciones** | Muestra jsoninput output con error raw: "projectGet: unexpected error: 404 Not Found" |

### P-181: Unauthorized action

| Field | Value |
|-------|-------|
| **Prompt** | "Elimina el grupo 'engineering' del GitLab." |
| **Expected Path** | `gitlab_group` → delete → 403 forbidden (user doesn't have admin) |
| **Expected Data** | Permission denied error, clear message |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-182: Invalid parameters

| Field | Value |
|-------|-------|
| **Prompt** | "Crea una issue en el proyecto gitlab-mcp-server sin título." |
| **Expected Path** | `gitlab_issue` → create → validation error |
| **Expected Data** | Error indicating title is required |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-183: Large result pagination

| Field | Value |
|-------|-------|
| **Prompt** | "Dame la lista completa de TODOS los miembros del grupo engineering, incluyendo miembros heredados." |
| **Expected Path** | Group members list with pagination, possibly with inherited members |
| **Expected Data** | Paginated results with has_more indicator |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-184: Ambiguous project reference

| Field | Value |
|-------|-------|
| **Prompt** | "Dame las issues del proyecto 'pe-project-tools'." |
| **Expected Path** | LLM should handle ambiguity — there are multiple projects with similar names (id 932 and 1706) |
| **Expected Data** | LLM should ask for clarification or pick the most relevant match |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## 20. Natural Language Comprehension (5 prompts)

### P-190: Implicit tool selection

| Field | Value |
|-------|-------|
| **Prompt** | "¿Cuántas líneas de código Go tiene el proyecto gitlab-mcp-server?" |
| **Expected Path** | `gitlab_project` → languages (gives percentages, not lines) — LLM should explain this limitation |
| **Expected Data** | Language percentages, with explanation that exact line count isn't available via API |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-191: Complex natural query

| Field | Value |
|-------|-------|
| **Prompt** | "Quiero saber quién ha sido más activo en gitlab-mcp-server últimamente. ¿Quién ha hecho más commits y MRs en la última semana?" |
| **Expected Path** | Chain: commit list (since date) → MR list (created_after) → aggregate |
| **Expected Data** | Activity summary (likely testuser dominates) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-192: Cross-domain query

| Field | Value |
|-------|-------|
| **Prompt** | "En el proyecto my-project, dame un resumen que incluya: issues abiertas, último pipeline, ramas activas y contribuidores." |
| **Expected Path** | Chain: issues list (opened) → pipeline latest → branch list → contributors |
| **Expected Data** | Multi-domain summary of my-project |
| **Result** | |
| **Error / Observaciones** | al usar la accion gitlab_pipeline con accion latest, se produce el error: "pipelineGetLatest: access denied — your token lacks the required permissions for this operation: GET <https://gitlab.example.com/api/v4/projects/393/pipelines/latest>: 403 {message: 403 Forbidden}". Si se uda la accion list si que obtiene datos. El resto ha ido bien |

### P-193: Conditional logic

| Field | Value |
|-------|-------|
| **Prompt** | "Si el proyecto gitlab-mcp-server tiene alguna MR abierta, muéstrame sus detalles. Si no, dime cuántas MRs mergeadas tiene." |
| **Expected Path** | MR list (state=opened) → if empty → MR list (state=merged) → count |
| **Expected Data** | Either open MRs details or count of merged MRs (15 merged) |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

### P-194: Comparative analysis

| Field | Value |
|-------|-------|
| **Prompt** | "Compara los proyectos gitlab-mcp-server y redmine-mcp-server: ¿cuál tiene más actividad reciente? Mira commits, MRs y issues de ambos." |
| **Expected Path** | Chain: (project get × 2) → (commit list × 2) → (MR list × 2) → compare |
| **Expected Data** | Comparison of activity between both MCP projects |
| **Result** | |
| **Error / Observaciones** | _Rellenar solo si hay error o comportamiento inesperado_ |

---

## Results Summary

| Section | Total | ✅ | ⚠️ | ❌ | Notes |
|---------|------:|:--:|:--:|:--:|-------|
| 1. User & Auth | 5 | | | | |
| 2. Projects | 10 | | | | |
| 3. Groups | 7 | | | | |
| 4. Issues | 10 | | | | |
| 5. Labels & Milestones | 8 | | | | |
| 6. Branches & Tags | 8 | | | | |
| 7. Commits & Repository | 8 | | | | |
| 8. Merge Requests | 10 | | | | |
| 9. Releases | 5 | | | | |
| 10. CI/CD | 6 | | | | |
| 11. Project CRUD | 6 | | | | |
| 12. Search | 6 | | | | |
| 13. Multi-Step Workflows | 10 | | | | |
| 14. Wikis | 4 | | | | |
| 15. Templates | 4 | | | | |
| 16. Environments | 3 | | | | |
| 17. Todos & Notifications | 3 | | | | |
| 18. Admin & Instance | 4 | | | | |
| 19. Edge Cases | 5 | | | | |
| 20. Natural Language | 5 | | | | |
| **TOTAL** | **127** | | | | |

---

## Findings & Improvements

Use this section to document issues discovered during evaluation:

### Tool Discovery Issues

| # | Prompt | Issue | Severity | Fix |
|---|--------|-------|----------|-----|
| | | | | |

### Parameter Naming Issues

| # | Prompt | Expected Param | Sent Param | Tool |
|---|--------|---------------|------------|------|
| | | | | |

### Routing / Path Issues

| # | Prompt | Expected Tool | Chosen Tool | Impact |
|---|--------|--------------|-------------|--------|
| | | | | |

### Error Handling Issues

| # | Prompt | Error Type | Message Quality | Improvement |
|---|--------|-----------|-----------------|-------------|
| | | | | |

### Missing Capabilities

| # | Prompt | What Was Missing | Suggested Addition |
|---|--------|-----------------|-------------------|
| | | | |

### Pruebas separadas que tambien hay que revisar y corregir

---

Cuando digo que borre definitivamente un proyecto lo primero que intenta es borrar normal pero el proyecto ya estaba marcado como borrado y entonces genera error, el LLM envia:
{
  "action": "delete",
  "params": {
    "project_id": 1883
  }
}
Y la respuesta es
projectDelete: bad request — check your input parameters: DELETE <https://gitlab.example.com/api/v4/projects/1883>: 400 {message: Project has already been marked for deletion}

Despues reintenta y ya lo consigue pero deberia en el primer intento confirmar estado para saber que herramienta usar o informar al usuario de que ya esta marcado y que preguntar si se quiere eliminar definitivamente (en nuestro caso hemos dicho en el prompt ue lo haga deifnitivamente, asi que deberia haberlo hecho directamente sin errores.)

---

Cuando pido que habilite las issus en un proyecto, primero el LLM envia:

{
  "action": "update",
  "params": {
    "issues_access_level": "enabled",
    "project_id": 1064
  }
}
Que el mcp devuelve:
projectUpdate: bad request — check your input parameters: PUT <https://gitlab.example.com/api/v4/projects/1064>: 400 {error: allow_merge_on_skipped_pipeline, analytics_access_level, autoclose_referenced_issues, auto_devops_enabled, auto_devops_deploy_strategy, auto_cancel_pending_pipelines, build_git_strategy, build_timeout, builds_access_level, ci_config_path, ci_default_git_depth, ci_allow_fork_pipelines_to_run_in_parent_project, ci_id_token_sub_claim_components, ci_forward_deployment_enabled, ci_forward_deployment_rollback_allowed, ci_separated_caches, container_registry_access_level, container_expiration_policy_attributes, default_branch, description, emails_disabled, emails_enabled, forking_access_level, issues_access_level, lfs_enabled, merge_pipelines_enabled, merge_requests_access_level, merge_requests_template, merge_trains_enabled, merge_method, merge_request_title_regex, merge_request_title_regex_description, name, only_allow_merge_if_all_discussions_are_resolved, only_allow_merge_if_pipeline_succeeds, package_registry_access_level, pages_access_level, path, printing_merge_request_link_enabled, public_builds, public_jobs, remove_source_branch_after_merge, repository_access_level, request_access_enabled, resolve_outdated_diff_discussions, restrict_user_defined_variables, show_diff_preview_in_email, security_and_compliance_access_level, squash_option, shared_runners_enabled, group_runners_enabled, resource_group_default_process_mode, snippets_access_level, tag_list, topics, visibility, wiki_access_level, avatar, suggestion_commit_message, merge_commit_template, squash_commit_template, issue_branch_template, repository_storage, packages_enabled, service_desk_enabled, keep_latest_artifact, mr_default_target_self, enforce_auth_checks_on_uploads, releases_access_level, environments_access_level, feature_flags_access_level, infrastructure_access_level, monitor_access_level, model_experiments_access_level, model_registry_access_level, warn_about_potentially_unwanted_characters, ci_pipeline_variables_minimum_override_role, ci_push_repository_for_job_token_allowed, ci_delete_pipelines_in_seconds, max_artifacts_size, issues_enabled, jobs_enabled, merge_requests_enabled, wiki_enabled, snippets_enabled, container_registry_enabled are missing, at least one parameter must be provided}

Despues reintenta con:
{
  "action": "update",
  "params": {
    "issues_enabled": true,
    "project_id": 1064
  }
}

Y ya lo consigue correctamente, hay que revisar para que no se euivoque en el primer intento

---

He pedido al LLM que borre unos packages y ha recibido forbidden, es de un repositorio mio donde tengo permisos completos y ha podido en el mismo prompt subir packages:

{
  "action": "delete",
  "params": {
    "package_id": 3238,
    "project_id": 1402
  }
}

Devuelve:
packageDelete: access denied — your token lacks the required permissions for this operation: DELETE <https://gitlab.example.com/api/v4/projects/1402/packages/3238>: 403 {message: 403 Forbidden}

---

Parece que el autoupdate no funciona porque el checksums subido es `checksums.sha256` en lugar de .txt que es el que busca, usando gitlab_server_check_update, devuelve: "checking for update: autoupdate: detecting latest release: validation file not found: "checksums.txt""

Despues hace gitlab_admin con:
{
  "action": "metadata_get"
}

Y obtiene:
{"version":"18.9.1","revision":"95bf6656b5a","kas":{"enabled":true,"external_url":"wss://gitlab.example.com/-/kubernetes-agent/","external_k8s_proxy_url":"<https://gitlab.example.com/-/kubernetes-agent/k8s-proxy/","version":"18.9.1"},"enterprise":false}>
{
  "version": "18.9.1",
  "revision": "95bf6656b5a",
  "kas": {
    "enabled": true,
    "external_url": "wss://gitlab.example.com/-/kubernetes-agent/",
    "external_k8s_proxy_url": "<https://gitlab.example.com/-/kubernetes-agent/k8s-proxy/>",
    "version": "18.9.1"
  },
  "enterprise": false
}

Revisa porque se ha subido como .sha256, y corrigelo para que no vuelva a ocurrir en la siguiente version que subiremos

---
