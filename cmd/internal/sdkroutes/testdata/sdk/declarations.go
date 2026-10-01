// Package gitlab is a stand-in for client-go's root package: parsed by the
// tests of sdkroutes and never compiled, so it spells each shape the reader has
// to handle once, the odd ones included, rather than being a working client.
package gitlab

import (
	"bytes"
	"text/template"
)

var (
	routeProjectsIDIssuesID          = route("projects/%s/issues/%d")
	routeProjectsIDIssues            = route("projects/%s/issues")
	routeProjectsIDIDIDAwardEmojiID  = route("projects/%s/%s/%d/award_emoji/%d")
	routeProjectsIDIDIDAddSpentTime  = route("projects/%s/%s/%d/add_spent_time")
	routeIDIDUploads                 = route("%s/%s/uploads")
	routeProjectsIDRepositoryArchive = route("projects/%s/repository/archive%s")
	routeSearch                      = route("search?scope=%s")
	routeProjectsIDPackagesID        = route("projects/%s/packages/%s.%s")
	routeOnlyPlaceholders            = route("%s/%s")
	notARoute                        = other("projects")
	tooManyArgs                      = route("projects", "issues")
	notALiteral                      = route(routeSearch)
	pickedFromAList                  = routes[0]
	firstOfTwo, secondOfTwo          = route("first")
)

const (
	awardIssue    = "issues"
	awardSnippets = "snippets"
	answer        = 42
)

// ResourceType is a typed string, which is how client-go spells a resource
// segment a generic helper is handed.
type ResourceType string

const ProjectResource ResourceType = "projects"

// listAchievementsQuery is a named document a body names.
const listAchievementsQuery = `query { achievements { nodes { id } } }`

// templateSource is a named document only a template variable names.
const templateSource = `query { workItem { id } }`

// workItemTemplate is parsed out of a named document and an inline one.
var workItemTemplate = template.Must(template.New("item").Parse(templateSource + `
query { inlineInInitializer }
`))

// getWorkItemTemplate is chained from workItemTemplate, the shape the work item
// templates are written in.
var getWorkItemTemplate = template.Must(template.Must(workItemTemplate.Clone()).New("get").Parse(`query { chained }`))

// unrelatedTemplate is a variable no sending body reaches.
var unrelatedTemplate = template.Must(template.New("unrelated").Parse(`query { unrelated }`))

// declaredWithoutValue holds nothing a reader could follow.
var declaredWithoutValue int

// cycleA and cycleB name each other, which the initializer closure must not
// follow forever.
var (
	cycleA = cycleB
	cycleB = cycleA
)

// Client is the stand-in client; none of its fields is a service of the kind a
// method delegates to.
type Client struct {
	GraphQL *GraphQLService
	client  *Client
	bytes.Buffer
}

// IssuesService carries helpers in fields, one a service and one not.
type IssuesService struct {
	client    *Client
	timeStats *timeStatsService
	other     *Helper
}

type timeStatsService struct {
	client *Client
}

type (
	RepositoriesService     struct{ client *Client }
	AwardEmojiService       struct{ client *Client }
	GenericPackagesService  struct{ client *Client }
	AchievementsService     struct{ client *Client }
	WorkItemsService        struct{ client *Client }
	GraphQLService          struct{ client *Client }
	ProjectUploadsService   struct{ client *Client }
	Service                 struct{ client *Client }
	internalService         struct{ client *Client }
	Helper                  struct{}
	Issue                   struct{}
	AwardEmoji              struct{}
	TimeStats               struct{}
	Upload                  struct{}
	Response                struct{}
	ProjectID               struct{ value any }
	ListIssuesOptions       struct{}
	AddSpentTimeOptions     struct{}
	ExtraOptions            struct{}
	ArchiveOptions          struct{ Format *string }
	Generic[T any]          struct{}
	Pair[K comparable, V any] struct{}
	Count                   = int
)
