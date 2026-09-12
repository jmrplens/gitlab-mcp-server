//go:build e2e

// iteration_test.go drives the GraphQL sender the iteration builder stands
// on against the stub: one send per document, the mutation's own errors
// surfaced, and a refused document told apart from an empty answer.

package fixture

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRunGraphQL_Answers_AreDecodedFromTheDataHalf checks the happy path:
// the document is sent exactly once and the data half lands in the caller's
// own shape. Once matters because every document here is a mutation, and a
// sender that asked twice would create two cadences and report one.
func TestRunGraphQL_Answers_AreDecodedFromTheDataHalf(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"iterationCreate":{"iteration":{"id":"gid://gitlab/Iteration/5","iid":"3"},"errors":[]}}}`}
	})

	var got struct {
		IterationCreate struct {
			Iteration struct {
				ID  string `json:"id"`
				IID string `json:"iid"`
			} `json:"iteration"`
			Errors []string `json:"errors"`
		} `json:"iterationCreate"`
	}
	err := runGraphQL(context.Background(), client, iterationMutation, map[string]any{"title": "x"}, &got)
	if err != nil {
		t.Fatalf("runGraphQL() error = %v, want nil", err)
	}
	if got.IterationCreate.Iteration.ID != "gid://gitlab/Iteration/5" || got.IterationCreate.Iteration.IID != "3" {
		t.Errorf("decoded %+v, want the iteration the stub answered", got.IterationCreate.Iteration)
	}
	if sent := len(stub.graphqlDocuments); sent != 1 {
		t.Errorf("the document was sent %d times, want exactly once", sent)
	}
}

// TestRunGraphQL_RefusedDocument_IsAnErrorNamingTheReason checks that the
// top-level errors GitLab reports inside a 200 become an error, since a
// caller reading only the data half would see an empty mutation payload and
// conclude the mutation did nothing rather than that it was refused.
func TestRunGraphQL_RefusedDocument_IsAnErrorNamingTheReason(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":null,"errors":[{"message":"Field 'iterationCadenceCreate' doesn't exist on type 'Mutation'"}]}`}
	})

	var got map[string]any
	err := runGraphQL(context.Background(), client, cadenceMutation, nil, &got)
	if err == nil {
		t.Fatal("runGraphQL() error = nil, want the refusal")
	}
	if !strings.Contains(err.Error(), "doesn't exist on type 'Mutation'") {
		t.Errorf("runGraphQL() error = %q, want it to carry GitLab's own reason", err)
	}
}

// TestRunGraphQL_EmptyAnswer_IsAnError checks that an answer with neither
// data nor errors is reported rather than decoded into nothing.
func TestRunGraphQL_EmptyAnswer_IsAnError(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{}`}
	})

	var got map[string]any
	err := runGraphQL(context.Background(), client, cadenceMutation, nil, &got)
	if !errors.Is(err, errGraphQLNoData) {
		t.Errorf("runGraphQL() error = %v, want %v", err, errGraphQLNoData)
	}
}

// TestRunGraphQL_UndecodableData_NamesTheShape checks that a data half the
// caller's shape cannot hold is an error naming that shape, so a changed
// schema fails where it is read rather than as a zero value downstream.
func TestRunGraphQL_UndecodableData_NamesTheShape(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"iterationCreate":"not an object"}}`}
	})

	var got struct {
		IterationCreate struct {
			Errors []string `json:"errors"`
		} `json:"iterationCreate"`
	}
	err := runGraphQL(context.Background(), client, iterationMutation, nil, &got)
	if err == nil || !strings.Contains(err.Error(), "decoding the document's data") {
		t.Errorf("runGraphQL() error = %v, want a decoding error naming the shape", err)
	}
}

// TestIteration_String_NamesEveryHalf checks that the message form of an
// iteration carries the GID, the iid and the title, which are the three
// things a failure about one needs to be looked up by.
func TestIteration_String_NamesEveryHalf(t *testing.T) {
	iteration := Iteration{GID: "gid://gitlab/Iteration/9", IID: 4, Title: "e2e-iteration"}
	got := iteration.String()
	for _, want := range []string{"gid://gitlab/Iteration/9", "iid 4", "e2e-iteration"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("String() = %q, want it to carry %q", got, want)
			}
		})
	}
}
