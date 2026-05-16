// Package packages implements MCP tools for GitLab Generic Packages registry
// operations.
//
// The package supports publishing, downloading, streaming downloads to disk,
// listing packages and package files, deleting package records and files, and
// higher-level helpers such as publishing a release asset link or publishing all
// matching files in a directory. Markdown formatters render package outputs for
// individual tools and meta-tool dispatchers.
//
// # Large Payloads
//
// Download helpers stream package files through bounded file utilities rather
// than forcing large artifacts into MCP text content. Metadata-oriented tools
// keep structured output compact and use Markdown tables for human review.
//
// # GitLab API References
//
// The package wraps the GitLab Packages API:
//
//   - https://docs.gitlab.com/api/packages/
package packages
