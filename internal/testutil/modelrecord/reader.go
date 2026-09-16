// The reading half: every shard of a directory tree, merged in one pass, with a
// line nobody can read reported rather than dropped.

package modelrecord

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Shard is one shard file read back: where it was, and every line it held.
//
// It exists because a shard is the one unit that says which run a line belongs
// to. Every other join in this record is by name, and a run line is the
// exception: no attempt, turn, call or verify line names its run, one process
// writes one shard and one run line, so a reader that wants to place an attempt
// in its run reads the shards apart and joins each to the run line beside it.
// Merged, that attribution is gone, and with it the commit, the instance and
// the tier a published row has to carry.
type Shard struct {
	// Path is the file the lines were read from.
	Path string
	// Records is every line of the file, in file order.
	Records []Record
}

// ReadShards reads every shard under dir, subdirectories included, one entry
// per file in the order the directory tree walks.
//
// Subdirectories are read because one run per Docker target writes into a
// directory of its own, and a cross-surface or cross-vendor table is built by
// comparing them: making a reader out of two of them would otherwise mean
// copying files around before anything could be compared.
//
// A directory holding no shard is an error rather than an empty result. These
// shards are written by a run that was paid for, so nothing there means the run
// did not record, and publishing that as a table of zeroes would be a claim
// about four models made from a claim about one environment variable.
func ReadShards(dir string) ([]Shard, error) {
	var (
		shards      []Shard
		readFailure error
	)
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, entryErr error) error {
		if entryErr != nil {
			return entryErr
		}
		if entry.IsDir() || !isShard(entry.Name()) {
			return nil
		}
		read, err := readShard(path)
		if err != nil {
			readFailure = err
			return fs.SkipAll
		}
		shards = append(shards, Shard{Path: path, Records: read})
		return nil
	})
	if readFailure != nil {
		return nil, readFailure
	}
	if walkErr != nil {
		return nil, fmt.Errorf("read shard directory %s: %w", dir, walkErr)
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("no %s shard under %s: a run records only when %s is set to an absolute directory", ShardPattern, dir, DirEnv)
	}
	return shards, nil
}

// Read merges every shard under dir, subdirectories included, in the order the
// directory tree walks. It is [ReadShards] with the file boundaries dropped,
// for a reader that wants the lines and not the run each belongs to.
func Read(dir string) ([]Record, error) {
	shards, err := ReadShards(dir)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, shard := range shards {
		records = append(records, shard.Records...)
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
	// The path comes from a walk of the directory the caller named.
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open shard: %w", err)
	}
	defer func() { _ = file.Close() }()

	var records []Record
	scanner := bufio.NewScanner(file)
	// The same bound the writer refuses to exceed, so a shard this package wrote
	// is a shard this package reads. A line past it is an error below rather
	// than a skip, and the writer reports it at the moment it happens, when the
	// run that produced it is still there to be fixed.
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
