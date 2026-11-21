// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The landing page as markdown, for its twin.
//
// The two landings are one <Home content={en|es} /> tag over copy that lives
// in a typed object (src/data/home.ts) and numbers read from stats.json and
// token-footprint.json, so there is no prose in the page to reduce. This file
// walks the same blocks src/components/home/Home.astro renders, in the same
// order, from the same objects and the same arithmetic, so the twin states
// the figures the landing shows. A block added to Home.astro and not here is
// a block missing from the twin: keep the two in step.
import { inlineHtml } from "./page-markdown.mjs";

/** A table cell: a pipe inside one would split the row. */
const cell = (value) => String(value).replace(/\|/g, "\\|");

/**
 * @param {string} code
 * @param {string} lang
 * @returns {string}
 */
const fence = (code, lang) => ["```" + lang, code.trimEnd(), "```"].join("\n");

/**
 * Renders the landing's body.
 *
 * @param {any} content one locale's object from src/data/home.ts
 * @param {{ stats: any, footprint: any, file: string }} data
 * @returns {string} markdown whose code spans and fences are not yet masked
 */
export function renderHome(content, { stats, footprint, file }) {
	if (!content || typeof content !== "object") {
		throw new Error(`${file}: <Home> without its content object`);
	}
	const html = (value) => inlineHtml(value, file);
	const count = (n) => n.toLocaleString(content.numberLocale);

	// The same figures Home.astro computes, from the same two files.
	const defaultContext =
		footprint.dynamic.tool_schema_tokens + footprint.shared.full;
	const minimalContext =
		footprint.dynamic.tool_schema_tokens + footprint.shared.minimal;
	const readout = [
		[String(stats.dynamic), "dynamic"],
		[count(defaultContext), "context"],
		[count(stats.tools.gitlab_com), "tools"],
		[String(stats.prompts), "prompts"],
		[String(stats.resources), "resources"],
	];
	for (const placeholder of ["{default}", "{minimal}"]) {
		if (!content.contextClaim.includes(placeholder)) {
			throw new Error(`${file}: contextClaim lacks ${placeholder}`);
		}
	}
	const contextClaim = content.contextClaim
		.replaceAll("{default}", count(defaultContext))
		.replaceAll("{minimal}", count(minimalContext));
	// Home.astro joins a range with an en dash; it is spelled by code point
	// here so this source stays free of one.
	const range = (low, high) =>
		`${count(low)}${String.fromCharCode(0x2013)}${count(high)}`;
	const surfaceCounts = {
		dynamic: String(stats.dynamic),
		meta: range(stats.meta.base, stats.meta.gitlab_com),
		individual: range(stats.tools.free, stats.tools.gitlab_com),
	};

	const blocks = [];
	blocks.push(
		`## ${content.statsLabel}`,
		readout
			.map(
				([value, key]) =>
					`- **${value}** [${content.statLabels[key]}](${content.statHrefs[key]})`,
			)
			.join("\n"),
		html(contextClaim),
	);

	blocks.push(
		`## ${content.what.title}`,
		...content.what.body.map(html),
		`### ${content.who.title}`,
		content.who.items.map((item) => `- ${item}`).join("\n"),
		`### ${content.honest.title}`,
		content.honest.items.map((item) => `- ${item}`).join("\n"),
	);

	const [ask, does] = content.proof.promptHeaders;
	blocks.push(
		`## ${content.proof.title}`,
		content.proof.lead,
		`### ${content.proof.installTitle}`,
		content.proof.installNote,
		fence(content.proof.install, "sh"),
		`### ${content.proof.promptsTitle}`,
		content.proof.promptsNote,
		[
			`| ${cell(ask)} | ${cell(does)} |`,
			"| --- | --- |",
			...content.proof.prompts.map(
				([request, action]) =>
					`| ${cell(html(request))} | ${cell(html(action))} |`,
			),
		].join("\n"),
	);

	const [surface, tools, when] = content.surfaces.headers;
	blocks.push(
		`## ${content.surfaces.title}`,
		content.surfaces.lead,
		[
			`| ${cell(surface)} | ${cell(tools)} | ${cell(when)} |`,
			"| --- | --- | --- |",
			...content.surfaces.rows.map((row) => {
				const counted = surfaceCounts[row.id];
				if (counted === undefined) {
					throw new Error(`${file}: no count for the surface ${row.id}`);
				}
				return `| ${cell(`**${row.name}**: ${row.summary}`)} | ${cell(counted)} | ${cell(row.when)} |`;
			}),
		].join("\n"),
		`${content.surfaces.tierNames.join(", ")}. ${content.surfaces.legend}`,
	);

	blocks.push(
		`## ${content.start.title}`,
		content.start.lead,
		content.start.steps
			.map((step, index) => {
				const body = [step.body];
				if (step.code) body.push(fence(step.code, "bash"));
				body.push(`[${step.linkText}](${step.href})`);
				return `${index + 1}. **${step.title}**\n\n${body
					.join("\n\n")
					.split("\n")
					.map((line) => (line ? `   ${line}` : line))
					.join("\n")}`;
			})
			.join("\n\n"),
	);

	return blocks.join("\n\n");
}
