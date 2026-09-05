#!/usr/bin/env python3
"""Validate the NuGet packages before anything is pushed.

A pushed NuGet version can be unlisted but never replaced or deleted, so
everything checkable is checked before `dotnet nuget push` ever sees the
packages:

- the pointer package and all six runtime-identifier packages exist for the
  given version, and nothing else;
- each package is a well-formed OPC container whose relationships part names
  its nuspec, and the nuspec's id, version and package types are the ones the
  SDK and NuGet.org expect for that role;
- the pointer's DotnetToolSettings.xml declares the command and maps exactly
  the six runtime identifiers to the six packages that carry it;
- the pointer's README carries the mcp-name ownership token the MCP Registry
  validates, on a boundary the registry's matcher accepts, and its
  .mcp/server.json names this package at this version;
- each runtime package holds exactly one binary, at the path its settings
  name, with the right magic number and machine type for its runtime
  identifier, the executable bit set, and a size floor;
- every embedded binary is the exact one build_nuget.py verified against the
  release's cosign-signed checksums.txt: the checks above are shape checks,
  and a wrong-but-plausible file of the right size with the right first bytes
  passes all of them for the five platforms this host cannot execute;
- the tool is installed from the packages with `dotnet tool install` into a
  throwaway directory and run again through `dnx`, and both must answer an
  MCP initialize handshake over stdio with pure JSON-RPC and the right server
  version (skipped with --no-install; needs the .NET 10 SDK on PATH).

Standard library only. Usage:
    python3 scripts/validate_nuget.py --packages nuget/dist --version 2.8.0
"""

import argparse
import hashlib
import json
import os
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import threading
import xml.etree.ElementTree as ET
import zipfile

PKG_ID = "gitlab-mcp-server"
COMMAND = "gitlab-mcp-server"
SERVER_NAME = "io.github.jmrplens/gitlab-mcp-server"
MCP_NAME_TOKEN = "mcp-name: " + SERVER_NAME
TOOL_FRAMEWORK = "net10.0"
MIN_BINARY_BYTES = 20 * 1024 * 1024
NUSPEC_NS = "{http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd}"
VERIFIED_MANIFEST = "verified-binaries.json"

# Runtime identifier -> the plat_key build_nuget.py records digests under.
RID_PLAT_KEYS = {
    "linux-x64": "linux-amd64",
    "linux-arm64": "linux-arm64",
    "osx-x64": "darwin-amd64",
    "osx-arm64": "darwin-arm64",
    "win-x64": "windows-amd64",
    "win-arm64": "windows-arm64",
}

# The registry accepts the token followed by whitespace, a tag, a comment
# close or the end of the text; a token glued to a longer name is rejected.
TOKEN_PATTERN = re.compile(r"(^|\s)" + re.escape(MCP_NAME_TOKEN) + r"(?=\s|<|-->|$)")

HANDSHAKE_TIMEOUT = 120


def bin_name_for(rid):
    return COMMAND + (".exe" if rid.startswith("win") else "")


def check_magic(rid, data):
    if rid.startswith("linux"):
        if data[:4] != b"\x7fELF":
            return "not an ELF binary"
        machine = struct.unpack_from("<H", data, 18)[0]
        want = 0x3E if rid.endswith("x64") else 0xB7
        if machine != want:
            return "ELF machine 0x{:x}, want 0x{:x}".format(machine, want)
    elif rid.startswith("osx"):
        if data[:4] != b"\xcf\xfa\xed\xfe":
            return "not a 64-bit Mach-O binary"
        cputype = struct.unpack_from("<I", data, 4)[0]
        want = 0x01000007 if rid.endswith("x64") else 0x0100000C
        if cputype != want:
            return "Mach-O cputype 0x{:x}, want 0x{:x}".format(cputype, want)
    else:
        if data[:2] != b"MZ":
            return "not a PE binary"
        pe_off = struct.unpack_from("<I", data, 0x3C)[0]
        if data[pe_off:pe_off + 4] != b"PE\0\0":
            return "MZ without PE header"
        machine = struct.unpack_from("<H", data, pe_off + 4)[0]
        want = 0x8664 if rid.endswith("x64") else 0xAA64
        if machine != want:
            return "PE machine 0x{:x}, want 0x{:x}".format(machine, want)
    return None


def read_verified(packages_dir, version, problems):
    """Load the digests build_nuget.py recorded, or fail if there are none.

    The manifest lives beside the packages and is removed once read, so the
    publish step never sees a non-package file in the directory it pushes.
    """
    path = os.path.join(packages_dir, VERIFIED_MANIFEST)
    if not os.path.isfile(path):
        problems.append("packages were assembled without {}: build_nuget.py did not check them "
                        "against the release's checksums.txt".format(VERIFIED_MANIFEST))
        return {}
    with open(path, encoding="utf-8") as fh:
        manifest = json.load(fh)
    os.remove(path)
    if not manifest.get("verified"):
        problems.append("{} says the packages were built with --allow-unverified".format(VERIFIED_MANIFEST))
    if manifest.get("version") != version:
        problems.append("{} records version {} but this is {}".format(
            VERIFIED_MANIFEST, manifest.get("version"), version))
    return manifest.get("binaries") or {}


def read_nuspec(zf, pkg_id, name, problems):
    """Check the OPC skeleton and return the parsed nuspec metadata, or None."""
    names = zf.namelist()
    if "[Content_Types].xml" not in names:
        problems.append("{}: no [Content_Types].xml (not an OPC container)".format(name))
    rels_name = "_rels/.rels"
    if rels_name not in names:
        problems.append("{}: no _rels/.rels".format(name))
    elif "/{}.nuspec".format(pkg_id) not in zf.read(rels_name).decode("utf-8"):
        problems.append("{}: _rels/.rels does not name /{}.nuspec as the manifest".format(name, pkg_id))
    nuspec_name = pkg_id + ".nuspec"
    if nuspec_name not in names:
        problems.append("{}: {} missing".format(name, nuspec_name))
        return None
    try:
        root = ET.fromstring(zf.read(nuspec_name))
    except ET.ParseError as exc:
        problems.append("{}: {} does not parse: {}".format(name, nuspec_name, exc))
        return None
    metadata = root.find(NUSPEC_NS + "metadata")
    if metadata is None:
        problems.append("{}: {} has no <metadata>".format(name, nuspec_name))
        return None
    return metadata


def nuspec_text(metadata, element):
    node = metadata.find(NUSPEC_NS + element)
    return None if node is None else (node.text or "")


def nuspec_package_types(metadata):
    return sorted(
        node.get("name", "")
        for node in metadata.findall(NUSPEC_NS + "packageTypes/" + NUSPEC_NS + "packageType")
    )


def check_metadata(metadata, name, pkg_id, version, package_types, problems):
    if nuspec_text(metadata, "id") != pkg_id:
        problems.append("{}: nuspec id is {!r}, want {!r}".format(name, nuspec_text(metadata, "id"), pkg_id))
    if nuspec_text(metadata, "version") != version:
        problems.append("{}: nuspec version is {!r}, want {!r}".format(
            name, nuspec_text(metadata, "version"), version))
    got = nuspec_package_types(metadata)
    if got != sorted(package_types):
        problems.append("{}: nuspec packageTypes are {}, want {}".format(name, got, sorted(package_types)))
    for element in ("authors", "description", "license"):
        if metadata.find(NUSPEC_NS + element) is None:
            problems.append("{}: nuspec lacks <{}>".format(name, element))
    # nuget.org rejects a license expression whose <licenseUrl> is missing or
    # points anywhere but the expression's own page; the push is a 400 that
    # nothing before it reports.
    license_url = nuspec_text(metadata, "licenseUrl")
    if license_url != "https://licenses.nuget.org/MIT":
        problems.append("{}: nuspec licenseUrl is {!r}, want 'https://licenses.nuget.org/MIT' beside the MIT expression".format(
            name, license_url))


def check_declared_file(zf, metadata, name, element, problems):
    """A nuspec <readme> or <icon> names a file that has to be in the package."""
    declared = nuspec_text(metadata, element)
    if declared is None:
        return None
    if declared not in zf.namelist():
        problems.append("{}: nuspec <{}> names {} but the package has no such file".format(name, element, declared))
        return None
    return declared


def parse_settings(zf, arcname, name, problems):
    if arcname not in zf.namelist():
        problems.append("{}: {} missing".format(name, arcname))
        return None
    try:
        root = ET.fromstring(zf.read(arcname))
    except ET.ParseError as exc:
        problems.append("{}: {} does not parse: {}".format(name, arcname, exc))
        return None
    if root.tag != "DotNetCliTool" or root.get("Version") != "2":
        problems.append("{}: {} is not a DotNetCliTool manifest of Version 2 "
                        "(the runtime-identifier layout needs it)".format(name, arcname))
        return None
    return root


def check_readme_token(zf, readme_name, name, problems):
    text = zf.read(readme_name).decode("utf-8", errors="replace")
    if not TOKEN_PATTERN.search(text):
        problems.append("{}: {} lost the MCP Registry ownership token ({!r} on its own boundary)".format(
            name, readme_name, MCP_NAME_TOKEN))


def validate_pointer(path, version, problems):
    name = os.path.basename(path)
    with zipfile.ZipFile(path) as zf:
        metadata = read_nuspec(zf, PKG_ID, name, problems)
        if metadata is None:
            return
        check_metadata(metadata, name, PKG_ID, version, ("DotnetTool", "McpServer"), problems)

        readme_name = check_declared_file(zf, metadata, name, "readme", problems)
        if readme_name is None:
            problems.append("{}: nuspec declares no <readme>; the MCP Registry validates NuGet ownership "
                            "through the published README".format(name))
        else:
            check_readme_token(zf, readme_name, name, problems)
        check_declared_file(zf, metadata, name, "icon", problems)

        settings = parse_settings(zf, "tools/{}/any/DotnetToolSettings.xml".format(TOOL_FRAMEWORK), name, problems)
        if settings is not None:
            commands = settings.findall("Commands/Command")
            if len(commands) != 1 or commands[0].get("Name") != COMMAND or commands[0].get("EntryPoint"):
                problems.append("{}: the pointer must declare exactly one command named {} with no entry point "
                                "(the runtime packages carry it)".format(name, COMMAND))
            declared = {
                node.get("RuntimeIdentifier"): node.get("Id")
                for node in settings.findall("RuntimeIdentifierPackages/RuntimeIdentifierPackage")
            }
            want = {rid: "{}.{}".format(PKG_ID, rid) for rid in RID_PLAT_KEYS}
            if declared != want:
                problems.append("{}: RuntimeIdentifierPackages are {}, want {}".format(name, declared, want))

        if ".mcp/server.json" not in zf.namelist():
            problems.append("{}: no .mcp/server.json (NuGet.org renders the install snippet from it)".format(name))
        else:
            try:
                doc = json.loads(zf.read(".mcp/server.json").decode("utf-8"))
            except ValueError as exc:
                problems.append("{}: .mcp/server.json is not JSON: {}".format(name, exc))
                doc = {}
            if doc.get("name") != SERVER_NAME:
                problems.append("{}: .mcp/server.json names {!r}, want {!r}".format(name, doc.get("name"), SERVER_NAME))
            if doc.get("version") != version:
                problems.append("{}: .mcp/server.json carries version {!r}, want {!r}".format(
                    name, doc.get("version"), version))
            entries = [p for p in doc.get("packages", []) if p.get("registryType") == "nuget"]
            if len(entries) != 1 or entries[0].get("identifier") != PKG_ID or entries[0].get("version") != version:
                problems.append("{}: .mcp/server.json must declare one nuget package, {} at {}".format(
                    name, PKG_ID, version))

        binaries = [n for n in zf.namelist() if os.path.basename(n) in (COMMAND, COMMAND + ".exe")]
        if binaries:
            problems.append("{}: the pointer carries a binary ({}); only runtime packages do".format(
                name, ", ".join(binaries)))


def validate_rid_package(path, version, rid, verified, problems):
    name = os.path.basename(path)
    pkg_id = "{}.{}".format(PKG_ID, rid)
    bin_name = bin_name_for(rid)
    with zipfile.ZipFile(path) as zf:
        metadata = read_nuspec(zf, pkg_id, name, problems)
        if metadata is None:
            return
        check_metadata(metadata, name, pkg_id, version, ("DotnetToolRidPackage",), problems)
        check_declared_file(zf, metadata, name, "readme", problems)

        settings = parse_settings(zf, "tools/any/{}/DotnetToolSettings.xml".format(rid), name, problems)
        if settings is not None:
            commands = settings.findall("Commands/Command")
            if (len(commands) != 1 or commands[0].get("Name") != COMMAND
                    or commands[0].get("EntryPoint") != bin_name or commands[0].get("Runner") != "executable"):
                problems.append("{}: the command must be {} with EntryPoint={} and Runner=executable".format(
                    name, COMMAND, bin_name))

        prefix = "tools/any/{}/".format(rid)
        payload = [n for n in zf.namelist() if n.startswith(prefix) and not n.endswith("DotnetToolSettings.xml")]
        arc = prefix + bin_name
        if payload != [arc]:
            problems.append("{}: expected exactly {} under {}, found {}".format(name, arc, prefix, payload))
            return
        info = zf.getinfo(arc)
        data = zf.read(arc)
        if len(data) < MIN_BINARY_BYTES:
            problems.append("{}: binary is {} bytes, under the {} floor".format(name, len(data), MIN_BINARY_BYTES))
        mode = info.external_attr >> 16
        if not rid.startswith("win") and not ((mode & 0o170000) == 0o100000 and mode & 0o111):
            problems.append("{}: binary must be a regular file with the executable bit, got mode {:o}".format(
                name, mode))
        problem = check_magic(rid, data)
        if problem:
            problems.append("{}: {}".format(name, problem))
        if verified:
            want = verified.get(RID_PLAT_KEYS[rid])
            got = hashlib.sha256(data).hexdigest()
            if want != got:
                problems.append("{}: the embedded binary is sha256 {}, but the release's signed "
                                "checksums.txt named {}".format(name, got, want))


def expected_packages(version):
    """Package file name -> runtime identifier (None for the pointer)."""
    expected = {"{}.{}.nupkg".format(PKG_ID, version): None}
    for rid in RID_PLAT_KEYS:
        expected["{}.{}.{}.nupkg".format(PKG_ID, rid, version)] = rid
    return expected


def validate_packages(packages_dir, version):
    """Run every offline check and return the problems found."""
    problems = []
    verified = read_verified(packages_dir, version, problems)
    expected = expected_packages(version)

    present = sorted(f for f in os.listdir(packages_dir) if f.endswith(".nupkg"))
    if set(present) != set(expected):
        problems.append("the directory holds {} but the release needs exactly {}".format(present, sorted(expected)))

    leftovers = sorted(f for f in os.listdir(packages_dir) if not f.endswith(".nupkg"))
    if leftovers:
        problems.append("the directory holds files the publish step would try to push: {}".format(leftovers))

    for fname in present:
        if fname not in expected:
            continue
        path = os.path.join(packages_dir, fname)
        if not zipfile.is_zipfile(path):
            problems.append("{}: not a zip archive".format(fname))
            continue
        rid = expected[fname]
        if rid is None:
            validate_pointer(path, version, problems)
        else:
            validate_rid_package(path, version, rid, verified, problems)
    return problems


def dotnet_env():
    """The environment every dotnet invocation here runs under.

    The first-use banner and telemetry notice are printed on stdout, which is
    the stream the handshake below has to find pure JSON-RPC on; the
    variables turn both off for the validator's own runs.
    """
    return {**os.environ, "DOTNET_NOLOGO": "1", "DOTNET_CLI_TELEMETRY_OPTOUT": "1",
            "DOTNET_SKIP_FIRST_TIME_EXPERIENCE": "1"}


def handshake(packages_dir, version, problems):
    """Install from the packages and drive an MCP initialize over stdio,
    once through the installed shim and once through dnx."""
    source = os.path.abspath(packages_dir)
    tmp = tempfile.mkdtemp(prefix="nuget-validate-")
    try:
        tool_path = os.path.join(tmp, "tools")
        # --source replaces every configured feed, so the package installed is
        # the one in this directory and nothing nuget.org may hold under the
        # same id and version.
        install = subprocess.run(
            ["dotnet", "tool", "install", PKG_ID, "--version", version,
             "--tool-path", tool_path, "--source", source, "--ignore-failed-sources"],
            env=dotnet_env(), capture_output=True, text=True, check=False,
        )
        if install.returncode != 0:
            problems.append("dotnet tool install failed:\n{}{}".format(install.stdout, install.stderr))
            return
        shim = os.path.join(tool_path, bin_name_for("win-x64" if os.name == "nt" else "linux-x64"))
        if not os.path.isfile(shim):
            problems.append("dotnet tool install left no {} in {}".format(os.path.basename(shim), tool_path))
            return
        run_handshake([shim], version, tmp, "dotnet tool install", problems)
        # `dotnet dnx` rather than the dnx script: the script sits in the SDK
        # root, which is on PATH on a developer machine but not necessarily
        # after a CI SDK install, and the two are the same command.
        run_handshake(
            ["dotnet", "dnx", "{}@{}".format(PKG_ID, version), "--source", source, "--ignore-failed-sources"],
            version, tmp, "dnx", problems,
        )
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def run_handshake(argv, version, tmp, label, problems):
    request = json.dumps({
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {"protocolVersion": "2025-11-25", "capabilities": {},
                   "clientInfo": {"name": "validate-nuget", "version": "1"}},
    }) + "\n"
    stderr_path = os.path.join(tmp, "stderr.log")
    env = dotnet_env()
    env.update({
        "GITLAB_URL": "http://127.0.0.1:1",
        "GITLAB_TOKEN": "glpat-validate-nuget",
        "GITLAB_MCP_LOG_LEVEL": "error",
        # The fake URL is unreachable by design; skipping scope detection
        # avoids its retry loop in the handshake path.
        "GITLAB_MCP_IGNORE_SCOPES": "true",
    })
    with open(stderr_path, "wb") as stderr_file:
        proc = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr_file, env=env)
        # stdin stays open while the response is read: closing it signals
        # shutdown to a stdio MCP server, and a server told to shut down before
        # it answered is allowed to just go.
        line_holder = []
        reader = threading.Thread(target=lambda: line_holder.append(proc.stdout.readline()))
        reader.daemon = True
        proc.stdin.write(request.encode("utf-8"))
        proc.stdin.flush()
        reader.start()
        reader.join(timeout=HANDSHAKE_TIMEOUT)
        proc.stdin.close()
        proc.terminate()
        try:
            proc.wait(timeout=15)
        except subprocess.TimeoutExpired:
            proc.kill()
    if not line_holder or not line_holder[0]:
        with open(stderr_path, "rb") as fh:
            tail = fh.read()[-800:]
        problems.append("handshake ({}): no answer within {}s; stderr tail: {!r}".format(
            label, HANDSHAKE_TIMEOUT, tail))
        return
    line = line_holder[0].rstrip(b"\n")
    try:
        response = json.loads(line)
    except ValueError:
        problems.append("handshake ({}): stdout is not pure JSON-RPC: {!r}".format(label, line[:120]))
        return
    server_info = response.get("result", {}).get("serverInfo", {})
    if server_info.get("version") != version:
        problems.append("handshake ({}): serverInfo.version = {!r}, want {!r}".format(
            label, server_info.get("version"), version))
    else:
        print("handshake:", label, "answered initialize with version", version)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--packages", default="nuget/dist")
    parser.add_argument("--version", required=True)
    parser.add_argument("--no-install", action="store_true",
                        help="skip the dotnet tool install, dnx and MCP handshake")
    args = parser.parse_args()

    problems = validate_packages(args.packages, args.version)
    if not args.no_install and not problems:
        if shutil.which("dotnet") is None:
            problems.append("dotnet is not on PATH; install the .NET 10 SDK or pass --no-install")
        else:
            handshake(args.packages, args.version, problems)

    for problem in problems:
        print("FAIL:", problem)
    if problems:
        sys.exit("validate_nuget: {} failure(s)".format(len(problems)))
    print("validate_nuget: all packages valid for version", args.version)


if __name__ == "__main__":
    main()
