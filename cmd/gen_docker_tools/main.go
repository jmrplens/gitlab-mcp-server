// Command gen_docker_tools generates a tools.json file in the format expected
// by the Docker MCP Registry (https://github.com/docker/mcp-registry).
//
// The format is:
//
//	[
//	  {
//	    "name": "tool_name",
//	    "description": "tool description",
//	    "arguments": [
//	      {"name": "arg", "type": "string", "desc": "..."}
//	    ]
//	  }
//	]
//
// Usage:
//
//	go run ./cmd/gen_docker_tools/ > tools.json
//
// By default it emits the base meta-tools (META_TOOLS=true, no enterprise).
// Pass --enterprise to include enterprise meta-tools, or --individual to emit
// the full 1000-tool surface.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

type dockerArg struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Desc string `json:"desc"`
}

type dockerTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Arguments   []dockerArg `json:"arguments"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	enterprise := flag.Bool("enterprise", false, "include enterprise meta-tools")
	individual := flag.Bool("individual", false, "emit individual tools instead of meta-tools")
	flag.Parse()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{ //#nosec G101 -- dummy token, no real credential
		GitLabURL:   srv.URL,
		GitLabToken: "gen-docker-tools-token",
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}

	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: "gen-docker-tools", Version: "0.0.1"}, opts)

	switch {
	case *individual:
		tools.RegisterAll(server, client, true)
	case *enterprise:
		tools.RegisterAllMeta(server, client, true)
	default:
		tools.RegisterAllMeta(server, client, false)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, connErr := server.Connect(ctx, st, nil); connErr != nil {
		return fmt.Errorf("server connect: %w", connErr)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "gen-docker-tools-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return fmt.Errorf("client connect: %w", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	out := make([]dockerTool, 0, len(result.Tools))
	for _, t := range result.Tools {
		out = append(out, dockerTool{
			Name:        t.Name,
			Description: t.Description,
			Arguments:   schemaArgs(t.InputSchema),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(out); encErr != nil {
		return fmt.Errorf("encode: %w", encErr)
	}
	return nil
}

// schemaArgs flattens a JSON Schema object into Docker's argument format.
// It only emits top-level properties; nested objects are described as type "object".
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
	if unmarshalErr := json.Unmarshal(raw, &s); unmarshalErr != nil {
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
