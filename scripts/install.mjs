import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { chmodSync, cpSync, existsSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { downloadRelease } from "./download.mjs";
import { optionalBinary, platformKey } from "./platform.mjs";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
const suffix = process.platform === "win32" ? ".exe" : "";
platformKey();
const nativeDir = join(root, "native");
mkdirSync(nativeDir, { recursive: true });

const name = "wowdoc";
const target = join(nativeDir, name + suffix);
const supplied = process.env.WOWDOC_BINARY_DIR && join(process.env.WOWDOC_BINARY_DIR, name + suffix);
const packaged = optionalBinary(root);
if (supplied && existsSync(supplied)) {
  process.stderr.write(`wowdoc: using supplied ${platformKey()} binary\n`);
  installBinary(supplied, target);
} else if (existsSync(join(root, "go.mod"))) {
  process.stderr.write(`wowdoc: building ${platformKey()} binary from source\n`);
  const built = `${target}.build-${process.pid}`;
  execFileSync("go", ["build", "-trimpath", "-ldflags", `-s -w -X github.com/follenfang/wowdoc/internal/app.Version=${pkg.version}`, "-o", built, `./cmd/${name}`], { cwd: root, stdio: "inherit" });
  installBinary(built, target);
  rmSync(built, { force: true });
} else if (packaged && existsSync(packaged)) {
  process.stderr.write(`wowdoc: using verified ${platformKey()} platform package\n`);
  installBinary(packaged, target);
} else {
  process.stderr.write(`wowdoc: platform package unavailable; downloading verified GitHub Release fallback\n`);
  try {
    const downloaded = await downloadRelease({ version: pkg.version, root, stderr: process.stderr });
    installBinary(downloaded, target);
  } catch (error) {
    process.stderr.write(`wowdoc: binary download failed after bounded retries: ${error.message}\n`);
    process.stderr.write(`wowdoc: retry npm install with --foreground-scripts --verbose after checking npm and GitHub connectivity\n`);
    throw error;
  }
}

const skillSource = join(root, "skill");
const skillTarget = join(homedir(), ".agents", "skills", "wowdoc");
const manifestPath = join(skillTarget, ".wowdoc-manifest.json");
const incoming = files(skillSource);
let modified = false;
if (existsSync(manifestPath)) {
  const previous = JSON.parse(readFileSync(manifestPath, "utf8"));
  modified = Object.entries(previous.files ?? {}).some(([path, expected]) => {
    const current = join(skillTarget, path);
    return !existsSync(current) || hash(readFileSync(current)) !== expected;
  });
}
if (modified) {
  const sideBySide = `${skillTarget}.update-${pkg.version}`;
  rmSync(sideBySide, { recursive: true, force: true });
  cpSync(skillSource, sideBySide, { recursive: true });
  writeManifest(sideBySide, incoming);
  process.stderr.write(`wowdoc: existing Skill was modified; update installed at ${sideBySide}\n`);
} else {
  rmSync(skillTarget, { recursive: true, force: true });
  cpSync(skillSource, skillTarget, { recursive: true });
  writeManifest(skillTarget, incoming);
}

function files(directory) {
  const output = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) output.push(...files(path));
    else output.push(path);
  }
  return output;
}
function hash(buffer) { return createHash("sha256").update(buffer).digest("hex"); }
function installBinary(source, destination) {
  const temporary = `${destination}.tmp-${process.pid}`;
  rmSync(temporary, { force: true });
  try {
    cpSync(source, temporary);
    if (process.platform !== "win32") chmodSync(temporary, 0o755);
    try { renameSync(temporary, destination); }
    catch (error) {
      if (error.code !== "EEXIST" && error.code !== "EPERM") throw error;
      rmSync(destination, { force: true });
      renameSync(temporary, destination);
    }
  } finally {
    rmSync(temporary, { force: true });
  }
}
function writeManifest(target, sourceFiles) {
  const mapped = {};
  for (const source of sourceFiles) mapped[relative(skillSource, source).replaceAll("\\", "/")] = hash(readFileSync(source));
  writeFileSync(join(target, ".wowdoc-manifest.json"), JSON.stringify({ package: pkg.name, version: pkg.version, files: mapped }, null, 2));
}
