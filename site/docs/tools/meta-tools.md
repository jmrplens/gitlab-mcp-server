# Meta-herramientas

Referencia completa de las 40 meta-herramientas base (59 con Enterprise). Cada una agrupa operaciones de un dominio de GitLab bajo el parámetro `action`.

---

## Proyectos y repositorios

### `gitlab_project`

Gestión de proyectos: CRUD, hooks, badges, pages, import/export.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar proyectos accesibles |
| `get` | Obtener detalles de un proyecto |
| `create` | Crear un nuevo proyecto |
| `update` | Actualizar configuración de un proyecto |
| `delete` | Eliminar un proyecto |
| `fork` | Hacer fork de un proyecto |
| `archive` / `unarchive` | Archivar o desarchivar un proyecto |
| `hooks_list` / `hooks_create` | Gestionar webhooks |
| `badges_list` / `badges_create` | Gestionar badges |
| `pages_get` / `pages_delete` | Gestionar GitLab Pages |

??? example "Ejemplos en lenguaje natural"
    - *"Lista mis proyectos"*
    - *"Crea un proyecto llamado 'api-gateway' en el grupo 'backend'"*
    - *"Archiva el proyecto legacy-app"*

### `gitlab_repository`

Operaciones sobre el repositorio: árbol de archivos, contenido, comparaciones, commits.

| Acción | Descripción |
|--------|-------------|
| `tree` | Listar árbol de archivos |
| `file_get` | Obtener metadatos de un archivo |
| `file_raw` | Obtener contenido bruto de un archivo |
| `file_create` / `file_update` / `file_delete` | CRUD de archivos |
| `compare` | Comparar ramas, tags o commits |
| `commit_list` | Listar commits |
| `commit_get` | Obtener detalles de un commit |
| `contributors` | Listar contribuidores |

??? example "Ejemplos en lenguaje natural"
    - *"Muéstrame los archivos en la raíz del proyecto my-app"*
    - *"Muéstrame el contenido de src/main.go"*
    - *"Compara la rama develop con main"*

### `gitlab_branch`

Gestión de ramas y ramas protegidas.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar ramas |
| `get` | Obtener detalles de una rama |
| `create` | Crear una rama nueva |
| `delete` | Eliminar una rama |
| `protect` / `unprotect` | Proteger o desproteger ramas |

### `gitlab_tag`

Gestión de tags y tags protegidos.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar tags |
| `get` | Obtener detalles de un tag |
| `create` | Crear un tag |
| `delete` | Eliminar un tag |

### `gitlab_release`

Releases y assets de release.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar releases |
| `get` | Obtener detalles de una release |
| `create` | Crear una release |
| `update` | Actualizar una release |
| `delete` | Eliminar una release |
| `link_list` / `link_create` / `link_delete` | Gestionar links de assets |

---

## Merge Requests

### `gitlab_merge_request`

Operaciones CRUD sobre merge requests.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar merge requests |
| `get` | Obtener detalles de un MR |
| `create` | Crear un merge request |
| `update` | Actualizar un MR (título, descripción, asignados...) |
| `merge` | Hacer merge de un MR |
| `approve` / `unapprove` | Aprobar o desaprobar un MR |
| `rebase` | Hacer rebase de un MR |
| `pipelines` | Ver pipelines asociados al MR |
| `changes` | Ver archivos cambiados |
| `commits` | Ver commits del MR |

??? example "Ejemplos en lenguaje natural"
    - *"Lista los MR abiertos de my-app"*
    - *"Crea un MR de feature-login a main"*
    - *"Aprueba el MR !15"*
    - *"Haz merge del MR !15 con squash"*

### `gitlab_mr_review`

Revisión de merge requests: notas, discusiones, borradores, cambios.

| Acción | Descripción |
|--------|-------------|
| `notes_list` | Listar comentarios del MR |
| `notes_create` | Añadir un comentario |
| `discussion_list` | Listar discusiones (hilos de revisión) |
| `discussion_create` | Crear una discusión |
| `discussion_resolve` | Resolver o reabrir una discusión |
| `changes_get` | Obtener el diff completo |
| `draft_notes_list` / `draft_notes_create` | Gestionar borradores de notas |

---

## Issues y seguimiento

### `gitlab_issue`

Gestión completa de issues.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar issues (filtros: estado, etiquetas, asignado...) |
| `get` | Obtener detalles de un issue |
| `get_by_id` | Obtener issue por ID global |
| `create` | Crear un issue |
| `update` | Actualizar un issue |
| `delete` | Eliminar un issue |
| `notes_list` / `notes_create` | Comentarios del issue |
| `links_list` / `links_create` | Enlaces entre issues |
| `time_stats` | Estadísticas de tiempo |

??? example "Ejemplos en lenguaje natural"
    - *"Lista los bugs abiertos asignados a mí"*
    - *"Crea un issue 'Optimizar queries' con etiqueta 'performance'"*
    - *"Cierra el issue #42 con comentario 'Resuelto en MR !15'"*

---

## CI/CD

### `gitlab_pipeline`

Pipelines: ejecución, estado, retry, variables.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar pipelines |
| `get` | Obtener detalles de un pipeline |
| `create` | Lanzar un nuevo pipeline |
| `cancel` | Cancelar un pipeline |
| `retry` | Reintentar un pipeline |
| `delete` | Eliminar un pipeline |
| `variables` | Ver variables del pipeline |
| `test_report` | Ver resultados de tests |

### `gitlab_job`

Jobs individuales de CI/CD.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar jobs de un pipeline |
| `get` | Obtener detalles de un job |
| `trace` | Ver el log completo del job |
| `retry` | Reintentar un job |
| `cancel` | Cancelar un job |
| `artifacts` | Descargar artifacts |

??? example "Ejemplos en lenguaje natural"
    - *"¿Por qué falló el último pipeline?"*
    - *"Muéstrame el log del job de tests"*
    - *"Reintenta el job fallido del pipeline #123"*

### `gitlab_pipeline_schedule`

Schedules de pipeline y sus variables.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar schedules |
| `get` | Obtener un schedule |
| `create` / `update` / `delete` | CRUD de schedules |
| `variables` | Gestionar variables del schedule |

### `gitlab_ci_variable`

Variables CI/CD a nivel de proyecto, grupo e instancia.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar variables |
| `get` | Obtener una variable |
| `create` / `update` / `delete` | CRUD de variables |
| `group_list` / `group_create` | Variables de grupo |

### `gitlab_template`

Plantillas de CI/CD, Dockerfile, .gitignore.

| Acción | Descripción |
|--------|-------------|
| `ci_list` / `ci_get` | Plantillas CI/CD |
| `dockerfile_list` / `dockerfile_get` | Plantillas Dockerfile |
| `gitignore_list` / `gitignore_get` | Plantillas .gitignore |

---

## Grupos y usuarios

### `gitlab_group`

Gestión de grupos: miembros, etiquetas, milestones.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar grupos |
| `get` | Obtener detalles de un grupo |
| `create` / `update` / `delete` | CRUD de grupos |
| `members_list` / `members_add` / `members_remove` | Gestionar miembros |
| `labels_list` / `labels_create` | Gestionar etiquetas |
| `milestones_list` / `milestones_create` | Gestionar milestones |

### `gitlab_user`

Usuarios, eventos y preferencias.

| Acción | Descripción |
|--------|-------------|
| `current` | Obtener usuario actual (quién soy) |
| `get` | Obtener un usuario por ID |
| `list` | Listar usuarios |
| `events` | Ver actividad de un usuario |
| `ssh_keys` | Gestionar claves SSH |

---

## Otros dominios

### `gitlab_wiki`

Wikis de proyecto y grupo.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar páginas wiki |
| `get` | Obtener una página |
| `create` / `update` / `delete` | CRUD de páginas |

### `gitlab_environment`

Entornos y periodos de freeze.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar entornos |
| `get` | Obtener un entorno |
| `create` / `update` / `stop` / `delete` | Gestionar entornos |
| `protected_list` / `protected_create` | Entornos protegidos |

### `gitlab_deployment`

Despliegues.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar despliegues |
| `get` | Obtener un despliegue |

### `gitlab_runner`

Runners de CI/CD.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar runners |
| `get` | Obtener detalles de un runner |
| `update` / `delete` | Gestionar runners |
| `jobs` | Ver jobs de un runner |

### `gitlab_package`

Paquetes y container registry.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar paquetes |
| `get` | Obtener un paquete |
| `delete` | Eliminar un paquete |
| `registry_list` | Listar repositorios del registry |
| `registry_tags` | Listar tags de imagen |

### `gitlab_snippet`

Snippets de código.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar snippets |
| `get` | Obtener un snippet |
| `create` / `update` / `delete` | CRUD de snippets |
| `content` | Obtener contenido bruto |

### `gitlab_feature_flags`

Feature flags.

| Acción | Descripción |
|--------|-------------|
| `list` | Listar feature flags |
| `get` | Obtener un flag |
| `create` / `update` / `delete` | CRUD de feature flags |

### `gitlab_search`

Búsqueda global, por proyecto y por grupo.

| Acción | Descripción |
|--------|-------------|
| `global` | Búsqueda en toda la instancia |
| `project` | Búsqueda dentro de un proyecto |
| `group` | Búsqueda dentro de un grupo |

### `gitlab_access`

Tokens de acceso, deploy tokens, deploy keys.

| Acción | Descripción |
|--------|-------------|
| `tokens_list` / `tokens_create` / `tokens_revoke` | Tokens de acceso del proyecto |
| `deploy_tokens_list` / `deploy_tokens_create` | Deploy tokens |
| `deploy_keys_list` / `deploy_keys_create` | Deploy keys |

### `gitlab_admin`

Administración del servidor (requiere permisos de administrador).

| Acción | Descripción |
|--------|-------------|
| `settings_get` / `settings_update` | Configuración del servidor |
| `broadcasts_list` / `broadcasts_create` | Mensajes broadcast |

---

## Meta-herramientas Enterprise

Con `GITLAB_ENTERPRISE=true` se activan 19 meta-herramientas adicionales para funcionalidades Premium/Ultimate de GitLab:

| Meta-herramienta | Dominio |
|------------------|--------|
| `gitlab_epic` | Epics y sub-epics |
| `gitlab_vulnerability` | Gestión de vulnerabilidades |
| `gitlab_compliance` | Frameworks y reportes de compliance |
| `gitlab_iteration` | Iteraciones y cadencias |
| `gitlab_value_stream` | Value stream analytics |
| `gitlab_approval_rule` | Reglas de aprobación avanzadas |
| `gitlab_code_review` | Revisiones de código avanzadas |
| `gitlab_audit_event` | Eventos de auditoría |
| `gitlab_license` | Gestión de licencias |
| `gitlab_push_rule` | Reglas de push |
| `gitlab_merge_train` | Merge trains |
| `gitlab_dora_metrics` | Métricas DORA |

!!! note "Disponibilidad"
    Estas herramientas solo funcionan con instancias GitLab Premium o Ultimate. En instancias Community Edition, las llamadas devolverán errores de la API.
