// model_facing.go narrows what the token footprint counts to what a model
// actually receives.
//
// The measurement used to marshal whole catalog entries, which meant it also
// counted `icons` — base64-encoded SVG data URIs that exist for client user
// interfaces. No MCP client puts an icon into a model's context, so counting
// them overstated the figure the footprint exists to report, and by a lot: 16%
// of the individual tool surface, 12% of the dynamic one, and 53% of the
// shared resource and prompt surface.
package main

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// marshalModelFacing marshals a catalog entry with the presentation-only
// fields removed, so the result reflects context cost rather than wire cost.
//
// Entries are copied before being cleared: these come from a live session and
// clearing the originals would corrupt any later measurement of the same list.
// Types without presentation-only fields marshal unchanged.
func marshalModelFacing(entry any) ([]byte, error) {
	switch typed := entry.(type) {
	case *mcp.Tool:
		clone := *typed
		clone.Icons = nil
		return json.Marshal(&clone)
	case *mcp.Resource:
		clone := *typed
		clone.Icons = nil
		return json.Marshal(&clone)
	case *mcp.ResourceTemplate:
		clone := *typed
		clone.Icons = nil
		return json.Marshal(&clone)
	case *mcp.Prompt:
		clone := *typed
		clone.Icons = nil
		return json.Marshal(&clone)
	default:
		return json.Marshal(entry)
	}
}
