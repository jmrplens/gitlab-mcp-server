// markdown_test.go contains unit tests for model registry Markdown
// formatting functions.

package modelregistry

import (
	"strings"
	"testing"
)

// TestFormatDownloadMarkdown validates Markdown rendering of a downloaded
// ML model package file. Covers all output fields (project, model version,
// path, filename, size) and verifies the hints section is appended.
func TestFormatDownloadMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    DownloadOutput
		contains []string
	}{
		{
			name: "all fields populated",
			input: DownloadOutput{
				ProjectID:      "42",
				ModelVersionID: "7",
				Path:           "models/v1",
				Filename:       "classifier.bin",
				ContentBase64:  "bW9kZWwtZGF0YQ==",
				SizeBytes:      1024,
			},
			contains: []string{
				"## ML Model Package: classifier.bin",
				"| Project | 42 |",
				"| Model Version | 7 |",
				"| Path | models/v1 |",
				"| Filename | classifier.bin |",
				"| Size | 1024 bytes |",
				"base64-encoded",
				"gitlab_package_list",
			},
		},
		{
			name: "empty fields render without panic",
			input: DownloadOutput{
				ProjectID:      "",
				ModelVersionID: "",
				Path:           "",
				Filename:       "",
				SizeBytes:      0,
			},
			contains: []string{
				"## ML Model Package:",
				"| Size | 0 bytes |",
				"base64-encoded",
			},
		},
		{
			name: "large file size renders correctly",
			input: DownloadOutput{
				ProjectID:      "group/project",
				ModelVersionID: "candidate:5",
				Path:           "deep/nested/path",
				Filename:       "weights.h5",
				SizeBytes:      104857600,
			},
			contains: []string{
				"## ML Model Package: weights.h5",
				"| Project | group/project |",
				"| Model Version | candidate:5 |",
				"| Path | deep/nested/path |",
				"| Size | 104857600 bytes |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDownloadMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, got)
				}
			}
		})
	}
}
