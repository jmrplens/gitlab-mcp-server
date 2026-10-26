// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// The header as each comment syntax spells it, one line at a time, with the
// newline that ends each line.
const (
	slashC = "// " + fileheader.CopyrightStatement + "\n"
	slashL = "// " + fileheader.LicenseStatement + "\n"
	hashC  = "# " + fileheader.CopyrightStatement + "\n"
	hashL  = "# " + fileheader.LicenseStatement + "\n"
	blockC = "/* " + fileheader.CopyrightStatement + " */\n"
	blockL = "/* " + fileheader.LicenseStatement + " */\n"
)

// mustKind returns the kind of name, failing the test when name is not a
// source file.
func mustKind(t *testing.T, name string) kind {
	t.Helper()
	k, ok := kindOf(name)
	if !ok {
		t.Fatalf("kindOf(%q) is not a source file", name)
	}
	return k
}

// TestKindOf_SourceFiles_SpellTheHeaderInTheirCommentSyntax verifies which
// files the rule judges, by base name and by extension, and the comment
// syntax each one's header is written in.
func TestKindOf_SourceFiles_SpellTheHeaderInTheirCommentSyntax(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{name: "cmd/server/main.go", want: []string{slashC, slashL}},
		{name: "scripts/install.sh", want: []string{hashC, hashL}},
		{name: "scripts/run.bash", want: []string{hashC, hashL}},
		{name: "scripts/build_pypi.py", want: []string{hashC, hashL}},
		{name: "cmd/gen_api_live/introspect.rb", want: []string{hashC, hashL}},
		{name: "scripts/install.ps1", want: []string{hashC, hashL}},
		{name: "scripts/Module.psm1", want: []string{hashC, hashL}},
		{name: "Makefile", want: []string{hashC, hashL}},
		{name: "site/Makefile", want: []string{hashC, hashL}},
		{name: "build/rules.mk", want: []string{hashC, hashL}},
		{name: "mcpb/platform.jq", want: []string{hashC, hashL}},
		{name: "Dockerfile", want: []string{hashC, hashL}},
		{name: "npm/gitlab-mcp-server/cli.js", want: []string{slashC, slashL}},
		{name: "scripts/build-npm.mjs", want: []string{slashC, slashL}},
		{name: "a/config.cjs", want: []string{slashC, slashL}},
		{name: "site/src/routeData.ts", want: []string{slashC, slashL}},
		{name: "a/module.mts", want: []string{slashC, slashL}},
		{name: "a/module.cts", want: []string{slashC, slashL}},
		{name: "a/View.jsx", want: []string{slashC, slashL}},
		{name: "a/View.tsx", want: []string{slashC, slashL}},
		{name: "site/src/styles/theme.css", want: []string{blockC, blockL}},
		{name: "site/src/components/Hero.astro", want: []string{slashC, slashL}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustKind(t, tt.name).header()
			want := []string{tt.want[0][:len(tt.want[0])-1], tt.want[1][:len(tt.want[1])-1]}
			if !slices.Equal(got, want) {
				t.Errorf("header() = %q, want %q", got, want)
			}
		})
	}
}

// TestKindOf_OtherFiles_AreNotJudged verifies that documentation, data,
// configuration and binary files are outside the rule, and that a base name
// is matched whole.
func TestKindOf_OtherFiles_AreNotJudged(t *testing.T) {
	for _, name := range []string{
		"README.md", "site/src/content/docs/index.mdx", "go.mod", "go.sum", ".github/workflows/ci.yml",
		"server.json", "LICENSE", "VERSION", "site/public/favicon.svg", "cmd/x/testdata/want.go.golden",
		"Makefile.orig", "OldDockerfile", "docs/notes.txt", "internal/toolutil/icons/brand.webp",
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := kindOf(name); ok {
				t.Errorf("kindOf(%q) is a source file, want it outside the rule", name)
			}
		})
	}
}

// TestJudge_HeaderWhereTheKindPutsIt_Passes verifies the header is found
// where each kind puts it: first, after a shebang, after an encoding
// declaration, after Dockerfile parser directives, after a CSS @charset and
// inside an Astro component's frontmatter, including at the end of a file
// with no trailing newline.
func TestJudge_HeaderWhereTheKindPutsIt_Passes(t *testing.T) {
	tests := []struct {
		name, path, content string
	}{
		{name: "go", path: "a.go", content: slashC + slashL + "\npackage a\n"},
		{name: "go above a build constraint", path: "a.go", content: slashC + slashL + "\n//go:build e2e\n\npackage a\n"},
		{name: "go without a trailing newline", path: "a.go", content: slashC + slashL[:len(slashL)-1]},
		{name: "shell after its shebang", path: "a.sh", content: "#!/bin/sh\n" + hashC + hashL + "\necho a\n"},
		{name: "makefile", path: "Makefile", content: hashC + hashL + "\nall:\n"},
		{name: "jq after its shebang", path: "a.jq", content: "#!/usr/bin/jq -f\n" + hashC + hashL + "\n.name\n"},
		{name: "python after shebang and encoding", path: "a.py", content: "#!/usr/bin/env python3\n# -*- coding: latin-1 -*-\n" + hashC + hashL},
		{name: "python after an encoding on the first line", path: "a.py", content: "# vim: set fileencoding=utf-8 :\n" + hashC + hashL},
		{name: "ruby after an encoding", path: "a.rb", content: "# encoding: utf-8\n" + hashC + hashL},
		{name: "dockerfile after its parser directives", path: "Dockerfile", content: "# syntax=docker/dockerfile:1\n# Escape=`\n#check=skip=all\n" + hashC + hashL + "\nFROM scratch\n"},
		{name: "javascript after its shebang", path: "a.mjs", content: "#!/usr/bin/env node\n" + slashC + slashL},
		{name: "css after @charset", path: "a.css", content: "@charset \"UTF-8\";\n" + blockC + blockL + "\n.a {}\n"},
		{name: "astro inside the frontmatter", path: "a.astro", content: "---\n" + slashC + slashL + "---\n<p>a</p>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := judge(mustKind(t, tt.path), tt.content)
			if got.missing || len(got.notices) != 0 {
				t.Errorf("judge() = %+v, want the header found and no notice", got)
			}
		})
	}
}

// TestJudge_HeaderAbsentOrOutOfPlace_IsMissing verifies the header counts only
// as the two lines, in order, exactly where the kind puts it.
func TestJudge_HeaderAbsentOrOutOfPlace_IsMissing(t *testing.T) {
	tests := []struct {
		name, path, content string
		notices             []int
	}{
		{name: "empty file", path: "a.go", content: ""},
		{name: "no header", path: "a.go", content: "package a\n"},
		{name: "copyright alone at the end of the file", path: "a.go", content: slashC[:len(slashC)-1], notices: []int{1}},
		{name: "license line missing", path: "a.go", content: slashC + "\npackage a\n", notices: []int{1}},
		{name: "lines swapped", path: "a.go", content: slashL + slashC + "\npackage a\n", notices: []int{1, 2}},
		{name: "below the package doc comment", path: "a.go", content: "// Package a.\n" + slashC + slashL + "package a\n", notices: []int{2, 3}},
		{name: "wrong syntax for the kind", path: "a.sh", content: slashC + slashL, notices: []int{1, 2}},
		{name: "after the script's first command", path: "a.sh", content: "#!/bin/sh\necho a\n" + hashC + hashL, notices: []int{3, 4}},
		{name: "astro without frontmatter", path: "a.astro", content: slashC + slashL + "<p>a</p>\n", notices: []int{1, 2}},
		{name: "astro after the frontmatter", path: "a.astro", content: "---\n---\n" + slashC + slashL, notices: []int{3, 4}},
		{name: "css after a comment", path: "a.css", content: "/* theme */\n" + blockC + blockL, notices: []int{2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := judge(mustKind(t, tt.path), tt.content)
			if !got.missing {
				t.Errorf("judge() = %+v, want the header missing", got)
			}
			if !slices.Equal(got.notices, tt.notices) {
				t.Errorf("judge() notices = %v, want %v", got.notices, tt.notices)
			}
		})
	}
}

// TestJudge_AnotherNotice_IsReportedByLine verifies a file carrying a notice
// other than the header is reported at every line that carries one, whether
// the header is there or not, so neither a file somebody else wrote nor a
// second copy of the header goes unseen.
func TestJudge_AnotherNotice_IsReportedByLine(t *testing.T) {
	tests := []struct {
		name, path, content string
		missing             bool
		notices             []int
	}{
		{name: "third-party go file", path: "a.go", content: "// Copyright 2009 The Go Authors. All rights reserved.\n\npackage a\n", missing: true, notices: []int{1}},
		{name: "second copy of the header", path: "a.go", content: slashC + slashL + "\npackage a\n\n" + slashC, notices: []int{6}},
		{name: "another license beside the header", path: "a.py", content: hashC + hashL + "# SPDX-License-Identifier: Apache-2.0\n", notices: []int{3}},
		{name: "REUSE copyright text", path: "a.css", content: blockC + blockL + "/* SPDX-FileCopyrightText: 2020 Somebody */\n", notices: []int{3}},
		{name: "block comment continuation", path: "a.ts", content: slashC + slashL + "/**\n * Copyright (c) Somebody\n */\n", notices: []int{4}},
		{name: "indented line comment", path: "a.go", content: slashC + slashL + "\nfunc f() {\n\t// Copyright notice\n}\n", notices: []int{5}},
		{name: "a line of a multi-line string read as a comment", path: "a.go", content: slashC + slashL + "\nconst s = `\n// Copyright 2020 Somebody\n`\n", notices: []int{5}},
		{name: "a line of a heredoc read as a comment", path: "a.sh", content: hashC + hashL + "\ncat <<'EOF'\n# SPDX-License-Identifier: Apache-2.0\nEOF\n", notices: []int{5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := judge(mustKind(t, tt.path), tt.content)
			if got.missing != tt.missing || !slices.Equal(got.notices, tt.notices) {
				t.Errorf("judge() = %+v, want missing %v and notices %v", got, tt.missing, tt.notices)
			}
		})
	}
}

// TestIsNotice_OnlyACommentOpeningWithTheStatement verifies what counts as a
// notice: a comment line whose text opens with "Copyright" or with an SPDX
// tag, and nothing that merely mentions either, quotes a comment or is not a
// comment at all.
func TestIsNotice_OnlyACommentOpeningWithTheStatement(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "// Copyright (c) 2026 Somebody", want: true},
		{line: "#Copyright Somebody", want: true},
		{line: "  # SPDX-License-Identifier: MIT", want: true},
		{line: "/* Copyright */", want: true},
		{line: " * SPDX-FileCopyrightText: Somebody", want: true},
		{line: "//Copyright", want: true},
		{line: "// Copyright", want: true},
		{line: "// Copyright, all rights reserved", want: true},
		{line: "// CopyrightStatement is the statement every file carries.", want: false},
		{line: "// Copyright_holder is a name.", want: false},
		{line: "// Copyright2026 is a name.", want: false},
		{line: "// Copyrighted material", want: false},
		{line: "// The Copyright line opens every file.", want: false},
		{line: "// copyright notices are kept", want: false},
		{line: "//\t// Copyright (c) 2026 Somebody", want: false},
		{line: "Copyright (c) 2019 Somebody", want: false},
		{line: `const c = "// Copyright"`, want: false},
		{line: "", want: false},
		{line: "//", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := isNotice(tt.line); got != tt.want {
				t.Errorf("isNotice(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestFix_WritesTheHeaderWhereTheKindPutsIt verifies what the fix writes into
// each kind of file: the two lines where the kind puts them, a blank line
// between them and what follows unless one is there already, an Astro
// frontmatter when the component has none, and everything else kept byte for
// byte, a missing trailing newline included.
func TestFix_WritesTheHeaderWhereTheKindPutsIt(t *testing.T) {
	tests := []struct {
		name, path, content, want string
	}{
		{name: "go package clause", path: "a.go", content: "package a\n", want: fileheader.Go + "package a\n"},
		{name: "go build constraint", path: "a.go", content: "//go:build e2e\n\npackage a\n", want: fileheader.Go + "//go:build e2e\n\npackage a\n"},
		{name: "go package doc comment", path: "a.go", content: "// Package a does.\npackage a\n", want: fileheader.Go + "// Package a does.\npackage a\n"},
		{name: "go opening with a blank line", path: "a.go", content: "\npackage a\n", want: slashC + slashL + "\npackage a\n"},
		{name: "go generated", path: "a.go", content: "// Code generated by x; DO NOT EDIT.\n\npackage a\n", want: fileheader.Go + "// Code generated by x; DO NOT EDIT.\n\npackage a\n"},
		{name: "empty file", path: "a.py", content: "", want: hashC + hashL},
		{name: "shell script", path: "a.sh", content: "#!/bin/sh\necho a\n", want: "#!/bin/sh\n" + hashC + hashL + "\necho a\n"},
		{name: "shebang alone", path: "a.sh", content: "#!/bin/sh\n", want: "#!/bin/sh\n" + hashC + hashL},
		{name: "shebang without a newline", path: "a.sh", content: "#!/bin/sh", want: "#!/bin/sh\n" + hashC + hashL[:len(hashL)-1]},
		{name: "makefile", path: "Makefile", content: ".PHONY: all\n", want: hashC + hashL + "\n.PHONY: all\n"},
		{name: "jq program", path: "a.jq", content: "# Derives a manifest.\n.name\n", want: hashC + hashL + "\n# Derives a manifest.\n.name\n"},
		{name: "python shebang and encoding", path: "a.py", content: "#!/usr/bin/env python3\n# coding: utf-8\n\"\"\"Doc.\"\"\"\n", want: "#!/usr/bin/env python3\n# coding: utf-8\n" + hashC + hashL + "\n\"\"\"Doc.\"\"\"\n"},
		{name: "python encoding without a shebang", path: "a.py", content: "# -*- coding: utf-8 -*-\nimport os\n", want: "# -*- coding: utf-8 -*-\n" + hashC + hashL + "\nimport os\n"},
		{name: "python encoding on the second line after a comment", path: "a.py", content: "# a\n# coding: latin-1\nimport os\n", want: "# a\n# coding: latin-1\n" + hashC + hashL + "\nimport os\n"},
		{name: "python encoding on the third line is a comment", path: "a.py", content: "#!/usr/bin/env python3\n# a\n# coding: utf-8\n", want: "#!/usr/bin/env python3\n" + hashC + hashL + "\n# a\n# coding: utf-8\n"},
		{name: "dockerfile directives", path: "Dockerfile", content: "# syntax=docker/dockerfile:1\n# escape=\\\n\n# Build stage\nFROM scratch\n", want: "# syntax=docker/dockerfile:1\n# escape=\\\n" + hashC + hashL + "\n# Build stage\nFROM scratch\n"},
		{name: "dockerfile of directives alone", path: "Dockerfile", content: "# syntax=docker/dockerfile:1", want: "# syntax=docker/dockerfile:1\n" + hashC + hashL[:len(hashL)-1]},
		{name: "dockerfile without directives", path: "Dockerfile", content: "# Build stage\nFROM scratch\n", want: hashC + hashL + "\n# Build stage\nFROM scratch\n"},
		{name: "powershell help", path: "a.ps1", content: "#!/usr/bin/env pwsh\n<#\n.SYNOPSIS\n#>\n", want: "#!/usr/bin/env pwsh\n" + hashC + hashL + "\n<#\n.SYNOPSIS\n#>\n"},
		{name: "javascript shebang", path: "cli.js", content: "#!/usr/bin/env node\n// cli.js runs.\n", want: "#!/usr/bin/env node\n" + slashC + slashL + "\n// cli.js runs.\n"},
		{name: "typescript", path: "a.ts", content: "import a from \"a\";\n", want: slashC + slashL + "\nimport a from \"a\";\n"},
		{name: "css", path: "a.css", content: "/* theme */\n.a {}\n", want: blockC + blockL + "\n/* theme */\n.a {}\n"},
		{name: "css charset", path: "a.css", content: "@charset \"UTF-8\";\n.a {}\n", want: "@charset \"UTF-8\";\n" + blockC + blockL + "\n.a {}\n"},
		{name: "astro frontmatter", path: "a.astro", content: "---\nconst a = 1;\n---\n<p>{a}</p>\n", want: "---\n" + slashC + slashL + "\nconst a = 1;\n---\n<p>{a}</p>\n"},
		{name: "astro empty frontmatter", path: "a.astro", content: "---\n---\n<p>a</p>\n", want: "---\n" + slashC + slashL + "\n---\n<p>a</p>\n"},
		{name: "astro without frontmatter", path: "a.astro", content: "<p>a</p>\n", want: "---\n" + slashC + slashL + "---\n<p>a</p>\n"},
		{name: "astro empty file", path: "a.astro", content: "", want: "---\n" + slashC + slashL + "---\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := mustKind(t, tt.path)
			got := fix(k, tt.content)
			if got != tt.want {
				t.Fatalf("fix() = %q, want %q", got, tt.want)
			}
			if v := judge(k, got); v.missing || len(v.notices) != 0 {
				t.Errorf("judge(fix()) = %+v, want what the fix wrote to pass", v)
			}
		})
	}
}
