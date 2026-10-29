// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// Where a call's answer is recorded from, beside a GraphQL data path. Since
// GitLab.com removed five of the six Orbit REST routes (2026-10-27 and -28),
// three handlers read GitLab's GraphQL API and two its Orbit MCP endpoint, and
// neither answers with the body the handler reads: GraphQL wraps it in data and
// the fields above the one asked for, and MCP in a JSON-RPC envelope. The
// record keeps the part the handler reads, so its keys are held against the
// output type the way a REST body's are, and says where it was taken from.
const (
	// rootMCPResult is the result of a JSON-RPC answer, which is what
	// tools/list answers the tool list in.
	rootMCPResult = "result"
	// rootMCPText is the text a tools/call result carries, its text blocks
	// joined, read as JSON when it is JSON (a command asked for raw) and as
	// text otherwise (llm).
	rootMCPText = "result.content[].text"
	// graphQLData is the prefix of a root inside a GraphQL answer.
	graphQLData = "data."
)

// mediaJSON and mediaText are the media types an extracted answer is recorded
// with: JSON when it decoded as JSON, and text otherwise.
const (
	mediaJSON = "application/json"
	mediaText = "text/plain"
)

// errNoRoot is what [rootedAnswer] answers when the answer has nothing at the
// root the call is recorded from, which a recording cannot describe.
var errNoRoot = errors.New("the answer holds nothing at the root the call is recorded from")

// rootedAnswer returns the part of an answer a call is recorded from, with the
// media type to record it under. An empty root is the whole body as it came.
// A GraphQL root is the value at that path below data, its connections
// recorded as the lists they page (see [elideConnections]). The MCP roots are
// the JSON-RPC result, and the text of a tool result.
func rootedAnswer(root, contentType string, payload []byte) (kind string, rooted []byte, err error) {
	if root == "" {
		return contentType, payload, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var decoded any
	if err = decoder.Decode(&decoded); err != nil {
		return "", nil, fmt.Errorf("the answer does not decode, so nothing can be read at %s: %w", root, err)
	}
	if root == rootMCPText {
		return toolText(decoded)
	}
	value, found := at(decoded, root)
	if !found || value == nil {
		return "", nil, fmt.Errorf("%w (%s)", errNoRoot, root)
	}
	if strings.HasPrefix(root, graphQLData) {
		value = elideConnections(value)
	}
	// The value was decoded from JSON, numbers as json.Number, so encoding it
	// again cannot fail, and this repository's rule for that is to say so at
	// the leaf.
	return mediaJSON, cmdutil.Must(json.Marshal(value)), nil
}

// at walks a dotted path of object keys through a decoded body.
func at(body any, path string) (any, bool) {
	value := body
	for key := range strings.SplitSeq(path, ".") {
		object, isObject := value.(map[string]any)
		if !isObject {
			return nil, false
		}
		child, has := object[key]
		if !has {
			return nil, false
		}
		value = child
	}
	return value, true
}

// toolText reads the text of an MCP tool result: its text blocks joined, as
// JSON when the text decodes as JSON and as text otherwise.
func toolText(body any) (kind string, text []byte, err error) {
	content, found := at(body, "result.content")
	blocks, isList := content.([]any)
	if !found || !isList {
		return "", nil, fmt.Errorf("%w (%s)", errNoRoot, rootMCPText)
	}
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if value, isText := at(block, "text"); isText {
			if typed, isString := value.(string); isString {
				texts = append(texts, typed)
			}
		}
	}
	joined := []byte(strings.Join(texts, "\n"))
	if json.Valid(joined) {
		return mediaJSON, joined, nil
	}
	return mediaText, joined, nil
}

// elideConnections replaces every GraphQL connection in a decoded value with
// the list of its nodes. A connection is an object carrying nodes beside
// pageInfo; the handler pages through it and publishes the nodes as a list, so
// the record holds the list where the connection was and leaves the paging
// out, which is the handler's and not a field of the answer it publishes.
func elideConnections(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if nodes, isConnection := typed["nodes"]; isConnection {
			if _, paged := typed["pageInfo"]; paged {
				return elideConnections(nodes)
			}
		}
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = elideConnections(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, element := range typed {
			out[i] = elideConnections(element)
		}
		return out
	default:
		return value
	}
}
