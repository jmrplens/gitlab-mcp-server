package suite

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

const stableHashLength = 10

var (
	e2eRunID      = newE2ERunID(time.Now())
	uniqueCounter atomic.Int64
	unsafeChars   = regexp.MustCompile(`[^a-z0-9-]`)
	runIDCounter  atomic.Int64
)

func configuredE2ERunID(now time.Time) string {
	if value := os.Getenv("E2E_RUN_ID"); value != "" {
		if runID := sanitizeNamePart(value, 48); runID != "" {
			return runID
		}
	}
	return newE2ERunID(now)
}

func newE2ERunID(now time.Time) string {
	source := fmt.Sprintf("%d-%d-%d", now.UnixNano(), os.Getpid(), runIDCounter.Add(1))
	return fmt.Sprintf("%s-%s", now.UTC().Format("20060102t150405z"), shortStableHash(source))
}

func shortStableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:stableHashLength]
}

func sanitizeTestName(name string) string {
	return sanitizeNamePart(name, 40)
}

func sanitizeNamePrefix(prefix string) string {
	name := sanitizeNamePart(prefix, 80)
	if name == "" {
		return "e2e"
	}
	return name
}

func sanitizeNamePart(name string, maxLength int) string {
	sanitized := strings.ToLower(name)
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = unsafeChars.ReplaceAllString(sanitized, "")
	sanitized = strings.Trim(sanitized, "-")
	if maxLength > 0 && len(sanitized) > maxLength {
		sanitized = strings.Trim(sanitized[:maxLength], "-")
	}
	return sanitized
}

func uniqueName(prefix string) string {
	sanitizedPrefix := sanitizeNamePrefix(prefix)
	return fmt.Sprintf("%s-%s-%s-%d", sanitizedPrefix, e2eRunID, shortStableHash(sanitizedPrefix), uniqueCounter.Add(1))
}
