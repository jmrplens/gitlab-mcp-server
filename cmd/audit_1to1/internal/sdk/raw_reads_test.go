// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// toldTool is a tool package whose functions build two raw requests each and
// decode one answer into a struct that reads archived beside client-go's
// widget, through the request the first of them builds, through a variable
// a loop assigns the request another variable held the round before, and
// through a field the first request is kept in and a method that reads it
// back.
const toldTool = `package told

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

type widgetAPI struct {
	gl.Widget
	Archived bool ~json:"archived"~
}

type pending struct {
	req *gl.Request
}

func (p *pending) request() *gl.Request { return p.req }

func First(c *gl.Client) error {
	get, err := c.NewRequest("GET", "widgets/1", nil, nil)
	if err != nil {
		return err
	}
	var archived widgetAPI
	if _, err := c.Do(get, &archived); err != nil {
		return err
	}
	del, err := c.NewRequest("DELETE", "widgets/1", nil, nil)
	if err != nil {
		return err
	}
	_, err = c.Do(del, nil)
	return err
}

func Chained(c *gl.Client) error {
	if _, err := c.NewRequest("POST", "widgets", nil, nil); err != nil {
		return err
	}
	var previous, latest *gl.Request
	for _, path := range []string{"widgets/1", "widgets/2"} {
		previous = latest
		req, err := c.NewRequest("GET", path, nil, nil)
		if err != nil {
			return err
		}
		latest = req
	}
	var archived widgetAPI
	_, err := c.Do(previous, &archived)
	return err
}

func Held(c *gl.Client, held *pending) error {
	req, err := c.NewRequest("GET", "widgets/1", nil, nil)
	if err != nil {
		return err
	}
	held.req = req
	if _, err := c.NewRequest("DELETE", "widgets/2", nil, nil); err != nil {
		return err
	}
	var archived widgetAPI
	_, err = c.Do(held.request(), &archived)
	return err
}
`

// TestWalkToolWorkarounds_TheRequestADoCallSends_IsToldOnlyThroughVariables
// verifies which raw request a call of Do is counted against when its
// function builds two. The first request a function builds is told like any
// other, so its decode is not handed to the second. A variable assigned
// another before that one is assigned a request, as a loop does with the
// request of its previous round, still holds the request, however late in
// the source the assignment it depends on comes. A request kept in a field
// is in no variable the walk follows, so a call of Do that reads it back
// through a method cannot be told and its decode is counted against both:
// storing the request in a field is not read as handing it to a variable,
// and the name of the method Do reads it through is not read as one.
func TestWalkToolWorkarounds_TheRequestADoCallSends_IsToldOnlyThroughVariables(t *testing.T) {
	fixture := newWaitsFixtureWith(t, nil, map[string]string{"internal/tools/told/told.go": strings.ReplaceAll(toldTool, "~", "`")})

	found, _, _ := walkToolWorkarounds(fixture.pkgs, fixture.clientGo)

	bySite := map[string][]workaround{}
	for _, raw := range found {
		if raw.Form == formRawRequest && strings.HasPrefix(raw.Site, "told.") {
			bySite[raw.Site] = append(bySite[raw.Site], raw)
		}
	}
	decode := []waits.Superset{{Type: "told.widgetAPI", Root: "Widget", Keys: []string{"archived"}, Needs: []string{"LacksKey{Widget, archived}"}}}
	request := func(site, verb string, decodes []waits.Superset) workaround {
		return workaround{Form: formRawRequest, Site: site, Constructor: "NewRequest", Verb: verb, Decodes: decodes}
	}
	for _, tc := range []struct {
		name, site string
		want       []workaround
	}{
		{
			name: "the first request decodes", site: "told.First",
			want: []workaround{request("told.First", "GET", decode), request("told.First", "DELETE", nil)},
		},
		{
			name: "an alias written before the one it reads", site: "told.Chained",
			want: []workaround{request("told.Chained", "POST", nil), request("told.Chained", "GET", decode)},
		},
		{
			name: "a request read back from a field", site: "told.Held",
			want: []workaround{request("told.Held", "GET", decode), request("told.Held", "DELETE", decode)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bySite[tc.site]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("raw requests of %s =\n%+v\nwant\n%+v", tc.site, got, tc.want)
			}
		})
	}
}
