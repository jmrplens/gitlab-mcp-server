// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"slices"
	"strings"
)

// dirname is the placeholder Claude Desktop replaces with the directory an
// extension is unpacked in, written with the separator that makes it the
// leading component of a path inside the bundle.
const dirname = "${__dirname}/"

// desktopPlatforms are the values Claude Desktop reports as
// process.platform, the only keys platform_overrides and
// compatibility.platforms mean anything under.
var desktopPlatforms = []string{"darwin", "win32", "linux"}

// derivePlatformManifest returns the manifest of the bundle for one
// operating system, derived from the manifest of the universal bundle. That
// manifest starts the macOS binary as its base command and overrides it for
// win32 and linux; a bundle for one system carries only that system's
// server, so its manifest
//   - lists only that platform in compatibility.platforms, which makes Claude
//     Desktop refuse it on another system with a clear message rather than
//     start a binary that is not there;
//   - promotes that platform's override to the base command and drops every
//     override, since the base command is the only one the bundle can start;
//   - names as server.entry_point the first path that command line names
//     inside the bundle (the binary, or the Linux launcher /bin/sh runs).
//
// Everything else, the name above all, stays as it is: the bundles are one
// extension, so installing one replaces the universal one in place.
//
// This is what mcpb/platform.jq did, member for member, and it keeps jq's
// reading of the shapes no committed manifest has: a missing or null
// override is no override, an override without command or args keeps the
// base one, and a member set where none was is appended.
func derivePlatformManifest(manifest *jsonValue, platform string) (*jsonValue, error) {
	if !slices.Contains(desktopPlatforms, platform) {
		return nil, fmt.Errorf("platform must be darwin, win32 or linux, got %s", platform)
	}
	config, err := index(manifest, "server", "mcp_config")
	if err != nil {
		return nil, err
	}
	overrides := config.field("platform_overrides")
	if !overrides.truthy() {
		overrides = &jsonValue{kind: jsonObject}
	}
	override, err := index(overrides, platform)
	if err != nil {
		return nil, err
	}
	if !override.truthy() {
		override = &jsonValue{kind: jsonObject}
	}
	if override.kind != jsonObject {
		return nil, fmt.Errorf("cannot check whether %s has a string key", override.typeName())
	}

	promoted := config.clone()
	switch promoted.kind {
	case jsonObject:
		promoted = promoted.without("platform_overrides")
	case jsonNull:
	default:
		return nil, fmt.Errorf("cannot delete field at object index of %s", promoted.typeName())
	}
	// A null config has no overrides, so only an object takes one.
	for _, key := range []string{"command", "args"} {
		if value := override.get(key); value != nil {
			promoted.set(key, value.clone())
		}
	}

	line, err := commandLine(promoted)
	if err != nil {
		return nil, err
	}
	at := slices.IndexFunc(line, func(word *jsonValue) bool {
		return word.kind == jsonString && strings.HasPrefix(word.text, dirname)
	})
	if at < 0 {
		return nil, fmt.Errorf("the %s command line names no path inside the bundle", platform)
	}
	entry := strings.TrimPrefix(line[at].text, dirname)

	derived := manifest.clone()
	for _, step := range []struct {
		path  []string
		value *jsonValue
	}{
		{[]string{"server", "mcp_config"}, promoted},
		{[]string{"server", "entry_point"}, jsonText(entry)},
		{[]string{"compatibility", "platforms"}, jsonList(jsonText(platform))},
	} {
		if derived, err = setPath(derived, step.path, step.value); err != nil {
			return nil, err
		}
	}
	return derived, nil
}

// stampVersion returns a copy of a manifest, which is an object, with its
// version set, as jq's `.version = $v` sets it: in its place, or last.
func stampVersion(manifest *jsonValue, version string) *jsonValue {
	stamped := manifest.clone()
	stamped.set("version", jsonText(version))
	return stamped
}

// commandLine is the command of a mcp_config, an object or null, followed by
// its args, each as written: the words a host would start, of any kind.
// Args that are null or false are none; an object's args are its values, as
// jq iterates them.
func commandLine(config *jsonValue) ([]*jsonValue, error) {
	line := []*jsonValue{config.field("command")}
	args := config.field("args")
	if !args.truthy() {
		return line, nil
	}
	words, err := iterate(args)
	if err != nil {
		return nil, err
	}
	return append(line, words...), nil
}

// field is jq's .key on a value index already accepted: an object's member
// or null.
func (v *jsonValue) field(key string) *jsonValue {
	if v.kind == jsonObject {
		if member := v.get(key); member != nil {
			return member
		}
	}
	return &jsonValue{}
}

// index is jq's .a.b...: an object's member, null for a member it lacks or
// for anything indexed on null, and an error for any other kind.
func index(v *jsonValue, path ...string) (*jsonValue, error) {
	for _, key := range path {
		switch v.kind {
		case jsonNull:
			return v, nil
		case jsonObject:
			v = v.field(key)
		default:
			return nil, fmt.Errorf("cannot index %s with %q", v.typeName(), key)
		}
	}
	return v, nil
}

// iterate is jq's .[]: an array's items or an object's values, and an error
// for any other kind.
func iterate(v *jsonValue) ([]*jsonValue, error) {
	switch v.kind {
	case jsonArray:
		return v.items, nil
	case jsonObject:
		values := make([]*jsonValue, 0, len(v.members))
		for _, member := range v.members {
			values = append(values, member.value)
		}
		return values, nil
	}
	return nil, fmt.Errorf("cannot iterate over %s", v.typeName())
}

// setPath is jq's .a.b = value: every object on the way is created where
// the path finds null and kept in its place where it finds one, and any
// other kind on the way is an error.
func setPath(root *jsonValue, path []string, value *jsonValue) (*jsonValue, error) {
	if len(path) == 0 {
		return value, nil
	}
	switch root.kind {
	case jsonNull:
		root = &jsonValue{kind: jsonObject}
	case jsonObject:
	default:
		return nil, fmt.Errorf("cannot index %s with %q", root.typeName(), path[0])
	}
	child, err := setPath(root.field(path[0]), path[1:], value)
	if err != nil {
		return nil, err
	}
	root.set(path[0], child)
	return root, nil
}
