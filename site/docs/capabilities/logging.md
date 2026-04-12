# Logging

> **Dirección**: Servidor → Cliente
> **Método MCP**: `notifications/message`

## Arquitectura

El sistema de logging envía cada mensaje a dos destinos simultáneamente:

1. **Stderr** — salida estándar de Go `slog` para operadores del servidor
2. **Cliente MCP** — notificación `session.Log()` para clientes MCP conectados

### Niveles de log

| Nivel | Stderr | Cliente MCP |
| ----- | :----: | :---------: |
| `debug` | ✅ (si está habilitado) | ✅ |
| `info` | ✅ | ✅ |
| `warning` | ✅ | ✅ |
| `error` | ✅ | ✅ |

## Logging de ejecución de herramientas

Cada invocación de herramienta se registra automáticamente con:

- Nombre de la herramienta
- Duración de ejecución
- Estado (éxito/error)
- Mensaje de error (si falló)

```text
INFO tool call completed tool=gitlab_list_issues duration=245ms
ERROR tool call failed tool=gitlab_issue_get duration=102ms error="not found"
```

## Formato de datos

Los mensajes de log enviados al cliente MCP siguen esta estructura:

```json
{
  "level": "info",
  "logger": "gitlab-mcp-server",
  "data": {
    "message": "tool call completed",
    "tool": "gitlab_list_issues",
    "duration": "245ms",
    "status": "ok"
  }
}
```

- Si no hay datos adicionales: se usa el mensaje como cadena directa
- Si los datos son un `map`: se añade el mensaje como clave `"message"`
- En cualquier otro caso: se envuelven ambos en un nuevo mapa

## Configuración

| Variable | Valores | Por defecto |
| -------- | ------- | ----------- |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `LOG_FORMAT` | `text`, `json` | `text` |

### Formato JSON

Usa `LOG_FORMAT=json` para agregadores de logs estructurados:

```json
{"time":"2024-01-15T10:30:00Z","level":"INFO","msg":"tool call completed","tool":"gitlab_list_issues","duration":"245ms"}
```

## Seguridad

!!! warning "Nunca incluir secretos en logs"
    Los datos enviados al cliente MCP son visibles para el usuario. Nunca incluir tokens, contraseñas o credenciales en los datos de log. Es responsabilidad del código que llama al logger asegurar que los datos estén limpios.

## Referencias

- [Especificación MCP — Logging](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/logging)
- [Capacidades MCP](index.md) — todas las capacidades
