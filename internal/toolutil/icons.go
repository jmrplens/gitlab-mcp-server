// icons.go provides shared MCP icon constants for tools, resources, and prompts.
// Icons use inline SVG data: URIs to avoid external network dependencies.
package toolutil

import (
	"encoding/base64"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Minimal 16×16 SVG icons encoded as data: URIs.
// Each icon is a single-path SVG using currentColor for theme compatibility.
const (
	svgBranch      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M11.75 2.5a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5m.75 3.17a2.25 2.25 0 1 0-1.5 0v.58A2.25 2.25 0 0 1 8.75 8.5h-2.5A3.73 3.73 0 0 0 4.5 9.3v.45a2.25 2.25 0 1 0 1.5 0V9.3a2.24 2.24 0 0 1 .25-.04h2.5a3.75 3.75 0 0 0 3.75-3.75zM4.25 12a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5"/></svg>`
	svgCommit      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="3" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="0" y1="8" x2="5" y2="8" stroke="currentColor" stroke-width="1.5"/><line x1="11" y1="8" x2="16" y2="8" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgIssue       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.5"/><circle cx="8" cy="8" r="2" fill="currentColor"/></svg>`
	svgMR          = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M4.25 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5M4.5 5.67a2.25 2.25 0 1 1-1.5 0v4.66a2.25 2.25 0 1 1 1.5 0zm0 0"/><circle cx="11.75" cy="12.25" r=".75" fill="currentColor"/><path fill="currentColor" d="M11.75 9.75a2.25 2.25 0 1 0 .75 4.37V9.3a2.25 2.25 0 0 1-.75.45m0-7.25a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5m.75 3.17a2.25 2.25 0 1 0-1.5 0v3.58h1.5z"/></svg>`
	svgPipeline    = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="3" cy="8" r="2" fill="currentColor"/><circle cx="8" cy="8" r="2" fill="currentColor"/><circle cx="13" cy="8" r="2" fill="currentColor"/><line x1="5" y1="8" x2="6" y2="8" stroke="currentColor" stroke-width="1"/><line x1="10" y1="8" x2="11" y2="8" stroke="currentColor" stroke-width="1"/></svg>`
	svgJob         = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="2" width="12" height="12" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M6 5.5l4.5 2.5L6 10.5z" fill="currentColor"/></svg>`
	svgRelease     = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 1l2 3h4l-3 3 1.5 4L8 8.5 3.5 11 5 7 2 4h4z"/></svg>`
	svgTag         = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M2 2h5.5l6.5 6.5-5.5 5.5L2 7.5zm3 1.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3"/></svg>`
	svgProject     = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="3" width="12" height="10" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="2" y1="6" x2="14" y2="6" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgGroup       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="5" cy="5" r="2" fill="currentColor"/><circle cx="11" cy="5" r="2" fill="currentColor"/><path fill="currentColor" d="M1 12c0-2 2-3 4-3s4 1 4 3zm6 0c0-2 2-3 4-3s4 1 4 3z"/></svg>`
	svgUser        = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="5" r="3" fill="currentColor"/><path fill="currentColor" d="M2 14c0-3 3-5 6-5s6 2 6 5z"/></svg>`
	svgWiki        = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M2 2h8l4 4v8H2zm8 0v4h4"/><line x1="4" y1="8" x2="10" y2="8" stroke="currentColor" stroke-width="1"/><line x1="4" y1="10" x2="10" y2="10" stroke="currentColor" stroke-width="1"/></svg>`
	svgFile        = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.5" d="M3 1.5h7l3 3v10H3z"/><line x1="5" y1="7" x2="11" y2="7" stroke="currentColor" stroke-width="1"/><line x1="5" y1="9.5" x2="11" y2="9.5" stroke="currentColor" stroke-width="1"/></svg>`
	svgPackage     = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.5" d="M8 1L14 4.5v7L8 15 2 11.5v-7z"/><line x1="8" y1="8" x2="8" y2="15" stroke="currentColor" stroke-width="1"/><line x1="2" y1="4.5" x2="8" y2="8" stroke="currentColor" stroke-width="1"/><line x1="14" y1="4.5" x2="8" y2="8" stroke="currentColor" stroke-width="1"/></svg>`
	svgSearch      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="6.5" cy="6.5" r="4.5" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="10" y1="10" x2="14.5" y2="14.5" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgLabel       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="1" y="4" width="14" height="8" rx="4" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgMilestone   = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.5" d="M2 14L8 2l6 12z"/></svg>`
	svgEnvironment = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-width="1.5"/><ellipse cx="8" cy="8" rx="3" ry="6" fill="none" stroke="currentColor" stroke-width="1"/><line x1="2" y1="8" x2="14" y2="8" stroke="currentColor" stroke-width="1"/></svg>`
	svgDeploy      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2v8m-3-3l3 3 3-3"/><line x1="3" y1="13" x2="13" y2="13"/></svg>`
	svgSchedule    = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="8" y1="4" x2="8" y2="8" stroke="currentColor" stroke-width="1.5"/><line x1="8" y1="8" x2="11" y2="10" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgVariable    = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5 2c-1.5 0-2 .5-2 2v2c0 1-.5 2-2 2 1.5 0 2 1 2 2v2c0 1.5.5 2 2 2"/><path d="M11 2c1.5 0 2 .5 2 2v2c0 1 .5 2 2 2-1.5 0-2 1-2 2v2c0 1.5-.5 2-2 2"/></svg>`
	svgRunner      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="3" y="2" width="10" height="12" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><circle cx="6" cy="6" r="1" fill="currentColor"/><circle cx="10" cy="6" r="1" fill="currentColor"/><line x1="5" y1="10" x2="11" y2="10" stroke="currentColor" stroke-width="1"/></svg>`
	svgTodo        = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="2" width="12" height="12" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M5 8l2 2 4-4" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgHealth      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 14s-5.5-3.5-5.5-7A3.5 3.5 0 0 1 8 4.5 3.5 3.5 0 0 1 13.5 7c0 3.5-5.5 7-5.5 7"/></svg>`
	svgUpload      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 10V2m-3 3l3-3 3 3"/><line x1="3" y1="13" x2="13" y2="13"/></svg>`
	svgBoard       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="1" y="2" width="4" height="12" rx="1" fill="none" stroke="currentColor" stroke-width="1"/><rect x="6" y="2" width="4" height="8" rx="1" fill="none" stroke="currentColor" stroke-width="1"/><rect x="11" y="2" width="4" height="10" rx="1" fill="none" stroke="currentColor" stroke-width="1"/></svg>`
	svgSnippet     = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.5" d="M5 4L1 8l4 4m6-8l4 4-4 4"/></svg>`
	svgToken       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="6" cy="8" r="4" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="10" y1="8" x2="15" y2="8" stroke="currentColor" stroke-width="1.5"/><line x1="13" y1="6" x2="13" y2="10" stroke="currentColor" stroke-width="1.5"/></svg>` // #nosec G101 -- SVG icon, not a credential
	svgIntegration = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M6 2v4H2v4h4v4h4v-4h4V6h-4V2z"/></svg>`
	svgNotify      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 1C5 1 4 4 4 6v3l-2 2h12l-2-2V6c0-2-1-5-4-5m-2 13h4c0 1-1 2-2 2s-2-1-2-2"/></svg>`
	svgServer      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="2" width="12" height="5" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><rect x="2" y="9" width="12" height="5" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><circle cx="5" cy="4.5" r=".75" fill="currentColor"/><circle cx="5" cy="11.5" r=".75" fill="currentColor"/></svg>`
	svgSecurity    = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 1L2 4v4c0 4 3 6 6 7 3-1 6-3 6-7V4z"/></svg>`
	svgConfig      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="2.5" fill="none" stroke="currentColor" stroke-width="1.5"/><path fill="currentColor" d="M7 1h2v2.1a5 5 0 0 1 1.7.7L12.1 2.4l1.4 1.4-1.4 1.4a5 5 0 0 1 .7 1.7H15v2h-2.1a5 5 0 0 1-.7 1.7l1.4 1.4-1.4 1.4-1.4-1.4a5 5 0 0 1-1.7.7V15H7v-2.1a5 5 0 0 1-1.7-.7L3.9 13.6 2.5 12.2l1.4-1.4a5 5 0 0 1-.7-1.7H1V7h2.1a5 5 0 0 1 .7-1.7L2.5 3.9 3.9 2.5l1.4 1.4A5 5 0 0 1 7 3.1z"/></svg>`
	svgAnalytics   = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><polyline points="1,14 5,6 9,10 15,2" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgKey         = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="5" cy="8" r="3" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="8" y1="8" x2="14" y2="8" stroke="currentColor" stroke-width="1.5"/><line x1="12" y1="6" x2="12" y2="8" stroke="currentColor" stroke-width="1.5"/></svg>`
	svgLink        = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.5" d="M6.5 9.5l3-3M4.5 8.5L3 10a2.8 2.8 0 0 0 4 4l1.5-1.5M11.5 7.5L13 6a2.8 2.8 0 0 0-4-4L7.5 3.5"/></svg>`
	svgDiscussion  = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M2 2h12v8H6l-3 3v-3H2z"/></svg>`
	svgEvent       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M10 1L6 9h3l-2 6 6-8H9l3-6z"/></svg>`
	svgContainer   = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="3" width="12" height="10" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="5" y1="3" x2="5" y2="13" stroke="currentColor" stroke-width="1"/><line x1="8" y1="3" x2="8" y2="13" stroke="currentColor" stroke-width="1"/><line x1="11" y1="3" x2="11" y2="13" stroke="currentColor" stroke-width="1"/><line x1="2" y1="8" x2="14" y2="8" stroke="currentColor" stroke-width="1"/></svg>`
	svgImport      = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 2v8m0 0l3-3m-3 3L5 7"/><rect x="2" y="12" width="12" height="2" rx="1" fill="currentColor"/></svg>`
	svgAlert       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" d="M8 1L1 14h14z"/><line x1="8" y1="6" x2="8" y2="10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><circle cx="8" cy="12" r=".9" fill="currentColor"/></svg>`
	svgTemplate    = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="2" width="12" height="12" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><line x1="2" y1="6" x2="14" y2="6" stroke="currentColor" stroke-width="1"/><line x1="6" y1="6" x2="6" y2="14" stroke="currentColor" stroke-width="1"/></svg>`
	svgInfra       = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="5" y="1" width="6" height="4" rx="1" fill="none" stroke="currentColor" stroke-width="1"/><rect x="1" y="11" width="6" height="4" rx="1" fill="none" stroke="currentColor" stroke-width="1"/><rect x="9" y="11" width="6" height="4" rx="1" fill="none" stroke="currentColor" stroke-width="1"/><line x1="8" y1="5" x2="8" y2="8" stroke="currentColor" stroke-width="1"/><line x1="4" y1="8" x2="12" y2="8" stroke="currentColor" stroke-width="1"/><line x1="4" y1="8" x2="4" y2="11" stroke="currentColor" stroke-width="1"/><line x1="12" y1="8" x2="12" y2="11" stroke="currentColor" stroke-width="1"/></svg>`
	svgEpic        = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M2 3h12v2H2zm1 3h10v2H3zm2 3h6v2H5zm2 3h2v2H7z"/></svg>`

	// Shield: outlined shield with check — for protected resources (branches, envs, packages).
	svgShield = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round"><path d="M8 1L2 4v4c0 4 3 6 6 7 3-1 6-3 6-7V4z"/><path d="M5.5 8l2 2 3.5-4"/></svg>`
	// Audit: clipboard with lines — for audit events / activity logs.
	svgAudit = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="3" y="2" width="10" height="13" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><rect x="6" y="1" width="4" height="2" rx=".5" fill="currentColor"/><line x1="5.5" y1="7" x2="10.5" y2="7" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><line x1="5.5" y1="9.5" x2="10.5" y2="9.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/><line x1="5.5" y1="12" x2="8.5" y2="12" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>`
	// Queue: hourglass — for concurrency-controlled resource_groups.
	svgQueue = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M3 2h10M3 14h10" stroke-linecap="round"/><path d="M4 2v2.5L8 8 4 11.5V14M12 2v2.5L8 8l4 3.5V14"/></svg>`
	// Bot: robot head — for service accounts and automated actors.
	svgBot = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="3" y="5" width="10" height="9" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/><circle cx="6" cy="9" r="1" fill="currentColor"/><circle cx="10" cy="9" r="1" fill="currentColor"/><line x1="8" y1="2" x2="8" y2="5" stroke="currentColor" stroke-width="1.5"/><circle cx="8" cy="2" r="1" fill="currentColor"/><line x1="6" y1="12" x2="10" y2="12" stroke="currentColor" stroke-width="1" stroke-linecap="round"/></svg>`
	// Vulnerability: bug — for security findings and vulnerabilities.
	svgVulnerability = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"><ellipse cx="8" cy="9" rx="3" ry="4" fill="currentColor"/><path d="M5 9H2m12 0h-3M5 6L3 4m10 2l-2-2M5 12l-2 2m10-2l-2 2M6 4.5a2 2 0 0 1 4 0"/></svg>`
	// Compliance: document with checkmark — for policies and attestations.
	svgCompliance = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M3 1.5h7l3 3v10H3z"/><path d="M5.5 9l2 2 3.5-4" stroke-linecap="round"/></svg>`
)

const svgMIME = "image/svg+xml"

// icon wraps an SVG string as an [mcp.Icon] slice with a base64-encoded
// data URI. Base64 encoding is required because raw SVG markup contains
// characters (<, >, ", spaces, #, {, }) that are not valid in an unencoded
// RFC 2397 data URI. The MCP spec also documents base64 as the canonical
// form for embedded image data.
//
// Sizes is set to ["any"] to advertise that the SVG is resolution-independent
// and can be rendered at any size by the client.
func icon(svg string) []mcp.Icon {
	encoded := base64.StdEncoding.EncodeToString([]byte(svg))
	return []mcp.Icon{{
		Source:   "data:" + svgMIME + ";base64," + encoded,
		MIMEType: svgMIME,
		Sizes:    []string{"any"},
	}}
}

// Domain icons — each returns a one-element []mcp.Icon ready for the Icons field.
var (
	IconBranch        = icon(svgBranch)
	IconCommit        = icon(svgCommit)
	IconIssue         = icon(svgIssue)
	IconMR            = icon(svgMR)
	IconPipeline      = icon(svgPipeline)
	IconJob           = icon(svgJob)
	IconRelease       = icon(svgRelease)
	IconTag           = icon(svgTag)
	IconProject       = icon(svgProject)
	IconGroup         = icon(svgGroup)
	IconUser          = icon(svgUser)
	IconWiki          = icon(svgWiki)
	IconFile          = icon(svgFile)
	IconPackage       = icon(svgPackage)
	IconSearch        = icon(svgSearch)
	IconLabel         = icon(svgLabel)
	IconMilestone     = icon(svgMilestone)
	IconEnvironment   = icon(svgEnvironment)
	IconDeploy        = icon(svgDeploy)
	IconSchedule      = icon(svgSchedule)
	IconVariable      = icon(svgVariable)
	IconRunner        = icon(svgRunner)
	IconTodo          = icon(svgTodo)
	IconHealth        = icon(svgHealth)
	IconUpload        = icon(svgUpload)
	IconBoard         = icon(svgBoard)
	IconSnippet       = icon(svgSnippet)
	IconToken         = icon(svgToken)
	IconIntegration   = icon(svgIntegration)
	IconNotify        = icon(svgNotify)
	IconServer        = icon(svgServer)
	IconSecurity      = icon(svgSecurity)
	IconConfig        = icon(svgConfig)
	IconAnalytics     = icon(svgAnalytics)
	IconKey           = icon(svgKey)
	IconLink          = icon(svgLink)
	IconDiscussion    = icon(svgDiscussion)
	IconEvent         = icon(svgEvent)
	IconContainer     = icon(svgContainer)
	IconImport        = icon(svgImport)
	IconAlert         = icon(svgAlert)
	IconTemplate      = icon(svgTemplate)
	IconInfra         = icon(svgInfra)
	IconEpic          = icon(svgEpic)
	IconShield        = icon(svgShield)
	IconAudit         = icon(svgAudit)
	IconQueue         = icon(svgQueue)
	IconBot           = icon(svgBot)
	IconVulnerability = icon(svgVulnerability)
	IconCompliance    = icon(svgCompliance)
)
