package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func main() {
	// Crear el registry con rutas de prueba
	registry := dynamic.NewRegistry(testRoutes())

	// Queries para probar
	testQueries := []string{
		// Query original que no funcionaba
		"merge request list open author project",
		// Variaciones y casos edge
		"merge request list",
		"mr approve",
		"project delete",
		"issue close",
		"webhook create",
		"list open issues",
		"pipeline run trigger",
		"ci variable secret",
		"branch protect",
		// Typos / términos incorrectos
		"merge request list open author INVALID",
		"foobar baz qux",
		"mr list",
		"create project",
	}

	ctx := context.Background()

	for _, query := range testQueries {
		result, output, err := registry.Search(ctx, nil, dynamic.SearchInput{
			Query: query,
			Limit: 5,
		})

		fmt.Printf("\n%s\n", strings.Repeat("─", 80))
		fmt.Printf("Query: %q\n", query)
		fmt.Printf("─%s\n", strings.Repeat("─", 79))

		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		if result.IsError {
			fmt.Printf("❌ Result is error\n")
			continue
		}

		if output.Count == 0 {
			fmt.Printf("⚠️  No matches found\n")
			continue
		}

		fmt.Printf("✅ Found %d matches:\n", output.Count)

		// Ordenar por score descendente
		sort.Slice(output.Results, func(i, j int) bool {
			return output.Results[i].Score > output.Results[j].Score
		})

		for i, res := range output.Results {
			fmt.Printf("  %d. [%3d] %s\n", i+1, res.Score, res.ID)
			fmt.Printf("     Tool: %s | Action: %s | Domain: %s\n", res.Tool, res.Action, res.Domain)
			if res.Destructive {
				fmt.Printf("     ⚠️  Destructive\n")
			}
		}
	}
}

// testRoutes retorna un mapa de rutas para pruebas
func testRoutes() map[string]toolutil.ActionMap {
	return map[string]toolutil.ActionMap{
		"gitlab_project": toolutil.ActionMap{
			"list": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"projects": true}, nil
				},
			},
			"create": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"created": true}, nil
				},
			},
			"delete": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"deleted": true}, nil
				},
				Destructive: true,
			},
			"hook_add": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"hook": "added"}, nil
				},
			},
		},
		"gitlab_merge_request": toolutil.ActionMap{
			"list": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"mrs": true}, nil
				},
			},
			"approve": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"approved": true}, nil
				},
			},
			"merge": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"merged": true}, nil
				},
				Destructive: true,
			},
		},
		"gitlab_issue": toolutil.ActionMap{
			"list": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"issues": true}, nil
				},
			},
			"create": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"created": true}, nil
				},
			},
			"update": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"updated": true}, nil
				},
			},
			"close": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"closed": true}, nil
				},
			},
		},
		"gitlab_branch": toolutil.ActionMap{
			"list": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"branches": true}, nil
				},
			},
			"protect": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"protected": true}, nil
				},
			},
			"unprotect": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"unprotected": true}, nil
				},
			},
		},
		"gitlab_pipeline": toolutil.ActionMap{
			"list": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"pipelines": true}, nil
				},
			},
			"run": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"triggered": true}, nil
				},
			},
		},
		"gitlab_ci_variable": toolutil.ActionMap{
			"list": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"variables": true}, nil
				},
			},
			"create": toolutil.ActionRoute{
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"created": true}, nil
				},
			},
		},
	}
}
