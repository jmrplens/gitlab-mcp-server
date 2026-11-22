// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

// The NuGet checks.
const (
	nugetMinBinary      = 20 * 1024 * 1024
	nugetHandshakeLimit = 120 * time.Second
	nugetSignatureEntry = ".signature.p7s"
)

// nugetTokenPattern is the MCP Registry ownership token on a boundary the
// registry accepts: followed by whitespace, a tag, a comment close or the end
// of the text, so a token glued to a longer name is refused.
var nugetTokenPattern = regexp.MustCompile(`(^|\s)` + regexp.QuoteMeta(mcpNameToken) + `(\s|<|-->|$)`)

// nugetDotnetEnv turns off the SDK's first-use banner and telemetry notice,
// which it prints on stdout, the stream the handshake reads JSON-RPC from.
var nugetDotnetEnv = []string{"DOTNET_NOLOGO=1", "DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1"}

// runValidateNuget is the validate-nuget subcommand.
func runValidateNuget(root string, args []string, stdout, stderr io.Writer) int {
	flags := newValidatorFlags("validate-nuget", stderr)
	packages := flags.set.String("packages", nugetOutDir, "the directory the nuget subcommand wrote")
	if !flags.parse(args, stderr) {
		return 2
	}
	v := newValidation("validate-nuget", root, flags.version, nugetMinBinary, stdout, stderr)
	dir := localPath(root, *packages)
	v.validateNuget(dir)
	if !flags.noInstall && len(v.problems) == 0 {
		if _, err := exec.LookPath(v.dotnet); err != nil {
			v.failf("dotnet is not on PATH; install the .NET 10 SDK or pass --no-install")
		} else {
			v.nugetHandshake(dir)
		}
	}
	return v.finish("all packages")
}

// nugetExpected maps each package file name of the version to its platform,
// the pointer to the zero platform.
func nugetExpected(version string) map[string]releasePlatform {
	expected := map[string]releasePlatform{nugetPackageName(nugetID, version): {}}
	for _, p := range releasePlatforms {
		expected[nugetPackageName(nugetID+"."+p.rid, version)] = p
	}
	return expected
}

// validateNuget runs every offline check over the packages in dir: exactly
// the seven packages of the version and nothing else, each archive's layout
// as nuget.org's signature leaves intact, and each package's contents.
func (v *validation) validateNuget(dir string) {
	record := v.readVerified(dir, true)
	expected := nugetExpected(v.version)
	entries, err := os.ReadDir(dir)
	if err != nil {
		v.failf("%s cannot be read: %v", dir, err)
		return
	}
	var present, leftovers []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".nupkg") {
			present = append(present, entry.Name())
		} else {
			leftovers = append(leftovers, entry.Name())
		}
	}
	if wanted := mapKeys(expected); !slices.Equal(present, wanted) {
		v.failf("the directory holds %q but the release needs exactly %q", present, wanted)
	}
	v.check(len(leftovers) == 0, "the directory holds files the publish step would try to push: %q", leftovers)
	license := v.repositoryLicense()
	for _, name := range present {
		p, ok := expected[name]
		if !ok {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, name)) //#nosec G304 -- a package the nuget subcommand wrote
		if readErr != nil {
			v.failf("%s: %v", name, readErr)
			continue
		}
		archive, zipErr := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if zipErr != nil {
			v.failf("%s: not a zip archive: %v", name, zipErr)
			continue
		}
		for _, problem := range signableLayoutProblems(data) {
			v.failf("%s: %s", name, problem)
		}
		if p.rid == "" {
			v.validatePointer(name, archive, license, record)
		} else {
			v.validateRIDPackage(name, archive, p, license, record)
		}
	}
}

// signableLayoutProblems are the ways an archive departs from one nuget.org's
// repository signature can be added to and removed from again byte for byte:
// an archive comment, anything in zip64 form, an entry written with a data
// descriptor (general purpose flag bit 3), and a .signature.p7s already in
// it. The release attests the packages it pushes, and a verifier finds that
// attestation by removing the signature from what nuget.org serves and
// hashing the rest, so a layout the signing would rewrite leaves an
// attestation nobody can look up, silently. The records are read here rather
// than through archive/zip, which resolves zip64 fields and data descriptors
// on the way in and so cannot say whether an archive used them.
func signableLayoutProblems(data []byte) []string {
	eocd := bytes.LastIndex(data, []byte("PK\x05\x06"))
	if eocd < 0 || eocd+22 > len(data) {
		return []string{"no end-of-central-directory record"}
	}
	var problems []string
	le := binary.LittleEndian
	nDisk, nTotal := le.Uint16(data[eocd+8:]), le.Uint16(data[eocd+10:])
	cdSize, cdOffset := le.Uint32(data[eocd+12:]), le.Uint32(data[eocd+16:])
	if trailing := len(data) - (eocd + 22); trailing > 0 {
		problems = append(problems, fmt.Sprintf("%d byte(s) of archive comment follow the end-of-central-directory record; the signed layout nuget.org produces carries none", trailing))
	}
	locator := eocd - 20
	if locator >= 0 && bytes.Equal(data[locator:locator+4], []byte("PK\x06\x07")) || nDisk == 0xffff || nTotal == 0xffff || cdSize == 0xffffffff || cdOffset == 0xffffffff {
		return append(problems, "the archive ends in zip64 form")
	}
	if int(cdOffset)+int(cdSize) > eocd {
		return append(problems, "the central directory runs past the end-of-central-directory record")
	}
	offset := int(cdOffset)
	for range nTotal {
		found, next, ok := centralRecordProblems(data, offset)
		problems = append(problems, found...)
		if !ok {
			return problems
		}
		offset = next
	}
	return problems
}

// centralRecordProblems reads the central directory record at offset and
// the local header it names, and returns what it found, where the next
// record starts, and whether there was a record at offset at all.
func centralRecordProblems(data []byte, offset int) (problems []string, next int, ok bool) {
	le := binary.LittleEndian
	if offset+46 > len(data) || !bytes.Equal(data[offset:offset+4], []byte("PK\x01\x02")) {
		return []string{fmt.Sprintf("no central directory record at offset %d", offset)}, offset, false
	}
	flags := le.Uint16(data[offset+8:])
	compressed, uncompressed := le.Uint32(data[offset+20:]), le.Uint32(data[offset+24:])
	nameLen, extraLen, commentLen := int(le.Uint16(data[offset+28:])), int(le.Uint16(data[offset+30:])), int(le.Uint16(data[offset+32:]))
	local := le.Uint32(data[offset+42:])
	end := min(len(data), offset+46+nameLen+extraLen)
	entry := string(data[offset+46 : min(end, offset+46+nameLen)])
	extra := data[min(end, offset+46+nameLen):end]
	next = offset + 46 + nameLen + extraLen + commentLen
	if strings.EqualFold(entry, nugetSignatureEntry) {
		problems = append(problems, "already carries "+entry+", the entry nuget.org's signature adds and a verifier strips before it looks the package up")
	}
	descriptor := flags&0x8 != 0
	zip64 := compressed == 0xffffffff || uncompressed == 0xffffffff || local == 0xffffffff || slices.Contains(extraFieldIDs(extra), 0x0001)
	// A record in zip64 form keeps its real local offset in the extra field,
	// so the local header is read only when the record says where it is.
	if !zip64 {
		at := int(local)
		if at+30 > len(data) || !bytes.Equal(data[at:at+4], []byte("PK\x03\x04")) {
			return append(problems, fmt.Sprintf("%s names a local header at offset %d that is not one", entry, at)), next, true
		}
		descriptor = descriptor || le.Uint16(data[at+6:])&0x8 != 0
		localName, localExtra := int(le.Uint16(data[at+26:])), int(le.Uint16(data[at+28:]))
		start := min(len(data), at+30+localName)
		zip64 = slices.Contains(extraFieldIDs(data[start:min(len(data), start+localExtra)]), 0x0001)
	}
	if descriptor {
		problems = append(problems, entry+" is written with a data descriptor (general purpose flag bit 3)")
	}
	if zip64 {
		problems = append(problems, entry+" is stored in zip64 form")
	}
	return problems, next, true
}

// extraFieldIDs are the header ids of a zip extra field, in order, stopping
// at a tail too short to hold another header.
func extraFieldIDs(extra []byte) []uint16 {
	var ids []uint16
	for offset := 0; offset+4 <= len(extra); {
		ids = append(ids, binary.LittleEndian.Uint16(extra[offset:]))
		offset += 4 + int(binary.LittleEndian.Uint16(extra[offset+2:]))
	}
	return ids
}

// nuspecMetadata is the part of a nuspec the checks read.
type nuspecMetadata struct {
	XMLName  xml.Name `xml:"package"`
	Metadata *struct {
		ID           *string `xml:"id"`
		Version      *string `xml:"version"`
		Authors      *string `xml:"authors"`
		Description  *string `xml:"description"`
		License      *string `xml:"license"`
		LicenseURL   *string `xml:"licenseUrl"`
		Readme       *string `xml:"readme"`
		Icon         *string `xml:"icon"`
		PackageTypes []struct {
			Name string `xml:"name,attr"`
		} `xml:"packageTypes>packageType"`
	} `xml:"metadata"`
}

// toolSettings is a DotnetToolSettings.xml.
type toolSettings struct {
	XMLName  xml.Name `xml:"DotNetCliTool"`
	Version  string   `xml:"Version,attr"`
	Commands []struct {
		Name       string  `xml:"Name,attr"`
		EntryPoint *string `xml:"EntryPoint,attr"`
		Runner     string  `xml:"Runner,attr"`
	} `xml:"Commands>Command"`
	Packages []struct {
		RID string `xml:"RuntimeIdentifier,attr"`
		ID  string `xml:"Id,attr"`
	} `xml:"RuntimeIdentifierPackages>RuntimeIdentifierPackage"`
}

// nugetPackage is one open package under validation.
type nugetPackage struct {
	v     *validation
	name  string
	files map[string]*zip.File
	names []string
}

// read reads one entry, or reports it missing.
func (n *nugetPackage) read(entry string) ([]byte, bool) {
	f, ok := n.files[entry]
	if !ok {
		return nil, false
	}
	data, err := readZipEntry(f)
	if err != nil {
		n.v.failf("%s: %s cannot be read: %v", n.name, entry, err)
		return nil, false
	}
	return data, true
}

// nuspec checks the OPC skeleton and returns the parsed nuspec of id, or nil.
func (n *nugetPackage) nuspec(id string) *nuspecMetadata {
	v := n.v
	if _, ok := n.files["[Content_Types].xml"]; !ok {
		v.failf("%s: no [Content_Types].xml (not an OPC container)", n.name)
	}
	if rels, ok := n.read("_rels/.rels"); !ok {
		v.failf("%s: no _rels/.rels", n.name)
	} else {
		v.check(bytes.Contains(rels, []byte("/"+id+".nuspec")), "%s: _rels/.rels does not name /%s.nuspec as the manifest", n.name, id)
	}
	data, ok := n.read(id + ".nuspec")
	if !ok {
		v.failf("%s: %s.nuspec missing", n.name, id)
		return nil
	}
	var spec nuspecMetadata
	if err := xml.Unmarshal(data, &spec); err != nil {
		v.failf("%s: %s.nuspec does not parse: %v", n.name, id, err)
		return nil
	}
	if spec.XMLName.Space != nugetNuspecNS || spec.Metadata == nil {
		v.failf("%s: %s.nuspec has no <metadata> in the nuspec namespace", n.name, id)
		return nil
	}
	return &spec
}

// elementText is an optional element's text, or "<absent>".
func elementText(s *string) string {
	if s == nil {
		return "<absent>"
	}
	return *s
}

// checkMetadata holds a nuspec to the id, version and package types its role
// needs, the elements nuget.org requires, and the license URL nuget.org
// requires beside an MIT expression (the push of 2.7.5 was refused with a
// 400 for its absence, which nothing before the push reported).
func (n *nugetPackage) checkMetadata(spec *nuspecMetadata, id string, packageTypes []string) {
	v, m := n.v, spec.Metadata
	v.check(elementText(m.ID) == id, "%s: nuspec id is %q, want %q", n.name, elementText(m.ID), id)
	v.check(elementText(m.Version) == v.version, "%s: nuspec version is %q, want %q", n.name, elementText(m.Version), v.version)
	var types []string
	for _, t := range m.PackageTypes {
		types = append(types, t.Name)
	}
	slices.Sort(types)
	v.check(slices.Equal(types, packageTypes), "%s: nuspec packageTypes are %q, want %q", n.name, types, packageTypes)
	for element, value := range map[string]*string{"authors": m.Authors, "description": m.Description, "license": m.License} {
		v.check(value != nil, "%s: nuspec lacks <%s>", n.name, element)
	}
	v.check(elementText(m.LicenseURL) == nugetLicenseURL, "%s: nuspec licenseUrl is %q, want %q beside the MIT expression", n.name, elementText(m.LicenseURL), nugetLicenseURL)
}

// declaredFile reports whether the file a nuspec <readme> or <icon> names is
// in the package, and names it.
func (n *nugetPackage) declaredFile(element string, declared *string) (string, bool) {
	if declared == nil {
		return "", false
	}
	if _, ok := n.files[*declared]; !ok {
		n.v.failf("%s: nuspec <%s> names %s but the package has no such file", n.name, element, *declared)
		return "", false
	}
	return *declared, true
}

// settings parses a DotnetToolSettings.xml, which must be a version 2
// manifest, the one the runtime-identifier layout needs.
func (n *nugetPackage) settings(entry string) *toolSettings {
	data, ok := n.read(entry)
	if !ok {
		n.v.failf("%s: %s missing", n.name, entry)
		return nil
	}
	var settings toolSettings
	if err := xml.Unmarshal(data, &settings); err != nil {
		n.v.failf("%s: %s does not parse: %v", n.name, entry, err)
		return nil
	}
	if settings.Version != "2" {
		n.v.failf("%s: %s is not a DotNetCliTool manifest of Version 2 (the runtime-identifier layout needs it)", n.name, entry)
		return nil
	}
	return &settings
}

// checkLicenses holds the license texts at the package root: each a regular
// file, the repository's LICENSE byte for byte and the release's notices.
func (n *nugetPackage) checkLicenses(license []byte, record *verified) {
	v := n.v
	for _, entry := range []string{licensePath, noticesName} {
		f, ok := n.files[entry]
		if !ok {
			v.failf("%s: no %s at the package root", n.name, entry)
			continue
		}
		mode := f.ExternalAttrs >> 16
		v.check(mode&0o170000 == 0o100000, "%s: %s is not a regular file (mode %o)", n.name, entry, mode)
		data, _ := n.read(entry)
		if entry == noticesName {
			digest := ""
			if record != nil {
				digest = record.Notices
			}
			v.checkNotices(n.name, data, digest)
			continue
		}
		v.check(bytes.Equal(data, license), "%s: %s is not the repository's LICENSE", n.name, entry)
	}
}

// openNuget indexes an archive under validation.
func (v *validation) openNuget(name string, archive *zip.Reader) *nugetPackage {
	files, names := zipIndex(archive)
	return &nugetPackage{v: v, name: name, files: files, names: names}
}

// validatePointer checks the pointer package: its nuspec, its licenses, a
// README carrying the ownership token the MCP Registry validates nuget
// ownership through, its settings naming the six runtime packages, a
// .mcp/server.json naming this package at this version, and no binary.
func (v *validation) validatePointer(name string, archive *zip.Reader, license []byte, record *verified) {
	n := v.openNuget(name, archive)
	spec := n.nuspec(nugetID)
	if spec == nil {
		return
	}
	n.checkMetadata(spec, nugetID, []string{"DotnetTool", "McpServer"})
	n.checkLicenses(license, record)
	if readme, ok := n.declaredFile("readme", spec.Metadata.Readme); ok {
		data, _ := n.read(readme)
		v.check(nugetTokenPattern.Match(data), "%s: %s lost the MCP Registry ownership token (%q on its own boundary)", name, readme, mcpNameToken)
	} else if spec.Metadata.Readme == nil {
		v.failf("%s: nuspec declares no <readme>; the MCP Registry validates NuGet ownership through the published README", name)
	}
	n.declaredFile("icon", spec.Metadata.Icon)
	if settings := n.settings("tools/" + nugetToolFramework + "/any/DotnetToolSettings.xml"); settings != nil {
		c := settings.Commands
		v.check(len(c) == 1 && c[0].Name == binaryName && c[0].EntryPoint == nil,
			"%s: the pointer must declare exactly one command named %s with no entry point (the runtime packages carry it)", name, binaryName)
		var declared, want []string
		for _, p := range settings.Packages {
			declared = append(declared, p.RID+"="+p.ID)
		}
		for _, p := range releasePlatforms {
			want = append(want, p.rid+"="+nugetID+"."+p.rid)
		}
		slices.Sort(declared)
		slices.Sort(want)
		v.check(slices.Equal(declared, want), "%s: RuntimeIdentifierPackages are %q, want %q", name, declared, want)
	}
	if data, ok := n.read(".mcp/server.json"); !ok {
		v.failf("%s: no .mcp/server.json (nuget.org renders the install snippet from it)", name)
	} else {
		var doc struct {
			Name     string `json:"name"`
			Version  string `json:"version"`
			Packages []struct {
				RegistryType string `json:"registryType"`
				Identifier   string `json:"identifier"`
				Version      string `json:"version"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			v.failf("%s: .mcp/server.json is not JSON: %v", name, err)
		}
		v.check(doc.Name == serverName, "%s: .mcp/server.json names %q, want %q", name, doc.Name, serverName)
		v.check(doc.Version == v.version, "%s: .mcp/server.json carries version %q, want %q", name, doc.Version, v.version)
		var nuget []string
		for _, p := range doc.Packages {
			if p.RegistryType == "nuget" {
				nuget = append(nuget, p.Identifier+"@"+p.Version)
			}
		}
		v.check(slices.Equal(nuget, []string{nugetID + "@" + v.version}), "%s: .mcp/server.json must declare one nuget package, %s at %s", name, nugetID, v.version)
	}
	var binaries []string
	for _, entry := range n.names {
		if base := path.Base(entry); base == binaryName || base == binaryName+".exe" {
			binaries = append(binaries, entry)
		}
	}
	v.check(len(binaries) == 0, "%s: the pointer carries a binary (%s); only runtime packages do", name, strings.Join(binaries, ", "))
}

// validateRIDPackage checks one runtime package: its nuspec, licenses and
// settings, and exactly one binary at the path its settings name.
func (v *validation) validateRIDPackage(name string, archive *zip.Reader, p releasePlatform, license []byte, record *verified) {
	n := v.openNuget(name, archive)
	id := nugetID + "." + p.rid
	spec := n.nuspec(id)
	if spec == nil {
		return
	}
	n.checkMetadata(spec, id, []string{"DotnetToolRidPackage"})
	n.declaredFile("readme", spec.Metadata.Readme)
	n.checkLicenses(license, record)
	prefix := "tools/any/" + p.rid + "/"
	if settings := n.settings(prefix + "DotnetToolSettings.xml"); settings != nil {
		c := settings.Commands
		v.check(len(c) == 1 && c[0].Name == binaryName && elementText(c[0].EntryPoint) == p.binary() && c[0].Runner == "executable",
			"%s: the command must be %s with EntryPoint=%s and Runner=executable", name, binaryName, p.binary())
	}
	var payload []string
	for _, entry := range n.names {
		if strings.HasPrefix(entry, prefix) && !strings.HasSuffix(entry, "DotnetToolSettings.xml") {
			payload = append(payload, entry)
		}
	}
	entry := prefix + p.binary()
	if !slices.Equal(payload, []string{entry}) {
		v.failf("%s: expected exactly %s under %s, found %q", name, entry, prefix, payload)
		return
	}
	data, _ := n.read(entry)
	if mode := n.files[entry].ExternalAttrs >> 16; !p.exe && !executableMode(mode) {
		v.failf("%s: the binary must be a regular file with the execute bit, got mode %o", name, mode)
	}
	v.checkBinary(name, p, data, record, p.key)
}

// nugetHandshake installs the tool from the packages with dotnet tool
// install into a throwaway directory and runs it again through dnx, and
// both must answer an MCP handshake. --source replaces every configured
// feed, so the package installed is the one in dir and nothing nuget.org may
// hold under the same id and version.
func (v *validation) nugetHandshake(dir string) {
	source, err := filepath.Abs(dir)
	if err != nil {
		v.failf("handshake: %v", err)
		return
	}
	tmp, err := os.MkdirTemp("", "nuget-validate-")
	if err != nil {
		v.failf("handshake: %v", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	tools := filepath.Join(tmp, "tools")
	out, err := v.run(tmp, nugetDotnetEnv, v.dotnet, "tool", "install", nugetID, "--version", v.version,
		"--tool-path", tools, "--source", source, "--ignore-failed-sources")
	if err != nil {
		v.failf("dotnet tool install failed: %v\n%s", err, out)
		return
	}
	shim := filepath.Join(tools, binaryName)
	if runtime.GOOS == "windows" {
		shim += ".exe"
	}
	if _, err = os.Stat(shim); err != nil {
		v.failf("dotnet tool install left no %s in %s", filepath.Base(shim), tools)
		return
	}
	v.handshake("dotnet tool install", tmp, nugetDotnetEnv, nugetHandshakeLimit, shim)
	// dotnet dnx rather than the dnx script: the script sits in the SDK
	// root, which is on PATH on a developer machine but not necessarily after
	// a CI SDK install, and the two are the same command.
	v.handshake("dnx", tmp, nugetDotnetEnv, nugetHandshakeLimit,
		v.dotnet, "dnx", nugetID+"@"+v.version, "--source", source, "--ignore-failed-sources")
}
