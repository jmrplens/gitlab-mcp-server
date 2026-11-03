// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package derive

import (
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/actionfixture"
)

// guardedSource is the fixture the nil checks are held to: one handler per
// shape the argument of a nil-guarded parameter takes, through the real
// client-go signatures and an ActionSpec, the way a domain package writes one.
const guardedSource = `package guarded

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type Input struct {
	ID      string               @@json:"id"@@
	Label   toolutil.StringOrInt @@json:"label"@@
	Name    string               @@json:"name"@@
	Pointer *string              @@json:"pointer"@@
	Any     any                  @@json:"any"@@
}

type Output struct {
	OK bool @@json:"ok"@@
}

func selector(label toolutil.StringOrInt, name string) string {
	if label == "" {
		return name
	}
	return string(label)
}

func del(ctx context.Context, client *gitlabclient.Client, project, label any) error {
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func onward(ctx context.Context, client *gitlabclient.Client, project, label any) error {
	return del(ctx, client, project, label)
}

func apply(fn func(any, any, *gl.DeleteLabelOptions, ...gl.RequestOptionFunc) (*gl.Response, error), project any) error {
	_, err := fn(project, "x", nil)
	return err
}

func blank(target *any) {
	*target = nil
}

func rewritten(ctx context.Context, client *gitlabclient.Client, project, label any) error {
	label = nil
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func compoundResult(ctx context.Context, client *gitlabclient.Client, project, label any) error {
	label, err := pick()
	if err != nil {
		return err
	}
	_, err = client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func pick() (any, error) {
	return nil, nil
}

func rangedKey(ctx context.Context, client *gitlabclient.Client, project, label any, labels map[any]bool) error {
	for label = range labels {
	}
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func rangedValue(ctx context.Context, client *gitlabclient.Client, project, label any, labels []any) error {
	for _, label = range labels {
	}
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func addressed(ctx context.Context, client *gitlabclient.Client, project, label any) error {
	blank(&label)
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

type holder struct {
	value any
}

func untouched(ctx context.Context, client *gitlabclient.Client, project, label any, ready bool, labels []any) error {
	var other any
	other = ready
	for other = range labels {
	}
	for range labels {
	}
	blank(&other)
	var kept holder
	blank(&kept.value)
	blocked := !ready
	_ = blocked
	{
		label := "a name of its own"
		_ = label
	}
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func generic[T any](ctx context.Context, client *gitlabclient.Client, project any, label T) error {
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func genericZero[T any](ctx context.Context, client *gitlabclient.Client, project any) error {
	var label T
	_, err := client.GL().Labels.DeleteLabel(project, label, nil, gl.WithContext(ctx))
	return err
}

func identity[T any](value T) T {
	return value
}

func genericCalled[T any](ctx context.Context, client *gitlabclient.Client, project any, label T) error {
	_, err := client.GL().Labels.DeleteLabel(project, identity(label), nil, gl.WithContext(ctx))
	return err
}

func Converted(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, string(input.Label), nil, gl.WithContext(ctx))
	return Output{}, err
}

func Selected(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, selector(input.Label, input.Name), nil, gl.WithContext(ctx))
	return Output{}, err
}

func Parenthesized(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, (input.Name), nil, gl.WithContext(ctx))
	return Output{}, err
}

func LiteralString(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, "bug", nil, gl.WithContext(ctx))
	return Output{}, err
}

func LiteralNil(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, nil, nil, gl.WithContext(ctx))
	return Output{}, err
}

func ParenthesizedNil(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, (nil), nil, gl.WithContext(ctx))
	return Output{}, err
}

func PointerField(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, input.Pointer, nil, gl.WithContext(ctx))
	return Output{}, err
}

func AnyField(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.DeleteLabel(input.ID, input.Any, nil, gl.WithContext(ctx))
	return Output{}, err
}

func AnyLocal(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	label := input.Any
	_, err := client.GL().Labels.DeleteLabel(input.ID, label, nil, gl.WithContext(ctx))
	return Output{}, err
}

func HelperValue(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, del(ctx, client, input.ID, input.Name)
}

func HelperNil(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, del(ctx, client, input.ID, nil)
}

func HelperAny(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, del(ctx, client, input.ID, input.Any)
}

func TwoHelpers(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, onward(ctx, client, input.ID, input.Name)
}

func MethodValue(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, apply(client.GL().Labels.DeleteLabel, input.ID)
}

func HelperRewritten(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, rewritten(ctx, client, input.ID, input.Name)
}

func HelperCompoundResult(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, compoundResult(ctx, client, input.ID, nil)
}

func HelperRangedKey(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, rangedKey(ctx, client, input.ID, input.Name, nil)
}

func HelperRangedValue(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, rangedValue(ctx, client, input.ID, input.Name, nil)
}

func HelperAddressed(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, addressed(ctx, client, input.ID, input.Name)
}

func HelperUntouched(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, untouched(ctx, client, input.ID, input.Name, true, nil)
}

func GenericNil(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, generic[any](ctx, client, input.ID, nil)
}

func GenericValue(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, generic(ctx, client, input.ID, input.Name)
}

func GenericAny(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, generic[any](ctx, client, input.ID, input.Any)
}

func GenericZero(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, genericZero[any](ctx, client, input.ID)
}

func GenericCalled(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, genericCalled[any](ctx, client, input.ID, nil)
}

func Unguarded(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _, err := client.GL().Labels.GetLabel(input.ID, input.Name, gl.WithContext(ctx))
	return Output{}, err
}

func PastTheEnd(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.UnsubscribeFromLabel(input.ID, input.Name, gl.WithContext(ctx))
	return Output{}, err
}

func FirstFails(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.PromoteLabel(nil, input.Name, gl.WithContext(ctx))
	return Output{}, err
}

func SecondFails(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.PromoteLabel(input.ID, nil, gl.WithContext(ctx))
	return Output{}, err
}

func BothPass(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().Labels.PromoteLabel(input.ID, input.Name, gl.WithContext(ctx))
	return Output{}, err
}

func NothingLeft(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _, err := client.GL().Labels.SubscribeToLabel(input.ID, input.Name, gl.WithContext(ctx))
	return Output{}, err
}

func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		spec("converted", toolutil.RouteAction(client, Converted)),
		spec("selected", toolutil.RouteAction(client, Selected)),
		spec("parenthesized", toolutil.RouteAction(client, Parenthesized)),
		spec("literal_string", toolutil.RouteAction(client, LiteralString)),
		spec("literal_nil", toolutil.RouteAction(client, LiteralNil)),
		spec("parenthesized_nil", toolutil.RouteAction(client, ParenthesizedNil)),
		spec("pointer_field", toolutil.RouteAction(client, PointerField)),
		spec("any_field", toolutil.RouteAction(client, AnyField)),
		spec("any_local", toolutil.RouteAction(client, AnyLocal)),
		spec("helper_value", toolutil.RouteAction(client, HelperValue)),
		spec("helper_nil", toolutil.RouteAction(client, HelperNil)),
		spec("helper_any", toolutil.RouteAction(client, HelperAny)),
		spec("two_helpers", toolutil.RouteAction(client, TwoHelpers)),
		spec("method_value", toolutil.RouteAction(client, MethodValue)),
		spec("helper_rewritten", toolutil.RouteAction(client, HelperRewritten)),
		spec("helper_compound_result", toolutil.RouteAction(client, HelperCompoundResult)),
		spec("helper_ranged_key", toolutil.RouteAction(client, HelperRangedKey)),
		spec("helper_ranged_value", toolutil.RouteAction(client, HelperRangedValue)),
		spec("helper_addressed", toolutil.RouteAction(client, HelperAddressed)),
		spec("helper_untouched", toolutil.RouteAction(client, HelperUntouched)),
		spec("generic_nil", toolutil.RouteAction(client, GenericNil)),
		spec("generic_value", toolutil.RouteAction(client, GenericValue)),
		spec("generic_any", toolutil.RouteAction(client, GenericAny)),
		spec("generic_zero", toolutil.RouteAction(client, GenericZero)),
		spec("generic_called", toolutil.RouteAction(client, GenericCalled)),
		spec("unguarded", toolutil.RouteAction(client, Unguarded)),
		spec("past_the_end", toolutil.RouteAction(client, PastTheEnd)),
		spec("first_fails", toolutil.RouteAction(client, FirstFails)),
		spec("second_fails", toolutil.RouteAction(client, SecondFails)),
		spec("both_pass", toolutil.RouteAction(client, BothPass)),
		spec("nothing_left", toolutil.RouteAction(client, NothingLeft)),
	}
}

func spec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{Usage: "fixture"})
}
`

// The two routes the fixture's delete chooses between by a nil check of its
// label, spelled once for the expectations below.
const (
	labelsRoute = "DELETE /projects/:/labels"
	labelRoute  = "DELETE /projects/:/labels/:"
)

// guardedSDK is the client-go the guarded fixture calls: the delete as client-go
// has it; a read with no check; a route behind a check of an argument past the
// call's end; a route behind two checks at once; and one whose only route sits
// behind a check its one caller cannot pass.
func guardedSDK() stubSDK {
	route := func(method, path string) Request { return Request{Kind: KindREST, Method: method, Path: path} }
	return stubSDK{
		"Labels.DeleteLabel": {
			routes:    []Request{route("DELETE", "/projects/:/labels"), route("DELETE", "/projects/:/labels/:")},
			nilChecks: map[string][]NilCheck{labelsRoute: {{Index: 1, Nil: true}}, labelRoute: {{Index: 1}}},
		},
		"Labels.GetLabel": {routes: []Request{route("GET", "/projects/:/labels/:")}},
		"Labels.UnsubscribeFromLabel": {
			routes:    []Request{route("POST", "/unsubscribe")},
			nilChecks: map[string][]NilCheck{"POST /unsubscribe": {{Index: 3, Nil: true}}},
		},
		"Labels.PromoteLabel": {
			routes:    []Request{route("PUT", "/promote")},
			nilChecks: map[string][]NilCheck{"PUT /promote": {{Index: 0}, {Index: 1}}},
		},
		"Labels.SubscribeToLabel": {
			routes:    []Request{route("POST", "/subscribe")},
			nilChecks: map[string][]NilCheck{"POST /subscribe": {{Index: 1, Nil: true}}},
		},
	}
}

// guardedCache memoizes the loaded fixture, which every test of this file reads
// without changing it.
var guardedCache *actionrequests.Program

// loadGuarded loads the guarded fixture program.
func loadGuarded(t *testing.T) *actionrequests.Program {
	t.Helper()
	if guardedCache != nil {
		return guardedCache
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	root, err := actionfixture.Root(dir)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	prog, err := actionrequests.Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, map[string]string{"guarded": guardedSource}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	guardedCache = prog
	return prog
}

// TestDerive_ANilGuardedParameter_DerivesTheBranchesItsArgumentCanTake verifies,
// for every shape the argument takes, which of a method's routes the action is
// derived to send: a value of a type that cannot hold nil, converted, returned
// by a helper, in parentheses, or a literal, derives the branch of a parameter
// that is not nil, and alone; the predeclared nil, in parentheses too, derives
// the other alone; a pointer field, an interface field and a local of an
// interface type derive both; a helper's parameter is read through what its
// caller handed it, through two helpers as well, and through an interface it is
// both, unless the helper assigns it, ranges into it or takes its address, when
// it derives both whatever it was handed, while writing another name, or a name
// of its own that hides the parameter, changes nothing; a parameter of a type
// parameter is read through what it was handed as well, since its instantiation
// may hold nil, and a value of that type no call handed derives both; a method
// named without being called derives both, having no argument to read; a method
// with no check is as it was, and a check of an argument past the call's end
// decides nothing. With two checks at once, the route is gone when either fails
// and kept when both pass, and an action whose only route went sends nothing.
func TestDerive_ANilGuardedParameter_DerivesTheBranchesItsArgumentCanTake(t *testing.T) {
	both := labelsRoute + " [alternative]\n" + labelRoute + " [alternative]\npath [0]\npath [1]\n"
	names := map[string]string{
		"helper_rewritten":       both,
		"helper_compound_result": both,
		"helper_ranged_key":      both,
		"helper_ranged_value":    both,
		"helper_addressed":       both,
		"helper_untouched":       labelRoute + " [mandatory]\npath [0]\n",
		"generic_nil":            labelsRoute + " [mandatory]\npath [0]\n",
		"generic_value":          labelRoute + " [mandatory]\npath [0]\n",
		"generic_any":            both,
		"generic_zero":           both,
		"generic_called":         both,

		"converted":         labelRoute + " [mandatory]\npath [0]\n",
		"selected":          labelRoute + " [mandatory]\npath [0]\n",
		"parenthesized":     labelRoute + " [mandatory]\npath [0]\n",
		"literal_string":    labelRoute + " [mandatory]\npath [0]\n",
		"literal_nil":       labelsRoute + " [mandatory]\npath [0]\n",
		"parenthesized_nil": labelsRoute + " [mandatory]\npath [0]\n",
		"pointer_field":     labelsRoute + " [alternative]\n" + labelRoute + " [alternative]\npath [0]\npath [1]\n",
		"any_field":         labelsRoute + " [alternative]\n" + labelRoute + " [alternative]\npath [0]\npath [1]\n",
		"any_local":         labelsRoute + " [alternative]\n" + labelRoute + " [alternative]\npath [0]\npath [1]\n",
		"helper_value":      labelRoute + " [mandatory]\npath [0]\n",
		"helper_nil":        labelsRoute + " [mandatory]\npath [0]\n",
		"helper_any":        labelsRoute + " [alternative]\n" + labelRoute + " [alternative]\npath [0]\npath [1]\n",
		"two_helpers":       labelRoute + " [mandatory]\npath [0]\n",
		"method_value":      labelsRoute + " [alternative]\n" + labelRoute + " [alternative]\npath [0]\npath [1]\n",
		"unguarded":         "GET /projects/:/labels/: [mandatory]\npath [0]\n",
		"past_the_end":      "POST /unsubscribe [mandatory]\npath [0]\n",
		"first_fails":       "path []\n",
		"second_fails":      "path []\n",
		"both_pass":         "PUT /promote [mandatory]\npath [0]\n",
		"nothing_left":      "path []\n",
	}
	actions := make([]actionrequests.Action, 0, len(names))
	for name := range names {
		actions = append(actions, actionrequests.Action{ID: "guarded." + name, Name: name, Owner: "guarded"})
	}
	result := Derive(loadGuarded(t), actions, guardedSDK(), nil)
	for name, want := range names {
		t.Run(name, func(t *testing.T) {
			if got := describe(actionByID(t, result, "guarded."+name)); got != want {
				t.Errorf("guarded.%s derives\n%s\nwant\n%s", name, got, want)
			}
		})
	}
}
