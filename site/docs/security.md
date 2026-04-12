# Seguridad

Modelo de seguridad y buenas prácticas para Gitlab MCP.

---

## Autenticación

### Modo Stdio (token individual)

El servidor requiere `GITLAB_TOKEN` en modo stdio. El token se carga desde:

1. Variables de entorno configuradas en el cliente MCP
2. Archivo `.env` en el directorio de trabajo
3. Archivo `~/.gitlab-mcp-server.env` como fallback global

El token:

- Nunca se muestra en logs (solo los últimos 4 caracteres: `...abcd`)
- Se envía exclusivamente como cabecera `PRIVATE-TOKEN` via HTTPS
- Se puede restringir con alcance `read_api` para acceso de solo lectura

### Modo HTTP (por petición)

En modo HTTP, el servidor **no requiere token al arrancar**. Cada cliente proporciona su propio token en cada petición via:

- Cabecera `PRIVATE-TOKEN: <token>` (recomendado)
- Cabecera `Authorization: Bearer <token>`

Las peticiones sin token válido se rechazan. Cada token obtiene una instancia aislada de servidor MCP.

---

## TLS / HTTPS

### Verificación de certificados

Por defecto, los certificados TLS se verifican completamente con las CAs raíz del sistema.

Para certificados autofirmados:

```bash
GITLAB_SKIP_TLS_VERIFY=true
```

!!! warning "Solo para desarrollo"
    Desactivar la verificación TLS es aceptable en entornos de desarrollo local. En producción, usa siempre certificados válidos.

### Auto-actualización y TLS

El cliente de auto-actualización usa un cliente HTTP dedicado con:

- Verificación TLS independiente de `GITLAB_SKIP_TLS_VERIFY`
- Versión mínima TLS 1.2 forzada
- HTTPS obligatorio (rechaza URLs sin `https://`)

---

## Enmascaramiento de tokens

Los tokens y credenciales se enmascaran en todos los contextos:

| Contexto | Protección |
|----------|------------|
| **Logs** | Solo últimos 4 caracteres: `...abcd` |
| **Errores** | Nunca incluyen el token |
| **Configuración** | Los métodos `String()` muestran `***` |
| **Pool HTTP** | Tokens hasheados con SHA-256 |
| **Auto-update** | Token de compilación ofuscado via `ldflags` |

---

## Validación de entrada

### Entradas de herramientas

Todas las entradas de herramientas se validan por el esquema JSON del SDK MCP antes de llegar a los handlers. Los handlers realizan validación semántica adicional:

- Campos requeridos verificados antes de llamadas API
- IDs numéricos validados como enteros positivos
- URLs construidas via parámetros (sin concatenación)
- Bodies JSON codificados (sin interpolación directa)

### Validación de URL

La URL de GitLab (`GITLAB_URL`) se valida al arrancar:

- Debe empezar con `http://` o `https://`
- Debe tener un host válido
- Se rechaza si está vacía o malformada

---

## Protección contra inyección de prompts

### Boundary tags en salida

Todo el contenido de usuario proveniente de GitLab (descripciones, notas, comentarios, contenido de ficheros) se envuelve en tags de frontera aleatorios para prevenir inyección de prompts:

```text
<!-- insecure-content-a1b2c3d4 -->
Contenido de GitLab controlado por el usuario
<!-- /insecure-content-a1b2c3d4 -->
```

Esto evita que contenido malicioso en issues o merge requests manipule al asistente de IA.

---

## Cabeceras de seguridad HTTP

En modo HTTP, el servidor añade cabeceras de seguridad a todas las respuestas:

| Cabecera | Valor |
|----------|-------|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Content-Security-Policy` | `default-src 'none'; frame-ancestors 'none'` |
| `Cache-Control` | `no-store` |
| `Referrer-Policy` | `no-referrer` |

### Rate limiting

Se aplica un límite de **10 intentos de autenticación fallidos por IP por minuto**. Las peticiones que superen el límite son bloqueadas temporalmente.

### Protección contra DNS rebinding

Cuando el servidor escucha en `localhost`, valida la cabecera `Host` para prevenir ataques de DNS rebinding. Esta protección se desactiva automáticamente para `0.0.0.0`.

---

## Recomendaciones

!!! tip "Buenas prácticas"
    - Usa alcance `read_api` si no necesitas escritura
    - Rota los tokens periódicamente
    - Nunca compartas tu `.env` ni lo incluyas en repositorios
    - En modo HTTP, usa HTTPS con un reverse proxy (nginx, Caddy)
    - Habilita la verificación PGP en auto-actualización para entornos sensibles
