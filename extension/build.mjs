// Builds the browser extension into dist/chrome and dist/firefox.
// Usage: node extension/build.mjs [outputDir]
import { cpSync, mkdirSync, rmSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const out = process.argv[2] ? process.argv[2] : join(here, "dist");
const version = JSON.parse(readFileSync(join(here, "..", "wails.json"), "utf8")).info.productVersion;

for (const target of ["chrome", "firefox"]) {
  const dir = join(out, target);
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });
  cpSync(join(here, "src"), dir, { recursive: true });
  const manifest = JSON.parse(readFileSync(join(here, `manifest.${target}.json`), "utf8"));
  manifest.version = version;
  writeFileSync(join(dir, "manifest.json"), JSON.stringify(manifest, null, 2));
  console.log(`built ${target} extension -> ${dir}`);
}
