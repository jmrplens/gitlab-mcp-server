package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkOrphans is G6: every exported constant of the register's value and
// code files is what some Alias or Arg site reads, and something outside the
// register does read it; every exported function of its rule files is named
// in some row's Functions. A register value nothing reads is a policy that
// decides nothing, and it reads as though it did.
//
// A constant of a pending row is deferred: its site has not moved yet, so
// nothing reads it by design until that row's layer lands.
func (g *gate) checkOrphans() []Finding {
	leaf := g.p.byDir[g.reg.leaf]
	if leaf == nil {
		return nil
	}
	deferred := map[string]bool{}
	for _, d := range g.reg.decisions {
		if g.pending[d.ID] {
			for _, name := range rowConstants(d) {
				deferred[name] = true
			}
		}
	}
	var found []Finding
	for _, c := range exportedConsts(g.p, g.leafFiles(g.rules.valueFiles)) {
		if !deferred[c.name] {
			if msg := g.orphanProblem(c.name); msg != "" {
				found = append(found, Finding{Rule: "G6", Subject: siteKey(g.reg.leaf, c.name), Position: c.at, Message: msg})
			}
		}
	}
	named := map[string]bool{}
	for _, d := range g.reg.decisions {
		for _, name := range d.Functions {
			named[name] = true
		}
	}
	for _, fn := range exportedFuncs(g.p, g.leafFiles(g.rules.ruleFiles)) {
		if !named[fn.name] {
			found = append(found, Finding{
				Rule: "G6", Subject: siteKey(g.reg.leaf, fn.name), Position: fn.at,
				Message: "is a register function no row names in Functions",
			})
		}
	}
	return found
}

// rowConstants are the register constants one row owns: its values, and what
// its Alias and Arg sites read.
func rowConstants(d tenancy.Decision) []string {
	names := slices.Clone(d.Values)
	for _, s := range d.Sites {
		if s.Role == tenancy.Alias || s.Role == tenancy.Arg {
			names = append(names, s.Reads)
		}
	}
	return names
}

// orphanProblem says why the register constant name is an orphan, or returns
// empty when it is not.
func (g *gate) orphanProblem(name string) string {
	declared := false
	for _, d := range g.reg.decisions {
		for _, s := range d.Sites {
			declared = declared || ((s.Role == tenancy.Alias || s.Role == tenancy.Arg) && s.Reads == name)
		}
	}
	if !declared {
		return "is a register value no Alias or Arg site reads"
	}
	if !g.readOutsideLeaf(name) {
		return "is a register value nothing outside the register reads"
	}
	return ""
}

// readOutsideLeaf reports whether a package other than the register refers to
// the register object named name. The set is built once per run, on the first
// question, since it takes one pass over every identifier of the program.
func (g *gate) readOutsideLeaf(name string) bool {
	if g.leafUsed == nil {
		g.leafUsed = map[string]bool{}
		leafPath := goprogram.ModulePath + "/" + g.reg.leaf
		for _, pkg := range g.p.packages {
			if pkg.PkgPath == leafPath {
				continue
			}
			for _, obj := range pkg.TypesInfo.Uses {
				if obj.Pkg() != nil && obj.Pkg().Path() == leafPath {
					g.leafUsed[obj.Name()] = true
				}
			}
		}
	}
	return g.leafUsed[name]
}

// leafDecl is an exported declaration of the register, with where it is.
type leafDecl struct {
	name string
	at   string
}

// leafFiles are the register's files with the named base names.
func (g *gate) leafFiles(names []string) []*ast.File {
	var out []*ast.File
	for _, file := range g.p.byDir[g.reg.leaf].Syntax {
		if slices.Contains(names, filepath.Base(g.p.fset.Position(file.Package).Filename)) {
			out = append(out, file)
		}
	}
	return out
}

// exportedConsts lists the exported constants the files declare.
func exportedConsts(p *program, files []*ast.File) []leafDecl {
	var out []leafDecl
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, _ := spec.(*ast.ValueSpec)
				for _, name := range vs.Names {
					if name.IsExported() {
						out = append(out, leafDecl{name: name.Name, at: p.position(name.Pos())})
					}
				}
			}
		}
	}
	return out
}

// exportedFuncs lists the exported functions, not methods, the files
// declare.
func exportedFuncs(p *program, files []*ast.File) []leafDecl {
	var out []leafDecl
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Recv == nil && fn.Name.IsExported() {
				out = append(out, leafDecl{name: fn.Name.Name, at: p.position(fn.Name.Pos())})
			}
		}
	}
	return out
}

// orphanFindingsFor are the G6 findings a pending row would have if it were
// not pending, for the pending check.
func (g *gate) orphanFindingsFor(d tenancy.Decision) []Finding {
	var found []Finding
	for _, name := range rowConstants(d) {
		if g.leafConst(name) == nil {
			continue
		}
		if msg := g.orphanProblem(name); msg != "" {
			found = append(found, Finding{Rule: "G6", Subject: siteKey(g.reg.leaf, name), Message: fmt.Sprintf("%s (%s)", msg, d.ID)})
		}
	}
	return found
}
