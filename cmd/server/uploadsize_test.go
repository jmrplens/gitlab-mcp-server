package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestUploadMaxFileSize_HTTPModeHonorsTheSetting is the regression for a flag
// that worked on one transport and silently did nothing on the other.
//
// --upload-max-file-size writes UPLOAD_MAX_FILE_SIZE into the environment,
// which config.Load reads. HTTP mode does not go through Load: it builds its
// configuration from flags, and that path hardcoded the default. So an operator
// raising the limit saw it applied over stdio and ignored over HTTP, with
// nothing anywhere saying which they had got.
func TestUploadMaxFileSize_HTTPModeHonorsTheSetting(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  int64
	}{
		{name: "unset falls back to the default", want: config.DefaultMaxFileSize},
		{name: "a plain byte count", set: true, value: "1048576", want: 1048576},
		{name: "a human-friendly size", set: true, value: "50MB", want: 50 * 1024 * 1024},
		{
			name:  "an unparseable value falls back rather than failing the start",
			set:   true,
			value: "not-a-size",
			want:  config.DefaultMaxFileSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("UPLOAD_MAX_FILE_SIZE", tc.value)
			}
			if got := uploadMaxFileSize(); got != tc.want {
				t.Errorf("uploadMaxFileSize() = %d, want %d", got, tc.want)
			}
		})
	}
}
