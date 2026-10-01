// validate-npm.mjs checks the assembled npm distribution before it is
// published. A published npm version is permanent — the registry refuses to
// replace it — so a broken tarball caught here is cheap and caught after
// publish is not.
//
// Two tiers of check:
//   1. Structural, for all seven packages. What actually ships in each tarball:
//      the exact file set, the executable bit on the binary, the binary's magic
//      number for the platform it claims, a size floor, and the package.json
//      os/cpu/name/version. Every tarball carries the repository's LICENSE,
//      byte for byte, and THIRD_PARTY_NOTICES, opening with the generator's
//      header. This runs anywhere; it does not execute anything.
//   1b. Provenance: every packed binary is compared against the digest
//      build-npm.mjs recorded when it verified that binary against the
//      release's signed checksums.txt. Without this, the checks below are
//      shape checks only — a random file with the right first four bytes and
//      the right size passes all of them, and five of the six platforms are
//      never executed by anything. The notices every tarball carries are
//      compared the same way, against the digest recorded for them.
//   2. Runtime, for the one platform the validating host can run (linux-x64
//      inside the node:22 container `make validate-npm` uses). It installs the
//      launcher plus that platform package from their tarballs into a throwaway
//      project, confirms npm resolved only the matching package, then drives an
//      MCP initialize handshake over stdio and asserts stdout carries pure
//      JSON-RPC — the property a stray print would silently break.
//   2b. Stopping, on a Linux host. The installed package is started the way
//      a client configured with npx starts it, twice: in stdio mode with its
//      stdin held open, and with --http on a loopback port. Each time SIGTERM
//      goes to the npx process alone, as a supervisor sends it, and the server
//      process must be gone within STOP_DEADLINE_MS (in HTTP mode its port
//      closed too). npm hands the signal to the `sh -c` it runs the launcher
//      through, the shell dies, and only the launcher's own parent watch can
//      tell the server; up to 3.1.0 the launcher had none, and an HTTP server
//      started this way kept its port and answered /health after the npx
//      process was gone. The server process is found under /proc, which is
//      also how the check knows it is the binary this project installed and
//      not a copy npx fetched from the registry.
//
// Usage: node scripts/validate-npm.mjs --packages <dir> --main <dir> --version <x.y.z> [--no-install]
//
// --no-install runs tier 1 alone: nothing is installed or executed, which is
// what a test of the structural checks over stand-in binaries needs. The
// release never passes it.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  closeSync,
  existsSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readdirSync,
  readlinkSync,
  realpathSync,
  rmSync,
  writeFileSync,
  writeSync,
} from "node:fs";
import { spawn } from "node:child_process";
import { get as httpGet } from "node:http";
import { connect, createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

// The licence texts every tarball carries, by name, and the repository file
// each must equal byte for byte, so a package can never carry a licence the
// source does not.
const LICENSE_FILES = [{ name: "LICENSE", src: join(repoRoot, "LICENSE") }];

// The third-party notices every tarball carries beside the licence. They are
// generated with the release rather than kept in the repository, so they are
// held to the generator's header and to the digest build-npm.mjs recorded
// after checking them against the release's signed checksums.txt.
const NOTICES = "THIRD_PARTY_NOTICES";
const NOTICES_HEADER = "Third-party notices for gitlab-mcp-server\n";
const SHIPPED_TEXTS = [...LICENSE_FILES.map((f) => f.name), NOTICES];

// Magic numbers by target OS: ELF for linux, Mach-O 64-bit LE for darwin, PE/MZ
// for windows. A binary whose first bytes do not match is one built for the
// wrong platform or a truncated download, which os/cpu gating would not catch.
const MAGIC = {
  linux: [[0x7f, 0x45, 0x4c, 0x46]], // \x7fELF
  darwin: [[0xcf, 0xfa, 0xed, 0xfe], [0xca, 0xfe, 0xba, 0xbe]], // Mach-O thin LE / universal
  win32: [[0x4d, 0x5a]], // MZ
};
const MIN_BINARY_BYTES = 5_000_000;

// How long the server may outlive a SIGTERM to the npx process that started
// it. The launcher checks its parent once a second and the server's own
// shutdown takes well under one, so ten seconds fails only a server nothing
// stopped.
const STOP_DEADLINE_MS = 10_000;
// How long npx may take to install nothing and start the server, which is
// longer than it looks on a cold runner.
const START_DEADLINE_MS = 30_000;

const PLATFORMS = [
  { key: "linux-x64", os: "linux", cpu: "x64", libc: "glibc", exe: false },
  { key: "linux-arm64", os: "linux", cpu: "arm64", libc: "glibc", exe: false },
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
    else if (argv[i] === "--no-install") out.noInstall = true;
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

function entryBytes(tgz, entryName) {
  return execFileSync("tar", ["-xzOf", tgz, entryName], { maxBuffer: 1 << 30 });
}

// checkLicenses holds each licence text a tarball ships to the repository's
// own file. The file set check above already requires each to be present.
function checkLicenses(label, tgz, shipped, verified) {
  for (const file of LICENSE_FILES) {
    if (!shipped.includes(file.name)) continue;
    const packed = entryBytes(tgz, `package/${file.name}`);
    check(packed.equals(readFileSync(file.src)), `${label}: ${file.name} in the tarball is not the repository's ${file.name}`);
  }
  if (!shipped.includes(NOTICES)) return;
  const notices = entryBytes(tgz, `package/${NOTICES}`);
  check(
    notices.subarray(0, NOTICES_HEADER.length).toString("utf8") === NOTICES_HEADER,
    `${label}: ${NOTICES} in the tarball does not open with the generator's header`,
  );
  if (verified) {
    const got = createHash("sha256").update(notices).digest("hex");
    check(
      verified.notices === got,
      `${label}: ${NOTICES} in the tarball is sha256 ${got}, but the release's signed checksums.txt named ${verified.notices}`,
    );
  }
}

// readVerifiedBinaries loads the digests build-npm.mjs recorded after checking
// each binary against the release's signed checksums.txt. Absent means the
// packages were assembled by something that skipped that check.
function readVerifiedBinaries(packagesDir) {
  const path = join(packagesDir, "verified-binaries.json");
  if (!existsSync(path)) return null;
  return JSON.parse(readFileSync(path, "utf8"));
}

function validatePlatform(plat, packagesDir, version, workDir, verified) {
  const label = `@jmrp.io/gitlab-mcp-server-${plat.key}`;
  const dir = join(packagesDir, plat.key);
  const pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));

  check(pkg.name === label, `${plat.key}: name is ${pkg.name}, want ${label}`);
  check(pkg.version === version, `${plat.key}: version ${pkg.version}, want ${version}`);
  check(JSON.stringify(pkg.os) === JSON.stringify([plat.os]), `${plat.key}: os ${JSON.stringify(pkg.os)}, want [${plat.os}]`);
  check(JSON.stringify(pkg.cpu) === JSON.stringify([plat.cpu]), `${plat.key}: cpu ${JSON.stringify(pkg.cpu)}, want [${plat.cpu}]`);
  const wantLibc = plat.libc ? JSON.stringify([plat.libc]) : undefined;
  check(JSON.stringify(pkg.libc) === wantLibc, `${plat.key}: libc ${JSON.stringify(pkg.libc)}, want ${wantLibc ?? "absent"}`);

  const binaryName = plat.exe ? "gitlab-mcp-server.exe" : "gitlab-mcp-server";
  const { tgz, entries } = packAndList(dir, workDir);
  const shipped = entries.map((e) => e.name.replace(/^package\//, "")).filter((n) => n && !n.endsWith("/"));
  const want = new Set([binaryName, "package.json", "README.md", ...SHIPPED_TEXTS]);
  check(
    shipped.length === want.size && shipped.every((n) => want.has(n)),
    `${plat.key}: tarball ships ${JSON.stringify(shipped)}, want ${JSON.stringify([...want])}`,
  );
  checkLicenses(plat.key, tgz, shipped, verified);

  const binEntry = entries.find((e) => e.name.endsWith("/" + binaryName));
  if (check(binEntry, `${plat.key}: binary ${binaryName} not in tarball`)) {
    check(binEntry.mode.includes("x"), `${plat.key}: binary is not executable in the tarball (mode ${binEntry.mode})`);
    check(binEntry.size >= MIN_BINARY_BYTES, `${plat.key}: binary is ${binEntry.size} bytes, below the ${MIN_BINARY_BYTES} floor`);
    const bytes = entryBytes(tgz, `package/${binaryName}`);
    const magic = Array.from(bytes.subarray(0, 4));
    const ok = MAGIC[plat.os].some((sig) => sig.every((b, i) => magic[i] === b));
    check(ok, `${plat.key}: binary magic ${magic.map((b) => b.toString(16)).join(" ")} is not ${plat.os}`);

    if (verified) {
      const want = verified.binaries?.[plat.key];
      const got = createHash("sha256").update(bytes).digest("hex");
      check(
        want === got,
        `${plat.key}: the packed binary is sha256 ${got}, but the release's signed checksums.txt named ${want}`,
      );
    }
  }
}

function validateMain(mainDir, version, workDir, verified) {
  const pkg = JSON.parse(readFileSync(join(mainDir, "package.json"), "utf8"));
  check(pkg.name === "@jmrp.io/gitlab-mcp-server", `main: name is ${pkg.name}`);
  check(pkg.version === version, `main: version ${pkg.version}, want ${version}`);
  check(pkg.bin && pkg.bin["gitlab-mcp-server"] === "cli.js", `main: bin does not point at cli.js`);
  for (const plat of PLATFORMS) {
    const dep = `@jmrp.io/gitlab-mcp-server-${plat.key}`;
    check(pkg.optionalDependencies?.[dep] === version, `main: optionalDependency ${dep} pinned to ${pkg.optionalDependencies?.[dep]}, want ${version}`);
  }
  const { tgz, entries } = packAndList(mainDir, workDir);
  const shipped = entries.map((e) => e.name.replace(/^package\//, "")).filter((n) => n && !n.endsWith("/"));
  const want = new Set(["cli.js", "package.json", "README.md", ...SHIPPED_TEXTS]);
  check(
    shipped.length === want.size && shipped.every((n) => want.has(n)),
    `main: tarball ships ${JSON.stringify(shipped)}, want ${JSON.stringify([...want])}`,
  );
  checkLicenses("main", tgz, shipped, verified);
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

  if (process.platform === "linux") {
    const server = realpathSync(join(proj, "node_modules", "@jmrp.io", `gitlab-mcp-server-${plat.key}`, "gitlab-mcp-server"));
    await stopUnderNpx("stdio", proj, server, version, workDir);
    await stopUnderNpx("http", proj, server, version, workDir);
  } else {
    process.stdout.write(`  stop: skipped on ${process.platform}, which has no /proc to find the server process in\n`);
  }
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

// procStat reads the state and parent of a process from /proc, or null when
// there is no such process. The command name sits in parentheses and may
// itself hold spaces and parentheses, so the fields are read after the last
// closing one.
function procStat(pid) {
  let text;
  try {
    text = readFileSync(`/proc/${pid}/stat`, "utf8");
  } catch {
    return null;
  }
  const fields = text.slice(text.lastIndexOf(")") + 2).split(" ");
  return { state: fields[0], ppid: Number(fields[1]) };
}

// alive is false for a process that is gone and for one that has exited and
// waits only to be reaped, which is all an orphan may be left as in a
// container whose first process does not reap.
function alive(pid) {
  const stat = procStat(pid);
  return stat !== null && stat.state !== "Z";
}

// serverUnder finds the process running exe whose chain of parents reaches
// ancestor, or null. Matching on the executable is what tells the binary this
// project installed from one npx might have fetched from the registry.
function serverUnder(ancestor, exe) {
  for (const entry of readdirSync("/proc")) {
    if (!/^\d+$/.test(entry)) continue;
    let target;
    try {
      target = readlinkSync(`/proc/${entry}/exe`);
    } catch {
      continue;
    }
    if (target !== exe) continue;
    for (let pid = Number(entry), hops = 0; pid > 1 && hops < 16; hops += 1) {
      const stat = procStat(pid);
      if (stat === null) break;
      if (stat.ppid === ancestor) return Number(entry);
      pid = stat.ppid;
    }
  }
  return null;
}

async function freePort() {
  const probe = createServer();
  await new Promise((resolve) => probe.listen(0, "127.0.0.1", resolve));
  const { port } = probe.address();
  await new Promise((resolve) => probe.close(resolve));
  return port;
}

// healthStatus answers the status /health gave, or null when nothing
// answered. agent:false closes the connection with the response, so the check
// leaves no idle connection behind for the shutdown to wait on.
function healthStatus(port) {
  return new Promise((resolve) => {
    const req = httpGet({ host: "127.0.0.1", port, path: "/health", agent: false }, (res) => {
      res.resume();
      resolve(res.statusCode);
    });
    req.on("error", () => resolve(null));
    req.setTimeout(2000, () => req.destroy());
  });
}

function portRefuses(port) {
  return new Promise((resolve) => {
    const socket = connect({ host: "127.0.0.1", port });
    socket.on("connect", () => {
      socket.destroy();
      resolve(false);
    });
    socket.on("error", () => resolve(true));
  });
}

// until polls cond every interval until it is true or the deadline passes,
// and answers the elapsed milliseconds or null.
async function until(cond, deadlineMs, intervalMs = 100) {
  const start = Date.now();
  while (Date.now() - start < deadlineMs) {
    if (await cond()) return Date.now() - start;
    await sleep(intervalMs);
  }
  return null;
}

// stopUnderNpx starts the installed package through npx in one transport,
// waits until the server answers, sends SIGTERM to the npx process alone and
// requires the server to be gone within STOP_DEADLINE_MS. --offline keeps npx
// from reaching the registry, where a published version of the same name
// would otherwise be a candidate.
//
// The server's stdio is a FIFO and two files rather than Node's pipes, which
// are socket pairs: npm shuts its stdio sockets down as it exits, and since
// every process down the chain shares them, the server then reads an end of
// input and dies writing to a closed stderr. That ends the server whatever the
// launcher does, so a check over Node's pipes passes a launcher that forwards
// nothing. A client holding ordinary pipes, or a supervisor logging to a file,
// sees no such end, and that is the case the check stands for. The FIFO is
// opened for reading and writing, so the server never sees the end of its
// input while the check runs, as with a client that has not closed it.
async function stopUnderNpx(mode, proj, exe, version, workDir) {
  const label = `stop (${mode})`;
  const env = { ...process.env, GITLAB_URL: "https://gitlab.com", GITLAB_TOKEN: "glpat-validate-invalid" };
  const args = ["--offline", "--yes", `@jmrp.io/gitlab-mcp-server@${version}`];
  const port = mode === "http" ? await freePort() : 0;
  if (mode === "http") args.push("--http", "--http-addr", `127.0.0.1:${port}`, "--allow-any-gitlab-url");
  const dir = mkdtempSync(join(workDir, `stop-${mode}-`));
  const outPath = join(dir, "stdout");
  const errPath = join(dir, "stderr");
  let stdin = "ignore";
  if (mode === "stdio") {
    execFileSync("mkfifo", [join(dir, "stdin")]);
    stdin = openSync(join(dir, "stdin"), "r+");
  }
  const outFd = openSync(outPath, "w");
  const errFd = openSync(errPath, "w");
  const npx = spawn("npx", args, { cwd: proj, env, stdio: [stdin, outFd, errFd] });
  closeSync(outFd);
  closeSync(errFd);
  const out = () => readFileSync(outPath, "utf8");
  const err = () => readFileSync(errPath, "utf8").slice(-2000);
  const npxExited = new Promise((resolve) => {
    npx.on("exit", resolve);
    npx.on("error", (e) => {
      failures.push(`${label}: npx could not be started: ${e.message}`);
      resolve();
    });
  });

  let server = null;
  let launcher = null;
  try {
    if (mode === "stdio") {
      const init = {
        jsonrpc: "2.0", id: 1, method: "initialize",
        params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "validate", version: "1" } },
      };
      writeSync(stdin, JSON.stringify(init) + "\n");
    }
    const ready = await until(async () => {
      server ??= serverUnder(npx.pid, exe);
      if (server === null) return false;
      if (mode === "http") return (await healthStatus(port)) === 200;
      return out().split("\n").some((line) => line.includes('"id":1') && line.includes(`"version":"${version}"`));
    }, START_DEADLINE_MS, 200);
    if (!check(ready !== null, `${label}: through npx the server ${server === null ? "never started" : "never answered"} within ${START_DEADLINE_MS} ms; npx stderr: ${err().trim()}`)) return;
    launcher = procStat(server)?.ppid ?? null;

    npx.kill("SIGTERM");
    const gone = await until(() => !alive(server), STOP_DEADLINE_MS);
    if (!check(gone !== null, `${label}: the server outlived a SIGTERM to npx by ${STOP_DEADLINE_MS} ms (pid ${server}, state ${procStat(server)?.state})`)) return;
    if (mode === "http") {
      check(await portRefuses(port), `${label}: port ${port} still accepts connections after the server exited`);
    }
    process.stdout.write(`  ${label}: SIGTERM to npx ended the server in ${gone} ms ✓\n`);
  } finally {
    // Whatever a failed check left running is ended here, so the validator
    // never leaves a server holding a port or a token behind it.
    for (const pid of [server, launcher]) {
      if (pid === null || !alive(pid)) continue;
      try {
        process.kill(pid, "SIGKILL");
      } catch {
        // Gone between the look and the kill, which is the outcome wanted.
      }
    }
    if (npx.exitCode === null && npx.signalCode === null) npx.kill("SIGKILL");
    await npxExited;
    if (typeof stdin === "number") closeSync(stdin);
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const workDir = mkdtempSync(join(tmpdir(), "validate-npm-"));
  process.stdout.write(`Validating npm distribution v${args.version}\n`);
  const verified = readVerifiedBinaries(args.packages);
  check(
    verified !== null,
    "packages were assembled without verified-binaries.json — build-npm.mjs did not check them against the release's checksums.txt",
  );
  check(
    verified === null || verified.verified === true,
    "verified-binaries.json says the binaries were packaged with --allow-unverified",
  );
  check(
    verified === null || verified.version === args.version,
    `verified-binaries.json records version ${verified?.version}, but this is v${args.version}`,
  );

  try {
    for (const plat of PLATFORMS) validatePlatform(plat, args.packages, args.version, workDir, verified);
    validateMain(args.main, args.version, workDir, verified);
    process.stdout.write(`  structural: 7 packages checked (files, licence, notices, exec bit, magic, sha256 vs checksums.txt, os/cpu, pins)${failures.length ? "" : " ✓"}\n`);
    if (args.noInstall) process.stdout.write("  runtime: skipped (--no-install)\n");
    else await runtimeCheck(args.packages, args.main, args.version, workDir);
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
