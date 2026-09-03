package sdk

import (
	"go/types"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
)

// sdkService is one service the client-go Client struct declares, held against
// the decision every service owes: covered by a call, or declared an exception.
type sdkService struct {
	// Service is the bare service name, e.g. Branches.
	Service string `json:"service"`
	// Field is the Client struct field that publishes it. It differs from the
	// service name often enough (Boards is IssueBoards, GroupCluster is
	// GroupClusters) that reading the exception table against field names
	// invents services that do not exist.
	Field string `json:"field"`
	// Interface is the declared interface type name.
	Interface string `json:"interface"`
	// APIMethods is how many endpoint methods it declares.
	APIMethods int `json:"api_methods"`
	// Status is covered, declared or undeclared.
	Status string `json:"status"`
	// Category and Reason are set for a declared exception.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Packages lists the internal/tools packages that call it, when covered.
	Packages []string `json:"packages,omitempty"`
}

// collectSDKServices enumerates every service the client-go Client struct
// declares. Reading the universe from the struct rather than from our own call
// sites is the whole point of this scope: a service nothing references never
// entered the call-site map, so an entire new upstream service could be added
// and audited as zero gaps.
func collectSDKServices(clientGo *types.Package) ([]sdkService, error) {
	st, err := shared.ClientStruct(clientGo)
	if err != nil {
		return nil, err
	}
	var services []sdkService
	for field := range st.Fields() {
		if !field.Exported() {
			continue
		}
		named, ok := shared.ClientGoServiceInterface(field.Type())
		if !ok {
			continue
		}
		services = append(services, sdkService{
			Service:    shared.ServiceName(named),
			Field:      field.Name(),
			Interface:  named.Obj().Name(),
			APIMethods: len(shared.APIMethodNames(named)),
		})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Service < services[j].Service })
	return services, nil
}

// adjudicateServices assigns every enumerated service its status from the
// call-site universe and the declared-exception table. The table is a
// parameter rather than the package global so a test can state the whole
// universe it is asserting about.
func adjudicateServices(services []sdkService, usage map[string]*shared.ServiceUsage, declaredBy map[string]declaration) []sdkService {
	out := make([]sdkService, 0, len(services))
	for _, service := range services {
		if use, called := usage[service.Interface]; called {
			service.Status = statusCovered
			service.Packages = shared.SortedSet(use.Packages)
			out = append(out, service)
			continue
		}
		if declared, ok := declaredBy[service.Service]; ok {
			service.Status = statusDeclared
			service.Category = declared.Category
			service.Reason = declared.Reason
			out = append(out, service)
			continue
		}
		service.Status = statusUndeclared
		out = append(out, service)
	}
	return out
}

// staleServiceDeclarations lists declared exceptions that no longer apply: a
// service our code now calls, or one upstream has removed. Either way the
// declaration is a claim about code that has moved on, and leaving it in place
// is how a table stops describing reality.
func staleServiceDeclarations(services []sdkService, declaredBy map[string]declaration) []string {
	byName := map[string]sdkService{}
	for _, service := range services {
		byName[service.Service] = service
	}
	var stale []string
	for name := range declaredBy {
		service, exists := byName[name]
		switch {
		case !exists:
			stale = append(stale, "service "+name+" (no longer declared by client-go)")
		case service.Status == statusCovered:
			stale = append(stale, "service "+name+" (now called directly)")
		}
	}
	sort.Strings(stale)
	return stale
}

// serviceIndex keys the enumerated services by bare name, for the GraphQL rule's
// package-to-service lookup.
func serviceIndex(services []sdkService) map[string]sdkService {
	index := make(map[string]sdkService, len(services))
	for _, service := range services {
		index[service.Service] = service
	}
	return index
}
