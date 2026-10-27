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
// directive's frontend, each FROM that names neither scratch nor a stage an
// earlier FROM declares, each COPY --from that names neither a stage nor a
// stage's index, and each RUN --mount from= that names no stage, with the
// defaults of the ARGs declared before the first FROM substituted.
//
// The file is read as BuildKit reads it, since what the audit holds to a
// digest has to be what the build pulls: an instruction's keyword in any case
// (a lower-case from passed unread otherwise), the escape directive only among
// the known directives at the top, white space in front of it allowed, its
// character continuing a line unless doubled, a comment or empty line inside a
// continuation dropped, the syntax directive behind # or //, past a #! line,
// or as a JSON file's syntax member, a FROM bound to a stage only an earlier
// FROM declares while COPY --from looks one up in the whole file and in any
// case, and a heredoc's body skipped, since a body line that starts with FROM
// is not an instruction, with its words split at white space outside quotes.
// A variable no default sets, or one written in a form this audit does not
// substitute, is reported unresolved rather than read as a name, and a
// heredoc whose end the audit cannot name stops the reading rather than
// guessing which lines are its body.
func TestDockerfileImages_Dockerfile_NamesEveryImageTheBuildPulls(t *testing.T) {
	t.Parallel()

	digest := "@" + testDigest
	cases := []struct {
		name       string
		text       string
		want       []string
		unreadable string
	}{
		{name: "a FROM", text: "FROM alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{name: "a FROM in lower case", text: "from alpine:3\n", want: []string{"1: FROM alpine:3"}},
		{
			name: "a platform flag and a stage", text: "FROM --platform=$BUILDPLATFORM golang:1" + digest + " AS build\nFROM build\n",
			want: []string{"1: FROM golang:1" + digest},
		},
		{name: "scratch", text: "FROM scratch\n"},
		{name: "a stage declared after the FROM that names it, which names an image", text: "FROM base\nFROM alpine:3 as Base\n", want: []string{"1: FROM base", "2: FROM alpine:3"}},
		{name: "a FROM naming the stage it declares, which names an image", text: "FROM alpine AS alpine\n", want: []string{"1: FROM alpine"}},
		{
			name: "a FROM naming an earlier stage, as written and in another case", text: "FROM alpine" + digest + " AS base\nFROM base\nFROM Base\n",
			want: []string{"1: FROM alpine" + digest, "3: FROM Base"},
		},
		{name: "COPY --from a stage declared after it, in another case", text: "FROM x\nCOPY --from=Later / /\nFROM y AS later\n", want: []string{"1: FROM x", "3: FROM y"}},
		{
			name: "RUN --mount from= a number, which names an image rather than a stage",
			text: "FROM x AS s\nRUN --mount=type=bind,from=0,target=/m make\n", want: []string{"1: FROM x", "2: RUN --mount from=0"},
		},
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
		{
			name: "the syntax directive past a #! line", text: "#!/usr/bin/env -S docker build -f\n# syntax=evil/frontend:1\nFROM x\n",
			want: []string{"2: # syntax=evil/frontend:1", "3: FROM x"},
		},
		{name: "a #! line that is not the first, which ends the directives", text: "# syntax=a\n#!/x\n# syntax=b\nFROM x\n", want: []string{"1: # syntax=a", "4: FROM x"}},
		{name: "the syntax directive behind //", text: "// syntax=evil/frontend:1\nFROM x\n", want: []string{"1: // syntax=evil/frontend:1", "2: FROM x"}},
		{name: "the syntax directive's first word", text: "# syntax=docker/dockerfile:1 --flag\nFROM x\n", want: []string{"1: # syntax=docker/dockerfile:1", "2: FROM x"}},
		{
			name: "a syntax directive past one BuildKit does not know and behind white space", text: "# foo=bar\n \t# syntax=docker/dockerfile:1\nFROM x\n",
			want: []string{"2: # syntax=docker/dockerfile:1", "3: FROM x"},
		},
		{name: "a JSON file's syntax member", text: "{\"syntax\": \"evil/frontend:1\"}\n", want: []string{"1: \"syntax\": evil/frontend:1"}},
		{name: "a JSON file past a #! line", text: "#!/x\n{\"syntax\": \"evil/frontend:1\"}", want: []string{"2: \"syntax\": evil/frontend:1"}},
		{name: "a JSON file whose syntax member is not text", text: "{\"syntax\": 1}\n"},
		{name: "the escape directive behind white space", text: " \t# escape=`\nFROM x\nRUN echo \\\nFROM y\n", want: []string{"2: FROM x", "4: FROM y"}},
		{
			name: "an escape directive past one BuildKit does not know, which is a comment", text: "# foo=bar\n# escape=`\nFROM x\nRUN echo `\nFROM y\n",
			want: []string{"3: FROM x", "5: FROM y"},
		},
		{name: "an escape directive past a #! line, which is a comment", text: "#!/x\n# escape=`\nFROM x\nRUN echo `\nFROM y\n", want: []string{"3: FROM x", "5: FROM y"}},
		{name: "a doubled escape character, which ends the line", text: "FROM x\nRUN echo \\\\\nFROM y\n", want: []string{"1: FROM x", "3: FROM y"}},
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
		{name: "a heredoc on an instruction that reads none", text: "FROM x\nLABEL a=b <<EOF\nFROM y\nEOF\n", want: []string{"1: FROM x", "3: FROM y"}},
		{
			name: "another instruction shaped like FROM ... AS, which declares no stage",
			text: "FROM x\nRUN echo as tools\nCOPY --from=tools / /\n", want: []string{"1: FROM x", "3: COPY --from=tools"},
		},
		{name: "a heredoc on the first line", text: "RUN <<EOF\nFROM evil:1\nEOF\nFROM x\n", want: []string{"4: FROM x"}},
		{
			name: "<< inside quotes and after an escape, which opens no heredoc",
			text: "FROM x\nRUN echo 'a <<EOF' \"b \\\" <<EOF\" c\\ <<EOF\nFROM y\nEOF\n",
			want: []string{"1: FROM x", "3: FROM y"},
		},
		{
			name: "a backslash inside single quotes, which escapes nothing",
			text: "FROM x\nRUN echo 'a\\' <<EOF\nFROM evil:1\nEOF\nFROM y\n",
			want: []string{"1: FROM x", "5: FROM y"},
		},
		{
			name: "a heredoc after white space, a variable and an escaped space",
			text: "FROM x\n RUN  $X a\\ b <<EOF \nFROM evil:1\nEOF\nFROM y\n",
			want: []string{"1: FROM x", "5: FROM y"},
		},
		{
			name: "<< that opens no heredoc: at the end, after a dash, inside a word",
			text: "FROM x\nRUN a <<  \nFROM y\nRUN a <<- EOF\nFROM z\nRUN a<<EOF\nFROM w\nEOF\n",
			want: []string{"1: FROM x", "3: FROM y", "5: FROM z", "7: FROM w"},
		},
		{
			name: "a backslash the escape directive leaves at the end of a heredoc's line",
			text: "# escape=`\nFROM x\nRUN cat <<EOF \\\nFROM evil:1\nEOF\nFROM y\n",
			want: []string{"2: FROM x", "6: FROM y"},
		},
		{name: "ONBUILD with nothing after it", text: "FROM x\nONBUILD\nFROM y\n", want: []string{"1: FROM x", "3: FROM y"}},
		{name: "a << on ONBUILD that opens no heredoc", text: "FROM x\nONBUILD RUN echo '<<EOF'\nFROM y\n", want: []string{"1: FROM x", "3: FROM y"}},
		{name: "a heredoc on ONBUILD", text: "FROM x\nonbuild run <<EOF\nFROM evil:1\nEOF\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "a heredoc word holding a quote", text: "FROM x\nRUN cat <<E\"O\"F\nFROM evil:1\nEOF\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "a heredoc word holding a variable", text: "FROM x\nRUN cat <<EOF$X\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "an empty heredoc word", text: "FROM x\nRUN cat <<''\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "a variable beside <<", text: "FROM x\nRUN cat $X<<EOF\nFROM evil:1\nEOF\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "a << another word follows", text: "FROM x\nRUN cat << EOF\nFROM evil:1\nEOF\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "a quote left open beside <<", text: "FROM x\nRUN cat <<EOF \"open\nFROM y\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnread},
		{name: "a heredoc no line ends", text: "FROM x\nRUN \\\n  <<EOF\necho\n", want: []string{"1: FROM x"}, unreadable: "2: " + heredocUnterminated},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			images, unreadable := dockerfileImages(testCase.text)
			got := make([]string, 0, len(images))
			for _, image := range images {
				shown := fmt.Sprintf("%d: %s%s", image.line, image.instruction, image.reference)
				if !image.resolved {
					shown += " (unresolved)"
				}
				got = append(got, shown)
			}
			stopped := ""
			if unreadable.line > 0 {
				stopped = fmt.Sprintf("%d: %s", unreadable.line, unreadable.reason)
			}
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") || stopped != testCase.unreadable {
				t.Errorf("dockerfileImages() =\n%s\nunreadable %q, want\n%s\nunreadable %q",
					strings.Join(got, "\n"), stopped, strings.Join(testCase.want, "\n"), testCase.unreadable)
			}
		})
	}
}
