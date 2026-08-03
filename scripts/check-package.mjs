import { existsSync, readFileSync } from "node:fs";

for (const path of ["bin/wowdoc.mjs", "scripts/install.mjs", "skill/SKILL.md"]) {
  if (!existsSync(new URL(`../${path}`, import.meta.url))) throw new Error(`package file missing: ${path}`);
}

const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
if (JSON.stringify(pkg.bin) !== JSON.stringify({ wowdoc: "bin/wowdoc.mjs" })) {
  throw new Error("package must expose only the wowdoc executable");
}
