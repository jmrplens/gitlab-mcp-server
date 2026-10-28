// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdkoptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// workhorseFile is the type the record gives a param Workhorse uploads: the
// file itself, whose attributes (`file.etag`, `file.sha256` and the rest)
// Workhorse fills in and no caller sends.
const workhorseFile = "API::Validations::Types::WorkhorseFile"

// routeVerdict is what judging one route found.
type routeVerdict struct {
	judged   int
	covered  int
	findings []Finding
}

// paramJudge judges the params of one route.
type paramJudge struct {
	use      *routeUse
	recorded map[string]bool
	offers   func(param string) bool
	// children maps each judged param to the declared params directly under
	// it, and the empty name to the params at the top level.
	children map[string][]string
	verdict  routeVerdict
}

// judgeRoute judges every param one route declares: covered when a recorded
// request of the route carries it or a request sends it on every call, a
// finding of its class otherwise, a path param and an attribute Workhorse
// fills left out. offers says whether an input of an action reaching the
// route can place the param.
func judgeRoute(use *routeUse, recorded map[string]bool, offers func(param string) bool) routeVerdict {
	judge := paramJudge{use: use, recorded: recorded, offers: offers, children: map[string][]string{}}
	skipped := pathParams(use.route.Path)
	for _, name := range sortedKeys(use.route.Params) {
		if skipped[name] || workhorseAttribute(use.route.Params, name) {
			continue
		}
		parent := declaredParent(use.route.Params, skipped, name)
		judge.children[parent] = append(judge.children[parent], name)
	}
	for _, name := range judge.children[""] {
		findings, _ := judge.node(name)
		judge.verdict.findings = append(judge.verdict.findings, findings...)
	}
	return judge.verdict
}

// node judges one param and the params under it, returning the findings that
// stand for the subtree and whether any param of it is covered.
//
// A param with nothing under it is covered when a recorded request carries
// it, or when a request sends it on every call. The inventory records the
// top-level keys of a body and not what is nested in them, so a recorded
// top-level key covers a param nested under it only where an option struct
// the route's requests carry models that param: what the struct cannot
// write, the recorded body did not carry either. A container is
// covered when it or one of its members is, and its uncovered members are
// findings of their own; a container none of whose members is covered stands
// for them as one finding when every one of them is of its own class, and is
// otherwise dropped for their findings.
func (j *paramJudge) node(name string) ([]Finding, bool) {
	j.verdict.judged++
	own := j.leaf(name)
	if own.Class == "" {
		j.verdict.covered++
	}
	members := j.children[name]
	if len(members) == 0 {
		if own.Class == "" {
			return nil, true
		}
		return []Finding{own}, false
	}
	var findings []Finding
	covered := own.Class == ""
	for _, member := range members {
		memberFindings, memberCovered := j.node(member)
		findings = append(findings, memberFindings...)
		covered = covered || memberCovered
	}
	if covered {
		return findings, true
	}
	for _, finding := range findings {
		if finding.Class != own.Class {
			return findings, false
		}
	}
	for _, finding := range findings {
		own.Children = append(own.Children, slices.Concat([]string{finding.Param}, finding.Children)...)
	}
	slices.Sort(own.Children)
	return []Finding{own}, false
}

// leaf is the verdict on one param taken alone: no class when a recorded
// request carries it, carries the top-level key it sits under and an option
// struct of the route models it, or a request carries it on every call, its
// finding otherwise.
func (j *paramJudge) leaf(name string) Finding {
	canonical := sdkoptions.Canonical(name)
	top, _, _ := strings.Cut(canonical, "[")
	if j.recorded[canonical] || (j.recorded[top] && j.use.sends[canonical]) || j.use.always[canonical] {
		return Finding{}
	}
	param := j.use.route.Params[name]
	finding := Finding{
		Route:       apilive.RouteName(j.use.route),
		Param:       name,
		SDKModels:   j.use.sends[canonical],
		Offered:     j.offers(name),
		Type:        param.Type,
		Required:    param.Required,
		Actions:     sortedKeys(j.use.actions),
		OptionTypes: sortedKeys(j.use.types),
	}
	finding.Class = classOf(finding.SDKModels, finding.Offered, j.use.unread)
	return finding
}

// classOf names the class of a param nothing recorded carries.
func classOf(sdkModels, offered, unread bool) string {
	switch {
	case offered:
		return ClassOfferedUnrecorded
	case sdkModels:
		return ClassNotOffered
	case unread:
		return ClassUnjudged
	}
	return ClassUnmodeled
}

// pathParams names every param a route's path places, in any spelling of
// its optional groups.
func pathParams(path string) map[string]bool {
	names := map[string]bool{}
	for _, variant := range apilive.ExpandOptional(apilive.NormalizePath(path)) {
		for segment := range strings.SplitSeq(variant, "/") {
			for _, prefix := range []string{":", "*"} {
				if name, isParam := strings.CutPrefix(segment, prefix); isParam && name != "" {
					names[name] = true
				}
			}
		}
	}
	return names
}

// workhorseAttribute reports whether name is an attribute Workhorse fills
// for an uploaded file: a dotted name under a param the record types as one.
func workhorseAttribute(params map[string]apilive.Param, name string) bool {
	file, _, dotted := strings.Cut(name, ".")
	return dotted && params[file].Type == workhorseFile
}

// declaredParent names the nearest param the route declares that name sits
// under (`a[b]` for `a[b][c]`, else `a`), and the empty name for a param at
// the top level or under no declared one.
func declaredParent(params map[string]apilive.Param, skipped map[string]bool, name string) string {
	for parent := name; ; {
		open := strings.LastIndex(parent, "[")
		if open == -1 {
			return ""
		}
		parent = strings.TrimSuffix(parent[:open], "[]")
		if _, declared := params[parent]; declared && !skipped[parent] {
			return parent
		}
	}
}
