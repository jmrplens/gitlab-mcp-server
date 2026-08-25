# Reference

**Reference** — precise, look-up-oriented descriptions of every knob and surface.

Come here when you know *what* you want and need the exact name, type, default,
or shape: a configuration variable, a CLI flag, the response format, or the full
inventory of tools, resources, prompts, and capabilities the server exposes.
Reference pages describe the product as it is — they don't teach workflows (see
[Guides](../guides/README.md)) or explain rationale (see [Concepts](../concepts/README.md)).

Several pages here are **generated** from the codebase (`tools/`, and the
counts in `prompts`/`resources`) and validated in CI — treat them as
authoritative and edit the source, not the Markdown.

> **Diátaxis type**: Reference · **Audience**: 👤🔧 All users & integrators

| Reference                              | Covers                                                                                      |
| -------------------------------------- | ------------------------------------------------------------------------------------------- |
| [Configuration](configuration.md)      | Transport modes, `.env` setup, and how settings are loaded                                  |
| [Environment Variables](env.md)        | Every environment variable with defaults and descriptions                                   |
| [CLI Reference](cli.md)                | All command-line flags with usage examples                                                  |
| [Output Format](output-format.md)      | How responses are structured: Markdown + JSON, annotations, links, next-step hints          |
| [Tools](tools/README.md)               | Per-domain tool documentation across every catalog group                                    |
| [Resources](resources.md)              | MCP resources and URI templates, including the surface-aware tool manifest                  |
| [Prompts](prompts.md)                  | Every prompt with its arguments and output format                                           |
| [Capabilities](capabilities/README.md) | The MCP capabilities (progress, completions, elicitation, resource subscriptions) and icons |

**Looking for something else?**
[Guides](../guides/README.md) for step-by-step tasks ·
[Concepts](../concepts/README.md) for the "why" behind these surfaces.
