package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// dockerArg describes one top-level argument in the Docker MCP Registry format.
//
// Name is the JSON Schema property name. Type is the Docker-flavored type
// string ("string", "boolean", etc.) derived from the MCP JSON Schema. Desc
// is the property description copied from the input schema verbatim.
type dockerArg struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Desc string `json:"desc"`
}

// dockerTool describes one MCP tool entry emitted for the Docker MCP Registry.
//
// Name is the MCP tool name. Description is the tool description trimmed to
// fit Docker's compact format. Arguments lists the top-level input
// properties; nested objects are flattened into a single object argument.
type dockerTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Arguments   []dockerArg `json:"arguments"`
}

// main runs the Docker MCP Registry generator and writes failures to stderr.
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// run introspects the selected MCP catalog mode and writes a tools.json payload
// compatible with Docker MCP Registry ingestion.
//
// Two failures travel back to main: a bad flag, and a failed write of the
// payload, which is a real stream the caller owns (a closed pipe is the usual
// one). Registering the compiled-in catalog on an in-memory server and listing
// it back are neither, so they abort instead.
func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("gen_docker_tools", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	enterprise := flags.Bool("enterprise", false, "include enterprise meta-tools")
	individual := flags.Bool("individual", false, "emit individual tools instead of meta-tools")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	client, closeStub := auditshared.NewStubGitLabClient("gen-docker-tools-token") //#nosec G101 -- dummy token, no real credential
	defer closeStub()

	opts := &mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}}
	server := mcp.NewServer(&mcp.Implementation{Name: "gen-docker-tools", Version: "0.0.1"}, opts)

	switch {
	case *individual:
		tools.RegisterAll(server, client, edition.Ultimate)
	case *enterprise:
		cmdutil.MustDo(tools.RegisterAllMeta(server, client, edition.Ultimate))
	default:
		cmdutil.MustDo(tools.RegisterAllMeta(server, client, edition.Free))
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	cmdutil.Must(server.Connect(ctx, st, nil))
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "gen-docker-tools-client", Version: "0.0.1"}, nil)
	session := cmdutil.Must(mcpClient.Connect(ctx, ct, nil))
	defer func() { _ = session.Close() }()

	result := cmdutil.Must(session.ListTools(ctx, nil))

	out := make([]dockerTool, 0, len(result.Tools))
	for _, t := range result.Tools {
		out = append(out, dockerTool{
			Name:        t.Name,
			Description: t.Description,
			Arguments:   schemaArgs(t.InputSchema),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(out); encErr != nil {
		return fmt.Errorf("encode: %w", encErr)
	}
	return nil
}

// schemaArgs flattens a JSON Schema object into Docker's argument format.
// It only emits top-level properties; nested objects are described as type "object".
//
// Properties are returned sorted by name so that the generated output is
// deterministic across runs. Returns nil for nil, non-object, or
// unmarshalable inputs so callers can treat the helper as fail-closed.
func schemaArgs(schema any) []dockerArg {
	if schema == nil {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var s struct {
		Properties map[string]struct {
			Type        any    `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err = json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	args := make([]dockerArg, 0, len(s.Properties))
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := s.Properties[k]
		args = append(args, dockerArg{
			Name: k,
			Type: typeString(p.Type),
			Desc: p.Description,
		})
	}
	return args
}

// typeString converts a JSON Schema type value into Docker's single string
// representation, falling back to string when the schema is ambiguous.
func typeString(t any) string {
	switch v := t.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return "string"
}
