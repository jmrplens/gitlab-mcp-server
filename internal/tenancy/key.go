package tenancy

import "fmt"

// Key is what a declared decision is counted against: the unit one caller
// holds one of, or the process that no caller can multiply (spec section 4.1).
//
// A key is chosen from this vocabulary, never invented at the site. A decision
// that needs a key the vocabulary does not have adds it here, with what it
// costs a caller to mint and the evidence for that cost, which is how the
// mintable-key question gets answered once rather than per limit.
type Key uint8

// The keys of spec section 4.1. The carrier and the shape are deliberately
// absent: nothing is counted against the carrier, and the shape keys a catalog
// cache that INV-010 governs rather than an allowance.
const (
	// KeyNone is no key. As a [Decision.StdioKey] it says the decision does
	// not exist on stdio.
	KeyNone Key = iota
	// KeyRequest is one request, argument set or watched URI (class Q).
	KeyRequest
	// KeyCredential is the raw token a request presents (IDN-001).
	KeyCredential
	// KeyEntry is one credential on one instance: the pool entry (IDN-002).
	KeyEntry
	// KeyOwner is the random name of one entry build (IDN-003).
	KeyOwner
	// KeyTenant is the pair (canonical instance URL, GitLab user id)
	// (TEN-001).
	KeyTenant
	// KeyVerified is the (instance, token) pair the OAuth identity cache is
	// keyed on (ADM-002, ADM-003, ADM-005). The rejected-token cache hashes
	// the same pair, and is [KeyRefused]'s (ADM-006).
	KeyVerified
	// KeyApplication is the OAuth application uid a token was issued to
	// (TEN-004).
	KeyApplication
	// KeySession is an SDK session id, recorded to an owner (IDN-010).
	KeySession
	// KeyAddress is the charged client address (IDN-004).
	KeyAddress
	// KeySource is the socket peer, with every header ignored (IDN-005).
	KeySource
	// KeyRefused is a credential GitLab refused, held as a digest: what
	// AUB-003 counts and what ADM-006 remembers.
	KeyRefused
	// KeyUnbound is every request no credential was bound to (IDN-007).
	KeyUnbound
	// KeyProcess is the running binary.
	KeyProcess
	// KeyDeployment is the operator's configuration.
	KeyDeployment
)

// Axis says which side of admission a key lives on (TEN-010, spec 7.3).
type Axis uint8

// The axes a key can lie on.
const (
	// AxisNone is the axis of [KeyNone].
	AxisNone Axis = iota
	// AxisRequest is one request: bounding it bounds what a request may cost,
	// never what a caller may hold.
	AxisRequest
	// AxisPreAdmission is before a tenant exists, where the address, the source
	// or a refused credential's digest is the only key available.
	AxisPreAdmission
	// AxisRequester is a tenant, a credential or something one credential
	// holds.
	AxisRequester
	// AxisProcess is the process or the deployment, which no caller can
	// multiply.
	AxisProcess
)

// MintCost is what producing one more value of a key costs a caller (spec 4.1,
// "Mint cost to the caller").
type MintCost uint8

// The mint costs, cheapest last among the mintable ones.
const (
	// NotMintable is a key no caller can produce another value of.
	NotMintable MintCost = iota
	// MintFree costs nothing: an invented string, a new request, a new session.
	MintFree
	// MintAddress costs another client address.
	MintAddress
	// MintCredential costs another credential of the same GitLab user.
	MintCredential
	// MintPrincipal costs another GitLab user, which on self-managed any user
	// with a personal project can create as a project bot (TEN-008).
	MintPrincipal
)

// Unit is the equivalence class a stated reason is compared in. A reason about
// a credential agrees with a key on the entry, because both are
// [UnitCredential].
type Unit uint8

// The units a reason can be about.
const (
	// UnitNone is the unit of a reason that names none.
	UnitNone Unit = iota
	// UnitRequest is one request.
	UnitRequest
	// UnitCredential is one credential, or anything held per credential.
	UnitCredential
	// UnitPrincipal is one GitLab user on one instance.
	UnitPrincipal
	// UnitSession is one session.
	UnitSession
	// UnitApplication is one OAuth application.
	UnitApplication
	// UnitAddress is one client address.
	UnitAddress
	// UnitProcess is the process.
	UnitProcess
)

// Keys returns every key of the vocabulary, [KeyNone] first.
func Keys() []Key {
	return []Key{
		KeyNone, KeyRequest, KeyCredential, KeyEntry, KeyOwner, KeyTenant,
		KeyVerified, KeyApplication, KeySession, KeyAddress, KeySource,
		KeyRefused, KeyUnbound, KeyProcess, KeyDeployment,
	}
}

// keyInfo is everything the vocabulary says about one key.
type keyInfo struct {
	name       string
	axis       Axis
	mint       MintCost
	unit       Unit
	evidence   string
	derivation []Site
}

// info answers every question about k in one place, so that a key's axis, mint
// cost, unit and evidence cannot be declared in four switches that drift.
//
// The evidence is spec section 4.1's, restated with the published pages it
// rests on; the two sentences that decided issues 540 and 561 are the
// credential's and the tenant's.
func (k Key) info() keyInfo {
	switch k {
	case KeyNone:
		return keyInfo{
			name: "none", axis: AxisNone, mint: NotMintable, unit: UnitNone,
			evidence: "No key: the decision is not made in this mode.",
		}
	case KeyRequest:
		return keyInfo{
			name: "request", axis: AxisRequest, mint: MintFree, unit: UnitRequest,
			evidence: "Every request is a fresh value, so minting one costs nothing; a bound keyed on it " +
				"bounds what one request may cost, never what a caller may hold.",
		}
	case KeyCredential:
		return keyInfo{
			name: "credential", axis: AxisRequester, mint: MintCredential, unit: UnitCredential,
			evidence: "Another credential of the same GitLab user: a personal access token through the UI, " +
				"with no administrator, no count cap and no creation rate limit " +
				"(https://docs.gitlab.com/user/profile/personal_access_tokens/); a fine-grained token " +
				"through the API; an OAuth token by authorizing again. Only an administrator of a Premium " +
				"or Ultimate self-managed instance can switch personal access tokens off.",
			derivation: []Site{
				derive(pkgPool, "ExtractToken"),
				derive(pkgServer, "mcpServerGate.extractCredential"),
			},
		}
	case KeyEntry:
		return keyInfo{
			name: "entry", axis: AxisRequester, mint: MintCredential, unit: UnitCredential,
			evidence: "As a credential: one entry per credential per instance. The instance half is the " +
				"operator's published list, and several published instances do not multiply keys, " +
				"because the credential is verified against the one the request selected (issue 540). " +
				"Under --allow-any-gitlab-url (DST-002, loopback only) nothing is published and the " +
				"caller names the instance, so one credential mints one entry per instance it names " +
				"that the pool builds an entry for.",
			derivation: []Site{
				derive(pkgPool, "sessionKey"),
				derive(pkgPool, "canonicalHost"),
			},
		}
	case KeyOwner:
		return keyInfo{
			name: "owner", axis: AxisRequester, mint: MintCredential, unit: UnitCredential,
			evidence: "As an entry: a random name minted per entry build, so a rebuilt entry is a new owner.",
			derivation: []Site{
				derive(pkgPool, "ServerPool.buildEntry"),
			},
		}
	case KeyTenant:
		return keyInfo{
			name: "tenant", axis: AxisRequester, mint: MintPrincipal, unit: UnitPrincipal,
			evidence: "Another GitLab user. On a self-managed instance any user who may create a personal " +
				"project may create project access tokens on it, and each is a bot user of its own, with no " +
				"administrator, no count cap and no creation rate limit " +
				"(https://docs.gitlab.com/user/project/settings/project_access_tokens/). On GitLab.com it " +
				"takes a paid namespace, or a service account in a top-level group, 100 per group on Free " +
				"(https://docs.gitlab.com/user/profile/service_accounts/). GitLab has no per-person key one " +
				"person cannot multiply, so the tenant is the unit of identity and authority, never of a share.",
			derivation: []Site{
				derive(pkgPool, "resolveIdentity"),
				derive(pkgOAuth, "admitToken"),
				derive(pkgServer, "prepareStdioCatalog"),
			},
		}
	case KeyVerified:
		return keyInfo{
			name: "verified", axis: AxisRequester, mint: MintCredential, unit: UnitCredential,
			evidence: "As a credential: the pair (instance, token), each value of which holds one entry of " +
				"the OAuth identity cache for its lifetime.",
			derivation: []Site{
				derive(pkgOAuth, "tokenKey"),
			},
		}
	case KeyApplication:
		return keyInfo{
			name: "application", axis: AxisRequester, mint: MintFree, unit: UnitApplication,
			evidence: "Nothing: GitLab registers an OAuth application dynamically without authentication, " +
				"ten an hour per address, and a user-owned application is one form away " +
				"(https://docs.gitlab.com/integration/oauth_provider/).",
			derivation: []Site{
				derive(pkgOAuth, "acceptedRecipient"),
			},
		}
	case KeySession:
		return keyInfo{
			name: "session", axis: AxisRequester, mint: MintFree, unit: UnitSession,
			evidence: "Nothing: any initialize opens a session, and nothing bounds how many exist.",
			derivation: []Site{
				derive(pkgServer, "sessionOwners.record"),
			},
		}
	case KeyAddress:
		return keyInfo{
			name: "address", axis: AxisPreAdmission, mint: MintAddress, unit: UnitAddress,
			evidence: "Address rotation: the key is the whole address, with no IPv6 prefix aggregation; " +
				"behind a trusted proxy that copies a caller's header it is that header's value; on a unix " +
				"socket every caller shares one.",
			derivation: []Site{
				derive(pkgServer, "clientIP"),
			},
		}
	case KeySource:
		return keyInfo{
			name: "source", axis: AxisPreAdmission, mint: MintAddress, unit: UnitAddress,
			evidence: "Address rotation only: the socket peer, with every header ignored.",
			derivation: []Site{
				derive(pkgServer, "transportSource"),
			},
		}
	case KeyRefused:
		return keyInfo{
			name: "refused", axis: AxisPreAdmission, mint: MintFree, unit: UnitCredential,
			evidence: "Nothing: a refused credential is an invented string, held only as a digest.",
			derivation: []Site{
				derive(pkgOAuth, "rejectedKey"),
				derive(pkgPool, "DistinctTokenBudget.Charge"),
			},
		}
	case KeyUnbound:
		return keyInfo{
			name: "unbound", axis: AxisProcess, mint: NotMintable, unit: UnitProcess,
			evidence: "Not mintable: every request no credential was bound to shares the one value.",
		}
	case KeyProcess:
		return keyInfo{
			name: "process", axis: AxisProcess, mint: NotMintable, unit: UnitProcess,
			evidence: "Not mintable: the running binary is the only unit no caller can multiply.",
		}
	case KeyDeployment:
		return keyInfo{
			name: "deployment", axis: AxisProcess, mint: NotMintable, unit: UnitProcess,
			evidence: "Not mintable by a caller: the operator's configuration.",
		}
	default:
		return keyInfo{name: fmt.Sprintf("Key(%d)", uint8(k))}
	}
}

// String names the key as spec section 4.1 does.
func (k Key) String() string { return k.info().name }

// Axis reports which side of admission the key lives on.
func (k Key) Axis() Axis { return k.info().axis }

// MintCost reports what producing another value of the key costs a caller.
func (k Key) MintCost() MintCost { return k.info().mint }

// Mintable reports whether a caller can produce another value of the key at
// all. It is false only for the keys no caller controls, which is TEN-008 as
// data: the tenant is mintable too.
func (k Key) Mintable() bool { return k.MintCost() != NotMintable }

// Unit reports the equivalence class a reason about this key is compared in.
func (k Key) Unit() Unit { return k.info().unit }

// Evidence is spec 4.1's sentence for the key, with the published pages it
// rests on. [Validate] quotes it when it refuses a share on the key.
func (k Key) Evidence() string { return k.info().evidence }

// Derivation names the symbols that compute a value of the key today. They are
// identity mechanics rather than policy (a hash, a host canonicalization, an
// owner minted from the operating system's entropy), so they are declared here
// and not moved.
func (k Key) Derivation() []Site { return k.info().derivation }
