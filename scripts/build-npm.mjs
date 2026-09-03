// build-npm.mjs assembles the npm publish set from the release binaries.
//
// The npm distribution is one thin launcher package plus one package per
// platform that carries the matching prebuilt binary (the esbuild/biome model:
// npm installs only the package whose os/cpu fit, and no code runs at install
// time). This script is the single place the two vocabularies meet — Node's
// platform/arch names on the npm side, Go's GOOS/GOARCH on the asset side — so
// nothing downstream has to translate.
//
// Usage:
//   node scripts/build-npm.mjs --binaries <dir> --version <x.y.z> [--out <dir>]
//   node scripts/build-npm.mjs --sync-only --version <x.y.z>
//
// Every binary is checked against <dir>/checksums.txt — the release's own
// manifest, cosign-signed at build time — before it is copied into a package.
// The packages used to be assembled from an unverified `cp` of the build
// directory, so a stale or swapped binary reached an immutable registry with
// nothing looking at it: the validator checks a size floor, a magic number and
// one handshake on the host platform, all of which a wrong-but-plausible file
// passes. Pass --allow-unverified only when there is genuinely no manifest
// (never in CI).
//
// <dir> holds the release assets under their published names
// (gitlab-mcp-server-linux-amd64, …). Output is one directory per package under
// <out> (default npm/packages), plus the main package's version and dependency
// pins rewritten in place under npm/gitlab-mcp-server.
//
// --sync-only rewrites just the committed main package.json (version and the
// optionalDependency pins) and builds nothing. It needs no binaries, so the
// release version-stamp step can keep the checked-in file honest between
// releases without staging a whole distribution.

import { createHash } from "node:crypto";
import { chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

// PLATFORMS is the whole distribution matrix. `key` is the npm suffix and the
// runtime lookup the launcher performs; `os`/`cpu` gate the install; `asset` is
// the release filename the binary is copied from. Keep this in lockstep with
// the launcher's supported set and the main package's optionalDependencies.
const PLATFORMS = [
  // libc: the release binaries are PIE ELFs linked against the glibc dynamic
  // loader (CGO_ENABLED=0 + -buildmode=pie), so on musl (Alpine) npm skips
  // these packages and the launcher points users at the Docker image.
  { key: "linux-x64", os: "linux", cpu: "x64", libc: "glibc", asset: "gitlab-mcp-server-linux-amd64", exe: false },
  { key: "linux-arm64", os: "linux", cpu: "arm64", libc: "glibc", asset: "gitlab-mcp-server-linux-arm64", exe: false },
  { key: "darwin-x64", os: "darwin", cpu: "x64", asset: "gitlab-mcp-server-darwin-amd64", exe: false },
  { key: "darwin-arm64", os: "darwin", cpu: "arm64", asset: "gitlab-mcp-server-darwin-arm64", exe: false },
  { key: "win32-x64", os: "win32", cpu: "x64", asset: "gitlab-mcp-server-windows-amd64.exe", exe: true },
  { key: "win32-arm64", os: "win32", cpu: "arm64", asset: "gitlab-mcp-server-windows-arm64.exe", exe: true },
];

function parseArgs(argv) {
  const out = {
    binaries: null,
    version: null,
    out: join(repoRoot, "npm", "packages"),
    syncOnly: false,
    allowUnverified: false,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--binaries") out.binaries = argv[++i];
    else if (arg === "--version") out.version = argv[++i];
    else if (arg === "--out") out.out = argv[++i];
    else if (arg === "--sync-only") out.syncOnly = true;
    else if (arg === "--allow-unverified") out.allowUnverified = true;
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (!out.version) throw new Error("--version <x.y.z> is required");
  if (!/^\d+\.\d+\.\d+$/.test(out.version)) throw new Error(`--version must be semver, got ${out.version}`);
  if (!out.syncOnly && !out.binaries) throw new Error("--binaries <dir> is required (or pass --sync-only)");
  return out;
}

// sha256 of a file, hex, the same form checksums.txt uses.
function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

// readChecksums parses GoReleaser's checksums.txt ("<hex>  <name>" lines).
// Returns null when the file is absent so the caller can decide whether that
// is acceptable.
function readChecksums(binariesDir) {
  const path = join(binariesDir, "checksums.txt");
  if (!existsSync(path)) return null;
  const entries = new Map();
  for (const line of readFileSync(path, "utf8").split("\n")) {
    const m = line.trim().match(/^([0-9a-f]{64})\s+\*?(.+)$/);
    if (m) entries.set(m[2], m[1]);
  }
  return entries;
}

const mainRepository = {
  type: "git",
  url: "git+https://github.com/jmrplens/gitlab-mcp-server.git",
};

// writePlatformPackage emits one per-platform package: the binary renamed to a
// stable name the launcher resolves, made executable so the bit survives into
// the tarball, and a package.json whose os/cpu confine the install to the
// platform it serves.
function writePlatformPackage(plat, version, binariesDir, outDir, checksums) {
  const dir = join(outDir, plat.key);
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });

  const binaryName = plat.exe ? "gitlab-mcp-server.exe" : "gitlab-mcp-server";
  const src = join(binariesDir, plat.asset);
  const digest = sha256(src);
  if (checksums) {
    const want = checksums.get(plat.asset);
    if (!want) {
      throw new Error(`${plat.asset} is not listed in ${join(binariesDir, "checksums.txt")}`);
    }
    if (want !== digest) {
      throw new Error(`${plat.asset} is sha256 ${digest}, but checksums.txt says ${want}`);
    }
  }
  const dst = join(dir, binaryName);
  copyFileSync(src, dst);
  // 0o755 both here and in the copy: npm records the file mode in the tarball,
  // so a binary packed without the executable bit installs un-runnable on the
  // consumer's machine.
  chmodSync(dst, 0o755);

  const pkg = {
    name: `@jmrp.io/gitlab-mcp-server-${plat.key}`,
    version,
    description: `gitlab-mcp-server prebuilt binary for ${plat.os} ${plat.cpu}. Installed automatically as an optional dependency of @jmrp.io/gitlab-mcp-server.`,
    license: "MIT",
    funding: "https://github.com/sponsors/jmrplens",
    author: "José M. Requena Plens",
    homepage: "https://jmrp.io/docs/gitlab-mcp-server",
    repository: mainRepository,
    os: [plat.os],
    cpu: [plat.cpu],
    ...(plat.libc ? { libc: [plat.libc] } : {}),
    files: [binaryName],
    preferUnplugged: true,
  };
  writeFileSync(join(dir, "package.json"), JSON.stringify(pkg, null, 2) + "\n");
  writeFileSync(
    join(dir, "README.md"),
    `# @jmrp.io/gitlab-mcp-server-${plat.key}\n\n` +
      `The ${plat.os}/${plat.cpu} binary for ` +
      `[@jmrp.io/gitlab-mcp-server](https://www.npmjs.com/package/@jmrp.io/gitlab-mcp-server). ` +
      "You do not install this directly; it comes in as an optional dependency of the main package.\n",
  );
  return { name: pkg.name, dir, binaryName, digest };
}

// syncMainPackage rewrites the launcher package's own version and pins every
// optional dependency to the same version, so the whole set moves as one and a
// consumer never resolves a launcher against a mismatched binary package.
function syncMainPackage(version, outDir) {
  const dir = join(repoRoot, "npm", "gitlab-mcp-server");
  const path = join(dir, "package.json");
  const pkg = JSON.parse(readFileSync(path, "utf8"));
  pkg.version = version;
  pkg.optionalDependencies = Object.fromEntries(
    PLATFORMS.map((p) => [`@jmrp.io/gitlab-mcp-server-${p.key}`, version]),
  );
  writeFileSync(path, JSON.stringify(pkg, null, 2) + "\n");
  return { name: pkg.name, dir };
}

function main() {
  const args = parseArgs(process.argv.slice(2));

  if (args.syncOnly) {
    const mainPackage = syncMainPackage(args.version, args.out);
    process.stdout.write(`npm main package synced to v${args.version} (${mainPackage.name})\n`);
    return;
  }

  mkdirSync(args.out, { recursive: true });

  const checksums = readChecksums(args.binaries);
  if (!checksums && !args.allowUnverified) {
    throw new Error(
      `${join(args.binaries, "checksums.txt")} not found — refusing to package unverified binaries ` +
        "(pass --allow-unverified to override)",
    );
  }
  if (!checksums) {
    process.stderr.write("WARNING: --allow-unverified — binaries are being packaged without a checksum manifest\n");
  }

  const platformPackages = PLATFORMS.map((p) =>
    writePlatformPackage(p, args.version, args.binaries, args.out, checksums),
  );
  const mainPackage = syncMainPackage(args.version, args.out);

  // Record what was verified so validate-npm.mjs can confirm the packed
  // tarballs still carry those exact bytes. Written beside the package
  // directories, never inside one, so it cannot be published.
  writeFileSync(
    join(args.out, "verified-binaries.json"),
    JSON.stringify(
      {
        version: args.version,
        verified: Boolean(checksums),
        binaries: Object.fromEntries(platformPackages.map((p, i) => [PLATFORMS[i].key, p.digest])),
      },
      null,
      2,
    ) + "\n",
  );

  // Publish order matters: the platform packages must exist on the registry
  // before the launcher that lists them, or an install racing the publish
  // resolves optional dependencies that are not there yet.
  process.stdout.write(`npm distribution assembled for v${args.version}\n\n`);
  process.stdout.write("Publish in this order (platform packages first):\n");
  for (const p of platformPackages) process.stdout.write(`  ${p.dir}\n`);
  process.stdout.write(`  ${mainPackage.dir}   (${mainPackage.name})\n`);
}

main();
