# MCP Tool Icons

Visual identity for every tool, resource, and prompt in gitlab-mcp-server.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, contributors, integrators

## Overview

gitlab-mcp-server ships **50 unique domain icons** assigned to all 1073 self-managed Enterprise/Premium tools (1079 on GitLab.com Enterprise/Premium with Orbit), 32 base meta-tools (49 self-managed Enterprise, 50 GitLab.com Enterprise), 45 resources, and 37 prompts. Each icon carries an SVG entry plus light/dark WebP fallbacks (see [Encoding Format](#encoding-format)), so MCP clients render recognizable UI elements for each GitLab domain (branches, issues, pipelines, merge requests, Orbit, etc.) whether or not they accept SVG.

Icons are defined in [`internal/toolutil/icons.go`](../../../internal/toolutil/icons.go) and consumed via the `Icons` field on every `mcp.Tool`, `mcp.Resource`, and `mcp.Prompt` registration.

## MCP Specification

Icons follow the [MCP Icon interface](https://modelcontextprotocol.io/specification/2025-11-25) (protocol version 2025-11-25):

```typescript
interface Icon {
  src: string;          // URI pointing to the icon (HTTP/HTTPS or data: URI)
  mimeType?: string;    // MIME type override
  sizes?: string[];     // Available sizes
  theme?: "light" | "dark"; // Theme hint
}
```

### Client MIME Type Support

| MIME Type       | Support Level      | Notes                          |
| --------------- | ------------------ | ------------------------------ |
| `image/png`     | **MUST** support   | Universal compatibility        |
| `image/jpeg`    | **MUST** support   | Universal compatibility        |
| `image/svg+xml` | **SHOULD** support | Scalable, used by this project |
| `image/webp`    | **SHOULD** support | Modern efficient format        |

gitlab-mcp-server ships every icon as **three entries**: one `image/svg+xml`
entry plus two `image/webp` fallbacks, so a client that rejects the SVG
entry (an allowed choice — SVG is only **SHOULD**-level per the table above)
still renders an icon instead of none. The entry **order is a stable
contract**: `icons[0]` is always the canonical SVG (`sizes: ["any"]`, no
`theme`), `icons[1]` the light-theme 16×16 WebP, `icons[2]` the dark-theme
16×16 WebP. Consumers should still select by `mimeType` and `theme`, but a
positional reader can rely on the SVG staying first — the order is pinned by
unit tests and will not change without a documented break. Clients that support neither format
(PNG/JPEG-only, MUST-level clients) will not render these icons.

### Client Compatibility

Sourced from a primary-source (client source code and official docs, not
marketing copy) survey of 15 MCP clients on 2026-08-24, cross-checked
against each client's actual behavior rather than the spec's aspirational
"SHOULD support" language:

| MCP Client                        | Renders | Notes                                                                                                                                                                                                                                                                |
| --------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| VS Code (GitHub Copilot)          | Yes     | Rejects the SVG entry (`image/svg+xml` not in its MIME allowlist) but renders the `Theme`-matched WebP fallback                                                                                                                                                      |
| Cursor                            | Unclear | Accepts the SVG format in validation, but server-icon rendering itself was staff-confirmed absent as of March 2026 (an unconfirmed hint of improvement followed in June 2026); tool/resource/prompt icon rendering unverified either way                             |
| OpenAI Codex (CLI + IDE core)     | Unclear | Server and tool icons are captured and forwarded through Codex's internal protocol, but no code that reads them back out for rendering was found anywhere in the open-source TUI; the closed-source VS Code extension and ChatGPT desktop app could not be inspected |
| Windsurf / Devin Desktop          | Unknown | No icon documentation found anywhere; closed source, no public client to inspect                                                                                                                                                                                     |
| JetBrains AI Assistant / Junie    | No      | Settings docs enumerate only generic action-button icons (add/remove/edit/reconnect) and a connection-status glyph — no server-supplied icon field; resources and prompts remain wholly unsupported (JUNIE-1606/1607, both open)                                     |
| Claude Desktop                    | No      | The protocol-level `Implementation.icons`/`Tool.icons` fields are not confirmed rendered anywhere; a *separate*, unrelated `.mcpb` manifest icon mechanism does work for locally-installed extensions, which is not what this project ships                          |
| Claude Code                       | No      | Text-only interface — a generic placeholder and the raw tool name, confirmed by several open feature requests asking for exactly this                                                                                                                                |
| Kiro (AWS)                        | No      | Prompts and resource templates show a generic "MCP" protocol badge next to every entry, not the per-item `Icons` field this project populates                                                                                                                        |
| Zed                               | No      | Verified in source: none of its `Implementation`/`Tool`/`Resource`/`Prompt` types carry an `icons` field at all                                                                                                                                                      |
| Cline                             | No      | No icon rendering found anywhere in the client (verified in source)                                                                                                                                                                                                  |
| Continue.dev                      | No      | Does not read the MCP Icons field in any version, in any format; has an unrelated proprietary `faviconUrl` setting instead                                                                                                                                           |
| Goose (Block)                     | No      | The underlying `rmcp` SDK carries the field; Goose's own client code never reads it back out                                                                                                                                                                         |
| LibreChat                         | No      | No icon rendering found anywhere in the client                                                                                                                                                                                                                       |
| Open WebUI                        | No      | No icon rendering found anywhere in the client                                                                                                                                                                                                                       |
| 5ire / Witsy                      | No      | No icon rendering found in either client                                                                                                                                                                                                                             |
| *Custom clients (direct LLM API)* | Depends | Renders whichever entries its own MCP client implementation honors — see [Building a Custom MCP Client](#building-a-custom-mcp-client) below for a concrete selection algorithm                                                                                      |

## Building a Custom MCP Client

If you're talking to an MCP server directly over JSON-RPC — no VS Code, Claude
Desktop, or Cursor in between — icon rendering is entirely your client's job.
`icons` is optional, additive metadata (SEP-973, protocol revision 2025-11-25):
a server that ignores it (Zed, Continue.dev, and most hand-rolled clients today)
is still fully spec-compliant. Everything below is for clients that *do* want
to render a UI.

### Where to find `icons`

The array shows up on four object types, wherever your client already reads
their name/description:

| Carrier                         | Where it comes from                                                           |
| ------------------------------- | ----------------------------------------------------------------------------- |
| `Implementation`                | `serverInfo.icons` in the `initialize` response — the server's own brand mark |
| `Tool`                          | each entry in `tools/list`                                                    |
| `Resource` / `ResourceTemplate` | each entry in `resources/list`                                                |
| `Prompt`                        | each entry in `prompts/list`                                                  |

Each `Icon` is `{ src, mimeType?, sizes?, theme? }`. `src` is a `data:` or
`https:`/`http:` URI. Treat an absent or empty `icons` array as "no icon for
this object" and fall back to a generic per-type glyph (wrench for tools,
document for resources, etc.) — never fail the call over it.

### Selection algorithm

Run every candidate icon array through three filters, in this order. Stop as
soon as one candidate survives; if none do, fall back to your generic glyph.

1. **Format support.** Decide up front what your renderer can display —
   typically raster formats your UI toolkit/image decoder handles natively
   (`image/png`, `image/jpeg`, `image/webp`, `image/gif`), plus `image/svg+xml`
   *only if* you've built the sanitization path below. Filter on the declared
   `mimeType` when present; when it's absent, sniff the actual bytes (magic
   number for `data:`, or a `HEAD`/`Content-Type` check for `https:`) rather
   than guessing from the URI's file extension — `mimeType` is
   server-supplied, untrusted metadata, not a guarantee.
2. **Theme match.** If your client has a light/dark UI theme, prefer icons
   whose `theme` matches it. Critically: **an icon with no `theme` field is
   theme-agnostic, not unthemed** — it's designed to render correctly
   regardless of host theme (e.g. an SVG using `currentColor` that inherits
   your UI's text color). Match it in *both* light and dark modes rather than
   skipping it or treating it as a coin-flip default. Concretely:
   gitlab-mcp-server ships three `Icon` entries per icon — one SVG with
   `sizes: ["any"]` and no `theme` (adapts via `currentColor`), plus a WebP
   pair with `sizes: ["16x16"]` and explicit `theme: "light"` /
   `theme: "dark"` (pre-rendered near-black / near-white). A correct client
   either renders the SVG (it's already theme-correct, so this step is a
   no-op) or, if it can't do SVG, picks the WebP whose `theme` equals its
   current UI theme.
3. **Size match, with fallback.** `sizes: ["any"]` means scalable/vector — it
   satisfies any requested size, so treat it as your first choice whenever
   your format filter allows SVG. For raster candidates, parse each `WxH`
   string, sort ascending, and pick the smallest one that's still `>= your
   target render size` (this is what VS Code does) rather than the nearest by
   absolute difference — an oversized icon downscales cleanly; an undersized
   one doesn't upscale cleanly. If nothing meets the target, take the largest
   available rather than failing.

### Security checklist

Icons come from a server you may not control, so treat every field as
untrusted input, per the spec's own SHOULDs:

- **Scheme allowlist.** Accept only `data:` and `https:` (treat plain `http:`
  as untrusted unless you're deliberately supporting local/dev HTTP-transport
  servers). Reject everything else (`file:`, `javascript:`, etc.) outright.
- **Domain check on remote URLs.** The spec says consumers SHOULD verify an
  `https:`/`http:` icon URL is same-domain as the client/server or otherwise
  trusted — don't blindly render an `<img src>` pointing at an arbitrary
  third party. VS Code's approach is a good concrete bar: for a server
  reached over HTTP transport, only accept icon URLs whose authority matches
  that same server's authority; reject anything pointing elsewhere.
- **`data:` URIs still need validation.** Verify the declared/sniffed MIME is
  one you actually allow, cap the decoded byte size before handing it to your
  image decoder (a malicious server can embed an oversized or malformed
  payload as a cheap DoS against your renderer), and reject on any decode
  error rather than best-effort passing it through.
- **Remote fetches are real network requests.** An `https:` icon isn't inert
  metadata — resolving it leaks the viewer's IP and enables tracking/beacon
  patterns, and if *you* fetch it server-side (e.g. to proxy or cache it)
  that's a user-controlled URL fetch and needs the same SSRF guards as any
  other: block private/link-local ranges, cap redirects, set a timeout and a
  byte-size ceiling.
- **SVG is the highest-risk format** — the spec calls this out explicitly:
  SVG can embed `<script>` and event-handler attributes. Two acceptable
  paths: (a) don't support `image/svg+xml` at all and drop straight to the
  next candidate (this is what VS Code does — simplest safe default), or (b)
  if you want crisp vector icons, sanitize before rendering — strip
  `<script>`, all `on*` attributes, `<foreignObject>`, and external
  references (e.g. with an SVG-profile DOMPurify config) before inlining the
  markup or handing it to your DOM/renderer. Never pass raw SVG bytes from an
  untrusted server straight into an inline `<svg>`/`dangerouslySetInnerHTML`-
  style sink.
- **Cap what you evaluate.** A hostile server can return hundreds of `Icon`
  entries per object; bound how many you inspect per selection call so a
  malformed response can't turn icon rendering into an amplification vector.

### Selection logic, end to end

```typescript
type Icon = { src: string; mimeType?: string; sizes?: string[]; theme?: "light" | "dark" };

const SUPPORTED_MIME = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
// Flip to true only once you've wired the sanitize-before-render path below.
const SUPPORTS_SANITIZED_SVG = false;

function selectIcon(
  icons: Icon[] | undefined,
  opts: { uiTheme: "light" | "dark"; targetPx: number; serverAuthority: string },
): Icon | undefined {
  if (!icons?.length) return undefined;

  const candidates = icons
    // 1. scheme allowlist + domain check
    .filter((i) => {
      const uri = safeParseUri(i.src);
      if (!uri) return false;
      if (uri.scheme === "data") return true;
      if (uri.scheme === "https" || uri.scheme === "http") {
        return uri.authority.toLowerCase() === opts.serverAuthority.toLowerCase();
      }
      return false;
    })
    // 2. format support (sniff, don't trust the label blindly)
    .filter((i) => {
      const mime = i.mimeType ?? sniffMimeType(i.src);
      if (mime === "image/svg+xml") return SUPPORTS_SANITIZED_SVG;
      return mime !== undefined && SUPPORTED_MIME.has(mime);
    });

  if (!candidates.length) return undefined;

  // 3. theme: exact match OR theme-agnostic (no `theme` field at all)
  const themed = candidates.filter((i) => i.theme === undefined || i.theme === opts.uiTheme);
  const pool = themed.length ? themed : candidates;

  // 4. size: prefer scalable ("any"), else smallest raster >= target, else largest available
  const scalable = pool.find((i) => i.sizes?.includes("any"));
  if (scalable) return scalable;

  const bySize = pool
    .flatMap((i) => (i.sizes ?? []).map((s) => ({ icon: i, ...parseWxH(s) })))
    .sort((a, b) => a.width - b.width);

  return (
    bySize.find((c) => c.width >= opts.targetPx)?.icon ??
    bySize[bySize.length - 1]?.icon ??
    pool[0]
  );
}

// Render step (only reached for icon.mimeType === "image/svg+xml"):
// svgMarkup = fetchOrDecode(icon.src)
// safeMarkup = sanitizeSvg(svgMarkup) // strip <script>, on*, <foreignObject>, external refs
// renderInline(safeMarkup)
```

This gives you graceful degradation at every layer: no `icons` field → your
generic glyph; icons present but none pass your format/scheme/domain filters
→ same generic glyph; a full match → the theme- and size-correct variant,
rendered through a path that never trusts server-supplied bytes further than
necessary.

## Implementation Details

### Encoding Format

All icons use inline **base64-encoded data URIs** to avoid external network dependencies and to comply with strict client URI parsers (some MCP clients reject percent-encoded SVG markup):

```text
data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIC4uLj4uLi48L3N2Zz4=
data:image/webp;base64,UklGRiYAAABXRUJQVlA4...
```

Each icon's `[]mcp.Icon` slice carries three entries:

| Entry | `MIMEType`      | `Sizes`     | `Theme`  | Fill                 |
| ----- | --------------- | ----------- | -------- | -------------------- |
| SVG   | `image/svg+xml` | `["any"]`   | *(none)* | `currentColor`       |
| WebP  | `image/webp`    | `["16x16"]` | `light`  | near-black `#1A1A1A` |
| WebP  | `image/webp`    | `["16x16"]` | `dark`   | near-white `#FAFAFA` |

The SVG entry is resolution-independent and theme-adaptive through
`currentColor`; every SVG-capable client renders it. The two WebP entries
are pre-rasterized at 16×16 (a raster format cannot declare
`Sizes: ["any"]`) and colored per theme rather than left in a single
saturated color, so a client that reads `Icon.Theme` gets an icon that
matches its UI instead of clashing with it. See
[`cmd/gen_icon_webp`](../../../cmd/gen_icon_webp/main.go) for the generator
and the size/format tradeoff it was built against: WebP lossless beat
optimized PNG by ~32% at the same 16×16 size; JPEG and GIF were disqualified
outright (JPEG has no alpha channel, and GIF's 256-color, 1-bit-alpha
palette would band the antialiased edges).

### Design Principles

- **16×16 viewport** — minimal size optimized for tool lists and sidebars
- **Single-path SVGs** — lightweight markup, fast parsing
- **`currentColor` fill on the SVG entry** — icons inherit the client's text color, adapting to light and dark themes automatically
- **`Theme`-paired WebP fallback** — clients that reject SVG still get a themed icon instead of none
- **No external dependencies** — data URIs embedded in the binary, zero network requests
- **One icon per domain** — related tools share the same icon for visual grouping

### Catalog Projection Pattern

Icons are attached per **catalog group**, not per package: `internal/tools/catalog_group_metadata.go` maps every group name (`gitlab_branch`, `gitlab_issue`, ...) to its icon, a group missing from that map falls back to `IconServer`, and a meta-tool, the individual tools projected from its actions, and the dynamic surface's `gitlab://tools` entries all inherit the group's icon. Each catalog-backed tool receives it when `RegisterAll` projects the canonical action catalog into individual MCP tools:

```go
func RegisterAll(server *mcp.Server, client *gitlabclient.Client, tier edition.Tier) *actioncatalog.Catalog {
    catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: tier, IncludeMCP: true})
    if err != nil {
        panic(fmt.Errorf("build individual action catalog: %w", err))
    }
    RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
        IncludeStandaloneUtilities: true,
        SchemaCacheKey:             "individual|" + tier.String(),
    })
    RegisterMetaStandaloneTools(server, client)
    return catalog
}
```

Resources and prompts name their icon directly at registration (`Icons: toolutil.IconIssue` on the `mcp.Resource` or `mcp.Prompt` literal).

The `icon()` helper in `toolutil` base64-encodes each SVG constant, reads its
pre-generated WebP fallbacks from an embedded filesystem, and wraps all
three as a `[]mcp.Icon` slice:

```go
//go:embed icons/webp/*.webp
var webpFS embed.FS

func icon(name, svg string) []mcp.Icon {
    encoded := base64.StdEncoding.EncodeToString([]byte(svg))
    return []mcp.Icon{
        {Source: "data:" + svgMIME + ";base64," + encoded, MIMEType: svgMIME, Sizes: []string{"any"}},
        webpIcon(name, "light", mcp.IconThemeLight),
        webpIcon(name, "dark", mcp.IconThemeDark),
    }
}
```

The WebP assets under `internal/toolutil/icons/webp/` are generated by
[`cmd/gen_icon_webp`](../../../cmd/gen_icon_webp/main.go) from the SVG
constants (`go run ./cmd/gen_icon_webp/`, or `make gen-icon-webp`) and
committed to the repository; ordinary builds only embed them, they never
regenerate them. Regeneration requires `rsvg-convert` (librsvg) and `cwebp`
(libwebp) on `PATH` and is a maintainer-only step — `make check-icon-webp`
verifies the committed assets are still current.

## Icon Gallery

All 50 domain icons with their SVG preview, exported variable name, and the catalog groups that carry each one. A group is a meta-tool (`gitlab_branch`, `gitlab_issue`, ...); the individual tools projected from its actions and the `gitlab://tools` entries on the dynamic surface inherit its icon, so a group name below stands for every tool it aggregates. Resources and prompts pick their icon directly and are listed in the [complete reference](#complete-icon-to-group-reference) further down. Twelve icons (`IconAlert`, `IconBot`, `IconContainer`, `IconDiscussion`, `IconEpic`, `IconImport`, `IconIntegration`, `IconLink`, `IconNotify`, `IconSchedule`, `IconTodo`, `IconUpload`) currently have no consumer on any served surface: the domains they were drawn for route under a broader group (`gitlab_admin`, `gitlab_group`, `gitlab_issue`, `gitlab_merge_request`, ...) and take that group's icon. The brand mark is documented separately below.

<!-- markdownlint-disable MD033 -->

### Source Control

| Preview                                                            | Name          | Catalog groups                                                                                                  |
| ------------------------------------------------------------------ | ------------- | --------------------------------------------------------------------------------------------------------------- |
| <img src="icons/branch.svg" width="32" height="32" alt="Branch">   | `IconBranch`  | `gitlab_branch`                                                                                                 |
| <img src="icons/commit.svg" width="32" height="32" alt="Commit">   | `IconCommit`  | none (resource `commit` and prompt `audit_commit_hygiene` only; commit actions route under `gitlab_repository`) |
| <img src="icons/tag.svg" width="32" height="32" alt="Tag">         | `IconTag`     | `gitlab_tag`                                                                                                    |
| <img src="icons/release.svg" width="32" height="32" alt="Release"> | `IconRelease` | `gitlab_release`                                                                                                |
| <img src="icons/file.svg" width="32" height="32" alt="File">       | `IconFile`    | `gitlab_repository`                                                                                             |

### Issues and Planning

| Preview                                                                | Name            | Catalog groups                                                                                                            |
| ---------------------------------------------------------------------- | --------------- | ------------------------------------------------------------------------------------------------------------------------- |
| <img src="icons/issue.svg" width="32" height="32" alt="Issue">         | `IconIssue`     | `gitlab_issue`                                                                                                            |
| <img src="icons/label.svg" width="32" height="32" alt="Label">         | `IconLabel`     | none (label resources and the `label_distribution` prompt; label actions route under `gitlab_project` and `gitlab_group`) |
| <img src="icons/milestone.svg" width="32" height="32" alt="Milestone"> | `IconMilestone` | none (milestone resources and prompts; milestone actions route under `gitlab_project` and `gitlab_group`)                 |
| <img src="icons/board.svg" width="32" height="32" alt="Board">         | `IconBoard`     | none (resource `board`; board actions route under `gitlab_project` and `gitlab_group`)                                    |
| <img src="icons/link.svg" width="32" height="32" alt="Link">           | `IconLink`      | none                                                                                                                      |
| <img src="icons/epic.svg" width="32" height="32" alt="Epic">           | `IconEpic`      | none (epic actions route under `gitlab_group`)                                                                            |
| <img src="icons/todo.svg" width="32" height="32" alt="Todo">           | `IconTodo`      | none (todo actions route under `gitlab_user`)                                                                             |

### Merge Requests

| Preview                                                                  | Name             | Catalog groups                                                      |
| ------------------------------------------------------------------------ | ---------------- | ------------------------------------------------------------------- |
| <img src="icons/mr.svg" width="32" height="32" alt="MR">                 | `IconMR`         | `gitlab_merge_request`, `gitlab_mr_review`                          |
| <img src="icons/discussion.svg" width="32" height="32" alt="Discussion"> | `IconDiscussion` | none (discussion and note actions route under their parent's group) |

### CI/CD

| Preview                                                              | Name           | Catalog groups                                                                                     |
| -------------------------------------------------------------------- | -------------- | -------------------------------------------------------------------------------------------------- |
| <img src="icons/pipeline.svg" width="32" height="32" alt="Pipeline"> | `IconPipeline` | `gitlab_pipeline`                                                                                  |
| <img src="icons/job.svg" width="32" height="32" alt="Job">           | `IconJob`      | `gitlab_job`                                                                                       |
| <img src="icons/runner.svg" width="32" height="32" alt="Runner">     | `IconRunner`   | `gitlab_runner`                                                                                    |
| <img src="icons/schedule.svg" width="32" height="32" alt="Schedule"> | `IconSchedule` | none (pipeline schedules route under `gitlab_pipeline`, freeze periods under `gitlab_environment`) |
| <img src="icons/variable.svg" width="32" height="32" alt="Variable"> | `IconVariable` | `gitlab_ci_variable`                                                                               |

### Environments and Deployments

| Preview                                                                    | Name              | Catalog groups                                                                    |
| -------------------------------------------------------------------------- | ----------------- | --------------------------------------------------------------------------------- |
| <img src="icons/environment.svg" width="32" height="32" alt="Environment"> | `IconEnvironment` | `gitlab_environment`                                                              |
| <img src="icons/deploy.svg" width="32" height="32" alt="Deploy">           | `IconDeploy`      | none (resource `deployment`; deployment actions route under `gitlab_environment`) |
| <img src="icons/infra.svg" width="32" height="32" alt="Infra">             | `IconInfra`       | `gitlab_geo`, `gitlab_storage_move`                                               |

### Projects and Groups

| Preview                                                            | Name          | Catalog groups                                                                         |
| ------------------------------------------------------------------ | ------------- | -------------------------------------------------------------------------------------- |
| <img src="icons/project.svg" width="32" height="32" alt="Project"> | `IconProject` | `gitlab_project`, `gitlab_project_alias`, and the standalone `gitlab_discover_project` |
| <img src="icons/group.svg" width="32" height="32" alt="Group">     | `IconGroup`   | `gitlab_group`, `gitlab_group_scim`                                                    |
| <img src="icons/queue.svg" width="32" height="32" alt="Queue">     | `IconQueue`   | `gitlab_merge_train`                                                                   |
| <img src="icons/user.svg" width="32" height="32" alt="User">       | `IconUser`    | `gitlab_user`, `gitlab_enterprise_user`                                                |
| <img src="icons/bot.svg" width="32" height="32" alt="Bot">         | `IconBot`     | none (service accounts route under `gitlab_group` and `gitlab_project`)                |

### Packages and Registry

| Preview                                                                | Name            | Catalog groups                                                 |
| ---------------------------------------------------------------------- | --------------- | -------------------------------------------------------------- |
| <img src="icons/package.svg" width="32" height="32" alt="Package">     | `IconPackage`   | `gitlab_package`, `gitlab_dependency`, `gitlab_model_registry` |
| <img src="icons/container.svg" width="32" height="32" alt="Container"> | `IconContainer` | none (the container registry routes under `gitlab_package`)    |

### Search and Analytics

| Preview                                                                | Name            | Catalog groups                                                   |
| ---------------------------------------------------------------------- | --------------- | ---------------------------------------------------------------- |
| <img src="icons/search.svg" width="32" height="32" alt="Search">       | `IconSearch`    | `gitlab_search`, and `gitlab_find_action` on the dynamic surface |
| <img src="icons/analytics.svg" width="32" height="32" alt="Analytics"> | `IconAnalytics` | `gitlab_dora_metrics`, `gitlab_orbit`                            |

### Security and Access

| Preview                                                                        | Name                | Catalog groups                                                                                                     |
| ------------------------------------------------------------------------------ | ------------------- | ------------------------------------------------------------------------------------------------------------------ |
| <img src="icons/security.svg" width="32" height="32" alt="Security">           | `IconSecurity`      | `gitlab_security_attribute`, `gitlab_security_category`, `gitlab_security_finding`, `gitlab_security_scan_profile` |
| <img src="icons/shield.svg" width="32" height="32" alt="Shield">               | `IconShield`        | `gitlab_attestation`, `gitlab_external_status_check`                                                               |
| <img src="icons/vulnerability.svg" width="32" height="32" alt="Vulnerability"> | `IconVulnerability` | `gitlab_vulnerability`                                                                                             |
| <img src="icons/compliance.svg" width="32" height="32" alt="Compliance">       | `IconCompliance`    | `gitlab_compliance_policy`                                                                                         |
| <img src="icons/token.svg" width="32" height="32" alt="Token">                 | `IconToken`         | `gitlab_access`                                                                                                    |
| <img src="icons/key.svg" width="32" height="32" alt="Key">                     | `IconKey`           | none (resource `deploy_key`; key actions route under `gitlab_access`, `gitlab_user` and `gitlab_group`)            |

### Documentation and Content

| Preview                                                            | Name          | Catalog groups                                       |
| ------------------------------------------------------------------ | ------------- | ---------------------------------------------------- |
| <img src="icons/wiki.svg" width="32" height="32" alt="Wiki">       | `IconWiki`    | `gitlab_wiki`, and the five workflow guide resources |
| <img src="icons/snippet.svg" width="32" height="32" alt="Snippet"> | `IconSnippet` | `gitlab_snippet`                                     |

### Configuration and Administration

| Preview                                                              | Name           | Catalog groups                                                                                                                                |
| -------------------------------------------------------------------- | -------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| <img src="icons/config.svg" width="32" height="32" alt="Config">     | `IconConfig`   | `gitlab_admin`, `gitlab_feature_flags`, `gitlab_member_role`, the `gitlab_interactive_*` wizards, and the `gitlab://tools` manifest resources |
| <img src="icons/server.svg" width="32" height="32" alt="Server">     | `IconServer`   | `gitlab_execute_action` on the dynamic surface, and the fallback for a group missing from the icon map                                        |
| <img src="icons/template.svg" width="32" height="32" alt="Template"> | `IconTemplate` | `gitlab_template`, `gitlab_ci_catalog`                                                                                                        |

### Notifications and Events

| Preview                                                          | Name         | Catalog groups                                                           |
| ---------------------------------------------------------------- | ------------ | ------------------------------------------------------------------------ |
| <img src="icons/notify.svg" width="32" height="32" alt="Notify"> | `IconNotify` | none (notification actions route under `gitlab_user` and `gitlab_admin`) |
| <img src="icons/event.svg" width="32" height="32" alt="Event">   | `IconEvent`  | `gitlab_custom_emoji`                                                    |
| <img src="icons/audit.svg" width="32" height="32" alt="Audit">   | `IconAudit`  | `gitlab_audit_event`                                                     |
| <img src="icons/alert.svg" width="32" height="32" alt="Alert">   | `IconAlert`  | none (alert and error-tracking actions route under `gitlab_admin`)       |

### Integrations and Operations

| Preview                                                                    | Name              | Catalog groups                                                                                   |
| -------------------------------------------------------------------------- | ----------------- | ------------------------------------------------------------------------------------------------ |
| <img src="icons/integration.svg" width="32" height="32" alt="Integration"> | `IconIntegration` | none (integration actions route under `gitlab_project`, system hooks under `gitlab_admin`)       |
| <img src="icons/health.svg" width="32" height="32" alt="Health">           | `IconHealth`      | `gitlab_server` (`gitlab_server_status` on the individual surface)                               |
| <img src="icons/upload.svg" width="32" height="32" alt="Upload">           | `IconUpload`      | none (upload actions route under `gitlab_project` and `gitlab_group`)                            |
| <img src="icons/import.svg" width="32" height="32" alt="Import">           | `IconImport`      | none (import and export actions route under `gitlab_project`, `gitlab_group` and `gitlab_admin`) |

<!-- markdownlint-enable MD033 -->

## Complete Icon-to-Group Reference

Alphabetical listing of all 50 domain icons, the catalog groups that carry each one (from `internal/tools/catalog_group_metadata.go`, plus the standalone and dynamic tools), and the resources and prompts that name it directly.

| Icon          | Variable            | Catalog groups and standalone tools                                                                                | Resources and prompts                                                                                                                                                                                                                                                                                       |
| ------------- | ------------------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Alert         | `IconAlert`         | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Analytics     | `IconAnalytics`     | `gitlab_dora_metrics`, `gitlab_orbit`                                                                              | prompts `merge_velocity`, `project_activity_report`, `weekly_team_recap`                                                                                                                                                                                                                                    |
| Audit         | `IconAudit`         | `gitlab_audit_event`                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Board         | `IconBoard`         | none                                                                                                               | resource `board`                                                                                                                                                                                                                                                                                            |
| Bot           | `IconBot`           | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Branch        | `IconBranch`        | `gitlab_branch`                                                                                                    | resources `branch`, `project_branches`; prompts `branch_mr_summary`, `compare_branches`                                                                                                                                                                                                                     |
| Commit        | `IconCommit`        | none                                                                                                               | resource `commit`; prompt `audit_commit_hygiene`                                                                                                                                                                                                                                                            |
| Compliance    | `IconCompliance`    | `gitlab_compliance_policy`                                                                                         | none                                                                                                                                                                                                                                                                                                        |
| Config        | `IconConfig`        | `gitlab_admin`, `gitlab_feature_flags`, `gitlab_member_role`; the `gitlab_interactive_*` wizards                   | resources `feature_flag`, `tool_manifest`, `tool_detail`                                                                                                                                                                                                                                                    |
| Container     | `IconContainer`     | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Deploy        | `IconDeploy`        | none                                                                                                               | resource `deployment`                                                                                                                                                                                                                                                                                       |
| Discussion    | `IconDiscussion`    | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Epic          | `IconEpic`          | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Environment   | `IconEnvironment`   | `gitlab_environment`                                                                                               | resource `environment`                                                                                                                                                                                                                                                                                      |
| Event         | `IconEvent`         | `gitlab_custom_emoji`                                                                                              | none                                                                                                                                                                                                                                                                                                        |
| File          | `IconFile`          | `gitlab_repository`                                                                                                | resource `file_blob`                                                                                                                                                                                                                                                                                        |
| Group         | `IconGroup`         | `gitlab_group`, `gitlab_group_scim`                                                                                | resources `group`, `groups`; prompt `team_overview`                                                                                                                                                                                                                                                         |
| Health        | `IconHealth`        | `gitlab_server` (`gitlab_server_status` on the individual surface)                                                 | prompt `project_health_check`                                                                                                                                                                                                                                                                               |
| Import        | `IconImport`        | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Infra         | `IconInfra`         | `gitlab_geo`, `gitlab_storage_move`                                                                                | none                                                                                                                                                                                                                                                                                                        |
| Integration   | `IconIntegration`   | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Issue         | `IconIssue`         | `gitlab_issue`                                                                                                     | resources `issue`, `project_issues`; prompts `my_issues`, `stale_items_report`, `unassigned_items`                                                                                                                                                                                                          |
| Job           | `IconJob`           | `gitlab_job`                                                                                                       | resources `job`, `pipeline_jobs`                                                                                                                                                                                                                                                                            |
| Key           | `IconKey`           | none                                                                                                               | resource `deploy_key`                                                                                                                                                                                                                                                                                       |
| Label         | `IconLabel`         | none                                                                                                               | resources `label`, `group_label`, `project_labels`; prompt `label_distribution`                                                                                                                                                                                                                             |
| Link          | `IconLink`          | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| MR            | `IconMR`            | `gitlab_merge_request`, `gitlab_mr_review`                                                                         | resources `merge_request`, `merge_request_notes`, `merge_request_discussions`; prompts `summarize_mr_changes`, `review_mr`, `suggest_mr_reviewers`, `mr_risk_assessment`, `summarize_open_mrs`, `my_open_mrs`, `my_pending_reviews`, `group_mr_dashboard`, `mr_discussion_health`, `mr_description_quality` |
| Milestone     | `IconMilestone`     | none                                                                                                               | resources `milestone`, `group_milestone`, `project_milestones`; prompts `milestone_progress`, `group_milestone_progress`                                                                                                                                                                                    |
| Notify        | `IconNotify`        | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Package       | `IconPackage`       | `gitlab_package`, `gitlab_dependency`, `gitlab_model_registry`                                                     | none                                                                                                                                                                                                                                                                                                        |
| Pipeline      | `IconPipeline`      | `gitlab_pipeline`                                                                                                  | resources `pipeline`, `latest_pipeline`; prompt `summarize_pipeline_status`                                                                                                                                                                                                                                 |
| Project       | `IconProject`       | `gitlab_project`, `gitlab_project_alias`; the standalone `gitlab_discover_project`                                 | resources `project`, `group_projects`                                                                                                                                                                                                                                                                       |
| Queue         | `IconQueue`         | `gitlab_merge_train`                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Release       | `IconRelease`       | `gitlab_release`                                                                                                   | resources `release`, `project_releases`; prompts `generate_release_notes`, `release_cadence`, `release_readiness`                                                                                                                                                                                           |
| Runner        | `IconRunner`        | `gitlab_runner`                                                                                                    | none                                                                                                                                                                                                                                                                                                        |
| Schedule      | `IconSchedule`      | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Search        | `IconSearch`        | `gitlab_search`; `gitlab_find_action` on the dynamic surface                                                       | none                                                                                                                                                                                                                                                                                                        |
| Security      | `IconSecurity`      | `gitlab_security_attribute`, `gitlab_security_category`, `gitlab_security_finding`, `gitlab_security_scan_profile` | prompts `audit_project_workflow`, `audit_project_full`                                                                                                                                                                                                                                                      |
| Server        | `IconServer`        | `gitlab_execute_action` on the dynamic surface; the fallback for a group missing from the icon map                 | none                                                                                                                                                                                                                                                                                                        |
| Shield        | `IconShield`        | `gitlab_attestation`, `gitlab_external_status_check`                                                               | none                                                                                                                                                                                                                                                                                                        |
| Snippet       | `IconSnippet`       | `gitlab_snippet`                                                                                                   | resources `snippet`, `project_snippet`                                                                                                                                                                                                                                                                      |
| Tag           | `IconTag`           | `gitlab_tag`                                                                                                       | resources `tag`, `project_tags`                                                                                                                                                                                                                                                                             |
| Template      | `IconTemplate`      | `gitlab_template`, `gitlab_ci_catalog`                                                                             | none                                                                                                                                                                                                                                                                                                        |
| Todo          | `IconTodo`          | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Token         | `IconToken`         | `gitlab_access`                                                                                                    | none                                                                                                                                                                                                                                                                                                        |
| Upload        | `IconUpload`        | none                                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| User          | `IconUser`          | `gitlab_user`, `gitlab_enterprise_user`                                                                            | resource `current_user`; prompts `daily_standup`, `team_member_workload`, `user_stats`, `user_activity_report`, `reviewer_workload`, `my_activity_summary`, `project_contributors`                                                                                                                          |
| Variable      | `IconVariable`      | `gitlab_ci_variable`                                                                                               | none                                                                                                                                                                                                                                                                                                        |
| Vulnerability | `IconVulnerability` | `gitlab_vulnerability`                                                                                             | none                                                                                                                                                                                                                                                                                                        |
| Wiki          | `IconWiki`          | `gitlab_wiki`                                                                                                      | resource `wiki_page`; the five `gitlab://guides/*` workflow guides                                                                                                                                                                                                                                          |

## Brand Mark

`IconBrand` is the one icon that identifies no domain. It is attached to
`Implementation.Icons` — the server's own identity in the handshake — rather
than to any tool, resource or prompt.

| Variable    | Attached to                              | Source                                                   |
| ----------- | ---------------------------------------- | -------------------------------------------------------- |
| `IconBrand` | `Implementation.Icons` in `createServer` | `cmd/gen_brand` (original artwork, generated `svgBrand`) |

The mark is **"the fan-out"**: a source node projecting three branch arcs,
each ending in a node — a git graph, and the project's own architecture (one
canonical action catalog reaching the dynamic, meta and individual tool
surfaces). It is original artwork for this project; earlier versions shipped
the GitLab tanuki, which is GitLab's trademark and carried no identity of its
own. The canonical vector lives in `cmd/gen_brand` and is scaled to the
24×24 `currentColor` constant in `internal/toolutil/brandmark_gen.go` by
`make brand`, so the in-binary mark can never drift from the site logo,
favicon and cards emitted in the same run.

Three details differ from the domain icons:

- **It keeps a 24×24 viewBox** instead of the domain icons' 16×16 — it is
  scaled from the shared 64×64 brand geometry rather than drawn at glyph
  size, and the viewBox only defines a coordinate system.
- **Its SVG entry uses `currentColor` and declares no `Theme`**, same as the
  domain icons — it works in themes the specification does not name. It gets
  the same `light`/`dark` WebP pair as every other icon, for the same reason:
  a client that rejects SVG (VS Code) still needs something to render.
  `cmd/gen_icon_webp` scans `brandmark_gen.go` alongside `icons.go` so the
  fallbacks regenerate with the domain icons.
- Under SEP-2575 the whole `Implementation` — all three entries — rides in
  the `_meta` of every response, not just the handshake, which is why the
  mark stays a handful of primitives (three arcs, four circles) rather than
  detailed artwork.

Before a brand mark existed the server advertised `IconServer`, the same
glyph as `gitlab_execute_action`, so a client rendering both showed one
picture for the server and for one of its tools.

## Testing

Icon integrity is validated by 9 unit tests in [`internal/toolutil/icons_test.go`](../../../internal/toolutil/icons_test.go):

| Test                                | Validates                                                                                                  |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `TestAllIcons_ThreeEntries`         | Every icon has exactly 3 entries (SVG + WebP light + WebP dark)                                            |
| `TestAllIcons_ValidDataURI`         | Every entry's Source starts with the matching `data:<MIMEType>;base64,`                                    |
| `TestAllIcons_CorrectMIMEType`      | Entry 0 is `image/svg+xml`; entries 1–2 are `image/webp`                                                   |
| `TestAllIcons_NonEmpty`             | No entry's Source is empty                                                                                 |
| `TestAllIcons_DecodesToSVG`         | The SVG entry's base64 payload decodes to a `<svg>...</svg>` document                                      |
| `TestAllIcons_SizesAny`             | The SVG entry's `Sizes` field equals `["any"]` (scalable)                                                  |
| `TestAllIcons_WebPFallbackTheme`    | WebP entries declare `Theme` `light`/`dark` and `Sizes: ["16x16"]`                                         |
| `TestAllIcons_WebPFallbackDecodes`  | WebP payloads decode to a real 16×16 image via `golang.org/x/image/webp`                                   |
| `TestWebpIcon_PanicsOnMissingAsset` | A name with no generated WebP pair panics at build time instead of shipping an icon with two empty entries |

## Security Considerations

Per the MCP specification, clients should treat icon data as untrusted:

- **Validate URIs** — only accept `data:`, `https:`, or `http:` schemes
- **Sanitize SVGs** — SVG content may contain scripts; clients should strip `<script>` tags and event handlers before rendering
- **Restrict resource loading** — icons should not trigger network requests for sub-resources

This project mitigates these risks by using self-contained inline SVGs with no external references, no JavaScript, and no event handlers.

## References

- [MCP Specification — Icons](https://modelcontextprotocol.io/specification/2025-11-25)
- [MCP Go SDK — Icon Type](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#Icon)
- [SVG Data URI Encoding](https://developer.mozilla.org/en-US/docs/Web/SVG/Tutorial/SVG_as_an_Image)
- [Source Code — icons.go](../../../internal/toolutil/icons.go)
- [Source Code — icons_test.go](../../../internal/toolutil/icons_test.go)
- [Source Code — cmd/gen_icon_webp](../../../cmd/gen_icon_webp/main.go)
