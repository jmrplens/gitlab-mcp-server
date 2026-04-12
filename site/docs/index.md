# Gitlab MCP

Servidor MCP (Model Context Protocol) para GitLab que permite a los asistentes de IA interactuar con tu instancia de GitLab mediante lenguaje natural.

---

## ¿Qué es Gitlab MCP?

Gitlab MCP es un servidor que implementa el [Model Context Protocol](https://modelcontextprotocol.io/) para conectar asistentes de IA (VS Code con Copilot, Claude Desktop, Cursor, Claude Code...) con la API de GitLab.

En lugar de memorizar comandos o navegar por la interfaz web, simplemente describe lo que necesitas y tu asistente de IA se encarga del resto.

---

## Características principales

| Característica | Descripción |
|---|---|
| :material-tools: **40+ meta-herramientas** | Operaciones CRUD sobre proyectos, issues, MRs, pipelines, runners... |
| :material-chart-line: **11 herramientas de análisis** | Análisis asistidos por IA: pipelines, MRs, CI/CD, deuda técnica... |
| :material-database: **24 recursos MCP** | Datos de solo lectura para contexto del asistente |
| :material-file-document: **38 prompts MCP** | Plantillas optimizadas para generación de informes |
| :material-account-group: **Modo multi-usuario** | Servidor HTTP para equipos con tokens individuales |
| :material-check-all: **Compatible con GitLab CE y EE** | Funciona con Community Edition y Enterprise |

---

## Ejemplo rápido

```text
> Lista los merge requests abiertos del proyecto my-app

✅ El asistente invoca gitlab_merge_request con action="list"
   y te muestra una tabla formateada con los MRs.
```

```text
> ¿Por qué falló el último pipeline?

✅ El asistente usa gitlab_analyze_pipeline_failure
   para diagnosticar el error y sugerir soluciones.
```

---

## Primeros pasos

→ Sigue la **[Guía de primeros pasos](getting-started.md)** para instalar y configurar Gitlab MCP en tu cliente favorito.

→ Consulta la **[Arquitectura](architecture.md)** para entender cómo funciona internamente.

→ Explora las **[Herramientas](tools/index.md)** disponibles para ver todo lo que puedes hacer.
