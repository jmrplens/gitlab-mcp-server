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
	found := Pairings{ClientGoDir: clientGoDir(pkgs)}
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
	}
	return found, nil
}

// clientGoDir finds the directory of the client-go root package in the import
// graph of the loaded tool packages.
//
// The root is identified by the Client struct rather than by its import path,
// because the path carries a major-version suffix that moves with every major
// release and several client-go packages share the prefix. A package with no
// files on disk is passed over: the answer is a directory to parse, and one
// that names nothing is worse than none.
func clientGoDir(pkgs []*packages.Package) string {
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
