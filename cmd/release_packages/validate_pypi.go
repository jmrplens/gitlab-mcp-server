// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The PyPI checks. A Linux binary may demand no glibc symbol newer than
// 2.17, which is what the manylinux_2_17 tag promises.
const (
	pypiMinBinary      = 20 * 1024 * 1024
	pypiGlibcMajor     = 2
	pypiGlibcMinor     = 17
	pypiHandshakeLimit = 60 * time.Second
)

// pypiTokenPattern is the MCP Registry ownership token on its own boundary,
// which is how the registry's matcher reads the description.
var pypiTokenPattern = regexp.MustCompile(`(^|\s)` + regexp.QuoteMeta(mcpNameToken) + `(\s|$)`)

// pypiRequiredURLs are the Project-URL fields the wheel must carry under the
// labels the well-known project URLs specification names.
var pypiRequiredURLs = [][2]string{
	{"Issues", "https://github.com/jmrplens/gitlab-mcp-server/issues"},
	{"Security", "https://github.com/jmrplens/gitlab-mcp-server/security/policy"},
}

// runValidatePypi is the validate-pypi subcommand.
func runValidatePypi(root string, args []string, stdout, stderr io.Writer) int {
	flags := newValidatorFlags("validate-pypi", stderr)
	wheels := flags.set.String("wheels", pypiOutDir, "the wheelhouse the pypi subcommand wrote")
	if !flags.parse(args, stderr) {
		return 2
	}
	v := newValidation("validate-pypi", root, flags.version, pypiMinBinary, stdout, stderr)
	v.validatePypi(localPath(root, *wheels), !flags.noInstall)
	return v.finish("all wheels")
}

// validatePypi checks the wheelhouse in dir: exactly the six wheels of the
// version and nothing else, each wheel's contents, twine's reading of them
// when twine is installed, and with install the wheel for this host
// installed into a throwaway virtual environment, whose two console scripts
// must each answer an MCP handshake.
func (v *validation) validatePypi(dir string, install bool) {
	record := v.readVerified(dir, true)
	expected := map[string]releasePlatform{}
	for _, p := range releasePlatforms {
		expected[pypiWheelName(v.version, p.wheelTag)] = p
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		v.failf("the wheelhouse %s cannot be read: %v", dir, err)
		return
	}
	var present, leftovers []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".whl") {
			present = append(present, entry.Name())
		} else {
			leftovers = append(leftovers, entry.Name())
		}
	}
	wanted := mapKeys(expected)
	if !slices.Equal(present, wanted) {
		v.failf("the wheelhouse holds %q but the release needs exactly %q", present, wanted)
	}
	v.check(len(leftovers) == 0, "the wheelhouse holds files other than wheels, which the publish step would try to upload: %q", leftovers)
	license := v.repositoryLicense()
	var paths []string
	for _, name := range present {
		if p, ok := expected[name]; ok {
			v.validateWheel(filepath.Join(dir, name), p, record, license)
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	if len(paths) > 0 {
		v.twineCheck(paths)
	}
	if !install || len(v.problems) > 0 {
		return
	}
	host, ok := hostPlatform()
	if !ok {
		fmt.Fprintf(v.stdout, "handshake: no wheel for %s/%s, skipping the install\n", runtime.GOOS, runtime.GOARCH)
		return
	}
	v.pypiHandshake(filepath.Join(dir, pypiWheelName(v.version, host.wheelTag)))
}

// mapKeys returns a map's keys, sorted.
func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// hostPlatform is the release platform of the running host, if it is one.
func hostPlatform() (releasePlatform, bool) {
	for _, p := range releasePlatforms {
		if p.goos() == runtime.GOOS && p.goarch() == runtime.GOARCH {
			return p, true
		}
	}
	return releasePlatform{}, false
}

// readZipEntry reads one entry of an open archive whole.
func readZipEntry(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

// zipIndex reads every entry of an archive by name, in order.
func zipIndex(archive *zip.Reader) (files map[string]*zip.File, names []string) {
	files = map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
		names = append(names, file.Name)
	}
	return files, names
}

// metadataFields are the fields of a METADATA header block by their name as
// written, each with every value it was given.
type metadataFields map[string][]string

// Get is a field's first value, or "".
func (m metadataFields) Get(key string) string {
	if values := m[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

// Values is every value of a field.
func (m metadataFields) Values(key string) []string { return m[key] }

// metadataHeaders reads the header block of a METADATA file, the part
// before the description, as packaging tools read it: RFC 822 fields.
func metadataHeaders(metadata []byte) metadataFields {
	head, _, _ := bytes.Cut(metadata, []byte("\n\n"))
	headers := metadataFields{}
	for line := range strings.SplitSeq(string(head), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found {
			headers[key] = append(headers[key], strings.TrimSpace(value))
		}
	}
	return headers
}

// validateWheel checks one wheel: RECORD against every entry, METADATA,
// entry_points.txt and WHEEL against the file name, the version and the
// console script, the license declaration, every entry a regular file, and
// the binary.
func (v *validation) validateWheel(file string, p releasePlatform, record *verified, license []byte) {
	name := filepath.Base(file)
	archive, err := zip.OpenReader(file)
	if err != nil {
		v.failf("%s: not a zip archive: %v", name, err)
		return
	}
	defer func() { _ = archive.Close() }()
	files, names := zipIndex(&archive.Reader)
	read := func(entry string) []byte {
		f, ok := files[entry]
		if !ok {
			v.failf("%s: %s missing", name, entry)
			return nil
		}
		data, readErr := readZipEntry(f)
		if readErr != nil {
			v.failf("%s: %s cannot be read: %v", name, entry, readErr)
		}
		return data
	}
	distInfo := fmt.Sprintf("%s-%s.dist-info", pypiNorm, v.version)
	v.checkRecord(name, distInfo, files, names, read)

	metadata := read(distInfo + "/METADATA")
	for _, needle := range []string{"Name: " + pypiDist, "Version: " + v.version, "Description-Content-Type: text/markdown"} {
		v.check(bytes.Contains(metadata, []byte(needle)), "%s: METADATA missing %q", name, needle)
	}
	v.check(pypiTokenPattern.Match(metadata), "%s: METADATA description lost the MCP Registry ownership token", name)
	v.checkWheelLicensing(name, distInfo, files, names, metadata, license, record)
	headers := metadataHeaders(metadata)
	urls := map[string]string{}
	for _, value := range headers.Values("Project-URL") {
		label, url, _ := strings.Cut(value, ",")
		urls[strings.TrimSpace(label)] = strings.TrimSpace(url)
	}
	for _, want := range pypiRequiredURLs {
		v.check(urls[want[0]] == want[1], "%s: Project-URL %q is %q, want %q", name, want[0], urls[want[0]], want[1])
	}
	for _, f := range archive.File {
		mode := f.ExternalAttrs >> 16
		v.check(mode&0o170000 == 0o100000, "%s: %s is not a regular file (mode %o)", name, f.Name, mode)
	}
	v.check(bytes.Contains(read(distInfo+"/entry_points.txt"), []byte(pypiDist+" = "+pypiPackage+":main")),
		"%s: the console script named after the distribution is missing (uvx matches the name through it)", name)
	wheel := read(distInfo + "/WHEEL")
	v.check(bytes.Contains(wheel, []byte("Tag: py3-none-"+p.wheelTag)), "%s: WHEEL tag disagrees with the file name", name)
	v.check(bytes.Contains(wheel, []byte("Root-Is-Purelib: false")), "%s: the wheel must be platlib (Root-Is-Purelib: false)", name)

	entry := fmt.Sprintf("%s-%s.data/scripts/%s", pypiNorm, v.version, p.binary())
	f, ok := files[entry]
	if !ok {
		v.failf("%s: bundled binary %s missing", name, entry)
		return
	}
	binary := read(entry)
	if mode := f.ExternalAttrs >> 16; !p.exe && !executableMode(mode) {
		v.failf("%s: the binary must be a regular file with the execute bit (pip honors the bits only on one), got mode %o", name, mode)
	}
	v.checkBinary(name, p, binary, record, p.key)
	if p.goos() == "linux" {
		if major, minor := newestGlibc(binary); major > pypiGlibcMajor || major == pypiGlibcMajor && minor > pypiGlibcMinor {
			v.failf("%s: the binary demands glibc %d.%d but the tag promises %d.%d", name, major, minor, pypiGlibcMajor, pypiGlibcMinor)
		}
	}
}

// checkRecord holds RECORD to the archive: the same names, and for every
// entry but RECORD itself the digest and size of its bytes.
func (v *validation) checkRecord(name, distInfo string, files map[string]*zip.File, names []string, read func(string) []byte) {
	text := read(distInfo + "/RECORD")
	recorded := map[string][2]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(text)), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			v.failf("%s: RECORD line %q is not path,digest,size", name, line)
			continue
		}
		size, digest := fields[len(fields)-1], fields[len(fields)-2]
		recorded[strings.Join(fields[:len(fields)-2], ",")] = [2]string{digest, size}
	}
	v.check(slices.Equal(mapKeys(recorded), slices.Sorted(slices.Values(names))), "%s: RECORD names differ from the archive's contents", name)
	for _, entry := range names {
		if strings.HasSuffix(entry, "/RECORD") {
			continue
		}
		data, err := readZipEntry(files[entry])
		if err != nil {
			continue
		}
		got := recorded[entry]
		v.check(got[0] == recordHash(data) && got[1] == strconv.Itoa(len(data)), "%s: RECORD mismatch for %s", name, entry)
	}
}

// checkWheelLicensing holds the license declaration to core metadata 2.4
// (PEP 639): Metadata-Version 2.4 and the MIT expression, no legacy License
// field and no License :: classifier beside it, which PyPI refuses or
// deprecates, every License-File under .dist-info/licenses/ and nothing
// there undeclared, the repository's LICENSE byte for byte and the
// release's notices.
func (v *validation) checkWheelLicensing(name, distInfo string, files map[string]*zip.File, names []string, metadata, license []byte, record *verified) {
	headers := metadataHeaders(metadata)
	v.check(headers.Get("Metadata-Version") == "2.4", "%s: Metadata-Version is %q, want \"2.4\" (License-Expression needs it)", name, headers.Get("Metadata-Version"))
	v.check(headers.Get("License-Expression") == "MIT", "%s: License-Expression is %q, want \"MIT\"", name, headers.Get("License-Expression"))
	v.check(len(headers.Values("License")) == 0, "%s: METADATA carries the legacy License field beside License-Expression, which PyPI refuses", name)
	for _, classifier := range headers.Values("Classifier") {
		v.check(!strings.HasPrefix(classifier, "License ::"), "%s: METADATA carries the classifier %q, deprecated beside License-Expression", name, classifier)
	}
	declared := slices.Sorted(slices.Values(headers.Values("License-File")))
	prefix := distInfo + "/licenses/"
	var shipped []string
	for _, entry := range names {
		if rest, found := strings.CutPrefix(entry, prefix); found {
			shipped = append(shipped, rest)
		}
	}
	sort.Strings(shipped)
	want := []string{licensePath, noticesName}
	v.check(slices.Equal(declared, want), "%s: License-File declares %q, want %q", name, declared, want)
	v.check(slices.Equal(shipped, declared), "%s: %s holds %q but METADATA declares %q", name, prefix, shipped, declared)
	if f, ok := files[prefix+licensePath]; ok {
		data, err := readZipEntry(f)
		v.check(err == nil && bytes.Equal(data, license), "%s: %s is not the repository's LICENSE", name, prefix+licensePath)
	}
	if f, ok := files[prefix+noticesName]; ok {
		data, _ := readZipEntry(f)
		digest := ""
		if record != nil {
			digest = record.Notices
		}
		v.checkNotices(name, data, digest)
	}
}

// twineCheck runs twine check --strict over the wheels when the python3 on
// PATH can import twine: the reading PyPI's upload path makes of the
// metadata, so a field it would refuse fails here rather than after a file
// name is burned. Where twine is not installed it says so and moves on;
// scripts/publish-pypi.sh runs the same check with the twine its
// hash-pinned requirements install before it uploads.
func (v *validation) twineCheck(wheels []string) {
	if _, err := v.run("", nil, "python3", "-c", "import twine"); err != nil {
		fmt.Fprintln(v.stdout, "twine: not installed here, skipping twine check (scripts/publish-pypi.sh runs its own)")
		return
	}
	out, err := v.run("", nil, "python3", append([]string{"-m", "twine", "check", "--strict"}, wheels...)...)
	if err != nil {
		v.failf("twine check --strict refused the wheels:\n%s", out)
		return
	}
	fmt.Fprintf(v.stdout, "twine: check --strict passed for %d wheels\n", len(wheels))
}

// pypiHandshake installs the wheel into a throwaway virtual environment and
// drives both console scripts, the binary under its own name and the one
// named after the distribution. pip runs in the temporary directory, so the
// wheel reaches it as an absolute path: the release names the wheelhouse
// relative to the checkout.
func (v *validation) pypiHandshake(wheel string) {
	wheel, err := filepath.Abs(wheel)
	if err != nil {
		v.failf("handshake: %v", err)
		return
	}
	tmp, err := os.MkdirTemp("", "pypi-validate-")
	if err != nil {
		v.failf("handshake: %v", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	venv := filepath.Join(tmp, "venv")
	if out, venvErr := v.run(tmp, nil, "python3", "-m", "venv", venv); venvErr != nil {
		v.failf("handshake: python3 -m venv failed: %v\n%s", venvErr, out)
		return
	}
	bin := filepath.Join(venv, "bin")
	if runtime.GOOS == "windows" {
		bin = filepath.Join(venv, "Scripts")
	}
	if out, pipErr := v.run(tmp, nil, filepath.Join(bin, "pip"), "install", "--quiet", "--no-index", wheel); pipErr != nil {
		v.failf("handshake: pip install failed: %v\n%s", pipErr, out)
		return
	}
	for _, command := range []string{binaryName, pypiDist} {
		v.handshake(command, tmp, nil, pypiHandshakeLimit, filepath.Join(bin, command))
	}
}
