# Configuración

Gitlab MCP se configura mediante variables de entorno. Un archivo `.env` en el directorio actual se carga automáticamente, y el servidor también carga `~/.pe-mcp-gitlab.env` como respaldo para secretos escritos por el asistente de configuración.

---

## Variables principales

### Obligatorias

| Variable | Descripción | Ejemplo |
| --- | --- | --- |
| `GITLAB_URL` | URL base de la instancia GitLab | `https://gitlab.example.com` |
| `GITLAB_TOKEN` | Personal Access Token con scope `api` | `glpat-xxxxxxxxxxxxxxxxxxxx` |

### Opciones comunes

| Variable | Default | Descripción |
| --- | --- | --- |
| `GITLAB_USER` | _(ninguno)_ | Nombre de usuario de GitLab (usado por algunos prompts y recursos) |
| `GITLAB_SKIP_TLS_VERIFY` | `false` | Omitir verificación TLS para certificados auto-firmados |
| `META_TOOLS` | `true` | Activar meta-herramientas de dominio (40 base / 59 enterprise) |
| `GITLAB_ENTERPRISE` | `false` | Activar meta-herramientas Enterprise/Premium (19 adicionales) |
| `GITLAB_READ_ONLY` | `false` | Modo solo lectura: desactiva todas las herramientas de escritura |
| `LOG_LEVEL` | `info` | Nivel de log: `debug`, `info`, `warn`, `error` |

### Componentes opcionales

| Variable | Default | Descripción |
| --- | --- | --- |
| `ENABLE_ANALYSIS_TOOLS` | `true` | Activar herramientas de análisis (requiere sampling en el cliente) |
| `ENABLE_ELICITATION` | `true` | Activar asistentes interactivos de creación de recursos |
| `ENABLE_RESOURCES` | `true` | Activar recursos MCP de solo lectura |
| `ENABLE_PROMPTS` | `true` | Activar prompts/plantillas MCP |

Consulta la [Arquitectura](architecture.md#componentes-opcionales) para una visión general de estos componentes.

### Ejemplo de archivo .env

```env
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_USER=miusuario
GITLAB_SKIP_TLS_VERIFY=true
META_TOOLS=true
GITLAB_READ_ONLY=false
LOG_LEVEL=info
```

!!! warning "Seguridad"
    El archivo `.env` está en el `.gitignore`. Nunca hagas commit de tokens ni credenciales.

---

## Configuración de clientes MCP

Si prefieres configurar los clientes manualmente en vez de usar el asistente.

=== "VS Code / GitHub Copilot"

    Añade a `.vscode/mcp.json` en tu proyecto:

    ```json
    {
      "servers": {
        "gitlab": {
          "type": "stdio",
          "command": "/ruta/a/pe-mcp-gitlab",
          "env": {
            "GITLAB_URL": "https://tu-instancia-gitlab",
            "GITLAB_TOKEN": "glpat-tu-token"
          }
        }
      }
    }
    ```

=== "Claude Desktop"

    Añade a `claude_desktop_config.json`:

    ```json
    {
      "mcpServers": {
        "gitlab": {
          "command": "/ruta/a/pe-mcp-gitlab",
          "env": {
            "GITLAB_URL": "https://tu-instancia-gitlab",
            "GITLAB_TOKEN": "glpat-tu-token"
          }
        }
      }
    }
    ```

=== "Cursor"

    Añade a `.cursor/mcp.json`:

    ```json
    {
      "mcpServers": {
        "gitlab": {
          "command": "/ruta/a/pe-mcp-gitlab",
          "env": {
            "GITLAB_URL": "https://tu-instancia-gitlab",
            "GITLAB_TOKEN": "glpat-tu-token"
          }
        }
      }
    }
    ```

=== "Claude Code (CLI)"

    Añade a `~/.claude.json`:

    ```json
    {
      "mcpServers": {
        "gitlab": {
          "command": "/ruta/a/pe-mcp-gitlab",
          "env": {
            "GITLAB_URL": "https://tu-instancia-gitlab",
            "GITLAB_TOKEN": "glpat-tu-token"
          }
        }
      }
    }
    ```

---

## Configuración segura del token

### VS Code — Variables de entrada

VS Code permite solicitar el token al iniciar el servidor y almacenarlo de forma segura:

```json
{
  "inputs": [
    {
      "type": "promptString",
      "id": "gitlab-token",
      "description": "GitLab Personal Access Token",
      "password": true
    }
  ],
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/ruta/a/pe-mcp-gitlab",
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "${input:gitlab-token}"
      }
    }
  }
}
```

### VS Code — Archivo de entorno

Carga variables de entorno desde un archivo, manteniendo los secretos fuera del JSON:

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/ruta/a/pe-mcp-gitlab",
      "envFile": "${userHome}/.pe-mcp-gitlab.env"
    }
  }
}
```

---

## Variables de administración

Para operadores que despliegan el servidor para un equipo.

| Variable | Default | Descripción |
| --- | --- | --- |
| `AUTO_UPDATE` | `true` | Actualizaciones automáticas (`true`/`check`/`false`) |
| `AUTO_UPDATE_INTERVAL` | `1h` | Intervalo entre comprobaciones de actualizaciones |
| `YOLO_MODE` | `false` | Omitir confirmaciones en acciones destructivas |

---

## Modo servidor HTTP

Para despliegues multi-usuario con HTTP:

| Parámetro | Default | Descripción |
| --- | --- | --- |
| `--http` | _(off)_ | Activar modo HTTP |
| `--http-addr` | `localhost:8080` | Dirección de escucha |
| `--gitlab-url` | _(obligatorio)_ | URL de la instancia GitLab |
| `--skip-tls-verify` | `false` | Omitir verificación TLS |
| `--meta-tools` | `true` | Activar meta-herramientas |
| `--enterprise` | `false` | Activar meta-herramientas Enterprise |
| `--max-http-clients` | `100` | Máximo de sesiones concurrentes |
| `--session-timeout` | `30m` | Timeout de sesión inactiva |

Cada cliente proporciona su propio token por petición vía cabecera `PRIVATE-TOKEN` o `Authorization: Bearer`.

---

## Orden de carga

La configuración se carga en este orden (la última gana):

1. `~/.pe-mcp-gitlab.env` (credenciales del asistente)
2. `.env` en el directorio actual
3. Variables de entorno del sistema
4. Flags de línea de comandos (`--http`, `--http-addr`)
