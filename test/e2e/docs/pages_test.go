// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// pages_test.go holds what the live run reads before it runs anything: the
// pages that mark blocks, the release the site renders, and the docker
// commands that find the GitLab's network and start a reader, the last
// against a stand-in docker so no daemon is needed.
package docse2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/docblocks"
)

// writeTree writes files under a fresh root and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	return root
}

// TestMarkedPages_SiteTree_ListsThePagesThatMarkBlocks pins which files the
// live run reads: English .md and .mdx pages, each with its twin, listed
// when they mark a block and in path order.
func TestMarkedPages_SiteTree_ListsThePagesThatMarkBlocks(t *testing.T) {
	t.Parallel()

	marked := "```bash run\nls\n```\n"
	plain := "```bash\nls\n```\n"
	root := writeTree(t, map[string]string{
		"z.mdx":             marked,
		"es/z.mdx":          marked,
		"ops/a.md":          marked,
		"es/ops/a.md":       marked,
		"plain.mdx":         plain,
		"es/plain.mdx":      plain,
		"image.png":         "not a page",
		"es/only-spanish.x": "not read",
	})
	got, err := markedPages(root)
	if err != nil {
		t.Fatalf("markedPages: %v", err)
	}
	block := docblocks.Block{Lang: "bash", Info: "run", Run: docblocks.RunForeground, Body: "ls\n", Line: 1}
	want := []markedPage{{rel: "ops/a.md", blocks: []docblocks.Block{block}}, {rel: "z.mdx", blocks: []docblocks.Block{block}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("markedPages:\n got %#v\nwant %#v", got, want)
	}
}

// TestMarkedPages_PageUnreadable_FailsTheWalk pins that the live run never
// skips a page it could not read: a page without its twin, a twin marking
// other blocks, and a root that is not there each fail the walk.
func TestMarkedPages_PageUnreadable_FailsTheWalk(t *testing.T) {
	t.Parallel()

	marked := "```bash run\nls\n```\n"
	tests := []struct {
		name  string
		files map[string]string
		root  string
		want  error
		names string
	}{
		{name: "a page with no twin", files: map[string]string{"a.mdx": marked}, want: os.ErrNotExist, names: "a.mdx"},
		{name: "a twin that lost the mark", files: map[string]string{"a.mdx": marked, "es/a.mdx": "```bash\nls\n```\n"}, want: docblocks.ErrRunNotInTwin, names: "a.mdx"},
		{name: "no content root", root: "missing", want: os.ErrNotExist, names: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, tt.files)
			if tt.root != "" {
				root = filepath.Join(root, tt.root)
			}
			got, err := markedPages(root)
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.names) {
				t.Errorf("markedPages = %#v, %v; want an error wrapping %q naming %s", got, err, tt.want, tt.names)
			}
		})
	}
}

// TestMarkedPages_TheSite_ReadsWithEveryTwin runs the walk over the site the
// live run reads, so a marked block whose twin disagrees fails here, offline,
// before a GitLab is booted for it.
func TestMarkedPages_TheSite_ReadsWithEveryTwin(t *testing.T) {
	t.Parallel()

	if _, err := markedPages(filepath.Join(repoRoot(t), "site", "src", "content", "docs")); err != nil {
		t.Fatalf("markedPages over the site: %v", err)
	}
}

// TestReleaseVersion_VersionFile_IsWhatTheSiteRenders pins the value
// %%VERSION%% becomes, read the way the site reads it, and the files that
// cannot give one.
func TestReleaseVersion_VersionFile_IsWhatTheSiteRenders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		want    string
		wantErr string
	}{
		{name: "a release", files: map[string]string{"VERSION": "3.6.0\n"}, want: "3.6.0"},
		{name: "an empty file", files: map[string]string{"VERSION": " \n"}, wantErr: "empty"},
		{name: "no file", files: map[string]string{}, wantErr: "VERSION"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := releaseVersion(writeTree(t, tt.files))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("releaseVersion = %q, %v; want an error naming %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("releaseVersion = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// fakeDocker plants a stand-in docker that appends each command line it is
// given to a log and answers from the environment: FAKE_PS for compose ps,
// FAKE_NETWORKS for inspect, FAKE_FAIL for the subcommand it refuses.
func fakeDocker(t *testing.T) (bin, log string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "docker")
	log = filepath.Join(dir, "calls.log")
	script := `#!/usr/bin/env bash
echo "$*" >>"` + log + `"
if [ "$1" = "${FAKE_FAIL:-}" ]; then echo "refused: $1" >&2; exit 1; fi
case "$1" in
  compose) printf '%s' "${FAKE_PS:-}" ;;
  inspect) printf '%b' "${FAKE_NETWORKS:-}" ;;
  exec) [ "$2" = "--interactive" ] && cat >/dev/null ;;
esac
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //#nosec G306 -- an executable the test runs
		t.Fatalf("planting the stand-in docker: %v", err)
	}
	return bin, log
}

// TestGitlabNetwork_ComposeAnswer_NamesTheNetwork pins how the reader finds
// the GitLab: the network its gitlab container joined, and an error when no
// container runs, it is on no network, or docker refuses.
func TestGitlabNetwork_ComposeAnswer_NamesTheNetwork(t *testing.T) {
	tests := []struct {
		name     string
		ps       string
		networks string
		fail     string
		want     string
		wantErr  string
	}{
		{name: "one network", ps: "abc123\n", networks: "\ne2e_default\n", want: "e2e_default"},
		{name: "no container", ps: "", wantErr: "not running"},
		{name: "no network", ps: "abc123\n", networks: "\n", wantErr: "on no network"},
		{name: "compose refused", fail: "compose", wantErr: "refused: compose"},
		{name: "inspect refused", ps: "abc123\n", fail: "inspect", wantErr: "refused: inspect"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin, _ := fakeDocker(t)
			t.Setenv("FAKE_PS", tt.ps)
			t.Setenv("FAKE_NETWORKS", tt.networks)
			t.Setenv("FAKE_FAIL", tt.fail)
			got, err := gitlabNetwork(context.Background(), bin, "compose.yml")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("gitlabNetwork = %q, %v; want an error naming %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("gitlabNetwork = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// TestReader_Lifecycle_UsesNoBindMount pins the docker commands a reader is
// built, started, written to, run in and removed with: the binary is copied
// in rather than mounted, so a daemon on another machine serves the run.
func TestReader_Lifecycle_UsesNoBindMount(t *testing.T) {
	bin, log := fakeDocker(t)
	t.Setenv("FAKE_FAIL", "")
	ctx := context.Background()
	image, err := buildReader(ctx, bin)
	if err != nil || image != readerImage {
		t.Fatalf("buildReader = %q, %v", image, err)
	}
	rd, err := startReader(ctx, bin, image, "e2e_default", "reader-1", "/dist/gitlab-mcp-server", readerAsset)
	if err != nil {
		t.Fatalf("startReader: %v", err)
	}
	if writeErr := rd.write(ctx, ".docs-runner/block-3.sh", []byte("ls\n")); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	if _, runErr := rd.run(ctx, "true"); runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if removeErr := rd.remove(ctx); removeErr != nil {
		t.Fatalf("remove: %v", removeErr)
	}
	raw, err := os.ReadFile(log) //#nosec G304 -- the stand-in's log, under the test's temporary directory
	if err != nil {
		t.Fatalf("reading the stand-in's log: %v", err)
	}
	want := strings.Join([]string{
		"build --quiet --tag " + readerImage + " -",
		"run --detach --init --name reader-1 --network e2e_default --workdir /root " + readerImage + " sleep infinity",
		"cp /dist/gitlab-mcp-server reader-1:/usr/local/bin/gitlab-mcp-server",
		"cp /dist/gitlab-mcp-server reader-1:/root/gitlab-mcp-server-linux-amd64",
		"exec reader-1 chmod 0644 /root/gitlab-mcp-server-linux-amd64",
		`exec --interactive --workdir /root reader-1 bash -c mkdir -p "$(dirname "$1")" && cat >"$1" bash .docs-runner/block-3.sh`,
		"exec --workdir /root reader-1 bash -c true",
		"rm --force reader-1",
	}, "\n") + "\n"
	if got := string(raw); got != want {
		t.Errorf("docker commands:\n got %s\nwant %s", got, want)
	}
}

// TestStartReader_StepRefused_ReturnsTheReaderToRemove pins that a reader
// that started and then could not be set up is handed back, so the caller
// removes it rather than leaving a container behind.
func TestStartReader_StepRefused_ReturnsTheReaderToRemove(t *testing.T) {
	tests := []struct {
		name       string
		fail       string
		wantReader bool
	}{
		{name: "the start refused", fail: "run", wantReader: false},
		{name: "the copy refused", fail: "cp", wantReader: true},
		{name: "the build refused", fail: "build"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin, _ := fakeDocker(t)
			t.Setenv("FAKE_FAIL", tt.fail)
			if tt.fail == "build" {
				if _, err := buildReader(context.Background(), bin); err == nil || !strings.Contains(err.Error(), "refused: build") {
					t.Errorf("buildReader = %v; want the refusal", err)
				}
				return
			}
			rd, err := startReader(context.Background(), bin, readerImage, "net", "reader-2", "/bin/x", "asset")
			if err == nil || !strings.Contains(err.Error(), "refused: "+tt.fail) {
				t.Fatalf("startReader = %v; want the refusal", err)
			}
			if (rd != nil) != tt.wantReader {
				t.Errorf("startReader returned a reader: %v, want %v", rd != nil, tt.wantReader)
			}
		})
	}
}
