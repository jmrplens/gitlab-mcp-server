package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// TestWarnVersionRefused_EachSentence_GivesItsOneWarning verifies the line a
// stdio start writes when GitLab refused its token the instance version: one
// warning, the missing-permission line naming what GitLab listed as a count
// and the names, the not-yet-supported line when fine-grained tokens are
// disabled for the user (no permission is missing then), and the
// missing-permission line with nothing named for a sentence this server cannot
// read. A sentence carrying control characters is named only as the door's
// log line names it, filtered and cut, so the instance's text cannot forge a
// second line.
func TestWarnVersionRefused_EachSentence_GivesItsOneWarning(t *testing.T) {
	tests := []struct {
		name      string
		sentence  string
		want      string
		wantCount float64
		wantNamed string
	}{
		{
			name:      "Metadata: Read missing",
			sentence:  "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata: Read].",
			want:      versionRefusedMessage,
			wantCount: 1,
			wantNamed: "Metadata: Read",
		},
		{
			name:     "fine-grained tokens not yet supported",
			sentence: "Access denied: Fine-grained personal access tokens are not yet supported.",
			want:     fineGrainedDisabledMessage,
		},
		{
			name:     "a sentence this server cannot read",
			sentence: "Access denied, and that is all.",
			want:     versionRefusedMessage,
		},
		{
			name:      "a sentence with control characters",
			sentence:  "Access denied: This operation requires a fine-grained personal access token with the following instance permissions: [Metadata:\nRead].",
			want:      versionRefusedMessage,
			wantCount: 1,
			wantNamed: "Metadata: Read",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			t.Cleanup(func() { slog.SetDefault(previous) })
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))

			warnVersionRefused(t.Context(), tt.sentence)

			lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
			if len(lines) != 1 {
				t.Fatalf("logged %d lines, want exactly one:\n%s", len(lines), logged.String())
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
				t.Fatalf("decoding the log line: %v", err)
			}
			if record["level"] != "WARN" || record["msg"] != tt.want {
				t.Errorf("logged %v %q, want WARN %q", record["level"], record["msg"], tt.want)
			}
			if tt.want == fineGrainedDisabledMessage {
				if _, named := record["permissions"]; named {
					t.Errorf("the disabled line names permissions: %v", record)
				}
				return
			}
			if record["permissions"] != tt.wantCount || record["named"] != tt.wantNamed {
				t.Errorf("permissions = %v, named = %q; want %v and %q", record["permissions"], record["named"], tt.wantCount, tt.wantNamed)
			}
		})
	}
}
