package gitlab

import (
	"fmt"
	"net/http"
	"strings"
)

// StreamArchive builds its path with fmt.Sprintf and reassigns it from itself
// under a condition.
func (s *RepositoriesService) StreamArchive(pid any, opt *ArchiveOptions, options ...RequestOptionFunc) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/archive", PathEscape(project))
	if opt != nil && opt.Format != nil {
		u = fmt.Sprintf("%s.%s", u, *opt.Format)
	}
	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

// FormatPackageURL computes a path through a helper whose failure branch
// returns an empty string.
func (s *GenericPackagesService) FormatPackageURL(pid any, name, version, fileName string) (string, error) {
	escaped, err := escapeFileName(fileName)
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("projects/%s/packages/generic/%s/%s/%s", PathEscape(pid), PathEscape(name), PathEscape(version), escaped)
	return u, nil
}

func escapeFileName(name string) (string, error) {
	if name == "" {
		return "", errEmpty
	}
	run := func() string { return "ignored" }
	_ = run
	return strings.Join([]string{name}, "/"), nil
}

// PublishPackageFile builds its request under one verb and sends it under
// another.
func (s *GenericPackagesService) PublishPackageFile(pid any, name, version, fileName string) (*Upload, *Response, error) {
	u, err := s.FormatPackageURL(pid, name, version, fileName)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.NewRequest(http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Method = http.MethodPut
	resp, err := s.client.Do(req, nil)
	return nil, resp, err
}

// Fetch hands a parameter through whole, which folds to no static segment, and
// names its verb through a parameter, which is no net/http constant.
func (s *RepositoriesService) Fetch(path, method string) (*Upload, *Response, error) {
	var req struct{ Method string }
	req.Method = method
	s.client.NewRequest(http.MethodGet, path, nil, nil)
	s.client.NewRequest(method, "projects", nil, nil)
	s.client.NewRequest(http.MethodGet)
	s.client.NewRequest(http.MethodDelete, "two/args")
	s.client.UploadRequest(http.MethodPost, ("projects/" + "uploads"), nil, nil, nil)
	return nil, nil, nil
}

// Verbs writes a net/http verb where no request reads it: into a variable
// that is no request's Method field, into a Method field beside a second
// target, and into a Method field from one of two values. Go would refuse the
// last two, but the reading only parses, and none of them is the verb a
// request is sent with.
func (s *RepositoriesService) Verbs() (*Upload, *Response, error) {
	var req, other struct{ Method string }
	chosen := http.MethodPatch
	req.Method, other.Method = http.MethodPut
	req.Method = http.MethodDelete, http.MethodPost
	_ = chosen
	s.client.NewRequest(http.MethodGet, "verbs", nil, nil)
	return nil, nil, nil
}

// Folds spells every expression shape the path folding reads.
func (s *RepositoriesService) Folds(id string) (*Upload, *Response, error) {
	var holder struct{ path string }
	holder.path = "ignored"
	value, ok := lookup[id]
	_ = value
	_ = ok
	formatted := fmt.Sprintf()
	printed := fmt.Println("x")
	nested := s.client.fmt.Sprintf("x")
	aliased := strs.Sprintf("x")
	product := 2 * 3
	recursive := recurse("x")
	empty := noReturn()
	literal := func() string { return "literal" }()
	prefix := "prefixed"
	_, second := pair()
	bare := bareReturn()
	s.client.NewRequest(http.MethodGet, "projects/"+id, nil, nil)
	s.client.NewRequest(http.MethodGet, prefix+"/"+id, nil, nil)
	s.client.NewRequest(http.MethodGet, "pairs/"+second+"/"+bare, nil, nil)
	s.client.NewRequest(http.MethodGet, fmt.Sprintf("%%/%-5s/%d/%s", "groups", 7), nil, nil)
	s.client.NewRequest(http.MethodGet, fmt.Sprintf("ends/%"), nil, nil)
	s.client.NewRequest(http.MethodGet, formatted+printed+nested+aliased, nil, nil)
	s.client.NewRequest(http.MethodGet, "products/"+product, nil, nil)
	s.client.NewRequest(http.MethodGet, "recursive/"+recursive+"/"+empty+"/"+literal, nil, nil)
	s.client.NewRequest(http.MethodGet, value, nil, nil)
	s.client.NewRequest(http.MethodGet, answerPath, nil, nil)
	s.client.NewRequest(http.MethodGet, s.relative(id), nil, nil)
	s.client.NewRequest(http.MethodGet, "many/"+choice()+"/"+choice()+"/"+choice()+"/"+choice()+"/"+choice(), nil, nil)
	return nil, nil, nil
}

func recurse(value string) string {
	return recurse(value)
}

func noReturn() {}

func pair() (string, string) {
	return "one", "two"
}

func bareReturn() (s string) {
	s = "named"
	return
}

func (s *RepositoriesService) relative(id string) string {
	return "relative/" + id
}

func choice() string {
	if a {
		return "a"
	}
	if b {
		return "b"
	}
	if c {
		return "c"
	}
	return "d"
}

const answerPath = "answers"

// Do is the GraphQL transport: its request has an empty path and is no route.
func (g *GraphQLService) Do(query any, response any, options ...RequestOptionFunc) (*Response, error) {
	request, err := g.client.NewRequest(http.MethodPost, "", query, options)
	if err != nil {
		return nil, err
	}
	return g.client.Do(request, response)
}

// ListAchievements names its document, and calls a helper of the package
// after it, so the walk visits a body once the method is known to send
// GraphQL.
func (s *AchievementsService) ListAchievements() ([]*Upload, *Response, error) {
	noReturn()
	query := GraphQLQuery{Query: listAchievementsQuery}
	return nil, s.client.GraphQL.Do(query, nil)
}

// DescribeAchievements names a document and sends a REST request: naming a
// document is not sending it, so none is attributed to this method.
func (s *AchievementsService) DescribeAchievements() (*Upload, *Response, error) {
	_ = listAchievementsQuery
	s.client.NewRequest(http.MethodGet, "achievements", nil, nil)
	return nil, nil, nil
}

// InlineAchievement writes its document inline.
func (s *AchievementsService) InlineAchievement() (*Upload, *Response, error) {
	query := GraphQLQuery{Query: `query { inlineInBody }`}
	return nil, s.client.GraphQL.Do(query, nil)
}

// GetWorkItem reaches its documents through the chained templates only.
func (s *WorkItemsService) GetWorkItem() (*Upload, *Response, error) {
	var builder strings.Builder
	getWorkItemTemplate.Execute(&builder, nil)
	_ = cycleA
	return nil, s.client.GraphQL.Do(builder.String(), nil)
}

// NotGraphQL calls a method named Do on something that is not the GraphQL
// transport.
func (s *WorkItemsService) NotGraphQL() (*Upload, *Response, error) {
	return nil, s.client.Do(nil, nil)
}
