// Command gen_lhm_manifest regenerates the tools, prompts, and resources arrays
// in lhm.plugin.json, the manifest published to the LobeHub Marketplace.
//
// LobeHub derives a listing's capability badges from the manifest's own
// tools/prompts/resources arrays — its scanner cannot introspect a server that
// ships as a Go binary or a Docker image, so a manifest without them lists the
// server as having zero tools and zero prompts no matter what the server
// actually registers. The arrays are therefore data we owe the marketplace, and
// this command derives them from a real tools/list, prompts/list, and
// resources/list round-trip against an in-memory server rather than from a
// hand-written copy that would drift on the next release.
//
// The declared tool surface is the default one, dynamic, pinned explicitly
// rather than read from TOOL_SURFACE: the manifest describes what a user gets
// with no configuration, and reading the environment would make the committed
// file depend on the machine that generated it.
//
// Every other field the manifest carries is preserved, version included — the
// release stamp owns that one.
//
// Usage:
//
//	go run ./cmd/gen_lhm_manifest/
//	go run ./cmd/gen_lhm_manifest/ --check
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
)

// manifestFileName is the LobeHub manifest read by `lhm plugin publish`.
const manifestFileName = "lhm.plugin.json"

// manifest mirrors the schema accepted by the LobeHub publish endpoint
// (market.lobehub.com/s/publish-mcp/references/manifest). Every documented field
// is listed so that decoding with DisallowUnknownFields rejects a typo instead
// of silently dropping it on the next regeneration — the endpoint strips unknown
// fields without complaint, which is exactly how a misspelled key would ship.
//
// homepage and cloudEndpoint are validated but not stored by the endpoint; they
// stay in the struct so a manifest that carries them round-trips unchanged.
type manifest struct {
	Identifier    string             `json:"identifier"`
	Name          string             `json:"name"`
	Version       string             `json:"version"`
	Description   string             `json:"description,omitempty"`
	Author        string             `json:"author,omitempty"`
	AuthorURL     string             `json:"authorUrl,omitempty"`
	Category      string             `json:"category,omitempty"`
	Homepage      string             `json:"homepage,omitempty"`
	CloudEndpoint string             `json:"cloudEndpoint,omitempty"`
	Icon          string             `json:"icon,omitempty"`
	Tags          []string           `json:"tags,omitempty"`
	Localizations []json.RawMessage  `json:"localizations,omitempty"`
	Tools         []manifestTool     `json:"tools,omitempty"`
	Prompts       []manifestPrompt   `json:"prompts,omitempty"`
	Resources     []manifestResource `json:"resources,omitempty"`
}

// manifestTool is one entry of the manifest's tools array, in the standard MCP
// tool shape. The output schema and the icons are deliberately left out: neither
// is part of the shape LobeHub documents, and the icon data URIs alone would
// triple the file for something the listing never renders.
type manifestTool struct {
	Name        string               `json:"name"`
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	InputSchema any                  `json:"inputSchema,omitempty"`
	Annotations *mcp.ToolAnnotations `json:"annotations,omitempty"`
}

// manifestPrompt is one entry of the manifest's prompts array, in the standard
// MCP prompt shape.
type manifestPrompt struct {
	Name        string                `json:"name"`
	Title       string                `json:"title,omitempty"`
	Description string                `json:"description,omitempty"`
	Arguments   []*mcp.PromptArgument `json:"arguments,omitempty"`
}

// manifestResource is one entry of the manifest's resources array. Only static
// resources are listed: a resource template is identified by a URI template
// rather than a URI, which the marketplace shape has no field for.
type manifestResource struct {
	URI         string           `json:"uri"`
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	MIMEType    string           `json:"mimeType,omitempty"`
	Annotations *mcp.Annotations `json:"annotations,omitempty"`
}

// surface holds the registered capabilities the manifest declares.
type surface struct {
	tools     []manifestTool
	prompts   []manifestPrompt
	resources []manifestResource
}

func main() {
	checkOnly := flag.Bool("check", false, "verify the committed manifest matches the registered surface without writing it")
	flag.Parse()

	if err := run(*checkOnly); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate %s: %v\n", manifestFileName, err)
		os.Exit(1)
	}
}

// run rewrites the manifest's capability arrays from the live MCP surface, or,
// in check mode, reports whether the committed file already matches them.
func run(checkOnly bool) error {
	rootDir, err := mcpsurface.ProjectRoot()
	if err != nil {
		return err
	}
	// The manifest is addressed relative to an os.Root rooted at the project
	// directory, so the command can only ever read and write that one file.
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return fmt.Errorf("open project root: %w", err)
	}
	defer func() { _ = root.Close() }()

	current, err := root.ReadFile(manifestFileName)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestFileName, err)
	}

	generated, counts, err := generate(current)
	if err != nil {
		return err
	}

	if checkOnly {
		if !bytes.Equal(current, generated) {
			return fmt.Errorf("%s is stale: run `make gen-lhm-manifest` and commit the result", manifestFileName)
		}
		fmt.Printf("%s is current (%s)\n", manifestFileName, counts)
		return nil
	}

	if writeErr := root.WriteFile(manifestFileName, generated, 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", manifestFileName, writeErr)
	}
	fmt.Printf("Generated %s (%s)\n", manifestFileName, counts)
	return nil
}

// generate returns the manifest bytes that current should have, with the
// capability arrays replaced by the registered surface and every other field
// left untouched.
// surfaceCache memoizes readSurface: the listed capability arrays depend
// only on the compiled-in catalog, the round-trip costs seconds, and this
// one-shot command (and its test binary) may assemble more than one
// manifest from the same surface.
var surfaceCache struct {
	once       sync.Once
	registered surface
	err        error
}

func cachedSurface() (surface, error) {
	surfaceCache.once.Do(func() {
		surfaceCache.registered, surfaceCache.err = readSurface()
	})
	return surfaceCache.registered, surfaceCache.err
}

func generate(current []byte) (out []byte, counts string, err error) {
	m, err := decodeManifest(current)
	if err != nil {
		return nil, "", err
	}

	registered, err := cachedSurface()
	if err != nil {
		return nil, "", err
	}
	m.Tools = registered.tools
	m.Prompts = registered.prompts
	m.Resources = registered.resources

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(m); err != nil {
		return nil, "", fmt.Errorf("encode %s: %w", manifestFileName, err)
	}
	return buf.Bytes(), fmt.Sprintf("%d tools, %d prompts, %d resources",
		len(m.Tools), len(m.Prompts), len(m.Resources)), nil
}

// decodeManifest parses the committed manifest and rejects anything the publish
// endpoint would not accept: an unknown field, a missing required field, or a
// second JSON value after the manifest object. Without the trailing-value check
// a decoder that reads only the first value would let the rest be dropped on the
// next rewrite; without the required-field check --check would certify a
// manifest that cannot be published.
func decodeManifest(current []byte) (manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(current))
	decoder.DisallowUnknownFields()
	var m manifest
	if err := decoder.Decode(&m); err != nil {
		return manifest{}, fmt.Errorf("parse %s: %w", manifestFileName, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return manifest{}, fmt.Errorf("parse %s: unexpected content after the manifest object", manifestFileName)
	}
	if err := requireManifestFields(m); err != nil {
		return manifest{}, err
	}
	return m, nil
}

// requireManifestFields reports the manifest fields the LobeHub publish endpoint
// treats as mandatory and that this command never fills in.
func requireManifestFields(m manifest) error {
	var missing []string
	for _, field := range []struct {
		name  string
		value string
	}{
		{"identifier", m.Identifier},
		{"name", m.Name},
		{"version", m.Version},
	} {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s is missing required field(s): %s", manifestFileName, strings.Join(missing, ", "))
	}
	return nil
}

// readSurface introspects the registered capabilities over real MCP list
// round-trips against an offline stub client.
func readSurface() (surface, error) {
	client, closeStub, err := mcpsurface.NewStubClient()
	if err != nil {
		return surface{}, fmt.Errorf("create client: %w", err)
	}
	defer closeStub()

	toolList, err := mcpsurface.DynamicTools(client)
	if err != nil {
		return surface{}, err
	}
	promptList, err := mcpsurface.Prompts(client)
	if err != nil {
		return surface{}, err
	}
	resourceList, _, err := mcpsurface.Resources(client)
	if err != nil {
		return surface{}, err
	}
	return surface{
		tools:     manifestTools(toolList),
		prompts:   manifestPrompts(promptList),
		resources: manifestResources(resourceList),
	}, nil
}

// manifestTools converts the tools/list result into manifest entries. The
// dynamic surface arrives find-then-execute; the entries are sorted by name so
// the file does not churn when that presentation order changes.
func manifestTools(list []*mcp.Tool) []manifestTool {
	out := make([]manifestTool, 0, len(list))
	for _, tool := range list {
		out = append(out, manifestTool{
			Name:        tool.Name,
			Title:       tool.Title,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			Annotations: tool.Annotations,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// manifestPrompts converts the prompts/list result into manifest entries,
// ordered by name so the output does not depend on registration order.
func manifestPrompts(list []*mcp.Prompt) []manifestPrompt {
	out := make([]manifestPrompt, 0, len(list))
	for _, prompt := range list {
		out = append(out, manifestPrompt{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
			Arguments:   prompt.Arguments,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// manifestResources converts the resources/list result into manifest entries,
// ordered by name for the same reason as the prompts.
func manifestResources(list []*mcp.Resource) []manifestResource {
	out := make([]manifestResource, 0, len(list))
	for _, resource := range list {
		out = append(out, manifestResource{
			URI:         resource.URI,
			Name:        resource.Name,
			Title:       resource.Title,
			Description: resource.Description,
			MIMEType:    resource.MIMEType,
			Annotations: resource.Annotations,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
