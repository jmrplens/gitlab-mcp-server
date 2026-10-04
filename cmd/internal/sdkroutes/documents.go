package sdkroutes

import (
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// Documents returns the documents, out of docs, that the method posts to the
// GraphQL endpoint, in the order docs lists them.
//
// docs is what [graphqldocs.SDKDocuments] reads out of the same module, and a
// document belongs to the method when the method, or a function it delegates
// to, names it or writes it: a named document whose name a body names, or
// whose name the initializer of a package variable a body names was built
// from, through as many variables as it took; and an inline document written
// inside one of those bodies or one of those initializers. The work item
// queries are text/template shells parsed into package variables, chained
// from one another, and reach the method only that way.
//
// It returns nothing for a key client-go does not declare or for a method that
// sends no GraphQL, and a template shell is returned like any other document,
// since what one means is the caller's question ([graphqldocs.IsTemplate]).
func (s *SDK) Documents(key string, docs []graphqldocs.Document) []graphqldocs.Document {
	method, ok := s.methods[key]
	if !ok || !method.GraphQL {
		return nil
	}
	entry := s.reading.funcs[method.Service+serviceSuffix+"."+method.Name]
	var bodies []*function
	names := map[string]bool{}
	s.reading.walk(entry, nil, map[string]bool{}, func(fn *function, _ map[string]string) {
		bodies = append(bodies, fn)
		for ident := range fn.idents {
			names[ident] = true
		}
	})
	initializers := s.reading.initializers(names)
	for _, init := range initializers {
		for ref := range init.refs {
			names[ref] = true
		}
	}
	var out []graphqldocs.Document
	for _, doc := range docs {
		if doc.Name != "" && names[doc.Name] || doc.Name == "" && writtenIn(doc, bodies, initializers) {
			out = append(out, doc)
		}
	}
	return out
}

// initializers returns every package variable the names reach, directly or
// through the initializer of another.
func (r *reading) initializers(names map[string]bool) []variable {
	seen := map[string]bool{}
	var queue []string
	for name := range names {
		queue = append(queue, name)
	}
	var out []variable
	for len(queue) > 0 {
		name := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		declared, isVar := r.vars[name]
		if !isVar || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, declared)
		for ref := range declared.refs {
			queue = append(queue, ref)
		}
	}
	return out
}

// writtenIn reports whether an inline document is written inside one of the
// bodies or one of the initializers.
func writtenIn(doc graphqldocs.Document, bodies []*function, initializers []variable) bool {
	for _, fn := range bodies {
		if within(doc, fn.file, fn.start, fn.end) {
			return true
		}
	}
	for _, init := range initializers {
		if within(doc, init.file, init.start, init.end) {
			return true
		}
	}
	return false
}

// within reports whether a document sits between two lines of one file. The
// file is compared by name: a module directory is read once, and the loader
// that found the document and the parse that found the body may spell the
// same directory differently.
func within(doc graphqldocs.Document, file string, start, end int) bool {
	line := doc.Position.Line
	return filepath.Base(doc.Position.Filename) == filepath.Base(file) && line >= start && line <= end
}
