// Package join joins what each action requests to what GitLab demands of a
// fine-grained token for each request, into the table the server carries.
//
// The derivation (package derive) says which REST routes and GraphQL
// documents an action sends and how they combine; the live record
// (docs/development/gitlab-api-live.json, schema 4) says, for the GitLab
// release it was taken from, which permissions at which boundary each route
// declares, which each GraphQL object type and mutation declares, and the
// vocabulary a token is granted in. This package reads both and writes a
// [finegrained.Table].
//
// # REST
//
// A derived route is matched to the record's routes after Grape's optional
// segments are expanded and every identifier is collapsed to a placeholder;
// a record placeholder may stand for a literal the derivation folded (an
// integration slug), never the other way round. The route's primary
// requirement and each additional scope become groups; a callable boundary is
// read as any of the boundary types it declares, or all four when it
// declares none, which over-approximates in the direction GitLab's own 403
// corrects. A skip reason leaves the route to GitLab, and a todo or a route
// declaring nothing is one no fine-grained token passes.
//
// # GraphQL
//
// A document is walked against the pinned schema, position by position, with
// each position's signature read from the record where it has one, since
// whether a denied position takes its parent with it is decided by the
// signature GitLab serves. The answer spine starts at the root field (for a
// mutation, at the first object its payload selects other than errors) and
// follows the one object a position selects while nothing else but connection
// framing is selected beside it. A position GitLab checks and that declares
// nothing is fatal when its null lands on the spine, directly or through a
// chain of non-null positions, and degraded otherwise; a declared position is
// judged on the spine the same way, so a grant failing it withholds the
// action, and off the spine it only empties a field. An abstract position is
// judged as the worst of the types GitLab may resolve there.
package join
