// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// bots_test.go covers the declared bots: what each declaration has to state,
// and that the exemption needs the bot's identity and its login together.
package main

import (
	"strings"
	"testing"
)

// TestDeclaredBots_Declarations_StateAnIdentityALoginAndAReason pins the
// declarations: Dependabot alone, with the identity its commits carry and the
// login that opens its pull requests, and every declaration with an identity
// a trailer could name, an app's login and a reason.
func TestDeclaredBots_Declarations_StateAnIdentityALoginAndAReason(t *testing.T) {
	if len(declaredBots) != 1 {
		t.Fatalf("declaredBots holds %d bots, want Dependabot alone", len(declaredBots))
	}
	if got := declaredBots[0]; got.author != dependabot || got.login != "dependabot[bot]" {
		t.Errorf("declaredBots[0] = %v as %q, want %v as dependabot[bot]", got.author, got.login, dependabot)
	}
	for _, bot := range declaredBots {
		t.Run(bot.login, func(t *testing.T) {
			if parsed, ok := parseIdentity(bot.author.String()); !ok || parsed != bot.author {
				t.Errorf("author %q does not read back as an identity: %#v, %v", bot.author, parsed, ok)
			}
			if !strings.HasSuffix(bot.login, "[bot]") {
				t.Errorf("login %q is not an app's login, which GitHub ends with [bot]", bot.login)
			}
			if !strings.Contains(bot.reason, "cannot certify") {
				t.Errorf("reason %q does not say why the bot cannot certify the DCO", bot.reason)
			}
		})
	}
}

// TestExemptingBot_Commits_AreExemptOnlyWhereTheIdentityAndTheLoginAgree
// verifies that a commit is exempt when it carries a declared bot's identity
// in a pull request that bot opened, the login compared without regard to
// case as GitHub compares logins, and in no other case.
func TestExemptingBot_Commits_AreExemptOnlyWhereTheIdentityAndTheLoginAgree(t *testing.T) {
	tests := []struct {
		name     string
		author   identity
		openedBy string
		want     bool
	}{
		{name: "bot in its own pull request", author: dependabot, openedBy: "dependabot[bot]", want: true},
		{name: "login in another case", author: dependabot, openedBy: "Dependabot[bot]", want: true},
		{name: "identity in another case", author: identity{name: "Dependabot[bot]", email: strings.ToUpper(dependabot.email)}, openedBy: "dependabot[bot]", want: true},
		{name: "bot in a person's pull request", author: dependabot, openedBy: "jmrplens"},
		{name: "bot with nobody named as the opener", author: dependabot},
		{name: "person in the bot's pull request", author: jane, openedBy: "dependabot[bot]"},
		{name: "bot's name with another email", author: identity{name: dependabot.name, email: "support@github.com"}, openedBy: "dependabot[bot]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bot, ok := exemptingBot(tc.author, tc.openedBy)
			if ok != tc.want {
				t.Errorf("exemptingBot(%v, %q) = %v, want %v", tc.author, tc.openedBy, ok, tc.want)
			}
			if ok && bot.login != "dependabot[bot]" {
				t.Errorf("exemptingBot(%v, %q) = %q, want dependabot[bot]", tc.author, tc.openedBy, bot.login)
			}
			if !ok && bot != (declaredBot{}) {
				t.Errorf("exemptingBot(%v, %q) = %+v with no exemption, want the zero declaration", tc.author, tc.openedBy, bot)
			}
		})
	}
}

// TestClaimedBot_Authors_NameTheBotWhoseIdentityTheyCarry verifies that a
// commit carrying a declared bot's identity is told apart from a person's,
// whoever opened its pull request.
func TestClaimedBot_Authors_NameTheBotWhoseIdentityTheyCarry(t *testing.T) {
	if bot, ok := claimedBot(dependabot); !ok || bot.login != "dependabot[bot]" {
		t.Errorf("claimedBot(%v) = %q, %v; want dependabot[bot], true", dependabot, bot.login, ok)
	}
	if bot, ok := claimedBot(jane); ok || bot != (declaredBot{}) {
		t.Errorf("claimedBot(%v) = %+v, %v; want the zero declaration, false", jane, bot, ok)
	}
}
