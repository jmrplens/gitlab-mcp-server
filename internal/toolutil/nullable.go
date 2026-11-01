// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// nullable.go holds the input type of a field GitLab clears with null, where
// leaving the key out, sending null and sending a value are three answers.

package toolutil

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// Nullable is an optional input whose key, left out, leaves the stored value
// alone, whose null removes it, and whose value sets it, the zero value of T
// included. It exists for the fields where GitLab's zero is a value of its own
// and null is the only way to remove one: an issue's weight, where 0 is a
// weight (issue 1390), is the case it was written for.
//
// A pointer cannot carry the difference. encoding/json decodes a JSON null into
// a nil pointer, the same nil an absent key leaves, so a caller who sent null
// to remove the weight was read as one who sent nothing, and the update went
// out without it. The field is tagged omitzero, which [Nullable.IsZero] answers
// for an absent key, so the input schema keeps it optional and a marshaled
// input leaves out a key the caller never sent.
type Nullable[T any] struct {
	value T
	state nullableState
}

// nullableState is which of the three answers a [Nullable] holds.
type nullableState uint8

const (
	// nullableAbsent is the key left out, the zero of the type.
	nullableAbsent nullableState = iota
	// nullableNull is the key sent as null.
	nullableNull
	// nullableSet is the key sent with a value.
	nullableSet
)

// NullableOf returns a [Nullable] holding value, as a caller who sent it does.
func NullableOf[T any](value T) Nullable[T] {
	return Nullable[T]{value: value, state: nullableSet}
}

// NullableNull returns a [Nullable] holding null, as a caller who asked for the
// value to be removed does.
func NullableNull[T any]() Nullable[T] {
	return Nullable[T]{state: nullableNull}
}

// IsZero reports whether the key was left out. encoding/json reads it for the
// omitzero option, and the schema reflection reads that option as optional.
func (n Nullable[T]) IsZero() bool {
	return n.state == nullableAbsent
}

// IsNull reports whether the key was sent as null, which is a request to
// remove the value.
func (n Nullable[T]) IsNull() bool {
	return n.state == nullableNull
}

// Ptr returns the value the key was sent with, or nil when it was left out or
// sent as null, which is the shape an option struct of client-go takes.
func (n Nullable[T]) Ptr() *T {
	if n.state != nullableSet {
		return nil
	}
	value := n.value
	return &value
}

// UnmarshalJSON reads null as a removal and anything else as a value of T,
// refusing what T refuses. encoding/json calls it only for a key that is
// present, which is what leaves an absent key at the zero [Nullable].
func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*n = NullableNull[T]()
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*n = NullableOf(value)
	return nil
}

// MarshalJSON writes the value, or null for a removal. An absent key is left
// out by its omitzero option before this is asked, and writes null if a field
// is declared without one.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.state != nullableSet {
		return []byte("null"), nil
	}
	return json.Marshal(n.value)
}

// nullableElem is what [UnmarshalParams] reads to coerce a value a model sent
// as a numeric string to the type the [Nullable] holds, the coercion a pointer
// to that type gets.
func (Nullable[T]) nullableElem() reflect.Type {
	return reflect.TypeFor[T]()
}

// nullableHolder is implemented by every [Nullable] instantiation.
type nullableHolder interface {
	nullableElem() reflect.Type
}

// nullableElemType returns the type a [Nullable] holds, or target itself when
// target is not one.
func nullableElemType(target reflect.Type) reflect.Type {
	if holder, ok := reflect.TypeAssert[nullableHolder](reflect.Zero(target)); ok {
		return holder.nullableElem()
	}
	return target
}
