// Contrast audit for the site palette.
//
// Parses src/styles/theme.css, resolves the light and the dark theme exactly as
// the cascade does, and reports the WCAG 2.1 contrast ratio of every pair the
// site actually renders.
//
// Pairs are either enforced (text 4.5:1, meaningful non-text 3:1, per WCAG
// 1.4.3 / 1.4.11) or informational — the decorative hairline, and the seam
// between a Mermaid node plate and the page, where the goal is a number rather
// than a floor. Only enforced pairs can fail the run.
//
// This exists because pa11y is not part of `pnpm run lint` and CI sets
// PUPPETEER_SKIP_DOWNLOAD=true, so nothing else in the pipeline would notice an
// accent that stopped clearing AA. The sibling site learned this the hard
// way: its original teal rendered a body link at 3.74:1 for a year without a
// gate saying so. This palette starts gated.
//
// Run: node scripts/check-contrast.mjs [--json]
import { readFileSync } from "node:fs";

const css = readFileSync(
	new URL("../src/styles/theme.css", import.meta.url),
	"utf8",
);

/** All `--token: #hex;` declarations of the rule whose selector list is exactly `selector`. */
function block(selector) {
	const out = {};
	const re = /([^{}]+)\{([^{}]*)\}/g;
	let m;
	const want = selector.replace(/\s+/g, " ").trim();
	while ((m = re.exec(css))) {
		const found = m[1]
			.replace(/\/\*[\s\S]*?\*\//g, "")
			.replace(/\s+/g, " ")
			.trim();
		if (found !== want) continue;
		for (const [, name, value] of m[2].matchAll(
			/(--[\w-]+)\s*:\s*(#[0-9a-fA-F]{6}|var\(--[\w-]+\))\s*;/g,
		)) {
			out[name] = value.toLowerCase();
		}
	}
	return out;
}

/** Resolve `var(--other)` aliases to their hex, then drop what never resolved. */
function resolve(palette) {
	for (let pass = 0; pass < 5; pass++) {
		for (const [name, value] of Object.entries(palette)) {
			const alias = /^var\((--[\w-]+)\)$/.exec(value);
			if (alias && palette[alias[1]]) palette[name] = palette[alias[1]];
		}
	}
	for (const [name, value] of Object.entries(palette)) {
		if (!value.startsWith("#")) delete palette[name];
	}
	return palette;
}

// The dark theme is the root scope. theme.css writes the palette on Starlight's
// own `:root, ::backdrop` pair — the search dialog paints its backdrop from
// these variables and a `:root`-only rule never reaches it — and adds two plain
// `:root` blocks afterwards for the aside hues and the shared identity tokens,
// so both selector forms are collected. The light theme overrides the result,
// which is why it is resolved on top of the dark one rather than on its own.
const dark = { ...block(":root, ::backdrop"), ...block(":root") };

/** The `--sl-hue-*: <integer>;` declarations, wherever in the sheet they sit. */
function hues() {
	const out = {};
	for (const [, name, value] of css.matchAll(
		/(--sl-hue-[\w-]+)\s*:\s*(\d+)\s*;/g,
	)) {
		out[name] = Number(value);
	}
	return out;
}

// Starlight builds each callout family from one hue with fixed saturation and
// lightness, so the rendered colours exist nowhere in this file — only the hue
// does. They are reconstructed here from Starlight's own formulas (style/
// props.css) rather than hard-coded, so retuning a hue re-measures the family
// instead of silently keeping an old verdict. The four families that deviate
// from the common formula are spelled out.
const HSL = (h, s, l) => {
	const a = (s / 100) * Math.min(l / 100, 1 - l / 100);
	const f = (n) => {
		const k = (n + h / 30) % 12;
		return l / 100 - a * Math.max(-1, Math.min(k - 3, 9 - k, 1));
	};
	const hex = (v) =>
		Math.round(v * 255)
			.toString(16)
			.padStart(2, "0");
	return `#${hex(f(0))}${hex(f(8))}${hex(f(4))}`;
};

// [family, aside class, hue token, dark high S/L, light high S/L]
const ASIDES = [
	["note", "note", "--sl-hue-blue", [100, 87], [80, 30]],
	["tip", "tip", "--sl-hue-purple", [82, 89], [90, 30]],
	["success", "success", "--sl-hue-green", [82, 80], [80, 22]],
	["caution", "caution", "--sl-hue-orange", [82, 87], [80, 25]],
	["danger", "danger", "--sl-hue-red", [82, 87], [80, 30]],
];
const HUES = hues();
const light = block(
	':root[data-theme="light"], [data-theme="light"] ::backdrop',
);
const PALETTES = {
	"circuit-violet / dark": resolve({ ...dark }),
	"circuit-violet / light": resolve({ ...dark, ...light }),
};

for (const [name, p] of Object.entries(PALETTES)) {
	if (!p["--sl-color-black"])
		throw new Error(`palette "${name}" did not parse`);
}

// Both themes have to be written, not half-inherited. The light block sits on
// top of the dark one in the cascade, and this sheet is deliberately unlayered,
// so a colour token the light theme forgets keeps its DARK value everywhere and
// beats Starlight's own light-theme default — which is how a plain
// `--sl-color-hairline` paints a near-black rule on a white page. The converse
// is allowed: the light theme may add tokens of its own, such as gray-7.
const resolvedLight = resolve({ ...light });
const inheritedFromDark = Object.keys(resolve({ ...dark })).filter(
	(token) => !(token in resolvedLight),
);

const srgb = (hex) => {
	const n = parseInt(hex.slice(1), 16);
	return [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
		const s = c / 255;
		return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
	});
};
const luminance = (hex) => {
	const [r, g, b] = srgb(hex);
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const ratio = (a, b) => {
	const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (l1 + 0.05) / (l2 + 0.05);
};

const rows = [];
let failures = 0;

const check = (palette, label, fg, bg, min) => {
	if (!fg || !bg)
		throw new Error(`missing colour for "${label}" in ${palette}`);
	const r = ratio(fg, bg);
	const enforced = min > 0;
	const pass = !enforced || r >= min;
	if (!pass) failures++;
	rows.push({
		palette,
		label,
		fg,
		bg,
		ratio: Math.round(r * 100) / 100,
		min,
		enforced,
		pass,
	});
};

for (const [name, p] of Object.entries(PALETTES)) {
	const isLight = name.endsWith("light");
	const bg = p["--sl-color-black"]; // --sl-color-bg resolves to this
	// Starlight paints a link with --sl-color-text-accent, which is the accent
	// in light and the high step in dark.
	const textAccent = isLight
		? p["--sl-color-accent"]
		: p["--sl-color-accent-high"];

	check(name, "body text (gray-2) on page", p["--sl-color-gray-2"], bg, 4.5);
	check(name, "muted text (gray-3) on page", p["--sl-color-gray-3"], bg, 4.5);
	check(name, "headings on page", p["--sl-color-white"], bg, 4.5);
	check(
		name,
		"body text on nav / sidebar",
		p["--sl-color-gray-2"],
		p["--sl-color-bg-nav"],
		4.5,
	);
	check(name, "link colour on page", textAccent, bg, 4.5);
	check(
		name,
		"link colour on card surface",
		textAccent,
		p["--gm-surface"],
		4.5,
	);
	check(
		name,
		"muted text on card surface",
		p["--sl-color-gray-3"],
		p["--gm-surface"],
		4.5,
	);
	// Primary button: --sl-color-bg-accent ground with --sl-color-text-invert label.
	check(
		name,
		"primary button label",
		isLight ? p["--sl-color-black"] : p["--sl-color-accent-low"],
		isLight ? p["--sl-color-accent"] : p["--sl-color-accent-high"],
		4.5,
	);
	check(name, "focus ring / accent mark on page", p["--gm-mark"], bg, 3);

	// Inline code is body text on a tinted plate, so it carries the same 4.5:1
	// floor in both themes: an exemption on one would let a regression through.
	check(
		name,
		"inline code text on its ground",
		p["--sl-color-gray-2"],
		p["--sl-color-bg-inline-code"],
		4.5,
	);

	// Page-title chips. The text is enforced at 4.5:1 like any other text,
	// and the border at 3:1 — with only an outline to go on, the border is
	// what carries the chip's state, which makes it meaningful non-text
	// under WCAG 1.4.11 rather than decoration.
	check(name, "plain chip text on page", p["--gm-chip-ink"], bg, 4.5);
	check(name, "plain chip border on page", p["--gm-chip-line"], bg, 3);
	// Edition (tier) chips: the ink sits on the chip's own fill, the border
	// on the page around it.
	check(
		name,
		"tier chip text on its fill",
		p["--gm-tier-ink"],
		p["--gm-tier-fill"],
		4.5,
	);
	check(name, "tier chip border on page", p["--gm-tier-line"], bg, 3);

	// The three parts of the brand mark, against the nav ground the header
	// mark actually sits on (WCAG 1.4.11 non-text, 3:1).
	const nav = p["--sl-color-bg-nav"];
	check(name, "mark source node on nav", p["--gm-mark-node"], nav, 3);
	check(name, "mark branches on nav", p["--gm-mark-branch"], nav, 3);
	check(name, "mark terminals on nav", p["--gm-mark-tip"], nav, 3);

	check(name, "decorative hairline on page", p["--gm-border-strong"], bg, 0);

	// The Mermaid diagrams are painted from these same tokens (see the alias
	// block in custom.css). The label and cluster backings resolve to
	// --sl-color-bg, so their seam against the page is 1.00 by construction;
	// what is worth watching is the node plate, which is meant to read as a
	// raised surface without becoming a box. Informational: "just visible" is
	// not a threshold.
	// Asides. theme.css flattens their ground to the panel surface and gives
	// the rule the title's ink, so both are measured against that surface: the
	// title as text at 4.5:1, the rule as meaningful non-text at 3:1 (WCAG
	// 1.4.11 — with the wash gone, the rule's colour is the only thing that
	// says which kind of callout this is). Body copy inside one is
	// --sl-color-white, checked once.
	const asideGround = p["--gm-surface"];
	check(
		name,
		"aside body text on its ground",
		p["--sl-color-white"],
		asideGround,
		4.5,
	);
	for (const [family, , hueToken, darkHigh, lightHigh] of ASIDES) {
		const hue = HUES[hueToken];
		if (hue === undefined) throw new Error(`theme.css declares no ${hueToken}`);
		const [sat, light] = isLight ? lightHigh : darkHigh;
		const ink = HSL(hue, sat, light);
		check(name, `${family} aside title on its ground`, ink, asideGround, 4.5);
		check(name, `${family} aside rule on its ground`, ink, asideGround, 3);
	}

	check(name, "Mermaid node plate against page", p["--gm-surface"], bg, 0);
}

if (inheritedFromDark.length) failures += inheritedFromDark.length;

if (process.argv.includes("--json")) {
	console.log(JSON.stringify({ rows, inheritedFromDark }, null, 2));
} else {
	let current = "";
	for (const r of rows) {
		if (r.palette !== current) {
			current = r.palette;
			console.log(`\n### ${current}`);
		}
		const flag = r.enforced ? (r.pass ? "ok  " : "FAIL") : "info";
		const min = r.enforced ? `min ${r.min}` : "decorative";
		console.log(
			`  ${flag} ${r.ratio.toFixed(2).padStart(6)}:1  (${min})  ${r.label}  ${r.fg} on ${r.bg}`,
		);
	}
	console.log("\n### both themes are written, not half-inherited");
	if (inheritedFromDark.length) {
		for (const token of inheritedFromDark) {
			console.log(
				`  FAIL ${token} has no light-theme value, so it keeps the dark one`,
			);
		}
	} else {
		console.log(
			"  ok   every colour token of the dark theme has a light-theme value",
		);
	}
	console.log(
		`\n${rows.length} pairs measured, ${failures} enforced pair(s) below threshold` +
			` or token(s) missing from the light theme.`,
	);
}

process.exit(failures ? 1 : 0);
