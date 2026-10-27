// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// download_ceiling_test.go holds the shared download ceiling to its two
// promises: a body that fits is returned whole, and one that does not, or
// that a response ceiling stopped on the way in, is refused with the way out
// rather than handed back as a prefix.
package toolutil

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// downloadHint is the way out the tests hand the helper.
const downloadHint = "download it from GitLab directly (GET /x/download)"

// erroringReader fails every read with err.
type erroringReader struct{ err error }

// Read implements io.Reader by failing.
func (r erroringReader) Read([]byte) (int, error) { return 0, r.err }

// TestReadDownload_BodyAtTheCeiling_IsReturnedWhole verifies that a body of
// up to MaxDownloadBytes comes back byte for byte, the case exactly at the
// ceiling included, which an off-by-one in the limit reader would refuse.
func TestReadDownload_BodyAtTheCeiling_IsReturnedWhole(t *testing.T) {
	for _, size := range []int{0, 1024, MaxDownloadBytes} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			body := bytes.Repeat([]byte{'x'}, size)
			got, err := ReadDownload("op", bytes.NewReader(body), downloadHint)
			if err != nil {
				t.Fatalf("ReadDownload(%d bytes) error = %v", size, err)
			}
			if !bytes.Equal(got, body) {
				t.Errorf("ReadDownload(%d bytes) returned %d bytes, want the whole body", size, len(got))
			}
		})
	}
}

// TestReadDownload_BodyOverTheCeiling_IsRefusedWithTheWayOut verifies that a
// body one byte over the ceiling is refused, never truncated, and that the
// refusal names the ceiling and the way out the caller gave.
func TestReadDownload_BodyOverTheCeiling_IsRefusedWithTheWayOut(t *testing.T) {
	got, err := ReadDownload("op", bytes.NewReader(make([]byte, MaxDownloadBytes+1)), downloadHint)
	if err == nil {
		t.Fatalf("ReadDownload(ceiling+1) returned %d bytes, want a refusal", len(got))
	}
	for _, want := range []string{"op", "32 MiB", downloadHint} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal %q does not mention %q", err, want)
			}
		})
	}
}

// TestReadDownload_ReadFails_ReportsWhy verifies that a read a response
// ceiling stopped carries the way out, as the action's own ceiling does,
// and that any other failed read is reported as one without it.
func TestReadDownload_ReadFails_ReportsWhy(t *testing.T) {
	_, err := ReadDownload("op", erroringReader{fmt.Errorf("body: %w", gitlabclient.ErrResponseTooLarge)}, downloadHint)
	if err == nil || !strings.Contains(err.Error(), downloadHint) || !errors.Is(err, gitlabclient.ErrResponseTooLarge) {
		t.Errorf("ceiling stop = %v, want it wrapped with the way out", err)
	}
	_, err = ReadDownload("op", erroringReader{errors.New("connection reset")}, downloadHint)
	if err == nil || strings.Contains(err.Error(), downloadHint) || !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("failed read = %v, want the read's own error without the way out", err)
	}
}

// TestIsDownloadTooLarge_RecognizesOnlyAResponseCeiling verifies the
// predicate a handler asks of an SDK call's error.
func TestIsDownloadTooLarge_RecognizesOnlyAResponseCeiling(t *testing.T) {
	if !IsDownloadTooLarge(fmt.Errorf("wrapped: %w", gitlabclient.ErrResponseTooLarge)) {
		t.Error("IsDownloadTooLarge(wrapped ErrResponseTooLarge) = false, want true")
	}
	if IsDownloadTooLarge(errors.New("404 Not Found")) || IsDownloadTooLarge(nil) {
		t.Error("IsDownloadTooLarge recognized an error that is not a response ceiling")
	}
}
