package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	evalElicitationReleaseTag   atomic.Value
	evalElicitationSourceBranch atomic.Value
)

// liveResourceSequence stores the package-level live resource sequence state.
var liveResourceSequence atomic.Uint64

// liveUniqueSuffix returns unique suffix for live evaluation runs.
func liveUniqueSuffix() string {
	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err == nil {
		return hex.EncodeToString(randomBytes[:])
	}
	return fmt.Sprintf("%x-%s", os.Getpid(), strconv.FormatUint(liveResourceSequence.Add(1), 36))
}

// waitForContext waits for for context to become available.
func waitForContext(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// options holds options data for the main package.
