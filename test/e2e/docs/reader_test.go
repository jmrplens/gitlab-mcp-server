// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// reader_test.go is the reader of the live run: a disposable container on the
// e2e GitLab's Docker network, built from readerDockerfile, holding the
// tree's server binary and what the pages take as given.
//
// Everything goes through the docker command, as the rest of test/e2e does,
// and nothing is bind-mounted: the binary is copied in, and a block's text is
// written through docker exec, so the daemon can be on another machine
// (DOCKER_HOST), as run-docker-e2e.sh allows.
package docse2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// readerImage is the tag the reader is built under. The build caches its
// layers, so a run after the first rebuilds nothing.
const readerImage = "gitlab-mcp-docs-reader:local"

// readerHome is the reader's home directory and the directory every block
// runs in.
const readerHome = "/root"

// readerDockerfile builds the reader: a Debian with the tools the pages'
// shell blocks call, and what the pages take as a prerequisite rather than
// print as a step, a certificate and key for mcp.example.com where the TLS
// examples read them and the directory the socket examples listen in. It is
// piped to docker build rather than kept as a file, so it needs no build
// context and is not a Dockerfile of the repository's.
const readerDockerfile = `FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends bash ca-certificates curl jq openssl procps sudo \
 && rm -rf /var/lib/apt/lists/*
RUN openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
      -subj /CN=mcp.example.com \
      -addext subjectAltName=DNS:mcp.example.com,DNS:localhost,IP:127.0.0.1 \
      -keyout /etc/ssl/mcp.key -out /etc/ssl/mcp.crt \
 && mkdir -p /run/gitlab-mcp
WORKDIR /root
`

// docker runs the docker command with args and returns what it printed to
// standard output; a failure carries what it printed to standard error.
func docker(ctx context.Context, bin string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// buildReader builds the reader image and returns its tag.
func buildReader(ctx context.Context, bin string) (string, error) {
	if _, err := docker(ctx, bin, []byte(readerDockerfile), "build", "--quiet", "--tag", readerImage, "-"); err != nil {
		return "", err
	}
	return readerImage, nil
}

// gitlabNetwork returns the Docker network the e2e GitLab of compose file
// runs on, the one its gitlab service's container joined first.
func gitlabNetwork(ctx context.Context, bin, compose string) (string, error) {
	id, err := docker(ctx, bin, nil, "compose", "-f", compose, "ps", "-q", "gitlab")
	if err != nil {
		return "", err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("the e2e GitLab is not running: docker compose lists no container for its gitlab service")
	}
	out, err := docker(ctx, bin, nil, "inspect", "--format", "{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}", id)
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(out) {
		if name := strings.TrimSpace(line); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("the e2e GitLab's container %s is on no network", id)
}

// reader is a running reader container, the shell of the live run.
type reader struct {
	bin  string
	name string
}

// startReader starts a reader named name from image on network, and copies
// the server binary in twice: onto the PATH, as an install leaves it, and
// into the home directory as asset, the file a reader downloads from a
// release, without the executable bit a download does not carry.
func startReader(ctx context.Context, bin, image, network, name, binary, asset string) (*reader, error) {
	if _, err := docker(ctx, bin, nil, "run", "--detach", "--init", "--name", name, "--network", network, "--workdir", readerHome, image, "sleep", "infinity"); err != nil {
		return nil, err
	}
	r := &reader{bin: bin, name: name}
	steps := [][]string{
		{"cp", binary, name + ":/usr/local/bin/gitlab-mcp-server"},
		{"cp", binary, name + ":" + readerHome + "/" + asset},
		{"exec", name, "chmod", "0644", readerHome + "/" + asset},
	}
	for _, step := range steps {
		if _, err := docker(ctx, bin, nil, step...); err != nil {
			return r, err
		}
	}
	return r, nil
}

// write puts content at name under the reader's home, through docker exec.
func (r *reader) write(ctx context.Context, name string, content []byte) error {
	_, err := docker(ctx, r.bin, content, "exec", "--interactive", "--workdir", readerHome, r.name,
		"bash", "-c", `mkdir -p "$(dirname "$1")" && cat >"$1"`, "bash", name)
	return err
}

// run runs script with bash in the reader's home and returns its standard
// output and standard error together. The error of a script that ran and
// failed is the docker command's, whose exit status is the script's.
func (r *reader) run(ctx context.Context, script string) (string, error) {
	//#nosec G204 -- the script is the runner's own, run in the disposable reader the test started
	cmd := exec.CommandContext(ctx, r.bin, "exec", "--workdir", readerHome, r.name, "bash", "-c", script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// remove removes the reader and everything it ran.
func (r *reader) remove(ctx context.Context) error {
	_, err := docker(ctx, r.bin, nil, "rm", "--force", r.name)
	return err
}
