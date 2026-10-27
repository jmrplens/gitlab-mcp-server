// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"regexp"
	"testing"
)

// siteSpelling is how a site is written: a package name, then a function or
// a method name and its receiver.
var siteSpelling = regexp.MustCompile(`^[a-z]+(\.[A-Za-z][A-Za-z0-9]*)+$`)

// TestEntries_Table_IsWrittenAsTheTableRequires verifies the committed table
// has no entry Problems refuses and spells every site and replaced key the
// way the walk and R-ACTION spell them, so an entry cannot answer nothing
// for a spelling mistake alone.
func TestEntries_Table_IsWrittenAsTheTableRequires(t *testing.T) {
	entries := Entries()
	if problems := Problems(entries); len(problems) != 0 {
		t.Errorf("Problems(Entries()) = %q, want none", problems)
	}
	for _, entry := range entries {
		t.Run(entry.Label(), func(t *testing.T) {
			for _, site := range append(append([]string{}, entry.Sites...), entry.Unseen...) {
				if !siteSpelling.MatchString(site) {
					t.Errorf("site %q is not spelled <package>.<function>", site)
				}
			}
			if entry.Reason == "" {
				t.Error("entry carries no reason")
			}
		})
	}
}

// TestCoveredRaw_Table_ReplacesWhatRActionAndTheServiceRuleDeclaredBefore
// pins what moved into the table: the 72 methods acceptedMissingMethods
// declared COVERED_RAW (22 with a reason of their own and the 50 generic
// integration methods) and the three services declaredServices did. A
// change here is a method or a service that stopped or started being
// replaced, which the entry that changed says why.
func TestCoveredRaw_Table_ReplacesWhatRActionAndTheServiceRuleDeclaredBefore(t *testing.T) {
	entries := Entries()
	if got := len(CoveredRawMethods(entries)); got != 72 {
		t.Errorf("the table replaces %d methods, want 72", got)
	}
	services := CoveredRawServices(entries)
	for _, service := range []string{"ApplicationStatistics", "Epics", "GroupEpicBoards"} {
		t.Run(service, func(t *testing.T) {
			if _, ok := services[service]; !ok {
				t.Errorf("the table does not replace the %s service", service)
			}
		})
	}
	if len(services) != 3 {
		t.Errorf("the table replaces %d services, want 3", len(services))
	}
}
