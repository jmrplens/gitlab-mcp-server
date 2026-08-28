// validate-npm.mjs checks the assembled npm distribution before it is
// published. A published npm version is permanent — the registry refuses to
// replace it — so a broken tarball caught here is cheap and caught after
// publish is not.
//
// Two tiers of check:
//   1. Structural, for all seven packages. What actually ships in each tarball:
//      the exact file set, the executable bit on the binary, the binary's magic
//      number for the platform it claims, a size floor, and the package.json
//      os/cpu/name/version. This runs anywhere; it does not execute anything.
//   2. Runtime, for the one platform the validating host can run (linux-x64
//      inside the node:22 container `make validate-npm` uses). It installs the
//      launcher plus that platform package from their tarballs into a throwaway
//      project, confirms npm resolved only the matching package, then drives an
//      MCP initialize handshake over stdio and asserts stdout carries pure
//      JSON-RPC — the property a stray print would silently break.
//
// Usage: node scripts/validate-npm.mjs --packages <dir> --main <dir> --version <x.y.z>

import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { spawn } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Magic numbers by target OS: ELF for linux, Mach-O 64-bit LE for darwin, PE/MZ
// for windows. A binary whose first bytes do not match is one built for the
// wrong platform or a truncated download, which os/cpu gating would not catch.
const MAGIC = {
  linux: [[0x7f, 0x45, 0x4c, 0x46]], // \x7fELF
  darwin: [[0xcf, 0xfa, 0xed, 0xfe], [0xca, 0xfe, 0xba, 0xbe]], // Mach-O thin LE / universal
  win32: [[0x4d, 0x5a]], // MZ
};
const MIN_BINARY_BYTES = 5_000_000;

const PLATFORMS = [
  { key: "linux-x64", os: "linux", cpu: "x64", exe: false },
  { key: "linux-arm64", os: "linux", cpu: "arm64", exe: false },
  { key: "darwin-x64", os: "darwin", cpu: "x64", exe: false },
  { key: "darwin-arm64", os: "darwin", cpu: "arm64", exe: false },
  { key: "win32-x64", os: "win32", cpu: "x64", exe: true },
  { key: "win32-arm64", os: "win32", cpu: "arm64", exe: true },
];

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === "--packages") out.packages = argv[++i];
    else if (argv[i] === "--main") out.main = argv[++i];
    else if (argv[i] === "--version") out.version = argv[++i];
    else throw new Error(`unknown argument: ${argv[i]}`);
  }
  for (const k of ["packages", "main", "version"]) {
    if (!out[k]) throw new Error(`--${k} is required`);
  }
  return out;
}

const failures = [];
function check(cond, msg) {
  if (!cond) failures.push(msg);
  return cond;
}

// packAndList packs a package to tgz and returns { tgz, entries } where each
// entry is { mode, size, name } parsed from `tar -tzv`. Packing (not reading
// the source dir) is the point: it validates the artifact that ships.
function packAndList(dir, destDir) {
  const out = execFileSync("npm", ["pack", "--pack-destination", destDir, "--silent"], {
    cwd: dir,
    encoding: "utf8",
  }).trim();
  const tgz = join(destDir, out.split("\n").pop().trim());
  const listing = execFileSync("tar", ["-tzvf", tgz], { encoding: "utf8" });
  const entries = listing
    .trim()
    .split("\n")
    .map((line) => {
      const parts = line.split(/\s+/);
      // e.g. "-rwxr-xr-x 0/0  57368876 2026-... package/gitlab-mcp-server"
      return { mode: parts[0], size: Number(parts[2]), name: parts[parts.length - 1] };
    });
  return { tgz, entries };
}

function firstBytes(tgz, entryName, n) {
  const buf = execFileSync("tar", ["-xzOf", tgz, entryName], { maxBuffer: 1 << 30 });
  return Array.from(buf.subarray(0, n));
}

function validatePlatform(plat, packagesDir, version, workDir) {
  const label = `@jmrp.io/gitlab-mcp-server-${plat.key}`;
  const dir = join(packagesDir, plat.key);
  const pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));

  check(pkg.name === label, `${plat.key}: name is ${pkg.name}, want ${label}`);
  check(pkg.version === version, `${plat.key}: version ${pkg.version}, want ${version}`);
  check(JSON.stringify(pkg.os) === JSON.stringify([plat.os]), `${plat.key}: os ${JSON.stringify(pkg.os)}, want [${plat.os}]`);
  check(JSON.stringify(pkg.cpu) === JSON.stringify([plat.cpu]), `${plat.key}: cpu ${JSON.stringify(pkg.cpu)}, want [${plat.cpu}]`);

  const binaryName = plat.exe ? "gitlab-mcp-server.exe" : "gitlab-mcp-server";
  const { tgz, entries } = packAndList(dir, workDir);
  const shipped = entries.map((e) => e.name.replace(/^package\//, "")).filter((n) => n && !n.endsWith("/"));
  const want = new Set([binaryName, "package.json", "README.md"]);
  check(
    shipped.length === want.size && shipped.every((n) => want.has(n)),
    `${plat.key}: tarball ships ${JSON.stringify(shipped)}, want ${JSON.stringify([...want])}`,
  );

  const binEntry = entries.find((e) => e.name.endsWith("/" + binaryName));
  if (check(binEntry, `${plat.key}: binary ${binaryName} not in tarball`)) {
    check(binEntry.mode.includes("x"), `${plat.key}: binary is not executable in the tarball (mode ${binEntry.mode})`);
    check(binEntry.size >= MIN_BINARY_BYTES, `${plat.key}: binary is ${binEntry.size} bytes, below the ${MIN_BINARY_BYTES} floor`);
    const magic = firstBytes(tgz, `package/${binaryName}`, 4);
    const ok = MAGIC[plat.os].some((sig) => sig.every((b, i) => magic[i] === b));
    check(ok, `${plat.key}: binary magic ${magic.map((b) => b.toString(16)).join(" ")} is not ${plat.os}`);
  }
}

function validateMain(mainDir, version, workDir) {
  const pkg = JSON.parse(readFileSync(join(mainDir, "package.json"), "utf8"));
  check(pkg.name === "@jmrp.io/gitlab-mcp-server", `main: name is ${pkg.name}`);
  check(pkg.version === version, `main: version ${pkg.version}, want ${version}`);
  check(pkg.bin && pkg.bin["gitlab-mcp-server"] === "cli.js", `main: bin does not point at cli.js`);
  for (const plat of PLATFORMS) {
    const dep = `@jmrp.io/gitlab-mcp-server-${plat.key}`;
    check(pkg.optionalDependencies?.[dep] === version, `main: optionalDependency ${dep} pinned to ${pkg.optionalDependencies?.[dep]}, want ${version}`);
  }
  const { entries } = packAndList(mainDir, workDir);
  const shipped = entries.map((e) => e.name.replace(/^package\//, "")).filter((n) => n && !n.endsWith("/"));
  const want = new Set(["cli.js", "package.json", "README.md"]);
  check(
    shipped.length === want.size && shipped.every((n) => want.has(n)),
    `main: tarball ships ${JSON.stringify(shipped)}, want ${JSON.stringify([...want])}`,
  );
}

// runtimeCheck installs the launcher plus the host-native platform package from
// their tarballs and drives an MCP handshake, asserting stdout is pure
// JSON-RPC. Returns the platform key it exercised, or null if none matched.
async function runtimeCheck(packagesDir, mainDir, version, workDir) {
  const key = `${process.platform}-${process.arch}`;
  const plat = PLATFORMS.find((p) => p.key === key);
  if (!plat) {
    process.stdout.write(`  runtime: host is ${key}, no matching package to execute — structural checks only\n`);
    return null;
  }

  const platTgz = packAndList(join(packagesDir, plat.key), workDir).tgz;
  const mainTgz = packAndList(mainDir, workDir).tgz;

  const proj = mkdtempSync(join(workDir, "proj-"));
  writeFileSync(join(proj, "package.json"), JSON.stringify({ name: "v", version: "1.0.0", private: true }));
  execFileSync("npm", ["install", "--no-audit", "--no-fund", platTgz, mainTgz], { cwd: proj, stdio: "pipe" });

  const installed = readdirSync(join(proj, "node_modules", "@jmrp.io"));
  check(
    installed.includes("gitlab-mcp-server") && installed.includes(`gitlab-mcp-server-${plat.key}`),
    `runtime: node_modules/@jmrp.io holds ${JSON.stringify(installed)}`,
  );
  check(
    !installed.some((n) => n.startsWith("gitlab-mcp-server-") && n !== `gitlab-mcp-server-${plat.key}`),
    `runtime: a non-host platform package was installed: ${JSON.stringify(installed)}`,
  );

  const bin = join(proj, "node_modules", ".bin", "gitlab-mcp-server");
  const version_seen = await handshake(bin);
  check(version_seen === version, `runtime: handshake serverInfo.version ${version_seen}, want ${version}`);
  process.stdout.write(`  runtime: installed + MCP handshake on ${plat.key}, stdout pure JSON-RPC ✓\n`);
  return plat.key;
}

// handshake spawns the launcher, sends an initialize request, and verifies every
// non-empty stdout line is JSON-RPC 2.0. Returns the negotiated server version.
function handshake(bin) {
  return new Promise((resolve) => {
    const env = { ...process.env, GITLAB_URL: "https://gitlab.com", GITLAB_TOKEN: "glpat-validate-invalid" };
    const child = spawn(bin, [], { env });
    let out = "";
    child.stdout.on("data", (d) => (out += d));
    child.stderr.on("data", () => {}); // logs live on stderr; not our concern here
    const init = {
      jsonrpc: "2.0", id: 1, method: "initialize",
      params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "validate", version: "1" } },
    };
    child.stdin.write(JSON.stringify(init) + "\n");
    setTimeout(() => {
      child.kill();
      let version = null;
      for (const line of out.split("\n")) {
        const s = line.trim();
        if (!s) continue;
        let msg;
        try {
          msg = JSON.parse(s);
        } catch {
          failures.push(`runtime: non-JSON on stdout would corrupt the MCP stream: ${JSON.stringify(s.slice(0, 100))}`);
          continue;
        }
        if (msg.jsonrpc !== "2.0") failures.push(`runtime: stdout line is not JSON-RPC 2.0: ${JSON.stringify(s.slice(0, 100))}`);
        if (msg.id === 1 && msg.result) version = msg.result.serverInfo?.version ?? null;
      }
      if (!version) failures.push("runtime: no initialize result on stdout");
      resolve(version);
    }, 4000);
  });
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const workDir = mkdtempSync(join(tmpdir(), "validate-npm-"));
  process.stdout.write(`Validating npm distribution v${args.version}\n`);
  try {
    for (const plat of PLATFORMS) validatePlatform(plat, args.packages, args.version, workDir);
    validateMain(args.main, args.version, workDir);
    process.stdout.write(`  structural: 7 packages checked (files, exec bit, magic, os/cpu, pins)${failures.length ? "" : " ✓"}\n`);
    await runtimeCheck(args.packages, args.main, args.version, workDir);
  } finally {
    rmSync(workDir, { recursive: true, force: true });
  }

  if (failures.length) {
    process.stdout.write(`\nFAILED (${failures.length}):\n`);
    for (const f of failures) process.stdout.write(`  ✗ ${f}\n`);
    process.exit(1);
  }
  process.stdout.write("\nnpm distribution valid ✓\n");
}

main().catch((e) => {
  process.stderr.write(`validate-npm: ${e.stack || e}\n`);
  process.exit(1);
});
