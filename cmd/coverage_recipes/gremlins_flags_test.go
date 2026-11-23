// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestReadGremlinsFlags_EachSpelling_IsReadAsPflagReadsIt verifies what the
// baseline is told by words the recorded runs do not cover: a lone dash and a
// word that is no flag, which pflag passes over, a switch written with a
// value, a value that opens with an equals sign alone, and every value-taking
// long flag taking the next word, so a tag behind one is still read.
func TestReadGremlinsFlags_EachSpelling_IsReadAsPflagReadsIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		flags string
		want  gremlinsFlags
	}{
		{"- --tags e2e", gremlinsFlags{tags: "e2e"}},
		{"word -t e2e", gremlinsFlags{tags: "e2e"}},
		{"xtra", gremlinsFlags{}},
		{"-t=", gremlinsFlags{tags: "="}},
		{"-d=x -t e2e", gremlinsFlags{tags: "e2e"}},
		{"-d= -t e2e", gremlinsFlags{}},
		{"-i=1 -E= x", gremlinsFlags{integration: true, excluded: true}},
		{"--coverpkg=./a=b", gremlinsFlags{coverpkg: "./a=b"}},
		{"--silent --tags=", gremlinsFlags{}},
		{"--integration=T", gremlinsFlags{integration: true}},
		{"--integration=yes", gremlinsFlags{}},
	}
	for _, long := range valueFlags {
		if long != "tags" && long != "coverpkg" && long != "exclude-files" && long != "timeout-coefficient" {
			cases = append(cases, struct {
				flags string
				want  gremlinsFlags
			}{"--" + long + " --tags=no --tags e2e", gremlinsFlags{tags: "e2e"}})
		}
	}
	for _, tc := range cases {
		t.Run(tc.flags, func(t *testing.T) {
			t.Parallel()
			got, err := readGremlinsFlags([]string{"GREMLINS_FLAGS=" + tc.flags})
			if tc.flags == "-d= -t e2e" {
				// `=` with nothing after it is not a value, so the next
				// letter read is `=`, which is no shorthand.
				if err == nil || !strings.Contains(err.Error(), "cannot read -d= the way gremlins would") {
					t.Fatalf("readGremlinsFlags(%q) = %+v, %v; want it refused", tc.flags, got, err)
				}
				return
			}
			tc.want.words = strings.Fields(tc.flags)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("readGremlinsFlags(%q) = %+v, %v; want %+v", tc.flags, got, err, tc.want)
			}
		})
	}
}
