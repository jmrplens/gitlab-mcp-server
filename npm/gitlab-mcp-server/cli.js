#!/usr/bin/env node
// cli.js resolves the prebuilt gitlab-mcp-server binary for the current
// platform and hands control to it. The binary ships inside a per-platform
// optional dependency (see the package's optionalDependencies); npm installs
// only the one whose os/cpu match, and this launcher runs it.
//
// It is a thin shim on purpose. The server speaks MCP over stdio, so stdio is
// inherited untouched; argv is forwarded verbatim; the child's exit code and
// terminating signal are mirrored so `npx` callers and process supervisors see
// the real outcome.
//
// The one thing a shim cannot leave to the operating system is stopping the
// server. A client or a supervisor signals the process it started, which is
// this launcher, or npm under npx, and a signal sent to one process never
// reaches its children. So the launcher waits for the server in the event loop
// rather than blocking in spawnSync, where no handler of its own could run,
// and on POSIX:
//
//   - SIGTERM, SIGINT and SIGHUP are passed on to the server, which then runs
//     its own shutdown (and in HTTP mode its drain) instead of being orphaned;
//   - under npm (npx, or an npm script) it also watches its parent. A signal
//     sent to npx stops at npm's shell: npm passes it to the `sh -c` it runs
//     this launcher through, the shell dies, and the launcher is reparented
//     without ever being signalled. Its parent changing is how it learns that,
//     and it then sends the server SIGTERM. Started any other way, the parent
//     is the client or a shell, and a server deliberately left running after
//     it (an HTTP server put in the background) is not stopped.
//
// On Windows neither applies, and the launcher does what it always did. There
// are no POSIX signals there: Ctrl+C and closing the console reach every
// process attached to the console, the server included, which shuts down on
// its own, whereas child.kill would end it outright before it could; and a
// process whose parent exits keeps that parent's id, so there is no change to
// watch.
"use strict";

const { spawn } = require("node:child_process");
const { writeSync } = require("node:fs");
const os = require("node:os");

// The signals passed on to the server, which decides what each does: SIGTERM
// and SIGINT start its shutdown, and SIGHUP ends it as it would end the binary
// run directly. A Ctrl+C in a terminal already reaches the server through the
// process group, so it sees that SIGINT twice; its handler starts the shutdown
// on the first and ignores the rest.
const FORWARDED = process.platform === "win32" ? [] : ["SIGTERM", "SIGINT", "SIGHUP"];

// How often the parent is checked under npm. A second is the longest the
// server outlives npm's shell, and the check costs nothing: the timer is
// unref'd, so it never keeps the launcher alive by itself.
const PARENT_POLL_MS = 1000;

// platformKey maps Node's platform/arch names to the per-platform package
// suffix. The suffixes follow Node's vocabulary (win32, x64), not Go's
// (windows, amd64); the release generator translates when it builds each
// package, so the two never have to agree at runtime.
function platformKey() {
  const platform = process.platform;
  const arch = process.arch;
  const supported = {
    "linux-x64": true,
    "linux-arm64": true,
    "darwin-x64": true,
    "darwin-arm64": true,
    "win32-x64": true,
    "win32-arm64": true,
  };
  const key = `${platform}-${arch}`;
  return supported[key] ? key : null;
}

function binaryName() {
  return process.platform === "win32"
    ? "gitlab-mcp-server.exe"
    : "gitlab-mcp-server";
}

// resolveBinary finds the binary inside the matching per-platform package.
// require.resolve walks the same node_modules the launcher was loaded from, so
// it finds the dependency whether the install is flat, nested, hoisted, or run
// through npx's throwaway prefix.
function resolveBinary(key) {
  const pkg = `@jmrp.io/gitlab-mcp-server-${key}`;
  try {
    return require.resolve(`${pkg}/${binaryName()}`);
  } catch {
    return null;
  }
}

function fail(message) {
  process.stderr.write(`gitlab-mcp-server: ${message}\n`);
  process.exit(1);
}

const key = platformKey();
if (!key) {
  fail(
    `unsupported platform ${process.platform}/${process.arch}. ` +
      "Prebuilt binaries exist for linux, macOS and Windows on x64 and arm64; " +
      "for anything else, build from source or use a released binary directly " +
      "(https://github.com/jmrplens/gitlab-mcp-server/releases).",
  );
}

const binary = resolveBinary(key);
if (!binary) {
  // The platform is supported but its package is absent. The usual cause is an
  // install that skipped optional dependencies (npm install --no-optional, or
  // a lockfile pinned on a different OS), not a broken release.
  fail(
    `the @jmrp.io/gitlab-mcp-server-${key} package is not installed. ` +
      "It is an optional dependency that carries the binary for this platform; " +
      "reinstall without --no-optional, or delete node_modules and the lockfile " +
      "and install again. On musl systems (Alpine) the linux packages are " +
      "skipped on purpose: the prebuilt binaries need the glibc dynamic loader — " +
      "use the Docker image (musl-based) or build from source instead " +
      "(https://github.com/jmrplens/gitlab-mcp-server).",
  );
}

const env = { ...process.env };

// The handlers are installed before the server starts, so a signal that
// arrives while it is starting is passed on rather than ending the launcher
// with the server left behind. Node runs a handler on a later turn of the
// event loop, by which time `child` is set.
let child = null;
for (const signal of FORWARDED) {
  process.on(signal, () => child.kill(signal));
}

child = spawn(binary, process.argv.slice(2), {
  stdio: "inherit",
  env,
});

// npm sets npm_lifecycle_event for whatever it runs, npx included ("npx"),
// and runs it through a shell whose death is the only sign the launcher gets
// that npm was told to stop. The parent is read once, now, and polled.
if (FORWARDED.length > 0 && process.env.npm_lifecycle_event) {
  const parent = process.ppid;
  const watch = setInterval(() => {
    if (process.ppid === parent) return;
    clearInterval(watch);
    child.kill("SIGTERM");
  }, PARENT_POLL_MS);
  watch.unref();
}

child.on("error", (err) => {
  // Emitted when the binary could not be started, and, rarely, when a signal
  // could not be delivered to it. Only the first ends the launcher here: a
  // server that is running still ends with an "exit" event, mirrored below.
  if (child.pid === undefined) fail(`failed to start the binary: ${err.message}`);
  // Written to the descriptor directly: process.stderr would switch the pipe
  // the server shares into non-blocking mode under its feet.
  writeSync(2, `gitlab-mcp-server: ${err.message}\n`);
});

child.on("exit", (code, signal) => {
  // A child killed by a signal reports null status and a signal name. Re-raise
  // it so the launcher dies the same way rather than masking a SIGINT as exit
  // 0. The handler that would pass it on goes first, or the launcher would
  // catch its own signal and send it to a server that is already gone.
  if (signal) {
    process.removeAllListeners(signal);
    process.kill(process.pid, signal);
    // If the re-raise did not terminate us (signal ignored, or no default
    // disposition), exit with the conventional 128+signal code so the caller
    // still sees the child's terminating signal rather than a bare failure.
    const signum = os.constants.signals[signal];
    process.exit(signum ? 128 + signum : 1);
  }
  process.exit(code === null ? 1 : code);
});
