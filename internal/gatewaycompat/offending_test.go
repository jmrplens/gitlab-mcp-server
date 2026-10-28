// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gatewaycompat_test

import (
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/gatewaycompat"
)

// TestOffendingRune_IsTheServedProsePolicy pins the one predicate the listed
// catalog's character policy is judged by, which cmd/audit_gateway_chars
// scans every listing with and the message catalog's own policy test applies
// to the English it serves a person. ASCII passes whole, the control
// characters and DEL included, since the policy is about what a validator's
// character class refuses and not about what renders; the semicolon is the
// ASCII character a gateway refused; and everything above U+007F is refused
// as a class, from the first codepoint past ASCII to the last one Unicode
// has, so the next gateway's list of "unsafe characters" cannot surprise it.
// The punctuation above ASCII is written as code points (an em dash and an
// ellipsis), so this file carries none of it.
func TestOffendingRune_IsTheServedProsePolicy(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
	}{
		{r: 'a', want: false},
		{r: 'Z', want: false},
		{r: '0', want: false},
		{r: ' ', want: false},
		{r: '\n', want: false},
		{r: '\t', want: false},
		{r: '\x00', want: false},
		{r: ',', want: false},
		{r: ':', want: false},
		{r: '.', want: false},
		{r: '~', want: false},
		{r: '\u007f', want: false},
		{r: ';', want: true},
		{r: '\u0080', want: true},
		{r: 'é', want: true},
		{r: '¿', want: true},
		{r: rune(0x2014), want: true},
		{r: rune(0x2026), want: true},
		{r: '\U0001F600', want: true},
		{r: '\U0010FFFF', want: true},
	} {
		t.Run(fmt.Sprintf("%U", tc.r), func(t *testing.T) {
			if got := gatewaycompat.OffendingRune(tc.r); got != tc.want {
				t.Errorf("OffendingRune(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}
