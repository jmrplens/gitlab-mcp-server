package graphqlintrospect

import (
	"fmt"
	"net/url"
	"strings"
)

// CredentialFor decides whether token may be sent to endpoint, and says why
// when it may not.
//
// Both commands that introspect take their endpoint from a flag: one pins the
// schema of whatever `-url` names, the other re-probes whatever `-live` names.
// Reading GITLAB_TOKEN beside such a flag and sending it unconditionally makes
// every mistyped host, and every host somebody else chose, a place the
// operator's GitLab credential arrives. The token buys one thing here, the
// version string in the report, because GitLab answers introspection to anyone;
// so the trade is a whole credential against a line of provenance, and it is
// not worth making by default.
//
// The rule is the narrowest one that is still true: a token belongs to the
// instance GITLAB_URL names, and goes nowhere else. Host and port must match
// exactly; the scheme is compared too, so a plaintext copy of an https instance
// is a different place. When nothing is withheld the reason is empty.
func CredentialFor(endpoint, instance, token string) (credential, withheld string) {
	if token == "" {
		return "", ""
	}
	if strings.TrimSpace(instance) == "" {
		return "", "GITLAB_TOKEN is set and GITLAB_URL is not, so nothing says which instance that token belongs to and it was not sent"
	}

	endpointOrigin, err := origin(endpoint)
	if err != nil {
		return "", fmt.Sprintf("the endpoint %q is not a URL, so GITLAB_TOKEN was not sent", endpoint)
	}
	instanceOrigin, err := origin(instance)
	if err != nil {
		return "", fmt.Sprintf("GITLAB_URL is %q, which is not a URL, so GITLAB_TOKEN was not sent", instance)
	}
	if endpointOrigin != instanceOrigin {
		return "", fmt.Sprintf(
			"GITLAB_TOKEN belongs to %s and this run asks %s, so it was not sent: the version will be unknown, which is the right trade for not handing a credential to an instance nobody named",
			instanceOrigin, endpointOrigin,
		)
	}
	return token, ""
}

// origin reduces a URL to the part that decides who receives a request. The
// path is dropped because an endpoint carries /api/graphql and the instance
// URL does not, and the host keeps its port because two ports on one host are
// two servers.
func origin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%q names no scheme and host", raw)
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}
