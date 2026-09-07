// Command audit_meta_descriptions checks the prose a meta-tool serves against
// the parameters its actions actually accept.
//
// A meta group's description enumerates its parameters by hand, and nothing
// connected the prose to the schemas. The text is read out of
// internal/tools/testdata/tools_meta.json, which is the golden snapshot the
// regenerator writes from the running server, so the description's only source
// is the file that records the description and TestToolSnapshots compares two
// copies of one string. Repairing an action can therefore leave the served
// prose offering a parameter GitLab refuses, with every gate green. This audit
// is the third party: it reads the served description on one side and the
// routes' input schemas on the other.
//
// # What it fails on
//
//   - a parameter name the description enumerates that no route of that group
//     accepts;
//   - an enum value the description spells for a parameter whose routes publish
//     an enum that does not carry it;
//   - a value the description spells for a parameter that publishes no enum but
//     whose own schema description spells a value set of its own;
//   - a "Parameter guidance:" line written for a parameter the action it names
//     does not accept.
//
// The third rule exists because a schema enum is a closed set of strings, so a
// numeric value set cannot be one. gitlab_member_role's base_access_level
// offered 5 in the prose while the schema description said 10 to 50, and
// doc/api/member_roles.md agrees with the schema: GitLab refuses 5. Both sides
// of that comparison are read by one function, so the description's own
// "10=Guest, 20=Reporter" list is the oracle and a set nothing spells is not
// judged at all.
//
// # The extraction rule
//
// Descriptions follow a house pattern: after the usage preamble and the action
// guidance (both removed by [toolutil.StripMetaToolDescriptionPrefix]) comes a
// block of one line per action, "- <action>: <parameters>". A line is read as a
// parameter enumeration only when every comma-separated item on it parses as a
// parameter token, so ordinary prose and the "Returns:" block are skipped whole
// rather than mined for words. A required parameter carries a trailing
// asterisk, an item may name alternatives with " or ", and an item may end in a
// parenthesised annotation whose slash-separated parts are the parameter's enum
// values when every part is a bare identifier. Commas and " or " inside an
// annotation belong to the annotation, so "(numeric ID or full path)" names one
// parameter and not two.
//
// Shapes it parses, each quoted so the leading bullet is read as part of the
// line rather than as a list marker:
//
//	"- feature_delete: name*"
//	"- list: search, scope (ALL/NAMESPACES), first (max 100), after (cursor)"
//	"- token_project_get / token_group_get: project_id* or group_id*, token_id*"
//	"- namespace_get: id* (numeric ID or full path)"
//	"- list_all: (admin) type, status, paused, tag_list"
//	"- license_get: (no params)"
//	"- create_group: group_id*, name*. Same permission booleans as create_instance."
//
// A shape it deliberately does not parse:
//
//	"- hook_edit: group_id*, hook_id*, same params as hook_add"
//
// The trailing item is prose that no sentence break separates from the
// parameters, so the whole line is skipped. Reading it would mean guessing
// which words are parameters, and a guess in an audit is a false failure that
// teaches people to ignore it.
//
// # Coverage
//
// The cost of skipping is coverage, so the report names it. Every line read is
// counted, and so is every line the rule refused, which -uncovered then prints
// one by one. A refusal counts only when the line's head names an action of the
// group: "- Destructive: …" and "- HTTPS: …" open the same way and name no
// action, and the bullets under a "Returns:" heading say what an action answers
// with rather than what it accepts, so that block is bounded and skipped whole.
// What is left is the honest work list, and the served descriptions carry none:
// the forty lines that ended in prose were rewritten to end at a sentence
// break instead, which is what turned "- hook_edit: group_id*, hook_id*, same
// params as hook_add" into "- hook_edit: group_id*, hook_id*. Same params as
// hook_add." One of them was hiding a defect: bulk_import_start offered a flat
// url and access_token, and the action takes a nested configuration object.
//
// # Which schema a line is judged against
//
// A line whose head names exactly one action the catalog knows is judged
// against that action's own schema; a line shared by several actions, or one
// whose head is a wildcard, is judged against the group's pooled union, which
// is the only set that can hold a line whose items belong to different
// actions. The union alone used to judge every line, and it is what let
// gitlab_project offer pages_update a pages_access_level that belongs to
// project.update, and what hid bulk_import_start's flat url behind another
// action's url of the same name.
//
// The "Parameter guidance:" block is generated per action from a hand-written
// map keyed by parameter name, and is read by its own rule, since its shape is
// exact: "- <action>.<parameter>: <role>. Source: …". Nothing filters that map
// against the schema either, so a key renamed out from under it serves advice
// about a parameter that does not exist. Because the line names one action, it
// is judged against that action's schema rather than against the group's pool.
//
// # Surface
//
// The descriptions come from a real tools/list round-trip on the meta surface
// at the widest tier, through cmd/internal/mcpsurface, so what is judged is
// what crosses the wire. The accepted parameter names come from the same
// compiled catalog: for a meta group, each route's input schema on its own and
// the union over all of them, chosen per line by the rule above; for a
// standalone meta tool such as gitlab_discover_project, which is no group, the
// tool's own input schema.
//
// With -check it exits non-zero on any finding, which is the CI gate
// (make check-meta-descriptions); without it, it prints the findings and exits
// zero.
package main
