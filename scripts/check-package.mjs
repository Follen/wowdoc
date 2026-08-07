import { existsSync, readFileSync } from "node:fs";

for (const path of ["bin/wowdoc.mjs", "scripts/install.mjs", "scripts/download.mjs", "scripts/platform.mjs", "skill/SKILL.md"]) {
  if (!existsSync(new URL(`../${path}`, import.meta.url))) throw new Error(`package file missing: ${path}`);
}

const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
if (JSON.stringify(pkg.bin) !== JSON.stringify({ wowdoc: "bin/wowdoc.mjs" })) {
  throw new Error("package must expose only the wowdoc executable");
}
const expectedPlatforms = [
  "@follenfang/wowdoc-darwin-arm64",
  "@follenfang/wowdoc-darwin-x64",
  "@follenfang/wowdoc-linux-arm64",
  "@follenfang/wowdoc-linux-x64",
  "@follenfang/wowdoc-win32-x64",
].sort();
if (JSON.stringify(Object.keys(pkg.optionalDependencies ?? {}).sort()) !== JSON.stringify(expectedPlatforms)) {
  throw new Error("platform optional dependencies are incomplete");
}
for (const name of expectedPlatforms) {
  if (pkg.optionalDependencies[name] !== pkg.version) throw new Error(`platform dependency version mismatch: ${name}`);
  const directory = name.replace("@follenfang/", "");
  const platform = JSON.parse(readFileSync(new URL(`../packages/${directory}/package.json`, import.meta.url), "utf8"));
  if (platform.name !== name || platform.version !== pkg.version || platform.files?.[0] !== "bin/") {
    throw new Error(`invalid platform package manifest: ${name}`);
  }
}
