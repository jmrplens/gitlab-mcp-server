// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Tests for the reading of a container action's Dockerfile: which images the
// build it describes pulls, read as BuildKit's parser reads the file.
package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestDockerfileImages_Dockerfile_NamesEveryImageTheBuildPulls verifies which
// images dockerfileImages finds in a Dockerfile, and where: the syntax
// directive's frontend, each FROM that is neither scratch nor a stage, and
// each COPY --from and RUN --mount from= that names neither a stage nor a
// stage's index, with the defaults of the ARGs declared before the first FROM
// substituted.
//
// The file is read as BuildKit reads it, since what the audit holds to a
// digest has to be what the build pulls: an instruction's keyword in any case
// (a lower-case from passed unread otherwise), a parser directive only at the
// top, the escape directive's character continuing a line, a comment or empty
// line inside a continuation dropped, a stage named in any case or after the
// line that uses it, and a heredoc's body skipped, since a body line that
// starts with FROM is not an instruction. A variable no default sets, or one
// written in a form this audit does not substitute, is reported unresolved
// rather than read as a name.
func TestDockerfileImages_Dockerfile_NamesEveryImageTheBuildPulls(t *testing.T) {
	t.Parallel()

	digest := "@" + testDigest
	cases := []struct {
		name         string
		text         string
		want         []string
		unterminated int
	}{
		{name: "a FROM", text: "FROM alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{name: "a FROM in lower case", text: "from alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{
			name: "a platform flag and a stage", text: "FROM --platform=$BUILDPLATFORM golang:1" + digest + " AS build\nFROM build\n",
			want: []string{"1: FROM golang:1" + digest},
		},
		{name: "scratch", text: "FROM scratch\n"},
		{name: "a stage named after the line that uses it, in another case", text: "FROM base\nFROM alpine:3 as Base\n", want: []string{"2: FROM alpine:3"}},
		{name: "a FROM naming no image", text: "FROM\nFROM --platform=linux/amd64\n"},
		{name: "a FROM whose second word is not AS", text: "FROM alpine:3 to x\nFROM x\n", want: []string{"1: FROM alpine:3", "2: FROM x"}},
		{name: "a file of directives alone", text: "# syntax=docker/dockerfile:1", want: []string{"1: # syntax=docker/dockerfile:1"}},
		{name: "a --from flag on RUN, which reads none", text: "FROM x\nRUN --from=alpine:3 make\n", want: []string{"1: FROM x"}},
		{name: "a --mount flag on COPY, which reads none", text: "FROM x\nCOPY --mount=from=alpine:3 a b\n", want: []string{"1: FROM x"}},
		{
			name: "COPY --from a stage, an index and an image",
			text: "FROM alpine AS builder\nCOPY --from=builder /a /b\nCOPY --from=0 /a /b\nCOPY --chown=1 --from=alpine:3 /a /b\nCOPY a b\n",
			want: []string{"1: FROM alpine", "4: COPY --from=alpine:3"},
		},
		{
			name: "RUN --mount from a stage and an image",
			text: "FROM x AS s\nRUN --network=none --mount=type=bind,from=alpine:3,target=/m --mount=type=cache,target=/c --mount=type=bind,\"FROM=s\" make\n",
			want: []string{"1: FROM x", "2: RUN --mount from=alpine:3"},
		},
		{name: "a --mount in the command rather than a flag", text: "FROM x\nRUN make --mount=from=alpine:3\n", want: []string{"1: FROM x"}},
		{name: "a variable in a stage's source", text: "FROM x\nCOPY --from=$SOURCE / /\n", want: []string{"1: FROM x", "2: COPY --from=$SOURCE (unresolved)"}},
		{name: "ONBUILD, which runs in another build", text: "FROM x\nONBUILD COPY --from=alpine:3 / /\n", want: []string{"1: FROM x"}},
		{
			name: "the syntax directive", text: "# syntax=docker/dockerfile:1\n# check=skip=all\nFROM x\n",
			want: []string{"1: # syntax=docker/dockerfile:1", "3: FROM x"},
		},
		{name: "the syntax directive in any case", text: "#SYNTAX = docker/dockerfile:1 \nFROM x\n", want: []string{"1: # syntax=docker/dockerfile:1", "2: FROM x"}},
		{name: "a directive after an empty line, which is a comment", text: "\n# syntax=docker/dockerfile:1\nFROM x\n", want: []string{"3: FROM x"}},
		{name: "a continuation line", text: "FROM \\\n  alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{name: "white space after the escape character", text: "FROM \\  \n  alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{name: "a comment and an empty line inside a continuation", text: "FROM \\\n# a comment\n\n  alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{name: "the escape directive", text: "# escape=`\nFROM `\n  alpine:3\nFROM x \\\nFROM y\n", want: []string{"2: FROM alpine:3", "4: FROM x", "5: FROM y"}},
		{name: "the escape directive naming the default", text: "# escape=\\\nFROM \\\n  alpine:3\n", want: []string{"2: FROM alpine:3"}},
		{name: "a continuation the file ends in", text: "FROM alpine:3 \\\n", want: []string{"1: FROM alpine:3"}},
		{name: "lines holding only the escape character", text: "FROM x\n\\\n\\\n", want: []string{"1: FROM x"}},
		{name: "Windows line endings and a byte order mark", text: "\xef\xbb\xbf# syntax=docker/dockerfile:1\r\nFROM x\r\n", want: []string{"1: # syntax=docker/dockerfile:1", "2: FROM x"}},
		{
			name: "global ARG defaults",
			text: "ARG REG=\"ghcr.io\"\nARG IMG=${REG}/x" + digest + "\nFROM $IMG\nFROM ${IMG} AS y\nFROM y\n",
			want: []string{"3: FROM ghcr.io/x" + digest, "4: FROM ghcr.io/x" + digest},
		},
		{name: "an ARG with no default", text: "ARG BASE\nFROM $BASE\n", want: []string{"2: FROM $BASE (unresolved)"}},
		{name: "an ARG declared after the first FROM", text: "FROM x\nARG LATER=alpine:3\nFROM $LATER\n", want: []string{"1: FROM x", "3: FROM $LATER (unresolved)"}},
		{name: "an ARG no line declares", text: "FROM alpine:$TARGETARCH\n", want: []string{"1: FROM alpine:$TARGETARCH (unresolved)"}},
		{name: "an ARG whose default names one with no default", text: "ARG A\nARG B=$A\nFROM $B\n", want: []string{"3: FROM $B (unresolved)"}},
		{name: "a substitution modifier", text: "ARG B=x\nFROM ${B:-alpine:3}\n", want: []string{"2: FROM ${B:-alpine:3} (unresolved)"}},
		{name: "an unclosed brace", text: "ARG B=x\nFROM ${B\n", want: []string{"2: FROM ${B (unresolved)"}},
		{name: "a $ naming nothing", text: "FROM alpine$\nFROM ${}\n", want: []string{"1: FROM alpine$ (unresolved)", "2: FROM ${} (unresolved)"}},
		{
			name: "heredoc bodies",
			text: "FROM x\nRUN <<EOF\nFROM evil:1\nEOF\nCOPY <<-'END' 2<<\"TWO\" /x\n\tFROM evil:2\n\tEND\nFROM evil:3\nTWO\nRUN cat <<<FROM <<-\nFROM y\nRUN \\\n  <<EOF\nFROM evil:4\nEOF\nFROM z\n",
			want: []string{"1: FROM x", "11: FROM y", "16: FROM z"},
		},
		{name: "a heredoc on an instruction that reads none", text: "FROM x\nLABEL a=<<EOF\nFROM y\nEOF\n", want: []string{"1: FROM x", "3: FROM y"}},
		{name: "a heredoc on the first line", text: "RUN <<EOF\nFROM evil:1\nEOF\nFROM x\n", want: []string{"4: FROM x"}},
		{name: "a heredoc no line ends", text: "FROM x\nRUN \\\n  <<EOF\necho\n", want: []string{"1: FROM x"}, unterminated: 2},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			images, unterminated := dockerfileImages(testCase.text)
			got := make([]string, 0, len(images))
			for _, image := range images {
				shown := fmt.Sprintf("%d: %s%s", image.line, image.instruction, image.reference)
				if !image.resolved {
					shown += " (unresolved)"
				}
				got = append(got, shown)
			}
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") || unterminated != testCase.unterminated {
				t.Errorf("dockerfileImages() =\n%s\nunterminated %d, want\n%s\nunterminated %d",
					strings.Join(got, "\n"), unterminated, strings.Join(testCase.want, "\n"), testCase.unterminated)
			}
		})
	}
}
