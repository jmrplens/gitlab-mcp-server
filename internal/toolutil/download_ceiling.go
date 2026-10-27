// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"errors"
	"fmt"
	"io"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// MaxDownloadBytes is the largest body a download action returns in one MCP
// response: a project or group export archive, a dependency list export, a
// job artifact read as a whole.
//
// It bounds what the download becomes rather than what it costs to read: the
// body is buffered, base64-encoded a third larger again, and copied into a
// JSON-RPC message, so one call carries several times the body itself and
// nothing else caps that chain. The client-wide response ceiling caps the
// read alone (RQB-009), and a caller can start as many downloads as it
// likes.
//
// A body above the ceiling is refused rather than truncated: a prefix of an
// archive cannot be imported or opened, and a prefix of a JSON document is
// not valid JSON, so a partial body answers the request wrongly rather than
// partially.
const MaxDownloadBytes = 32 << 20

// ReadDownload reads a download's body and returns it when it holds at most
// MaxDownloadBytes, reading one byte past the ceiling and no more to tell. A
// longer body is refused with hint, the way out the action names (usually
// the GitLab route to download from directly); so is a read a response
// ceiling stopped ([IsDownloadTooLarge]), and any other failed read is
// reported as one.
func ReadDownload(op string, r io.Reader, hint string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxDownloadBytes+1))
	if err != nil {
		if IsDownloadTooLarge(err) {
			return nil, WrapErrWithHint(op, err, hint)
		}
		return nil, WrapErrWithMessage(op, fmt.Errorf("reading the download: %w", err))
	}
	if len(data) > MaxDownloadBytes {
		return nil, WrapErrWithHint(op,
			fmt.Errorf("the download exceeds the %d MiB one response carries", MaxDownloadBytes>>20), hint)
	}
	return data, nil
}

// IsDownloadTooLarge reports whether err is a response ceiling refusing a
// body (gitlabclient.ErrResponseTooLarge): the client-wide one, or the one an
// action set on its request with gitlabclient.WithResponseLimit. A handler
// asks it of an SDK call's error so that a body the read stopped gets the
// same way out as one the action's own ceiling refused, rather than a bare
// "unexpected error".
func IsDownloadTooLarge(err error) bool {
	return errors.Is(err, gitlabclient.ErrResponseTooLarge)
}
