package config

import (
	"fmt"
	"time"
)

// authBudgetEnv is the four settings that decide how much failed
// authentication one address may produce before it is refused, read together
// because they are two pairs and neither pair means anything half-read.
type authBudgetEnv struct {
	failureLimit   int
	failureWindow  time.Duration
	distinctLimit  int
	distinctWindow time.Duration
}

// loadAuthBudgetEnv reads the four, each bounded and each defaulting to the
// figure the constant block documents.
//
// A limit of zero turns its budget off, which is why they go through
// [parseIntNonNegative] rather than a positive-only parser: an operator who
// wants no distinct-token budget at all should be able to say so, and a
// deployment behind a gateway that already does this may well want to.
func loadAuthBudgetEnv() (authBudgetEnv, error) {
	var out authBudgetEnv
	var err error

	if out.failureLimit, err = parseIntNonNegative(Getenv("AUTH_FAILURE_LIMIT"), DefaultAuthFailureLimit); err != nil {
		return authBudgetEnv{}, fmt.Errorf("invalid AUTH_FAILURE_LIMIT value: %w", err)
	}
	if out.failureLimit > MaxAuthFailureLimit {
		return authBudgetEnv{}, fmt.Errorf("AUTH_FAILURE_LIMIT exceeds maximum of %d (got %d)", MaxAuthFailureLimit, out.failureLimit)
	}
	if out.failureWindow, err = parseDisableableDurationEnv("AUTH_FAILURE_WINDOW", DefaultAuthFailureWindow, MaxAuthFailureWindow); err != nil {
		return authBudgetEnv{}, err
	}

	if out.distinctLimit, err = parseIntNonNegative(Getenv("AUTH_DISTINCT_TOKEN_LIMIT"), DefaultAuthDistinctTokenLimit); err != nil {
		return authBudgetEnv{}, fmt.Errorf("invalid AUTH_DISTINCT_TOKEN_LIMIT value: %w", err)
	}
	if out.distinctLimit > MaxAuthDistinctTokenLimit {
		return authBudgetEnv{}, fmt.Errorf("AUTH_DISTINCT_TOKEN_LIMIT exceeds maximum of %d (got %d)", MaxAuthDistinctTokenLimit, out.distinctLimit)
	}
	if out.distinctWindow, err = parseDisableableDurationEnv("AUTH_DISTINCT_TOKEN_WINDOW", DefaultAuthDistinctWindow, MaxAuthDistinctWindow); err != nil {
		return authBudgetEnv{}, err
	}

	return out, nil
}
