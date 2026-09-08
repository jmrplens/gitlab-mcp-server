package docgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// generatedFileMode is the mode a generated artifact is created with. The
	// copies this replaced disagreed, 0o644 in one and 0o600 in seven, and
	// 0o600 wins for two reasons: it is what gosec's G306 accepts without a
	// suppression, and the mode only ever applies to a file the generator
	// creates from nothing, since neither os.WriteFile nor Root.WriteFile
	// changes the mode of a file that already exists. Every artifact these
	// helpers write is committed, so a checkout gets git's mode and the
	// difference is invisible in practice.
	generatedFileMode = 0o600

	// generatedDirMode is the mode a missing parent directory is created with.
	generatedDirMode = 0o750
)

// WriteOrCheck writes content to path, or -- when check is set -- reports
// whether the file already there holds exactly that content. It is the one
// place the whole-file freshness convention is decided for the generators that
// own a committed artifact: the comparison is line-ending agnostic, so a
// Windows checkout does not report drift a Linux one does not see; content is
// given the trailing newline that keeps a generated file from being the one
// text file in the repository without one; the parent directory is created on
// the way in; and a stale artifact is reported with one sentence naming the
// file and the command that refreshes it, which regenerate supplies.
//
// The file is written through an os.Root opened on its directory, so the write
// can only ever land on the named file: that containment came from
// cmd/gen_llms and cmd/gen_lhm_manifest, and giving every caller the strictest
// of the merged behaviors is cheaper than explaining which two commands keep
// it.
//
// This is deliberately not the same function as [WriteReport]. A generated
// artifact is a file the repository commits and CI compares; a report is a
// destination an operator names on a flag, where "-" means stdout and no
// freshness question is being asked. One function with a mode flag would put
// two contracts behind one signature.
func WriteOrCheck(path string, content []byte, check bool, regenerate string) error {
	content = ensureTrailingNewline(content)
	dir, name := filepath.Split(path)
	if dir == "" {
		dir = "."
	}

	if !check {
		if err := os.MkdirAll(dir, generatedDirMode); err != nil {
			return fmt.Errorf("create directory for %s: %w", path, err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	if check {
		existing, readErr := root.ReadFile(name)
		if readErr != nil {
			// Wrapped rather than replaced: a caller distinguishes an artifact
			// that is missing from one that is stale with errors.Is, and the
			// two failures want different fixes.
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if !bytes.Equal(normalizeNewlines(existing), normalizeNewlines(content)) {
			return fmt.Errorf("%s is stale; run %s", path, regenerate)
		}
		return nil
	}
	if writeErr := root.WriteFile(name, content, generatedFileMode); writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	return nil
}

// WriteReport writes an audit's report to path, where the sentinel "-" means
// stdout. It is the one place that convention is decided for the auditors whose
// -output flag names a file the repository does not commit.
//
// Unlike [WriteOrCheck] the path is the operator's own, so nothing here is
// contained to a directory and nothing is compared: the report is whatever this
// run found, and the only question is where it lands.
func WriteReport(path string, content []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(content)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), generatedDirMode); err != nil {
		return err
	}
	return os.WriteFile(path, content, generatedFileMode)
}

// normalizeNewlines strips carriage returns so a freshness comparison is
// line-ending agnostic across platforms.
func normalizeNewlines(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// ensureTrailingNewline appends the final newline when content lacks one.
// Empty content is left empty: a file with nothing in it is a caller's mistake
// to report, not one this helper should paper over with a blank line.
func ensureTrailingNewline(content []byte) []byte {
	if len(content) == 0 || content[len(content)-1] == '\n' {
		return content
	}
	return append(append(make([]byte, 0, len(content)+1), content...), '\n')
}
