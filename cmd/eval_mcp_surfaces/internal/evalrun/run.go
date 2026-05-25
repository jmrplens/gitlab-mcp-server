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
