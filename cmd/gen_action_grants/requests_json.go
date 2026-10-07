package main

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// requestsPath is the committed record of what each action sends. Its shape
// is actionrequests', since R-GRANT reads the same file back.
const requestsPath = actionrequests.RecordPath

// renderRequests renders the record of what every action sends, with the
// classic scope of each request and of each action, and the scopes each
// action's catalog group demands besides, by action ID.
func renderRequests(actions []join.Action, groupScopes map[string][]string) []byte {
	records := make([]actionrequests.RecordAction, 0, len(actions))
	for _, act := range actions {
		entry := actionrequests.RecordAction{
			ID: act.ID, Handlers: act.Handlers, Declaration: act.Declaration, Paths: act.Paths,
			Classic: actionClassic(act.Row).String(), GroupScopes: groupScopes[act.ID],
		}
		for _, request := range act.Requests {
			item := actionrequests.RecordRequest{
				Kind: string(request.Kind), Class: string(request.Class), Sites: request.Sites, SDKMethods: request.SDKMethods,
				Directives: request.Directives, Declaration: request.Declaration,
				Classic: request.Classic.String(), ClassicDeclaration: request.ClassicDeclaration,
			}
			switch request.Kind {
			case derive.KindREST:
				item.Route, item.Derived = request.Name, request.Route
			case derive.KindGraphQL:
				item.Operation, item.RootFields, item.Positions = request.Name, request.RootFields, request.Positions
			default:
				item.Reason = request.Reason
			}
			entry.Requests = append(entry.Requests, item)
		}
		records = append(records, entry)
	}
	return actionrequests.RenderRecord(records)
}

// actionClassic is a row's classic scope, unknown for an action the join
// could not place, which is one a run with findings never writes.
func actionClassic(row *finegrained.Requirement) finegrained.ClassicScope {
	if row == nil {
		return finegrained.ClassicUnknown
	}
	return row.Classic
}
