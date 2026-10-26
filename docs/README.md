# Documentation

The documentation for people who install, configure and use gitlab-mcp-server is
the documentation site, in [English](https://docs.jmrp.io/gitlab-mcp-server/) and
[Spanish](https://docs.jmrp.io/gitlab-mcp-server/es/). Its source is
[`site/src/content/docs`](../site/src/content/docs), one page per slug with its
Spanish twin under `es/`, and nothing in this directory repeats it.

This directory holds the contributor documentation, under
[`development/`](development/README.md): building and testing the server, its
internal architecture, the gates and generators, and the
[Architectural Decision Records](development/adr/README.md).
[ADR-0025](development/adr/adr-0025-the-site-is-the-only-home-of-user-documentation.md)
records why the user documentation lives only on the site. Start with
[CONTRIBUTING.md](../CONTRIBUTING.md) for how a change is proposed and reviewed,
and [GOVERNANCE.md](../GOVERNANCE.md) for how the project makes decisions and who
holds which role. [SECURITY.md](../SECURITY.md) holds the threat model, and the
[Security Review, October 2026](development/security-review-2026-10.md) records
how its requirements and boundaries were last checked against the code.
