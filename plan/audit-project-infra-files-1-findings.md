# Findings — Audit Project Infrastructure Files (Phase 1 Research)

> Generated as part of plan [audit-project-infra-files-1.md](audit-project-infra-files-1.md). Each section captures up-to-date (2026) best practices and concrete deltas vs the current repo state. Sources are cited inline.

## TASK-001 — GoReleaser v2 best practices

**Versión actual:** GoReleaser v2.15 es el último release estable (abr 2026). El blog menciona Flatpak/SRPM como novedades; nada disruptivo respecto al `version: 2` actual.

**Fuentes:** <https://goreleaser.com/customization/>, `/customization/builds/builders/go/`, `/customization/sbom/`, `/customization/sign/`, `/customization/publish/mcp/`, `/customization/publish/changelog/`, `/customization/publish/snapshots/`.

### Hallazgos accionables

1. **🔴 MCP Registry publisher nativo (`mcp:` top-level)** — Desde v2.13. Reemplaza completamente el bloque actual del workflow `release.yml` que descarga `mcp-publisher` desde GitHub Releases (sin SHA), hace login con GitHub y publica. GoReleaser genera el `server.json` directamente y soporta `auth.type: github | github-oidc | none`. Resultado: elimina la descarga de binario sin verificar y `continue-on-error: true`. Ref: `/customization/publish/mcp/`.
   - Schema generado: `https://static.modelcontextprotocol.io/schemas/2025-10-17/server.schema.json` (NB: nuestro `server.json` actual apunta a 2025-12-11; verificar qué usa GoReleaser exactamente — puede que deba esperar update upstream).
   - Soporta `packages.registry_type`: `oci`, `npm`, `pypi`, `nuget`, `mcpb`. Cubre nuestros 6 paquetes actuales.
   - **Tip crítico**: si se publica con `oci`, la imagen debe llevar la label `io.modelcontextprotocol.server.name`.

2. **🟡 Reproducible builds** — Recomendaciones oficiales GoReleaser:
   - Añadir `builds[].mod_timestamp: "{{ .CommitTimestamp }}"` (actualmente: no presente).
   - Añadir `-trimpath` a `builds[].flags` para evitar paths absolutos en el binario (revisar `.goreleaser.yml` actual).
   - Cambiar `-X main.date={{.Date}}` por `{{.CommitDate}}` o eliminar la variable date.

3. **🟡 Cosign moderno con `--bundle`** — La doc actualizada usa `signature: "${artifact}.sigstore.json"` con `--bundle=${signature}` (formato sigstore-bundle, single-file con cert + signature). Si el repo aún usa `.bundle` o archivos `.cert` + `.sig` separados, migrar a `.sigstore.json`. Verifier: `cosign verify-blob --bundle file.tar.gz.sigstore.json file.tar.gz`.

4. **🟡 SBOM (`sboms:`)**:
   - Default `artifacts: archive` y `args: ["$artifact", "--output", "spdx-json=$document", "--enrich", "all"]` — `--enrich all` enriquece con info de vulnerabilidades/licencias. Confirmar que el repo aplica esto.
   - Default `documents: ["{{ .ArtifactName }}.sbom.json"]`. Si el actual usa `formats: [binary]` (no soportado en `sboms`, ese campo va en `archives`), revisar mezcla.
   - El default `env: [SYFT_FILE_METADATA_CATALOGER_ENABLED=true]` ayuda con auditorías SCA.

5. **🟡 Snapshot** — Default `version_template: "{{ .Version }}-SNAPSHOT-{{.ShortCommit}}"`. ⚠️ El campo correcto es `version_template`, no `name_template` (el último se mantiene como alias deprecated). Si el `.goreleaser.yml` actual usa `name_template`, marcarlo para migración.

6. **🟢 Changelog mejorable** — `use: github` (no `git`) con `format` que incluya `{{ .Logins | englishJoin }}` (v2.14+) para listar autores con `@username`. Permite `groups` + `filters.exclude` para `^docs:`, `(?i)typo`, etc., produciendo release notes estructuradas. Alternativa simple: `use: github-native` (delega 100% a GitHub API; sin grupos). Si se quiere mantener control fino, mantener `git` con `groups`.

7. **🟢 Schema pinning** — Añadir `# yaml-language-server: $schema=https://goreleaser.com/static/schema.json` en la cabecera del `.goreleaser.yml` para autocompletado en IDE. Para reproducibilidad estricta, pin a tag concreto: `https://raw.githubusercontent.com/goreleaser/goreleaser/v2.15.4/www/docs/static/schema.json`.

8. **🟢 Docker v2 (`dockers_v2:`)** — Es la nueva sintaxis recomendada (la antigua `dockers:` está marcada deprecated). Ofrece soporte nativo de multi-arch + labels OCI. Migración pendiente si actualmente se usa `dockers:`.

9. **🟢 Otros publishers no usados que pueden interesar a futuro** (no hace falta activar ahora):
   - `npms:` para distribución como paquete npm (`@jmrplens/gitlab-mcp-server`).
   - `winget:` para Microsoft Store.
   - `homebrew_casks:` (Homebrew).
   - `nfpms:` para `.deb`/`.rpm`/`.apk`.
   - `attestations:` (GitHub provenance attestations) — alternativa moderna a SBOM/sigs separados.

### Resumen para Phase 4 (release pipeline)

| Cambio | Tipo | Donde |
| ------ | ---- | ----- |
| Migrar publish MCP a `mcp:` nativo de GoReleaser | refactor mayor | `.goreleaser.yml` + `release.yml` |
| Añadir `mod_timestamp: "{{ .CommitTimestamp }}"` y `-trimpath` | mejora | `.goreleaser.yml` builds |
| Verificar `sboms.args` incluye `--enrich all` | revisión | `.goreleaser.yml` sboms |
| Verificar `signs` usa `${artifact}.sigstore.json` con `--bundle` | revisión | `.goreleaser.yml` signs |
| Añadir schema pin en cabecera | trivial | `.goreleaser.yml` |
| Reemplazar download de `mcp-publisher` sin SHA | crítico | `.github/workflows/release.yml` |
| Eliminar `continue-on-error: true` en step MCP publish | crítico | `.github/workflows/release.yml` |

## TASK-002 — Dockerfile best practices 2026

**Fuentes:** <https://docs.docker.com/build/building/best-practices/>, <https://github.com/GoogleContainerTools/distroless>, <https://github.com/hadolint/hadolint/wiki> (DL3000–DL4006).

**Estado actual** (`Dockerfile` revisado): `syntax=docker/dockerfile:1@sha256:…` pinned ✓, multi-stage ✓, `CGO_ENABLED=0 -trimpath -buildmode=pie -ldflags="-s -w …"` ✓, non-root con `addgroup -S/adduser -S` ✓, `ca-certificates + tzdata` en runtime ✓, `HEALTHCHECK` con wget ✓, `EXPOSE 8080` ✓.

### Deltas accionables

1. **🔴 Pin de bases por digest** — Falta. Buenas prácticas Docker oficiales: pin `FROM alpine:3.23@sha256:…` y `FROM golang:1.26-alpine@sha256:…`. Docker Scout puede emitir PRs automáticos para mantenerlos actualizados (Renovate/Dependabot también soportan `digest` updates en `docker` ecosystem).

2. **🔴 OCI image labels ausentes** — Añadir set estándar (`org.opencontainers.image.*`) más la label MCP requerida:
   ```dockerfile
   ARG VERSION="" COMMIT="" BUILD_DATE=""
   LABEL org.opencontainers.image.title="gitlab-mcp-server" \
         org.opencontainers.image.description="MCP server for GitLab REST API v4" \
         org.opencontainers.image.source="https://github.com/jmrplens/gitlab-mcp-server" \
         org.opencontainers.image.documentation="https://github.com/jmrplens/gitlab-mcp-server/tree/main/docs" \
         org.opencontainers.image.version="${VERSION}" \
         org.opencontainers.image.revision="${COMMIT}" \
         org.opencontainers.image.created="${BUILD_DATE}" \
         org.opencontainers.image.licenses="MIT" \
         org.opencontainers.image.authors="jmrplens" \
         io.modelcontextprotocol.server.name="io.github.jmrplens/gitlab-mcp-server"
   ```
   La label `io.modelcontextprotocol.server.name` es **requerida** por el MCP Registry para paquetes `oci` (ver TASK-001 — tip de GoReleaser).

3. **🟡 BuildKit cache mounts** — Acelera CI ~2× sin coste de imagen final:
   ```dockerfile
   RUN --mount=type=cache,target=/go/pkg/mod \
       --mount=type=cache,target=/root/.cache/go-build \
       go mod download
   RUN --mount=type=cache,target=/go/pkg/mod \
       --mount=type=cache,target=/root/.cache/go-build \
       CGO_ENABLED=0 go build …
   ```

4. **🟡 UID/GID explícitos** — Recomendación oficial Docker (sección USER): asignar UID/GID fijos para evitar drift entre rebuilds y para que Kubernetes `runAsUser` sea estable:
   ```dockerfile
   RUN addgroup -S -g 10001 appgroup && \
       adduser  -S -u 10001 -G appgroup -h /home/appuser appuser
   ```

5. **🟡 TARGETPLATFORM / multi-arch awareness** — Para que `docker buildx --platform=linux/amd64,linux/arm64` funcione automáticamente con la misma Dockerfile sin GoReleaser:
   ```dockerfile
   ARG TARGETOS TARGETARCH
   RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build …
   ```
   Hoy el build sólo respeta `GOOS/GOARCH` por defecto (host). Con esto, una build local `docker buildx build --platform=linux/arm64 .` produce binario arm64 cross-compilado.

6. **🟢 Distroless como alternativa runtime** — `gcr.io/distroless/static-debian12:nonroot` (~2 MiB vs alpine ~5 MiB), CA roots incluidos, `nonroot` user pre-creado (UID 65532). Trade-offs:
   - **Pro:** superficie ataque mínima, no shell, no apk, signed con cosign keyless por Google.
   - **Contra:** sin `wget`/`curl` → `HEALTHCHECK` interno deja de funcionar; hay que mover el healthcheck a un endpoint Go interno o delegar a probes K8s.
   - **Sugerencia:** ofrecer dos targets — `Dockerfile` (alpine, con wget) y `Dockerfile.distroless` (production-grade) — o añadir un sub-comando `gitlab-mcp-server healthcheck` que haga `GET /health` y use `HEALTHCHECK CMD ["gitlab-mcp-server","healthcheck"]`. Esto último permite distroless sin perder healthcheck.

7. **🟢 Hadolint** — Instalable como linter en CI o pre-commit:
   ```yaml
   # .github/workflows/lint.yml
   - uses: hadolint/[email protected]
     with:
       dockerfile: Dockerfile
       failure-threshold: warning
   ```
   Reglas a observar dado el Dockerfile actual:
   - **DL3007** *(usar `latest` está prohibido — pero NO se usa ✓)*.
   - **DL3008** *(pin de versiones apk add — actualmente `apk add --no-cache git ca-certificates` sin `=version`)*. La doc oficial Docker reconoce que en Alpine es aceptable confiar en `apk` rolling sin pin estricto, pero hadolint marca warning. Aceptable suprimir o pinear si se quiere reproducibilidad estricta.
   - **DL3018** *(versiones apk pinneadas)*.
   - **DL3025** *(usar formato JSON `["arg","arg"]` en CMD/ENTRYPOINT — ya se cumple ✓)*.
   - **DL3059** *(múltiples `RUN` consecutivos consolidables — bajo riesgo aquí)*.

8. **🟢 `.dockerignore`** — Verificar que existe y excluye `.git/`, `dist/`, `*.md`, `test/`, `docs/`, `site/`, `node_modules/`. Reduce contexto enviado al daemon → builds más rápidos y evita filtrar secrets/credenciales accidentalmente.

### Resumen para Phase 6 (Docker)

| Cambio | Severidad | Línea |
| ------ | --------- | ----- |
| Pin `FROM` con digest sha256 | 🔴 alta | L4, L21 |
| Añadir `LABEL org.opencontainers.image.*` + `io.modelcontextprotocol.server.name` | 🔴 alta | L20 |
| BuildKit cache mounts en `go mod download` y `go build` | 🟡 media | L10, L16 |
| UID/GID explícitos (10001) | 🟡 media | L23 |
| `ARG TARGETOS TARGETARCH` para multi-arch | 🟡 media | L13 |
| Considerar distroless static + healthcheck Go | 🟢 baja | L21 |
| Añadir `hadolint` CI gate | 🟢 baja | nuevo workflow |

## TASK-003 — MCP Registry server.json schema (2025-12-11)

**Versión actual:** schema 2025-12-11 (último). Nuestro `server.json` lo referencia correctamente y **valida sin errores** contra el schema oficial (`jsonschema` Python).

**Fuentes:**
- Schema raw: <https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json>
- Guías: <https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/server-json/generic-server-json.md>
- Publicación: <https://github.com/modelcontextprotocol/registry/blob/main/docs/guides/publishing/publish-server.md>

### Resumen del schema 2025-12-11

| Campo top-level         | Requerido | Constraint clave                                    |
| ----------------------- | --------- | --------------------------------------------------- |
| `name`                  | ✅ sí      | regex `^[a-zA-Z0-9.-]+/[a-zA-Z0-9._-]+$`, max 200   |
| `description`           | ✅ sí      | **maxLength: 100** (¡crítico!), minLength 1         |
| `version`               | ✅ sí      | rechaza ranges (`^1.x`, `~1.0`, `>=`, `latest`)     |
| `title`                 | opcional  | maxLength 100                                       |
| `repository`            | opcional  | recomendado para transparencia                      |
| `websiteUrl`            | opcional  | format: uri                                         |
| `icons`                 | opcional  | array de `Icon` con `theme` y `sizes`               |
| `packages`              | opcional  | array de `Package`                                  |
| `remotes`               | opcional  | array de `RemoteTransport`                          |
| `_meta`                 | opcional  | namespacing reverse-DNS para vendor data            |
| `$schema`               | opcional  | URI del schema usado                                |

### Constraints `Package` (todos requeridos: `registryType`, `identifier`, `transport`)

- `registryType`: examples = `npm | pypi | oci | nuget | mcpb` (no es enum cerrado, son ejemplos)
- `fileSha256`: regex `^[a-f0-9]{64}$`. **Requerido para mcpb**, opcional para otros.
- `version`: rechaza `"latest"` (`not.const`)
- `transport`: anyOf `StdioTransport | StreamableHttpTransport | SseTransport`
- `runtimeArguments` debe llevar `runtimeHint` cuando se usa
- `packageArguments` y `runtimeArguments` son arrays de `Argument` (positional o named)

### `KeyValueInput` (env vars y headers)

| Campo         | Tipo                                              |
| ------------- | ------------------------------------------------- |
| `name`        | string (requerido)                                |
| `description` | string                                            |
| `default`     | string                                            |
| `placeholder` | string                                            |
| `format`      | enum `string | number | boolean | filepath`      |
| `isRequired`  | boolean (default false)                           |
| `isSecret`    | boolean (default false)                           |
| `choices`     | array<string>                                     |
| `value`       | string fijo (no editable). Soporta `{curly_braces}` con `variables` |
| `variables`   | mapping para sustitución en `value`               |

### Hallazgos accionables

1. **🟢 Schema válido** — `python -m jsonschema` confirma `server.json` cumple 2025-12-11. No hay errores.
2. **🟢 Description = 91 chars** — dentro del límite 100. Sin riesgo.
3. **🟡 Duplicación de `environmentVariables`** — el bloque idéntico se repite 6 veces (uno por package). El schema **no permite** centralizar a nivel top-level; cada package debe declarar sus env vars. Solución: generar `server.json` con un script que use un template (Jinja/Go template) en CI, o aceptar la duplicación.
4. **🟡 Vars de código no expuestas en server.json** — `EMBEDDED_RESOURCES`, `GITLAB_IGNORE_SCOPES` existen en `internal/config/config.go` (líneas 134, 139) pero no en `server.json`. Las vars HTTP-only (`MAX_HTTP_CLIENTS`, `SESSION_TIMEOUT`, `SESSION_REVALIDATE_INTERVAL`, `AUTH_MODE`, `OAUTH_CACHE_TTL`, `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`) están bien excluidas porque los packages declaran `transport.type: stdio`.
5. **🟡 Discrepancia GoReleaser nativo `mcp:`** — El publisher GoReleaser v2.15 emite schema **2025-10-17**, no 2025-12-11. Si se migra al publisher nativo (TASK-001 hallazgo #1), nuestro `server.json` curado pasaría a regenerarse con schema más antiguo. Decisión: mantener publish manual con `mcp-publisher v1.7.2` hasta que GoReleaser actualice.
6. **🟢 `_meta.io.modelcontextprotocol.registry/publisher-provided`** — campo opcional para info de build (commit, pipelineId, timestamp). Lo añade automáticamente el registry, no manualmente.
7. **🟢 `runtimeHint`** — actualmente no se usa porque los binarios son ejecutables nativos sin runtime. Solo aplicaría si añadimos paquete OCI (`docker run`).
8. **🟢 `value` + `variables` (NUEVO 2025-12-11)** — útil para vars con placeholders parametrizables. No aplica a nuestro caso (todas las env vars son configuración directa).

### Resumen

| Acción                                            | Severidad | Impacto                                  |
| ------------------------------------------------- | --------- | ---------------------------------------- |
| `server.json` ya válido contra 2025-12-11         | ✅ none    | sin trabajo                              |
| Generar `server.json` con template para evitar duplicación de envVars | 🟡 media | reduce maintenance (~480 líneas dup) |
| Añadir `EMBEDDED_RESOURCES` y `GITLAB_IGNORE_SCOPES` a `environmentVariables` | 🟡 baja | docs completos para clientes MCP |
| Documentar discrepancia schema GoReleaser (2025-10-17) vs server.json curado (2025-12-11) | 🟡 baja | ADR o nota en `docs/development/` |
| Añadir step `pip install jsonschema && python -m jsonschema --instance server.json /tmp/schema.json` al CI | 🟢 baja | gate de validación pre-merge |

## TASK-004 — GitHub Issue Forms schema

_Pending research._

## TASK-005 — OWASP Vulnerability Disclosure & SECURITY.md templates

_Pending research._

## TASK-006 — golangci-lint v2 mature linters

_Pending research._

## TASK-007 — Consolidated findings table

_Will be filled after TASK-001..006._
