// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// jsonKind is which of JSON's six kinds a [jsonValue] holds.
type jsonKind int

// The six kinds, null first so a zero jsonValue is null.
const (
	jsonNull jsonKind = iota
	jsonBool
	jsonNumber
	jsonString
	jsonArray
	jsonObject
)

// jsonValue is a JSON document that keeps what encoding/json's maps lose:
// the order of each object's members, and each number as it was written. jq
// keeps both, and the manifests this command writes are held to the bytes
// jq wrote them in, so the derivation works on this tree rather than on a
// map.
type jsonValue struct {
	kind    jsonKind
	boolean bool
	// text is a string's value or a number's literal.
	text    string
	items   []*jsonValue
	members []jsonMember
}

// jsonMember is one member of an object, in the place it was read or set.
type jsonMember struct {
	key   string
	value *jsonValue
}

// jsonText returns a JSON string.
func jsonText(s string) *jsonValue { return &jsonValue{kind: jsonString, text: s} }

// jsonList returns a JSON array of the given items.
func jsonList(items ...*jsonValue) *jsonValue { return &jsonValue{kind: jsonArray, items: items} }

// parseJSON reads exactly one JSON document. A duplicated key keeps the
// place of its first occurrence and the value of its last, as jq does.
func parseJSON(data []byte) (*jsonValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, after := decoder.Token(); !errors.Is(after, io.EOF) {
		return nil, errors.New("more than one JSON value")
	}
	return value, nil
}

// decodeValue reads the next value from decoder.
func decodeValue(decoder *json.Decoder) (*jsonValue, error) {
	token, err := decoder.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	switch t := token.(type) {
	case json.Delim:
		if t == '[' {
			return decodeArray(decoder)
		}
		return decodeObject(decoder)
	case bool:
		return &jsonValue{kind: jsonBool, boolean: t}, nil
	case json.Number:
		return &jsonValue{kind: jsonNumber, text: string(t)}, nil
	case string:
		return jsonText(t), nil
	}
	return &jsonValue{}, nil
}

// decodeArray reads the items of an array whose opening bracket was read.
func decodeArray(decoder *json.Decoder) (*jsonValue, error) {
	array := jsonList()
	for decoder.More() {
		item, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		array.items = append(array.items, item)
	}
	_, err := decoder.Token()
	return array, err
}

// decodeObject reads the members of an object whose opening brace was read.
func decodeObject(decoder *json.Decoder) (*jsonValue, error) {
	object := &jsonValue{kind: jsonObject}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, _ := token.(string)
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		object.set(key, value)
	}
	_, err := decoder.Token()
	return object, err
}

// get returns an object's member, or nil when it has none of that name.
func (v *jsonValue) get(key string) *jsonValue {
	for _, member := range v.members {
		if member.key == key {
			return member.value
		}
	}
	return nil
}

// has reports whether an object carries a member of that name.
func (v *jsonValue) has(key string) bool { return v.get(key) != nil }

// set replaces an object's member in its place, or appends it.
func (v *jsonValue) set(key string, value *jsonValue) {
	for index := range v.members {
		if v.members[index].key == key {
			v.members[index].value = value
			return
		}
	}
	v.members = append(v.members, jsonMember{key: key, value: value})
}

// without returns a copy of an object without the member of that name.
func (v *jsonValue) without(key string) *jsonValue {
	copied := &jsonValue{kind: jsonObject}
	for _, member := range v.members {
		if member.key != key {
			copied.members = append(copied.members, member)
		}
	}
	return copied
}

// clone returns a deep copy, so a derivation never writes into what it read.
func (v *jsonValue) clone() *jsonValue {
	copied := *v
	copied.items = nil
	copied.members = nil
	for _, item := range v.items {
		copied.items = append(copied.items, item.clone())
	}
	for _, member := range v.members {
		copied.members = append(copied.members, jsonMember{key: member.key, value: member.value.clone()})
	}
	return &copied
}

// truthy is jq's truth: everything but null and false.
func (v *jsonValue) truthy() bool {
	return v != nil && v.kind != jsonNull && (v.kind != jsonBool || v.boolean)
}

// equal is jq's equality of two values. Two numbers are compared by their
// literals, which the documents this command reads never spell two ways.
func (v *jsonValue) equal(other *jsonValue) bool {
	if v.kind != other.kind || v.boolean != other.boolean || v.text != other.text ||
		len(v.items) != len(other.items) || len(v.members) != len(other.members) {
		return false
	}
	for index, item := range v.items {
		if !item.equal(other.items[index]) {
			return false
		}
	}
	for _, member := range v.members {
		if found := other.get(member.key); found == nil || !member.value.equal(found) {
			return false
		}
	}
	return true
}

// typeName is jq's name for the kind, as its error messages spell it.
func (v *jsonValue) typeName() string {
	return [...]string{"null", "boolean", "number", "string", "array", "object"}[v.kind]
}

// pretty is the document as jq prints it by default: two spaces of indent,
// members in their order, numbers as written, and a final newline.
func (v *jsonValue) pretty() []byte {
	var out bytes.Buffer
	v.write(&out, "\n")
	out.WriteByte('\n')
	return out.Bytes()
}

// compact is the document on one line, as jq's tojson and string
// interpolation print a value that is not a string.
func (v *jsonValue) compact() string {
	var out bytes.Buffer
	v.write(&out, "")
	return out.String()
}

// write prints v, with newline holding the line break and the indent of the
// line v starts on, or empty for one line.
func (v *jsonValue) write(out *bytes.Buffer, newline string) {
	inner, separator := newline, ":"
	if newline != "" {
		inner, separator = newline+"  ", ": "
	}
	switch v.kind {
	case jsonNull:
		out.WriteString("null")
	case jsonBool:
		out.WriteString(strconv.FormatBool(v.boolean))
	case jsonNumber:
		out.WriteString(v.text)
	case jsonString:
		writeJSONString(out, v.text)
	case jsonArray:
		if len(v.items) == 0 {
			out.WriteString("[]")
			return
		}
		out.WriteByte('[')
		for index, item := range v.items {
			if index > 0 {
				out.WriteByte(',')
			}
			out.WriteString(inner)
			item.write(out, inner)
		}
		out.WriteString(newline)
		out.WriteByte(']')
	case jsonObject:
		if len(v.members) == 0 {
			out.WriteString("{}")
			return
		}
		out.WriteByte('{')
		for index, member := range v.members {
			if index > 0 {
				out.WriteByte(',')
			}
			out.WriteString(inner)
			writeJSONString(out, member.key)
			out.WriteString(separator)
			member.value.write(out, inner)
		}
		out.WriteString(newline)
		out.WriteByte('}')
	}
}

// writeJSONString writes s quoted the way jq writes a string: the quote and
// the backslash escaped, the five control characters JSON names by letter
// by that letter, every other control character and DEL as \u00xx, and
// everything else as the UTF-8 it is, the slash and U+2028 included.
func writeJSONString(out *bytes.Buffer, s string) {
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(out, `\u%04x`, r)
				continue
			}
			var encoded [utf8.UTFMax]byte
			out.Write(encoded[:utf8.EncodeRune(encoded[:], r)])
		}
	}
	out.WriteByte('"')
}

// interpolate is a value as jq's string interpolation spells it: a string
// as itself, anything else as its compact JSON.
func (v *jsonValue) interpolate() string {
	if v.kind == jsonString {
		return v.text
	}
	return v.compact()
}

// strings returns the string items of an array, in order.
func (v *jsonValue) strings() []string {
	var out []string
	for _, item := range v.items {
		if item.kind == jsonString {
			out = append(out, item.text)
		}
	}
	return out
}

// joinStrings joins the string items of an array with sep, as jq's join
// does for an array of strings.
func (v *jsonValue) joinStrings(sep string) string {
	return strings.Join(v.strings(), sep)
}
