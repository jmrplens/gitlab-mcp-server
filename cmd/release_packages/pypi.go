// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// The PyPI distribution. The unprefixed name gitlab-mcp-server is registered
// empty on PyPI by an unrelated account and is under PEP 541 reclamation, so
// the distribution ships under the author-prefixed name while the import
// package and the native command keep the unprefixed one: client
// configurations never change, uvx matches the distribution name through a
// console script of that name, and the reclaimed name will slot in as a
// rename of pypiDist.
const (
	pypiDist    = "jmrplens-gitlab-mcp-server"
	pypiNorm    = "jmrplens_gitlab_mcp_server"
	pypiPackage = "gitlab_mcp_server"
	pypiReadme  = "pypi/README.md"
	pypiOutDir  = "pypi/dist"
)

// pypiFileHeader is the copyright and license header the two Python modules
// of the wheel open with, and the blank line after it: the wheel publishes
// them as source, and every source file of the project carries the header
// (cmd/internal/fileheader). It is built from that package's two constants,
// as the Homebrew formula's is, so a change of holder or year reaches the
// next wheel, and no line of this file opens with the notice in another
// comment syntax.
const pypiFileHeader = "# " + fileheader.CopyrightStatement + "\n# " + fileheader.LicenseStatement + "\n\n"

// pypiLauncher is the wheel's gitlab_mcp_server/__init__.py, with %s the
// version. The binary itself ships in the wheel's .data/scripts directory,
// which the installer places on the scripts path with the execute bit set,
// so it is the gitlab-mcp-server command and no Python runs in that path;
// this module serves python -m gitlab_mcp_server and programmatic lookup,
// the shape uv's find_uv_bin takes.
const pypiLauncher = pypiFileHeader + `"""Locator for the gitlab-mcp-server binary installed by this wheel.

The binary itself ships in the wheel's .data/scripts directory, so the
installer places it on the scripts path (bin/ or Scripts/) with the
executable bit set and it IS the ` + "`gitlab-mcp-server`" + ` command; no Python
runs in that path. This module exists for ` + "`python -m gitlab_mcp_server`" + `
and for programmatic lookup, the same shape uv's find_uv_bin takes.
"""

import os
import subprocess
import sys
import sysconfig

__version__ = "%s"

_BIN = "gitlab-mcp-server.exe" if os.name == "nt" else "gitlab-mcp-server"


def find_binary():
    """Return the installed binary's path, searching the scripts
    directories of the running environment."""
    candidates = [
        sysconfig.get_path("scripts"),
        os.path.join(sys.prefix, "Scripts" if os.name == "nt" else "bin"),
        os.path.dirname(sys.executable),
    ]
    for scripts_dir in candidates:
        if not scripts_dir:
            continue
        path = os.path.join(scripts_dir, _BIN)
        if os.path.isfile(path):
            return path
    raise FileNotFoundError(
        "gitlab-mcp-server binary not found next to " + sys.executable)


def main():
    path = find_binary()
    argv = [path] + sys.argv[1:]
    if os.name == "nt":
        try:
            raise SystemExit(subprocess.call(argv))
        except KeyboardInterrupt:
            raise SystemExit(130) from None
    os.execv(path, argv)
`

// pypiMainModule is the wheel's gitlab_mcp_server/__main__.py.
const pypiMainModule = pypiFileHeader + `from gitlab_mcp_server import main

if __name__ == "__main__":
    main()
`

// pypiProjectURLs are the Project-URL fields, label and target. Issues and
// Security are labels the well-known project URLs specification names, so
// PyPI renders them as the issue tracker and the security policy rather than
// as two more links.
var pypiProjectURLs = []string{
	"Homepage, https://github.com/jmrplens/gitlab-mcp-server",
	"Documentation, https://docs.jmrp.io/gitlab-mcp-server/",
	"Repository, https://github.com/jmrplens/gitlab-mcp-server",
	"Changelog, https://github.com/jmrplens/gitlab-mcp-server/releases",
	"Issues, https://github.com/jmrplens/gitlab-mcp-server/issues",
	"Security, https://github.com/jmrplens/gitlab-mcp-server/security/policy",
}

// pypiMetadata is the wheel's METADATA. The license is declared as PEP 639
// and core metadata 2.4 define it, an SPDX License-Expression and a
// License-File for each text under .dist-info/licenses/; the legacy License
// field and the License :: classifier are left out, since the specification
// makes License and License-Expression mutually exclusive and PyPI refuses
// an upload carrying both.
func pypiMetadata(version string, readme []byte) []byte {
	fields := []string{
		"Metadata-Version: 2.4",
		"Name: " + pypiDist,
		"Version: " + version,
		"Summary: GitLab MCP server: GitLab REST v4 and GraphQL as Model Context Protocol tools (native Go binary)",
		"Author: jmrplens",
		"License-Expression: MIT",
		"License-File: " + licensePath,
		"License-File: " + noticesName,
	}
	for _, url := range pypiProjectURLs {
		fields = append(fields, "Project-URL: "+url)
	}
	fields = append(fields,
		"Classifier: Development Status :: 5 - Production/Stable",
		"Classifier: Intended Audience :: Developers",
		"Classifier: Topic :: Software Development",
		"Classifier: Programming Language :: Go",
		"Requires-Python: >=3.9",
		"Description-Content-Type: text/markdown",
	)
	return append([]byte(strings.Join(fields, "\n")+"\n\n"), readme...)
}

// pypiWheelFile is the wheel's WHEEL. The generator is this command, where
// the script named itself.
func pypiWheelFile(tag string) []byte {
	return []byte("Wheel-Version: 1.0\nGenerator: gitlab-mcp-server release_packages\nRoot-Is-Purelib: false\nTag: py3-none-" + tag + "\n")
}

// pypiWheelName is the file name of one platform's wheel.
func pypiWheelName(version, tag string) string {
	return fmt.Sprintf("%s-%s-py3-none-%s.whl", pypiNorm, version, tag)
}

// recordHash is a RECORD digest: the SHA-256, urlsafe base64 without
// padding.
func recordHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256=" + base64.RawURLEncoding.EncodeToString(sum[:])
}

// pypiWheelEntries are the entries of one platform's wheel, RECORD last.
// The binary lives in .data/scripts, which the wheel specification obliges
// the installer to place on the environment's scripts path with the execute
// bit set; a binary inside the package directory has no such guarantee, and
// pip installed one there without it.
func pypiWheelEntries(version string, p releasePlatform, binary, readme, license, notices []byte) []packageEntry {
	distInfo := fmt.Sprintf("%s-%s.dist-info", pypiNorm, version)
	entries := []packageEntry{
		{name: pypiPackage + "/__init__.py", data: []byte(fmt.Sprintf(pypiLauncher, version))},
		{name: pypiPackage + "/__main__.py", data: []byte(pypiMainModule)},
		{name: fmt.Sprintf("%s-%s.data/scripts/%s", pypiNorm, version, p.binary()), data: binary, executable: true},
		{name: distInfo + "/METADATA", data: pypiMetadata(version, readme)},
		{name: distInfo + "/WHEEL", data: pypiWheelFile(p.wheelTag)},
		{name: distInfo + "/entry_points.txt", data: []byte(fmt.Sprintf("[console_scripts]\n%s = %s:main\n", pypiDist, pypiPackage))},
		{name: distInfo + "/licenses/" + licensePath, data: license},
		{name: distInfo + "/licenses/" + noticesName, data: notices},
	}
	var record strings.Builder
	for _, entry := range entries {
		fmt.Fprintf(&record, "%s,%s,%d\n", entry.name, recordHash(entry.data), len(entry.data))
	}
	fmt.Fprintf(&record, "%s/RECORD,,\n", distInfo)
	return append(entries, packageEntry{name: distInfo + "/RECORD", data: []byte(record.String())})
}

// runPypi is the pypi subcommand: one platform wheel per release binary, the
// uv and ruff model, written to --out after it is emptied.
func runPypi(root string, args []string, stdout, stderr io.Writer) int {
	flags := newBuilderFlags("pypi", pypiOutDir, stderr)
	if !flags.parse(args, stderr, true) {
		return 2
	}
	b := &packageBuild{root: root, binaries: flags.binaries, allowUnverified: flags.allowUnverified, stdout: stdout, stderr: stderr, wheels: true}
	if err := b.buildPypi(flags.version, flags.out); err != nil {
		if !errors.Is(err, errRefused) {
			_ = b.refuse("%v", err)
		}
		return 1
	}
	return 0
}

// buildPypi writes the six wheels and the record of what was verified.
func (b *packageBuild) buildPypi(version, out string) error {
	if !releaseVersion.MatchString(version) {
		return b.refuse("version '%s' does not look like a release version", version)
	}
	readme, err := b.readTokenReadme(pypiReadme)
	if err != nil {
		return err
	}
	license, err := b.readLicense()
	if err != nil {
		return err
	}
	in, err := b.readRelease("wheels are being built")
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
		name := pypiWheelName(version, p.wheelTag)
		if writeErr := writePackageZip(filepath.Join(outDir, name), pypiWheelEntries(version, p, in.binaries[p.key], readme, license, in.notices)); writeErr != nil {
			return writeErr
		}
		built = append(built, name)
		if in.verified {
			digests = append(digests, jsonMember{key: p.key, value: jsonText(in.binaryDigests[p.key])})
		}
	}
	if writeErr := writeFileMode(filepath.Join(outDir, verifiedManifest), verifiedRecord(version, in.verified, digests, in.noticesDigest), 0o644); writeErr != nil {
		return writeErr
	}
	for _, name := range built {
		fmt.Fprintf(b.stdout, "built %s\n", path.Join(filepath.ToSlash(out), name))
	}
	fmt.Fprintf(b.stdout, "%s pypi: %d wheels for version %s\n", toolName, len(built), version)
	return nil
}
