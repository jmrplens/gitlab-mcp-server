// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import "strings"

// declaredBot is an app whose commits are exempt from the sign-off in a pull
// request it opened: the author identity its commits carry, the GitHub login
// that opens its pull requests, and why it is exempt.
type declaredBot struct {
	author identity
	login  string
	reason string
}

// declaredBots are the apps exempt from the sign-off. A declaration needs the
// identity and the login both, because the exemption holds only where they
// agree: a commit carrying the identity in a pull request opened by anybody
// else is judged like any other, since writing an app's name into a commit is
// something anybody can do and opening a pull request as the app is not.
//
// The release workflow's version stamp is not declared here: it is pushed to
// main after a release and never sits in a pull request, which is the only
// place this command reads commits.
var declaredBots = []declaredBot{
	{
		author: identity{name: "dependabot[bot]", email: "49699333+dependabot[bot]@users.noreply.github.com"},
		login:  "dependabot[bot]",
		reason: "Dependabot is an app and cannot certify anything; what it commits is the version bump .github/dependabot.yml asks for, of dependencies licensed by their own authors",
	},
}

// exemptingBot returns the declared bot that exempts a commit by author in a
// pull request openedBy opened, if there is one. The login is compared
// without regard to case, as GitHub compares logins.
func exemptingBot(author identity, openedBy string) (declaredBot, bool) {
	for _, bot := range declaredBots {
		if strings.EqualFold(bot.login, openedBy) && sameIdentity(bot.author, author) {
			return bot, true
		}
	}
	return declaredBot{}, false
}

// claimedBot returns the declared bot whose identity author carries, whoever
// opened the pull request, if there is one.
func claimedBot(author identity) (declaredBot, bool) {
	for _, bot := range declaredBots {
		if sameIdentity(bot.author, author) {
			return bot, true
		}
	}
	return declaredBot{}, false
}
