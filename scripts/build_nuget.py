#!/usr/bin/env python3
"""Assemble the NuGet distribution from released binaries.

Builds the seven packages of a .NET tool whose entry point is a native
executable, the layout the .NET 10 SDK uses for tools that ship one binary
per platform (tool manifest version 2): one pointer package, gitlab-mcp-server,
that names a package per runtime identifier, and six runtime-identifier
packages that each carry the gitlab-mcp-server binary for one platform.
`dotnet tool install` and `dnx` read the pointer, pick the package for the
host and run the binary directly; nothing in the packages is .NET code.

Standard library only: a .nupkg is a zip with an OPC content-types part and a
nuspec, so the release job and a contributor's machine need a Python
interpreter and no .NET SDK to pack (the validator needs one to install).

Usage:
    python3 scripts/build_nuget.py --binaries dist --version 2.8.0 [--out nuget/dist]

The binaries directory must hold the GoReleaser release assets under their
exact release names (gitlab-mcp-server-<os>-<arch>[.exe]) together with the
release's own checksums.txt, which is cosign-signed at build time. Every
binary is checked against it before it goes into a package: a published NuGet
version can be unlisted but never replaced. Pass --allow-unverified only when
there is genuinely no manifest (never in CI).

Packages land in --out (default nuget/dist), which is wiped first so a rebuild
cannot mix versions.
"""

import argparse
import hashlib
import json
import os
import re
import shutil
import sys
import zipfile
from xml.sax.saxutils import escape

# The bare id is free on NuGet.org and matches the command, the Homebrew
# formula and the image; NuGet ids are case-insensitive and allow hyphens.
PKG_ID = "gitlab-mcp-server"
COMMAND = "gitlab-mcp-server"
AUTHORS = "jmrplens"
SERVER_NAME = "io.github.jmrplens/gitlab-mcp-server"
MCP_NAME_TOKEN = "mcp-name: " + SERVER_NAME

# The pointer package's settings live under tools/<tfm>/any/: the SDK reads
# the tool's target framework from that path even when, as here, nothing in
# the package targets a framework at all.
TOOL_FRAMEWORK = "net10.0"

# Release-asset name fragments mapped to .NET runtime identifiers.
RIDS = {
    "linux-amd64": "linux-x64",
    "linux-arm64": "linux-arm64",
    "darwin-amd64": "osx-x64",
    "darwin-arm64": "osx-arm64",
    "windows-amd64": "win-x64",
    "windows-arm64": "win-arm64",
}

# Deterministic zip entry timestamp (zip's epoch): rebuilding the same
# inputs yields byte-identical packages.
ZIP_DATE = (1980, 1, 1, 0, 0, 0)

NUSPEC_NS = "http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd"

# Every extension a package carries needs a Default entry here, the empty
# extension of the Linux and macOS binaries included; NuGet's client does not
# consult the content types, but the OPC container has to be well formed.
CONTENT_TYPES = """<?xml version="1.0" encoding="utf-8"?>
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
"""

RELS = """<?xml version="1.0" encoding="utf-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Type="http://schemas.microsoft.com/packaging/2010/07/manifest" Target="/{id}.nuspec" Id="R1" />
</Relationships>
"""

POINTER_DESCRIPTION = (
    "GitLab MCP server: GitLab REST v4 and GraphQL as Model Context Protocol tools. "
    "A .NET tool wrapping the native Go binary; install with dotnet tool install or run with dnx."
)

RID_DESCRIPTION = (
    "The {rid} binary of gitlab-mcp-server. This package is selected by the "
    "gitlab-mcp-server tool package for {rid} hosts; install gitlab-mcp-server instead."
)

RID_README = """# gitlab-mcp-server ({rid})

The `{rid}` executable of [gitlab-mcp-server](https://www.nuget.org/packages/gitlab-mcp-server), a Model Context Protocol server for GitLab. The `gitlab-mcp-server` tool package selects this package on `{rid}` hosts; install or run that one:

```bash
dotnet tool install -g gitlab-mcp-server
dnx gitlab-mcp-server
```

Documentation: https://jmrp.io/docs/gitlab-mcp-server/
"""


def nuspec(pkg_id, version, description, package_types, readme=True, icon=False):
    """Render a nuspec with the metadata NuGet.org displays and validates."""
    types = "".join('      <packageType name="{}" />\n'.format(t) for t in package_types)
    optional = ""
    if readme:
        optional += "    <readme>README.md</readme>\n"
    if icon:
        optional += "    <icon>icon.png</icon>\n"
    return (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<package xmlns="{ns}">\n'
        "  <metadata>\n"
        "    <id>{id}</id>\n"
        "    <version>{version}</version>\n"
        "    <authors>{authors}</authors>\n"
        "    <description>{description}</description>\n"
        '    <license type="expression">MIT</license>\n'
        # nuget.org refuses a package whose license is an expression unless the
        # deprecated <licenseUrl> also points at that expression's page, "to
        # provide a better experience for older clients" (the 400 names
        # https://aka.ms/invalidNuGetLicenseUrl). The first push of 2.7.5 hit it.
        "    <licenseUrl>https://licenses.nuget.org/MIT</licenseUrl>\n"
        "    <projectUrl>https://jmrp.io/docs/gitlab-mcp-server/</projectUrl>\n"
        '    <repository type="git" url="https://github.com/jmrplens/gitlab-mcp-server" />\n'
        "{optional}"
        "    <tags>mcp mcp-server model-context-protocol gitlab ai dotnet-tool</tags>\n"
        "    <packageTypes>\n"
        "{types}"
        "    </packageTypes>\n"
        "  </metadata>\n"
        "</package>\n"
    ).format(
        ns=NUSPEC_NS, id=pkg_id, version=version, authors=AUTHORS,
        description=escape(description), optional=optional, types=types,
    )


def pointer_settings():
    """DotnetToolSettings.xml of the pointer: the command, and which package
    carries it for each runtime identifier."""
    packages = "".join(
        '    <RuntimeIdentifierPackage RuntimeIdentifier="{rid}" Id="{id}.{rid}" />\n'.format(rid=rid, id=PKG_ID)
        for rid in RIDS.values()
    )
    return (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<DotNetCliTool Version="2">\n'
        "  <Commands>\n"
        '    <Command Name="{command}" />\n'
        "  </Commands>\n"
        "  <RuntimeIdentifierPackages>\n"
        "{packages}"
        "  </RuntimeIdentifierPackages>\n"
        "</DotNetCliTool>\n"
    ).format(command=COMMAND, packages=packages)


def rid_settings(bin_name):
    """DotnetToolSettings.xml of a runtime package: the command runs the
    native executable beside it, with no .NET host in between."""
    return (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<DotNetCliTool Version="2">\n'
        "  <Commands>\n"
        '    <Command Name="{command}" EntryPoint="{entry}" Runner="executable" />\n'
        "  </Commands>\n"
        "</DotNetCliTool>\n"
    ).format(command=COMMAND, entry=bin_name)


def mcp_server_json(server_json, version):
    """Derive the .mcp/server.json NuGet.org reads from the repository's
    MCP Registry manifest, stamped with the version being packed.

    The repository file is stamped by the release only after the packages are
    published, so the copy that ships has to carry this build's version
    itself. Only the nuget package entry is kept: it is the one NuGet.org
    renders an install snippet from, and a remote or another registry's entry
    says nothing about this package.
    """
    entries = [p for p in server_json.get("packages", [])
               if p.get("registryType") == "nuget" and p.get("identifier") == PKG_ID]
    if len(entries) != 1:
        sys.exit("build_nuget: server.json must declare exactly one nuget package with identifier {}".format(PKG_ID))
    entry = dict(entries[0])
    entry["version"] = version
    doc = {"$schema": server_json.get("$schema")}
    for key in ("name", "title", "description"):
        if key in server_json:
            doc[key] = server_json[key]
    doc["version"] = version
    for key in ("repository", "websiteUrl"):
        if key in server_json:
            doc[key] = server_json[key]
    doc["packages"] = [entry]
    return json.dumps(doc, indent=2) + "\n"


def read_checksums(binaries_dir):
    """Parse `<sha256>  <name>` lines from the release's checksums.txt.

    Returns None when the file is absent, so the caller decides whether an
    unverified build is acceptable.
    """
    path = os.path.join(binaries_dir, "checksums.txt")
    if not os.path.isfile(path):
        return None
    entries = {}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            parts = line.replace("\r", "").strip().split(None, 1)
            if len(parts) == 2 and re.fullmatch(r"[0-9a-f]{64}", parts[0]):
                entries[parts[1].lstrip("*")] = parts[0]
    return entries


def verify_binary(binary_path, checksums):
    """Abort unless the file matches the digest the signed manifest names."""
    name = os.path.basename(binary_path)
    want = checksums.get(name)
    if want is None:
        sys.exit("build_nuget: {} is not listed in checksums.txt".format(name))
    with open(binary_path, "rb") as fh:
        got = hashlib.sha256(fh.read()).hexdigest()
    if got != want:
        sys.exit("build_nuget: {} is sha256 {}, but checksums.txt says {}".format(name, got, want))
    return got


def add_file(zf, arcname, data, executable=False):
    if isinstance(data, str):
        data = data.encode("utf-8")
    info = zipfile.ZipInfo(arcname, date_time=ZIP_DATE)
    # A regular file with the executable bits: the SDK extracts the entry with
    # this mode, and a binary installed without +x is a tool that cannot run.
    mode = 0o100755 if executable else 0o100644
    info.external_attr = mode << 16
    info.compress_type = zipfile.ZIP_DEFLATED
    zf.writestr(info, data)


def package_name(pkg_id, version):
    return "{}.{}.nupkg".format(pkg_id, version)


def build_pointer(out_dir, version, readme, icon, server_doc):
    name = package_name(PKG_ID, version)
    with zipfile.ZipFile(os.path.join(out_dir, name), "w") as zf:
        add_file(zf, "[Content_Types].xml", CONTENT_TYPES)
        add_file(zf, "_rels/.rels", RELS.format(id=PKG_ID))
        add_file(zf, PKG_ID + ".nuspec",
                 nuspec(PKG_ID, version, POINTER_DESCRIPTION, ("DotnetTool", "McpServer"),
                        icon=icon is not None))
        add_file(zf, "tools/{}/any/DotnetToolSettings.xml".format(TOOL_FRAMEWORK), pointer_settings())
        add_file(zf, ".mcp/server.json", server_doc)
        add_file(zf, "README.md", readme)
        if icon is not None:
            add_file(zf, "icon.png", icon)
    return name


def build_rid_package(out_dir, version, plat_key, rid, binary_path):
    pkg_id = "{}.{}".format(PKG_ID, rid)
    name = package_name(pkg_id, version)
    bin_name = COMMAND + (".exe" if plat_key.startswith("windows") else "")
    with open(binary_path, "rb") as fh:
        binary = fh.read()
    with zipfile.ZipFile(os.path.join(out_dir, name), "w") as zf:
        add_file(zf, "[Content_Types].xml", CONTENT_TYPES)
        add_file(zf, "_rels/.rels", RELS.format(id=pkg_id))
        add_file(zf, pkg_id + ".nuspec",
                 nuspec(pkg_id, version, RID_DESCRIPTION.format(rid=rid), ("DotnetToolRidPackage",)))
        add_file(zf, "tools/any/{}/DotnetToolSettings.xml".format(rid), rid_settings(bin_name))
        add_file(zf, "tools/any/{}/{}".format(rid, bin_name), binary, executable=True)
        add_file(zf, "README.md", RID_README.format(rid=rid))
    return name


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binaries", required=True, help="directory holding the release binaries")
    parser.add_argument("--version", required=True, help="release version, e.g. 2.8.0")
    parser.add_argument("--out", default="nuget/dist", help="package output directory")
    parser.add_argument(
        "--allow-unverified",
        action="store_true",
        help="build without a checksums.txt manifest (never in CI)",
    )
    args = parser.parse_args()

    if not re.fullmatch(r"\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?", args.version):
        sys.exit("build_nuget: version {!r} does not look like a release version".format(args.version))

    root = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
    with open(os.path.join(root, "nuget", "README.md"), encoding="utf-8") as fh:
        readme = fh.read()
    if MCP_NAME_TOKEN not in readme:
        sys.exit("build_nuget: nuget/README.md lost the mcp-name ownership token the MCP Registry validates")
    with open(os.path.join(root, "server.json"), encoding="utf-8") as fh:
        server_doc = mcp_server_json(json.load(fh), args.version)
    icon_path = os.path.join(root, "mcpb", "icon.png")
    icon = None
    if os.path.isfile(icon_path):
        with open(icon_path, "rb") as fh:
            icon = fh.read()

    checksums = read_checksums(args.binaries)
    if checksums is None and not args.allow_unverified:
        sys.exit(
            "build_nuget: {} not found: refusing to package unverified binaries "
            "(pass --allow-unverified to override)".format(os.path.join(args.binaries, "checksums.txt"))
        )
    if checksums is None:
        sys.stderr.write("WARNING: --allow-unverified: packages are being built without a checksum manifest\n")

    if os.path.isdir(args.out):
        shutil.rmtree(args.out)
    os.makedirs(args.out)

    built = []
    digests = {}
    for plat_key, rid in RIDS.items():
        suffix = ".exe" if plat_key.startswith("windows") else ""
        binary_path = os.path.join(args.binaries, "{}-{}{}".format(COMMAND, plat_key, suffix))
        if not os.path.isfile(binary_path):
            sys.exit("build_nuget: missing release binary {}".format(binary_path))
        if checksums is not None:
            digests[plat_key] = verify_binary(binary_path, checksums)
        built.append(build_rid_package(args.out, args.version, plat_key, rid, binary_path))
    built.append(build_pointer(args.out, args.version, readme, icon, server_doc))

    # Record what was verified so validate_nuget.py can confirm the packages
    # still carry those exact bytes. Written beside the packages, never inside
    # one.
    with open(os.path.join(args.out, "verified-binaries.json"), "w", encoding="utf-8") as fh:
        json.dump({"version": args.version, "verified": checksums is not None, "binaries": digests}, fh, indent=2)
        fh.write("\n")

    for name in built:
        print("built", os.path.join(args.out, name))
    print("build_nuget: {} packages for version {}".format(len(built), args.version))


if __name__ == "__main__":
    main()
