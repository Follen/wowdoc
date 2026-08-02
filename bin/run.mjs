import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export function run(name) {
  const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
  const suffix = process.platform === "win32" ? ".exe" : "";
  const executable = join(packageRoot, "native", name + suffix);
  if (!existsSync(executable)) {
    process.stderr.write(`${name}: native binary is missing; reinstall @follenfang/wowdoc\n`);
    process.exit(4);
  }
  const child = spawnSync(executable, process.argv.slice(2), { stdio: "inherit", windowsHide: true });
  if (child.error) {
    process.stderr.write(`${name}: ${child.error.message}\n`);
    process.exit(4);
  }
  process.exit(child.status ?? 1);
}
