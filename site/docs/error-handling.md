# Manejo de errores

Cómo Gitlab MCP gestiona y comunica los errores.

---

## Formato de errores

Los errores se devuelven al cliente MCP como resultado de herramienta con estado de error. El formato es consistente:

```json
{
  "isError": true,
  "content": [
    {
      "type": "text",
      "text": "Error: descripción del problema"
    }
  ]
}
```

---

## Tipos de errores comunes

### Errores de autenticación

| Error | Causa | Solución |
|-------|-------|----------|
| `401 Unauthorized` | Token inválido o expirado | Regenera tu PAT en GitLab |
| `403 Forbidden` | Token sin permisos suficientes | Usa alcance `api` o `read_api` |
| `403 Forbidden` en proyecto específico | Sin acceso al proyecto | Solicita acceso al propietario |

### Errores de conexión

| Error | Causa | Solución |
|-------|-------|----------|
| `connection refused` | GitLab no accesible | Verifica `GITLAB_URL` y conectividad |
| `TLS handshake failure` | Certificado inválido | Usa `GITLAB_SKIP_TLS_VERIFY=true` en desarrollo |
| `timeout` | Red lenta o GitLab sobrecargado | Reintenta o verifica la red |
| `no such host` | DNS no resuelve | Verifica el dominio de `GITLAB_URL` |

### Errores de API

| Error | Causa | Solución |
|-------|-------|----------|
| `404 Not Found` | Recurso no existe | Verifica el ID o path del recurso |
| `409 Conflict` | Conflicto de estado | El recurso fue modificado simultáneamente |
| `422 Unprocessable` | Datos de entrada inválidos | Revisa los parámetros enviados |
| `429 Too Many Requests` | Rate limit de GitLab | Espera y reintenta |

---

## Errores de cancelación

Si el cliente MCP cancela una operación (por ejemplo, al cerrar la conversación), el servidor propaga el error de cancelación de contexto limpiamente:

- Las llamadas API en curso se cancela
- No quedan goroutines huérfanas
- El estado del pool se limpia correctamente

---

## Errores en modo HTTP

### Rate limiting de autenticación

Después de **10 intentos fallidos** de autenticación desde la misma IP en un minuto, las peticiones se bloquean temporalmente.

### Pool lleno

Si se alcanza el límite de `--max-http-clients`, la sesión menos usada (LRU) se elimina para hacer espacio. Esto no es un error visible — la sesión evicta necesitará re-autenticarse.

---

## Formato de salida

Todas las herramientas devuelven resultados en **Markdown** para máxima compatibilidad con asistentes de IA:

### Elemento individual

```markdown
## Project: MiProyecto

- **ID**: 42
- **Path**: grupo/miproyecto
- **Visibility**: private
- **Default Branch**: develop
- **⭐ Stars**: 15
- **URL**: https://gitlab.ejemplo.com/grupo/miproyecto

**Hints:**
- Usa gitlab_branch action 'list' para ver ramas
- Usa gitlab_merge_request action 'list' para ver MRs abiertos
```

### Listados (tabla Markdown)

```markdown
## Projects (5)

Mostrando 5 de 10 elementos (página 1 de 2)

| ID | Nombre | Path | Visibilidad | ⭐ |
| --- | --- | --- | --- | --- |
| 42 | [MiProyecto](https://...) | grupo/miproyecto | private | 15 |
| 43 | [OtroProyecto](https://...) | grupo/otro | public | 3 |

**Siguiente:** page=2
```

### Características del formato

- **Tablas Markdown** para listados con paginación
- **Listas de propiedades** para elementos individuales
- **Hints contextuales** sugiriendo operaciones relacionadas
- **Links preservados** como sintaxis Markdown
- **Emojis** para indicadores visuales (⭐ estrellas, ✅ archivado)
- **Escapado seguro** de contenido de usuario en celdas de tabla y encabezados
