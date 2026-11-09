// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package docblocks

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestParse_Fences_AreReadAsTheSiteCheckReadsThem holds the parser to the
// rules check-i18n-code.mjs applies, case by case, since a block the two read
// differently would be compared by one and served by the other.
func TestParse_Fences_AreReadAsTheSiteCheckReadsThem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []Block
	}{
		{
			name: "a fence with a language and a key",
			src:  "Text.\n\n```nginx e2e=nginx\nserver {}\n```\n",
			want: []Block{{Lang: "nginx", Info: "e2e=nginx", Key: "nginx", Body: "server {}\n", Line: 3}},
		},
		{
			name: "a fence with no key",
			src:  "```bash\necho hi\n```\n",
			want: []Block{{Lang: "bash", Body: "echo hi\n", Line: 1}},
		},
		{
			name: "a tilde fence",
			src:  "~~~yaml e2e=compose\na: 1\n~~~\n",
			want: []Block{{Lang: "yaml", Info: "e2e=compose", Key: "compose", Body: "a: 1\n", Line: 1}},
		},
		{
			name: "the key among other options",
			src:  "```text comments=hash e2e=haproxy\nglobal\n```\n",
			want: []Block{{Lang: "text", Info: "comments=hash e2e=haproxy", Key: "haproxy", Body: "global\n", Line: 1}},
		},
		{
			name: "a key spelled inside a quoted title is not a key",
			src:  "```bash title=\"run e2e=fake here\"\nls\n```\n",
			want: []Block{{Lang: "bash", Info: `title="run e2e=fake here"`, Body: "ls\n", Line: 1}},
		},
		{
			name: "a key beside a quoted title",
			src:  "```bash title='a b' e2e=listing\nls\n```\n",
			want: []Block{{Lang: "bash", Info: "title='a b' e2e=listing", Key: "listing", Body: "ls\n", Line: 1}},
		},
		{
			name: "an indented fence loses its indentation on every line",
			src:  "<TabItem>\n  ```yaml e2e=labels\n    labels:\n      - a\n  ```\n</TabItem>\n",
			want: []Block{{Lang: "yaml", Info: "e2e=labels", Key: "labels", Body: "  labels:\n    - a\n", Line: 2}},
		},
		{
			name: "a line less indented than the fence is trimmed rather than cut",
			src:  "    ```text\n  short\n    ```\n",
			want: []Block{{Lang: "text", Body: "short\n", Line: 1}},
		},
		{
			name: "a longer fence holds a shorter one",
			src:  "````markdown\n```bash\nls\n```\n````\n",
			want: []Block{{Lang: "markdown", Body: "```bash\nls\n```\n", Line: 1}},
		},
		{
			name: "a fence of the other character does not close it",
			src:  "```text\n~~~\n```\n",
			want: []Block{{Lang: "text", Body: "~~~\n", Line: 1}},
		},
		{
			name: "a closing run followed by text does not close it",
			src:  "```text\n``` not a close\n```\n",
			want: []Block{{Lang: "text", Body: "``` not a close\n", Line: 1}},
		},
		{
			name: "an empty block",
			src:  "```text\n```\n",
			want: []Block{{Lang: "text", Line: 1}},
		},
		{
			name: "a backtick run whose info holds a backtick is inline code",
			src:  "```a`b```\n\n```text\nbody\n```\n",
			want: []Block{{Lang: "text", Body: "body\n", Line: 3}},
		},
		{
			name: "the frontmatter is not read for fences",
			src:  "---\ntitle: x\ndescription: \"```not a fence\"\n---\n\n```bash\nls\n```\n",
			want: []Block{{Lang: "bash", Body: "ls\n", Line: 6}},
		},
		{
			name: "a fence-like line closing the frontmatter's block scalar is text",
			src:  "---\ntitle: x\ndescription: |\n  ```not a fence\n---\n\n```bash\nls\n```\n",
			want: []Block{{Lang: "bash", Body: "ls\n", Line: 7}},
		},
		{
			name: "a page without frontmatter is read from its first line, past a thematic break",
			src:  "```bash\nls\n```\n\n---\n\n```text\nmore\n```\n",
			want: []Block{
				{Lang: "bash", Body: "ls\n", Line: 1},
				{Lang: "text", Body: "more\n", Line: 7},
			},
		},
		{
			name: "a frontmatter never closed is not one",
			src:  "---\n```bash\nls\n```\n",
			want: []Block{{Lang: "bash", Body: "ls\n", Line: 2}},
		},
		{
			name: "two blocks in page order",
			src:  "```nginx e2e=one\na\n```\n\n```nginx e2e=two\nb\n```\n",
			want: []Block{
				{Lang: "nginx", Info: "e2e=one", Key: "one", Body: "a\n", Line: 1},
				{Lang: "nginx", Info: "e2e=two", Key: "two", Body: "b\n", Line: 5},
			},
		},
		{
			name: "a page with carriage returns reads as one without them",
			src:  "```nginx e2e=crlf\r\nserver {}\r\n```\r\n",
			want: []Block{{Lang: "nginx", Info: "e2e=crlf", Key: "crlf", Body: "server {}\n", Line: 1}},
		},
		{
			name: "a page with no fence",
			src:  "Just prose.\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse:\n got %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// TestSplitOptions_Spaces_SeparateOptionsOutsideQuotes pins the splitting the
// key is found by: runs of spaces and tabs separate options and produce no
// empty one, and a quote holds whatever it encloses, spaces included.
func TestSplitOptions_Spaces_SeparateOptionsOutsideQuotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		options string
		want    []string
	}{
		{name: "nothing", options: "", want: nil},
		{name: "one option", options: "e2e=a", want: []string{"e2e=a"}},
		{name: "a run of spaces and a tab", options: "a  \t b", want: []string{"a", "b"}},
		{name: "a trailing space", options: "a ", want: []string{"a"}},
		{name: "double quotes", options: `title="a b" c`, want: []string{`title="a b"`, "c"}},
		{name: "single quotes holding a double one", options: `title='a "b' c`, want: []string{`title='a "b'`, "c"}},
		{name: "an unclosed quote runs to the end", options: `title="a b`, want: []string{`title="a b`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := splitOptions(tt.options); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitOptions(%q) = %#v, want %#v", tt.options, got, tt.want)
			}
		})
	}
}

// TestParse_MalformedFences_AreRefused pins the fences a test must never be
// handed a block from: one that never closes, which would otherwise be the
// rest of the page, and an e2e option that does not name exactly one key.
func TestParse_MalformedFences_AreRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want error
		// line is the fence the error must name.
		line string
	}{
		{name: "a fence never closed", src: "Text.\n```nginx e2e=open\nserver {}\n", want: ErrUnclosedFence, line: "line 2"},
		{name: "a fence closed by a shorter run", src: "````text\nbody\n```\n", want: ErrUnclosedFence, line: "line 1"},
		{name: "an empty key", src: "```nginx e2e=\na\n```\n", want: ErrBadKey, line: "line 1"},
		{name: "a key with an underscore", src: "```nginx e2e=a_b\na\n```\n", want: ErrBadKey, line: "line 1"},
		{name: "a key in capitals", src: "```nginx e2e=Nginx\na\n```\n", want: ErrBadKey, line: "line 1"},
		{name: "a key ending in a hyphen", src: "```nginx e2e=nginx-\na\n```\n", want: ErrBadKey, line: "line 1"},
		{name: "two keys on one fence", src: "\n```nginx e2e=a e2e=b\na\n```\n", want: ErrBadKey, line: "line 2"},
		{name: "a quoted key", src: "```nginx e2e=\"a\"\na\n```\n", want: ErrBadKey, line: "line 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(tt.src))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse = %#v, %v; want an error wrapping %q", got, err, tt.want)
			}
			if !strings.Contains(err.Error(), tt.line) {
				t.Errorf("the error does not name %s: %v", tt.line, err)
			}
		})
	}
}

// writePages writes an English page and its Spanish twin under a fresh
// documentation root and returns the root. A nil page is not written.
func writePages(t *testing.T, rel string, english, twin *string) string {
	t.Helper()
	root := t.TempDir()
	write := func(path string, body *string) {
		if body == nil {
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(*body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	write(filepath.Join(root, filepath.FromSlash(rel)), english)
	write(filepath.Join(root, "es", filepath.FromSlash(rel)), twin)
	return root
}

// TestPageBlock_KeyedBlock_IsReturnedFromTheEnglishPage pins the one answer a
// test builds on: the English body, whatever the Spanish fence holds around
// the same key, since comments are what a translation may change.
func TestPageBlock_KeyedBlock_IsReturnedFromTheEnglishPage(t *testing.T) {
	t.Parallel()

	const rel = "operations/guide.mdx"
	english := "# Guide\n\n```nginx e2e=nginx\n# Read me.\nserver {}\n```\n\n```bash\nls\n```\n"
	spanish := "# Guía\n\n```nginx e2e=nginx\n# Léeme.\nserver {}\n```\n\n```bash\nls\n```\n"
	root := writePages(t, rel, &english, &spanish)

	page, err := Open(root, rel)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if want := filepath.Join(root, "operations", "guide.mdx"); page.Path != want {
		t.Errorf("Path = %q, want %q", page.Path, want)
	}
	if want := filepath.Join(root, "es", "operations", "guide.mdx"); page.Twin != want {
		t.Errorf("Twin = %q, want %q", page.Twin, want)
	}
	block, err := page.Block("nginx")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	want := Block{Lang: "nginx", Info: "e2e=nginx", Key: "nginx", Body: "# Read me.\nserver {}\n", Line: 3}
	if block != want {
		t.Errorf("Block:\n got %#v\nwant %#v", block, want)
	}
}

// TestPageBlock_KeyNotServedOnce_IsRefused pins every way a key can fail to
// name one block in both languages. Each is a page edit a test would
// otherwise survive: a renamed key, a copied fence, a translation that lost
// the option.
func TestPageBlock_KeyNotServedOnce_IsRefused(t *testing.T) {
	t.Parallel()

	const rel = "guide.mdx"
	keyed := "```nginx e2e=nginx\nserver {}\n```\n"
	tests := []struct {
		name    string
		english string
		spanish string
		key     string
		want    error
		// names are what the error must mention, so a reader can find it.
		names []string
	}{
		{
			name:    "a key no fence carries",
			english: keyed, spanish: keyed, key: "caddy",
			want: ErrUnknownKey, names: []string{"caddy", "guide.mdx"},
		},
		{
			name:    "a key two English fences carry",
			english: keyed + "\n" + keyed, spanish: keyed, key: "nginx",
			want: ErrDuplicateKey, names: []string{"nginx", "lines 1 and 5"},
		},
		{
			name:    "a key the twin lost",
			english: keyed, spanish: "```nginx\nserver {}\n```\n", key: "nginx",
			want: ErrNotInTwin, names: []string{"nginx", filepath.Join("es", "guide.mdx")},
		},
		{
			name:    "a key the twin carries twice",
			english: keyed, spanish: keyed + "\n" + keyed, key: "nginx",
			want: ErrDuplicateKey, names: []string{"nginx", filepath.Join("es", "guide.mdx")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writePages(t, rel, &tt.english, &tt.spanish)
			page, err := Open(root, rel)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			got, err := page.Block(tt.key)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Block(%q) = %#v, %v; want an error wrapping %q", tt.key, got, err, tt.want)
			}
			for _, name := range tt.names {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("the error does not name %q: %v", name, err)
				}
			}
		})
	}
}

// TestOpen_PageOrTwinUnreadable_IsRefused pins that a page is never read
// without its twin, and that a malformed fence on either side stops the read
// before any block is handed out.
func TestOpen_PageOrTwinUnreadable_IsRefused(t *testing.T) {
	t.Parallel()

	const rel = "guide.mdx"
	keyed := "```nginx e2e=nginx\nserver {}\n```\n"
	tests := []struct {
		name    string
		english *string
		spanish *string
		want    error
		names   []string
	}{
		{name: "no English page", english: nil, spanish: &keyed, want: fs.ErrNotExist, names: []string{"guide.mdx"}},
		{name: "no Spanish twin", english: &keyed, spanish: nil, want: fs.ErrNotExist, names: []string{filepath.Join("es", "guide.mdx")}},
		{name: "an unclosed fence on the English page", english: new("```nginx\n"), spanish: &keyed, want: ErrUnclosedFence, names: []string{"guide.mdx", "line 1"}},
		{name: "an unclosed fence on the twin", english: &keyed, spanish: new("x\n```nginx\n"), want: ErrUnclosedFence, names: []string{filepath.Join("es", "guide.mdx"), "line 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writePages(t, rel, tt.english, tt.spanish)
			page, err := Open(root, rel)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Open = %#v, %v; want an error wrapping %q", page, err, tt.want)
			}
			for _, name := range tt.names {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("the error does not name %q: %v", name, err)
				}
			}
		})
	}
}
