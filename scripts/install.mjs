import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { chmodSync, cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
const suffix = process.platform === "win32" ? ".exe" : "";
const platformNames = {
  "linux-x64": "linux-amd64",
  "linux-arm64": "linux-arm64",
  "win32-x64": "windows-amd64",
  "darwin-x64": "darwin-amd64",
  "darwin-arm64": "darwin-arm64",
};
const platform = platformNames[`${process.platform}-${process.arch}`];
if (!platform) throw new Error(`unsupported_platform: ${process.platform}-${process.arch}`);
const nativeDir = join(root, "native");
mkdirSync(nativeDir, { recursive: true });

for (const name of ["wowdoc", "wowdata"]) {
  const target = join(nativeDir, name + suffix);
  const supplied = process.env.WOWDOC_BINARY_DIR && join(process.env.WOWDOC_BINARY_DIR, name + suffix);
  if (supplied && existsSync(supplied)) {
    cpSync(supplied, target);
  } else if (existsSync(join(root, "go.mod"))) {
    execFileSync("go", ["build", "-trimpath", "-ldflags", `-s -w -X github.com/follenfang/wowdoc/internal/app.Version=${pkg.version}`, "-o", target, `./cmd/${name}`], { cwd: root, stdio: "inherit" });
  } else {
    const asset = `${name}-${platform}${suffix}`;
    const url = `https://github.com/Follen/wowdoc/releases/download/v${pkg.version}/${asset}`;
    const response = await fetch(url, { redirect: "follow" });
    if (!response.ok) throw new Error(`binary_download_failed: ${response.status} ${url}`);
    writeFileSync(target, Buffer.from(await response.arrayBuffer()));
  }
  if (process.platform !== "win32") chmodSync(target, 0o755);
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
function writeManifest(target, sourceFiles) {
  const mapped = {};
  for (const source of sourceFiles) mapped[relative(skillSource, source).replaceAll("\\", "/")] = hash(readFileSync(source));
  writeFileSync(join(target, ".wowdoc-manifest.json"), JSON.stringify({ package: pkg.name, version: pkg.version, files: mapped }, null, 2));
}
