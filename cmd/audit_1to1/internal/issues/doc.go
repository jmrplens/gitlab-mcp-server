// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package issues holds every row of the 1:1 audit that answers a finding
// with an issue of this repository to that issue still being open (issue
// 1688, P17).
//
// A tracked row is a promise: a route of R-ROUTE's family table, or an
// identifier of the sdk scope's declarations, answered by an issue that says
// the work is planned. The declaration tables go stale when what they answer
// changes, and nothing asked about the issue: closed as done with the route
// still unsent, or closed as not planned, the row went on answering a finding
// in the name of work nobody meant to do.
//
// The rows are read from the reports that publish them, the routes scope's
// answered routes and the sdk scope's identifier arguments, each of which
// carries the issue its declaration names. A row whose category is tracked
// and that names no issue is an error, since it could not be judged. Each
// issue is asked of GitHub's REST API once, however many rows name it.
//
// The check fails on an issue that is closed, that is a pull request, or
// that the repository does not hold, naming the rows that rest on it, and on
// one it could not ask about: a refused token, a rate limit and an unreachable
// network are a failure to judge and never a pass.
package issues
