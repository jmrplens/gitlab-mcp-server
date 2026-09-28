package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// keyTree reduces an answer to the keys it carried and the kinds seen at each,
// keeping the paths named verbatim as their root only. A body that is not
// JSON is recorded as text; a body that says it is JSON and does not decode
// is an error, since the record would otherwise describe something GitLab
// did not send.
func keyTree(contentType string, payload []byte, verbatim []string) ([]orbitrecord.Key, error) {
	if !isJSON(contentType) {
		return []orbitrecord.Key{{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindText}}}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("the answer is declared %s and does not decode: %w", contentType, err)
	}
	walk := treeWalk{kinds: map[string]map[string]bool{}, verbatim: map[string]bool{}}
	for _, path := range verbatim {
		walk.verbatim[path] = true
	}
	if err := walk.visit(value, orbitrecord.Root); err != nil {
		return nil, err
	}
	keys := make([]orbitrecord.Key, 0, len(walk.kinds))
	for path, kinds := range walk.kinds {
		keys = append(keys, orbitrecord.Key{
			Path:     path,
			Kinds:    slices.Sorted(maps.Keys(kinds)),
			Verbatim: walk.verbatim[path],
		})
	}
	orbitrecord.SortKeys(keys)
	return keys, nil
}

// treeWalk accumulates the kinds seen at every path of one answer.
type treeWalk struct {
	kinds    map[string]map[string]bool
	verbatim map[string]bool
}

// visit records one value at path and descends into it, unless the path is
// verbatim. Every element of an array is recorded at the one element path,
// so the kinds there are the union of what the elements were.
func (w treeWalk) visit(value any, path string) error {
	seen := w.kinds[path]
	if seen == nil {
		seen = map[string]bool{}
		w.kinds[path] = seen
	}
	seen[kindOf(value)] = true
	if w.verbatim[path] {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if !orbitrecord.ValidKey(key) {
				return fmt.Errorf("the answer has the key %q under %s, which is not a key name: a subtree keyed by data needs a verbatim path, or its values would be recorded as keys", key, path)
			}
			if err := w.visit(child, orbitrecord.Child(path, key)); err != nil {
				return err
			}
		}
	case []any:
		for _, element := range typed {
			if err := w.visit(element, orbitrecord.ElementOf(path)); err != nil {
				return err
			}
		}
	}
	return nil
}

// kindOf is the JSON kind of a decoded value, numbers decoded as json.Number.
func kindOf(value any) string {
	switch value.(type) {
	case nil:
		return orbitrecord.KindNull
	case bool:
		return orbitrecord.KindBoolean
	case json.Number:
		return orbitrecord.KindNumber
	case string:
		return orbitrecord.KindString
	case []any:
		return orbitrecord.KindArray
	default:
		return orbitrecord.KindObject
	}
}
