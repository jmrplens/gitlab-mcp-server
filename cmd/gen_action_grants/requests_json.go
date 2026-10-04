package main

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
)

// requestsPath is the committed record of what each action sends. Its shape
// is actionrequests', since R-GRANT reads the same file back.
const requestsPath = actionrequests.RecordPath

// renderRequests renders the record of what every action sends.
func renderRequests(actions []join.Action) []byte {
	records := make([]actionrequests.RecordAction, 0, len(actions))
	for _, act := range actions {
		entry := actionrequests.RecordAction{ID: act.ID, Handlers: act.Handlers, Declaration: act.Declaration, Paths: act.Paths}
		for _, request := range act.Requests {
			item := actionrequests.RecordRequest{
				Kind: string(request.Kind), Class: string(request.Class), Sites: request.Sites, SDKMethods: request.SDKMethods,
				Directives: request.Directives, Declaration: request.Declaration,
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
