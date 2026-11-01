// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"regexp"
	"strings"
)

// ociAccept is what a manifest request accepts: an OCI index, a Docker
// manifest list, or a single OCI manifest.
const ociAccept = "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.oci.image.manifest.v1+json"

// ociLabel is the image label the registry validates an OCI package's
// ownership through.
const ociLabel = "io.modelcontextprotocol.server.name"

// ociRegistry is where one registry's API and its pull tokens are asked.
type ociRegistry struct {
	api, tokenPrefix, tokenSuffix string
}

// ociRegistries are the two registries this check inspects, by the host a
// reference names. They differ in two mechanical ways and in nothing else:
// Docker Hub's registry API does not live on the host the reference names
// (https://docker.io/v2/ redirects to the marketing site), and its token
// endpoint is a separate host that wants a service parameter.
var ociRegistries = map[string]ociRegistry{
	"ghcr.io": {api: "https://ghcr.io", tokenPrefix: "https://ghcr.io/token?scope=repository:", tokenSuffix: ":pull"}, //nolint:gosec // G101 reads the token endpoint's URL as a credential; it holds none
}

// dockerHub is Docker Hub's API, which three spellings of its host name.
var dockerHub = ociRegistry{ //nolint:gosec // G101 reads the token endpoint's URL as a credential; it holds none
	api:         "https://registry-1.docker.io",
	tokenPrefix: "https://auth.docker.io/token?service=registry.docker.io&scope=repository:",
	tokenSuffix: ":pull",
}

// registryFor returns the registry a reference's host names.
func registryFor(host string) (ociRegistry, bool) {
	switch host {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return dockerHub, true
	}
	registry, ok := ociRegistries[host]
	return registry, ok
}

// sha256Reference is a digest reference as a client resolves it.
var sha256Reference = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// dockerfileInfers is a Dockerfile CMD that reads the transport off file
// descriptor 0.
var dockerfileInfers = regexp.MustCompile(`(?m)^CMD.*--transport", *"(auto|stdio)`)

// checkOCI checks every oci package. The registry validates ownership
// through an image label, and rejects version, registryBaseUrl and
// fileSha256 on an OCI entry, server-side, at publish time, long after any
// local schema check has passed. Both published registries are inspected,
// because both are declared and a client picks whichever it likes.
func (c *serverJSONCheck) checkOCI() {
	for _, pkg := range c.declared("oci") {
		identifier := text(pkg, "identifier")
		if identifier == "" {
			continue
		}
		c.checked++
		c.sayf("oci: %s", identifier)
		c.checkImage(pkg, identifier)
	}
}

// imageReference is an OCI reference taken apart the way the registry
// accepts it: repo:tag, repo@digest or repo:tag@digest. server.json uses
// the third: the tag stays readable, the digest is what a client resolves.
type imageReference struct {
	host, repo, tag, digest string
}

// parseImageReference splits an identifier at its first slash into the host
// and the reference, the reference at its last @ into the reference and the
// digest, and what is left at its first colon into the repository and at its
// last into the tag.
func parseImageReference(identifier string) imageReference {
	host, ref, found := strings.Cut(identifier, "/")
	if !found {
		ref = identifier
	}
	var parsed imageReference
	parsed.host = host
	if at := strings.LastIndex(ref, "@"); at >= 0 {
		parsed.digest = ref[at+1:]
		ref = ref[:at]
	}
	parsed.repo, _, _ = strings.Cut(ref, ":")
	if colon := strings.LastIndex(ref, ":"); colon >= 0 {
		parsed.tag = ref[colon+1:]
	}
	return parsed
}

// checkImage checks one declared image.
func (c *serverJSONCheck) checkImage(pkg *jsonValue, identifier string) {
	if banned := bannedFields(pkg, "version", "registryBaseUrl", "fileSha256"); len(banned) > 0 {
		c.failf("carries field(s) the registry rejects for OCI packages: %s", strings.Join(banned, ", "))
	}
	ref := parseImageReference(identifier)
	registry, known := registryFor(ref.host)
	if !known {
		c.sayf("  NOTE: %s is neither ghcr.io nor Docker Hub, skipping the image inspection", ref.host)
		return
	}
	if ref.tag == "" && ref.digest == "" {
		c.failf("identifier carries neither a tag nor a digest")
		return
	}
	if ref.digest != "" && !sha256Reference.MatchString(ref.digest) {
		c.failf("digest is not a sha256:<64 hex> reference: %s", ref.digest)
		return
	}

	tokenAnswer, err := c.fetch.get(registry.tokenPrefix+ref.repo+registry.tokenSuffix, false, nil)
	token := ""
	if err == nil {
		var tokenDoc *jsonValue
		if tokenDoc, err = parseJSON(tokenAnswer.body); err == nil {
			token = tokenDoc.field("token").interpolate()
		}
	}
	if err != nil {
		c.failf("no pull token for %s could be read from the registry", ref.repo)
		return
	}
	headers := map[string]string{"Authorization": "Bearer " + token, "Accept": ociAccept}
	manifests := registry.api + "/v2/" + ref.repo + "/manifests/"

	pinned := ref.digest
	if pinned == "" {
		pinned = ref.tag
	}
	indexAnswer, err := c.fetch.get(manifests+pinned, false, headers)
	if err != nil {
		c.failf("image %s not found in the registry", pinned)
		return
	}
	// A published version tag must never move. If it no longer resolves to
	// the pinned digest, either the tag was re-pushed or the manifest is
	// stale, and both mean the reference no longer describes what the
	// release shipped.
	if ref.digest != "" && ref.tag != "" && !c.tagAgrees(manifests, ref, headers) {
		return
	}

	config, ok := c.checkPlatforms(registry, ref, indexAnswer.body, headers)
	if !ok {
		return
	}
	source, ok := c.transportSource(pkg, config)
	if !ok {
		return
	}
	if ref.digest != "" {
		c.sayf("  OK: digest resolves, tag agrees, ownership label matches, %s", source)
	} else {
		c.sayf("  OK: ownership label matches, %s", source)
	}
}

// tagAgrees checks that the tag resolves to the digest the reference pins.
func (c *serverJSONCheck) tagAgrees(manifests string, ref imageReference, headers map[string]string) bool {
	answer, err := c.fetch.fetch(request{method: "HEAD", url: manifests + ref.tag, headers: headers, timeout: metadataTimeout})
	tagDigest := ""
	if err == nil {
		tagDigest, _, _ = strings.Cut(strings.TrimSpace(answer.header.Get("Docker-Content-Digest")), " ")
	}
	switch tagDigest {
	case "":
		c.failf("tag %s does not resolve in the registry", ref.tag)
		return false
	case ref.digest:
		return true
	}
	c.failf("tag %s resolves to %s but the manifest pins %s", ref.tag, tagDigest, ref.digest)
	return false
}

// imagePlatform is one manifest an index lists: its digest and the platform
// it runs on, or the single manifest a reference resolved to.
type imagePlatform struct {
	digest, platform string
}

// checkPlatforms checks every runnable manifest the reference resolves to,
// not just the first: reading only the first meant only linux/amd64 was ever
// inspected, which is how a linux/arm64 image that could not exec at all
// stayed published and digest-pinned across a whole release. Attestation
// manifests (buildkit's provenance and SBOM entries, platform
// unknown/unknown) carry no runtime config and are skipped. It returns the
// config of the last manifest checked.
func (c *serverJSONCheck) checkPlatforms(registry ociRegistry, ref imageReference, indexBody []byte, headers map[string]string) (*jsonValue, bool) {
	index := parsedOrNull(indexBody)
	children, isIndex := runnableManifests(index)
	base := registry.api + "/v2/" + ref.repo
	checked, failed := 0, false
	var config *jsonValue
	for _, child := range children {
		if isIndex && child.digest == "" {
			continue
		}
		manifest := index
		if isIndex {
			answer, err := c.fetch.get(base+"/manifests/"+child.digest, false, headers)
			if err != nil {
				c.failf("%s: the image manifest could not be fetched", child.platform)
				failed = true
				continue
			}
			manifest = parsedOrNull(answer.body)
		}
		childConfig, ok := c.platformConfig(base, child.platform, manifest, headers["Authorization"])
		if !ok {
			failed = true
			continue
		}
		config = childConfig
		checked++
	}
	if failed {
		return nil, false
	}
	if checked == 0 {
		c.failf("the index declares no runnable platform manifest")
		return nil, false
	}
	c.sayf("  checked %d platform manifest(s)", checked)
	return config, true
}

// platformConfig fetches the config blob one platform's manifest declares
// and holds its ownership label to the server's name.
func (c *serverJSONCheck) platformConfig(base, platform string, manifest *jsonValue, authorization string) (*jsonValue, bool) {
	configDigest := textOr(manifest.field("config"), "digest")
	if configDigest == "" || configDigest == "null" {
		c.failf("%s: the image manifest declares no config digest", platform)
		return nil, false
	}
	answer, err := c.fetch.get(base+"/blobs/"+configDigest, true, map[string]string{"Authorization": authorization})
	if err != nil {
		c.failf("%s: image config blob %s could not be fetched", platform, configDigest)
		return nil, false
	}
	config := parsedOrNull(answer.body)
	if label := textOr(orEmpty(config.field("config")).field("Labels"), ociLabel); label != c.name {
		c.failf("%s: ownership label is \"%s\", expected \"%s\"", platform, label, c.name)
		return nil, false
	}
	return config, true
}

// parsedOrNull is an answer read as JSON, or null when it is not JSON, which
// reads as holding nothing: the field the script asked jq for came out empty
// when jq could not parse the answer.
func parsedOrNull(body []byte) *jsonValue {
	if value, err := parseJSON(body); err == nil {
		return value
	}
	return &jsonValue{}
}

// runnableManifests lists the manifests an index names that are not
// attestations, or, for a reference that resolved to a single manifest, that
// manifest's config as the one entry, and reports which it was.
func runnableManifests(index *jsonValue) ([]imagePlatform, bool) {
	var children []imagePlatform
	for _, child := range index.field("manifests").items {
		platform := orEmpty(child.field("platform"))
		system := textOr(platform, "os")
		if system == "unknown" {
			continue
		}
		if system == "" {
			system = "?"
		}
		arch := textOr(platform, "architecture")
		if arch == "" {
			arch = "?"
		}
		children = append(children, imagePlatform{digest: child.field("digest").interpolate(), platform: system + "/" + arch})
	}
	if len(children) > 0 {
		return children, true
	}
	return []imagePlatform{{digest: textOr(index.field("config"), "digest"), platform: "single"}}, false
}

// transportSource says how an image entry reaches stdio, or fails it. An MCP
// client speaks stdio to the container over the pipe `docker run -i` gives
// it, so the entry has to end up on stdio or the client hangs at initialize.
// The image's CMD reads the transport off that pipe, which is why no
// argument is needed; but any argument after the image name replaces CMD
// wholesale, so an entry that declares packageArguments has to name the
// transport itself.
//
// The pinned image is a published one, so it lags the repository by a
// release. An argument-free entry is correct for the image this Dockerfile
// builds and wrong for the one still on the registry, which is a transition
// rather than a defect. It is excused only while this repository's own CMD
// does reach stdio, and only when the Dockerfile could be read: a check that
// cannot reach its evidence has to fail.
func (c *serverJSONCheck) transportSource(pkg, config *jsonValue) (string, bool) {
	cmd := orEmpty(config.field("config")).field("Cmd").joinStrings(" ")
	var values []string
	for _, argument := range pkg.field("packageArguments").items {
		values = append(values, argument.field("value").interpolate())
	}
	args := strings.Join(values, " ")
	dockerfile := c.sibling("Dockerfile")
	switch {
	case args != "":
		if !containsAny(args, "--http=false", "--transport stdio", "--transport auto") {
			c.failf("package arguments \"%s\" replace CMD but name no stdio transport", args)
			return "", false
		}
		return "package arguments name the transport", true
	case containsAny(cmd, "--transport auto", "--transport stdio"):
		return "CMD infers the transport", true
	}
	data, err := os.ReadFile(c.local(dockerfile)) //#nosec G304,G703 -- the Dockerfile beside the server.json the operator named, read and never written
	if err != nil {
		c.failf("image CMD is \"%s\", the package names no transport, and %s could not be read to check the next release's", cmd, dockerfile)
		return "", false
	}
	if !dockerfileInfers.Match(data) {
		c.failf("image CMD is \"%s\", which never reaches stdio, and neither the package nor %s names one", cmd, dockerfile)
		return "", false
	}
	c.sayf("  NOTE: published CMD is \"%s\"; the argument-free entry is correct from the next release, whose image infers the transport", cmd)
	return "the next release's CMD infers the transport", true
}

// containsAny reports whether s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
