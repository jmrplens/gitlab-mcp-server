# Progress

> **Dirección**: Servidor → Cliente
> **Método MCP**: `notifications/progress`

## ¿Qué problema resuelve?

Algunas operaciones del servidor MCP toman tiempo: subidas de archivos, operaciones en lote, análisis con sampling. Sin feedback, el usuario no sabe si la herramienta está trabajando o se ha quedado bloqueada.

**Progress** envía notificaciones al cliente MCP durante operaciones largas, permitiendo que los clientes muestren barras de progreso o indicadores de estado.

## Cómo funciona

```json
{
  "method": "notifications/progress",
  "params": {
    "progressToken": "token-de-la-petición",
    "progress": 3,
    "total": 10,
    "message": "Procesando archivo 3 de 10..."
  }
}
```

El servidor extrae el `progressToken` de la petición original del cliente y lo usa para enviar actualizaciones durante la ejecución.

## Métodos disponibles

| Método | Parámetros | Uso |
| ------ | ---------- | --- |
| `Update(ctx, progress, total, message)` | float64, float64, string | Progreso explícito (ej. 50 de 100) |
| `Step(ctx, step, totalSteps, message)` | int, int, string | Progreso por pasos (1 de 5, 2 de 5...) |
| `IsActive()` | — | Verificar si el tracker puede enviar notificaciones |

## Operaciones con progreso

| Operación | Granularidad |
| --------- | ------------ |
| Subida de archivos | Por archivo |
| Operaciones en lote | Por elemento |
| Verificación de actualizaciones | Inicio/completado |
| Análisis con sampling | Por iteración |

## Propiedades

- **Zero-value safe**: todos los métodos son no-ops en un tracker inactivo — no se necesitan comprobaciones de nil
- **Sin propagación de errores**: las notificaciones fallidas se registran en log pero no abortan la herramienta
- **Degradación elegante**: funciona si el cliente soporta progress; silenciosamente no hace nada si no

## Soporte de clientes

Progress solo se envía cuando el cliente proporciona un `progressToken` en la petición de la herramienta. Si no se proporciona token, el reporte de progreso se omite silenciosamente.

| Cliente MCP | Soporte de progress |
| ----------- | :-----------------: |
| VS Code (GitHub Copilot) | Sí |
| Claude Desktop | Parcial |
| Continue.dev | Depende de la versión |

## Referencias

- [Especificación MCP — Progress](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/progress)
- [Capacidades MCP](index.md) — todas las capacidades
