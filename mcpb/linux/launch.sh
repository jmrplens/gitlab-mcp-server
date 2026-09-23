#!/bin/sh
# Linux entry point of the Claude Desktop extension (gitlab-mcp-server.mcpb).
#
# The manifest's platform_overrides are chosen by operating system only, and no
# manifest version can choose by CPU architecture, so the bundle carries both
# Linux release binaries beside this script and the linux override runs it as
#
#   /bin/sh ${__dirname}/server/linux/launch.sh
#
# Running it through /bin/sh means it needs only read permission. The binary it
# starts needs the execute bit, which the bundle records; a host that extracts
# the archive without it gets the bit back from the chmod below.
#
# Three rules follow from how Claude Desktop runs the server:
#   - Nothing is written to stdout, ever. Desktop reads the server's stdout as
#     JSON-RPC, so every diagnostic goes to stderr, which lands in its MCP log.
#   - Paths come from $0, never from the working directory, which is whatever
#     Desktop's own is. The extension directory contains a space
#     (~/.config/Claude/Claude Extensions/...), so every path is quoted.
#   - The binary replaces this shell through exec. Desktop signals only the PID
#     it spawned when it stops a server, so a shell left in between would take
#     the signals and leave the server running.

machine=$(uname -m)
case "$machine" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *)
    printf 'gitlab-mcp-server: no Linux binary for machine type "%s"; the extension carries amd64 (x86_64) and arm64 (aarch64) only\n' "$machine" >&2
    exit 1
    ;;
esac

case "$0" in
  */*) dir=${0%/*} ;;
  *) dir=. ;;
esac
bin="$dir/gitlab-mcp-server-linux-$arch"

if [ ! -f "$bin" ]; then
  printf 'gitlab-mcp-server: %s is missing from the extension; reinstall it\n' "$bin" >&2
  exit 127
fi

if [ ! -x "$bin" ]; then
  if ! chmod u+x "$bin"; then
    printf 'gitlab-mcp-server: %s is not executable and chmod failed\n' "$bin" >&2
    exit 126
  fi
  if [ ! -x "$bin" ]; then
    printf 'gitlab-mcp-server: %s is still not executable after chmod; is the filesystem mounted noexec?\n' "$bin" >&2
    exit 126
  fi
fi

exec "$bin" "$@"
