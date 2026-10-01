# Derives the manifest of the bundle for one operating system from the
# manifest beside this file, which describes the universal bundle:
#
#   jq --arg platform linux -f mcpb/platform.jq mcpb/manifest.json
#
# $platform is a value Claude Desktop reports as process.platform: darwin,
# win32 or linux. scripts/build-mcpb.sh packs the result into
# gitlab-mcp-server-<os>.mcpb, and `make check-mcpb` validates it.
#
# The universal manifest starts the macOS binary as its base command and
# overrides it for win32 and linux. A bundle for one operating system carries
# only that system's server, so its manifest:
#   - lists only that platform in compatibility.platforms, which is what makes
#     Claude Desktop refuse the bundle on another system with a clear message
#     rather than start a binary that is not there;
#   - promotes that platform's override to the base command and drops every
#     override, since the base command is the only one this bundle can start;
#   - names as server.entry_point the first path that command line names inside
#     the bundle (the binary, or the Linux launcher that /bin/sh runs).
# Everything else, the name above all, stays as it is: the three bundles are
# one extension, so installing one replaces the universal one in place.

if ($platform | IN("darwin", "win32", "linux") | not) then
  error("platform must be darwin, win32 or linux, got \($platform)")
else . end
| .server.mcp_config as $config
| (($config.platform_overrides // {})[$platform] // {}) as $override
| ($config
   | del(.platform_overrides)
   | if $override | has("command") then .command = $override.command else . end
   | if $override | has("args") then .args = $override.args else . end) as $promoted
| ([$promoted.command, ($promoted.args // [])[]]
   | map(select(type == "string" and startswith("${__dirname}/")) | ltrimstr("${__dirname}/"))
   | first) as $entry
| if $entry == null then
    error("the \($platform) command line names no path inside the bundle")
  else . end
| .server.mcp_config = $promoted
| .server.entry_point = $entry
| .compatibility.platforms = [$platform]
