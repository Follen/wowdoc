import { existsSync } from "node:fs";
for (const path of ["bin/wowdoc.mjs", "bin/wowdata.mjs", "scripts/install.mjs", "skill/SKILL.md"]) {
  if (!existsSync(new URL(`../${path}`, import.meta.url))) throw new Error(`package file missing: ${path}`);
}
