# Documentation

The documentation for people who install, configure and use gitlab-mcp-server is
the documentation site, in [English](https://jmrp.io/docs/gitlab-mcp-server/) and
[Spanish](https://jmrp.io/docs/gitlab-mcp-server/es/). Its source is
[`site/src/content/docs`](../site/src/content/docs), one page per slug with its
Spanish twin under `es/`, and nothing in this directory repeats it.

This directory holds the contributor documentation, under
[`development/`](development/README.md): building and testing the server, its
internal architecture, the gates and generators, and the
[Architectural Decision Records](development/adr/README.md).
[ADR-0025](development/adr/adr-0025-the-site-is-the-only-home-of-user-documentation.md)
records why the user documentation lives only on the site. Start with
[CONTRIBUTING.md](../CONTRIBUTING.md) for how a change is proposed and reviewed.
