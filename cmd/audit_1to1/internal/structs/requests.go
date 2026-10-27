// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/types"
	"strings"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// serviceInterfaceSuffix names a client-go service interface, the only kind
// of interface whose methods send a REST request built from their arguments.
// GraphQLInterface is left out on purpose: the struct its Do takes carries a
// document and its variables, which the GraphQL rules judge, and no field of
// it is a parameter GitLab declares on a route.
const serviceInterfaceSuffix = "ServiceInterface"

// RequestStructs is what client-go's service methods take: the structs a
// method receives as an argument, and the structs those reach through a field.
//
// The second set is what the pairing of an input with an options struct never
// saw. A handler builds a nested struct (a branch permission, an environment
// access level, a custom header) element by element out of a list its input
// carries, so pairing it with the handler's input compares it with the wrong
// struct, and the phantom rule then drops the pair as disjoint. Read off the
// SDK it is known to be nested before any literal is looked at, and paired
// with the struct its values are read from instead.
type RequestStructs struct {
	// Received holds every client-go struct a service method takes as a
	// parameter, pointer or value, the variadic request options excluded.
	Received map[*types.TypeName]bool
	// Nested holds every client-go struct with an exported field that a
	// received struct reaches through a field, a pointer, a slice, an array
	// or a map value, at any depth, and that no method receives itself.
	Nested map[*types.TypeName]bool
}

// requestCache memoizes RequestStructs per client-go package: the universe is
// a property of the SDK, read once however many tool packages ask.
var requestCache sync.Map // *types.Package -> *RequestStructs

// RequestStructsOf returns the request universe of the client-go package
// clientGo, memoized.
func RequestStructsOf(clientGo *types.Package) *RequestStructs {
	if cached, ok := requestCache.Load(clientGo); ok {
		found, _ := cached.(*RequestStructs)
		return found
	}
	found := collectRequestStructs(clientGo)
	requestCache.Store(clientGo, found)
	return found
}

func collectRequestStructs(clientGo *types.Package) *RequestStructs {
	found := &RequestStructs{Received: map[*types.TypeName]bool{}, Nested: map[*types.TypeName]bool{}}
	scope := clientGo.Scope()
	for _, name := range scope.Names() {
		obj, isType := scope.Lookup(name).(*types.TypeName)
		if !isType || !strings.HasSuffix(name, serviceInterfaceSuffix) {
			continue
		}
		iface, isIface := obj.Type().Underlying().(*types.Interface)
		if !isIface {
			continue
		}
		for method := range iface.Methods() {
			for _, received := range ReceivedStructs(method.Signature()) {
				found.Received[received.Obj()] = true
			}
		}
	}
	for received := range found.Received {
		named, _ := received.Type().(*types.Named)
		walkNested(named, func(nested *types.Named) bool {
			obj := nested.Obj()
			if found.Received[obj] || found.Nested[obj] {
				return false
			}
			found.Nested[obj] = true
			return true
		})
	}
	return found
}

// NestedStructs returns the client-go structs named reaches through a field,
// at any depth, each once in the order a walk of the fields meets it: what a
// request built from named carries besides its own fields.
func NestedStructs(named *types.Named) []*types.Named {
	var out []*types.Named
	seen := map[*types.TypeName]bool{named.Obj(): true}
	walkNested(named, func(nested *types.Named) bool {
		if seen[nested.Obj()] {
			return false
		}
		seen[nested.Obj()] = true
		out = append(out, nested)
		return true
	})
	return out
}

// ReceivedStructs returns the client-go structs a method signature takes, in
// parameter order: every parameter whose type, or the type it points to, is a
// named client-go struct. The variadic request options are a slice, which is
// no struct, so they are never among them.
func ReceivedStructs(sig *types.Signature) []*types.Named {
	var out []*types.Named
	for param := range sig.Params().Variables() {
		if named, _, ok := clientGoNamedStruct(param.Type()); ok {
			out = append(out, named)
		}
	}
	return out
}

// walkNested hands visit every client-go struct of parameters that named
// reaches through a field, a pointer, a slice, an array or a map value,
// descending into an embedded struct as part of the struct that embeds it and
// into a nested struct as a struct of its own when visit returns true. The
// depth is bounded the way the field flattening bounds it, so a struct that
// reaches itself ends.
//
// A struct of parameters is one with an exported field that carries no
// io.Reader. The fields of a value type such as ISOTime are all unexported,
// and a struct with a reader is the content of an upload: client-go attaches
// GroupAvatar's Image as the file part of a multipart request, through
// withUpload, and writes no field of it as a parameter.
func walkNested(named *types.Named, visit func(*types.Named) bool) {
	st, _ := named.Underlying().(*types.Struct)
	walkNestedFields(st, visit, 0)
}

func walkNestedFields(st *types.Struct, visit func(*types.Named) bool, depth int) {
	if depth > maxEmbedDepth {
		return
	}
	for field := range st.Fields() {
		named, nested, ok := clientGoNamedStruct(elementType(field.Type()))
		if !ok {
			continue
		}
		if field.Embedded() {
			walkNestedFields(nested, visit, depth+1)
			continue
		}
		if hasExportedField(nested) && !carriesReader(nested) && visit(named) {
			walkNestedFields(nested, visit, depth+1)
		}
	}
}

// carriesReader reports whether a struct has a field of type io.Reader.
func carriesReader(st *types.Struct) bool {
	for field := range st.Fields() {
		if types.TypeString(field.Type(), nil) == "io.Reader" {
			return true
		}
	}
	return false
}

// elementType unwraps the pointers, slices, arrays and map values around a
// field's type to the type a value of it is built from.
func elementType(t types.Type) types.Type {
	for {
		switch wrapped := t.(type) {
		case *types.Pointer:
			t = wrapped.Elem()
		case *types.Slice:
			t = wrapped.Elem()
		case *types.Array:
			t = wrapped.Elem()
		case *types.Map:
			t = wrapped.Elem()
		default:
			return t
		}
	}
}

// hasExportedField reports whether a struct has a field an encoder writes,
// which is what tells a struct of parameters from a value type such as
// ISOTime, whose fields are all time.Time's unexported ones.
func hasExportedField(st *types.Struct) bool {
	for field := range st.Fields() {
		if field.Exported() {
			return true
		}
	}
	return false
}

// requestLiteral reports whether a client-go struct a composite literal
// builds is a request a handler pairs with its input: an Options struct, or a
// struct a service method receives under another name (a custom attribute,
// an Orbit query). A nested struct is not one, since the struct its values
// are read from is its input rather than the handler's.
func requestLiteral(named *types.Named) bool {
	clientGo := named.Obj().Pkg()
	requests := RequestStructsOf(clientGo)
	if requests.Nested[named.Obj()] {
		return false
	}
	return strings.HasSuffix(named.Obj().Name(), optionsSuffix) || requests.Received[named.Obj()]
}

// builtRequests returns the request structs a composite literal of type t
// builds: the client-go request struct t is, or, for a struct of our own,
// each client-go request struct it embeds. A struct that embeds one beside
// the fields client-go does not model (groupboards.updateGroupBoardBody) is
// that request with more parameters, and the request goes out with the
// embedded struct's fields flattened in; a client-go struct it holds in a
// field of its own is a value of it, not the request, and an embedded result
// (a captured commit) is no request at all.
func builtRequests(t types.Type) []*types.Named {
	if named, _, isSDK := clientGoNamedStruct(t); isSDK {
		if requestLiteral(named) {
			return []*types.Named{named}
		}
		return nil
	}
	_, st, isStruct := derefNamedStruct(t)
	if !isStruct {
		return nil
	}
	var out []*types.Named
	for field := range st.Fields() {
		if named, _, isSDK := clientGoNamedStruct(field.Type()); isSDK && field.Embedded() && requestLiteral(named) {
			out = append(out, named)
		}
	}
	return out
}

// IsRequestStruct reports whether a client-go struct is one a service method
// takes, or one such a struct reaches through a field.
func IsRequestStruct(named *types.Named) bool {
	requests := RequestStructsOf(named.Obj().Pkg())
	return requests.Received[named.Obj()] || requests.Nested[named.Obj()]
}

// isNestedRequest reports whether a client-go struct is one a received
// struct reaches through a field and no method receives itself.
func isNestedRequest(named *types.Named) bool {
	return RequestStructsOf(named.Obj().Pkg()).Nested[named.Obj()]
}

// isToolStruct reports whether a named struct is declared in a tool package,
// which is where every input a model sends is declared.
func isToolStruct(named *types.Named) bool {
	pkg := named.Obj().Pkg()
	return pkg != nil && strings.Contains(pkg.Path(), shared.ToolsPkgInfix)
}
