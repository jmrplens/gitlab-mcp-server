// The reading half: every shard of a directory tree, merged in one pass, with a
// line nobody can read reported rather than dropped.

package shardio

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
// It exists because a shard is the one unit that says which process wrote a
// line. Most lines of these records name no run of their own, and the run line
// is the exception: one process writes one shard and one run line, so a reader
// that wants to place a line in its run reads the shards apart and joins each
// to the run line beside it. Merged, that attribution is gone.
type Shard[R any] struct {
	// Path is the file the lines were read from.
	Path string
	// Records is every line of the file, in file order.
	Records []R
}

// ReadShards reads every shard under dir, subdirectories included, one entry
// per file in the order the directory tree walks.
//
// Subdirectories are read because one run per target writes into a directory of
// its own, and what a reader wants is usually the comparison between them:
// making a reader out of two would otherwise mean copying files around before
// anything could be compared.
//
// A directory holding no shard is an error rather than an empty result. These
// shards are written by a run that was paid for, so nothing there means the run
// did not record, and reporting that as a record of zeroes would be a claim
// about the thing under test made from a claim about an environment variable.
func (s *Shards[R, L]) ReadShards(dir string) ([]Shard[R], error) {
	var (
		shards      []Shard[R]
		readFailure error
	)
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, entryErr error) error {
		if entryErr != nil {
			return entryErr
		}
		if entry.IsDir() || !s.IsShard(entry.Name()) {
			return nil
		}
		read, err := s.readShard(path)
		if err != nil {
			readFailure = err
			return fs.SkipAll
		}
		shards = append(shards, Shard[R]{Path: path, Records: read})
		return nil
	})
	if readFailure != nil {
		return nil, readFailure
	}
	if walkErr != nil {
		return nil, fmt.Errorf("read shard directory %s: %w", dir, walkErr)
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("no %s shard under %s: nothing is recorded unless %s names an absolute directory", s.pattern(), dir, s.spec.DirEnv)
	}
	return shards, nil
}

// Read merges every shard under dir, subdirectories included, in the order the
// directory tree walks. It is [Shards.ReadShards] with the file boundaries
// dropped, for a reader that wants the lines and not their provenance.
func (s *Shards[R, L]) Read(dir string) ([]R, error) {
	shards, err := s.ReadShards(dir)
	if err != nil {
		return nil, err
	}
	var records []R
	for _, shard := range shards {
		records = append(records, shard.Records...)
	}
	return records, nil
}

// readShard reads one shard file.
func (s *Shards[R, L]) readShard(path string) ([]R, error) {
	// The path comes from a walk of the directory the caller named.
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open shard: %w", err)
	}
	defer func() { _ = file.Close() }()

	var records []R
	scanner := bufio.NewScanner(file)
	// The same bound the writer refuses to exceed, so a shard this package
	// wrote is a shard this package reads. A line past it is an error below
	// rather than a skip, and the writer reports it at the moment it happens,
	// when the run that produced it is still there to be fixed.
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), MaxLine)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record R
		if unmarshalErr := json.Unmarshal([]byte(text), &record); unmarshalErr != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, line, unmarshalErr)
		}
		if validateErr := s.spec.Validate(record); validateErr != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, validateErr)
		}
		records = append(records, record)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("read %s: %w", path, scanErr)
	}
	return records, nil
}
