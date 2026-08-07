import { chmodSync, cpSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const dist = join(root, "dist");
const version = JSON.parse(readFileSync(join(root, "package.json"), "utf8")).version;
const targets = [
  ["linux-amd64", "wowdoc-linux-x64", "wowdoc"],
  ["linux-arm64", "wowdoc-linux-arm64", "wowdoc"],
  ["windows-amd64", "wowdoc-win32-x64", "wowdoc.exe"],
  ["darwin-amd64", "wowdoc-darwin-x64", "wowdoc"],
  ["darwin-arm64", "wowdoc-darwin-arm64", "wowdoc"],
];
const requested = process.env.WOWDOC_PLATFORM_PACKAGE;
const selected = requested ? targets.filter(([, packageName]) => packageName === requested) : targets;
if (selected.length === 0) throw new Error(`unknown platform package: ${requested}`);
for (const [asset, packageName, binary] of selected) {
  const source = join(dist, `wowdoc-${asset}${binary.endsWith(".exe") ? ".exe" : ""}`);
  if (!existsSync(source)) throw new Error(`missing platform binary: ${source}`);
  const dir = join(root, "packages", packageName, "bin");
  mkdirSync(dir, { recursive: true });
  rmSync(join(dir, binary), { force: true });
  cpSync(source, join(dir, binary));
  if (!binary.endsWith(".exe")) chmodSync(join(dir, binary), 0o755);
  cpSync(join(root, "LICENSE"), join(root, "packages", packageName, "LICENSE"));
  const packageJSON = join(root, "packages", packageName, "package.json");
  const data = JSON.parse(readFileSync(packageJSON, "utf8"));
  data.version = version;
  writeFileSync(packageJSON, `${JSON.stringify(data, null, 2)}\n`);
}
