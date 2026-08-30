# Privacy Policy

Last updated: 2026-08-06

**gitlab-mcp-server** is a local Model Context Protocol (MCP) server. It runs
entirely on your machine and acts as a bridge between your MCP client (Claude
Desktop, Claude Code, Cursor, VS Code, …) and the GitLab instance you
configure. This policy describes what data the server handles and where it
goes.

One path is different and is described separately below: the public hosted
endpoint at `mcp.jmrp.io/gitlab`, where the software runs on someone else's
machine rather than yours. See [Hosted endpoint](#hosted-endpoint).

## What we collect

**Nothing.** The server has no telemetry, no analytics, no crash reporting,
and no backend of its own. When you run it yourself — which is how this
documentation recommends using it — the maintainer never receives, stores, or
has access to any of your data, credentials, or usage information.

That is a statement about the software, and it holds wherever you run it. It
is not a statement about the [hosted endpoint](#hosted-endpoint), where the
same software runs on a machine the maintainer operates.

## Data flows

- **Your GitLab instance.** Every tool call results in requests to the GitLab
  URL you configure (`GITLAB_URL`), authenticated with your Personal Access
  Token (`GITLAB_TOKEN`). Data returned by GitLab (projects, issues, merge
  requests, pipeline logs, …) is passed directly to your MCP client and is
  never sent anywhere else. GitLab's handling of that data is governed by the
  [GitLab Privacy Statement](https://about.gitlab.com/privacy/) (for
  GitLab.com) or by your organization's own policies (for self-managed
  instances).
Your GitLab instance is the only destination. The server contacts nothing else,
and nothing it does is optional in that respect: there is no update check, no
license check, no registry ping and no default that reaches any other host.

This used to be untrue in one narrow way worth recording rather than quietly
dropping. The server carried a self-update feature that was **on by default**
for standalone binaries, so it periodically asked GitHub Releases whether a
newer version existed. No personal data travelled with that request, but it was
a network call to a third party that nobody had asked for, made by a program
whose privacy policy opens by saying it collects nothing. The feature is gone:
every way of installing this server already owns updates, and a binary that
downloads and runs new code is the last thing that belongs in a process holding
your GitLab token.

## Hosted endpoint

A public instance of this server is hosted at `https://mcp.jmrp.io/gitlab`.
Using it is optional and is never the default: nothing installs it, and no
configuration in this repository points at it.

What changes when you use it is worth stating plainly. Your GitLab Personal
Access Token and every tool call travel over the network to a machine operated
by this project's maintainer instead of staying on your own, and the requests
to your GitLab instance are then made by that machine rather than by yours, so
GitLab sees its address instead of yours. The token is used to authenticate
that request and is not stored server-side, but you are trusting a host you do
not control with it.

That instance is operated as part of [mcp.jmrp.io](https://mcp.jmrp.io/) and
its handling of requests is governed there, not by this policy, which describes
the software. This document can only tell you what the software does; it cannot
make promises on behalf of a server you are not running.

If your GitLab instance is private, or the token is scoped beyond what you are
willing to hand to a third-party host, run the server locally. That is the whole
of the advice, and it is why every install path in the documentation leads there
first.

## Credentials

Your GitLab Personal Access Token is provided by you through environment
variables or your MCP client's configuration UI. Claude Desktop stores
extension secrets in the operating system keychain. The server keeps the
token in process memory only, uses it solely to authenticate requests to your
configured GitLab instance, and never logs it.

## Local storage and logs

The server writes logs to standard error only (collected, if at all, by your
MCP client). It does not create databases, caches, or files with your GitLab
data. In HTTP mode, token identities are cached in memory for the configured
TTL and are never persisted to disk.

## Data retention and sharing

The server retains nothing after it exits and shares data with no third
parties beyond the GitLab instance you explicitly configure.

## Changes

Changes to this policy are published in this file and noted in release
changelogs.

## Contact

Questions or concerns: [open an issue](https://github.com/jmrplens/gitlab-mcp-server/issues)
or email <jmrplens@gmail.com>.
