// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRawRequests_TheStandInTree_ListsEachRawRequestWithItsOptions verifies
// what R-PARAM is handed of each raw request: the constructor, the verb and
// the static type of its options, the calls of a function that forwards its
// options with what each hands it, and the struct of the package's own a
// request carries beside client-go's, sorted by site, together with the
// client-go directory the tree compiles against.
func TestRawRequests_TheStandInTree_ListsEachRawRequestWithItsOptions(t *testing.T) {
	fixture := newWaitsFixture(t, nil)

	requests, dir, err := RawRequests(fixture.root)
	if err != nil {
		t.Fatalf("RawRequests: %v", err)
	}
	if want := filepath.Join(fixture.root, "gitlab.com", "gitlab-org", "api", "client-go", "v3"); dir != want {
		t.Errorf("client-go directory = %q, want %q", dir, want)
	}
	if !slices.IsSortedFunc(requests, func(a, b RawRequest) int { return strings.Compare(a.Site, b.Site) }) {
		t.Errorf("requests are not sorted by site: %+v", requests)
	}
	bySite := map[string]RawRequest{}
	for _, request := range requests {
		bySite[request.Site] = request
	}
	upload := bySite["widgets.importWidgets"]
	if upload.Constructor != "UploadRequest" || upload.Verb != "POST" || upload.Options != "*gitlab.ListWidgetsOptions" || upload.UploadField != "file" {
		t.Errorf("importWidgets = %+v, want an UploadRequest POST of *gitlab.ListWidgetsOptions sending its file as file", upload)
	}
	send := bySite["widgets.sendWidget"]
	if !slices.Contains(send.Callers, RawCaller{Site: "widgets.PatchWidget", Verb: "PATCH", Options: "*widgets.widgetBody"}) {
		t.Errorf("sendWidget callers = %+v, want PatchWidget handing it a *widgets.widgetBody", send.Callers)
	}
	if len(send.Sends) != 1 || send.Sends[0].Type != "widgets.widgetBody" || send.Sends[0].Root != "UpdateWidgetOptions" {
		t.Errorf("sendWidget sends = %+v, want widgets.widgetBody beside UpdateWidgetOptions", send.Sends)
	}
	for _, request := range requests {
		if request.Constructor == "" {
			t.Errorf("request %+v names no constructor, so it is no raw request", request)
		}
	}
}

// TestRawRequests_ATreeThatWillNotLoad_IsAnError verifies the two ways the
// walk can fail before it starts, each returned rather than read as a tree
// with no raw request: a root that holds no module, and a module whose tool
// packages import no client-go.
func TestRawRequests_ATreeThatWillNotLoad_IsAnError(t *testing.T) {
	t.Run("no module", func(t *testing.T) {
		if requests, dir, err := RawRequests(t.TempDir()); err == nil {
			t.Errorf("RawRequests = %+v, %q; want an error", requests, dir)
		}
	})
	t.Run("no client-go", func(t *testing.T) {
		root := t.TempDir()
		writeFiles(t, root, map[string]string{
			"go.mod":                        "module example.com/fixture\n\ngo 1.27\n",
			"internal/tools/plain/plain.go": "package plain\n\n// Plain imports nothing.\nfunc Plain() {}\n",
		})
		if requests, dir, err := RawRequests(root); err == nil {
			t.Errorf("RawRequests = %+v, %q; want an error", requests, dir)
		}
	})
}
