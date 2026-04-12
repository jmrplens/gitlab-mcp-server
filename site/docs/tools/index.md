# Herramientas

Gitlab MCP expone las operaciones de GitLab como herramientas MCP que los asistentes de IA invocan automáticamente. No necesitas llamar a las herramientas directamente — simplemente describe lo que necesitas en lenguaje natural.

---

## Tipos de herramientas

| Tipo | Cantidad | Descripción |
|------|----------|-------------|
| **Meta-herramientas** | 40 (59 Enterprise) | Operaciones CRUD agrupadas por dominio GitLab |
| **Herramientas individuales** | 1004 | Una herramienta por operación (alternativa a meta-herramientas) |
| **Herramientas de análisis** | 11 | Análisis inteligentes que usan el LLM vía sampling |
| **Herramientas de elicitación** | 4 | Asistentes paso a paso para crear recursos |
| **Recursos MCP** | 24 | Datos de solo lectura (proyectos, issues, pipelines...) |
| **Prompts MCP** | 38 | Plantillas optimizadas (code review, standup, analytics...) |

---

## ¿Cómo funcionan las meta-herramientas?

Cada meta-herramienta agrupa operaciones relacionadas bajo un parámetro `action`. Por ejemplo, `gitlab_project` maneja todas las operaciones de proyectos:

```text
Usuario: "Lista mis proyectos"
→ El LLM elige: gitlab_project con action="list"

Usuario: "Dame info del proyecto my-app"
→ El LLM elige: gitlab_project con action="get", project_id="my-app"

Usuario: "Crea un proyecto llamado new-api"
→ El LLM elige: gitlab_project con action="create", name="new-api"
```

Tú solo hablas en lenguaje natural. El asistente de IA elige la herramienta y acción correctas automáticamente.

!!! info "¿Por qué meta-herramientas?"
    Los modelos de lenguaje tienen un límite de tokens en su contexto. Con 40 meta-herramientas en lugar de 1004 individuales, el modelo tiene más espacio para razonar y responde más rápido.

---

## Herramientas individuales (alternativa)

Con la variable `META_TOOLS=false`, cada operación se registra como herramienta independiente. En lugar de:

- `gitlab_project` → `action: "list"`

Se registran herramientas separadas:

- `gitlab_list_projects`
- `gitlab_get_project`
- `gitlab_create_project`
- `gitlab_update_project`
- `gitlab_delete_project`
- ... (y así para cada operación de cada dominio)

!!! warning "Solo para casos específicos"
    Las meta-herramientas cubren la misma funcionalidad con menor consumo de tokens. Usa herramientas individuales solo si tu cliente MCP no soporta bien el parámetro `action` o tienes un caso de uso concreto.

---

## Siguientes páginas

- **[Meta-herramientas](meta-tools.md)** — referencia completa con todas las acciones por dominio
- **[Herramientas de análisis](analysis.md)** — análisis inteligentes asistidos por IA
- **[Recursos y prompts](resources-prompts.md)** — datos de solo lectura y plantillas
