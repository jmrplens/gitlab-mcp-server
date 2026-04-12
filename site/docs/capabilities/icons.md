# Icons

> **Formato**: SVG inline como data URIs
> **Tamaño**: 16×16 píxeles
> **MIME**: `image/svg+xml`

## Descripción general

gitlab-mcp-server proporciona **más de 40 iconos SVG únicos** asignados a herramientas, recursos y prompts. Los iconos permiten que los clientes MCP muestren representaciones visuales junto a cada herramienta en su interfaz.

## Especificación MCP

Los iconos siguen la [interfaz Icon de MCP](https://modelcontextprotocol.io/specification/2025-11-25) (versión de protocolo 2025-11-25):

```typescript
interface Icon {
  src: string;          // URI (HTTP/HTTPS o data: URI)
  mimeType?: string;    // Tipo MIME
  sizes?: string[];     // Tamaños disponibles
  theme?: "light" | "dark"; // Tema
}
```

### Soporte MIME en clientes

| Tipo MIME | Soporte requerido | Notas |
| --------- | ----------------- | ----- |
| `image/png` | **MUST** | Compatibilidad universal |
| `image/jpeg` | **MUST** | Compatibilidad universal |
| `image/svg+xml` | **SHOULD** | Escalable, usado por este proyecto |
| `image/webp` | **SHOULD** | Formato moderno eficiente |

gitlab-mcp-server usa `image/svg+xml` exclusivamente. Clientes que solo implementen PNG/JPEG no renderizarán estos iconos.

### Compatibilidad de clientes

| Cliente MCP | Iconos SVG | Notas |
| ----------- | :--------: | ----- |
| VS Code (GitHub Copilot) | Sí | Soporte completo de renderizado SVG |
| Claude Desktop | No | No renderiza iconos de herramientas |
| Continue.dev | Parcial | Depende de la versión |

## Principios de diseño

- **Viewport 16×16**: tamaño mínimo que se renderiza limpio a todas las escalas
- **SVGs de un solo path**: ligeros, sin formas complejas ni gradientes
- **`currentColor`**: todos los iconos usan `currentColor` para adaptación automática a tema claro/oscuro
- **Data URIs inline**: sin archivos externos ni peticiones de red — los iconos están embebidos en el binario
- **Sin JavaScript**: los SVGs no contienen scripts ni referencias a recursos externos

## Catálogo de iconos

### Control de código fuente

| Nombre | Uso |
| ------ | --- |
| `IconBranch` | Ramas |
| `IconCommit` | Commits |
| `IconTag` | Tags |
| `IconRelease` | Releases |
| `IconMR` | Merge Requests |

### Objetos de GitLab

| Nombre | Uso |
| ------ | --- |
| `IconProject` | Proyectos |
| `IconGroup` | Grupos |
| `IconIssue` | Issues |
| `IconPipeline` | Pipelines |
| `IconJob` | Jobs |
| `IconLabel` | Labels |
| `IconMilestone` | Milestones |
| `IconUser` | Usuarios |
| `IconWiki` | Wiki |
| `IconFile` | Archivos |
| `IconPackage` | Paquetes |
| `IconSnippet` | Snippets |
| `IconBoard` | Boards |

### Infraestructura

| Nombre | Uso |
| ------ | --- |
| `IconEnvironment` | Entornos |
| `IconDeploy` | Despliegues |
| `IconRunner` | Runners |
| `IconSchedule` | Programaciones |
| `IconVariable` | Variables CI/CD |
| `IconContainer` | Contenedores |
| `IconInfra` | Infraestructura |

### Operacionales

| Nombre | Uso |
| ------ | --- |
| `IconHealth` | Estado de salud |
| `IconSearch` | Búsqueda |
| `IconProgress` | Progreso |
| `IconUpload` | Subidas |
| `IconDownload` | Descargas |
| `IconTodo` | To-dos |

### Sistema

| Nombre | Uso |
| ------ | --- |
| `IconServer` | Servidor |
| `IconSecurity` | Seguridad |
| `IconConfig` | Configuración |
| `IconAnalytics` | Análisis |
| `IconKey` | Claves |
| `IconLink` | Enlaces |
| `IconNotify` | Notificaciones |
| `IconIntegration` | Integraciones |
| `IconImport` | Importaciones |
| `IconTemplate` | Plantillas |
| `IconEvent` | Eventos |
| `IconAlert` | Alertas |
| `IconDiscussion` | Discusiones |
| `IconEpic` | Epics |

## Patrón de uso

Cada herramienta, recurso y prompt asigna su icono en el momento de registro:

```go
mcp.AddTool(server, &mcp.Tool{
    Name:  "gitlab_list_branches",
    Icons: toolutil.IconBranch,
})
```

Los iconos son constantes a nivel de paquete de tipo `[]mcp.Icon`, pre-construidas por un helper que genera el data URI:

```go
func icon(svg string) []mcp.Icon {
    return []mcp.Icon{{
        Source:   "data:image/svg+xml," + svg,
        MIMEType: "image/svg+xml",
    }}
}
```

## Referencias

- [Especificación MCP — Icons](https://modelcontextprotocol.io/specification/2025-11-25)
- [Capacidades MCP](index.md) — todas las capacidades
