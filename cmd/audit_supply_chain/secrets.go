package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// writesTableFile is where a finding about the table of what each secret can
// write sends its reader.
const writesTableFile = "cmd/audit_supply_chain/secrets.go"

// secretDeclaration says what one repository secret can do in the hands of
// the job that reads it.
//
// writes decides the rule: a job reading a secret that can write is
// credentialed whatever its permissions block says, because the code it runs
// holds that write as surely as it would hold a GITHUB_TOKEN granted
// contents: write. where is the sentence a reviewer checks: what the secret
// can write and to whom, or why it writes nothing.
type secretDeclaration struct {
	where  string
	writes bool
}

// declaredSecrets are the secrets the workflows read, each with what it can
// write.
//
// Every secret a workflow reads has to be here, since one that is not could
// be a write nobody judged, and an entry no workflow reads any longer is
// reported as stale, on the terms every declaration table in this repository
// is held to. The names are GitHub's, which are case-insensitive and stored
// upper case, so an expression is matched in upper case too.
var declaredSecrets = map[string]secretDeclaration{
	"DOCKERHUB_TOKEN": {
		writes: true,
		where: "pushes, overwrites and deletes the images of the Docker Hub repository jmrplens/gitlab-mcp-server " +
			"and rewrites its description: a personal access token with the read, write and delete scope, " +
			"which is the narrowest the descriptions API accepts",
	},
	"DOCKERHUB_USERNAME": {
		where: "the account name the Docker Hub login pairs with DOCKERHUB_TOKEN, and no credential on its own",
	},
	"GITHUB_TOKEN": {
		where: "the job's own token, which can write only what the job's permissions grant; " +
			"those are judged as permissions, so naming the token here adds nothing to them",
	},
	"GITLAB_API_TOKEN": {
		writes: true,
		where:  "creates and edits the releases of the GitLab.com mirror project, and creates, edits and deletes their asset links",
	},
	"GITLAB_MIRROR_SSH_KEY": {
		writes: true,
		where:  "force-pushes and prunes every branch and tag of the GitLab.com mirror",
	},
	"INDEXNOW_KEY": {
		where: "asks search engines to recrawl pages that are already public; IndexNow checks the key " +
			"against a copy published at the site's root, so it proves nothing a reader of the site does not hold",
	},
	"RELEASE_DEPLOY_KEY_B64": {
		writes: true,
		where: "pushes the release stamp to main as a deploy key, which the Protect main ruleset " +
			"lets past its required review (SC-01 in docs/development/repository-settings.md)",
	},
	"SONAR_TOKEN": {
		writes: true,
		where: "publishes analyses to the SonarCloud project, and with them the quality gate verdict " +
			"the project reports for a branch or a pull request",
	},
	"TAP_DEPLOY_KEY_B64": {
		writes: true,
		where:  "pushes the formula to jmrplens/homebrew-tap, which every brew install of this server reads",
	},
	"WINGET_TOKEN": {
		writes: true,
		where: "pushes branches to the jmrplens/winget-pkgs fork and opens pull requests against " +
			"microsoft/winget-pkgs under the maintainer's account",
	},
}

// repositoryTables are the tables this repository's own audit judges by.
func repositoryTables() tables {
	return tables{secrets: declaredSecrets, declarations: declaredRunTimeCode}
}

// workflowExpression finds the ${{ }} expressions of a workflow value, the
// only place the secrets context can be read.
var workflowExpression = regexp.MustCompile(`(?s)\$\{\{(.*?)\}\}`)

// secretRead finds the secrets context inside an expression: secrets.NAME,
// secrets['NAME'], or the context alone, which is every secret at once.
var secretRead = regexp.MustCompile(`\bsecrets\b(?:\s*\.\s*([A-Za-z_][A-Za-z0-9_]*)|\s*\[\s*'([^']+)'\s*\])?`)

// jobSecrets returns the secrets a job reads, each once and sorted, from its
// own values and from the workflow's env block, which every job inherits; and
// apart from them each expression that reads the whole secrets context, which
// no table entry can describe.
func jobSecrets(doc, job map[string]any) (names, wholesale []string) {
	var values []string
	collectStrings(doc["env"], &values)
	collectStrings(job, &values)
	read := map[string]bool{}
	whole := map[string]bool{}
	for _, value := range values {
		for _, expression := range workflowExpression.FindAllStringSubmatch(value, -1) {
			for _, reference := range secretRead.FindAllStringSubmatch(expression[1], -1) {
				name := reference[1] + reference[2]
				if name == "" {
					whole[strings.TrimSpace(expression[1])] = true
					continue
				}
				read[strings.ToUpper(name)] = true
			}
		}
	}
	return sortedKeys(read), sortedKeys(whole)
}

// collectStrings gathers every string a decoded YAML value holds, however
// deep.
func collectStrings(value any, into *[]string) {
	switch typed := value.(type) {
	case string:
		*into = append(*into, typed)
	case map[string]any:
		for _, nested := range typed {
			collectStrings(nested, into)
		}
	case []any:
		for _, nested := range typed {
			collectStrings(nested, into)
		}
	}
}

// sortedKeys returns the keys of a set in sorted order.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// secretProblems holds the secret table to the workflows the audit read: an
// entry that says nothing about what its secret can write, and one no workflow
// reads any longer.
func (a *supplyChainAudit) secretProblems() []string {
	names := make([]string, 0, len(a.secrets))
	for name := range a.secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []string
	for _, name := range names {
		if strings.TrimSpace(a.secrets[name].where) == "" {
			problems = append(problems, fmt.Sprintf("%s: %s says nothing about what it can write", writesTableFile, name))
		}
		if !a.secretsRead[name] {
			problems = append(problems, fmt.Sprintf("%s: %s is declared and no workflow reads it", writesTableFile, name))
		}
	}
	return problems
}
