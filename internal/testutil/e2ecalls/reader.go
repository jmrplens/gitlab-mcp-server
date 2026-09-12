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

// Shard is one shard file read back: where it was, and every line it held.
//
// It exists because a shard is the one unit that says which process wrote a
// line. No call, skip or session line names its package, only the run line
// does, and one process writes one shard, so a reader that wants to place a
// call in its package reads the shards apart and joins each to the run line
// beside it. Merged, that attribution is gone.
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
// directory of its own, and the coverage audit compares the runtimes against
// each other: making a reader out of two of them would otherwise mean copying
// files around before anything could be compared.
//
// A directory holding no shard is an error rather than an empty result. These
// shards are written by a suite run, so nothing there means the suite did not
// run with recording on, and reporting that as zero coverage would be a claim
// about the server made from a claim about the harness.
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
		return nil, fmt.Errorf("no %s shard under %s: the suite records only when %s is set to an absolute directory", ShardPattern, dir, DirEnv)
	}
	return shards, nil
}

// Read merges every shard under dir, subdirectories included, in the order the
// directory tree walks. It is [ReadShards] with the file boundaries dropped,
// for a reader that wants the lines and not their provenance.
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
