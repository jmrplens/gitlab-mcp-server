package tenancy

import "fmt"

// Decisions returns every row of the register: one per requirement of the
// specification, eighty in all.
//
// The rows are grouped by the question of spec section 4.4 they answer, in the
// order the specification asks the questions (identify, admit, authorize,
// allow, end), with the per-request bounds last; inside a group they follow the
// specification's numbering. The table is built on each call and nothing holds
// it, so a binary that reaches no caller of this function carries none of it.
func Decisions() []Decision {
	families := [][]Decision{
		identifyDecisions(),
		admitDecisions(),
		authorizeDecisions(),
		allowDecisions(),
		endDecisions(),
		requestDecisions(),
	}
	var all []Decision
	for _, family := range families {
		all = append(all, family...)
	}
	return all
}

// Lookup returns the row for a requirement id.
func Lookup(id string) (Decision, bool) {
	for _, d := range Decisions() {
		if d.ID == id {
			return d, true
		}
	}
	return Decision{}, false
}

// requirementIDs are the specification's requirement ids, in its order: the
// list a complete register has exactly one row for.
func requirementIDs() []string {
	families := []struct {
		prefix string
		count  int
	}{
		{"IDN", 13},
		{"ADM", 13},
		{"AUB", 5},
		{"RTC", 6},
		{"HLD", 10},
		{"POL", 9},
		{"AUT", 6},
		{"DST", 3},
		{"END", 5},
		{"RQB", 10},
	}
	var ids []string
	for _, f := range families {
		for n := 1; n <= f.count; n++ {
			ids = append(ids, fmt.Sprintf("%s-%03d", f.prefix, n))
		}
	}
	return ids
}

// The packages the rows' sites live in.
const (
	pkgServer        = "cmd/server"
	pkgPool          = "internal/serverpool"
	pkgSubscriptions = "internal/subscriptions"
	pkgToolutil      = "internal/toolutil"
	pkgOAuth         = "internal/oauth"
	pkgConfig        = "internal/config"
	pkgGitLab        = "internal/gitlab"
	pkgElicitation   = "internal/elicitation"
	pkgCacheHints    = "internal/cachehints"
	pkgClientCompat  = "internal/clientcompat"
	pkgTools         = "internal/tools"
	pkgVisibility    = "internal/tools/toolvisibility"
	pkgDynamic       = "internal/tools/dynamic"
)

// Site builders, so a row reads as the list of what it names.

// alias is a package-level const or var whose initializer is the register
// constant reads.
func alias(pkg, name, reads string) Site {
	return Site{Pkg: pkg, Name: name, Role: Alias, Reads: reads}
}

// pin is a site that keeps its own literal, held equal to reads.
func pin(pkg, name, reads string) Site {
	return Site{Pkg: pkg, Name: name, Role: Pin, Reads: reads}
}

// arg is the index'th argument of count calls to call inside the function
// name, which carries reads.
func arg(pkg, name, call string, index, count int, reads string) Site {
	return Site{Pkg: pkg, Name: name, Role: Arg, Call: call, Arg: index, Count: count, Reads: reads}
}

// enforce is a function or var that applies the decision.
func enforce(pkg, name string) Site {
	return Site{Pkg: pkg, Name: name, Role: Enforce}
}

// refuse is a function, const or var that holds a refusal's text or builds
// its literal.
func refuse(pkg, name string) Site {
	return Site{Pkg: pkg, Name: name, Role: Refuse}
}

// reasonAt is the declaration whose comment states the reason.
func reasonAt(pkg, name string) Site {
	return Site{Pkg: pkg, Name: name, Role: Reason}
}

// derive is a symbol that computes a value of a key.
func derive(pkg, name string) Site {
	return Site{Pkg: pkg, Name: name, Role: Derive}
}

// gateRefusal is a refusal the gate writes before the SDK sees the request.
func gateRefusal(status, code int, prefix string, answer Answer, at Site) Refusal {
	return Refusal{
		Methods: []string{MethodGate}, Channel: Gate, Code: code, Status: status,
		Prefix: prefix, Answer: answer, At: at,
	}
}

// listenEnd is an ending of a subscriptions/listen, carried as a watch-end
// reason on the completion result.
func listenEnd(reason string, answer Answer, at Site) Refusal {
	return Refusal{
		Methods: []string{"subscriptions/listen"}, Era: EraModern, Channel: ListenEnd,
		Prefix: reason, Answer: answer, At: at,
	}
}

// The methods that share the tool-call bucket's JSON-RPC refusal.
func toolBucketRPCMethods() []string {
	return []string{"resources/read", "resources/subscribe", "subscriptions/listen", "prompts/get"}
}

// The methods a subscription is made through, in either era.
func subscribeMethods() []string {
	return []string{"resources/subscribe", "subscriptions/listen"}
}
