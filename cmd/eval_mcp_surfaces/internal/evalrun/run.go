// Package evalrun provides small helpers shared by the live evaluation
// command: deterministic unique suffixes for ephemeral GitLab resources and
// environment-driven run configuration used across e2e fixtures.
//
// The package keeps these helpers in a shared location so the evaluator CLI
// and the live fixture preparers can agree on a single naming convention and
// a single context-aware wait helper.
package evalrun

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

var liveResourceSequence atomic.Uint64

// UniqueSuffix returns a path-safe unique suffix for live evaluation resources.
func UniqueSuffix() string {
	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err == nil {
		return hex.EncodeToString(randomBytes[:])
	}
	return fmt.Sprintf("%x-%s", os.Getpid(), strconv.FormatUint(liveResourceSequence.Add(1), 36))
}

// WaitForContext waits for interval to elapse or for ctx to be canceled.
func WaitForContext(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
