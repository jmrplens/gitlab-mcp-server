// Command gen_tool_reference writes the per-domain tool reference of the
// documentation site: one page per catalog group, in English under
// site/src/content/docs/reference/tools/ and in Spanish under its es/ twin,
// plus an index page for each.
//
// # What a page is made of
//
// Two halves with two owners. Everything a page says about an action is read
// from the catalog the server assembles for the default dynamic surface
// (dynamiccatalog.Build), built six times: for a self-managed instance and for
// GitLab.com, at Free, Premium and Ultimate. That is what makes the tier of an
// action, and of each of its parameters, a measurement rather than a claim:
// an action's tier is the lowest build that serves it, and it is GitLab.com
// only when no self-managed build does. The meta-tool and individual tool
// names are held to the meta and individual surfaces as mcpsurface lists them,
// so a page cannot name a tool no surface registers. A description is the one
// the dynamic surface serves, its "See also" clause rewritten into canonical
// IDs the way gitlab://tools rewrites it there, and is quoted rather than
// translated.
//
// What the catalog cannot say (what a group is for, and what a reader would
// ask of it) is hand-written in domains.json, in both languages, and embedded.
// The data file has to name exactly the groups the catalog builds: a group the
// catalog gains without an entry stops generation, and so does an entry for a
// group that no longer exists.
//
// # The directories are the generator's
//
// Every page under the two directories is written here, and a page there that
// this run did not write is removed (or, under -check, reported), so a group
// the catalog drops does not leave its page behind.
//
// # Tables that stay the same shape in both languages
//
// The site decides per locale whether a table stacks on a narrow screen, from
// the lengths of its cells (site/src/lib/wide-tables.mjs), and its facts check
// fails when the two locales disagree. Every table written here is therefore
// built so that its translated cells never decide a column's width: a
// translated header has the length of its English twin, and a translated body
// cell is never longer than the header above it. The tests hold that by
// comparing the column measures of every table in both languages, without
// restating the site's thresholds.
//
// Usage:
//
//	go run ./cmd/gen_tool_reference/           # write the pages
//	go run ./cmd/gen_tool_reference/ -check    # fail when a page is stale
package main
