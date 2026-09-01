// Command audit_gateway_chars scans everything a client receives from
// tools/list, prompts/list and resources/list, on every tool surface and at the
// widest tier, for characters that MCP gateway validators reject.
//
// It exists because of a real rejection: a gateway introspecting this server
// refused onboarding with "Description contains unsafe characters: ';'". The
// semicolons were ordinary English punctuation, but the gateway is the door,
// and the door's rules win. This audit measures the served surface rather than
// grepping the source, because what matters is what crosses the wire: a
// description is assembled from several source strings, and a semicolon that
// survives assembly is a rejection wherever it came from.
//
// With -check it exits non-zero when any offending character is served, which
// is the CI gate; without it, it prints every offender with enough context to
// find the source string.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// offendingChars is what gateway validators are known to reject in served
// text. One entry today; the audit is a list so the next report is one line.
var offendingChars = []rune{';'}

// appliedSubstitutions holds the operator's substitutions when -apply is
// passed; nil otherwise, so the default scan judges the unmodified surface.
var appliedSubstitutions []gatewaycompat.Substitution

// fullStrings switches the report from excerpts to whole strings, which is
// the shape the fixing workflow needs: the whole string is what to grep the
// source for.
var fullStrings bool

type offender struct {
	surface string
	where   string
	excerpt string
}

func main() {
	check := flag.Bool("check", false, "exit non-zero if any offending character is served")
	apply := flag.Bool("apply", false,
		"apply "+gatewaycompat.EnvVar+" before scanning, to verify a substitution config clears the audit")
	full := flag.Bool("full", false, "print each offending string whole instead of a one-line excerpt")
	flag.Parse()
	fullStrings = *full

	// os.Exit lives here, not in run: run holds the stub client's deferred
	// cleanup, and an exit inside it would skip that defer.
	os.Exit(run(*check, *apply))
}

// run performs the scan and returns the process exit code.
func run(check, apply bool) int {
	if apply {
		subs, err := gatewaycompat.FromEnv()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if len(subs) == 0 {
			fmt.Fprintf(os.Stderr, "-apply: %s is empty, nothing to apply\n", gatewaycompat.EnvVar)
			return 1
		}
		appliedSubstitutions = subs
	}

	client, cleanup, err := mcpsurface.NewStubClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub client: %v\n", err)
		return 1
	}
	defer cleanup()

	var found []offender
	found = append(found, scanTools(client)...)
	found = append(found, scanPromptsAndResources(client)...)

	sort.Slice(found, func(i, j int) bool {
		if found[i].surface != found[j].surface {
			return found[i].surface < found[j].surface
		}
		return found[i].where < found[j].where
	})

	if len(found) == 0 {
		fmt.Println("gateway character audit: nothing served carries an offending character")
		return 0
	}
	for _, f := range found {
		if fullStrings {
			// Tab-separated, because the whole string is for machines and
			// greps; the padded excerpt form is for eyes.
			fmt.Printf("%s\t%s\t%s\n", f.surface, f.where, f.excerpt)
			continue
		}
		fmt.Printf("%-11s %-52s %s\n", f.surface, f.where, f.excerpt)
	}
	fmt.Printf("gateway character audit: %d served string(s) carry an offending character\n", len(found))
	if check {
		return 1
	}
	return 0
}

// scanTools lists every surface at the widest tier and scans what a client
// would receive: names, titles, descriptions, and the input and output schemas
// with every description they embed.
func scanTools(client *gitlabclient.Client) []offender {
	var found []offender

	for _, surface := range []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta, config.ToolSurfaceIndividual} {
		listed, err := listSurface(client, surface)
		if err != nil {
			fmt.Fprintf(os.Stderr, "listing %s tools: %v\n", surface, err)
			os.Exit(1)
		}
		for _, tool := range listed {
			found = append(found, scanText(surface, "tool "+tool.Name+" description", tool.Description)...)
			found = append(found, scanText(surface, "tool "+tool.Name+" title", tool.Title)...)
			found = append(found, scanSchema(surface, "tool "+tool.Name+" input schema", tool.InputSchema)...)
			found = append(found, scanSchema(surface, "tool "+tool.Name+" output schema", tool.OutputSchema)...)
		}
	}
	return found
}

// listSurface publishes one surface on an in-memory server and returns its
// tools, the same way the token and manifest generators do.
func listSurface(client *gitlabclient.Client, surface string) ([]*mcp.Tool, error) {
	if surface == config.ToolSurfaceDynamic {
		return mcpsurface.DynamicTools(client)
	}

	session, cleanup, err := mcpsurface.Session(func(server *mcp.Server) error {
		switch surface {
		case config.ToolSurfaceMeta:
			if registerErr := tools.RegisterAllMeta(server, client, edition.TierForEnterprise(true)); registerErr != nil {
				return registerErr
			}
			tools.RegisterMCPMeta(server, client)
		case config.ToolSurfaceIndividual:
			tools.RegisterAll(server, client, edition.TierForEnterprise(true))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// One ListTools call, not the Tools iterator: the session's page size
	// holds the whole surface, and the iterator left the connection with
	// state that made the deferred Close wait forever.
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// scanPromptsAndResources covers the two shared list surfaces.
func scanPromptsAndResources(client *gitlabclient.Client) []offender {
	var found []offender

	prompts, err := mcpsurface.Prompts(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing prompts: %v\n", err)
		os.Exit(1)
	}
	for _, prompt := range prompts {
		found = append(found, scanText("prompts", "prompt "+prompt.Name+" description", prompt.Description)...)
		for _, arg := range prompt.Arguments {
			found = append(found, scanText("prompts", "prompt "+prompt.Name+" argument "+arg.Name, arg.Description)...)
		}
	}

	resources, templates, err := mcpsurface.Resources(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing resources: %v\n", err)
		os.Exit(1)
	}
	for _, resource := range resources {
		found = append(found, scanText("resources", "resource "+resource.Name+" description", resource.Description)...)
	}
	for _, template := range templates {
		found = append(found, scanText("resources", "resource template "+template.Name+" description", template.Description)...)
	}
	return found
}

// scanSchema walks a schema the way a validator does: serialized, then
// descended with the same prose walk the substitution middleware uses
// (gatewaycompat.RewriteSchemaProse), so what this audit checks and what the
// knob can fix stay the same set by construction. Keys like pattern or const
// legitimately contain punctuation that is not prose, and a validator that
// rejects a regex is not the reported problem.
func scanSchema(surface, where string, schema any) []offender {
	if schema == nil {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var decoded any
	if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
		return nil
	}
	var found []offender
	gatewaycompat.RewriteSchemaProse(decoded, func(text string) string {
		found = append(found, scanText(surface, where, text)...)
		return text
	})
	return found
}

// scanText reports each offending character in one served string, once per
// string, with an excerpt centered on the first hit. With -apply, the
// operator's substitutions run first, so the scan judges the text a gateway
// would actually receive.
func scanText(surface, where, text string) []offender {
	text = gatewaycompat.Apply(appliedSubstitutions, text)
	for _, ch := range offendingChars {
		index := strings.IndexRune(text, ch)
		if index < 0 {
			continue
		}
		start := max(index-30, 0)
		end := min(index+30, len(text))
		excerpt := strings.ReplaceAll(text[start:end], "\n", " ")
		if fullStrings {
			excerpt = strings.ReplaceAll(text, "\n", "\\n")
		}
		return []offender{{surface: surface, where: where, excerpt: excerpt}}
	}
	return nil
}
