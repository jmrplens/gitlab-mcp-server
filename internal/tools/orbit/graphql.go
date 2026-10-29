// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The operations a GraphQL Orbit failure is reported under.
const (
	orbitStatusOp      = "orbit_status"
	orbitSchemaOp      = "orbit_schema"
	orbitGraphStatusOp = "orbit_graph_status"
)

// queryServiceStatus asks for the health of the Orbit service. GitLab names
// the overall and each component's health state; the document asks for both
// as status, the name StatusOutput has always published them under.
// serviceStatus can be resolved once per request, which this document does.
const queryServiceStatus = `
query OrbitServiceStatus {
  orbit {
    serviceStatus {
      status: state
      version
      components {
        name
        status: state
      }
    }
  }
}
`

// gqlServiceStatus is Query.orbit.serviceStatus as [queryServiceStatus]
// selects it.
type gqlServiceStatus struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	Components []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"components"`
}

// Status retrieves the health of the Orbit service on GitLab.com.
//
// Source: GitLab's GraphQL API, Query.orbit.serviceStatus, which replaced GET
// /api/v4/orbit/status when gitlab-org/gitlab!265633 removed the route on
// 2026-10-27 (client-go's Orbit.GetStatus still sends that GET; register row
// in docs/development/upstream-bugs.md). GitLab answers serviceStatus null
// when Orbit is not available on the instance or for the user, which is
// reported as an error naming what Orbit needs, not as an empty status.
func Status(ctx context.Context, client *gitlabclient.Client, input StatusInput) (StatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusOutput{}, err
	}
	if _, _, err := responseFormat(input.ResponseFormat, "response_format"); err != nil {
		return StatusOutput{}, err
	}
	var resp struct {
		Data struct {
			Orbit *struct {
				ServiceStatus *gqlServiceStatus `json:"serviceStatus"`
			} `json:"orbit"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: queryServiceStatus}, &resp, gl.WithContext(ctx)); err != nil {
		return StatusOutput{}, wrapOrbitErr(orbitStatusOp, err)
	}
	if err := toolutil.GraphQLTopLevelError(orbitStatusOp, resp.Errors); err != nil {
		return StatusOutput{}, err
	}
	if resp.Data.Orbit == nil || resp.Data.Orbit.ServiceStatus == nil {
		return StatusOutput{}, errOrbitUnavailable(orbitStatusOp, "the Orbit service status")
	}
	status := resp.Data.Orbit.ServiceStatus
	out := StatusOutput{Status: status.Status, Version: status.Version}
	for _, component := range status.Components {
		out.Components = append(out.Components, StatusComponent{Name: component.Name, Status: component.Status})
	}
	return out, nil
}

// ErrOrbitUnavailable is wrapped by every error an Orbit action returns when
// GitLab answered what it asked with null, which is how GitLab's GraphQL API
// answers a user or a namespace Orbit is not available to. The record
// generator reads it to tell GitLab declining the account from a shape the
// handler could not read.
var ErrOrbitUnavailable = errors.New("orbit is not available to this token")

// errOrbitUnavailable is what an Orbit action answers when GitLab answered
// what it asked with null: Orbit is not available to the token there. It is
// an error rather than an empty answer, since an empty status or schema would
// read as a service with nothing in it.
//
// It is written out rather than passed through [toolutil.WrapErrWithHint],
// which classifies an HTTP failure and would call an answer GitLab gave with
// 200 an unexpected error.
func errOrbitUnavailable(op, what string) error {
	return fmt.Errorf("%s: GitLab answered %s with null: %w. Suggestion: %s", op, what, ErrOrbitUnavailable, orbitUnavailableHint)
}

// schemaPageSize is the page size of every connection [querySchema] asks for,
// GitLab's maximum. GitLab.com's schema held 33 node kinds, 60 edge kinds, at
// most 44 properties on a node and at most 24 endpoints on an edge on
// 2026-10-29, so one page answers today.
const schemaPageSize = 100

// maxSchemaPages bounds how many requests one [Schema] call sends while it
// follows the node and edge cursors, so an answer whose cursor never ends
// cannot hold the call forever.
const maxSchemaPages = 20

// querySchema asks for one page of the Orbit graph schema's node kinds and of
// its edge kinds, with each node's properties when $withProperties is set.
// GitLab resolves schema once per request, so each page is its own request,
// and a connection already read whole is asked again after its last cursor,
// which GitLab answers with no node. The names the output publishes are asked
// for by alias.
const querySchema = `
query OrbitSchema($nodesAfter: String, $edgesAfter: String, $withProperties: Boolean!) {
  orbit {
    schema {
      nodes(first: 100, after: $nodesAfter) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          name
          description
          primary_key: primaryKey
          label_field: labelProperty
          domain {
            name
          }
          display {
            color
            size
          }
          properties(first: 100) @include(if: $withProperties) {
            pageInfo {
              hasNextPage
            }
            nodes {
              name
              data_type: dataType
              nullable
              enum_values: enumValues
            }
          }
        }
      }
      edges(first: 100, after: $edgesAfter) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          name
          description
          variants: endpoints(first: 100) {
            pageInfo {
              hasNextPage
            }
            nodes {
              source {
                name
              }
              target {
                name
              }
            }
          }
        }
      }
    }
  }
}
`

// gqlNestedPageInfo is the page info of a connection nested in a schema
// entry, which is read only to say it was not read whole.
type gqlNestedPageInfo struct {
	HasNextPage bool `json:"hasNextPage"`
}

// gqlSchemaNode is one OrbitSchemaNode as [querySchema] selects it.
type gqlSchemaNode struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PrimaryKey  string `json:"primary_key"`
	LabelField  string `json:"label_field"`
	Domain      struct {
		Name string `json:"name"`
	} `json:"domain"`
	Display *struct {
		Color string `json:"color"`
		Size  int64  `json:"size"`
	} `json:"display"`
	Properties *struct {
		PageInfo gqlNestedPageInfo `json:"pageInfo"`
		Nodes    []SchemaProperty  `json:"nodes"`
	} `json:"properties"`
}

// gqlSchemaEdge is one OrbitSchemaEdge as [querySchema] selects it.
type gqlSchemaEdge struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Variants    struct {
		PageInfo gqlNestedPageInfo `json:"pageInfo"`
		Nodes    []struct {
			Source struct {
				Name string `json:"name"`
			} `json:"source"`
			Target struct {
				Name string `json:"name"`
			} `json:"target"`
		} `json:"nodes"`
	} `json:"variants"`
}

// gqlSchemaPage is one answer to [querySchema].
type gqlSchemaPage struct {
	Data struct {
		Orbit *struct {
			Schema *gqlSchema `json:"schema"`
		} `json:"orbit"`
	} `json:"data"`
	Errors []toolutil.GraphQLError `json:"errors"`
}

// Schema retrieves the Orbit graph schema (node kinds, edge kinds, and the
// properties of the node kinds named in expand) from GitLab.com.
//
// Source: GitLab's GraphQL API, Query.orbit.schema, for the structured answer,
// which replaced GET /api/v4/orbit/schema when gitlab-org/gitlab!265633 removed
// the route on 2026-10-27 (client-go's Orbit.GetSchema still sends that GET).
// GraphQL has no compact text rendering, so format=llm is answered by the
// get_graph_schema command of GitLab's Orbit MCP endpoint ([schemaText]), the
// one source of it GitLab offers.
//
// expand keeps the meaning it had on the REST route: without it no property is
// asked for, and with it the properties are asked for and kept on the named
// nodes alone, matched without regard to case.
func Schema(ctx context.Context, client *gitlabclient.Client, input SchemaInput) (SchemaOutput, error) {
	if err := ctx.Err(); err != nil {
		return SchemaOutput{}, err
	}
	format, err := schemaResponseFormat(input)
	if err != nil {
		return SchemaOutput{}, err
	}
	if format != nil && *format == gl.OrbitResponseFormatLLM {
		//gitlab:request alternatives: format llm is answered by get_graph_schema of the Orbit MCP endpoint, and GitLab's GraphQL schema below is not asked
		return schemaText(ctx, client, mcpPath, input.Expand)
	}
	//gitlab:request alternatives: every other format is answered by GitLab's GraphQL schema, its first page on every call, and the MCP endpoint is not asked
	return schemaStructured(ctx, client, input.Expand)
}

// schemaStructured reads the schema through GraphQL, page by page, and
// projects it into [SchemaOutput].
func schemaStructured(ctx context.Context, client *gitlabclient.Client, expand []string) (SchemaOutput, error) {
	withProperties := len(expand) > 0
	page, err := schemaPage(ctx, client, map[string]any{"withProperties": withProperties})
	if err != nil {
		return SchemaOutput{}, err
	}
	var (
		nodes []gqlSchemaNode
		edges []gqlSchemaEdge
	)
	for pages := 1; ; pages++ {
		nodes = append(nodes, page.Nodes.Nodes...)
		edges = append(edges, page.Edges.Nodes...)
		if !page.Nodes.PageInfo.HasNextPage && !page.Edges.PageInfo.HasNextPage {
			return convertSchema(nodes, edges, expand)
		}
		if pages == maxSchemaPages {
			return SchemaOutput{}, toolutil.WrapErr(orbitSchemaOp, fmt.Errorf("the Orbit schema did not end within %d pages of %d", maxSchemaPages, schemaPageSize))
		}
		// A connection read whole is asked again after its last cursor,
		// which GitLab answers with no node, since one request asks for both.
		variables := map[string]any{"withProperties": withProperties}
		if cursor := page.Nodes.PageInfo.EndCursor; cursor != "" {
			variables["nodesAfter"] = cursor
		}
		if cursor := page.Edges.PageInfo.EndCursor; cursor != "" {
			variables["edgesAfter"] = cursor
		}
		//gitlab:request optional: a later page is asked only when GitLab says the node or the edge kinds have more than one page holds, which they did not on 2026-10-29
		page, err = schemaPage(ctx, client, variables)
		if err != nil {
			return SchemaOutput{}, err
		}
	}
}

// gqlSchema is Query.orbit.schema as one page of [querySchema] answers it.
type gqlSchema struct {
	Nodes struct {
		PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
		Nodes    []gqlSchemaNode                    `json:"nodes"`
	} `json:"nodes"`
	Edges struct {
		PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
		Nodes    []gqlSchemaEdge                    `json:"nodes"`
	} `json:"edges"`
}

// schemaPage asks GitLab's GraphQL API for one page of the Orbit schema.
func schemaPage(ctx context.Context, client *gitlabclient.Client, variables map[string]any) (*gqlSchema, error) {
	var page gqlSchemaPage
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: querySchema, Variables: variables}, &page, gl.WithContext(ctx)); err != nil {
		return nil, wrapOrbitErr(orbitSchemaOp, err)
	}
	if err := toolutil.GraphQLTopLevelError(orbitSchemaOp, page.Errors); err != nil {
		return nil, err
	}
	if page.Data.Orbit == nil || page.Data.Orbit.Schema == nil {
		return nil, errOrbitUnavailable(orbitSchemaOp, "the Orbit schema")
	}
	return page.Data.Orbit.Schema, nil
}

// errSchemaNested is what [convertSchema] answers for a node's properties or
// an edge's endpoints that one page did not hold: the schema would be
// published without part of an entry, with nothing to say so.
var errSchemaNested = errors.New("holds more entries than one page of the Orbit schema answers")

// convertSchema projects the schema pages into [SchemaOutput]: the nodes with
// the properties of the expanded ones, the domains grouped from each node's
// domain in the order GitLab listed the nodes, and the edges with the pair of
// node kinds each endpoint names.
func convertSchema(nodes []gqlSchemaNode, edges []gqlSchemaEdge, expand []string) (SchemaOutput, error) {
	out := SchemaOutput{}
	domainIndex := map[string]int{}
	for _, node := range nodes {
		converted := SchemaNode{
			Name:        node.Name,
			Description: node.Description,
			Domain:      node.Domain.Name,
			PrimaryKey:  node.PrimaryKey,
			LabelField:  node.LabelField,
		}
		if node.Display != nil {
			converted.Display = &SchemaNodeDisplay{Color: node.Display.Color, Size: node.Display.Size}
		}
		if node.Properties != nil && expanded(node.Name, expand) {
			if node.Properties.PageInfo.HasNextPage {
				return SchemaOutput{}, toolutil.WrapErr(orbitSchemaOp, fmt.Errorf("the properties of node %s %w", node.Name, errSchemaNested))
			}
			converted.Properties = node.Properties.Nodes
		}
		out.Nodes = append(out.Nodes, converted)
		index, seen := domainIndex[node.Domain.Name]
		if !seen {
			index = len(out.Domains)
			domainIndex[node.Domain.Name] = index
			out.Domains = append(out.Domains, SchemaDomain{Name: node.Domain.Name})
		}
		out.Domains[index].NodeNames = append(out.Domains[index].NodeNames, node.Name)
	}
	for _, edge := range edges {
		if edge.Variants.PageInfo.HasNextPage {
			return SchemaOutput{}, toolutil.WrapErr(orbitSchemaOp, fmt.Errorf("the endpoints of edge %s %w", edge.Name, errSchemaNested))
		}
		converted := SchemaEdge{Name: edge.Name, Description: edge.Description}
		for _, endpoint := range edge.Variants.Nodes {
			converted.Variants = append(converted.Variants, SchemaEdgeVariant{SourceType: endpoint.Source.Name, TargetType: endpoint.Target.Name})
		}
		out.Edges = append(out.Edges, converted)
	}
	return out, nil
}

// expanded reports whether a node kind is one of those expand names, ignoring
// case, since a caller writes a label the way a query does and GitLab's are
// capitalized.
func expanded(name string, expand []string) bool {
	for _, wanted := range expand {
		if strings.EqualFold(strings.TrimSpace(wanted), name) {
			return true
		}
	}
	return false
}

// The three documents [GraphStatus] asks a namespace's index with, one per way
// of naming it. Each reads the same selection of the Orbit namespace, under
// the names GraphStatusOutput publishes, written out in each rather than as a
// fragment, so each document is whole where it is sent and the audits that
// read documents judge every one with its decoder. indexingStatus and
// itemCounts.areas are not asked: they carry what indexing and
// itemCounts.domains do under GitLab's older and newer names. access and trial
// are the access gate rather than the index.

// queryGraphStatusByPath asks for the Orbit index of a full path, which names
// a group or a project; GitLab answers the other root null.
const queryGraphStatusByPath = `
query OrbitGraphStatusByPath($fullPath: ID!) {
  group(fullPath: $fullPath) {
    orbit {
      enabled
      enabled_at: enabledAt
      indexing {
        state
        last_completed_at: lastCompletedAt
        projects {
          indexed
          total_known: totalKnown
          gaps
          not_indexed: notIndexed
          total
        }
        areas {
          name
          state
        }
      }
      itemCounts {
        domains {
          name
          items: entities {
            name
            count
          }
        }
      }
    }
  }
  project(fullPath: $fullPath) {
    orbit {
      enabled
      enabled_at: enabledAt
      indexing {
        state
        last_completed_at: lastCompletedAt
        projects {
          indexed
          total_known: totalKnown
          gaps
          not_indexed: notIndexed
          total
        }
        areas {
          name
          state
        }
      }
      itemCounts {
        domains {
          name
          items: entities {
            name
            count
          }
        }
      }
    }
  }
}
`

// queryGraphStatusByGroupID asks for the Orbit index of a group by its global
// ID.
const queryGraphStatusByGroupID = `
query OrbitGraphStatusByGroupID($ids: [ID!]) {
  groups(ids: $ids, first: 1) {
    nodes {
      orbit {
        enabled
        enabled_at: enabledAt
        indexing {
          state
          last_completed_at: lastCompletedAt
          projects {
            indexed
            total_known: totalKnown
            gaps
            not_indexed: notIndexed
            total
          }
          areas {
            name
            state
          }
        }
        itemCounts {
          domains {
            name
            items: entities {
              name
              count
            }
          }
        }
      }
    }
  }
}
`

// queryGraphStatusByProjectID asks for the Orbit index of a project by its
// global ID.
const queryGraphStatusByProjectID = `
query OrbitGraphStatusByProjectID($ids: [ID!]) {
  projects(ids: $ids, first: 1) {
    nodes {
      orbit {
        enabled
        enabled_at: enabledAt
        indexing {
          state
          last_completed_at: lastCompletedAt
          projects {
            indexed
            total_known: totalKnown
            gaps
            not_indexed: notIndexed
            total
          }
          areas {
            name
            state
          }
        }
        itemCounts {
          domains {
            name
            items: entities {
              name
              count
            }
          }
        }
      }
    }
  }
}
`

// gqlGraphStatus is an OrbitNamespace as the graph status documents select it.
type gqlGraphStatus struct {
	Enabled   *bool  `json:"enabled"`
	EnabledAt string `json:"enabled_at"`
	Indexing  *struct {
		State           string               `json:"state"`
		LastCompletedAt string               `json:"last_completed_at"`
		Projects        *GraphStatusProjects `json:"projects"`
		Areas           []GraphStatusArea    `json:"areas"`
	} `json:"indexing"`
	ItemCounts *struct {
		Domains []struct {
			Name  string `json:"name"`
			Items []struct {
				Name  string `json:"name"`
				Count string `json:"count"`
			} `json:"items"`
		} `json:"domains"`
	} `json:"itemCounts"`
}

// gqlOrbitHolder is a group or project as the graph status documents select
// it: its Orbit namespace alone.
type gqlOrbitHolder struct {
	Orbit *gqlGraphStatus `json:"orbit"`
}

// GraphStatus retrieves the Orbit indexing status of one group or project on
// GitLab.com.
//
// Source: GitLab's GraphQL API, Group.orbit and Project.orbit, which replaced
// GET /api/v4/orbit/graph_status when gitlab-org/gitlab!264745 removed the
// route on 2026-10-28 (issue 1516; client-go's Orbit.GetGraphStatus still
// sends that GET). Exactly one of namespace_id, project_id or full_path is
// set: a full path is asked as both a group and a project, and an ID as the
// global ID of the one it names. GitLab answers indexing and itemCounts null
// when Orbit is not available to the token there or is off for the top-level
// group, which is reported as an error naming what Orbit needs.
func GraphStatus(ctx context.Context, client *gitlabclient.Client, input GraphStatusInput) (GraphStatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return GraphStatusOutput{}, err
	}
	fullPath, err := graphStatusScope(input)
	if err != nil {
		return GraphStatusOutput{}, err
	}
	var (
		holder *gqlOrbitHolder
		scope  string
	)
	switch {
	case input.NamespaceID > 0:
		scope = "group " + strconv.FormatInt(input.NamespaceID, 10)
		holder, err = graphStatusByGroupID(ctx, client, input.NamespaceID)
	case input.ProjectID > 0:
		scope = "project " + strconv.FormatInt(input.ProjectID, 10)
		holder, err = graphStatusByProjectID(ctx, client, input.ProjectID)
	default:
		scope = "group or project " + fullPath
		holder, err = graphStatusByPath(ctx, client, fullPath)
	}
	if err != nil {
		return GraphStatusOutput{}, err
	}
	if holder == nil {
		return GraphStatusOutput{}, fmt.Errorf("%s: GitLab answered no %s visible to this token. Suggestion: check the ID or full path, and that the token can read the group or project", orbitGraphStatusOp, scope)
	}
	if holder.Orbit == nil || (holder.Orbit.Indexing == nil && holder.Orbit.ItemCounts == nil) {
		return GraphStatusOutput{}, errOrbitUnavailable(orbitGraphStatusOp, "the indexing status of "+scope)
	}
	return convertGraphStatus(holder.Orbit), nil
}

// graphStatusAnswered classifies a sent graph status request: the HTTP
// failure, then the top-level GraphQL errors.
func graphStatusAnswered(err error, responseErrors []toolutil.GraphQLError) error {
	if err != nil {
		return wrapOrbitErr(orbitGraphStatusOp, err)
	}
	return toolutil.GraphQLTopLevelError(orbitGraphStatusOp, responseErrors)
}

// graphStatusByPath asks for the Orbit index of a full path, and returns the
// group or the project GitLab answered, or nil for neither.
func graphStatusByPath(ctx context.Context, client *gitlabclient.Client, fullPath string) (*gqlOrbitHolder, error) {
	var resp struct {
		Data struct {
			Group   *gqlOrbitHolder `json:"group"`
			Project *gqlOrbitHolder `json:"project"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	_, sendErr := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: queryGraphStatusByPath, Variables: map[string]any{"fullPath": fullPath}}, &resp, gl.WithContext(ctx))
	if err := graphStatusAnswered(sendErr, resp.Errors); err != nil {
		return nil, err
	}
	if resp.Data.Group != nil {
		return resp.Data.Group, nil
	}
	return resp.Data.Project, nil
}

// graphStatusByGroupID asks for the Orbit index of a group by its global ID.
func graphStatusByGroupID(ctx context.Context, client *gitlabclient.Client, id int64) (*gqlOrbitHolder, error) {
	var resp struct {
		Data struct {
			Groups *gqlHolderList `json:"groups"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	ids := []string{"gid://gitlab/Group/" + strconv.FormatInt(id, 10)}
	_, sendErr := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: queryGraphStatusByGroupID, Variables: map[string]any{"ids": ids}}, &resp, gl.WithContext(ctx))
	if err := graphStatusAnswered(sendErr, resp.Errors); err != nil {
		return nil, err
	}
	return resp.Data.Groups.first(), nil
}

// graphStatusByProjectID asks for the Orbit index of a project by its global
// ID.
func graphStatusByProjectID(ctx context.Context, client *gitlabclient.Client, id int64) (*gqlOrbitHolder, error) {
	var resp struct {
		Data struct {
			Projects *gqlHolderList `json:"projects"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	ids := []string{"gid://gitlab/Project/" + strconv.FormatInt(id, 10)}
	_, sendErr := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: queryGraphStatusByProjectID, Variables: map[string]any{"ids": ids}}, &resp, gl.WithContext(ctx))
	if err := graphStatusAnswered(sendErr, resp.Errors); err != nil {
		return nil, err
	}
	return resp.Data.Projects.first(), nil
}

// gqlHolderList is the connection groups(ids:) and projects(ids:) answer.
type gqlHolderList struct {
	Nodes []gqlOrbitHolder `json:"nodes"`
}

// first returns the list's first node, or nil for an absent or empty list.
func (l *gqlHolderList) first() *gqlOrbitHolder {
	if l == nil || len(l.Nodes) == 0 {
		return nil
	}
	return &l.Nodes[0]
}

// graphStatusScope validates that exactly one of namespace_id, project_id, or
// full_path is set, and that the response format is one the REST route took,
// and returns the full path trimmed.
func graphStatusScope(input GraphStatusInput) (string, error) {
	if input.NamespaceID < 0 {
		return "", errors.New("namespace_id must not be negative")
	}
	if input.ProjectID < 0 {
		return "", errors.New("project_id must not be negative")
	}
	if _, _, err := responseFormat(input.ResponseFormat, "response_format"); err != nil {
		return "", err
	}
	scopeCount := 0
	if input.NamespaceID > 0 {
		scopeCount++
	}
	if input.ProjectID > 0 {
		scopeCount++
	}
	fullPath := strings.TrimSpace(input.FullPath)
	if fullPath != "" {
		scopeCount++
	}
	if scopeCount != 1 {
		return "", errors.New("set exactly one of namespace_id, project_id, or full_path")
	}
	return fullPath, nil
}

// convertGraphStatus projects an Orbit namespace into [GraphStatusOutput]: the
// project counts where the output has always published them, at the top, and
// each entity count, which GitLab's GraphQL API spells as a BigInt string, as
// a number.
func convertGraphStatus(status *gqlGraphStatus) GraphStatusOutput {
	out := GraphStatusOutput{Enabled: status.Enabled, EnabledAt: status.EnabledAt}
	if status.Indexing != nil {
		out.Projects = status.Indexing.Projects
		out.Indexing = &GraphStatusIndexing{
			State:           status.Indexing.State,
			LastCompletedAt: status.Indexing.LastCompletedAt,
			Areas:           status.Indexing.Areas,
		}
	}
	if status.ItemCounts != nil {
		for _, domain := range status.ItemCounts.Domains {
			converted := GraphStatusDomain{Name: domain.Name}
			for _, item := range domain.Items {
				converted.Items = append(converted.Items, GraphStatusDomainItem{Name: item.Name, Count: toolutil.GraphQLNumber(item.Count)})
			}
			out.Domains = append(out.Domains, converted)
		}
	}
	return out
}
