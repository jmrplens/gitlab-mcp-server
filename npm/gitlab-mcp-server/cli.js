#!/usr/bin/env node
// cli.js resolves the prebuilt gitlab-mcp-server binary for the current
// platform and hands control to it. The binary ships inside a per-platform
// optional dependency (see the package's optionalDependencies); npm installs
// only the one whose os/cpu match, and this launcher execs it.
//
// It is a thin shim on purpose. The server speaks MCP over stdio, so stdio is
// inherited untouched; argv is forwarded verbatim; the child's exit code and
// terminating signal are mirrored so `npx` callers and process supervisors see
// the real outcome.
"use strict";

const { spawnSync } = require("node:child_process");
const os = require("node:os");

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
      "and install again.",
  );
}

// Self-update must stay off: npm owns this binary, and letting it replace
// itself on disk would desynchronize what npm believes is installed — the same
// reason the Homebrew formula and the Claude Desktop manifest pin it off. An
// explicit AUTO_UPDATE from the caller still wins.
const env = { ...process.env };
if (env.AUTO_UPDATE === undefined) {
  env.AUTO_UPDATE = "false";
}

const result = spawnSync(binary, process.argv.slice(2), {
  stdio: "inherit",
  env,
});

if (result.error) {
  fail(`failed to start the binary: ${result.error.message}`);
}
// A child killed by a signal reports null status and a signal name. Re-raise it
// so the launcher dies the same way rather than masking a SIGINT as exit 0.
if (result.signal) {
  process.kill(process.pid, result.signal);
  // If the re-raise did not terminate us (signal ignored, or no default
  // disposition), exit with the conventional 128+signal code so the caller
  // still sees the child's terminating signal rather than a bare failure.
  const signum = os.constants.signals[result.signal];
  process.exit(signum ? 128 + signum : 1);
}
process.exit(result.status === null ? 1 : result.status);
