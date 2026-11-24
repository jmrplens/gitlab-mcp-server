// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// settings are what one run reads before it does anything: the repository's
// root, the project and branch it is about, the host and the token it asks,
// what .env sets, the minimum coverage as it was written, so a message shows
// the caller's own figure, and how long the task is waited for and how often
// it is looked at.
type settings struct {
	root         string
	projectKey   string
	branch       string
	host         string
	token        string
	dotenv       []string
	coverageMin  string
	pollTimeout  time.Duration
	pollInterval time.Duration
}

// readSettings finds the repository's root and reads .env there, the
// environment and sonar-project.properties.
func (s *session) readSettings(ctx context.Context) (settings, error) {
	git, err := s.lookPath("git")
	if err != nil {
		return settings{}, fmt.Errorf("find git: %w", err)
	}
	read := settings{}
	if read.root, err = s.gitOutput(ctx, git, s.dir, "rev-parse", "--show-toplevel"); err != nil {
		return settings{}, err
	}
	if read.dotenv, err = readDotenv(filepath.Join(read.root, ".env")); err != nil {
		return settings{}, err
	}
	set := append(slices.Clone(s.environ), read.dotenv...)
	if read.token, _ = recipe.Getenv(set, "SONARQUBE_TOKEN"); read.token == "" {
		return settings{}, errors.New("SONARQUBE_TOKEN is not set in .env")
	}
	if envErr := s.readEnvironment(&read); envErr != nil {
		return settings{}, envErr
	}
	properties, err := os.ReadFile(filepath.Join(read.root, "sonar-project.properties"))
	if err != nil {
		return settings{}, err
	}
	if read.projectKey, err = property(properties, "sonar.projectKey"); err != nil {
		return settings{}, fmt.Errorf("sonar-project.properties: %w", err)
	}
	if read.branch, _ = recipe.Getenv(set, "SONAR_BRANCH"); read.branch == "" {
		read.branch, err = s.gitOutput(ctx, git, read.root, "rev-parse", "--abbrev-ref", "HEAD")
	}
	return read, err
}

// readDotenv returns what the .env at name sets, sorted, as [os.Environ] writes
// it. The parser's own error is never shown: it quotes the rest of the file
// from where it stopped, or the value that does not end, and the file holds
// tokens. Only an error of the file itself, which names no more than the file,
// is.
func readDotenv(name string) ([]string, error) {
	values, err := godotenv.Read(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New(".env not found in repo root (needs SONARQUBE_TOKEN)")
	}
	if unread, ok := errors.AsType[*fs.PathError](err); ok {
		return nil, fmt.Errorf(".env could not be read: %w", unread)
	}
	if err != nil {
		return nil, errors.New(".env could not be parsed: a line is not NAME=value or a quoted value does not end (the file is not shown, since it holds secrets)")
	}
	set := make([]string, 0, len(values))
	for variable, value := range values {
		set = append(set, variable+"="+value)
	}
	slices.Sort(set)
	return set, nil
}

// readEnvironment reads the settings that come from the environment alone.
func (s *session) readEnvironment(read *settings) error {
	variable := func(name, fallback string) string {
		value, _ := recipe.Getenv(s.environ, name)
		return cmp.Or(value, fallback)
	}
	read.host = variable("SONAR_HOST_URL", "https://sonarcloud.io")
	read.coverageMin = variable("COVERAGE_MIN", "90")
	// ParseFloat reads NaN and the infinities, and no total is below NaN.
	if minimum, err := strconv.ParseFloat(read.coverageMin, 64); err != nil || math.IsNaN(minimum) || math.IsInf(minimum, 0) {
		return fmt.Errorf("COVERAGE_MIN=%s is not a finite number", read.coverageMin)
	}
	for _, wait := range []struct {
		name, fallback string
		into           *time.Duration
	}{{"SONAR_POLL_TIMEOUT", "240", &read.pollTimeout}, {"SONAR_POLL_INTERVAL", "5", &read.pollInterval}} {
		given := variable(wait.name, wait.fallback)
		seconds, err := strconv.ParseUint(given, 10, 31)
		if err != nil {
			return fmt.Errorf("%s=%s is not a whole number of seconds", wait.name, given)
		}
		*wait.into = time.Duration(seconds) * time.Second
	}
	return nil
}

// gitOutput runs git in dir and returns what it wrote to its standard output.
func (s *session) gitOutput(ctx context.Context, git, dir string, args ...string) (string, error) {
	out, err := recipe.Output(ctx, s.run, recipe.Call{Program: git, Args: args, Dir: dir, Env: s.environ, Stderr: s.stderr})
	return strings.TrimSpace(out), err
}
