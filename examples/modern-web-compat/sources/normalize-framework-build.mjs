import { readdir, readFile, writeFile } from "node:fs/promises";
import { extname, join } from "node:path";

const root = process.argv[2];
if (!root) {
  throw new Error("usage: node sources/normalize-framework-build.mjs <build-directory>");
}

const replacements = new Map([
  ["\u180e", "\\u180e"],
  ["\u200b", "\\u200b"],
]);

async function normalize(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      await normalize(path);
      continue;
    }
    if (!entry.isFile() || ![".html", ".js", ".mjs"].includes(extname(entry.name))) {
      continue;
    }
    const original = await readFile(path, "utf8");
    let normalized = original;
    for (const [character, escape] of replacements) {
      normalized = normalized.replaceAll(character, escape);
    }
    normalized = normalized.replace(/[ \t]+$/gm, "");
    if (normalized !== original) {
      await writeFile(path, normalized, "utf8");
    }
  }
}

await normalize(root);
