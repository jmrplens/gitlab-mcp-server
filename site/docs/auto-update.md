# Auto-actualización

Sistema de actualización automática integrado en Gitlab MCP.

---

## Modos de actualización

| Modo | Valor | Comportamiento |
|------|-------|---------------|
| **Automático** | `true` | Descarga y aplica automáticamente |
| **Solo comprobar** | `check` | Registra en log, no aplica |
| **Desactivado** | `false` | Sin comprobaciones |

Configuración via variable de entorno o flag:

```bash
# Variable de entorno
AUTO_UPDATE=true

# Flag de línea de comandos
./gitlab-mcp-server --auto-update=check
```

---

## Funcionamiento

### Modo Stdio (pre-arranque)

En modo stdio, la actualización se ejecuta **antes** de iniciar el servidor MCP:

1. Consulta la API de releases de GitLab en el repositorio configurado
2. Compara la versión local con la última release
3. Si hay una versión nueva:
      - Descarga el binario para la plataforma actual
      - Verifica firma PGP (si está configurada)
      - Reemplaza el binario en disco
      - Re-ejecuta el proceso con el nuevo binario

!!! info "Guard de re-ejecución"
    La variable de entorno `PE_MCP_JUST_UPDATED` previene bucles infinitos de re-ejecución. Se establece automáticamente después de una actualización.

### Modo HTTP (periódica)

En modo HTTP, las comprobaciones se ejecutan periódicamente en background:

```bash
# Intervalo de comprobación (defecto: 1h)
./gitlab-mcp-server --http --auto-update-interval=30m
```

| Parámetro | Variable | Defecto | Descripción |
|-----------|----------|---------|-------------|
| `--auto-update-interval` | `AUTO_UPDATE_INTERVAL` | `1h` | Intervalo entre comprobaciones |
| `--auto-update` | `AUTO_UPDATE` | `true` | Modo de actualización |
| `--auto-update-repo` | `AUTO_UPDATE_REPO` | `jmrplens/gitlab-mcp-server` | Repositorio de releases |

---

## Verificación PGP

Opcionalmente, puedes verificar la firma criptográfica de los binarios:

```bash
./gitlab-mcp-server --auto-update-gpg-key=/ruta/a/clave-publica.asc
```

O via variable de entorno:

```bash
AUTO_UPDATE_GPG_KEY=/ruta/a/clave-publica.asc
```

Cuando está habilitada:

1. Descarga `checksums.txt` y `checksums.txt.asc` de la release
2. Verifica la firma PGP de los checksums
3. Verifica el hash SHA-256 del binario contra los checksums firmados
4. Si la verificación falla, la actualización se rechaza

!!! tip "Fallback seguro"
    Si la clave PGP es inválida o no se puede cargar, el sistema hace fallback a verificación de checksum sin PGP (sin firma). Esto evita que un error de configuración bloquee las actualizaciones.

---

## Reemplazo de binario

### Linux / macOS

1. El nuevo binario se descarga a un archivo temporal
2. Se renombra el binario actual a `<nombre>.old`
3. Se mueve el nuevo binario a la ruta original
4. Se re-ejecuta el proceso con `syscall.Exec` (reemplazo in-place)

### Windows

1. Mismo proceso de descarga y renombrado
2. El binario se reemplaza, pero el cambio toma efecto en el **siguiente reinicio**

### Terminaci?n externa (`--shutdown`)

Un actualizador externo (como un actualizador externo) puede invocar `gitlab-mcp-server --shutdown` para terminar todas las instancias en ejecuci?n antes de reemplazar el binario en disco:

```bash
# Paso 1: Terminar todas las instancias
gitlab-mcp-server --shutdown

# Paso 2: Reemplazar el binario
# Paso 3: El cliente MCP reinicia autom?ticamente con la nueva versi?n
```

Comportamiento:

1. Busca todos los procesos con el mismo nombre de binario (multiplataforma)
2. Env?a se?al de terminaci?n graceful (SIGTERM en Unix, TerminateProcess en Windows)
3. Espera hasta 5 segundos a que los procesos terminen
4. Fuerza la terminaci?n de los procesos que no respondieron
5. Sale con c?digo 0 en ?xito

---

## Timeout

La comprobación de actualización tiene un timeout configurable:

```bash
# Defecto: 15 segundos
AUTO_UPDATE_TIMEOUT=30s
```

Si el servidor de releases no responde dentro del timeout, la actualización se cancela silenciosamente y el servidor arranca con la versión actual.

---

## Repositorio de actualización

Por defecto, las actualizaciones se buscan en `jmrplens/gitlab-mcp-server` en la instancia GitLab configurada. Puedes cambiarlo:

```bash
./gitlab-mcp-server --auto-update-repo=mi-grupo/mi-fork
```

!!! warning "Seguridad"
    La URL de actualización siempre debe ser HTTPS. El sistema rechaza URLs HTTP para prevenir ataques man-in-the-middle.
