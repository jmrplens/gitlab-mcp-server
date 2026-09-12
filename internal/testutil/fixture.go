package testutil

import (
	"reflect"
	"strconv"
	"strings"
	"time"
)

// FixtureState is the pagination a filled fixture carries, which is the one
// part of a list output the rendering rules read differently: a multi-page
// response must name the total, a single page names what it holds, and a
// keyset page knows no total at all.
type FixtureState int

const (
	// FixtureZero is the zero value of the type, every field absent.
	FixtureZero FixtureState = iota
	// FixtureMultiPage is a populated value on page 1 of 3, 45 items in all.
	FixtureMultiPage
	// FixtureSinglePage is a populated value on the only page.
	FixtureSinglePage
	// FixtureKeyset is a populated value with no total: a keyset page with a
	// next page, which is what GitLab sends past its counting cutoff.
	FixtureKeyset
)

// String names the state for a subtest.
func (s FixtureState) String() string {
	switch s {
	case FixtureZero:
		return "zero"
	case FixtureMultiPage:
		return "multi-page"
	case FixtureSinglePage:
		return "single-page"
	case FixtureKeyset:
		return "keyset"
	default:
		return "fixture-" + strconv.Itoa(int(s))
	}
}

// FixtureSentinelTime is the instant every time-shaped field carries, chosen
// so its rendered forms are unmistakable: a display helper writes
// "3 Feb 2001 04:05 UTC" and a raw print writes the RFC 3339 text or Go's
// default form, either of which a scan can look for.
var FixtureSentinelTime = time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)

// FixtureSentinelRFC3339 is FixtureSentinelTime as GitLab sends it.
const FixtureSentinelRFC3339 = "2001-02-03T04:05:06Z"

// FixtureURLBase is the origin every URL-shaped field points at, so a scan
// can tell a fixture link from any other destination.
const FixtureURLBase = "https://gitlab.example"

// FixtureOptions configures FillFixture.
type FixtureOptions struct {
	State FixtureState
	// Text, when set, supplies every string value from the field's path
	// instead of the sentinel, which is how a hostile render is built.
	Text func(path string) string
}

// FillFixture builds a value of type t the way a GitLab response would fill
// it, with values a scan can recognize: a string gets a sentinel of letters
// and digits made from its field name, a URL-shaped field an address under
// FixtureURLBase, a time-shaped string the RFC 3339 sentinel, a number 7, a
// flag true, a time.Time the sentinel instant, a slice two elements, a map one
// entry, a pointer an allocated value, and the shared pagination shapes the
// state asked for. An interface stays nil, nesting stops at depth 5, and the
// zero state returns the zero value.
func FillFixture(t reflect.Type, opts FixtureOptions) reflect.Value {
	v := reflect.New(t).Elem()
	if opts.State == FixtureZero {
		return v
	}
	f := &filler{opts: opts}
	f.fill(v, "", 0)
	return v
}

// maxFixtureDepth bounds the nesting a fixture is filled to, so a type that
// contains itself terminates.
const maxFixtureDepth = 5

// filler is one fill, carrying the options.
type filler struct {
	opts FixtureOptions
}

// fill sets v to a populated value, path being the field path the sentinels
// are spelled from.
func (f *filler) fill(v reflect.Value, path string, depth int) {
	if depth > maxFixtureDepth || !v.CanSet() {
		return
	}
	t := v.Type()
	if t == reflect.TypeFor[time.Time]() {
		v.Set(reflect.ValueOf(FixtureSentinelTime))
		return
	}
	if t.Kind() == reflect.Struct && t.ConvertibleTo(reflect.TypeFor[time.Time]()) {
		v.Set(reflect.ValueOf(FixtureSentinelTime).Convert(t))
		return
	}
	switch t.Kind() {
	case reflect.String:
		v.SetString(f.text(path))
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(7)
	case reflect.Pointer:
		elem := reflect.New(t.Elem())
		f.fill(elem.Elem(), path, depth+1)
		v.Set(elem)
	case reflect.Slice:
		slice := reflect.MakeSlice(t, 2, 2)
		for i := range 2 {
			f.fill(slice.Index(i), path+strconv.Itoa(i), depth+1)
		}
		v.Set(slice)
	case reflect.Array:
		for i := range v.Len() {
			f.fill(v.Index(i), path+strconv.Itoa(i), depth+1)
		}
	case reflect.Map:
		m := reflect.MakeMap(t)
		key := reflect.New(t.Key()).Elem()
		f.fill(key, path+"Key", depth+1)
		value := reflect.New(t.Elem()).Elem()
		f.fill(value, path, depth+1)
		m.SetMapIndex(key, value)
		v.Set(m)
	case reflect.Struct:
		f.fillStruct(v, path, depth)
	default:
		// An interface, a channel or a function stays at its zero value: a
		// formatter has no static shape to render one by.
	}
}

// fillStruct fills the exported fields of a struct, with the shared shapes
// the renderers read: the pagination outputs by state, and the hints holder
// left empty, since the dispatcher fills it after the render.
func (f *filler) fillStruct(v reflect.Value, path string, depth int) {
	t := v.Type()
	switch {
	case isPaginationShape(t):
		f.fillPagination(v)
		return
	case isCursorPaginationShape(t):
		setBool(v, "HasNextPage", true)
		setString(v, "EndCursor", "cursor7")
		return
	case isHintsShape(t):
		return
	}
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		f.fill(v.Field(i), fieldPath(path, field), depth+1)
	}
}

// fillPagination sets the offset pagination the state asks for.
func (f *filler) fillPagination(v reflect.Value) {
	switch f.opts.State {
	case FixtureMultiPage:
		setInt(v, "Page", 1)
		setInt(v, "PerPage", 20)
		setInt(v, "TotalItems", 45)
		setInt(v, "TotalPages", 3)
		setInt(v, "NextPage", 2)
		setBool(v, "HasMore", true)
	case FixtureSinglePage:
		setInt(v, "Page", 1)
		setInt(v, "PerPage", 20)
		setInt(v, "TotalItems", 2)
		setInt(v, "TotalPages", 1)
	case FixtureKeyset:
		setInt(v, "Page", 1)
		setInt(v, "PerPage", 20)
		setInt(v, "NextPage", 2)
		setBool(v, "HasMore", true)
	case FixtureZero:
	}
}

// isPaginationShape, isCursorPaginationShape and isHintsShape recognize the
// three shared shapes the renderers read by their name and the fields they
// are known by, so the shared package is not imported here: it imports this
// one from its tests, and a type is judged by what it is rather than by
// where it was declared.
func isPaginationShape(t reflect.Type) bool {
	return isShape(t, "PaginationOutput", "Page", "TotalItems", "TotalPages", "NextPage")
}

func isCursorPaginationShape(t reflect.Type) bool {
	return isShape(t, "GraphQLPaginationOutput", "HasNextPage", "EndCursor") ||
		isShape(t, "GraphQLForwardPaginationOutput", "HasNextPage", "EndCursor")
}

func isHintsShape(t reflect.Type) bool {
	return isShape(t, "HintableOutput", "NextSteps")
}

// isShape reports whether t is a struct with the given name carrying every
// field named.
func isShape(t reflect.Type, name string, fields ...string) bool {
	if t.Kind() != reflect.Struct || t.Name() != name {
		return false
	}
	for _, field := range fields {
		if _, ok := t.FieldByName(field); !ok {
			return false
		}
	}
	return true
}

// fieldPath extends a path by one field, an embedded struct adding nothing.
func fieldPath(path string, field reflect.StructField) string {
	if field.Anonymous {
		return path
	}
	return path + "." + field.Name
}

// text supplies the value of one string field.
func (f *filler) text(path string) string {
	if f.opts.Text != nil {
		return f.opts.Text(path)
	}
	return FixtureText(path)
}

// FixtureText is the sentinel a string field carries: an address for a
// URL-shaped field, the RFC 3339 sentinel for a time-shaped one, and for the
// rest the field's own name and a digit, letters and digits only, so the
// value holds no Markdown and can be found in the render.
func FixtureText(path string) string {
	name := lastSegment(path)
	switch {
	case FixtureURLShaped(name):
		return FixtureURLBase + "/" + name + "/7"
	case timeShaped(name):
		return FixtureSentinelRFC3339
	}
	return sanitize(name) + "7"
}

// FixtureURLShaped reports whether a field name says it holds an address,
// which is what decides that the field gets a fixture URL, and what keeps a
// hostile render from putting its payload where only an address can go.
func FixtureURLShaped(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "url") || strings.Contains(lower, "href")
}

// timeShaped reports whether a field name is one GitLab uses for an instant
// or a date.
func timeShaped(name string) bool {
	trimmed := strings.TrimRight(name, "0123456789")
	for _, suffix := range []string{"At", "Date", "On", "Until", "Time", "Timestamp"} {
		if strings.HasSuffix(trimmed, suffix) && len(trimmed) > len(suffix) {
			return true
		}
	}
	return false
}

// lastSegment returns the last field name of a path, the leading dot and any
// slice index dropped, or "Value" for a path with no field at all.
func lastSegment(path string) string {
	segment := path
	if i := strings.LastIndex(path, "."); i >= 0 {
		segment = path[i+1:]
	}
	if segment == "" {
		return "Value"
	}
	return segment
}

// sanitize keeps the letters and digits of a name.
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Value"
	}
	return b.String()
}

// setInt, setBool and setString set a named field when the struct has it.
func setInt(v reflect.Value, name string, n int64) {
	if field := v.FieldByName(name); field.IsValid() && field.CanSet() && field.Kind() == reflect.Int64 {
		field.SetInt(n)
	}
}

func setBool(v reflect.Value, name string, b bool) {
	if field := v.FieldByName(name); field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
		field.SetBool(b)
	}
}

func setString(v reflect.Value, name, s string) {
	if field := v.FieldByName(name); field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
		field.SetString(s)
	}
}

// FixtureField is one leaf field a fixture can leave absent: the path of
// struct field indexes from the root, and its name for a report.
type FixtureField struct {
	Path []int
	Name string
}

// OptionalFields lists the fields of t a GitLab response can omit, by the
// struct's own declaration: a pointer, a slice, a map, or a json tag with
// omitempty, walked through embedded and nested structs and through pointers
// to them. It is the oracle of the absent-value rule: a field the type says
// may be empty must render as nothing rather than as a glyph.
func OptionalFields(t reflect.Type) []FixtureField {
	var fields []FixtureField
	collectOptional(t, nil, "", 0, &fields)
	return fields
}

// collectOptional walks one struct type.
func collectOptional(t reflect.Type, path []int, name string, depth int, out *[]FixtureField) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || depth > maxFixtureDepth {
		return
	}
	if isPaginationShape(t) || isCursorPaginationShape(t) || isHintsShape(t) {
		return
	}
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldName := name + "." + field.Name
		if field.Anonymous {
			fieldName = name
		}
		next := append(append([]int{}, path...), i)
		if isOptional(field) {
			*out = append(*out, FixtureField{Path: next, Name: strings.TrimPrefix(fieldName, ".")})
		}
		inner := field.Type
		for inner.Kind() == reflect.Pointer {
			inner = inner.Elem()
		}
		if inner.Kind() == reflect.Struct && inner != reflect.TypeFor[time.Time]() {
			collectOptional(inner, next, fieldName, depth+1, out)
		}
	}
}

// isOptional reports whether a field's declaration says GitLab may omit it.
func isOptional(field reflect.StructField) bool {
	switch field.Type.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return true
	}
	tag := field.Tag.Get("json")
	_, options, _ := strings.Cut(tag, ",")
	for opt := range strings.SplitSeq(options, ",") {
		if opt == "omitempty" {
			return true
		}
	}
	return false
}

// ZeroField returns v with the field at path set to its zero value, walking
// through pointers, or false when the path crosses a nil pointer the fixture
// left unallocated.
func ZeroField(v reflect.Value, path []int) bool {
	for i, index := range path {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return false
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct || index >= v.NumField() {
			return false
		}
		v = v.Field(index)
		if i == len(path)-1 {
			if !v.CanSet() {
				return false
			}
			v.Set(reflect.Zero(v.Type()))
			return true
		}
	}
	return false
}
