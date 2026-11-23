// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// valueFlags are the long flags of gremlins unleash that take a value; every
// other is a switch.
var valueFlags = []string{
	"tags", "coverpkg", "exclude-files", "output-statuses", "diff", "output", "threshold-efficacy",
	"threshold-mcover", "workers", "test-cpu", "timeout-coefficient", "config",
}

// The shorthands gremlins unleash reads: the switches, which include the root
// command's persistent -s (--silent) and cobra's -h, and those that take a
// value, written after `=`, joined to the letter, or as the next word.
const (
	shortSwitches = "dish"
	shortValues   = "StDoE"
)

// wholeNumber is a whole number written without a leading zero, the one
// spelling pflag, which reads an int with base 0, and this recipe agree on.
var wholeNumber = regexp.MustCompile(`^(0|[1-9]\d*)$`)

// gremlinsFlags is GREMLINS_FLAGS, and what it and gremlins' own environment
// bindings tell the baseline: the build tags, a -coverpkg, whether
// --integration widens the run to the whole module, whether the caller states
// an exclusion of their own, and a coefficient they fix.
//
// The baseline has to be a run of the command gremlins times, so it has to
// see what gremlins sees. The tags and -coverpkg come from a flag or from
// gremlins' own environment binding, and a flag wins, so the environment is
// read first and the flags over it in order, the last of a repeated one
// winning as it does in pflag. They are read the way pflag reads them,
// shorthand clusters such as -dte2e included, because the measured failure
// was a tag that reached gremlins and not the baseline. A word this cannot
// read could be hiding a -t, so it is refused rather than guessed at. A tag
// set only in a .gremlins.yaml is out of reach, and the test-file check stops
// such a run only where it would find no test file at all.
//
// --integration is read from the flag alone, the one source this can read
// that gremlins honors. gremlins binds GREMLINS_UNLEASH_INTEGRATION like the
// rest, but takes the value back with a bool type assertion
// (configuration.Get[bool], v0.6.0), and viper hands an environment value
// over as the string it was, so the assertion fails and gremlins runs one
// subtree whatever the variable says. Reading it here would time the whole
// module against a run of one package. A .gremlins.yaml can set it, since a
// YAML bool does pass that assertion, and that reaches gremlins and not this
// baseline, so an integration run is asked for with -i in GREMLINS_FLAGS.
//
// A coefficient the caller fixes is one gremlins reads after the one this
// recipe derives, and so uses instead of it. It has no environment binding to
// read here: a flag beats gremlins' own variable, and the recipe always
// passes one.
type gremlinsFlags struct {
	words       []string
	tags        string
	coverpkg    string
	integration bool
	excluded    bool
	// fixed is the coefficient the caller fixes, and nil when none is.
	fixed *string
}

// readGremlinsFlags reads GREMLINS_FLAGS, split into words, over gremlins' own
// variables. An error is a word this cannot read the way gremlins would, or a
// fixed coefficient whose deadline could not be told.
func readGremlinsFlags(environ []string) (gremlinsFlags, error) {
	given, _ := recipe.Getenv(environ, "GREMLINS_FLAGS")
	read := gremlinsFlags{words: strings.Fields(given)}
	read.tags, _ = recipe.Getenv(environ, "GREMLINS_UNLEASH_TAGS")
	read.coverpkg, _ = recipe.Getenv(environ, "GREMLINS_UNLEASH_COVERPKG")
	exclusion, _ := recipe.Getenv(environ, "GREMLINS_UNLEASH_EXCLUDE_FILES")
	read.excluded = exclusion != ""
	if err := read.readWords(); err != nil {
		return gremlinsFlags{}, err
	}
	err := read.checkFixed()
	return read, err
}

// readWords reads every word gremlins would read as a flag: those before a
// `--`, where pflag stops.
func (f *gremlinsFlags) readWords() error {
	for rest := f.words; len(rest) > 0 && rest[0] != "--"; {
		word := rest[0]
		rest = rest[1:]
		var taken int
		var err error
		if strings.HasPrefix(word, "--") {
			taken, err = f.long(word, rest)
		} else if letters, flagged := strings.CutPrefix(word, "-"); flagged && letters != "" {
			taken, err = f.cluster(word, rest)
		}
		if err != nil {
			return err
		}
		rest = rest[taken:]
	}
	return nil
}

// unreadable is the refusal of a word this cannot read as gremlins does.
func unreadable(word string) error {
	return fmt.Errorf("GREMLINS_FLAGS: cannot read %s the way gremlins would, so the baseline could run under other build tags than gremlins does; refusing to measure", word)
}

// long reads a long flag, its value from the words after it when it takes one
// and carries none, and returns how many of those it took.
//
// gremlins installs a pflag normalize function (cmd/unleash.go,
// setFlagsOnCmd) that reads `_` and `.` in a flag name as `-`.
func (f *gremlinsFlags) long(word string, rest []string) (taken int, err error) {
	name, value, inline := strings.Cut(strings.TrimPrefix(word, "--"), "=")
	name = strings.NewReplacer("_", "-", ".", "-").Replace(name)
	if !inline {
		value = "true"
		if slices.Contains(valueFlags, name) {
			if len(rest) == 0 {
				return 0, unreadable(word)
			}
			value, taken = rest[0], 1
		}
	}
	switch name {
	case "tags":
		f.tags = value
	case "coverpkg":
		f.coverpkg = value
	case "integration":
		f.integration = readsTrue(value)
	case "exclude-files":
		f.excluded = true
	case "timeout-coefficient":
		f.fixed = &value
	}
	return taken, nil
}

// cluster reads a cluster of shorthands, the last one's value from the words
// after it when it takes one and carries none, and returns how many of those
// it took.
func (f *gremlinsFlags) cluster(word string, rest []string) (taken int, err error) {
	for letters := word[1:]; letters != ""; {
		letter := letters[:1]
		letters = letters[1:]
		takesValue := strings.Contains(shortValues, letter)
		if !takesValue && !strings.Contains(shortSwitches, letter) {
			return 0, unreadable(word)
		}
		value := "true"
		if len(letters) > 1 && letters[0] == '=' {
			value, letters = letters[1:], ""
		} else if takesValue {
			value, letters = letters, ""
			if value == "" {
				if len(rest) == 0 {
					return 0, unreadable(word)
				}
				value, taken = rest[0], 1
			}
		}
		switch letter {
		case "t":
			f.tags = value
		case "i":
			f.integration = readsTrue(value)
		case "E":
			f.excluded = true
		}
	}
	return taken, nil
}

// readsTrue reports whether pflag reads value as true.
func readsTrue(value string) bool {
	return slices.Contains([]string{"1", "t", "T", "TRUE", "true", "True"}, value)
}

// checkFixed refuses a fixed coefficient that is not announced as gremlins
// reads it. The deadline gremlins will apply is announced, so a fixed
// coefficient has to be read as gremlins reads it. pflag parses an int with
// base 0, so a leading zero makes it octal, and gremlins reads 0 as its own
// default of 3 (DefaultTimeoutCoefficient, internal/engine/executor.go,
// v0.6.0). A plain whole number without a leading zero is the one spelling
// both agree on, and anything else is refused before the suite runs rather
// than announced wrong.
func (f *gremlinsFlags) checkFixed() error {
	if f.fixed == nil {
		return nil
	}
	if !wholeNumber.MatchString(*f.fixed) {
		return fmt.Errorf("GREMLINS_FLAGS: --timeout-coefficient '%s' is not a whole number written without a leading zero, so the deadline gremlins would apply cannot be told; refusing to measure", *f.fixed)
	}
	if *f.fixed == "0" {
		*f.fixed = "3"
	}
	return nil
}

// integrationNotice says what decided the run where gremlins' own variable
// asks for an integration run, which gremlins never reads, and is empty where
// it does not.
func (f *gremlinsFlags) integrationNotice(environ []string) string {
	asked, _ := recipe.Getenv(environ, "GREMLINS_UNLEASH_INTEGRATION")
	if asked == "" {
		return ""
	}
	if f.integration {
		return "GREMLINS_UNLEASH_INTEGRATION is set and gremlins v0.6.0 never reads it; this is an integration run because GREMLINS_FLAGS asks for one"
	}
	return "GREMLINS_UNLEASH_INTEGRATION is set, and gremlins v0.6.0 never reads it, so this is not an integration run; GREMLINS_FLAGS=-i makes one"
}

// tagArgs are the build tags as a go command takes them.
func (f *gremlinsFlags) tagArgs() []string {
	if f.tags == "" {
		return nil
	}
	return []string{"-tags", f.tags}
}

// tagsShown is how a message names the build tags.
func (f *gremlinsFlags) tagsShown() string {
	if f.tags == "" {
		return "(none)"
	}
	return f.tags
}
