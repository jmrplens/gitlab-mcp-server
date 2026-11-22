// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

// The NuGet distribution: a .NET tool whose entry point is a native
// executable, in the layout the .NET 10 SDK uses for a tool that ships one
// binary per platform (tool manifest version 2). The pointer package names a
// package per runtime identifier, and each of those carries the binary for
// one platform; dotnet tool install and dnx read the pointer, pick the
// package for the host and run the binary directly, so nothing in the
// packages is .NET code. The bare id is free on nuget.org and matches the
// command, the Homebrew formula and the image.
const (
	nugetID     = "gitlab-mcp-server"
	nugetReadme = "nuget/README.md"
	nugetOutDir = "nuget/dist"
	// nugetToolFramework is where the pointer's settings live,
	// tools/<tfm>/any/: the SDK reads the tool's target framework from that
	// path even when nothing in the package targets one.
	nugetToolFramework = "net10.0"
	nugetNuspecNS      = "http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd"
	nugetLicenseURL    = "https://licenses.nuget.org/MIT"
)

// nugetContentTypes is the OPC content-types part. Every extension a package
// carries needs a Default entry, the empty extension of the Linux and macOS
// binaries included; NuGet's client does not read it, but the container has
// to be well formed.
const nugetContentTypes = `<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml" />
  <Default Extension="nuspec" ContentType="application/octet" />
  <Default Extension="xml" ContentType="application/octet" />
  <Default Extension="json" ContentType="application/octet" />
  <Default Extension="md" ContentType="application/octet" />
  <Default Extension="png" ContentType="application/octet" />
  <Default Extension="exe" ContentType="application/octet" />
  <Default Extension="" ContentType="application/octet" />
</Types>
`

// nugetRels is the relationships part naming the package's nuspec.
func nugetRels(id string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Type="http://schemas.microsoft.com/packaging/2010/07/manifest" Target="/` + id + `.nuspec" Id="R1" />
</Relationships>
`
}

const nugetPointerDescription = "GitLab MCP server: GitLab REST v4 and GraphQL as Model Context Protocol tools. " +
	"A .NET tool wrapping the native Go binary; install with dotnet tool install or run with dnx."

func nugetRIDDescription(rid string) string {
	return fmt.Sprintf("The %s binary of gitlab-mcp-server. This package is selected by the "+
		"gitlab-mcp-server tool package for %s hosts; install gitlab-mcp-server instead.", rid, rid)
}

func nugetRIDReadme(rid string) string {
	return fmt.Sprintf("# gitlab-mcp-server (%[1]s)\n\n"+
		"The `%[1]s` executable of [gitlab-mcp-server](https://www.nuget.org/packages/gitlab-mcp-server), a Model Context Protocol server for GitLab. "+
		"The `gitlab-mcp-server` tool package selects this package on `%[1]s` hosts; install or run that one:\n\n"+
		"```bash\ndotnet tool install -g gitlab-mcp-server\ndnx gitlab-mcp-server\n```\n\n"+
		"Documentation: https://docs.jmrp.io/gitlab-mcp-server/\n", rid)
}

// xmlEscape escapes the three characters XML text cannot hold as they are,
// as Python's xml.sax.saxutils.escape does.
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// nugetNuspec renders a nuspec with the metadata nuget.org displays and
// validates. nuget.org refuses a package whose license is an expression
// unless the deprecated <licenseUrl> also points at that expression's page.
func nugetNuspec(id, version, description string, packageTypes []string, readme, icon bool) []byte {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	fmt.Fprintf(&out, "<package xmlns=%q>\n  <metadata>\n", nugetNuspecNS)
	fmt.Fprintf(&out, "    <id>%s</id>\n    <version>%s</version>\n    <authors>jmrplens</authors>\n", id, version)
	fmt.Fprintf(&out, "    <description>%s</description>\n", xmlEscape(description))
	out.WriteString(`    <license type="expression">MIT</license>` + "\n")
	out.WriteString("    <licenseUrl>" + nugetLicenseURL + "</licenseUrl>\n")
	out.WriteString("    <projectUrl>https://docs.jmrp.io/gitlab-mcp-server/</projectUrl>\n")
	out.WriteString(`    <repository type="git" url="https://github.com/jmrplens/gitlab-mcp-server" />` + "\n")
	if readme {
		out.WriteString("    <readme>README.md</readme>\n")
	}
	if icon {
		out.WriteString("    <icon>icon.png</icon>\n")
	}
	out.WriteString("    <tags>mcp mcp-server model-context-protocol gitlab ai dotnet-tool</tags>\n    <packageTypes>\n")
	for _, t := range packageTypes {
		fmt.Fprintf(&out, "      <packageType name=%q />\n", t)
	}
	out.WriteString("    </packageTypes>\n  </metadata>\n</package>\n")
	return []byte(out.String())
}

// nugetPointerSettings is the pointer's DotnetToolSettings.xml: the command,
// and the package that carries it for each runtime identifier.
func nugetPointerSettings() []byte {
	var out strings.Builder
	out.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<DotNetCliTool Version=\"2\">\n  <Commands>\n")
	out.WriteString("    <Command Name=\"" + binaryName + "\" />\n  </Commands>\n  <RuntimeIdentifierPackages>\n")
	for _, p := range releasePlatforms {
		fmt.Fprintf(&out, "    <RuntimeIdentifierPackage RuntimeIdentifier=%q Id=%q />\n", p.rid, nugetID+"."+p.rid)
	}
	out.WriteString("  </RuntimeIdentifierPackages>\n</DotNetCliTool>\n")
	return []byte(out.String())
}

// nugetRIDSettings is a runtime package's DotnetToolSettings.xml: the
// command runs the native executable beside it, with no .NET host between.
func nugetRIDSettings(entry string) []byte {
	return []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<DotNetCliTool Version=\"2\">\n  <Commands>\n" +
		"    <Command Name=\"" + binaryName + "\" EntryPoint=\"" + entry + "\" Runner=\"executable\" />\n" +
		"  </Commands>\n</DotNetCliTool>\n")
}

// nugetServerJSON derives the .mcp/server.json nuget.org reads from the
// repository's MCP Registry manifest, stamped with the version being packed:
// the repository file is stamped by the release only after the packages are
// published, so the copy that ships carries this build's version itself.
// Only the nuget package entry is kept, the one nuget.org renders an install
// snippet from. It is printed as Python's json.dumps printed it, every
// character past ASCII escaped.
func nugetServerJSON(serverJSON *jsonValue, version string) ([]byte, error) {
	var entries []*jsonValue
	for _, pkg := range serverJSON.field("packages").items {
		if pkg.field("registryType").interpolate() == "nuget" && pkg.field("identifier").interpolate() == nugetID {
			entries = append(entries, pkg)
		}
	}
	if len(entries) != 1 {
		return nil, fmt.Errorf("server.json must declare exactly one nuget package with identifier %s", nugetID)
	}
	entry := entries[0].clone()
	entry.set("version", jsonText(version))
	doc := &jsonValue{kind: jsonObject}
	schema := serverJSON.get("$schema")
	if schema == nil {
		schema = &jsonValue{}
	}
	doc.set("$schema", schema.clone())
	for _, key := range []string{"name", "title", "description"} {
		if v := serverJSON.get(key); v != nil {
			doc.set(key, v.clone())
		}
	}
	doc.set("version", jsonText(version))
	for _, key := range []string{"repository", "websiteUrl"} {
		if v := serverJSON.get(key); v != nil {
			doc.set(key, v.clone())
		}
	}
	doc.set("packages", jsonList(entry))
	var out bytes.Buffer
	writePythonJSON(&out, doc, "\n")
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// writePythonJSON prints v as Python's json.dumps(indent=2) does: jq's
// layout, and every character outside printable ASCII escaped as \uXXXX (a
// surrogate pair beyond the first plane), DEL included.
func writePythonJSON(out *bytes.Buffer, v *jsonValue, newline string) {
	inner := newline + "  "
	switch v.kind {
	case jsonString:
		writePythonString(out, v.text)
	case jsonArray:
		if len(v.items) == 0 {
			out.WriteString("[]")
			return
		}
		out.WriteByte('[')
		for index, item := range v.items {
			if index > 0 {
				out.WriteByte(',')
			}
			out.WriteString(inner)
			writePythonJSON(out, item, inner)
		}
		out.WriteString(newline + "]")
	case jsonObject:
		if len(v.members) == 0 {
			out.WriteString("{}")
			return
		}
		out.WriteByte('{')
		for index, member := range v.members {
			if index > 0 {
				out.WriteByte(',')
			}
			out.WriteString(inner)
			writePythonString(out, member.key)
			out.WriteString(": ")
			writePythonJSON(out, member.value, inner)
		}
		out.WriteString(newline + "}")
	default:
		v.write(out, newline)
	}
}

// writePythonString writes s quoted with ensure_ascii's escapes.
func writePythonString(out *bytes.Buffer, s string) {
	out.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			out.WriteString(`\"`)
		case r == '\\':
			out.WriteString(`\\`)
		case r == '\n':
			out.WriteString(`\n`)
		case r == '\r':
			out.WriteString(`\r`)
		case r == '\t':
			out.WriteString(`\t`)
		case r == '\b':
			out.WriteString(`\b`)
		case r == '\f':
			out.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(out, `\u%04x`, r)
		case r < 0x7f:
			out.WriteRune(r)
		case r > 0xffff:
			high, low := utf16.EncodeRune(r)
			fmt.Fprintf(out, `\u%04x\u%04x`, high, low)
		default:
			fmt.Fprintf(out, `\u%04x`, r)
		}
	}
	out.WriteByte('"')
}

// nugetPackageName is the file name of a package.
func nugetPackageName(id, version string) string { return id + "." + version + ".nupkg" }

// nugetRIDEntries are the entries of one runtime package. Every package
// carries the repository's LICENSE at its root beside the nuspec's MIT
// expression, a plain file rather than <license type="file">, since a nuspec
// declares one form or the other, and the notices beside it.
func nugetRIDEntries(version string, p releasePlatform, binary, license, notices []byte) []packageEntry {
	id := nugetID + "." + p.rid
	return []packageEntry{
		{name: "[Content_Types].xml", data: []byte(nugetContentTypes)},
		{name: "_rels/.rels", data: []byte(nugetRels(id))},
		{name: id + ".nuspec", data: nugetNuspec(id, version, nugetRIDDescription(p.rid), []string{"DotnetToolRidPackage"}, true, false)},
		{name: "tools/any/" + p.rid + "/DotnetToolSettings.xml", data: nugetRIDSettings(p.binary())},
		{name: "tools/any/" + p.rid + "/" + p.binary(), data: binary, executable: true},
		{name: "README.md", data: []byte(nugetRIDReadme(p.rid))},
		{name: licensePath, data: license},
		{name: noticesName, data: notices},
	}
}

// nugetPointerEntries are the entries of the pointer package.
func nugetPointerEntries(version string, readme, icon, serverDoc, license, notices []byte) []packageEntry {
	entries := []packageEntry{
		{name: "[Content_Types].xml", data: []byte(nugetContentTypes)},
		{name: "_rels/.rels", data: []byte(nugetRels(nugetID))},
		{name: nugetID + ".nuspec", data: nugetNuspec(nugetID, version, nugetPointerDescription, []string{"DotnetTool", "McpServer"}, true, icon != nil)},
		{name: "tools/" + nugetToolFramework + "/any/DotnetToolSettings.xml", data: nugetPointerSettings()},
		{name: ".mcp/server.json", data: serverDoc},
		{name: "README.md", data: readme},
	}
	if icon != nil {
		entries = append(entries, packageEntry{name: "icon.png", data: icon})
	}
	return append(entries, packageEntry{name: licensePath, data: license}, packageEntry{name: noticesName, data: notices})
}

// runNuget is the nuget subcommand: the six runtime packages and the
// pointer, written to --out after it is emptied.
func runNuget(root string, args []string, stdout, stderr io.Writer) int {
	flags := newBuilderFlags("nuget", nugetOutDir, stderr)
	if !flags.parse(args, stderr, true) {
		return 2
	}
	b := &packageBuild{root: root, binaries: flags.binaries, allowUnverified: flags.allowUnverified, stdout: stdout, stderr: stderr}
	if err := b.buildNuget(flags.version, flags.out); err != nil {
		if !errors.Is(err, errRefused) {
			_ = b.refuse("%v", err)
		}
		return 1
	}
	return 0
}

// buildNuget writes the seven packages and the record of what was verified.
func (b *packageBuild) buildNuget(version, out string) error {
	if !releaseVersion.MatchString(version) {
		return b.refuse("version '%s' does not look like a release version", version)
	}
	license, err := b.readLicense()
	if err != nil {
		return err
	}
	readme, err := b.readTokenReadme(nugetReadme)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(localPath(b.root, "server.json")) //#nosec G304 -- the repository's MCP Registry manifest
	if err != nil {
		return b.refuse("server.json could not be read: %v", err)
	}
	serverJSON, err := parseJSON(data)
	if err != nil {
		return b.refuse("server.json could not be read as JSON: %v", err)
	}
	serverDoc, err := nugetServerJSON(serverJSON, version)
	if err != nil {
		return b.refuse("%v", err)
	}
	icon, err := os.ReadFile(localPath(b.root, iconPath)) //#nosec G304 -- the repository's icon
	if err != nil {
		icon = nil
	}
	in, err := b.readRelease("packages are being built")
	if err != nil {
		return err
	}
	outDir := localPath(b.root, out)
	if writeErr := freshDir(outDir); writeErr != nil {
		return writeErr
	}
	var built []string
	var digests []jsonMember
	for _, p := range releasePlatforms {
		name := nugetPackageName(nugetID+"."+p.rid, version)
		if writeErr := writePackageZip(filepath.Join(outDir, name), nugetRIDEntries(version, p, in.binaries[p.key], license, in.notices)); writeErr != nil {
			return writeErr
		}
		built = append(built, name)
		if in.verified {
			digests = append(digests, jsonMember{key: p.key, value: jsonText(in.binaryDigests[p.key])})
		}
	}
	name := nugetPackageName(nugetID, version)
	if writeErr := writePackageZip(filepath.Join(outDir, name), nugetPointerEntries(version, readme, icon, serverDoc, license, in.notices)); writeErr != nil {
		return writeErr
	}
	built = append(built, name)
	if writeErr := writeFileMode(filepath.Join(outDir, verifiedManifest), verifiedRecord(version, in.verified, digests, in.noticesDigest), 0o644); writeErr != nil {
		return writeErr
	}
	for _, name := range built {
		fmt.Fprintf(b.stdout, "built %s\n", path.Join(filepath.ToSlash(out), name))
	}
	fmt.Fprintf(b.stdout, "%s nuget: %s packages for version %s\n", toolName, strconv.Itoa(len(built)), version)
	return nil
}
