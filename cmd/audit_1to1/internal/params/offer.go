// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
)

// offerer says whether an input of the actions reaching one route can place
// a param GitLab declares there.
type offerer struct {
	schemas map[string]map[string]any
	aliases map[string]map[string]string
	actions []string
}

// offers reports whether any of the actions' inputs offers param.
//
// A param is found among an input's fields by name, since the inputs here
// spell a param the way GitLab does, or under the field an alias the
// requiredness scope declares names for it. A param with brackets is found
// under the one name an input flattens it to (`not[author_id]` as
// `not_author_id`, which is how every negated filter here is spelled) or
// through the nested schema of the field it sits under (`custom_headers[key]`
// as the key property of the items of `custom_headers`). The placement
// rules of the requiredness scope pass over a bracketed name, which is why
// this one reads it.
func (o offerer) offers(param string) bool {
	for _, action := range o.actions {
		properties := schemaProperties(o.schemas[action])
		name := param
		if alias, aliased := o.aliases[action][param]; aliased {
			name = alias
		}
		if _, offered := properties[name]; offered {
			return true
		}
		if !strings.Contains(param, "[") {
			continue
		}
		if _, flattened := properties[shared.NormalizeSDKTag(param)]; flattened || nestedOffers(properties, param) {
			return true
		}
	}
	return false
}

// nestedOffers reports whether the schema properties hold the bracketed
// param through the field each segment names, a list's items read through.
func nestedOffers(properties map[string]any, param string) bool {
	segments := strings.Split(strings.NewReplacer("[]", "", "]", "").Replace(param), "[")
	last := len(segments) - 1
	for _, segment := range segments[:last] {
		node, declared := properties[segment].(map[string]any)
		if !declared {
			return false
		}
		properties = schemaProperties(itemsOf(node))
	}
	_, declared := properties[segments[last]]
	return declared
}

// itemsOf returns the schema a list's elements are held to, or the node
// itself for anything that is not a list.
func itemsOf(node map[string]any) map[string]any {
	if items, isList := node["items"].(map[string]any); isList {
		return items
	}
	return node
}

// schemaProperties returns the properties of a schema node.
func schemaProperties(schema map[string]any) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	return properties
}
