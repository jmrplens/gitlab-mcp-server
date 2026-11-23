// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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
	values, err := godotenv.Read(filepath.Join(read.root, ".env"))
	if errors.Is(err, os.ErrNotExist) {
		return settings{}, errors.New(".env not found in repo root (needs SONARQUBE_TOKEN)")
	}
	if err != nil {
		return settings{}, fmt.Errorf(".env could not be read: %w", err)
	}
	for name, value := range values {
		read.dotenv = append(read.dotenv, name+"="+value)
	}
	slices.Sort(read.dotenv)
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

// readEnvironment reads the settings that come from the environment alone.
func (s *session) readEnvironment(read *settings) error {
	variable := func(name, fallback string) string {
		value, _ := recipe.Getenv(s.environ, name)
		return cmp.Or(value, fallback)
	}
	read.host = variable("SONAR_HOST_URL", "https://sonarcloud.io")
	read.coverageMin = variable("COVERAGE_MIN", "90")
	if _, err := strconv.ParseFloat(read.coverageMin, 64); err != nil {
		return fmt.Errorf("COVERAGE_MIN=%s is not a number", read.coverageMin)
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
