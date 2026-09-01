package telemetry

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// ToolNamePolicy decides whether the tool name becomes a metric dimension.
type ToolNamePolicy string

const (
	// ToolNameAuto keeps the tool name on the surfaces where it is cheap and
	// drops it where it is not. The default, and the only value most operators
	// should need.
	ToolNameAuto ToolNamePolicy = "auto"
	// ToolNameOn keeps it on every surface, for an operator who has decided
	// their backend can afford the series and wants per-tool latency.
	ToolNameOn ToolNamePolicy = "on"
	// ToolNameOff drops it everywhere, for the smallest possible metric
	// footprint.
	ToolNameOff ToolNamePolicy = "off"
)

// EnvToolNameName is the environment variable that selects the policy.
const EnvToolNameName = "GITLAB_MCP_TELEMETRY_TOOL_NAME"

// ParseToolNamePolicy validates an operator-supplied value.
//
// An unrecognized value is an error rather than a silent default, for the same
// reason as everywhere else in this package: an operator who typed "yes" and
// got "auto" would find a dimension missing on one surface and present on
// another, with nothing anywhere saying why.
func ParseToolNamePolicy(value string) (ToolNamePolicy, error) {
	switch ToolNamePolicy(strings.TrimSpace(strings.ToLower(value))) {
	case "":
		return ToolNameAuto, nil
	case ToolNameAuto:
		return ToolNameAuto, nil
	case ToolNameOn:
		return ToolNameOn, nil
	case ToolNameOff:
		return ToolNameOff, nil
	default:
		return "", fmt.Errorf("unknown telemetry tool-name policy %q: use %q, %q or %q",
			value, ToolNameAuto, ToolNameOn, ToolNameOff)
	}
}

// DropToolName reports whether the tool name should be filtered out of metric
// attributes for a given tool surface.
//
// # The number that decides this
//
// The individual surface registers between about 850 and 1071 distinct tools,
// one per catalog action. As a metric dimension that is up to 1071 time series
// per method, multiplied by every other dimension, against a Go SDK default
// cardinality limit of 2000 per instrument per collection cycle.
//
// What happens at the limit is worse than an error. No measurement is lost, but
// everything past the limit collapses into one synthetic series marked
// otel.metric.overflow, and cumulative temporality makes it first-come-wins:
// the first combinations seen after startup are kept forever and everything
// later collapses. So the sample is biased by call order rather than by
// importance, and the long tail of rarely-used GitLab actions becomes
// unattributable precisely when somebody is trying to debug one. The only
// visible signal is the synthetic series itself.
//
// The dynamic surface has two tool names and the meta surface about fifty.
// Neither is a problem, and on the dynamic surface the attribute is nearly all
// a metric has to go on, so dropping it there would cost real information for
// no benefit.
//
// # Two attributes, one decision
//
// The drop takes gitlab_mcp.action with it, and it has to. On the individual
// surface the two are one to one, because that surface projects one visible
// tool per catalog action, so a filter that removed the tool name and left the
// action behind shed no series at all: the same eleven hundred values simply
// arrived under a different key. The budget is a property of the pair, so the
// decision is one decision.
//
// # This is a documented deviation, not a permitted variation
//
// gen_ai.tool.name is Conditionally Required in the MCP convention, and the
// requirement-levels table allows exclusion via configuration for Recommended
// and Opt-In only. Placing it at Conditionally Required is an assertion by the
// convention's authors that a tool name is not a high-cardinality metric
// attribute, which is true for a server with a dozen tools and false for one
// with a thousand. The attribute still passes the convention's own aggregation
// test, so this is a budget problem rather than a modeling error, and the
// deviation is recorded rather than hidden.
func DropToolName(policy ToolNamePolicy, toolSurface string) bool {
	switch policy {
	case ToolNameOn:
		return false
	case ToolNameOff:
		return true
	default:
		return strings.EqualFold(strings.TrimSpace(toolSurface), "individual")
	}
}

// The keys a drop filters out. Written out rather than imported for the same
// reason internal/mcpotel writes its keys out: the MCP convention ships as no
// Go package, and gitlab_mcp.action is this server's own.
const (
	attrGenAIToolName = "gen_ai.tool.name"

	// attrActionID goes with it, and leaving it behind was the defect. On the
	// individual surface the two are one to one: every visible tool is one
	// catalog action, so a metric that dropped the tool name and kept the
	// action carried exactly the same number of series it was meant to shed.
	// The View performed its filtering and achieved nothing.
	attrActionID = "gitlab_mcp.action"
)

// toolNameView returns the View that removes the tool name from metric
// attributes.
//
// # Why a View and not a raised limit
//
// Filtering happens before the limit is counted, which is what makes this the
// right lever: "Cardinality limit enforcement SHOULD occur after attribute
// filtering, if any. This ensures users can filter undesired attributes using
// views and prevent reaching the cardinality limit."
//
// Raising the limit instead is not available at the granularity that would be
// needed. Go's Stream has no CardinalityLimit field, so the specification's
// per-View route is unimplemented; what exists is a MeterProvider-wide option
// and a per-instrument-KIND selector. Every metric this server emits is a
// histogram, so a kind selector cannot separate the MCP duration from the HTTP
// ones and would raise the ceiling for all of them at once.
//
// The filter is an allow-everything-else predicate rather than an allow-list,
// which the specification permits: "Implementations MAY accept additional
// attribute filtering functionality for this parameter." An allow-list would
// have to enumerate every convention-owned key and would silently drop the next
// one somebody adds.
//
// # A filtered attribute still reaches the collector, through exemplars
//
// Measured rather than assumed: with this View active, the tool name is absent
// from every data point's attribute set and present in the exported payload,
// because an exemplar records the attributes that were filtered out along with
// the trace it came from. Setting OTEL_METRICS_EXEMPLAR_FILTER=always_off makes
// it disappear entirely, which is how it was confirmed.
//
// For cardinality that changes nothing, and cardinality is what this View is
// for: an exemplar reservoir is bounded per data point, so it mints no time
// series however many distinct values pass through it.
//
// It matters for anyone who later reaches for a View to keep a value away from
// a collector. That does not work, and the failure is silent. Privacy filtering
// in this server is done by never putting the value in an attribute in the
// first place, which is the only place it holds: see internal/mcpotel, where
// the span and metric attribute lists are built separately and neither ever
// carries a tool argument, a resource URI or a credential.
func toolNameView() sdkmetric.View {
	return func(instrument sdkmetric.Instrument) (sdkmetric.Stream, bool) {
		return sdkmetric.Stream{
			Name:        instrument.Name,
			Description: instrument.Description,
			Unit:        instrument.Unit,
			AttributeFilter: func(kv attribute.KeyValue) bool {
				key := string(kv.Key)
				return key != attrGenAIToolName && key != attrActionID
			},
		}, true
	}
}
