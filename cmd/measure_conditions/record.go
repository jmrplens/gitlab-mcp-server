package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// recordFile and outputFile are what each run of the workflow leaves in its
// artifact: the record of what ran where and how it ended, written by the
// measuring step, and everything scripts/coverage-conditions.sh printed, both
// streams, which is gobco's raw output with the script's own lines around it.
const (
	recordFile = "run.env"
	outputFile = "gobco.txt"
)

// recordKeys are the lines a record holds, each `key=value` and each once:
// the package measured, the runner label, the GOOS/GOARCH the go command
// reported there, and the script's exit status.
var recordKeys = []string{"package", "system", "platform", "status"}

// record is one run's record together with its raw output.
type record struct {
	pkg      string
	system   string
	platform string
	status   int
	output   string
}

// readRecords reads every record below root, wherever the artifact download
// put it: download-artifact extracts each artifact into a directory named
// after it, except when there is exactly one, which it extracts into root
// itself. A root that does not exist holds no record, which is what a
// dispatch whose every run ended before its upload leaves. A record that
// cannot be read is returned as a problem naming its directory, and the rest
// are still read.
func readRecords(root string) (records []record, problems []string) {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() != recordFile {
			return nil
		}
		dir := filepath.Dir(path)
		rec, readErr := readRecord(dir)
		if readErr != nil {
			rel := cmdutil.Must(filepath.Rel(root, dir))
			problems = append(problems, fmt.Sprintf("the record in %s cannot be read: %v", filepath.ToSlash(rel), readErr))
			return nil
		}
		records = append(records, rec)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		problems = append(problems, fmt.Sprintf("the records under %s cannot be walked: %v", root, err))
	}
	return records, problems
}

// readRecord reads the record and the raw output one run left in dir.
func readRecord(dir string) (record, error) {
	text, err := os.ReadFile(filepath.Join(dir, recordFile))
	if err != nil {
		return record{}, err
	}
	fields, err := parseRecordFields(string(text))
	if err != nil {
		return record{}, err
	}
	status, err := strconv.Atoi(fields["status"])
	if err != nil {
		return record{}, fmt.Errorf("status %q is not an exit status", fields["status"])
	}
	output, err := os.ReadFile(filepath.Join(dir, outputFile))
	if err != nil {
		return record{}, err
	}
	return record{
		pkg:      fields["package"],
		system:   fields["system"],
		platform: fields["platform"],
		status:   status,
		output:   string(output),
	}, nil
}

// parseRecordFields reads a record's `key=value` lines, refusing a line of
// another shape, a key it does not know, a key given twice and a key left
// out, since a record that says less or more than the workflow writes is one
// whose meaning would have to be guessed. A carriage return a Windows runner
// may have written is dropped, and blank lines are skipped.
func parseRecordFields(text string) (map[string]string, error) {
	fields := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("line %q is not key=value", line)
		}
		if !slices.Contains(recordKeys, key) {
			return nil, fmt.Errorf("key %q is not one of %s", key, strings.Join(recordKeys, ", "))
		}
		if _, given := fields[key]; given {
			return nil, fmt.Errorf("key %q is given twice", key)
		}
		fields[key] = value
	}
	for _, key := range recordKeys {
		if _, given := fields[key]; !given {
			return nil, fmt.Errorf("key %q is missing", key)
		}
	}
	return fields, nil
}
