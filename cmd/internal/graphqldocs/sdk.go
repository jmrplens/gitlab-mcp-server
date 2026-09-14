// The other half of this server's GraphQL surface: the documents client-go
// builds inside its own module, which nothing in this repository writes and
// every one of which still reaches GitLab through us.

package graphqldocs

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// SDKPatterns are the packages of client-go a document can live in, which is
// all of them: the operations sit beside the service methods that send them,
// in the module's root package, and the module is small enough that naming the
// files would be a list to keep current for no gain.
func SDKPatterns() []string { return []string{"./..."} }

// ForeignModuleEnv is the environment a load of a module this process does not
// own has to run under, appended to the caller's own.
//
// Both entries are about the module cache rather than about GraphQL, and both
// were found by a load that failed outright rather than by reasoning.
//
// GOWORK=off, because client-go ships a go.work in its module zip that names
// ./config, a directory the zip does not carry: in workspace mode the
// toolchain refuses to load anything there at all. Nothing about this
// repository's own build is affected, since a workspace of ours would be
// nobody's business during a load of somebody else's module.
//
// GOFLAGS=-mod=readonly, because a module cache directory is read-only and
// -mod=mod asks the toolchain for permission to write a go.mod it cannot. A
// developer or a build machine with -mod=mod exported (which is one way to run
// this repository's own tooling) would otherwise see this load fail with a
// message about workspace mode that says nothing about either cause.
func ForeignModuleEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=readonly")
}

// errNoSDKDirectory is an empty directory, which is refused rather than
// searched: loading "./..." from the empty string would load whatever the
// process's working directory happens to be, which during an audit is this
// repository, and reporting our own documents as the SDK's is the silent
// wrongness this whole audit exists to remove.
var errNoSDKDirectory = errors.New("no client-go directory to read documents from")

// SDKDocuments reads every GraphQL document the client-go module in dir builds,
// folded to the string GitLab receives.
//
// It is the same walk [Collect] runs over this repository, pointed at the
// module directory the audit already resolves for other questions, and it
// exists because that repository-only walk is a measured blind spot: the
// documents for the achievement, work item, saved view, security attribute,
// security category, scan profile, target branch rule and Terraform state
// services are assembled inside the SDK, sent through us, and judged by
// nothing except whichever of them a test happens to drive.
//
// Two differences from [Collect], each for a reason particular to reading a
// dependency.
//
// The standalone .graphql walk is left out. The only such files client-go
// ships are its own test fixtures under testdata, which nothing sends;
// counting them would put seven documents in the inventory that no caller can
// reach and, since several duplicate an operation the Go source also holds,
// would report the same operation twice under two provenances.
//
// A load failure is returned rather than absorbed, but every caller here
// reports instead of gating: this reads a module cache whose state it does not
// own, and a directory that has been evicted or a toolchain that refuses it is
// a fact about the machine and not about the server.
func SDKDocuments(dir string) ([]Document, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errNoSDKDirectory
	}
	loaded, err := goprogram.LoadWith(dir, SDKPatterns(), goprogram.Options{Env: ForeignModuleEnv()})
	if err != nil {
		return nil, fmt.Errorf("load the client-go source in %s: %w", dir, err)
	}
	return FromPackages(loaded), nil
}

// templateHole matches a placeholder a document is assembled around: a
// text/template action, or one of the printf verbs a format string carries.
//
// Both are written out rather than guessed at from a "%" alone, because a
// GraphQL document may legitimately hold one inside a string literal and a
// lone percent sign is not a hole.
//
// printf's space flag is deliberately left out of the flag class. It is legal
// ("% d") and nothing here writes one, while a percent followed by a space
// followed by a letter is how ordinary prose spells a percentage, so admitting
// it would excuse a real document ("100% coverage" in a search argument) from
// ever being judged. A hole excused is worse than a hole reported: the
// reported one is a row a reader dismisses once, and the excused one is a
// document nobody checks.
var templateHole = regexp.MustCompile(`\{\{[^}]*\}\}|%[#+\-0-9.]*[bcdeEfFgGoOpqstTUvxX]`)

// IsTemplate reports whether a document is a shell with holes in it rather
// than the text GitLab receives.
//
// client-go writes its documents in two shapes that are not sendable text.
// The work item list is a text/template whose folded value carries
// `{{ if .Decls }}` where a variable declaration list belongs. The Terraform
// state queries are printf format strings that interpolate the project path
// and the state name with %q, so what folds is a document with `%q` where a
// value goes.
//
// No schema can judge either, and both would be refused for the same reason on
// every single run. Reporting them as refusals would teach a reader to skip
// the refusal list, which is the one list here that must never be skipped, so
// they are counted apart, named, and left unjudged.
func IsTemplate(document Document) bool {
	return templateHole.MatchString(document.Text)
}
