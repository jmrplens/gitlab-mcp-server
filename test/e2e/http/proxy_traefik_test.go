// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// proxy_traefik_test.go serves the remote deployment guide's Traefik
// configuration and holds it to the guide's requirement table
// (guide_test.go).
//
// The guide configures Traefik in two blocks: Docker-provider labels on the
// server's container, and a static entry point. The static block is served
// as the page prints it. The labels are translated, label by label, into the
// file provider's dynamic configuration, which is what the Docker provider
// builds from them, so the case needs no Docker socket and no labeled
// container (decided on issue 1112, 31 October 2026); a label the
// translation does not know fails the case rather than being dropped.
package httpe2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// traefikImage is the Traefik case's image, pinned to a minor version.
const traefikImage = "traefik:v3.7"

// traefikProvider is what the case adds to the guide's static block: the file
// provider the translated labels are read from.
const traefikProvider = `providers:
  file:
    filename: /etc/traefik/dynamic.yml
`

// composePort is the port the guide's Compose service listens on, which its
// load balancer label names; the case's server listens elsewhere.
const composePort = "8080"

// errUnknownLabel is a label the translation has no rule for.
var errUnknownLabel = errors.New("the label has no file-provider translation")

// traefikLabel matches one label of the guide's list.
var traefikLabel = regexp.MustCompile(`^- "([^"=]+)=(.*)"$`)

// The labels the translation knows, as the Docker provider reads them.
var (
	routerLabel  = regexp.MustCompile(`^traefik\.http\.routers\.([^.]+)\.(rule|entrypoints|tls\.certresolver)$`)
	serviceLabel = regexp.MustCompile(`^traefik\.http\.services\.([^.]+)\.loadbalancer\.server\.port$`)
)

// traefikRouter is one router of the file provider's dynamic configuration.
type traefikRouter struct {
	Rule        string            `json:"rule"`
	EntryPoints []string          `json:"entryPoints,omitempty"`
	Service     string            `json:"service"`
	TLS         map[string]string `json:"tls,omitempty"`
}

// translateTraefikLabels turns the guide's Docker-provider labels into the
// routers the file provider serves and the name of the one service they are
// attached to; traefikDynamic sends that service to the server.
//
// The Docker provider attaches a router to the container's one service and
// reaches it at the container's address on the port the service label names;
// the file provider is told both, so the port the label names is the one
// substitution, and it must be the Compose service's.
func translateTraefikLabels(block string) (map[string]*traefikRouter, string, error) {
	routers := map[string]*traefikRouter{}
	var services []string
	enabled := false
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "labels:" {
			continue
		}
		match := traefikLabel.FindStringSubmatch(line)
		if match == nil {
			return nil, "", fmt.Errorf("%q is not a label: %w", line, errUnknownLabel)
		}
		key, value := strings.ToLower(match[1]), match[2]
		switch {
		case key == "traefik.enable":
			enabled = value == "true"
		case routerLabel.MatchString(key):
			parts := routerLabel.FindStringSubmatch(key)
			router := routers[parts[1]]
			if router == nil {
				router = &traefikRouter{}
				routers[parts[1]] = router
			}
			switch parts[2] {
			case "rule":
				router.Rule = value
			case "entrypoints":
				router.EntryPoints = strings.Split(value, ",")
			default:
				router.TLS = map[string]string{"certResolver": value}
			}
		case serviceLabel.MatchString(key):
			if value != composePort {
				return nil, "", fmt.Errorf("the service label names port %s and the case serves the Compose service's %s: %w", value, composePort, errEditCount)
			}
			services = append(services, serviceLabel.FindStringSubmatch(key)[1])
		default:
			return nil, "", fmt.Errorf("%s: %w", match[1], errUnknownLabel)
		}
	}
	if !enabled {
		return nil, "", errors.New("the labels do not enable the container, so the Docker provider would route nothing to it")
	}
	if len(services) != 1 || len(routers) == 0 {
		return nil, "", fmt.Errorf("the labels define %d service(s) and %d router(s); the translation attaches routers to one service", len(services), len(routers))
	}
	for _, router := range routers {
		router.Service = services[0]
	}
	return routers, services[0], nil
}

// traefikDynamic is the file provider's configuration: the translated routers,
// the service sent to the server, and the certificate Traefik serves.
//
// The guide's router asks for the certificate resolver "le", which the static
// block does not define. Traefik logs that the resolver does not exist and
// serves the default certificate instead, so the case makes its certificate
// the default one, which is where the Let's Encrypt one would have been.
func traefikDynamic(routers map[string]*traefikRouter, service string, upstream int) string {
	doc := map[string]any{
		"http": map[string]any{
			"routers": routers,
			"services": map[string]any{
				service: map[string]any{
					"loadBalancer": map[string]any{
						"servers": []map[string]string{{"url": fmt.Sprintf("http://127.0.0.1:%d", upstream)}},
					},
				},
			},
		},
		"tls": map[string]any{
			"stores": map[string]any{
				"default": map[string]any{
					"defaultCertificate": map[string]string{"certFile": "/e2e/pki/fullchain.pem", "keyFile": "/e2e/pki/privkey.pem"},
				},
			},
		},
	}
	// YAML is a superset of JSON, which the file provider reads as YAML.
	out, err := json.Marshal(doc)
	if err != nil {
		panic(err) // a map of strings always marshals
	}
	return string(out)
}

// guideTraefik is the guide's Traefik configuration, served with the entry
// point and upstream a case chooses. reroute, when set, changes the
// translated routers, for the prefix route the guide describes in prose.
func guideTraefik(cert *publicCert, reroute func(map[string]*traefikRouter, string)) documentedProxy {
	return documentedProxy{
		name:  "traefik",
		roots: cert.pool,
		// Traefik is the edge here, so it sets X-Forwarded-For itself.
		clientHeader: "X-Forwarded-For",
		start: func(t *testing.T, upstream int) (string, int) {
			t.Helper()
			port := freePort(t)
			static := guideBlock(t, remoteDeploymentPage, "traefik-static",
				edit{old: `address: ":443"`, new: fmt.Sprintf(`address: ":%d"`, port), count: 1})
			routers, service, err := translateTraefikLabels(guideBlock(t, remoteDeploymentPage, "traefik-labels"))
			if err != nil {
				t.Fatalf("translating the guide's Traefik labels: %v", err)
			}
			if reroute != nil {
				reroute(routers, service)
			}

			dir := containerDir(t)
			name := fmt.Sprintf("gitlab-mcp-httpe2e-guide-traefik-%d", port)
			startContainer(t, name, traefikImage, []string{
				writeContainerFile(t, dir, "traefik.yml", static+traefikProvider) + ":/etc/traefik/traefik.yml:ro",
				writeContainerFile(t, dir, "dynamic.yml", traefikDynamic(routers, service, upstream)) + ":/etc/traefik/dynamic.yml:ro",
				cert.dir + ":/e2e/pki:ro",
			})
			return name, port
		},
	}
}

// The parts of the guide's PathPrefix paragraph the prefix case serves.
var (
	prefixRule   = regexp.MustCompile("PathPrefix\\(`([^`]+)`\\)")
	exactRule    = regexp.MustCompile("\\bPath\\(`([^`]+)`\\)")
	prefixedURL  = regexp.MustCompile("--public-url=(https://[^`\\s]+)")
	prefixMarker = "is where the trap opens"
)

// TestProxyGuide_Traefik serves the guide's Traefik configuration, holds it
// to the requirement table with the X-Forwarded-For pairing the guide names,
// and serves the prefix route its prose warns about, with and without the
// second router it prescribes.
func TestProxyGuide_Traefik(t *testing.T) {
	t.Parallel()
	cert := newPublicCert(t)

	assertProxyRequirements(t, guideTraefik(cert, nil))

	// The paragraph says a server routed by a path prefix and started with
	// a --public-url under it publishes its metadata outside the prefix,
	// and that a second router for exactly that path fixes it. Both halves
	// are served from what the paragraph says.
	paragraph := guideProse(t, remoteDeploymentPage, prefixMarker)
	prefix, exact, public := prefixRule.FindStringSubmatch(paragraph), exactRule.FindStringSubmatch(paragraph), prefixedURL.FindStringSubmatch(paragraph)
	if prefix == nil || exact == nil || public == nil {
		t.Fatalf("the guide's PathPrefix paragraph no longer names the prefix, the exact path and the --public-url it is about: %s", paragraph)
	}
	metadata := exact[1]
	for _, tc := range []struct {
		name         string
		secondRouter bool
		want         int
	}{
		{name: "a prefix route alone does not reach the metadata", secondRouter: false, want: http.StatusNotFound},
		{name: "the second router the guide prescribes reaches it", secondRouter: true, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
			prefixed := guideTraefik(cert, func(routers map[string]*traefikRouter, service string) {
				var template traefikRouter
				for _, router := range routers {
					router.Rule += " && " + prefix[0]
					template = *router
				}
				if tc.secondRouter {
					routers["metadata"] = &traefikRouter{Rule: exact[0], EntryPoints: template.EntryPoints, Service: service, TLS: template.TLS}
				}
			})
			prefixed.ready = prefix[1] + "/health"
			through := prefixed.behind(t,
				"--auth-mode=oauth",
				"--gitlab-url="+gitlab.url,
				"--public-url="+public[1],
			).from(t, clientForHeaders)

			if got := through.do(t, request{method: http.MethodGet, path: prefix[1] + "/health"}); got.status != http.StatusOK {
				t.Errorf("GET %s/health through the prefix route = %d, want 200: %s", prefix[1], got.status, truncate(got.body))
			}
			got := through.do(t, request{method: http.MethodGet, path: metadata})
			if got.status != tc.want {
				t.Fatalf("GET %s = %d, want %d: %s", metadata, got.status, tc.want, truncate(got.body))
			}
			if tc.secondRouter && !strings.Contains(got.body, `"resource":"`+public[1]+`"`) {
				t.Errorf("the metadata the second router reaches names another resource than %s: %s", public[1], truncate(got.body))
			}
		})
	}
}

// TestTraefikLabels_LabelOutsideTheTranslation_IsRefused pins that the
// translation fails on a label it has no rule for, rather than serving the
// guide's labels without it.
func TestTraefikLabels_LabelOutsideTheTranslation_IsRefused(t *testing.T) {
	t.Parallel()
	const known = "    labels:\n" +
		"      - \"traefik.enable=true\"\n" +
		"      - \"traefik.http.routers.mcp.rule=Host(`mcp.example.com`)\"\n" +
		"      - \"traefik.http.services.mcp.loadbalancer.server.port=8080\"\n"
	tests := []struct {
		name  string
		block string
		want  error
	}{
		{name: "a middleware", block: known + "      - \"traefik.http.routers.mcp.middlewares=buffer\"\n", want: errUnknownLabel},
		{name: "a line that is not a label", block: known + "      networks: [web]\n", want: errUnknownLabel},
		{name: "a service on another port", block: strings.Replace(known, "port=8080", "port=9090", 1), want: errEditCount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := translateTraefikLabels(tt.block); !errors.Is(err, tt.want) {
				t.Errorf("translateTraefikLabels = %v, want an error wrapping %q", err, tt.want)
			}
		})
	}

	t.Run("the guide's labels translate", func(t *testing.T) {
		t.Parallel()
		routers, service, err := translateTraefikLabels(known)
		if err != nil || service != "mcp" || routers["mcp"] == nil || routers["mcp"].Service != "mcp" {
			t.Errorf("translateTraefikLabels = %v, %q, %v; want router mcp sent to service mcp", routers, service, err)
		}
	})
}
