// Publishes the canonical root-level llms.txt / llms-full.txt to the site so the
// deployed domain serves them at /llms.txt and /llms-full.txt (GEO: AI engines
// fetch these for structured context). Run as a `prebuild` step so the published
// copy is always regenerated from the single source of truth — never committed,
// never drifting. The root files are generated/validated by `cmd/gen_llms`.
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const publicDir = join(here, "..", "public");

mkdirSync(publicDir, { recursive: true });

for (const name of ["llms.txt", "llms-full.txt"]) {
	const src = join(repoRoot, name);
	if (!existsSync(src)) {
		console.warn(`[copy-llms] ${name} not found at repo root — skipping`);
		continue;
	}
	copyFileSync(src, join(publicDir, name));
	console.log(`[copy-llms] published ${name} -> site/public/${name}`);
}
