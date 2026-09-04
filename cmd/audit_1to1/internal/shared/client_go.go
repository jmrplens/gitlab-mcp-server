package shared

import (
	"errors"
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	// requestOptionMarker is the variadic tail every client-go REST endpoint
	// carries, and the only reliable way to tell an endpoint method apart from
	// a helper on the same interface.
	requestOptionMarker = "RequestOptionFunc"

	// serviceInterfaceSuffix is stripped from an interface name to get the bare
	// service key both the method adjudication table and the SDK service
	// universe are written in.
	serviceInterfaceSuffix = "ServiceInterface"

	// clientStructName is the client-go entry point whose fields declare the
	// complete service surface.
	clientStructName = "Client"

	// graphQLInterfaceName is the client-go interface a raw GraphQL call goes
	// through. Its absence means the SDK renamed it and the GraphQL rule would
	// otherwise silently find nothing.
	graphQLInterfaceName = "GraphQLInterface"

	// sdkPrefix opens every structural-lookup failure, so the reader knows the
	// missing declaration is upstream's rather than this repository's.
	sdkPrefix = "client-go "
)

// ServiceUsage accumulates, for one client-go service interface, the set of
// methods called and the internal/tools packages that reference it.
type ServiceUsage struct {
	// Named is the client-go service interface itself.
	Named *types.Named
	// Called holds the method names our handlers invoke on it.
	Called map[string]struct{}
	// Packages holds the short internal/tools package names that reference it.
	Packages map[string]struct{}
}

// Service returns the bare service name for this usage entry.
func (u *ServiceUsage) Service() string { return ServiceName(u.Named) }

// ServiceName returns the bare service name of a client-go service interface:
// the interface name without its ServiceInterface suffix (so
// BranchesServiceInterface is Branches). Interfaces that do not carry the
// suffix, such as GraphQLInterface, keep their name.
func ServiceName(named *types.Named) string {
	return strings.TrimSuffix(named.Obj().Name(), serviceInterfaceSuffix)
}

// CollectServiceUsage walks every call expression in pkgs and records calls
// whose receiver type is a client-go service interface, keyed by interface
// name. It is the call-site universe shared by the action-coverage scope
// (which methods of a used service are uncovered) and the SDK scope (which of
// the SDK's declared services are referenced at all), so the two cannot
// disagree about what "we call this" means.
func CollectServiceUsage(pkgs []*packages.Package) map[string]*ServiceUsage {
	usage := map[string]*ServiceUsage{}
	for _, pkg := range pkgs {
		collectPackageUsage(pkg, usage)
	}
	return usage
}

func collectPackageUsage(pkg *packages.Package, usage map[string]*ServiceUsage) {
	pkgName := ShortPackage(pkg.PkgPath)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			named, ok := ClientGoServiceInterface(pkg.TypesInfo.TypeOf(sel.X))
			if !ok {
				return true
			}
			use := usageFor(usage, named)
			use.Called[sel.Sel.Name] = struct{}{}
			use.Packages[pkgName] = struct{}{}
			return true
		})
	}
}

func usageFor(usage map[string]*ServiceUsage, named *types.Named) *ServiceUsage {
	key := named.Obj().Name()
	use, ok := usage[key]
	if !ok {
		use = &ServiceUsage{Named: named, Called: map[string]struct{}{}, Packages: map[string]struct{}{}}
		usage[key] = use
	}
	return use
}

// ClientGoServiceInterface returns the named interface if t is a client-go
// service interface (e.g. BranchesServiceInterface).
func ClientGoServiceInterface(t types.Type) (*types.Named, bool) {
	if t == nil {
		return nil, false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, false
	}
	if _, isIface := named.Underlying().(*types.Interface); !isIface {
		return nil, false
	}
	return named, IsClientGoObject(named.Obj())
}

// IsClientGoObject reports whether obj is declared in the client-go module.
func IsClientGoObject(obj *types.TypeName) bool {
	return obj != nil && obj.Pkg() != nil && strings.Contains(obj.Pkg().Path(), ClientGoPkgPath)
}

// APIMethodNames returns the exported methods of a client-go service interface
// whose signature ends in a variadic ...RequestOptionFunc (the REST endpoint
// marker).
func APIMethodNames(named *types.Named) []string {
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	var names []string
	for method := range iface.Methods() {
		if !method.Exported() {
			continue
		}
		if sig, isSig := method.Type().(*types.Signature); isSig && SignatureIsAPICall(sig) {
			names = append(names, method.Name())
		}
	}
	sort.Strings(names)
	return names
}

// SignatureIsAPICall reports whether sig is a client-go endpoint call, i.e. a
// variadic signature whose tail is a slice of a client-go RequestOptionFunc.
func SignatureIsAPICall(sig *types.Signature) bool {
	if !sig.Variadic() || sig.Params().Len() == 0 {
		return false
	}
	last := sig.Params().At(sig.Params().Len() - 1)
	slice, ok := last.Type().(*types.Slice)
	if !ok {
		return false
	}
	named, ok := slice.Elem().(*types.Named)
	if !ok {
		return false
	}
	if !IsClientGoObject(named.Obj()) {
		return false
	}
	return strings.Contains(named.Obj().Name(), requestOptionMarker)
}

// ClientGoTypes returns the type information for the client-go root package,
// found through the import graph of the already-loaded tool packages. Reading
// it from that graph rather than loading it again guarantees the SDK surface
// audited is the one the handlers actually compile against.
func ClientGoTypes(pkgs []*packages.Package) (*types.Package, error) {
	for _, pkg := range pkgs {
		for path, imported := range pkg.Imports {
			if !strings.Contains(path, ClientGoPkgPath) || imported.Types == nil {
				continue
			}
			if clientStructType(imported.Types) != nil {
				return imported.Types, nil
			}
		}
	}
	return nil, errors.New(sdkPrefix + "root package not found in the tool packages' import graph")
}

// ClientStruct returns the fields of the client-go Client struct, which declare
// the SDK's complete service surface.
func ClientStruct(clientGo *types.Package) (*types.Struct, error) {
	st := clientStructType(clientGo)
	if st == nil {
		return nil, errors.New(sdkPrefix + clientStructName + " struct not found")
	}
	return st, nil
}

func clientStructType(pkg *types.Package) *types.Struct {
	obj := pkg.Scope().Lookup(clientStructName)
	if obj == nil {
		return nil
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	return st
}

// GraphQLInterface returns the client-go interface every raw GraphQL call goes
// through. It is an error for it to be absent: without it the GraphQL rule
// would report nothing and read as passing.
func GraphQLInterface(clientGo *types.Package) (*types.Named, error) {
	obj := clientGo.Scope().Lookup(graphQLInterfaceName)
	if obj == nil {
		return nil, errors.New(sdkPrefix + graphQLInterfaceName + " not found")
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil, errors.New(sdkPrefix + graphQLInterfaceName + " is not a named type")
	}
	if _, isIface := named.Underlying().(*types.Interface); !isIface {
		return nil, errors.New(sdkPrefix + graphQLInterfaceName + " is not an interface")
	}
	return named, nil
}

// ShortPackage reduces a package path to the internal/tools domain name it
// carries, falling back to the last path element outside that tree.
func ShortPackage(pkgPath string) string {
	_, after, ok := strings.Cut(pkgPath, ToolsPkgInfix)
	if !ok {
		if last := strings.LastIndex(pkgPath, "/"); last >= 0 {
			return pkgPath[last+1:]
		}
		return pkgPath
	}
	return after
}

// SortedSet converts a set to a sorted slice, never nil.
func SortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
