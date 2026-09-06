package main

import "strings"

// hole is one place a runtime value lands inside a Markdown template: the verb
// that renders it, the argument index it consumes, and where in the template
// text it sits, which is what decides the Markdown context around it.
type hole struct {
	verb byte
	// arg is the index into the call's argument list, after the format
	// string. It is -1 when the verb consumes no argument.
	arg int
	// offset is the byte offset of the verb's '%' within the template.
	offset int
}

// parseVerbs splits a printf template into the holes it declares.
//
// Argument consumption follows the fmt package: flags, an explicit argument
// index, a width and a precision may precede the verb, and a '*' for either
// consumes an argument of its own. Getting this wrong would pair a verb with
// the wrong expression, which is worse than not looking at all, so the walk
// mirrors fmt's own grammar rather than matching a regular expression.
func parseVerbs(template string) []hole {
	var holes []hole
	next := 0
	for i := 0; i < len(template); i++ {
		if template[i] != '%' {
			continue
		}
		start := i
		i++
		if i >= len(template) {
			break
		}
		if template[i] == '%' {
			continue
		}
		i = skipFlags(template, i)
		if idx, after, ok := explicitArgIndex(template, i); ok {
			next = idx
			i = after
		}
		i, next = consumeStar(template, i, next, &holes, start)
		i = skipDigits(template, i)
		if i < len(template) && template[i] == '.' {
			i++
			i, next = consumeStar(template, i, next, &holes, start)
			i = skipDigits(template, i)
		}
		if i >= len(template) {
			break
		}
		holes = append(holes, hole{verb: template[i], arg: next, offset: start})
		next++
	}
	return holes
}

// skipFlags advances past the printf flag characters.
func skipFlags(template string, i int) int {
	for i < len(template) && strings.IndexByte("+-# 0", template[i]) >= 0 {
		i++
	}
	return i
}

// skipDigits advances past a run of decimal digits.
func skipDigits(template string, i int) int {
	for i < len(template) && template[i] >= '0' && template[i] <= '9' {
		i++
	}
	return i
}

// explicitArgIndex reads a "[n]" argument selector, returning the zero-based
// index it names and the offset just past it.
func explicitArgIndex(template string, i int) (index, after int, ok bool) {
	if i >= len(template) || template[i] != '[' {
		return 0, i, false
	}
	j := i + 1
	value := 0
	digits := 0
	for j < len(template) && template[j] >= '0' && template[j] <= '9' {
		value = value*10 + int(template[j]-'0')
		digits++
		j++
	}
	if digits == 0 || j >= len(template) || template[j] != ']' || value < 1 {
		return 0, i, false
	}
	return value - 1, j + 1, true
}

// consumeStar accounts for a '*' width or precision, which takes an argument
// of its own. The consumed argument is recorded as a hole so a caller counting
// arguments stays in step, with a verb of '*' that no context ever judges.
func consumeStar(template string, i, next int, holes *[]hole, start int) (offset, nextArg int) {
	if i >= len(template) || template[i] != '*' {
		return i, next
	}
	*holes = append(*holes, hole{verb: '*', arg: next, offset: start})
	return i + 1, next + 1
}

// stringishVerbs are the verbs that render a value as text a Markdown reader
// sees as text. They are the only ones through which a pipe, a newline or an
// opening angle bracket can reach the page: %d and its numeric siblings can
// emit none of the three whatever they are handed.
//
// %q is here despite quoting its argument. Go's quoting escapes a quote and a
// backslash and nothing else that matters here, so a title holding a pipe
// still ends the cell, and one holding an opening angle bracket still opens a
// tag.
var stringishVerbs = map[byte]bool{'s': true, 'v': true, 'q': true}
