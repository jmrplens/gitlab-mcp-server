// markdown_test.go contains unit tests for model registry Markdown
// formatting functions.
package modelregistry

import (
	"testing"
)

// TestFormatDownloadMarkdown validates Markdown rendering of a downloaded
// ML model package file.
//
// Each case pins the whole response rather than a substring: the card's rows,
// the note and the guidance section are one document, and a substring
// assertion is what let a row survive in a table another row had already
// closed. The cases cover a populated download, a response with nothing in it
// but a size, and a large file.
func TestFormatDownloadMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input DownloadOutput
		want  string
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
			want: "## ML Model Package: classifier.bin\n\n" +
				"- **Project**: 42\n" +
				"- **Model Version**: 7\n" +
				"- **Path**: models/v1\n" +
				"- **Filename**: classifier.bin\n" +
				"- **Size**: 1024 bytes\n\n" +
				"_Content is base64-encoded in the structured JSON output._\n\n" +
				"---\n💡 **Next steps:**\n" +
				"- Use `gitlab_package_list` to browse available model packages\n",
		},
		{
			name: "empty fields render the resource heading and the size alone",
			input: DownloadOutput{
				ProjectID:      "",
				ModelVersionID: "",
				Path:           "",
				Filename:       "",
				SizeBytes:      0,
			},
			want: "## ML Model Package\n\n" +
				"- **Size**: 0 bytes\n\n" +
				"_Content is base64-encoded in the structured JSON output._\n\n" +
				"---\n💡 **Next steps:**\n" +
				"- Use `gitlab_package_list` to browse available model packages\n",
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
			want: "## ML Model Package: weights.h5\n\n" +
				"- **Project**: group/project\n" +
				"- **Model Version**: candidate:5\n" +
				"- **Path**: deep/nested/path\n" +
				"- **Filename**: weights.h5\n" +
				"- **Size**: 104857600 bytes\n\n" +
				"_Content is base64-encoded in the structured JSON output._\n\n" +
				"---\n💡 **Next steps:**\n" +
				"- Use `gitlab_package_list` to browse available model packages\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDownloadMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatDownloadMarkdown() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
