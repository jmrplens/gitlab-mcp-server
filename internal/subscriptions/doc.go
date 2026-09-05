// Package subscriptions implements MCP resource subscriptions
// (resources/subscribe) over GitLab resources.
//
// Delivery is polling-based, in both transports: a watcher re-reads a
// subscribed URI on an interval and emits notifications/resources/updated
// only when the content actually changed. There is no webhook path — this
// server does not run an inbound receiver of any kind, a decision recorded
// in ADR-0016 (docs/development/adr/adr-0016-no-webhook-ingestion.md), not
// a gap left for later.
package subscriptions
