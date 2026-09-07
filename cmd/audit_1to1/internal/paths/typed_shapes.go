package paths

import (
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
)

// TypedShapeCheck is the same question [ShapeCheck] asks, at type grain: not
// what the endpoints a package calls answer with, but what the endpoints one
// output type actually models answer with.
//
// The chain is deterministic and every link already existed. A converter pairs
// an output type with a client-go struct, which is what the field diff runs
// over; client-go's service methods say which endpoints answer with that
// struct; and GitLab's document says what those endpoints send. Nothing in it
// consults the request inventory, which records a package and never an action
// and so cannot be sharpened.
//
// It reports and never gates, for the same reason the package-grain join does,
// and it is kept beside that join rather than replacing it: a reader comparing
// the two sees which of the package-grain findings survive a comparison that
// searched only the right responses.
type TypedShapeCheck struct {
	// Ran is false when the tool packages could not be loaded or their import
	// graph named no client-go to read, which are the only ways this join is
	// skipped.
	Ran bool `json:"ran"`
	// Compared counts the output types held against the response of the
	// operations their client-go struct models.
	Compared int `json:"compared"`
	// SkippedNoPairing counts the output types no converter pairs with a
	// client-go struct. They are our own wrappers around a JSON array, our own
	// answers to a 204 and to a not-found, and the synthetic results of a
	// handler that calls nothing: no endpoint sends their keys because they are
	// not an endpoint's response, and the package-grain join already carries
	// them as the lower bound it is.
	SkippedNoPairing int `json:"skipped_no_pairing"`
	// SkippedNoRoute counts the output types whose client-go struct no service
	// method was seen answering with, so there is no endpoint to ask about.
	SkippedNoRoute int `json:"skipped_no_route"`
	// SkippedNoSchema counts the output types whose every route either is
	// absent from GitLab's document or carries no response schema there. An
	// empty union means the document does not say rather than that GitLab sends
	// nothing, so judging against one would condemn a whole type for a hole in
	// the record.
	SkippedNoSchema int `json:"skipped_no_schema"`
	// Unpublished are the findings: an output field GitLab's document does not
	// list for any operation the type models.
	Unpublished []UnpublishedField `json:"unpublished,omitempty"`
}

// Grain names which join produced a finding, spelled once.
const (
	grainPackage = "package"
	grainType    = "type"
)

// Seams for the two inputs the real tree resolves and a test cannot: loading
// the typed tool packages costs twenty seconds, and the client-go source lives
// in a module cache. Each is a variable a test restores.
var (
	collectPairings = structs.CollectOutputPairings
	readRoutes      = readSDKRoutes
)

// typedShapeCheck compares each output type with the responses of the
// operations its client-go struct models.
func typedShapeCheck(root string, index *operationIndex, published []publishedType) TypedShapeCheck {
	pairings, err := collectPairings(root)
	if err != nil || pairings.ClientGoDir == "" {
		return TypedShapeCheck{}
	}
	routes := readRoutes(pairings.ClientGoDir)
	sdkTypes := pairedSDKTypes(pairings.Outputs)

	check := TypedShapeCheck{Ran: true}
	for _, candidate := range published {
		paired := sdkTypes[[2]string{shortPackage(candidate.Package), candidate.Name}]
		if len(paired) == 0 {
			check.SkippedNoPairing++
			continue
		}
		operations, known := describedRoutes(paired, routes, index)
		switch {
		case len(operations) == 0:
			check.SkippedNoRoute++
		case len(known) == 0:
			check.SkippedNoSchema++
		default:
			check.Compared++
			check.Unpublished = append(check.Unpublished, unpublishedAtTypeGrain(candidate, paired, operations, known)...)
		}
	}
	sortFindings(check.Unpublished)
	return check
}

// pairedSDKTypes indexes the client-go structs each output type models.
func pairedSDKTypes(pairings []structs.OutputPairing) map[[2]string][]string {
	out := map[[2]string][]string{}
	for _, pairing := range pairings {
		key := [2]string{pairing.Package, pairing.MCPType}
		if !slices.Contains(out[key], pairing.SDKType) {
			out[key] = append(out[key], pairing.SDKType)
		}
	}
	for key := range out {
		sort.Strings(out[key])
	}
	return out
}

// describedRoutes collects the endpoints every paired client-go struct is
// answered from and the union of the response names the document gives them.
//
// An empty operations list is a type nothing routes to; an empty union is a
// type whose routes the document says nothing about. The caller keeps those two
// apart, because only the second is a statement about GitLab's record.
func describedRoutes(paired []string, routes map[string][]sdkRoute, index *operationIndex) (operations []string, known map[string]bool) {
	seen := map[string]bool{}
	known = map[string]bool{}
	for _, sdkType := range paired {
		for _, route := range routes[sdkType] {
			if name := route.operation(); !seen[name] {
				seen[name] = true
				operations = append(operations, name)
			}
			// Only an exact match: a loose one accepts a literal segment of
			// ours where GitLab has a placeholder, which is evidence about a
			// fixture value in the inventory and would be a guess here.
			operation, quality, _ := index.lookup(route.Method, route.Path)
			if quality != matchExact {
				continue
			}
			for _, name := range operation.Response {
				known[name] = true
			}
		}
	}
	sort.Strings(operations)
	return operations, known
}

// unpublishedAtTypeGrain reports every field of one type that no operation it
// models declares.
func unpublishedAtTypeGrain(candidate publishedType, paired, operations []string, known map[string]bool) []UnpublishedField {
	var out []UnpublishedField
	for _, field := range candidate.Fields {
		if known[field] {
			continue
		}
		out = append(out, UnpublishedField{
			Grain:      grainType,
			Package:    candidate.Package,
			Type:       candidate.Name,
			Field:      field,
			SDKType:    strings.Join(paired, ", "),
			Endpoints:  len(operations),
			Operations: operations,
		})
	}
	return out
}

// shortPackage reduces the repository-relative package path the published types
// carry to the domain name the pairings are keyed on.
func shortPackage(pkg string) string {
	return strings.TrimPrefix(pkg, toolsDir+"/")
}
