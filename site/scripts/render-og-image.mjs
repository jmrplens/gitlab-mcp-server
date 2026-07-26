// Renders the social card from its SVG source to the PNG the site serves.
//
//   src/assets/social-preview.svg  ->  public/og-image.png        (1200x630, served)
//                                  ->  src/assets/social-preview.png (native, for registries)
//
// The card states measured token figures, so it has to be regenerated whenever
// `make gen-footprint` changes them — otherwise the image contradicts the page
// it is previewing. Run: `pnpm run og-image`.
//
// Rendering happens at 2x through the same headless Chromium that rehype-mermaid
// already requires, then downsamples with lanczos3: text edges stay crisp, and
// a 64-colour palette with light dithering lands the file near 55 KB, which keeps scrapers
// that cap preview downloads happy.
import { readFileSync, statSync } from "node:fs";

import { chromium } from "playwright";
import sharp from "sharp";

const SOURCE = "src/assets/social-preview.svg";
const SERVED = "public/og-image.png";
const NATIVE = "src/assets/social-preview.png";

// Open Graph's recommended size. Declared in astro.config.mjs (og:image:width /
// og:image:height and the ImageObject nodes), so the two must stay in step.
const OG_WIDTH = 1200;
const OG_HEIGHT = 630;
const SCALE = 2;

const raw = readFileSync(SOURCE, "utf8");

/** Forces the SVG to fill a given box exactly, dropping its intrinsic sizing. */
function sized(svg, width, height, preserve) {
	return svg.replace(/<svg([^>]*)>/, (_match, attrs) => {
		const cleaned = attrs.replace(
			/\s(width|height|preserveAspectRatio)="[^"]*"/g,
			"",
		);
		return `<svg${cleaned} width="${width}" height="${height}" preserveAspectRatio="${preserve}">`;
	});
}

async function shoot(browser, svg, width, height) {
	const page = await browser.newPage({
		viewport: { width, height },
		deviceScaleFactor: SCALE,
	});
	await page.setContent(
		`<style>html,body{margin:0;padding:0;overflow:hidden}svg{display:block}</style>${svg}`,
	);
	// Let the gradients and filters settle before capturing.
	await page.waitForTimeout(300);
	const buffer = await page.screenshot({ type: "png" });
	await page.close();
	return buffer;
}

const browser = await chromium.launch();
try {
	// The source is 1280x640 (2:1) and the served card is 1200x630, so it is
	// scaled rather than cropped — the design is centred with wide margins, and
	// the 5% difference in aspect is not perceptible.
	const served = await shoot(
		browser,
		sized(raw, OG_WIDTH, OG_HEIGHT, "none"),
		OG_WIDTH,
		OG_HEIGHT,
	);
	await sharp(served)
		.resize(OG_WIDTH, OG_HEIGHT, { kernel: "lanczos3" })
		.png({
			palette: true,
			colors: 64,
			dither: 0.5,
			effort: 10,
			compressionLevel: 9,
		})
		.toFile(SERVED);

	const viewBox = /viewBox="0 0 (\d+) (\d+)"/.exec(raw);
	const nativeWidth = viewBox ? Number(viewBox[1]) : OG_WIDTH;
	const nativeHeight = viewBox ? Number(viewBox[2]) : OG_HEIGHT;
	const native = await shoot(
		browser,
		sized(raw, nativeWidth, nativeHeight, "xMidYMid meet"),
		nativeWidth,
		nativeHeight,
	);
	await sharp(native)
		.resize(nativeWidth, nativeHeight, { kernel: "lanczos3" })
		.png({
			palette: true,
			colors: 64,
			dither: 0.5,
			effort: 10,
			compressionLevel: 9,
		})
		.toFile(NATIVE);
} finally {
	await browser.close();
}

for (const file of [SERVED, NATIVE]) {
	console.log(
		`[render-og-image] ${file} — ${(statSync(file).size / 1024).toFixed(1)} KB`,
	);
}
