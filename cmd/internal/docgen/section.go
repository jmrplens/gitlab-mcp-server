package docgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReplaceSection rewrites the file at path, replacing the content between
// startMark and endMark (both preserved) with content. It is used by the
// generator commands that maintain managed README/doc sections.
func ReplaceSection(path, startMark, endMark, content string) error {
	data, err := os.ReadFile(path) //#nosec G304 -- callers pass compile-time-constant paths
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	result, err := ComputeReplacedSection(string(data), startMark, endMark, content)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Clean(path), []byte(result), 0o644) //#nosec G306,G703 -- managed doc path is a compile-time constant, not user input
}

// ComputeReplacedSection returns text with the content between startMark and
// endMark replaced by content. It returns an error when either marker is absent,
// so callers fail fast instead of writing a malformed file.
func ComputeReplacedSection(text, startMark, endMark, content string) (string, error) {
	startIdx := strings.Index(text, startMark)
	if startIdx < 0 {
		return "", fmt.Errorf("start marker %s not found", startMark)
	}

	searchFrom := startIdx + len(startMark)
	relEndIdx := strings.Index(text[searchFrom:], endMark)
	if relEndIdx < 0 {
		return "", fmt.Errorf("end marker %s not found after start marker", endMark)
	}
	endIdx := searchFrom + relEndIdx

	before := text[:searchFrom]
	after := text[endIdx:]
	return before + "\n\n" + content + "\n" + after, nil
}
