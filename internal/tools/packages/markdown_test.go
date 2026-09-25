// markdown_test.go contains unit tests for the Markdown formatting functions
// in the packages package.
package packages

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The guidance sections the package formatters close with. Each card and
// list pins its whole response below, so these are named once rather than
// repeated in every expectation.
const (
	publishHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'publish_and_link' to also create a release asset link in one step\n" +
		"- Use action 'publish_directory' to batch-upload all files from a directory\n" +
		"- Use action 'list' to see all packages in this project\n"
	downloadHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'file_list' to see all files in this package\n" +
		"- Use action 'list' to browse other packages in the project\n"
	publishAndLinkHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'package.publish_directory' to batch-upload a directory instead of repeating this for each file\n" +
		"- Use action 'release.get' to verify the release links\n"
	publishDirHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'package.publish_and_link' to also create a release asset link for each file\n" +
		"- Use action 'release.link_create_batch' to link these packages to a release\n" +
		"- Use action 'package.list' to verify the uploaded packages\n"
	listHints = "\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'file_list' with a package_id to see individual files\n" +
		"- Use action 'delete' to remove a package\n" +
		"- Use action 'publish' or 'publish_directory' to upload new packages\n"
	// The group table's last column is the owning project's path, not a link,
	// so its guidance carries no instruction to preserve links.
	groupListHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'list' to scope packages to a single project\n" +
		"- Use action 'file_list' with a package_id to see individual files\n" +
		"- Use action 'delete' to remove a package\n"
	fileListHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'download' to retrieve a specific file\n" +
		"- Use action 'file_delete' to remove a single file\n"
	packageTableHeader = "| ID | Name | Version | Type | Status | Creator | Pipeline |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n"
	groupTableHeader = "| ID | Name | Version | Type | Status | Creator | Project |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n"
)

// TestFormatPublishMarkdown_WithChecksumAndURL verifies the whole publish card:
// the identifiers, the size with its unit, the digest as a code span and the
// URL linked to itself.
func TestFormatPublishMarkdown_WithChecksumAndURL(t *testing.T) {
	const url = "https://gitlab.example.com/api/v4/projects/1/packages/generic/pkg/1.0.0/app.tar.gz"
	out := PublishOutput{
		PackageFileID: 10,
		PackageID:     20,
		FileName:      "app.tar.gz",
		Size:          4096,
		SHA256:        "0123456789abcdef",
		URL:           url,
	}

	got := FormatPublishMarkdown(out)
	want := "## Package Published\n\n" +
		"- **Package File ID**: 10\n" +
		"- **Package ID**: 20\n" +
		"- **File Name**: app.tar.gz\n" +
		"- **Size**: 4096 bytes\n" +
		"- **SHA256**: `0123456789abcdef`\n" +
		"- **URL**: [" + url + "](" + url + ")\n" +
		publishHints
	if got != want {
		t.Errorf("FormatPublishMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatDownloadMarkdown_WithChecksum verifies the whole download card.
func TestFormatDownloadMarkdown_WithChecksum(t *testing.T) {
	got := FormatDownloadMarkdown(DownloadOutput{OutputPath: "/tmp/app.tar.gz", Size: 2048, SHA256: "abcdef"})
	want := "## Package Downloaded\n\n" +
		"- **Output Path**: /tmp/app.tar.gz\n" +
		"- **Size**: 2048 bytes\n" +
		"- **SHA256**: `abcdef`\n" +
		downloadHints
	if got != want {
		t.Errorf("FormatDownloadMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatPublishAndLinkMarkdown_RendersBothSections verifies the composite
// card keeps both objects visible, each under a section of its own.
func TestFormatPublishAndLinkMarkdown_RendersBothSections(t *testing.T) {
	out := PublishAndLinkOutput{
		Package: PublishOutput{
			PackageFileID: 11,
			FileName:      "app.tar.gz",
			Size:          1024,
			URL:           "https://gitlab.example.com/package/app.tar.gz",
		},
		ReleaseLink: releaselinks.Output{ID: 22, Name: "app.tar.gz", URL: "https://gitlab.example.com/release/app.tar.gz"},
	}

	got := FormatPublishAndLinkMarkdown(out)
	want := "## Package Published & Linked\n\n" +
		"### Package\n\n" +
		"- **Package File ID**: 11\n" +
		"- **File Name**: app.tar.gz\n" +
		"- **Size**: 1024 bytes\n" +
		"- **URL**: [https://gitlab.example.com/package/app.tar.gz](https://gitlab.example.com/package/app.tar.gz)\n\n" +
		"### Release Link\n\n" +
		"- **ID**: 22\n" +
		"- **Name**: app.tar.gz\n" +
		"- **URL**: [https://gitlab.example.com/release/app.tar.gz](https://gitlab.example.com/release/app.tar.gz)\n" +
		publishAndLinkHints
	if got != want {
		t.Errorf("FormatPublishAndLinkMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatPublishDirMarkdown_WithPublishedFiles verifies a run where every
// file succeeded: the heading claims success, the count names both halves, and
// the files are a table under a heading of its own.
func TestFormatPublishDirMarkdown_WithPublishedFiles(t *testing.T) {
	out := PublishDirOutput{
		TotalFiles: 2,
		TotalBytes: 1024,
		Published: []PublishDirItem{
			{FileName: "file1.txt", Size: 512, SHA256: "abcdef1234567890abcdef"},
			{FileName: "file2.txt", Size: 512, SHA256: "short"},
		},
	}
	got := FormatPublishDirMarkdown(out)
	want := "## Directory Published\n\n" +
		"- **Published**: 2 of 2 files\n" +
		"- **Total Bytes**: 1024 bytes\n\n" +
		"### Published Files\n\n" +
		"| File | Size (bytes) | SHA256 |\n" +
		"| --- | --- | --- |\n" +
		"| file1.txt | 512 | `abcdef123456...` |\n" +
		"| file2.txt | 512 | `short` |\n" +
		publishDirHints
	if got != want {
		t.Errorf("FormatPublishDirMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatPublishDirMarkdown_WithErrors verifies that a run where every file
// failed is headed by that outcome rather than by "Directory Published", which
// is what it used to claim above the list of failures.
func TestFormatPublishDirMarkdown_WithErrors(t *testing.T) {
	out := PublishDirOutput{
		TotalBytes: 100,
		Errors:     []string{"upload failed: timeout", "checksum mismatch"},
	}
	got := FormatPublishDirMarkdown(out)
	want := "## Directory Publish Failed\n\n" +
		"- **Published**: 0 of 2 files\n" +
		"- **Total Bytes**: 100 bytes\n\n" +
		"### Errors (2)\n\n" +
		"| Error |\n" +
		"| --- |\n" +
		"| upload failed: timeout |\n" +
		"| checksum mismatch |\n" +
		publishDirHints
	if got != want {
		t.Errorf("FormatPublishDirMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatPublishDirMarkdown_PartialRun verifies a run where some files
// failed is headed as partial, so the heading never contradicts the errors
// under it.
func TestFormatPublishDirMarkdown_PartialRun(t *testing.T) {
	out := PublishDirOutput{
		TotalFiles: 1,
		TotalBytes: 512,
		Published:  []PublishDirItem{{FileName: "ok.txt", Size: 512, SHA256: "short"}},
		Errors:     []string{"bad.txt: upload failed"},
	}
	got := FormatPublishDirMarkdown(out)
	want := "## Directory Partially Published\n\n" +
		"- **Published**: 1 of 2 files\n" +
		"- **Total Bytes**: 512 bytes\n\n" +
		"### Published Files\n\n" +
		"| File | Size (bytes) | SHA256 |\n" +
		"| --- | --- | --- |\n" +
		"| ok.txt | 512 | `short` |\n\n" +
		"### Errors (1)\n\n" +
		"| Error |\n" +
		"| --- |\n" +
		"| bad.txt: upload failed |\n" +
		publishDirHints
	if got != want {
		t.Errorf("FormatPublishDirMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatPublishDirMarkdown_Empty verifies that [FormatPublishDirMarkdown]
// handles zero files and no errors gracefully, opening no table.
func TestFormatPublishDirMarkdown_Empty(t *testing.T) {
	got := FormatPublishDirMarkdown(PublishDirOutput{})
	want := "## Directory Published\n\n" +
		"- **Published**: 0 of 0 files\n" +
		"- **Total Bytes**: 0 bytes\n" +
		publishDirHints
	if got != want {
		t.Errorf("FormatPublishDirMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdown_EmptyPackages verifies an empty list is the one
// sentence and nothing else: no heading counting zero above it.
func TestFormatListMarkdown_EmptyPackages(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Packages: nil, Pagination: toolutil.PaginationOutput{TotalItems: 0}})
	want := "No packages found.\n"
	if got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatListMarkdown_IncludesPreserveLinksHint verifies the whole table:
// the pipeline cell is a link, and the preserve-links hint leads the guidance
// because this table has one.
func TestFormatListMarkdown_IncludesPreserveLinksHint(t *testing.T) {
	out := ListOutput{
		Packages: []ListItem{{
			ID:      1,
			Name:    "pkg",
			Version: "1.0.0",
			Pipeline: &toolutil.PackagePipelineOutput{
				ID:     7,
				Status: "success",
				Ref:    "main",
				WebURL: "https://gitlab.example.com/project/-/pipelines/7",
			},
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	got := FormatListMarkdown(out)
	want := "## Packages (1)\n\n" + packageTableHeader +
		"| 1 | pkg | 1.0.0 |  |  |  | [7 success main](https://gitlab.example.com/project/-/pipelines/7) |\n" +
		"\n1 items total\n" + listHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdown_KeysetPageCountsWhatItShows verifies a page GitLab
// sent no total for is headed by what it shows and says more is available,
// rather than by a zero above a table of rows.
func TestFormatListMarkdown_KeysetPageCountsWhatItShows(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Packages:   []ListItem{{ID: 1, Name: "pkg", Version: "1.0.0"}},
		Pagination: toolutil.PaginationOutput{HasMore: true},
	})
	want := "## Packages (1 shown, more available)\n\n" + packageTableHeader +
		"| 1 | pkg | 1.0.0 |  |  |  |  |\n" + listHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatGroupListMarkdown_EmptyPackages verifies the group package list
// renders the empty-state sentence alone.
func TestFormatGroupListMarkdown_EmptyPackages(t *testing.T) {
	got := FormatGroupListMarkdown(GroupListOutput{Packages: nil, Pagination: toolutil.PaginationOutput{TotalItems: 0}})
	want := "No packages found.\n"
	if got != want {
		t.Errorf("FormatGroupListMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatGroupListMarkdown_RendersProjectAndHint verifies the whole table:
// the owning project path, the project id fallback, and a guidance section
// that does not tell the model to preserve links this table does not carry.
func TestFormatGroupListMarkdown_RendersProjectAndHint(t *testing.T) {
	got := FormatGroupListMarkdown(GroupListOutput{
		Packages: []GroupListItem{
			{ID: 1, Name: "pkg-a", Version: "1.0.0", PackageType: "generic", Status: "default", ProjectID: 7, ProjectPath: "grp/proj"},
			{ID: 2, Name: "pkg-b", Version: "2.0.0", PackageType: "npm", Status: "default", ProjectID: 8},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})
	want := "## Group Packages (2)\n\n" + groupTableHeader +
		"| 1 | pkg-a | 1.0.0 | generic | default |  | grp/proj |\n" +
		"| 2 | pkg-b | 2.0.0 | npm | default |  | 8 |\n" +
		"\n2 items total\n" + groupListHints
	if got != want {
		t.Errorf("FormatGroupListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestGroupProjectSummary_Variants verifies the owning-project summary prefers
// the project path, falls back to the project id, and returns empty otherwise.
func TestGroupProjectSummary_Variants(t *testing.T) {
	tests := []struct {
		name string
		pkg  GroupListItem
		want string
	}{
		{name: "path", pkg: GroupListItem{ProjectID: 7, ProjectPath: "grp/proj"}, want: "grp/proj"},
		{name: "id fallback", pkg: GroupListItem{ProjectID: 7}, want: "7"},
		{name: "none", pkg: GroupListItem{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupProjectSummary(tt.pkg); got != tt.want {
				t.Fatalf("groupProjectSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPipelineSummary_Variants verifies the pipeline cell: nothing when GitLab
// sent no pipeline, the id, status and ref otherwise, linked to the pipeline's
// page when GitLab gave one, and escaped when it did not, since a ref is a
// branch or tag name.
func TestPipelineSummary_Variants(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *toolutil.PackagePipelineOutput
		want     string
	}{
		{name: "none", pipeline: nil, want: ""},
		{name: "text", pipeline: &toolutil.PackagePipelineOutput{ID: 7, Status: "success", Ref: "main"}, want: "7 success main"},
		{name: "link", pipeline: &toolutil.PackagePipelineOutput{ID: 7, Status: "success", Ref: "main", WebURL: "https://gitlab.example.com/pipelines/7"}, want: "[7 success main](https://gitlab.example.com/pipelines/7)"},
		{name: "escaped ref", pipeline: &toolutil.PackagePipelineOutput{ID: 8, Status: "failed", Ref: "a|b"}, want: "8 failed a&#124;b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pipelineSummary(tt.pipeline); got != tt.want {
				t.Fatalf("pipelineSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCreatorSummary_Variants verifies the creator cell names the publishing
// user and stays empty when GitLab attributed the package to no one.
func TestCreatorSummary_Variants(t *testing.T) {
	for _, testCase := range []struct {
		name string
		pkg  ListItem
		want string
	}{
		{name: "with a creator", pkg: ListItem{CreatorID: 57}, want: "57"},
		{name: "without one", pkg: ListItem{}, want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := creatorSummary(testCase.pkg); got != testCase.want {
				t.Errorf("creatorSummary() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestFormatMarkdown_OmitTheChecksumsAndURLsGitLabDidNotSend verifies each
// formatter that names a digest or a URL says nothing at all when the answer
// carried neither, rather than printing an empty row. Each expectation is the
// whole card, which is the only way to be sure the row is absent rather than
// merely spelled differently.
func TestFormatMarkdown_OmitTheChecksumsAndURLsGitLabDidNotSend(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "publish",
			got:  FormatPublishMarkdown(PublishOutput{PackageFileID: 1, FileName: "app.bin"}),
			want: "## Package Published\n\n" +
				"- **Package File ID**: 1\n" +
				"- **Package ID**: 0\n" +
				"- **File Name**: app.bin\n" +
				"- **Size**: 0 bytes\n" +
				publishHints,
		},
		{
			name: "download",
			got:  FormatDownloadMarkdown(DownloadOutput{OutputPath: "/tmp/app.bin"}),
			want: "## Package Downloaded\n\n" +
				"- **Output Path**: /tmp/app.bin\n" +
				"- **Size**: 0 bytes\n" +
				downloadHints,
		},
		{
			name: "publish and link",
			got:  FormatPublishAndLinkMarkdown(PublishAndLinkOutput{}),
			want: "## Package Published & Linked\n\n" +
				"### Package\n\n" +
				"- **Package File ID**: 0\n" +
				"- **Size**: 0 bytes\n\n" +
				"### Release Link\n\n" +
				"- **ID**: 0\n" +
				publishAndLinkHints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s markdown =\n%q\nwant:\n%q", tt.name, tt.got, tt.want)
			}
			if strings.Contains(tt.got, "SHA256") || strings.Contains(tt.got, "**URL**") {
				t.Errorf("%s markdown names a digest or URL for an answer that carried none: %s", tt.name, tt.got)
			}
		})
	}
}

// TestFormatMarkdown_ShortensOnlyAChecksumLongerThanTheColumn verifies both
// checksum columns leave a digest of exactly twelve characters whole and
// shorten a longer one, so the ellipsis always means something was cut.
func TestFormatMarkdown_ShortensOnlyAChecksumLongerThanTheColumn(t *testing.T) {
	const exactly12 = "abcdef123456"
	const longer = "abcdef1234567890"
	t.Run("file list", func(t *testing.T) {
		got := FormatFileListMarkdown(FileListOutput{
			Files: []FileListItem{
				{PackageFileID: 1, FileName: "short.bin", SHA256: exactly12},
				{PackageFileID: 2, FileName: "long.bin", SHA256: longer},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 2},
		})
		want := "## Package Files (2)\n\n" +
			"| ID | File Name | Size (bytes) | SHA256 |\n" +
			"| --- | --- | --- | --- |\n" +
			"| 1 | short.bin | 0 | `" + exactly12 + "` |\n" +
			"| 2 | long.bin | 0 | `abcdef123456...` |\n" +
			"\n2 items total\n" + fileListHints
		if got != want {
			t.Errorf("FormatFileListMarkdown() =\n%q\nwant:\n%q", got, want)
		}
	})
	t.Run("directory publish", func(t *testing.T) {
		got := FormatPublishDirMarkdown(PublishDirOutput{
			TotalFiles: 2,
			Published: []PublishDirItem{
				{FileName: "short.bin", SHA256: exactly12},
				{FileName: "long.bin", SHA256: longer},
			},
		})
		want := "## Directory Published\n\n" +
			"- **Published**: 2 of 2 files\n" +
			"- **Total Bytes**: 0 bytes\n\n" +
			"### Published Files\n\n" +
			"| File | Size (bytes) | SHA256 |\n" +
			"| --- | --- | --- |\n" +
			"| short.bin | 0 | `" + exactly12 + "` |\n" +
			"| long.bin | 0 | `abcdef123456...` |\n" +
			publishDirHints
		if got != want {
			t.Errorf("FormatPublishDirMarkdown() =\n%q\nwant:\n%q", got, want)
		}
	})
}

// TestFormatPackageListMarkdown_SentFields verifies both list tables carry the
// creator column, and name the package's own version alone: a listing is never
// sent the other versions, so the cell has no count of them to give.
func TestFormatPackageListMarkdown_SentFields(t *testing.T) {
	item := ListItem{
		ID: 10, Name: "my-pkg", Version: "1.0.0", PackageType: "generic", Status: "default",
		CreatorID: 57,
	}
	const row = "| 10 | my-pkg | 1.0.0 | generic | default | 57 | "
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "project",
			got: FormatListMarkdown(ListOutput{
				Packages:   []ListItem{item},
				Pagination: toolutil.PaginationOutput{TotalItems: 1},
			}),
			want: "## Packages (1)\n\n" + packageTableHeader + row + " |\n\n1 items total\n" + listHints,
		},
		{
			name: "group",
			got: FormatGroupListMarkdown(GroupListOutput{
				Packages:   []GroupListItem{{ListItem: item, ProjectID: 42, ProjectPath: "grp/proj"}},
				Pagination: toolutil.PaginationOutput{TotalItems: 1},
			}),
			want: "## Group Packages (1)\n\n" + groupTableHeader + row + "grp/proj |\n\n1 items total\n" + groupListHints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s markdown =\n%q\nwant:\n%q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// getHints is the guidance the card of a package of any type but generic
// closes with.
const getHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'package.file_list' to list the files inside this package\n" +
	"- Use action 'package.delete' to delete this version of the package\n"

// genericGetHints is the guidance the card of a generic package closes with:
// the same, and the download package.download can make of its files.
const genericGetHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'package.file_list' to list the files inside this package\n" +
	"- Use action 'package.download' to download one of its files\n" +
	"- Use action 'package.delete' to delete this version of the package\n"

// packageCardJSON is one Conan package as the single-package endpoint renders
// it, timestamps in the millisecond form GitLab writes them in.
const packageCardJSON = `{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"conan","status":"default",` +
	`"conan_package_name":"recipe-name","creator_id":57,` +
	`"created_at":"2026-01-02T03:04:05.678Z","last_downloaded_at":"2026-01-03T04:05:06.789Z",` +
	`"_links":{"web_path":"/grp/proj/-/packages/10","delete_api_path":"/api/v4/projects/42/packages/10"},` +
	`"pipeline":{"id":71,"status":"success","ref":"main","web_url":"https://gitlab.example.com/p/-/pipelines/71"},` +
	`"tags":[{"name":"latest"},{"name":"stable"}],` +
	`"versions":[{"id":9,"version":"0.9.0","created_at":"2026-01-04T05:06:00.5Z",` +
	`"tags":[{"name":"old"},{"name":"lts"}],` +
	`"pipeline":{"id":70,"status":"failed","ref":"release","web_url":"https://gitlab.example.com/p/-/pipelines/70"}},` +
	`{"id":8,"version":"0.8.0"}]}`

// TestFormatGetMarkdown_WholeCard verifies the package card as a caller sees
// it, from GitLab's answer through the handler: its own fields, the pipeline
// that last built it linked to its page, the tags pointing at it, and the
// other versions as a nested table, each with its tags, the pipeline that
// built it and when it was published, and empty cells for what GitLab did not
// send. The card's input used to be written by hand in a form the handler
// never produced, which is how the package's own times reached the card in
// Go's time.String form while this test read them rendered.
func TestFormatGetMarkdown_WholeCard(t *testing.T) {
	out, err := Get(t.Context(), packagesClient(t, packageCardJSON), GetInput{ProjectID: "42", PackageID: "10"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got := FormatGetMarkdown(out)
	want := "## Package: my-pkg\n\n" +
		"- **ID**: 10\n" +
		"- **Version**: 1.0.0\n" +
		"- **Type**: conan\n" +
		"- **Status**: default\n" +
		"- **Conan Package**: recipe-name\n" +
		"- **Creator ID**: 57\n" +
		"- **Created**: 2 Jan 2026 03:04 UTC\n" +
		"- **Last Downloaded**: 3 Jan 2026 04:05 UTC\n" +
		"- **Web Path**: `/grp/proj/-/packages/10`\n" +
		"- **Pipeline**: [71 success main](https://gitlab.example.com/p/-/pipelines/71)\n" +
		"- **Tags**: latest, stable\n" +
		"\n### Other Versions\n\n" +
		"| ID | Version | Tags | Pipeline | Created |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 9 | 0.9.0 | old, lts | [70 failed release](https://gitlab.example.com/p/-/pipelines/70) | 4 Jan 2026 05:06 UTC |\n" +
		"| 8 | 0.8.0 |  |  |  |\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatGetMarkdown_OffersTheDownloadOnlyForAGenericPackage verifies the
// card names package.download for a generic package, whose files that action
// fetches from the Generic Package Registry, and for no other type, where the
// action would ask the generic route for a package it does not hold.
func TestFormatGetMarkdown_OffersTheDownloadOnlyForAGenericPackage(t *testing.T) {
	got := FormatGetMarkdown(GetOutput{Package: DetailItem{ListItem: ListItem{ID: 10, Name: testPackageName, Version: "1.0.0", PackageType: "generic"}}})
	want := "## Package: my-pkg\n\n" +
		"- **ID**: 10\n" +
		"- **Version**: 1.0.0\n" +
		"- **Type**: generic\n" +
		genericGetHints
	if got != want {
		t.Errorf("FormatGetMarkdown(generic) =\n%q\nwant:\n%q", got, want)
	}
	for _, packageType := range []string{"npm", "maven", "conan", "pypi", "Generic"} {
		t.Run(packageType, func(t *testing.T) {
			card := FormatGetMarkdown(GetOutput{Package: DetailItem{ListItem: ListItem{ID: 10, Name: testPackageName, PackageType: packageType}}})
			if !strings.HasSuffix(card, getHints) || strings.Contains(card, "package.download") {
				t.Errorf("FormatGetMarkdown(%s) ends\n%q\nwant the hints without package.download", packageType, card)
			}
		})
	}
}

// TestFormatGetMarkdown_LeavesOutWhatGitLabDidNotSend verifies a package with
// no creator, links, pipeline, tags or other versions is the card of what it
// has and nothing else: no empty row, and no table heading over no rows.
func TestFormatGetMarkdown_LeavesOutWhatGitLabDidNotSend(t *testing.T) {
	got := FormatGetMarkdown(GetOutput{Package: DetailItem{ListItem: ListItem{ID: 10, Name: testPackageName, Version: "1.0.0", Links: &LinksItem{}}}})
	want := "## Package: my-pkg\n\n" +
		"- **ID**: 10\n" +
		"- **Version**: 1.0.0\n" +
		getHints
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatGetMarkdown_EscapesWhatGitLabAuthored verifies a package name, a
// tag name and a version string carrying a pipe and markup reach the card
// escaped, so none of them can add a column, a heading or a tag of its own.
func TestFormatGetMarkdown_EscapesWhatGitLabAuthored(t *testing.T) {
	const (
		name    = "pkg|<b>x</b>"
		tag     = "rc|<i>1</i>"
		version = "2.0|<u>0</u>"
	)
	got := FormatGetMarkdown(GetOutput{Package: DetailItem{
		ListItem: ListItem{ID: 10, Name: name, Version: "1.0.0", Tags: []toolutil.PackageTagOutput{{Name: tag}}},
		Versions: []toolutil.PackageVersionOutput{{ID: 9, Version: version, Tags: []toolutil.PackageTagOutput{{Name: tag}}}},
	}})
	for _, raw := range []string{name, tag, version} {
		t.Run(raw, func(t *testing.T) {
			if strings.Contains(got, raw) {
				t.Errorf("card carries %q unescaped:\n%s", raw, got)
			}
		})
	}
	for _, escaped := range []string{
		toolutil.EscapeMdHeading("Package: " + name),
		"- **Tags**: " + toolutil.EscapeMdTableCell(tag) + "\n",
		"| 9 | " + toolutil.EscapeMdTableCell(version) + " | " + toolutil.EscapeMdTableCell(tag) + " |  |  |\n",
	} {
		t.Run(escaped, func(t *testing.T) {
			if !strings.Contains(got, escaped) {
				t.Errorf("card does not carry %q:\n%s", escaped, got)
			}
		})
	}
}

// TestFormatPackageNotFound_NamesBothReasonsForA404 verifies the not-found
// result is an error result naming the package as the caller gave it, with
// both reasons GitLab answers 404 for a package_id: a status this read
// refuses, on a package package.list still shows, which re-listing alone
// would hand back unchanged, and a deleted version.
func TestFormatPackageNotFound_NamesBothReasonsForA404(t *testing.T) {
	result := formatPackageNotFound(packageNotFoundOutput{Identifier: "10 in project 42"})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("not-found result = %+v, want one error content", result)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("not-found content = %T, want text", result.Content[0])
	}
	text := content.Text
	for _, want := range []string{
		"Package Not Found",
		"**10 in project 42**",
		"- GitLab reads only a package whose status is default or deprecated here, and answers 404 for one " +
			"package.list shows in error status, or in hidden, processing or pending_destruction when asked " +
			"for by status: read the status column there rather than listing again\n",
		"- Each version of a package has its own package_id, and deleting that version retires it\n",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("not-found result does not carry %q:\n%s", want, text)
			}
		})
	}
}

// TestFormatFileListMarkdown_EmptyFiles verifies an empty list is the one
// sentence and nothing else.
func TestFormatFileListMarkdown_EmptyFiles(t *testing.T) {
	got := FormatFileListMarkdown(FileListOutput{Files: nil, Pagination: toolutil.PaginationOutput{TotalItems: 0}})
	want := "No package files found.\n"
	if got != want {
		t.Errorf("FormatFileListMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatFileListMarkdown_LongSHA verifies the whole table for one file
// whose digest is longer than the column.
func TestFormatFileListMarkdown_LongSHA(t *testing.T) {
	out := FileListOutput{
		Files: []FileListItem{
			{PackageFileID: 1, FileName: "pkg.tar.gz", Size: 2048, SHA256: "0123456789abcdef01234567"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	got := FormatFileListMarkdown(out)
	want := "## Package Files (1)\n\n" +
		"| ID | File Name | Size (bytes) | SHA256 |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | pkg.tar.gz | 2048 | `0123456789ab...` |\n" +
		"\n1 items total\n" + fileListHints
	if got != want {
		t.Errorf("FormatFileListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

const (
	// pathPutPkg1 identifies the path put pkg 1 constant used by this package.
	pathPutPkg1 = "PUT /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz"
	// hdrContentType identifies the hdr content type constant used by this package.
	hdrContentType = "Content-Type"
	// mimeOctetStream identifies the mime octet stream constant used by this package.
	mimeOctetStream = "application/octet-stream"
	// fmtExpPkgVersionErr identifies the fmt exp pkg version err constant used by this package.
	fmtExpPkgVersionErr = "expected package_version error, got: %v"
	// pathTmpOutBin identifies the path tmp out bin constant used by this package.
	pathTmpOutBin = "/tmp/out.bin"
	// fmtExpProjectIDErr identifies the fmt exp project ID err constant used by this package.
	fmtExpProjectIDErr = "expected project_id error, got: %v"
	// testCtxCancelled identifies the test ctx cancelled constant used by this package.
	testCtxCancelled = "context canceled"
	// fmtExpCtxCancelErr identifies the fmt exp ctx cancel err constant used by this package.
	fmtExpCtxCancelErr = "expected context canceled error, got: %v"
	// pathAPIPkgs1 identifies the path API pkgs 1 constant used by this package.
	pathAPIPkgs1 = "/api/v4/projects/1/packages"
	// testFileAppBin identifies the test file app bin constant used by this package.
	testFileAppBin = "app.bin"
	// testFileOutBin identifies the test file out bin constant used by this package.
	testFileOutBin = "out.bin"
	// fmtExpCtxCancelGot identifies the fmt exp ctx cancel got constant used by this package.
	fmtExpCtxCancelGot = "expected context canceled, got: %v"
)

// ---------------------------------------------------------------------------
// Publish — missing package_version
// ---------------------------------------------------------------------------.

// TestPublish_MissingVersion verifies Publish when missing version.
func TestPublish_MissingVersion(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:     "42",
		PackageName:   testPackageName,
		FileName:      testFileName,
		ContentBase64: testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), "package_version") {
		t.Fatalf(fmtExpPkgVersionErr, err)
	}
}

// ---------------------------------------------------------------------------
// Publish — invalid file name (starts with ~)
// ---------------------------------------------------------------------------.

// TestPublish_InvalidFileName verifies Publish when invalid file name.
func TestPublish_InvalidFileName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       "~badname.tar.gz",
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal("expected error for invalid file name")
	}
}

// ---------------------------------------------------------------------------
// Publish — invalid base64 content
// ---------------------------------------------------------------------------.

// TestPublish_InvalidBase64 verifies Publish when invalid base 64.
func TestPublish_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  "!!!not-base64!!!",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid base64") {
		t.Fatalf("expected invalid base64 error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Download — missing required fields
// ---------------------------------------------------------------------------.

// TestDownload_MissingProjectID verifies Download when missing project ID.
func TestDownload_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		OutputPath:     pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestDownload_MissingPackageName verifies Download when missing package name.
func TestDownload_MissingPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		OutputPath:     pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "package_name") {
		t.Fatalf("expected package_name error, got: %v", err)
	}
}

// TestDownload_MissingVersion verifies Download when missing version.
func TestDownload_MissingVersion(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:   "42",
		PackageName: testPackageName,
		FileName:    testFileName,
		OutputPath:  pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "package_version") {
		t.Fatalf(fmtExpPkgVersionErr, err)
	}
}

// TestDownload_MissingFileName verifies Download when missing file name.
func TestDownload_MissingFileName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		OutputPath:     pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "file_name") {
		t.Fatalf("expected file_name error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List — API error, context canceled, with sort/order_by/version filters
// ---------------------------------------------------------------------------.

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_ContextCancelled verifies List when context cancelled.
func TestList_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(ctx, client, ListInput{ProjectID: "1"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestList_WithSortAndOrderBy verifies List when with sort and order by.
func TestList_WithSortAndOrderBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			q := r.URL.Query()
			if q.Get("order_by") != "created_at" {
				t.Errorf("expected order_by=created_at, got %q", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("expected sort=desc, got %q", q.Get("sort"))
			}
			if q.Get("package_version") != "2.0.0" {
				t.Errorf("expected package_version=2.0.0, got %q", q.Get("package_version"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:      "1",
		OrderBy:        "created_at",
		Sort:           "desc",
		PackageVersion: "2.0.0",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithEmptyPackage verifies list output when a package has no tags or links.
func TestList_WithEmptyPackage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"pkg","version":"1.0.0","package_type":"generic","status":"default"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(out.Packages))
	}
	if out.Packages[0].CreatedAt != "" {
		t.Errorf("CreatedAt should be empty when nil, got %q", out.Packages[0].CreatedAt)
	}
	if out.Packages[0].Links != nil {
		t.Errorf("Links should be nil when _links is absent, got %+v", out.Packages[0].Links)
	}
	if len(out.Packages[0].Tags) != 0 {
		t.Errorf("Tags should be empty, got %v", out.Packages[0].Tags)
	}
}

// ---------------------------------------------------------------------------
// FileList — API error, context canceled, missing project_id
// ---------------------------------------------------------------------------.

// TestFileList_APIError verifies FileList when API error.
func TestFileList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := FileList(context.Background(), client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFileList_ContextCancelled verifies FileList when context cancelled.
func TestFileList_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := FileList(ctx, client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestFileList_MissingProjectID verifies FileList when missing project ID.
func TestFileList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := FileList(context.Background(), client, FileListInput{PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestFileList_WithCreatedAt verifies file list output when created_at is present.
func TestFileList_WithCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/packages/10/package_files" {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":20,"package_id":10,"file_name":"app.bin","size":100,
				"file_sha256":"hash","created_at":"2026-06-01T10:00:00Z"
			}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := FileList(context.Background(), client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(out.Files))
	}
	if out.Files[0].CreatedAt != "2026-06-01T10:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-06-01T10:00:00Z", out.Files[0].CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, context canceled
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := Delete(context.Background(), nil, client, DeleteInput{ProjectID: "1", PackageID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_ContextCancelled verifies Delete when context cancelled.
func TestDelete_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(ctx, nil, client, DeleteInput{ProjectID: "1", PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// ---------------------------------------------------------------------------
// FileDelete — API error, context canceled, missing project_id, missing package_id
// ---------------------------------------------------------------------------.

// TestFileDelete_APIError verifies FileDelete when API error.
func TestFileDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{ProjectID: "1", PackageID: "10", PackageFileID: "20"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFileDelete_ContextCancelled verifies FileDelete when context cancelled.
func TestFileDelete_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := FileDelete(ctx, nil, client, FileDeleteInput{ProjectID: "1", PackageID: "10", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestFileDelete_MissingProjectID verifies FileDelete when missing project ID.
func TestFileDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{PackageID: "10", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestFileDelete_MissingPackageID verifies FileDelete when missing package ID.
func TestFileDelete_MissingPackageID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{ProjectID: "1", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), "package_id") {
		t.Fatalf("expected package_id error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PublishDirectory — missing project_id, invalid package name, nonexistent dir
// ---------------------------------------------------------------------------.

// TestPublishDirectory_MissingProjectID verifies PublishDirectory when missing project ID.
func TestPublishDirectory_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		DirectoryPath:  t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestPublishDirectory_InvalidPackageName verifies PublishDirectory when invalid package name.
func TestPublishDirectory_InvalidPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "1",
		PackageName:    ".invalid",
		PackageVersion: "1.0.0",
		DirectoryPath:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for invalid package name")
	}
}

// TestPublishDirectory_MissingVersion verifies PublishDirectory when missing version.
func TestPublishDirectory_MissingVersion(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:     "1",
		PackageName:   testPackageName,
		DirectoryPath: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "package_version") {
		t.Fatalf(fmtExpPkgVersionErr, err)
	}
}

// TestPublishDirectory_NonexistentDir verifies PublishDirectory when nonexistent dir.
func TestPublishDirectory_NonexistentDir(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "1",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		DirectoryPath:  filepath.Join(t.TempDir(), "nonexistent"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — context canceled
// ---------------------------------------------------------------------------.

// TestStreamDownload_ContextCancelled verifies StreamDownload when context cancelled.
func TestStreamDownload_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, _, err := streamDownloadPackageFile(ctx, nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testFileOutBin),
	})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelGot, err)
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip via meta-tool
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — successful download
// ---------------------------------------------------------------------------.

// TestStreamDownload_Success verifies StreamDownload when success.
func TestStreamDownload_Success(t *testing.T) {
	fileData := []byte("streaming-download-content")
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeOctetStream)
		w.Write(fileData)
	}))

	outputPath := filepath.Join(t.TempDir(), testFileOutBin)
	size, checksum, err := streamDownloadPackageFile(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileAppBin,
		OutputPath:     outputPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if size != int64(len(fileData)) {
		t.Errorf("expected size %d, got %d", len(fileData), size)
	}
	if checksum == "" {
		t.Error("expected non-empty checksum")
	}
	data, _ := os.ReadFile(outputPath)
	if string(data) != string(fileData) {
		t.Errorf("file content mismatch")
	}
}

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — API error on Do()
// ---------------------------------------------------------------------------.

// TestStreamDownload_APIError verifies StreamDownload when API error.
func TestStreamDownload_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))

	outputPath := filepath.Join(t.TempDir(), testFileOutBin)
	_, _, err := streamDownloadPackageFile(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileAppBin,
		OutputPath:     outputPath,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Publish — both file_path and content_base64
// ---------------------------------------------------------------------------.

// TestPublish_BothFileAndBase64 verifies Publish when both file and base 64.
func TestPublish_BothFileAndBase64(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		FilePath:       "/tmp/file.bin",
		ContentBase64:  testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected 'not both' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publish — neither file_path nor content_base64
// ---------------------------------------------------------------------------.

// TestPublish_NeitherFileNorBase64 verifies Publish when neither file nor base 64.
func TestPublish_NeitherFileNorBase64(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
	})
	if err == nil || !strings.Contains(err.Error(), "either file_path or content_base64") {
		t.Fatalf("expected 'either' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publish — API error on publish call
// ---------------------------------------------------------------------------.

// TestPublish_APIError verifies Publish when API error.
func TestPublish_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("test")),
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Publish — context canceled
// ---------------------------------------------------------------------------.

// TestPublish_ContextCancelled verifies Publish when context cancelled.
func TestPublish_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(ctx, nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelGot, err)
	}
}

// ---------------------------------------------------------------------------
// Publish — file_path with small file
// ---------------------------------------------------------------------------.

// TestPublish_FilePathSmallFile verifies Publish when file path small file.
func TestPublish_FilePathSmallFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "small.bin")
	if err := os.WriteFile(tmpFile, []byte("small-data"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 1, "package_id": 10, "file_name": "small.bin",
				"size": 10, "file_sha256": "abc", "file_md5": "md5",
				"file_sha1": "sha1", "file_store": 1,
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       "small.bin",
		FilePath:       tmpFile,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PackageFileID != 1 {
		t.Errorf("expected PackageFileID=1, got %d", out.PackageFileID)
	}
	if out.UpdatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-01-02T00:00:00Z", out.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// Publish — invalid package name
// ---------------------------------------------------------------------------.

// TestPublish_InvalidPackageName verifies Publish when invalid package name.
func TestPublish_InvalidPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    ".invalid",
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal("expected error for invalid package name")
	}
}

// ---------------------------------------------------------------------------
// Publish — missing project_id
// ---------------------------------------------------------------------------.

// TestPublish_MissingProjectID verifies Publish when missing project ID.
func TestPublish_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// List — with package_name and package_type filter
// ---------------------------------------------------------------------------.

// TestList_WithNameAndTypeFilter verifies List when with name and type filter.
func TestList_WithNameAndTypeFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			q := r.URL.Query()
			if q.Get("package_name") != testPackageName {
				t.Errorf("expected package_name=my-pkg, got %q", q.Get("package_name"))
			}
			if q.Get("package_type") != "generic" {
				t.Errorf("expected package_type=generic, got %q", q.Get("package_type"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default",
				"_links": {"web_path": "/packages/10"},
				"tags": [{"name": "latest"}],
				"created_at": "2026-01-01T00:00:00Z"
			}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:   "1",
		PackageName: testPackageName,
		PackageType: "generic",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(out.Packages))
	}
	if out.Packages[0].Links == nil || out.Packages[0].Links.WebPath != "/packages/10" {
		t.Errorf("expected Links.WebPath=/packages/10, got %+v", out.Packages[0].Links)
	}
	if len(out.Packages[0].Tags) != 1 || out.Packages[0].Tags[0].Name != "latest" {
		t.Errorf("expected 1 tag named latest, got %+v", out.Packages[0].Tags)
	}
}

// ---------------------------------------------------------------------------
// PublishDirectory — empty dir (no matching files)
// ---------------------------------------------------------------------------.

// TestPublishDirectory_EmptyDir verifies PublishDirectory when empty dir.
func TestPublishDirectory_EmptyDir(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	dir := t.TempDir()
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "1",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err == nil || !strings.Contains(err.Error(), "no matching files") {
		t.Fatalf("expected 'no matching files' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publish — file_path with nonexistent file
// ---------------------------------------------------------------------------.

// TestPublish_FilePathNonexistent verifies Publish when file path nonexistent.
func TestPublish_FilePathNonexistent(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		FilePath:       filepath.Join(t.TempDir(), "nonexistent.bin"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
