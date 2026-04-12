# Capacidades MCP

Documentación de las **7 capacidades MCP** implementadas por gitlab-mcp-server.

## ¿Qué son las capacidades?

Las capacidades son funcionalidades a nivel de protocolo que se negocian durante el handshake `initialize` de MCP. Determinan qué puede hacer el servidor y el cliente más allá de las llamadas a herramientas básicas: logging estructurado, notificaciones de progreso, autocompletado, descubrimiento de workspace, delegación al LLM e interacción directa con el usuario.

## Capacidades del servidor

Declaradas por el servidor y consumidas por los clientes MCP conectados.

| # | Capacidad | Propósito |
| --: | --------- | --------- |
| 1 | [Logging](logging.md) | Mensajes de log estructurados al cliente |
| 2 | [Progress](progress.md) | Notificaciones de progreso paso a paso |
| 3 | [Completions](completions.md) | Autocompletado de argumentos y URIs de recursos |

## Capacidades del cliente

Proporcionadas por el cliente MCP y consumidas por el servidor durante la ejecución de herramientas.

| # | Capacidad | Propósito |
| --: | --------- | --------- |
| 4 | [Roots](roots.md) | Descubrimiento de directorios del workspace |
| 5 | [Sampling](sampling.md) | Delegación de análisis al LLM |
| 6 | [Elicitation](elicitation.md) | Formularios interactivos de entrada de usuario |

## Funcionalidades transversales

| Funcionalidad | Propósito |
| ------------- | --------- |
| [Icons](icons.md) | 40+ iconos SVG para herramientas, recursos y prompts |

## Declaración de capacidades

Las capacidades se declaran al construir el servidor MCP:

```go
server := mcp.NewServer(
    &mcp.ServerCapabilities{
        Logging:     &mcp.LoggingCapabilities{},
        Completions: &mcp.CompletionCapability{},
    },
    &mcp.ServerOptions{
        CompletionHandler:       completionHandler.Complete,
        RootsListChangedHandler: rootsManager.Refresh,
    },
)
```

Sampling, elicitation y roots dependen de que el **cliente** declare soporte en su handshake. Si el cliente no soporta una capacidad, el servidor degrada silenciosamente (sin errores).
