# Copyright and License Headers

**Every source file of this repository opens with two lines naming its copyright holder and its license**, and a gate fails a file that does not. This page says what the lines are, which files carry them and where, how a generated file gets them, and what to do with a file this repository did not write.

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

## The two lines

```go
// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT
```

The first is the copyright statement, naming the holder [LICENSE](../../LICENSE) names; the second names the license LICENSE grants in the form [SPDX](https://spdx.dev/learn/handling-license-info/) defines for a comment near the top of a file. They answer the OpenSSF Best Practices criteria `copyright_per_file` and `license_per_file`, which ask every source file to say both, so that a file copied out of the repository still says whose it is and under what terms.

The year is the one LICENSE carries and it is the same in every file: the lines are a notice for the project, not a record of when each file was written. Both are constants in `cmd/internal/fileheader`, `CopyrightStatement` and `LicenseStatement`, beside `Go`, the form a Go file opens with.

## Which files

The files git holds in its index and the untracked ones it would add, so what `.gitignore` keeps out of the repository (`plan/`, `dist/`, `site/node_modules`) is never judged. Of those, the source files:

| Kind                        | Files                                                                | Comment   | Kept above the header                                 |
| --------------------------- | -------------------------------------------------------------------- | --------- | ----------------------------------------------------- |
| Go                          | `*.go`, `testdata` included                                          | `//`      | nothing: the header comes before any build constraint |
| Shell, PowerShell, jq, Make | `*.sh`, `*.bash`, `*.ps1`, `*.psm1`, `*.jq`, `*.mk`, `Makefile`      | `#`       | a shebang                                             |
| Python, Ruby                | `*.py`, `*.rb`                                                       | `#`       | a shebang, and an encoding declaration on line 1 or 2 |
| Dockerfile                  | `Dockerfile`                                                         | `#`       | the parser directives (`syntax`, `escape`, `check`)   |
| JavaScript, TypeScript      | `*.js`, `*.mjs`, `*.cjs`, `*.jsx`, `*.ts`, `*.mts`, `*.cts`, `*.tsx` | `//`      | a shebang                                             |
| CSS                         | `*.css`                                                              | `/* … */` | a `@charset` rule                                     |
| Astro                       | `*.astro`                                                            | `//`      | the `---` fence that opens the frontmatter            |

What stays above the header is what stops working anywhere else: the kernel reads a shebang only as a file's first line, Python and Ruby read an encoding declaration only on the first two, BuildKit stops reading parser directives at the first comment, a browser honors `@charset` only as a stylesheet's first bytes, and a comment in an Astro component's template would be rendered into the page, so the header lives in its frontmatter.

Documentation, data and configuration are not source and carry no header: Markdown and MDX, JSON, YAML (the workflows included), TOML, the pinned GraphQL schema, plain text, SVG and the images. Neither do symbolic links, which git lists but which point at a file judged in its own right.

A Go file looks like this, a blank line keeping the header apart from what follows:

```go
// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// Package harness is the one route from a test to a server.
package harness
```

Without the blank line the header would be read as the package's doc comment, which `cmd/godoc_tool` refuses, or would join the `// Code generated … DO NOT EDIT.` comment of a generated file, which Go's tools still find but a reader would not expect.

## The gate and the fix

`make check-file-headers` (`cmd/audit_file_headers`) fails on a source file that does not open with the two lines where its kind puts them, and at every comment line carrying another copyright or license notice: a comment whose text opens with the word `Copyright` or with an SPDX tag, such as a second copy of the header or one written below the package doc comment. The word has to end there, so a doc comment opening with an identifier such as `CopyrightStatement` is not a notice, while one opening with the word itself is, and is reworded. Each line is read alone, without the syntax of the file around it, so a line of a multi-line string or a heredoc that opens with a comment marker is read as a comment too. It is step 32 of `make analyze` and a step of the `generated` job of `ci.yml`, which runs on every pull request and every push to `main`. It reads git's file list, so it runs in a checkout rather than in a copy of the tree without its `.git`.

`make fix-file-headers` writes the header into every file that does not open with it and carries no other notice, after what its kind keeps above it and followed by a blank line, keeping every other byte and the file's mode. That is how a new file gets it, though writing the two lines by hand is as good. The command's flags and output are in the [command reference](cmd-utilities.md#audit_file_headers).

## Generated files

A generated file carries the header because its generator writes it, never by an edit of the output: `cmd/gen_action_catalog_manifest`, `cmd/gen_action_grants` and `cmd/gen_brand` open the Go file each writes with `fileheader.Go`, before the `// Code generated` line, and each has a test holding it there (`TestGenerateManifest_OpensWithTheFileHeader`, `TestRenderTable_OpensWithTheFileHeader`, `TestBrandMarkGo_OpensWithTheFileHeader`). A new generator that writes source does the same; the gate fails its output otherwise, the first time it is regenerated.

The other generators write documentation and data (the tool reference, the `llms*.txt` files, JSON records, SVG and WebP assets), which carry no header.

## A file this repository did not write

None is in the tree today: the module vendors nothing and the site's dependencies stay in `site/node_modules`, which git ignores. A file somebody else wrote keeps the notice it came with, so the gate never stamps this project's header on it: its own notice fails the gate at its line, and the fix mode leaves the file alone. Bringing one in is therefore a change to the gate as well, declaring that file with the reason it is kept as it came, in the change that adds it.

## Changing the holder or the year

The header is the two constants of `cmd/internal/fileheader`, LICENSE, and the two lines in every file. A change to the constants makes every existing header a notice other than the new one, which the fix mode does not rewrite, so the change is a bulk rewrite of those two lines across the tree, verified with `make check-file-headers` and the build, in the same pull request as the constants and LICENSE.

## How the header reached every file

The commit that added it to the tree was made by the fix mode, over the files as they stood on the branch it was made on, with one edit by hand: `cmd/audit_test_closed_servers` pins the line of every finding in its planted module, and the header moves each of them three lines down. Everything else the header would have broken or left pointing at the wrong line was changed in the commit before it, the one that adds the gate, so that making the rewrite again does not lose it: the help of `scripts/setup-orbit-fixtures.sh`, which printed a fixed range of the script's own lines and now prints its usage comment by pattern; three quoted compiler messages that name a line, in the testing README, `upstream-bugs.md` and `scripts/coverage-conditions.sh`; a comment in the end-to-end suite that named a line of `internal/tools/catalog_filter.go` and now names the declaration; and the header of `cmd/gen_action_grants/table_stub.go.txt`, the Go source the action grants generator compiles in its output's place, under a `.txt` name the gate does not read.

Three lines inserted at the top of a file conflict only with a change to that file's first lines, so the rewrite conflicts with few pull requests. What it asks of every one of them is the header in each source file it adds, which the gate fails without and `make fix-file-headers` writes. It lands by being made again on `main` as it is at that moment rather than rebased, because made again it is exactly the fix mode's output over that tree, with no conflict to resolve and no file `main` added since left without the header: the commit with the gate rebased as usual, then `make fix-file-headers`, then the positions in `cmd/audit_test_closed_servers` moved again, which `go test ./cmd/audit_test_closed_servers/` names, as `go test ./...` would name any other test that counts the lines of a file. It is meant to land with no other stack open, and a pull request still open at that moment that adds a source file fails the gate until `make fix-file-headers` gives that file its header.
