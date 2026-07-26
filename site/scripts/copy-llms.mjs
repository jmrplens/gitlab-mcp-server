// Publishes the canonical root-level llms*.txt files to the site so the deployed
// domain serves them at /llms.txt, /llms-medium.txt and friends (GEO: AI engines
// fetch these for structured context). Run as a `prebuild` step so the published
// copy is always regenerated from the single source of truth — never committed,
// never drifting. The root files are generated/validated by `cmd/gen_llms`.
//
// llms.txt is the index; llms-medium.txt is the one that fits in a context
// window; llms-full.txt and the three per-surface splits are for retrieval and
// for consumers that want only one surface.
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const publicDir = join(here, "..", "public");

mkdirSync(publicDir, { recursive: true });

const FILES = [
	"llms.txt",
	"llms-medium.txt",
	"llms-full.txt",
	"llms-full-meta-tools.txt",
	"llms-full-individual-tools.txt",
	"llms-full-resources-prompts.txt",
];

for (const name of FILES) {
	const src = join(repoRoot, name);
	if (!existsSync(src)) {
		console.warn(`[copy-llms] ${name} not found at repo root — skipping`);
		continue;
	}
	copyFileSync(src, join(publicDir, name));
	console.log(`[copy-llms] published ${name} -> site/public/${name}`);
}
