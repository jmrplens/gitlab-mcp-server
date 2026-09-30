# ADR-0024: A fine-grained token's authority is evaluated per action from what GitLab declares

## Status

Accepted, 2026-09-30. It records the decisions
[issue 952](https://github.com/jmrplens/gitlab-mcp-server/issues/952) took on 2026-09-28
and the four answers its maintainer gave on 2026-09-30. The mechanisms it names land in
stages under that issue; the first, reading the token kind as unknown authority, lands
with this record.

## Context

GitLab's fine-grained personal access token does not carry its authority in scopes. Its
legacy scope list is the single value `granular`
(`app/services/authn/personal_access_tokens/create_granular_service.rb` at `v19.4.1-ee`),
and what it may do is a grant fixed at creation: named permissions (857 assignable ones at
19.4.1, 71 of them deprecated), each held at a boundary (a project, a group, the user, the
instance). GitLab judges every request against that grant after the token authenticates.
A REST route outside it answers 403 with the RFC 6750 code `insufficient_granular_scope`
and a sentence naming the missing permissions; a GraphQL mutation outside it answers 200
with the field `null` and the same sentence in `errors[]`; a GraphQL type or mutation that
declares no fine-grained permission at all (862 types and 61 mutations are on GitLab's own
pending list at 19.4.1) cannot be reached by any fine-grained token. The code alone does
not prove the kind: a classic token under a root namespace that enforces fine-grained
tokens is answered with the same code and text.

This server read such a token through its scopes, which say nothing about it:

- **stdio, and every HTTP pool entry the door admitted,** asked
  `GET /personal_access_tokens/self` for the token's scopes. For a token granted
  Personal Access Token: Read, which that route requires of a fine-grained token, the
  answer was `["granular"]`: no `api` scope, so the session was narrowed to the read-only
  surface, withholding every write whatever the grant allowed and telling the model to
  reauthorize with `api`, and the scope filter removed the five `admin_mode` groups for
  want of a scope such a token cannot carry. A token without that permission was answered
  403, detection failed, and it was already read as unknown authority.
- **HTTP in legacy mode** read the 403 of its door probe (`GET /user`, which needs
  User: Read) as a refused credential, charged the address's failure budget and answered
  401 "reauthorize", so a valid token was told it was invalid and ten attempts blocked the
  address.
- **HTTP in OAuth mode** refused every fine-grained token at the door, since
  `oauth.SatisfiesMinimum` does not count `granular` as meeting `read_api`.
- **stdio** read GitLab's refusal of `GET /version`, which a fine-grained token reaches
  only when it grants Metadata: Read, as an unreachable instance: it never resolved the
  identity or detected the tier, and armed a lazy re-initialization that asked the
  version again once per thirty seconds of activity for the life of the process.
- **Every mode** described a call GitLab refused outside the grant with the generic 403
  sentence (a missing scope, a project role, an admin setting), none of which is what is
  missing, and a handler's hint written for that 403 (a role, a license, an owner) was
  appended to it. Only a write's error carried GitLab's own sentence, in client-go's
  flattening, and a GraphQL refusal a handler returned without wrapping it read as an
  unexpected error. Three raw GraphQL domains never read the refusal at all, since a
  refused mutation answers with its payload `null` and they decoded the payload alone:
  the vulnerability state mutations, the custom emoji delete and the epic issue link,
  unlink and reorder reported a change that had not happened, and the custom emoji
  create dropped GitLab's reason.

The tenant register records the misreading as finding F-17, filed under issue 952 and
carried by the rows that decide the read-only surface (`AUT-001`) and the door's
admission (`ADM-001`, `ADM-002`). The first stage answers it for `AUT-001`, and the
second, the door's uncharged answer and the OAuth minimum a fine-grained token meets,
answers it for `ADM-001` and `ADM-002`, so no row carries it any longer.

## Decision

**A fine-grained token is unknown authority, and its authority is evaluated per action
from what GitLab declares.** Three facts with three owners are kept apart and joined by a
generator: what each action requests (this repository's, derived from the handlers), what
GitLab demands of each route and GraphQL element (GitLab's, recorded from a booted GitLab
in `docs/development/gitlab-api-live.json`), and how an action's requests combine (the
handler author's, declared at the call site). The join is a table the binary embeds,
published on each catalog action.

1. **Phase A: admitted everywhere, as unknown authority.** A token whose scope list is
   exactly `["granular"]` is never narrowed to the read-only surface by that list
   (`gitlabclient.FineGrained`, `gitlabclient.WriteCapable`); the operator's read-only still
   applies to it as to any token. The catalog's scope filter and its cache key read its
   list as unknown (`gitlabclient.CatalogScopes`), so it shares the catalog of a classic
   token whose scopes are unknown and never that of a token with no scope. It is admitted
   in every mode; a door probe it lacks the permission for is answered 403, uncharged,
   naming the permission, and a call outside its grant is answered with GitLab's own
   refusal, quoted with its way out. No `ServerConfig` field carries the token kind: the
   authority attached to the credential's client is the per-request signal, so nothing
   that runs at registration can read it and let the first credential of a shape decide
   for the others.
2. **What no fine-grained token can reach is withheld, and says why.** An action whose
   GraphQL type or mutation declares no fine-grained permission at the GitLab release the
   table was recorded from is withheld from such a session with the reason, the GitLab
   version the verdict comes from and the way out (a classic token), and never answered as
   an unknown action. A GraphQL read that comes back `null` with no error, or a list that
   comes back empty, is given a hint for whatever the record misses.
3. **Phase B: a grant the server can read decides the surface.** A token whose grant is
   readable (it holds Personal Access Token: Read) is served only what the grant reaches,
   judged per action against the recorded requirement. A token whose grant cannot be read,
   or an instance outside the recorded versions, stays in phase A.
4. **The grant is never a key.** It is a value the caller mints, so no shared catalog,
   shape or manifest is keyed on it (`INV-010`); the authority is computed per pool entry
   (per process on stdio), lives with the entry the pool already bounds, and is read per
   request. The version bucket is never a key either.
5. **The maintainer's answers of 2026-09-30.** GitLab.com reports the minor after the
   newest release as `-pre`; that version is treated as inside the record for phase B's
   listing only, with the vocabulary guard kept, every call not denied in phase A allowed
   through, and the recorded release named in every withheld message. The listing shows
   only what the grant covers, while a call GitLab would serve on a public project or
   group is let through to GitLab rather than refused here. The per-action requirement
   lives in the generated table joined into the runtime catalog action and published by
   `gitlab://tools/{id}` and a generated reference page, with the author's intent as
   call-site directives, and never as a hand-written field of `ActionSpec`. The door's
   uncharged 403 for a token lacking User: Read is cached in the rejected-token cache of
   `ADM-006` in both modes, since nothing edits a grant after creation at 19.4.1. A
   deployment pinned to its OAuth applications (`--oauth-client-uid`, `ADM-004`) answers
   the same token with its recipient refusal instead, uncharged and cached as well: only
   a personal access token is refused a fine-grained grant, and a pinned deployment
   admits none, so the 403's advice would name two credentials it refuses.

**ADR-0018's asymmetry holds on REST, and is replaced on GraphQL.** On REST a wrong "yes"
surfaces as GitLab's own 403 on the one call that needed the permission, so reading
authority as unknown errs in the direction ADR-0018 chose. On GraphQL a wrong "yes" is
silent: a connection drops the items the grant does not cover, and a declared type the
grant does not reach comes back `null`. So phase B judges the answer's spine as part of
the requirement, and names what the grant leaves empty off it, rather than leaning on
GitLab to say so.

Out of scope, each with its own issue: serving over REST what GraphQL cannot reach for a
fine-grained token ([issue 1054](https://github.com/jmrplens/gitlab-mcp-server/issues/1054)),
and declaring the missing GraphQL permissions upstream
([issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055)). The admission
minimum issue 952 set for legacy HTTP and stdio (`read_api`) is decided there and not
here; when it lands it treats `["granular"]` as meeting the minimum, through
`gitlabclient.FineGrained`.

## Consequences

### Positive

- **POS-001**: A fine-grained token works in every mode. It is no longer served a
  read-only surface it did not ask for, refused at the door, or told it is invalid.
- **POS-002**: A refusal names the permission GitLab wants, in GitLab's own words, and
  says that a grant cannot be changed after creation, so the way out is a new token.
- **POS-003**: The surface a fine-grained session is shown is decided by GitLab's own
  declarations, recorded from a booted instance and joined per action, so a permission
  GitLab renames moves the record and not a hand-kept list.
- **POS-004**: A classic token pays nothing: its client carries no authority, and every
  path it takes is the one it took before.

### Negative

- **NEG-001**: The phase A set is the newest record's. On a newer instance it may withhold
  what that instance now serves; on an older one it may miss what that instance cannot,
  and on GraphQL an action it misses is served silently. Every withheld message names the
  release its verdict comes from.
- **NEG-002**: On GraphQL a wrong "yes" is silent. The spine rule and the null and
  empty-result hints narrow it; they cannot close it.
- **NEG-003**: The door's uncharged 403 is answered from the rejected-token cache for a
  token already refused, and a flood of distinct minted tokens lacking User: Read is
  bounded in concurrency by the probe and verifier slots, not in rate. Each is a genuine
  credential of a real account.
- **NEG-004**: A fine-grained pool entry pays two requests per revalidation interval (the
  grant and the version), and none per call.
- **NEG-005**: The public set the call guard reads is GitLab's anonymous policy evaluated
  on a public project and group with every feature enabled, so it over-approximates an
  instance whose settings serve less; on REST that surfaces as GitLab's own 403.
- **NEG-006**: The manifest a fine-grained session reads is filtered per read, which adds
  a copy of its entries; the marshal per read was already there.

### Neutral

- **NEU-001**: A list that carries `granular` beside another scope is not the
  fine-grained shape and is read as the classic list it spells, which is what GitLab does
  with it.

## Alternatives considered

**Treat a fine-grained token as read-only until its grant is read.** It is what the scope
reading did by accident. Rejected: it withholds every write a grant may allow and blames
a scope the token cannot carry, which is ADR-0018's invisible wrong "no".

**A hand-written permission field on each `ActionSpec`.** Rejected: 1098 literals across
some 180 packages, rewritten on every GitLab permission rename (two already, at 19.0 and
19.4), and exactly the hand-kept list the 1:1 audits exist to replace with an oracle.

**Key the shared catalog on the grant.** Rejected on the register's own terms: the grant
is minted by the caller, so a key on it is a cache a caller can grow at will (`INV-010`).

**A `ServerConfig` field for the token kind.** Rejected: registration reads the
configuration once per shape, so the first credential of a shape would decide for every
later one, and a classic token with unknown scopes and a fine-grained one must share a
shape.

## Compliance

- `TestFineGrained_OnlyTheSingleGranularScopeIsTheShape`,
  `TestWriteCapable_FineGrainedIsUnknownAuthority` and
  `TestCatalogScopes_FineGrainedReadsAsUnknown` in `internal/gitlab` hold the reading of the
  token kind.
- `TestCatalogRelevantScopes_EqualComponentsFilterIdentically`,
  `TestCatalogRelevantScopes_FineGrainedIsKeyedAndFilteredApartFromNoScope` and
  `TestCatalogFilterKey_FineGrainedTokenIsKeyedAsUnknownScopes` in `internal/tools` hold
  the filter and the cache key to one reading of the fine-grained list.
- `test/e2e/stdio/token_scope_test.go` holds the binary to it: on the individual surface a
  fine-grained token is listed exactly the tools a token whose scopes are unknown is
  listed, every write and the `admin_mode` groups among them, with no log line saying it
  cannot write, and on the default surface its write reaches the instance.
- `TestCheckCredential_FourAnswers_KeepsEachApart`,
  `TestCheckCredentialDetail_FineGrainedRefusal_CarriesGitLabsSentence` and the two
  `TestPermissionRefusal` tests in `internal/gitlab`,
  `TestGetOrCreate_FineGrainedTokenWithoutUserRead_IsRefusedAsAccepted` and
  `TestConfirmUnexplainedRefusal_ProbeRefusedAFineGrainedPermission_KeepsTheEntry` in
  `internal/serverpool`, and `TestNewGitLabVerifier_ForbiddenDistinguishesScope`,
  `TestIntrospectToken_FineGrainedRefusalOfSelf_AnswersWithoutTokenInfo` and
  `TestGitLabVerifier_PinnedDeployment_AnswersAMissingUserPermissionAsAnUnacceptedRecipient`
  in `internal/oauth` hold the door's reading of a missing User: Read.
- `TestMCPServerGate_FineGrainedTokenWithoutUserRead_IsForbiddenUncharged`,
  `TestBearerGuard_FineGrainedTokenWithoutUserRead_IsForbiddenUnchargedAndRemembered` and
  the two `PermissionMissing_QuotesAHostileSentenceOnlyFilteredAndCut` tests in
  `cmd/server` hold both doors to an uncharged, remembered 403 that quotes GitLab only
  filtered and cut; `test/e2e/http` holds the binary to it at both doors, and to
  admitting a fine-grained token that can read its own user.
- `TestParseGranularRefusal_GitLabsSentences_AreReadIntoTheirParts`,
  `TestParseGranularRefusal_AnythingElse_IsUnrecognized` and
  `TestParseGraphQLGranularRefusal_NotFound_IsTheServicesFourthAnswer` in
  `internal/gitlab` hold the reading of GitLab's four refusal texts, a permission named
  by its deprecated first match among them, and
  `TestClassifyError_FineGrainedRefusalOverREST_DescribesEachOfGitLabsTexts`,
  `TestClassifyError_FineGrainedRefusalOverGraphQL_IsReadFromEachEntry` and
  `TestSanitizeError_FineGrainedRefusal_IsDescribedOnce` in `internal/toolutil` hold what
  a model is told of each, whichever route the error took, and
  `TestWrapErrWithHint_FineGrainedRefusal_DropsTheHandlersHint` there and
  `TestPackageDelete403_FineGrainedRefusal_NamesThePermissionAndNoRole` in
  `internal/tools/packages` that no handler's hint follows it.
- `TestDismiss_RefusedMutation_IsAnErrorNamingGitLabsReason` in
  `internal/tools/vulnerabilities`, `TestMutations_Refused_IsAnErrorNamingGitLabsReason`
  in `internal/tools/customemoji` and
  `TestEpicIssueMutations_Refused_IsAnErrorNamingGitLabsReason` in
  `internal/tools/epicissues` hold a GraphQL mutation GitLab refused to an error carrying
  its reason rather than a success.
- `TestInitialize_FineGrainedVersionRefusal_IsReachableWithTheVersionUnknown`,
  `TestDetectEnterprise_VersionRefused_UsesTheFallbackWithoutAsking` and
  `TestVersion_OnlyAVersionGitLabCouldSend_IsKept` in `internal/gitlab`,
  `TestPrepareStdioCatalog_VersionRefusedToAFineGrainedToken_StartsWhole` in `cmd/server`
  and `TestTokenScope_FineGrainedTokenWithoutMetadataRead_StartsWhole` in
  `test/e2e/stdio` hold a start without Metadata: Read to a reachable instance: one
  warning, the version asked once over twenty calls, and the tier still detected.
- `make check-tenancy` holds the register rows `AUT-001` and `AUT-002` to the sites that
  read the token kind, and `ADM-001` to `ADM-004`, `ADM-006` and the failure table to
  the door's refusal, its sites and its charges.

## Related

- [ADR-0018](adr-0018-authorization-admits-per-action-gating.md) (admission at the
  minimum scope, authority per action), whose asymmetry this record keeps on REST.
- [ADR-0020](adr-0020-one-server-per-configuration-shape.md) (one server per
  configuration shape), whose shape key the grant never enters.
- [ADR-0023](adr-0023-tenant-policy-is-declared-once.md) (the tenant policy register),
  whose rows `AUT-001`, `AUT-002`, `ADM-001` to `ADM-004` and `ADM-006` this record
  amends, and beside which it adds the rows for what a fine-grained session is
  withheld and the bound on reading its grant.
- Issues [952](https://github.com/jmrplens/gitlab-mcp-server/issues/952),
  [1054](https://github.com/jmrplens/gitlab-mcp-server/issues/1054) and
  [1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055).
