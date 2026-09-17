// What a merge refuses, and why each refusal is a refusal rather than a
// warning.
//
// Merging a run into a row it already published is how a corrected case reaches
// a published figure without re-running the other 257. It is also, done
// carelessly, a way to put two measurements under one heading and let the last
// one to arrive name them both. The row key holds what identifies a
// measurement; these are the two things it does not hold and that a merge would
// otherwise fold together silently.

package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// mergeRefusal says why an incoming row may not be merged into a standing one,
// and is empty when it may.
//
// Both cases are refusals rather than warnings because what they prevent is a
// published figure nobody can tell is wrong. A warning on a generator's stdout
// is read once, by whoever ran it, and the record keeps the answer forever.
func mergeRefusal(standing, incoming row) string {
	if len(standing.Cases) == 0 {
		return fmt.Sprintf("the standing row records no per-case figures, so merging into it would replace %d case(s) "+
			"of measurement with the %d this run carries and no part of the record would say so; re-fold the run that "+
			"published it, or drop the row",
			standing.Counts.Attempts, incoming.Counts.Attempts)
	}
	if differs := provenanceDifference(standing.Provenance, incoming.Provenance); differs != "" {
		return "the two runs disagree about " + differs +
			", which the row key does not hold and the merged figures would carry only one of; " +
			"re-fold the whole run rather than merging part of it"
	}
	return ""
}

// provenanceDifference names the first field two runs disagree about that
// identifies the configuration their figures were produced under, and is empty
// when they agree.
//
// Commit and Date are deliberately not compared. They differ between any two
// runs, which is the whole point of being able to merge one into another, and
// they are recorded per case so a reader can still see which run measured what.
// Everything else here describes the thing being measured rather than the
// moment of measuring: an instance on another GitLab version, a session serving
// a different number of tools, a credential carrying different scopes, or a
// provider asked with a different temperature is a different measurement, and
// the row key holds none of them.
func provenanceDifference(standing, incoming provenance) string {
	for _, one := range []struct {
		field           string
		left, right     string
		leftOK, rightOK bool
	}{
		{field: "the GitLab version", left: standing.GitLabVersion, right: incoming.GitLabVersion},
		{field: "the instance edition", left: standing.Edition, right: incoming.Edition},
		{field: "the capability surface", left: standing.CapabilitySurface, right: incoming.CapabilitySurface},
		{field: "the provider", left: standing.Provider, right: incoming.Provider},
	} {
		if one.left != one.right {
			return fmt.Sprintf("%s (%q and %q)", one.field, one.left, one.right)
		}
	}
	if standing.TierConfirmed != incoming.TierConfirmed {
		return fmt.Sprintf("whether the tier was confirmed (%t and %t)", standing.TierConfirmed, incoming.TierConfirmed)
	}
	if standing.ServedTools != incoming.ServedTools {
		return fmt.Sprintf("how many tools the session served (%d and %d)", standing.ServedTools, incoming.ServedTools)
	}
	if !slices.Equal(standing.TokenScopes, incoming.TokenScopes) {
		return fmt.Sprintf("the credential's scopes (%s and %s)",
			strings.Join(standing.TokenScopes, "+"), strings.Join(incoming.TokenScopes, "+"))
	}
	if !maps.Equal(standing.RequestOptions, incoming.RequestOptions) {
		return fmt.Sprintf("the request options (%s and %s)",
			renderOptions(standing.RequestOptions), renderOptions(incoming.RequestOptions))
	}
	return ""
}

// renderOptions spells a request-options map in a stable order, so a refusal
// reads the same however the maps were built.
func renderOptions(options map[string]string) string {
	if len(options) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(options))
	for name, value := range options {
		parts = append(parts, name+"="+value)
	}
	slices.Sort(parts)
	return strings.Join(parts, ", ")
}
