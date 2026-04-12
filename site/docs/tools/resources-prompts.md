# Recursos y prompts

Además de las herramientas, Gitlab MCP ofrece **recursos** de solo lectura y **prompts** predefinidos que los clientes MCP pueden utilizar.

---

## Recursos MCP

Los recursos proporcionan datos de solo lectura que los clientes MCP pueden consultar directamente, sin necesidad de invocar herramientas.

### Datos del usuario

| Recurso | Descripción |
|---------|-------------|
| **Usuario actual** | Perfil, preferencias y permisos del usuario autenticado |
| **Grupos del usuario** | Listado de grupos accesibles con roles |

### Datos de proyectos

| Recurso | Descripción |
|---------|-------------|
| **Proyectos** | Listado de proyectos por grupo o usuario |
| **Issues** | Issues abiertos por proyecto |
| **Merge requests** | MRs abiertos por proyecto |
| **Pipelines** | Últimos pipelines por proyecto |
| **Releases** | Releases publicadas por proyecto |
| **Ramas** | Ramas activas por proyecto |
| **Tags** | Tags por proyecto |
| **Milestones** | Milestones activos por proyecto |
| **Labels** | Etiquetas de proyecto |
| **Miembros** | Miembros del proyecto con roles |

### Datos del workspace

| Recurso | Descripción |
|---------|-------------|
| **Workspace roots** | Rutas del workspace del cliente MCP actual |

### Guías de buenas prácticas

5 guías integradas sobre workflows de GitLab:

| Guía | Temática |
|------|----------|
| **Git Flow** | Estrategia de ramas y workflow de merge |
| **Code Review** | Buenas prácticas para revisión de código |
| **CI/CD** | Configuración eficiente de pipelines |
| **Release Management** | Gestión de versiones y changelogs |
| **Issue Tracking** | Gestión efectiva de issues y milestones |

---

## Prompts MCP

Los 38 prompts son plantillas optimizadas que los clientes MCP ofrecen como acciones rápidas. Están agrupados por categoría.

### Revisión de código

| Prompt | Descripción |
|--------|-------------|
| **Code Review** | Revisión completa de un merge request |
| **Security Review** | Revisión enfocada en seguridad |
| **Performance Review** | Revisión enfocada en rendimiento |
| **Quick Review** | Revisión rápida con puntos clave |

### Estado y diagnóstico

| Prompt | Descripción |
|--------|-------------|
| **Pipeline Status** | Estado actual y diagnóstico de pipelines |
| **Pipeline Debug** | Depuración de pipelines fallidos |
| **Environment Status** | Estado de entornos de despliegue |
| **Deployment History** | Historial de despliegues recientes |

### Informes y analytics

| Prompt | Descripción |
|--------|-------------|
| **Release Notes** | Generación de notas de versión |
| **Daily Standup** | Resumen de actividad para el standup |
| **Sprint Report** | Informe de progreso del sprint |
| **Milestone Report** | Informe de milestone |
| **Burndown Chart** | Datos para gráfico de burndown |

### Gestión de equipo

| Prompt | Descripción |
|--------|-------------|
| **Team Workload** | Carga de trabajo del equipo |
| **User Statistics** | Estadísticas de contribución por usuario |
| **Review Workload** | Carga de revisiones de código |
| **Contributor Summary** | Resumen de contribuidores |

### Evaluación de riesgo

| Prompt | Descripción |
|--------|-------------|
| **Risk Assessment** | Evaluación de riesgo de un MR |
| **Change Impact** | Análisis de impacto de cambios |
| **Dependency Check** | Verificación de dependencias |

### Analytics avanzado

| Prompt | Descripción |
|--------|-------------|
| **Cross-project Dashboard** | Panel de estado entre proyectos |
| **Trend Analysis** | Análisis de tendencias (issues, MRs, pipelines) |
| **Audit Log** | Auditoría de actividad reciente |
| **Technical Debt** | Evaluación de deuda técnica |

### Workflows adicionales

Los prompts adicionales cubren: gestión de branches, planificación de releases, comparación de entornos, análisis de cobertura de tests, y workflows personalizados.

!!! tip "Disponibilidad en tu cliente"
    Los prompts aparecen de forma diferente según el cliente MCP:

    - **VS Code**: panel MCP en la barra lateral
    - **Claude Desktop**: usa `/` para acceder a los prompts disponibles
    - **Claude Code**: `claude prompts list` en la terminal
    - **Cursor**: sección MCP en configuración
