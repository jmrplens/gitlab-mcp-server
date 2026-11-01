// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import "context"

// WithheldAnswer explains a call to a meta tool naming an action the
// session's configuration withheld from the tool's group: the cause, in the
// words the dynamic surface gives it, and true; or false when the action is
// not one the session withheld (issue 1282).
type WithheldAnswer func(toolName, action string) (string, bool)

// withheldAnswerKey is the context key a [WithheldAnswer] travels under.
type withheldAnswerKey struct{}

// WithWithheldAnswer returns ctx carrying answer, which the meta dispatcher
// consults before it calls an action unknown.
//
// The call middleware that knows what a server withheld answers almost every
// such call itself, before the SDK refuses the action as a value outside the
// tool's enum, and it installs this for the rest: an action the dispatcher
// arrives at that the call did not name, which the parameter-dependent alias
// rewrite produces, and a handler driven without that middleware in front of
// it. A nil answer is not installed.
func WithWithheldAnswer(ctx context.Context, answer WithheldAnswer) context.Context {
	if answer == nil {
		return ctx
	}
	return context.WithValue(ctx, withheldAnswerKey{}, answer)
}

// withheldAnswerFor is the answer ctx carries for a call to toolName naming
// action, if it carries one and the action was withheld.
func withheldAnswerFor(ctx context.Context, toolName, action string) (string, bool) {
	answer, carried := ctx.Value(withheldAnswerKey{}).(WithheldAnswer)
	if !carried {
		return "", false
	}
	return answer(toolName, action)
}
