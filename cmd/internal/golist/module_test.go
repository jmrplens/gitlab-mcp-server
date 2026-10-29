// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package golist

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// clientGoModule is a module this repository requires, whose directory the
// module cache holds wherever the suite runs.
const clientGoModule = "gitlab.com/gitlab-org/api/client-go/v3"

// TestModuleDir_ARequiredModule_IsTheDirectoryItsSourceSitsIn asks the real
// toolchain for client-go's directory from this package's own directory, and
// holds the answer to a directory that holds client-go's source: what
// R-ROUTE reads the pinned SDK out of.
func TestModuleDir_ARequiredModule_IsTheDirectoryItsSourceSitsIn(t *testing.T) {
	dir, err := ModuleDir(context.Background(), ".", clientGoModule)
	if err != nil {
		t.Fatalf("ModuleDir(%s) error = %v, want nil", clientGoModule, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "gitlab.go")); statErr != nil {
		t.Errorf("ModuleDir(%s) = %q, which holds no gitlab.go: %v", clientGoModule, dir, statErr)
	}
}

// TestModuleDir_EachWayTheAnswerFails_IsAnError verifies every failure is
// returned rather than read as a directory: a module the build list does not
// know, with what the go command wrote on its standard error; a failure that
// is no exit of the go command, and one that exits having written nothing,
// with the error alone; and a module the go command answers with no
// directory.
func TestModuleDir_EachWayTheAnswerFails_IsAnError(t *testing.T) {
	t.Run("unknown module", func(t *testing.T) {
		_, err := ModuleDir(context.Background(), ".", "example.com/no/such/module")
		if err == nil || !strings.Contains(err.Error(), "not a known dependency") {
			t.Errorf("ModuleDir of an unknown module = %v, want the go command's reason", err)
		}
	})
	cases := []struct {
		name    string
		out     string
		err     error
		wantErr string
	}{
		{name: "no exit", err: errors.New("boom"), wantErr: "go list -m example.com/m: boom"},
		{name: "exit with nothing written", err: &exec.ExitError{}, wantErr: "go list -m example.com/m: <nil>"},
		{name: "no directory", out: " \n", wantErr: "the module has no directory; run go mod download"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := listModule
			t.Cleanup(func() { listModule = original })
			listModule = func(context.Context, string, string) ([]byte, error) { return []byte(tc.out), tc.err }
			dir, err := ModuleDir(context.Background(), ".", "example.com/m")
			if dir != "" || err == nil || !strings.HasSuffix(err.Error(), tc.wantErr) {
				t.Errorf("ModuleDir = %q, %v; want no directory and an error ending %q", dir, err, tc.wantErr)
			}
		})
	}
}
