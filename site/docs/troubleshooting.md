# Solución de problemas

Soluciones a problemas comunes con Gitlab MCP.

---

## Conexión

### El servidor no se conecta a GitLab

**Síntoma**: errores de conexión o timeout al usar herramientas.

**Soluciones**:

1. Verifica que `GITLAB_URL` es correcto y accesible desde tu red
2. Prueba acceder a `$GITLAB_URL/api/v4/version` desde el navegador
3. Revisa que no haya proxy o firewall bloqueando la conexión
4. Si usas VPN, asegúrate de que está activa

### Token inválido o expirado

**Síntoma**: errores 401 Unauthorized.

**Soluciones**:

1. Verifica que tu PAT no ha expirado
2. Comprueba que el token tiene alcance `api` o `read_api`
3. Regenera el token si es necesario desde **Preferencias → Tokens de acceso** en tu GitLab

### Errores SSL/TLS

**Síntoma**: errores de certificado o handshake SSL.

**Soluciones**:

1. Verifica que la URL usa `https://`
2. Si tu instancia usa certificados autofirmados, configura `GITLAB_SKIP_TLS_VERIFY=true` en tu archivo `.env` o en las variables de entorno
3. Consulta la [Guía de configuración](configuration.md#opciones-comunes) para más opciones TLS

---

## Herramientas

### Las herramientas no aparecen en el cliente

**Síntoma**: el cliente MCP no muestra herramientas de Gitlab MCP.

**Soluciones**:

1. Verifica que el binario tiene permisos de ejecución (`chmod +x`)
2. Comprueba que la ruta al binario en la configuración es correcta y absoluta
3. Reinicia el cliente MCP completamente
4. Revisa los logs del cliente para errores de conexión al servidor MCP

### Las herramientas de análisis no funcionan

**Síntoma**: las herramientas de análisis devuelven errores o no producen resultados.

**Soluciones**:

1. Verifica que tu cliente MCP soporta **sampling** (VS Code con Copilot y Claude Desktop lo soportan)
2. Comprueba que `ENABLE_ANALYSIS_TOOLS=true` (valor por defecto)
3. Algunos clientes requieren aceptar permisos de sampling la primera vez
4. Consulta la página de [Herramientas de análisis](tools/analysis.md) para más detalles

### Las herramientas Enterprise devuelven errores

**Síntoma**: errores al usar herramientas marcadas como Enterprise.

**Soluciones**:

1. Verifica que `GITLAB_ENTERPRISE=true` está configurado
2. Comprueba que tu instancia GitLab es Premium o Ultimate
3. Las herramientas Enterprise no funcionan en GitLab Community Edition

### No se puede reemplazar el binario (archivo bloqueado)

**Síntoma**: errores al intentar actualizar el binario porque hay instancias en ejecución.

**Soluciones**:

1. Ejecuta `pe-mcp-gitlab --shutdown` para terminar todas las instancias
2. Reemplaza el binario
3. El cliente MCP reiniciará automáticamente con la nueva versión

---

## Rendimiento

### Respuestas lentas

**Síntoma**: las respuestas de las herramientas tardan mucho.

**Soluciones**:

1. Usa `META_TOOLS=true` (por defecto) para reducir el número de herramientas registradas
2. Si usas herramientas individuales (1004), el modelo tarda más en seleccionar la correcta
3. Verifica la latencia de red hacia tu instancia GitLab
4. Consulta la [Arquitectura](architecture.md) para entender las diferencias entre meta-herramientas e individuales

### El modelo elige la herramienta incorrecta

**Síntoma**: el asistente usa una herramienta diferente a la esperada.

**Soluciones**:

1. Sé más específico en tu petición (incluye el nombre del proyecto, números de issue/MR)
2. Si usas herramientas individuales, considera cambiar a meta-herramientas
3. Reformula la petición proporcionando más contexto

---

## Problemas por cliente

=== "VS Code + Copilot"

    - Asegúrate de tener la extensión GitHub Copilot actualizada
    - La configuración MCP se define en `settings.json` (global o por workspace)
    - Reinicia VS Code después de cambiar la configuración
    - Si las herramientas no aparecen, verifica en **Output → GitHub Copilot** los logs

=== "Claude Desktop"

    - Verifica la ruta del archivo de configuración:
        - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
        - Windows: `%APPDATA%\Claude\claude_desktop_config.json`
        - Linux: `~/.config/Claude/claude_desktop_config.json`
    - Reinicia Claude Desktop después de cambiar la configuración
    - Verifica que el JSON es válido (sin comas finales)

=== "Cursor"

    - La configuración MCP está en `.cursor/mcp.json`
    - Reinicia Cursor después de cambiar la configuración

=== "Claude Code"

    - Verifica servidores con `claude mcp list`
    - Para eliminar: `claude mcp remove gitlab`
    - Para reinstalar: usa el comando `claude mcp add` completo de la [Guía de primeros pasos](getting-started.md)

---

## Modo HTTP

### No puedo conectar al servidor HTTP

**Síntoma**: error de conexión al intentar usar el modo HTTP.

**Soluciones**:

1. Verifica que `MCP_TRANSPORT=http` está configurado
2. Comprueba que el puerto (`HTTP_PORT`, por defecto 8080) no está ocupado
3. Si accedes desde otra máquina, verifica el firewall
4. En producción, usa un reverse proxy con HTTPS para proteger los tokens
