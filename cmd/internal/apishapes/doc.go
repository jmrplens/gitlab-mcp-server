// Package apishapes is what GitLab says its own REST API accepts and returns.
//
// It exists because every oracle this repository had for that question was
// indirect. The 1:1 audit compares our input and output structs against
// client-go's, which describes what the SDK models rather than what GitLab
// serves, and lags it by however long a release takes. R-PATH compares our
// endpoints against GitLab's documentation pages, which are prose: an endpoint
// documented under doc/user, or in a sentence rather than a code block, looks
// exactly like one that does not exist. Neither can answer the question that
// produced https://github.com/jmrplens/gitlab-mcp-server/issues/580, which is
// whether GitLab sends the fields our output type publishes.
//
// GitLab generates an OpenAPI 3 document from the Grape definitions that render
// its responses, commits it to its own repository, and serves it unauthenticated
// at a raw URL. That document is the direct answer: 1847 operations, 867
// component schemas, a named response schema on 1294 of them and a request body
// on 667. It needs no running instance, no license, and no image, and because
// gitlab-org/gitlab is the Enterprise codebase it covers the Premium and
// Ultimate surface that a Community Edition instance would never reveal.
//
// The artifact this package reads and writes is the extraction rather than the
// document: for each operation, the property names of its success response, the
// names of its path and query parameters, and the property names of its request
// body. Those are the three lists a comparison with our own types needs, and
// keeping the extraction rather than the 3.7 MB source keeps a re-pin a
// readable diff.
package apishapes
