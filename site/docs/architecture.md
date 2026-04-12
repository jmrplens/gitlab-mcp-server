# Arquitectura

Cómo se comunican tus herramientas de IA con GitLab a través de Gitlab MCP.

---

## Visión general

Gitlab MCP actúa como puente entre tu cliente de IA y la API REST de GitLab. Traduce las peticiones en lenguaje natural del asistente a llamadas API estructuradas.

```mermaid
flowchart LR
    A["🧑 Usuario"] -->|lenguaje natural| B["🤖 Cliente IA"]
    B -->|protocolo MCP| C["⚙️ Gitlab MCP"]
    C -->|API REST v4| D["🦊 GitLab"]
    D -->|respuesta JSON| C
    C -->|resultado formateado| B
    B -->|respuesta| A
```

---

## Modos de transporte

Gitlab MCP soporta dos modos de comunicación con el cliente MCP:

### Modo stdio (por defecto)

El cliente MCP lanza el binario como proceso hijo. La comunicación es por entrada/salida estándar (stdin/stdout).

```mermaid
sequenceDiagram
    participant U as Usuario
    participant C as Cliente IA
    participant S as gitlab-mcp-server (stdio)
    participant G as GitLab API

    U->>C: "Lista mis proyectos"
    C->>S: tools/call: gitlab_project {action: "list"}
    S->>G: GET /api/v4/projects
    G-->>S: 200 OK [{...}, {...}]
    S-->>C: Tabla formateada con proyectos
    C-->>U: "Tienes 5 proyectos: ..."
```

**Ideal para**: uso individual, configuración sencilla, máxima seguridad (el token nunca sale del proceso local).

### Modo HTTP (multi-usuario)

El servidor se ejecuta como servicio web. Cada usuario proporciona su propio token por petición.

```mermaid
sequenceDiagram
    participant U1 as Usuario A
    participant U2 as Usuario B
    participant S as gitlab-mcp-server (HTTP :8080)
    participant G as GitLab API

    U1->>S: POST /mcp (token-A)
    U2->>S: POST /mcp (token-B)
    S->>G: GET /api/v4/... (como usuario A)
    S->>G: GET /api/v4/... (como usuario B)
    G-->>S: Respuestas
    S-->>U1: Resultado A
    S-->>U2: Resultado B
```

**Ideal para**: equipos, despliegues centralizados, entornos donde no se puede instalar binarios localmente.

---

## Registro de herramientas

Al conectarse, el servidor registra sus herramientas en el cliente MCP. Existen dos modos de registro:

```mermaid
flowchart TD
    A[Inicio del servidor] --> B{META_TOOLS?}
    B -->|true| C["40 meta-herramientas<br/>(agrupadas por dominio)"]
    B -->|false| D["1004 herramientas individuales<br/>(una por operación)"]
    C --> E["🤖 Cliente IA elige<br/>herramienta + acción"]
    D --> E
```

### Meta-herramientas vs herramientas individuales

| Aspecto | Meta-herramientas (40) | Individuales (1004) |
|---------|----------------------|---------------------|
| **Consumo de tokens** | :material-arrow-down: Bajo | :material-arrow-up: Alto |
| **Velocidad de selección** | :material-flash: Rápida | :material-clock-outline: Más lenta |
| **Granularidad** | Agrupadas por dominio | Una por operación |
| **Recomendado para** | La mayoría de usuarios | Casos muy específicos |

!!! info "Recomendación"
    Usa siempre meta-herramientas (`META_TOOLS=true`, que es el valor por defecto) salvo que tengas un caso de uso concreto que requiera herramientas individuales.

---

## Flujo de análisis con IA (sampling)

Las herramientas de análisis usan **MCP sampling** — un mecanismo donde el servidor solicita al cliente IA que procese datos con el LLM:

```mermaid
sequenceDiagram
    participant U as Usuario
    participant C as Cliente IA
    participant S as gitlab-mcp-server
    participant G as GitLab API

    U->>C: "Analiza por qué falló el pipeline"
    C->>S: tools/call: gitlab_analyze_pipeline_failure
    S->>G: GET /pipelines/{id}/jobs
    G-->>S: Jobs con logs de error
    S->>C: sampling/createMessage<br/>"Analiza estos logs..."
    Note over C: El LLM procesa los logs<br/>y genera un diagnóstico
    C-->>S: Diagnóstico del fallo
    S-->>C: Resultado formateado
    C-->>U: Análisis completo del fallo
```

Este flujo permite que las herramientas de análisis aprovechen la inteligencia del LLM para interpretar datos complejos como logs, diffs y métricas.

!!! note "Soporte de sampling"
    El sampling requiere que tu cliente MCP lo soporte. VS Code con GitHub Copilot y Claude Desktop lo soportan nativamente. Consulta la documentación de tu cliente si no estás seguro.

---

## Componentes opcionales

Gitlab MCP tiene una arquitectura modular. Puedes activar o desactivar componentes según tus necesidades:

```mermaid
flowchart TD
    A["Gitlab MCP"] --> B["Meta-herramientas / Individuales"]
    A --> C["Herramientas de análisis"]
    A --> D["Herramientas de elicitación"]
    A --> E["Recursos MCP"]
    A --> F["Prompts MCP"]

    B -->|"siempre activas"| G["✅"]
    C -->|"ENABLE_ANALYSIS_TOOLS"| H["✅ / ❌"]
    D -->|"ENABLE_ELICITATION"| I["✅ / ❌"]
    E -->|"ENABLE_RESOURCES"| J["✅ / ❌"]
    F -->|"ENABLE_PROMPTS"| K["✅ / ❌"]
```

Consulta la [Guía de configuración](configuration.md) para los detalles de cada variable.
