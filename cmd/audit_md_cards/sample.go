package main

import (
	"reflect"
	"time"
)

// sampleText is what a string field holds in a populated sample. It carries no
// Markdown punctuation on purpose: a pipe in an unescaped value is a finding
// of the escaping audit, and a shape audit that planted one would report its
// own fixture.
const sampleText = "sample"

// sampleDepth caps the recursion. A GitLab output nests a few levels at most,
// and the cap is what keeps a type that refers to itself — a group with a
// parent group, a note with a note — from building forever.
const sampleDepth = 8

// sampleTime is the timestamp a time.Time field holds, so two runs over one
// tree produce the same report.
var sampleTime = time.Date(2026, time.March, 14, 9, 30, 0, 0, time.UTC)

// sampleValue builds a value of type t for the registry to format.
//
// Populated fills every field, which is the sample that exercises the
// conditional lines a formatter writes only when GitLab returned something;
// the zero sample is the one that exercises the branches for when it did not.
func sampleValue(t reflect.Type, populated bool, depth int) reflect.Value {
	if t == nil || depth > sampleDepth {
		return reflect.Value{}
	}
	if t == reflect.TypeFor[time.Time]() {
		if !populated {
			return reflect.Zero(t)
		}
		return reflect.ValueOf(sampleTime)
	}

	switch t.Kind() {
	case reflect.String:
		if !populated {
			return reflect.Zero(t)
		}
		return reflect.ValueOf(sampleText).Convert(t)
	case reflect.Bool:
		return reflect.ValueOf(populated).Convert(t)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if !populated {
			return reflect.Zero(t)
		}
		return reflect.ValueOf(int64(sampleNumber)).Convert(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if !populated {
			return reflect.Zero(t)
		}
		return reflect.ValueOf(uint64(sampleNumber)).Convert(t)
	case reflect.Float32, reflect.Float64:
		if !populated {
			return reflect.Zero(t)
		}
		return reflect.ValueOf(float64(sampleNumber)).Convert(t)
	case reflect.Pointer:
		return samplePointer(t, populated, depth)
	case reflect.Slice:
		return sampleSlice(t, populated, depth)
	case reflect.Map:
		return sampleMap(t, populated, depth)
	case reflect.Struct:
		return sampleStruct(t, populated, depth)
	default:
		// An interface, a channel, a function: nothing a tool output holds
		// that a formatter reads, and nothing this audit can invent.
		return reflect.Zero(t)
	}
}

// sampleNumber is the number a populated sample holds. Seven rather than one,
// so a count and an identifier are told apart in a report.
//
// It is an untyped constant so each numeric branch converts it at compile
// time: converting one signed sample to unsigned at run time is a widening
// gosec cannot prove safe, and proving it is not worth a directive when the
// constant converts cleanly three times instead.
const sampleNumber = 7

func samplePointer(t reflect.Type, populated bool, depth int) reflect.Value {
	if !populated {
		return reflect.Zero(t)
	}
	elem := sampleValue(t.Elem(), populated, depth+1)
	if !elem.IsValid() {
		return reflect.Zero(t)
	}
	pointer := reflect.New(t.Elem())
	pointer.Elem().Set(elem)
	return pointer
}

func sampleSlice(t reflect.Type, populated bool, depth int) reflect.Value {
	if !populated {
		return reflect.Zero(t)
	}
	elem := sampleValue(t.Elem(), populated, depth+1)
	if !elem.IsValid() {
		return reflect.Zero(t)
	}
	// Two elements rather than one: a list formatter that writes a separator
	// between rows writes it only when there is a second row.
	slice := reflect.MakeSlice(t, 2, 2)
	slice.Index(0).Set(elem)
	slice.Index(1).Set(elem)
	return slice
}

func sampleMap(t reflect.Type, populated bool, depth int) reflect.Value {
	if !populated {
		return reflect.Zero(t)
	}
	key := sampleValue(t.Key(), populated, depth+1)
	value := sampleValue(t.Elem(), populated, depth+1)
	if !key.IsValid() || !value.IsValid() {
		return reflect.Zero(t)
	}
	m := reflect.MakeMap(t)
	m.SetMapIndex(key, value)
	return m
}

func sampleStruct(t reflect.Type, populated bool, depth int) reflect.Value {
	value := reflect.New(t).Elem()
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		filled := sampleValue(field.Type, populated, depth+1)
		if !filled.IsValid() {
			continue
		}
		value.Field(i).Set(filled)
	}
	return value
}
