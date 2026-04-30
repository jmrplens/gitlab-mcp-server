// text_test.go contains table-driven tests for the NormalizeText function,
// verifying that literal escape sequences are converted to real characters.
package toolutil

import "testing"

// TestNormalizeText uses table-driven subtests to verify NormalizeText handles
// literal backslash-n, backslash-t, mixed escapes, no-op inputs, and empty strings.
func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "literal backslash-n becomes newline",
			in:   `line1\nline2\nline3`,
			want: "line1\nline2\nline3",
		},
		{
			name: "literal backslash-t becomes tab",
			in:   `col1\tcol2`,
			want: "col1\tcol2",
		},
		{
			name: "mixed escapes",
			in:   `## Title\n\n- item1\n- item2\n\t- subitem`,
			want: "## Title\n\n- item1\n- item2\n\t- subitem",
		},
		{
			name: "no escapes unchanged",
			in:   "already fine",
			want: "already fine",
		},
		{
			name: "real newlines preserved",
			in:   "real\nnewline",
			want: "real\nnewline",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "literal backslash-r-n becomes newline",
			in:   `line1\r\nline2`,
			want: "line1\nline2",
		},
		{
			name: "literal backslash-r becomes newline",
			in:   `line1\rline2`,
			want: "line1\nline2",
		},
		{
			name: "double backslash becomes single backslash",
			in:   `a\\-b`,
			want: `a\-b`,
		},
		{
			name: "double-escaped newline cascades to real newline",
			in:   `prefix\\nsuffix`,
			want: "prefix\nsuffix",
		},
		{
			name: "double-escaped tab cascades to real tab",
			in:   `col\\tcol2`,
			want: "col\tcol2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeText(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestEscapeMdTableCell verifies that pipe characters and newlines
// are escaped so they do not break Markdown table structure.
func TestEscapeMdTableCell(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty string", in: "", want: ""},
		{name: "no special chars", in: "hello world", want: "hello world"},
		{name: "pipe in middle", in: "foo|bar", want: "foo&#124;bar"},
		{name: "multiple pipes", in: "a|b|c", want: "a&#124;b&#124;c"},
		{name: "newline replaced with space", in: "line1\nline2", want: "line1 line2"},
		{name: "carriage return replaced with space", in: "line1\rline2", want: "line1 line2"},
		{name: "CRLF replaced with single space", in: "line1\r\nline2", want: "line1 line2"},
		{name: "combined pipe and newline", in: "a|b\nc", want: "a&#124;b c"},
		{name: "already escaped pipe unchanged", in: `foo\|bar`, want: `foo\&#124;bar`},
		{name: "leading pipe", in: "|start", want: "&#124;start"},
		{name: "trailing pipe", in: "end|", want: "end&#124;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeMdTableCell(tt.in)
			if got != tt.want {
				t.Errorf("EscapeMdTableCell(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestWrapGFMBody verifies that user-generated GFM content is wrapped in
// blockquotes to prevent heading hierarchy conflicts.
func TestWrapGFMBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty body", in: "", want: ""},
		{name: "single line", in: "hello world", want: "> hello world"},
		{name: "multiline", in: "line1\nline2\nline3", want: "> line1\n> line2\n> line3"},
		{name: "body with heading", in: "## Sub-heading\ntext", want: "> ## Sub-heading\n> text"},
		{name: "body with empty lines", in: "para1\n\npara2", want: "> para1\n>\n> para2"},
		{name: "body with pipe", in: "a | b", want: "> a | b"},
		{name: "body with code block", in: "```go\nfmt.Println()\n```", want: "> ```go\n> fmt.Println()\n> ```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapGFMBody(tt.in)
			if got != tt.want {
				t.Errorf("WrapGFMBody(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDetectRichContent verifies detection of non-portable GFM features.
func TestDetectRichContent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty string", in: "", want: ""},
		{name: "plain text", in: "Just some text", want: ""},
		{name: "mermaid block", in: "text\n```mermaid\ngraph TD\n```\n", want: "mermaid"},
		{name: "math block", in: "The formula is $$E=mc^2$$", want: "math"},
		{name: "HTML details", in: "<details><summary>More</summary>content</details>", want: "HTML"},
		{name: "HTML table", in: "<table><tr><td>cell</td></tr></table>", want: "HTML"},
		{name: "HTML img", in: "See <img src=\"pic.png\"/>", want: "HTML"},
		{name: "multiple features", in: "```mermaid\ngraph\n```\n$$x$$\n<details>d</details>", want: "mermaid, math, HTML"},
		{name: "mermaid and math", in: "```mermaid\nA\n```\n$$y$$", want: "mermaid, math"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectRichContent(tt.in)
			if got != tt.want {
				t.Errorf("DetectRichContent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRichContentHint verifies the informational note for non-portable content.
func TestRichContentHint(t *testing.T) {
	tests := []struct {
		name     string
		features string
		webURL   string
		want     string
	}{
		{name: "empty features", features: "", webURL: "https://gitlab.com/p/1", want: ""},
		{name: "empty webURL", features: "mermaid", webURL: "", want: ""},
		{name: "both empty", features: "", webURL: "", want: ""},
		{name: "single feature", features: "mermaid", webURL: "https://gitlab.com/p/1",
			want: "\n> **Contains**: mermaid — [view in GitLab](https://gitlab.com/p/1) for full rendering.\n"},
		{name: "multiple features", features: "mermaid, math, HTML", webURL: "https://gitlab.com/p/2",
			want: "\n> **Contains**: mermaid, math, HTML — [view in GitLab](https://gitlab.com/p/2) for full rendering.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RichContentHint(tt.features, tt.webURL)
			if got != tt.want {
				t.Errorf("RichContentHint(%q, %q) = %q, want %q", tt.features, tt.webURL, got, tt.want)
			}
		})
	}
}

// TestEscapeMdHeading verifies heading injection prevention.
func TestEscapeMdHeading(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "normal text", input: "My Project", want: "My Project"},
		{name: "leading hash", input: "# injected heading", want: "injected heading"},
		{name: "leading double hash", input: "## sub-heading", want: "sub-heading"},
		{name: "leading triple hash with space", input: "### deep heading", want: "deep heading"},
		{name: "hash in middle preserved", input: "Issue #42 title", want: "Issue #42 title"},
		{name: "newline replaced", input: "first\nsecond", want: "first second"},
		{name: "CRLF replaced", input: "first\r\nsecond", want: "first second"},
		{name: "CR replaced", input: "first\rsecond", want: "first second"},
		{name: "combined hash and newline", input: "## injected\nbreak", want: "injected break"},
		{name: "only hashes", input: "###", want: ""},
		{name: "hash space only", input: "# ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeMdHeading(tt.input)
			if got != tt.want {
				t.Errorf("EscapeMdHeading(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsImageFile verifies image file extension detection.
func TestIsImageFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{name: "empty string", filename: "", want: false},
		{name: "no extension", filename: "README", want: false},
		{name: "png", filename: "screenshot.png", want: true},
		{name: "jpg", filename: "photo.jpg", want: true},
		{name: "jpeg", filename: "photo.jpeg", want: true},
		{name: "gif", filename: "anim.gif", want: true},
		{name: "svg", filename: "diagram.svg", want: true},
		{name: "webp", filename: "image.webp", want: true},
		{name: "ico", filename: "favicon.ico", want: true},
		{name: "bmp", filename: "old.bmp", want: true},
		{name: "uppercase PNG", filename: "IMAGE.PNG", want: true},
		{name: "mixed case Jpg", filename: "Photo.Jpg", want: true},
		{name: "txt file", filename: "notes.txt", want: false},
		{name: "go file", filename: "main.go", want: false},
		{name: "md file", filename: "README.md", want: false},
		{name: "pdf file", filename: "doc.pdf", want: false},
		{name: "dotfile", filename: ".gitignore", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsImageFile(tt.filename)
			if got != tt.want {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

// TestTitleFromName verifies that TitleFromName converts snake_case MCP tool
// names into human-readable titles.
func TestTitleFromName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple list", in: "gitlab_list_projects", want: "List Projects"},
		{name: "single word", in: "gitlab_search", want: "Search"},
		{name: "acronym MR", in: "gitlab_mr_get", want: "MR Get"},
		{name: "acronym CI", in: "gitlab_ci_lint", want: "CI Lint"},
		{name: "acronym SSH", in: "gitlab_ssh_key_get", want: "SSH Key Get"},
		{name: "no prefix", in: "list_projects", want: "List Projects"},
		{name: "long name", in: "gitlab_create_merge_request", want: "Create Merge Request"},
		{name: "meta tool", in: "gitlab_project", want: "Project"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TitleFromName(tt.in)
			if got != tt.want {
				t.Errorf("TitleFromName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestMdTitleLink verifies that MdTitleLink returns a Markdown link when
// url is non-empty, and the escaped title when url is empty. Pipes and
// newlines in the title are escaped via EscapeMdTableCell.
func TestMdTitleLink(t *testing.T) {
	tests := []struct {
		name  string
		title string
		url   string
		want  string
	}{
		{
			name:  "title with URL",
			title: "My Project",
			url:   "https://gitlab.example.com/project",
			want:  "[My Project](https://gitlab.example.com/project)",
		},
		{
			name:  "title without URL",
			title: "My Project",
			url:   "",
			want:  "My Project",
		},
		{
			name:  "title with pipe and URL",
			title: "a|b",
			url:   "https://example.com",
			want:  "[a&#124;b](https://example.com)",
		},
		{
			name:  "title with newline and URL",
			title: "line1\nline2",
			url:   "https://example.com",
			want:  "[line1 line2](https://example.com)",
		},
		{
			name:  "empty title with URL",
			title: "",
			url:   "https://example.com",
			want:  "[](https://example.com)",
		},
		{
			name:  "empty title and URL",
			title: "",
			url:   "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MdTitleLink(tt.title, tt.url)
			if got != tt.want {
				t.Errorf("MdTitleLink(%q, %q) = %q, want %q", tt.title, tt.url, got, tt.want)
			}
		})
	}
}

// TestImageMIMEType uses table-driven subtests to verify that ImageMIMEType maps common image extensions to the correct MIME types and returns an empty string for non-image files.
func TestImageMIMEType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "png", filename: "logo.png", want: "image/png"},
		{name: "jpg", filename: "photo.jpg", want: "image/jpeg"},
		{name: "jpeg", filename: "photo.jpeg", want: "image/jpeg"},
		{name: "gif", filename: "anim.gif", want: "image/gif"},
		{name: "webp", filename: "pic.webp", want: "image/webp"},
		{name: "svg", filename: "diagram.svg", want: "image/svg+xml"},
		{name: "ico", filename: "favicon.ico", want: "image/x-icon"},
		{name: "bmp", filename: "old.bmp", want: "image/bmp"},
		{name: "uppercase", filename: "IMAGE.PNG", want: "image/png"},
		{name: "txt", filename: "notes.txt", want: ""},
		{name: "pdf", filename: "doc.pdf", want: ""},
		{name: "empty", filename: "", want: ""},
		{name: "no ext", filename: "README", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImageMIMEType(tt.filename)
			if got != tt.want {
				t.Errorf("ImageMIMEType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

// TestIsBinaryFile uses table-driven subtests to verify that IsBinaryFile detects known binary extensions and returns false for text files.
func TestIsBinaryFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{name: "pdf", filename: "doc.pdf", want: true},
		{name: "zip", filename: "archive.zip", want: true},
		{name: "exe", filename: "app.exe", want: true},
		{name: "dll", filename: "lib.dll", want: true},
		{name: "so", filename: "lib.so", want: true},
		{name: "woff2", filename: "font.woff2", want: true},
		{name: "sqlite", filename: "data.sqlite", want: true},
		{name: "pyc", filename: "module.pyc", want: true},
		{name: "uppercase", filename: "ARCHIVE.ZIP", want: true},
		{name: "text", filename: "readme.txt", want: false},
		{name: "go", filename: "main.go", want: false},
		{name: "image png", filename: "logo.png", want: false},
		{name: "empty", filename: "", want: false},
		{name: "no ext", filename: "Makefile", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBinaryFile(tt.filename)
			if got != tt.want {
				t.Errorf("IsBinaryFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

// TestBuildTargetURL verifies URL construction for various target types
// including Issue, MergeRequest, Milestone, and edge cases (unknown type,
// empty URL, zero IID).
func TestBuildTargetURL(t *testing.T) {
	tests := []struct {
		name       string
		projectURL string
		targetType string
		targetIID  int64
		want       string
	}{
		{"issue", "https://gitlab.com/g/p", "Issue", 42, "https://gitlab.com/g/p/-/issues/42"},
		{"merge request", "https://gitlab.com/g/p", "MergeRequest", 10, "https://gitlab.com/g/p/-/merge_requests/10"},
		{"milestone", "https://gitlab.com/g/p", "Milestone", 3, "https://gitlab.com/g/p/-/milestones/3"},
		{"unknown type", "https://gitlab.com/g/p", "Tag", 1, ""},
		{"empty project URL", "", "Issue", 42, ""},
		{"zero IID", "https://gitlab.com/g/p", "Issue", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTargetURL(tt.projectURL, tt.targetType, tt.targetIID)
			if got != tt.want {
				t.Errorf("BuildTargetURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatTarget verifies Markdown cell rendering for target resources,
// including combinations of title/URL presence and the fallback to type+IID.
func TestFormatTarget(t *testing.T) {
	tests := []struct {
		name        string
		targetType  string
		targetIID   int64
		targetTitle string
		targetURL   string
		want        string
	}{
		{"with title and URL", "Issue", 42, "Bug fix", "https://example.com/issues/42", "[Bug fix](https://example.com/issues/42)"},
		{"with title no URL", "Issue", 42, "Bug fix", "", "Bug fix"},
		{"no title with IID", "Issue", 42, "", "", "Issue #42"},
		{"no title with IID and URL", "MergeRequest", 10, "", "https://example.com/mr/10", "[MergeRequest #10](https://example.com/mr/10)"},
		{"no title no IID", "Issue", 0, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTarget(tt.targetType, tt.targetIID, tt.targetTitle, tt.targetURL)
			if got != tt.want {
				t.Errorf("FormatTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
