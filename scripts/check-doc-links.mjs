#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

const repoRoot = execFileSync("git", ["rev-parse", "--show-toplevel"], {
	encoding: "utf8",
}).trim();
const trackedDocs = [
	...new Set(
		execFileSync(
			"git",
			["ls-files", "*.md", "*.mdx", ":(glob)**/*.md", ":(glob)**/*.mdx"],
			{
				cwd: repoRoot,
				encoding: "utf8",
			},
		)
			.trim()
			.split("\n")
			.filter(Boolean),
	),
]
	.filter((file) => !file.startsWith("plan/"))
	.filter((file) => !file.startsWith(".github/skills/"));

const issues = [];

for (const file of trackedDocs) {
	const absoluteFile = path.join(repoRoot, file);
	const content = readFileSync(absoluteFile, "utf8");
	let inFence = false;
	const lines = content.split("\n");

	lines.forEach((line, index) => {
		if (/^\s*(```|~~~)/.test(line)) {
			inFence = !inFence;
			return;
		}
		if (inFence) {
			return;
		}

		for (const target of inlineLinks(line)) {
			checkTarget(file, index + 1, target);
		}

		const reference = line.match(/^\s*\[(?!\^)[^\]]+\]:\s*(<[^>]+>|\S+)/);
		if (reference) {
			checkTarget(file, index + 1, reference[1]);
		}
	});
}

if (issues.length > 0) {
	console.error("Broken local documentation links:");
	for (const issue of issues) {
		console.error(`- ${issue.file}:${issue.line} -> ${issue.target}`);
	}
	process.exit(1);
}

console.log(
	`Checked ${trackedDocs.length} Markdown/MDX files; local links are valid.`,
);

function inlineLinks(line) {
	const targets = [];
	const pattern = /!?\[[^\]]*\]\(\s*(<[^>]+>|[^)\s]+)(?:\s+"[^"]*")?\s*\)/g;
	let match;
	while ((match = pattern.exec(line)) !== null) {
		targets.push(match[1]);
	}
	return targets;
}

function checkTarget(file, line, rawTarget) {
	const target = normalizeTarget(rawTarget);
	if (shouldSkipTarget(target)) {
		return;
	}

	const withoutFragment = target.replace(/[?#].*$/, "");
	if (withoutFragment === "") {
		return;
	}

	const decoded = safeDecodeURIComponent(withoutFragment);
	const basePath = path.resolve(
		path.dirname(path.join(repoRoot, file)),
		decoded,
	);
	const candidates = [
		basePath,
		`${basePath}.md`,
		`${basePath}.mdx`,
		path.join(basePath, "README.md"),
		path.join(basePath, "index.md"),
		path.join(basePath, "index.mdx"),
	];

	if (!candidates.some((candidate) => existsSync(candidate))) {
		issues.push({ file, line, target });
	}
}

function normalizeTarget(rawTarget) {
	return rawTarget.trim().replace(/^<|>$/g, "");
}

function shouldSkipTarget(target) {
	if (target === "") {
		return true;
	}
	if (/^(?:[a-z][a-z0-9+.-]*:|#|\/)/i.test(target)) {
		return true;
	}
	if (/^(?:url|link|path|file|filename|directory|fn)$/i.test(target)) {
		return true;
	}
	if (/^[<{].*[>}]$/.test(target)) {
		return true;
	}
	return false;
}

function safeDecodeURIComponent(value) {
	try {
		return decodeURIComponent(value);
	} catch {
		return value;
	}
}
