// Package apidocs is a shared fetcher for the GitLab API reference docs (the
// doc/api/<area>.md files in the gitlab-org/gitlab monorepo) used as a source of
// truth alongside the client-go SDK by the audit utilities.
//
// Docs are cached on disk in a single shared, gitignored directory
// (.cache/gitlab-api-docs/ under the repo root). A cached doc is reused while it
// is younger than MaxAge (7 days by default); older or missing docs are
// re-downloaded. Callers can force a full refresh or pin to offline (cached
// only). Downloads are polite: a server Retry-After is honored, otherwise
// exponential backoff with jitter is used, and successful fetches are spaced so
// a full sweep does not trip GitLab's raw rate limiter.
package apidocs
