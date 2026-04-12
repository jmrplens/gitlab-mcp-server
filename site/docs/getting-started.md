# Primeros pasos

Guía rápida para instalar y configurar Gitlab MCP con tu cliente de IA favorito.

---

## Requisitos previos

- **Binario de Gitlab MCP**: disponible como release en el proyecto GitLab, o instalable desde PE — The Agnostic Store
- **Token de acceso personal** (PAT) de GitLab con permisos `api` o `read_api`
- **Cliente MCP** compatible: VS Code + Copilot, Claude Desktop, Cursor, o Claude Code

!!! tip "¿Instalaste desde PE — The Agnostic Store?"
    Si usaste la Store para instalar Gitlab MCP, el binario ya está listo en tu sistema:

    | Sistema | Ruta del binario |
    |---------|------------------|
    | **Linux** | `~/.local/bin/pe-mcp-gitlab` |
    | **macOS** | `~/.local/bin/pe-mcp-gitlab` |
    | **Windows** | `%LOCALAPPDATA%\pe-mcp-gitlab\pe-mcp-gitlab.exe` |

    Solo necesitas ejecutar el asistente de configuración desde esa ruta.

---

## Instalación

### Descargar el binario

Descarga el binario correspondiente a tu sistema operativo desde las releases del proyecto:

| Sistema | Binario |
|---------|--------|
| Linux (amd64) | `pe-mcp-gitlab-linux-amd64` |
| macOS (amd64) | `pe-mcp-gitlab-darwin-amd64` |
| macOS (arm64) | `pe-mcp-gitlab-darwin-arm64` |
| Windows (amd64) | `pe-mcp-gitlab-windows-amd64.exe` |

### Hacer ejecutable (Linux/macOS)

```bash
chmod +x pe-mcp-gitlab-linux-amd64
```

---

## Asistente de configuración

La forma más rápida de empezar es usar el asistente interactivo:

```bash
./pe-mcp-gitlab-linux-amd64 setup-wizard
```

El asistente te guiará para:

1. Introducir la URL de tu instancia GitLab
2. Configurar tu token de acceso personal
3. Seleccionar un cliente MCP (VS Code, Claude Desktop, Cursor, Claude Code)
4. Generar la configuración automáticamente

!!! info "Token de acceso personal"
    Necesitas un PAT con alcance `api` (lectura/escritura) o `read_api` (solo lectura). Créalo desde **Preferencias → Tokens de acceso** en tu GitLab. [Más información](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html).

---

## Configuración manual por cliente

Si prefieres configurar manualmente, selecciona tu cliente:

=== "VS Code + Copilot"

    Añade a tu `settings.json`:

    ```json
    {
      "mcp": {
        "servers": {
          "gitlab": {
            "command": "/ruta/al/pe-mcp-gitlab-linux-amd64",
            "args": [],
            "env": {
              "GITLAB_URL": "https://tu-gitlab.ejemplo.com",
              "GITLAB_TOKEN": "tu-token-aquí"
            }
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
          "command": "/ruta/al/pe-mcp-gitlab-linux-amd64",
          "args": [],
          "env": {
            "GITLAB_URL": "https://tu-gitlab.ejemplo.com",
            "GITLAB_TOKEN": "tu-token-aquí"
          }
        }
      }
    }
    ```

=== "Cursor"

    Añade a la configuración MCP de Cursor:

    ```json
    {
      "mcpServers": {
        "gitlab": {
          "command": "/ruta/al/pe-mcp-gitlab-linux-amd64",
          "args": [],
          "env": {
            "GITLAB_URL": "https://tu-gitlab.ejemplo.com",
            "GITLAB_TOKEN": "tu-token-aquí"
          }
        }
      }
    }
    ```

=== "Claude Code"

    ```bash
    claude mcp add --transport stdio gitlab \
      /ruta/al/pe-mcp-gitlab-linux-amd64 \
      -e GITLAB_URL=https://tu-gitlab.ejemplo.com \
      -e GITLAB_TOKEN=tu-token-aquí
    ```

---

## Modo HTTP (multi-usuario)

Para equipos, puedes ejecutar Gitlab MCP como servidor HTTP:

```bash
export GITLAB_URL="https://tu-gitlab.ejemplo.com"
export MCP_TRANSPORT=http
export HTTP_PORT=8080
./pe-mcp-gitlab-linux-amd64
```

Cada usuario envía su propio token en las peticiones. Consulta la [Guía de configuración](configuration.md) para más detalles.

---

## Verificación

Una vez configurado, abre tu cliente MCP y prueba:

```text
> ¿Quién soy en GitLab?
```

Si todo funciona correctamente, el asistente mostrará tu perfil de usuario de GitLab.
