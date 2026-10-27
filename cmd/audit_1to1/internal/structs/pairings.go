// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// OutputPairing names one MCP output struct and the client-go struct a
// converter in the same package reads to fill it.
//
// It exists for the type-grain shape join in the paths scope, which needs to
// know which client-go struct an output type models before it can ask GitLab's
// document what the endpoints answering with that struct actually send. The
// pairing is the same one the field diff runs over, taken from here rather than
// rebuilt, so the two rules cannot disagree about what an output type is.
type OutputPairing struct {
	// Package is the internal/tools domain name, spelled the way
	// [shared.ShortPackage] spells it.
	Package string
	// MCPType is the output struct's Go name.
	MCPType string
	// SDKType is the client-go struct's Go name with no qualifier. [Pair]
	// carries it qualified by the last segment of the module path, which is
	// "v2" and names no package a reader would recognize, so the qualifier is
	// dropped rather than passed on.
	SDKType string
	// SDKFields is what that struct would deserialize, as the json names
	// encoding/json reads, an embed's promoted in, sorted.
	//
	// It is carried so a caller can ask the question neither existing rule
	// asks. The field diff compares our type with this struct, and the shape
	// join compares our type with what GitLab sends; between them nothing ever
	// holds **client-go** against GitLab. That is the comparison whose findings
	// belong upstream rather than here, and it needs exactly this list.
	SDKFields []string
}

// Pairings is one pass over the tool packages, for a caller outside this
// package.
type Pairings struct {
	// ClientGoDir is the directory the client-go source lives in, read out of
	// the import graph the handlers compile against rather than resolved
	// again, so a caller parsing that source parses the SDK this repository
	// was built with and not whichever copy a module cache offers first.
	// Empty when the loaded packages import no client-go root, which is a
	// tree that would fail every other rule too.
	ClientGoDir string
	// Outputs is every (MCP output struct, client-go struct) pairing the field
	// diff runs over, in the order [CollectPairs] returns them.
	Outputs []OutputPairing
	// Projections is every output struct a handler builds from a client-go
	// struct without a converter, package by package in load order and within
	// a package in the order [CollectProjections] returns them. The field diff
	// does not read it; see [ProjectionPairing] for why.
	Projections []ProjectionPairing
}

// CollectOutputPairings loads every package under internal/tools rooted at root
// and returns the output pairings together with the client-go directory they
// name.
//
// The load is the memoized one every other rule uses, so a run that has already
// paid for it pays nothing here.
func CollectOutputPairings(root string) (Pairings, error) {
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		return Pairings{}, err
	}
	found := Pairings{ClientGoDir: ClientGoDir(pkgs)}
	for _, pkg := range pkgs {
		short := shared.ShortPackage(pkg.PkgPath)
		for _, pair := range CollectPairs(pkg) {
			if pair.Kind != "output" {
				continue
			}
			found.Outputs = append(found.Outputs, OutputPairing{
				Package:   short,
				MCPType:   pair.MCPName,
				SDKType:   pair.SDKName[strings.LastIndexByte(pair.SDKName, '.')+1:],
				SDKFields: sdkFieldNames(pair.SDKType),
			})
		}
		found.Projections = append(found.Projections, CollectProjections(pkg)...)
	}
	return found, nil
}

// InputPairDiff is one input pair the field diff runs over, with the SDK
// fields its input lacks, for a rule outside this package that holds a pair
// to its fields.
//
// The option structs section of the sdk scope reads it twice: the pairs say
// which client-go structs an input is compared with at all, and the missing
// fields of a nested pair are the finding that section gates on, where the
// structs scope only lists them. The missing fields come out of the same diff,
// the declared omissions of acceptedMissingInputs already taken out, so the
// two scopes answer a missing field with one table.
type InputPairDiff struct {
	// Package is the internal/tools domain name, spelled the way
	// [shared.ShortPackage] spells it.
	Package string
	// MCPType is the input struct's Go name.
	MCPType string
	// SDKNamed is the client-go struct it is compared with.
	SDKNamed *types.Named
	// Via is how the pair was found, as [Pair.Via] has it.
	Via string
	// Missing lists the SDK tags the input carries no counterpart of and no
	// declaration answers, sorted.
	Missing []string
}

// DiffInputPairs returns the diff of every input pair of pkgs, package by
// package in load order and within a package in the order [CollectPairs]
// returns them.
func DiffInputPairs(pkgs []*packages.Package) []InputPairDiff {
	run := newDiffRun()
	var out []InputPairDiff
	for _, pkg := range pkgs {
		short := shared.ShortPackage(pkg.PkgPath)
		inputPairs, _ := discoverPairs(pkg)
		for _, pair := range sortedPairs(inputPairs) {
			if disjointPhantomInput(pair, inputPairs) {
				continue
			}
			g := run.diffPair(short, "input", pair)
			missing := make([]string, 0, len(g.MissingFields))
			for _, field := range g.MissingFields {
				missing = append(missing, field.Tag)
			}
			out = append(out, InputPairDiff{Package: short, MCPType: pair.mcpName, SDKNamed: pair.sdkNamed, Via: pair.via, Missing: missing})
		}
	}
	return out
}

// ClientGoDir finds the directory of the client-go root package in the import
// graph of the loaded tool packages. The sdk scope reads it too, to ask
// cmd/internal/sdkroutes which routes the pinned SDK sends.
//
// The root is identified by the Client struct rather than by its import path,
// because the path carries a major-version suffix that moves with every major
// release and several client-go packages share the prefix. A package with no
// files on disk is passed over: the answer is a directory to parse, and one
// that names nothing is worse than none.
func ClientGoDir(pkgs []*packages.Package) string {
	for _, pkg := range pkgs {
		for path, imported := range pkg.Imports {
			if !strings.Contains(path, shared.ClientGoPkgPath) || imported.Types == nil {
				continue
			}
			if _, err := shared.ClientStruct(imported.Types); err != nil {
				continue
			}
			if len(imported.GoFiles) == 0 {
				continue
			}
			return filepath.Dir(imported.GoFiles[0])
		}
	}
	return ""
}

// sdkFieldNames is what a client-go struct deserializes, as the json names
// encoding/json reads, sorted.
//
// It goes through the same flattening the field diff uses, so the two rules
// cannot disagree about what a struct carries: an embed's fields are promoted,
// and a struct that tags nothing falls back to its Go field names the way
// encoding/json does.
func sdkFieldNames(st *types.Struct) []string {
	fields := flattenFields(st, []string{tagKeyJSON})
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
