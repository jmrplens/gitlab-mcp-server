// graphql.go provides shared utilities for raw GraphQL queries via
// client-go's GraphQL.Do() method. It includes cursor-based pagination
// structs, GitLab Global ID (GID) helpers, and pagination summary formatting.
//
// These utilities are used by domain sub-packages that call the GitLab
// GraphQL API directly (without a client-go service wrapper), such as
// vulnerabilities, cicatalog, branchrules, and customemoji.
package toolutil

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
)

// GraphQL pagination defaults.
const (
	GraphQLDefaultFirst = 20
	GraphQLMaxFirst     = 100
)

// GraphQLPaginationInput holds cursor-based pagination parameters
// for GraphQL list queries. It mirrors the standard GraphQL connection
// model (first/after for forward, last/before for backward).
type GraphQLPaginationInput struct {
	First  *int   `json:"first,omitempty"  jsonschema:"Number of items to return (default 20, max 100)"`
	After  string `json:"after,omitempty"  jsonschema:"Cursor for forward pagination (from previous response end_cursor)"`
	Last   *int   `json:"last,omitempty"   jsonschema:"Number of items from the end (backward pagination)"`
	Before string `json:"before,omitempty" jsonschema:"Cursor for backward pagination (from previous response start_cursor)"`
}

// EffectiveFirst returns the requested page size, clamped to
// [1, GraphQLMaxFirst] with GraphQLDefaultFirst as fallback.
func (p GraphQLPaginationInput) EffectiveFirst() int {
	if p.First == nil {
		return GraphQLDefaultFirst
	}
	n := *p.First
	if n < 1 {
		return 1
	}
	if n > GraphQLMaxFirst {
		return GraphQLMaxFirst
	}
	return n
}

// Variables returns a map suitable for inclusion in a GraphQL variables
// payload. Only non-zero fields are included.
func (p GraphQLPaginationInput) Variables() map[string]any {
	v := map[string]any{}
	v["first"] = p.EffectiveFirst()
	if p.After != "" {
		v["after"] = p.After
	}
	if p.Last != nil {
		v["last"] = *p.Last
	}
	if p.Before != "" {
		v["before"] = p.Before
	}
	return v
}

// GraphQLPageInfo holds cursor-based pagination metadata returned by
// GraphQL connection responses. It maps directly to GitLab's PageInfo type.
type GraphQLPageInfo struct {
	HasNextPage     bool   `json:"has_next_page"`
	HasPreviousPage bool   `json:"has_previous_page"`
	EndCursor       string `json:"end_cursor,omitempty"`
	StartCursor     string `json:"start_cursor,omitempty"`
}

// GraphQLPaginationOutput holds pagination metadata for GraphQL list
// tool responses, presented in a consistent format for LLM consumers.
type GraphQLPaginationOutput struct {
	HasNextPage     bool   `json:"has_next_page"`
	HasPreviousPage bool   `json:"has_previous_page"`
	EndCursor       string `json:"end_cursor,omitempty"`
	StartCursor     string `json:"start_cursor,omitempty"`
}

// PageInfoToOutput converts a raw GraphQL PageInfo response struct
// (with camelCase JSON keys from the API) to the snake_case output struct.
func PageInfoToOutput(pi GraphQLRawPageInfo) GraphQLPaginationOutput {
	return GraphQLPaginationOutput(pi)
}

// GraphQLRawPageInfo matches the camelCase JSON shape returned by the
// GitLab GraphQL API before conversion to our snake_case output.
type GraphQLRawPageInfo struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	EndCursor       string `json:"endCursor"`
	StartCursor     string `json:"startCursor"`
}

// FormatGraphQLPagination renders cursor-based pagination metadata as a
// Markdown summary line, suitable for appending to list tool responses.
func FormatGraphQLPagination(p GraphQLPaginationOutput, shown int) string {
	parts := []string{fmt.Sprintf("Showing %d items", shown)}
	if p.HasNextPage {
		parts = append(parts, fmt.Sprintf("next page cursor: `%s`", p.EndCursor))
	}
	if p.HasPreviousPage {
		parts = append(parts, fmt.Sprintf("prev page cursor: `%s`", p.StartCursor))
	}
	if !p.HasNextPage && !p.HasPreviousPage {
		parts = append(parts, "no more pages")
	}
	return strings.Join(parts, " | ")
}

// GID helpers for GitLab Global IDs (gid://gitlab/Type/123).

// FormatGID builds a GitLab Global ID string from a type name and numeric ID.
//
//	FormatGID("Vulnerability", 42) → "gid://gitlab/Vulnerability/42"
func FormatGID(typeName string, id int64) string {
	return fmt.Sprintf("gid://gitlab/%s/%d", typeName, id)
}

// ParseGID extracts the type name and numeric ID from a GitLab Global ID.
// It returns an error if the format is invalid.
//
//	ParseGID("gid://gitlab/Vulnerability/42") → ("Vulnerability", 42, nil)
func ParseGID(gid string) (typeName string, id int64, err error) {
	const prefix = "gid://gitlab/"
	if !strings.HasPrefix(gid, prefix) {
		return "", 0, fmt.Errorf("invalid GitLab GID: must start with %q, got %q", prefix, gid)
	}
	rest := strings.TrimPrefix(gid, prefix)
	slash := strings.LastIndex(rest, "/")
	if slash < 0 || slash == 0 || slash == len(rest)-1 {
		return "", 0, fmt.Errorf("invalid GitLab GID format: expected gid://gitlab/Type/ID, got %q", gid)
	}
	typeName = rest[:slash]
	id, err = strconv.ParseInt(rest[slash+1:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid GitLab GID: ID %q is not a valid integer in %q", rest[slash+1:], gid)
	}
	return typeName, id, nil
}

// MergeVariables merges multiple variable maps into a single map.
// Later maps override earlier ones for duplicate keys.
func MergeVariables(sources ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range sources {
		maps.Copy(result, m)
	}
	return result
}
