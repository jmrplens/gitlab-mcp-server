// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"go/ast"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// What an integer parameter client-go narrows comes to. Two are findings:
// undeclared and unresolved.
const (
	// narrowingResolved: a declaration names the action that resolves it, and
	// every integer a handler hands the parameter is read from an input field
	// that takes a name as well, so a model can send what GitLab takes. The
	// walk does not look for the read that turns the name into the integer: a
	// handler that took a string and refused every value that is not a number
	// would come here too, and its own tests are what hold it to the read.
	narrowingResolved = "resolved"
	// narrowingUndeclared: a handler calls the method and no declaration
	// answers the parameter.
	narrowingUndeclared = "undeclared"
	// narrowingUnresolved: a declaration answers the parameter, and an integer
	// is read from an input field that offers an integer alone, or the trace
	// stopped, or the method is called only where the walk does not read the
	// call, so what a model can send there is not known to be a name.
	narrowingUnresolved = "unresolved"
	// narrowingNotFromInput: every call hands the parameter an integer read from
	// no input, so nothing narrows what a model sends there.
	narrowingNotFromInput = "not_from_input"
	// narrowingUncalled: no handler calls the method.
	narrowingUncalled = "uncalled"
)

// The four custom attribute actions, each the one whose handler reads a
// name into the ID of a group, a project or a user, which three methods
// take.
const (
	actionCustomAttrDelete = "admin.custom_attr_delete"
	actionCustomAttrGet    = "admin.custom_attr_get"
	actionCustomAttrList   = "admin.custom_attr_list"
	actionCustomAttrSet    = "admin.custom_attr_set"
)

// narrowingDeclaration answers one integer parameter of a client-go method that
// fills a placeholder the record says takes a name or a path.
type narrowingDeclaration struct {
	// Method and Param name the parameter as sdk_narrowed lists it.
	Method, Param string
	// Action is the canonical ID of the action whose handler reads a name into
	// the integer the method takes.
	Action string
}

// narrowingDeclarations answers the parameters client-go narrows: each is an
// int64 where GitLab takes the ID or the path of a group or a project or the ID
// or the username of a user, the handler of the action named reads a name into
// its ID first, and a client-go release that widens the method leaves its
// declaration stale, which is what retires it.
var narrowingDeclarations = []narrowingDeclaration{
	{Method: "CustomAttributes.DeleteCustomGroupAttribute", Param: "group", Action: actionCustomAttrDelete},
	{Method: "CustomAttributes.DeleteCustomProjectAttribute", Param: "project", Action: actionCustomAttrDelete},
	{Method: "CustomAttributes.DeleteCustomUserAttribute", Param: "user", Action: actionCustomAttrDelete},
	{Method: "CustomAttributes.GetCustomGroupAttribute", Param: "group", Action: actionCustomAttrGet},
	{Method: "CustomAttributes.GetCustomProjectAttribute", Param: "project", Action: actionCustomAttrGet},
	{Method: "CustomAttributes.GetCustomUserAttribute", Param: "user", Action: actionCustomAttrGet},
	{Method: "CustomAttributes.ListCustomGroupAttributes", Param: "group", Action: actionCustomAttrList},
	{Method: "CustomAttributes.ListCustomProjectAttributes", Param: "project", Action: actionCustomAttrList},
	{Method: "CustomAttributes.ListCustomUserAttributes", Param: "user", Action: actionCustomAttrList},
	{Method: "CustomAttributes.SetCustomGroupAttribute", Param: "group", Action: actionCustomAttrSet},
	{Method: "CustomAttributes.SetCustomProjectAttribute", Param: "project", Action: actionCustomAttrSet},
	{Method: "CustomAttributes.SetCustomUserAttribute", Param: "user", Action: actionCustomAttrSet},
	{Method: "GroupRepositoryStorageMove.GetStorageMoveForGroup", Param: "group", Action: "storage_move.get_group_for_group"},
	{Method: "GroupRepositoryStorageMove.RetrieveAllStorageMovesForGroup", Param: "group", Action: "storage_move.retrieve_group"},
	{Method: "GroupRepositoryStorageMove.ScheduleStorageMoveForGroup", Param: "group", Action: "storage_move.schedule_group"},
	{Method: "ProjectRepositoryStorageMove.GetStorageMoveForProject", Param: "project", Action: "storage_move.get_project_for_project"},
	{Method: "ProjectRepositoryStorageMove.RetrieveAllStorageMovesForProject", Param: "project", Action: "storage_move.retrieve_project"},
	{Method: "ProjectRepositoryStorageMove.ScheduleStorageMoveForProject", Param: "project", Action: "storage_move.schedule_project"},
}

// narrowingTable is the declarations with what they are held to.
type narrowingTable struct {
	declared []narrowingDeclaration
	// actions maps each catalog action to the struct its input is, spelled the
	// way the trace spells a field's struct ("customattributes.ListInput").
	actions map[string]string
}

// committedNarrowingTable is the table with the catalog compiled into this
// binary.
func committedNarrowingTable() narrowingTable {
	return narrowingTable{declared: narrowingDeclarations, actions: catalogActionInputs()}
}

// catalogActionInputs reads each action's input struct off the catalog.
func catalogActionInputs() map[string]string {
	client, cleanup := auditshared.NewStubGitLabClient(auditshared.StubToken)
	defer cleanup()
	inputs := map[string]string{}
	for _, group := range auditshared.CachedActionSpecs(client) {
		domain := actioncatalog.DomainFromToolName(group.ToolName)
		for _, spec := range group.Actions {
			inputs[domain+"."+spec.Name] = inputLabel(spec.Route.InputType)
		}
	}
	return inputs
}

// inputLabel spells a route's input type the way the trace spells a field's
// struct, empty for a route built without a named one.
func inputLabel(input reflect.Type) string {
	if input == nil || input.Name() == "" {
		return ""
	}
	return shared.ShortPackage(input.PkgPath()) + "." + input.Name()
}

// narrowingKey names one parameter of one client-go method.
func narrowingKey(method, param string) string { return method + " " + param }

// collectNarrowingOrigins follows, in every walked function, each argument a
// call of a method by name hands a parameter narrowed lists, to the input fields
// it is read from. A method held in a variable is not followed: the walk follows
// one only where it has a parameter of type any, so a call made through it leaves
// nothing recorded, which [narrowingStatus] reads as not known.
func collectNarrowingOrigins(funcs []tracedFunc, trace *inputTrace, values methodValues, narrowed []sdkNarrowing) map[string]*origins {
	wanted := map[string]bool{}
	for _, narrowing := range narrowed {
		wanted[narrowingKey(narrowing.Method, narrowing.Param)] = true
	}
	found := map[string]*origins{}
	for _, fn := range funcs {
		ast.Inspect(fn.body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			for _, held := range values.heldBy(fn.info, call) {
				params := held.method.signature.Params()
				for i := range min(len(call.Args), params.Len()) {
					key := narrowingKey(held.method.key, params.At(i).Name())
					if !wanted[key] {
						continue
					}
					if found[key] == nil {
						found[key] = &origins{}
					}
					found[key].add(trace.origins(fn, call.Args[i], held.via))
				}
			}
			return true
		})
	}
	return found
}

// inputHoldsName answers whether the field a traced input names holds a string,
// which is what lets a model send a name there. A field it cannot find answers
// no, which leaves the integer read from it unresolved rather than excused.
func inputHoldsName(pkgs []*packages.Package) func(string) bool {
	byName := map[string]*types.Package{}
	for _, pkg := range pkgs {
		byName[shared.ShortPackage(pkg.PkgPath)] = pkg.Types
	}
	return func(input string) bool {
		short, rest, _ := strings.Cut(input, ".")
		typeName, fieldName, _ := strings.Cut(rest, ".")
		pkg := byName[short]
		declared := pkg.Scope().Lookup(typeName)
		if declared == nil {
			return false
		}
		found, _, _ := types.LookupFieldOrMethod(declared.Type(), true, pkg, fieldName)
		field, isField := found.(*types.Var)
		if !isField {
			return false
		}
		basic, isBasic := field.Type().Underlying().(*types.Basic)
		return isBasic && basic.Info()&types.IsString != 0
	}
}

// narrowingStatus is what one parameter comes to with its declaration, if it has
// one, and what the calls the walk read hand it. A method called only where the
// walk reads no call, an integer read from a field that offers an integer alone
// and a trace that stopped all leave the parameter not known to take a name, which
// a declaration does not answer.
func narrowingStatus(narrowing sdkNarrowing, read *origins, declared bool, holdsName func(string) bool) string {
	if !narrowing.Called {
		return narrowingUncalled
	}
	known := read != nil && len(read.stopped) == 0
	if known && len(read.inputs) == 0 {
		return narrowingNotFromInput
	}
	if !declared {
		return narrowingUndeclared
	}
	if !known || !allHoldNames(read.inputs, holdsName) {
		return narrowingUnresolved
	}
	return narrowingResolved
}

// allHoldNames reports whether every input field holds a name.
func allHoldNames(inputs []string, holdsName func(string) bool) bool {
	for _, input := range inputs {
		if !holdsName(input) {
			return false
		}
	}
	return true
}

// applyNarrowingDeclarations gives each parameter its status and the inputs and
// stops the walk read for it, and returns the stale declarations: one written
// twice, and one whose parameter client-go no longer narrows or no handler calls,
// or whose action the catalog does not hold or whose input hands the method
// nothing. An empty action is one the catalog does not hold.
func applyNarrowingDeclarations(narrowed []sdkNarrowing, read map[string]*origins, table narrowingTable, holdsName func(string) bool) []string {
	var stale []string
	byKey := map[string]narrowingDeclaration{}
	var order []narrowingDeclaration
	for _, declaration := range table.declared {
		key := narrowingKey(declaration.Method, declaration.Param)
		if _, twice := byKey[key]; twice {
			stale = append(stale, "narrowing declaration "+key+" is declared twice")
			continue
		}
		byKey[key] = declaration
		order = append(order, declaration)
	}
	listed := map[string]sdkNarrowing{}
	for i := range narrowed {
		narrowing := &narrowed[i]
		key := narrowingKey(narrowing.Method, narrowing.Param)
		declaration, isDeclared := byKey[key]
		narrowing.Status = narrowingStatus(*narrowing, read[key], isDeclared, holdsName)
		narrowing.Action = declaration.Action
		if found := read[key]; found != nil {
			narrowing.Inputs = slices.Compact(slices.Sorted(slices.Values(found.inputs)))
			narrowing.StoppedAt = strings.Join(slices.Compact(slices.Sorted(slices.Values(found.stopped))), "; ")
		}
		listed[key] = *narrowing
	}
	for _, declaration := range order {
		if why := narrowingDeclarationStale(declaration, listed, table.actions); why != "" {
			stale = append(stale, "narrowing declaration "+narrowingKey(declaration.Method, declaration.Param)+": "+why)
		}
	}
	return stale
}

// narrowingDeclarationStale says why one declaration answers nothing, empty when
// it holds.
func narrowingDeclarationStale(declaration narrowingDeclaration, listed map[string]sdkNarrowing, actions map[string]string) string {
	narrowing, isNarrowed := listed[narrowingKey(declaration.Method, declaration.Param)]
	if !isNarrowed {
		return "client-go no longer narrows it, so the lookup in front of it and this declaration can go"
	}
	if !narrowing.Called {
		return "no handler calls the method"
	}
	input, inCatalog := actions[declaration.Action]
	if !inCatalog {
		return "the catalog holds no action " + strconv.Quote(declaration.Action)
	}
	if !slices.ContainsFunc(narrowing.Inputs, func(field string) bool { return strings.HasPrefix(field, input+".") }) {
		return "the input of " + declaration.Action + " hands the method nothing"
	}
	return ""
}

// isNarrowingFinding reports whether a parameter's status fails the gate.
func isNarrowingFinding(status string) bool {
	return status == narrowingUndeclared || status == narrowingUnresolved
}
