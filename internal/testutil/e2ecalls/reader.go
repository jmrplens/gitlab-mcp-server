// The reading half: every shard of a directory tree, merged in one pass, with
// a line nobody can read reported rather than dropped.

package e2ecalls

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxShardLine bounds one recorded line.
//
// The longest today is a session line carrying a thousand tool names, which is
// tens of kilobytes, and the scanner's default 64 KiB limit would nearly cover
// it. The cap is a megabyte because a line silently dropped for being long
// would look exactly like a call nobody made, which is the one thing this
// record exists to be believed about.
const maxShardLine = 1 << 20

// Read merges every shard under dir, subdirectories included, in the order the
// directory tree walks.
//
// Subdirectories are read because one run per Docker target writes into a
// directory of its own, and the coverage audit compares the runtimes against
// each other: making a reader out of two of them would otherwise mean copying
// files around before anything could be compared.
//
// A directory holding no shard is an error rather than an empty result. These
// shards are written by a suite run, so nothing there means the suite did not
// run with recording on, and reporting that as zero coverage would be a claim
// about the server made from a claim about the harness.
func Read(dir string) ([]Record, error) {
	var (
		records     []Record
		readFailure error
		shards      int
	)
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, entryErr error) error {
		if entryErr != nil {
			return entryErr
		}
		if entry.IsDir() || !isShard(entry.Name()) {
			return nil
		}
		shards++
		read, err := readShard(path)
		if err != nil {
			readFailure = err
			return fs.SkipAll
		}
		records = append(records, read...)
		return nil
	})
	if readFailure != nil {
		return nil, readFailure
	}
	if walkErr != nil {
		return nil, fmt.Errorf("read shard directory %s: %w", dir, walkErr)
	}
	// Asked as "did the walk reach at least one shard" rather than "is the
	// count exactly zero": the question is whether anything was read, and a
	// guard that only recognizes one value answers it for one value.
	if shards <= 0 {
		return nil, fmt.Errorf("no %s shard under %s: the suite records only when %s is set to an absolute directory", ShardPattern, dir, DirEnv)
	}
	return records, nil
}

// isShard reports whether a file name is one of this package's shards.
//
// It matches the two halves of [ShardPattern] rather than calling
// [path/filepath.Match] on it, which would hand back a pattern error this
// package owns the pattern for and could only discard.
func isShard(name string) bool {
	return strings.HasPrefix(name, shardPrefix) && strings.HasSuffix(name, shardExt)
}

// readShard reads one shard file.
func readShard(path string) ([]Record, error) {
	// #nosec G304 -- the path comes from a walk of the directory the caller named.
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open shard: %w", err)
	}
	defer func() { _ = file.Close() }()

	var records []Record
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxShardLine)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record Record
		if unmarshalErr := json.Unmarshal([]byte(text), &record); unmarshalErr != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, unmarshalErr)
		}
		if validateErr := record.validate(); validateErr != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, validateErr)
		}
		records = append(records, record)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("read %s: %w", path, scanErr)
	}
	return records, nil
}
