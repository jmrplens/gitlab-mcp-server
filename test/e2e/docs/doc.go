// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package docse2e runs the shell blocks the documentation marks as runnable,
// as a reader would type them, against the GitLab the end-to-end suite boots
// in Docker, so a command that stops working fails a check instead of a
// reader (issue 1111).
//
// A page marks a block with a fence option that
// internal/testutil/docblocks reads: `run` for a command that ends, held to
// a zero exit status, and `run=background` for one that keeps running, a
// server, started and left running while the page's later blocks run:
//
//	```bash run=background
//	gitlab-mcp-server --http --http-addr=127.0.0.1:8080 --gitlab-url=https://gitlab.example.com
//	```
//
// The Spanish twin marks the same blocks, which docblocks checks, so a reader
// of either language follows the steps the runner took.
//
// Each page runs in a reader of its own: a disposable container on the e2e
// GitLab's network, with bash, curl, jq, sudo and openssl, the tree's server
// binary on its PATH and, in the home directory, the release asset a reader
// downloads (not executable, as a download is not). It also holds what a page
// names as a prerequisite rather than a step: a certificate and key for
// mcp.example.com at /etc/ssl/mcp.crt and /etc/ssl/mcp.key, and the
// directory /run/gitlab-mcp for a socket. The marked blocks of a page run in
// page order in that reader, each from its own file with `bash -e`, so a
// failing command ends its block as it would end a script, while a pipeline
// is judged by its last command, as in a reader's terminal.
//
// A block is run as the page prints it, with three substitutions and no
// other edit: %%VERSION%% becomes the release the tree's VERSION file names,
// as the site renders it; https://gitlab.com and https://gitlab.example.com,
// the instances the pages name, become the e2e GitLab as the reader reaches it;
// and the token placeholders glpat-your-token and glpat-xxxx… become the e2e
// user's token, which no failure report prints.
//
// A background block keeps running until the page's next background block
// starts or the page ends, and either must find it still running: a server
// that exits on a flag it refuses fails its block. The first `run` block after
// a background one doubles as its readiness probe and is retried until it
// passes or the readiness window closes; every later block runs once. The
// first failing block ends its page, since the steps after it build on it.
//
// The tests carry the e2e build tag. The live run needs the Docker GitLab and
// runs only from make test-e2e-docs, never on CI; the negative controls,
// which hold the runner to failing on a block that fails, hangs, or a server
// that stops, need neither GitLab nor Docker and run with any go test of the
// package.
package docse2e
