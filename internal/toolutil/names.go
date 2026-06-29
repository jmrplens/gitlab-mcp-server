package toolutil

import (
	"slices"
	"strings"
)

// ReadOnlyNameSuffixes lists tool-name suffixes that imply a read-only
// operation. Shared between annotation derivation and surface-quality audits
// so both agree on what "looks read-only".
var ReadOnlyNameSuffixes = []string{
	"_list", "_lists", "_get", "_search",
	"_latest", "_blame", "_raw", "_diff", "_refs",
	"_statuses", "_signature", "_languages", "_statistics",
}

// IsReadToolName reports whether name ends with a read-only suffix.
func IsReadToolName(name string) bool {
	for _, sfx := range ReadOnlyNameSuffixes {
		if strings.HasSuffix(name, sfx) {
			return true
		}
	}
	return false
}

// IsDeleteToolName reports whether name contains or ends with "delete".
func IsDeleteToolName(name string) bool {
	if strings.HasSuffix(name, "_delete") {
		return true
	}
	return slices.Contains(strings.Split(name, "_"), "delete")
}
