# Completions

> **Dirección**: Cliente → Servidor
> **Método MCP**: `completion/complete`

## ¿Qué problema resuelve?

Cuando la IA o un usuario necesita proporcionar un valor para un argumento de herramienta (por ejemplo, `project_id`), normalmente tiene que adivinar o hacer una búsqueda separada. **Completions** proporciona sugerencias de autocompletado en tiempo real, permitiendo descubrir valores válidos sin llamadas API adicionales.

## Cómo funciona

1. El cliente envía una petición `completion/complete` con una referencia al argumento y un valor parcial
2. El handler identifica el tipo de argumento
3. Busca coincidencias en la API de GitLab usando filtrado por prefijo
4. Devuelve hasta 10 sugerencias

```json
// Petición
{
  "method": "completion/complete",
  "params": {
    "ref": {
      "type": "ref/argument",
      "name": "project_id"
    },
    "argument": {
      "name": "project_id",
      "value": "my-"
    }
  }
}

// Respuesta
{
  "completion": {
    "values": ["123: my-app", "456: my-service"],
    "hasMore": false,
    "total": 2
  }
}
```

## Argumentos soportados

| Argumento | Fuente | Máx. resultados |
| --------- | ------ | :-------------: |
| `project_id` | Búsqueda de proyectos en GitLab | 10 |
| `group_id` | Búsqueda de grupos en GitLab | 10 |
| `mr_iid` | MRs abiertos del proyecto | 10 |
| `issue_iid` | Issues abiertos del proyecto | 10 |
| `branch` | Ramas del proyecto | 10 |
| `tag` | Tags del proyecto | 10 |
| `username` | Búsqueda de usuarios en GitLab | 10 |
| `label` | Labels del proyecto | 10 |
| `milestone_id` | Milestones del proyecto | 10 |
| `pipeline_id` | Pipelines del proyecto | 10 |
| `sha` | Commits del proyecto | 10 |
| `from` / `to` | Ramas o tags | 10 |
| `ref` | Ramas o tags | 10 |
| `job_id` | Jobs del pipeline (requiere `project_id` + `pipeline_id`) | 10 |

## Contextos de completado

Dos tipos de referencia soportados:

| Tipo | Descripción |
| ---- | ----------- |
| `ref/prompt` | Completar argumentos de plantillas de prompts |
| `ref/resource` | Completar parámetros en URIs de recursos (`project_id`, `group_id`, `mr_iid`, `issue_iid`) |

## Degradación elegante

Si la búsqueda en GitLab falla (error de red, permisos):

- Se devuelven resultados vacíos silenciosamente
- Las herramientas siguen funcionando normalmente — el usuario debe proporcionar el valor exacto manualmente
- No se muestran errores al usuario por un fallo de autocompletado

## Preguntas frecuentes

### ¿Todos los clientes MCP soportan completions?

No todos. VS Code con GitHub Copilot tiene soporte de completions. Otros clientes pueden mostrar los argumentos como campos de texto simple sin autocompletado.

### ¿Las completions hacen llamadas API cada vez?

Sí, cada petición de completado hace una búsqueda real en la API de GitLab para obtener datos actualizados. Los resultados no se cachean para garantizar que siempre reflejen el estado actual del servidor.

## Referencias

- [Especificación MCP — Completions](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/completion)
- [Capacidades MCP](index.md) — todas las capacidades
