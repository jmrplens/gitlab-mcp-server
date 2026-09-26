package main

import (
	"strings"
)

// checkValidate is G11: the register's own validators accept it. The leaf's
// tests hold the same, and the gate asks again so that a run of the gate alone
// answers for the whole register: rows that break an invariant with no finding
// recorded for it, and a failure table that charges what the caller did not
// cause.
func (g *gate) checkValidate() []Finding {
	var found []Finding
	found = append(found, violations("tenancy.Validate", g.reg.validate(g.reg.decisions))...)
	found = append(found, violations("tenancy.ValidateFailures", g.reg.validateFailures(g.reg.failures))...)
	return found
}

// violations turns a joined error into one finding per line.
func violations(subject string, err error) []Finding {
	if err == nil {
		return nil
	}
	var found []Finding
	for line := range strings.SplitSeq(err.Error(), "\n") {
		if line != "" {
			found = append(found, Finding{Rule: "G11", Subject: subject, Message: line})
		}
	}
	return found
}
