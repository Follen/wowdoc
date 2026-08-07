import { createRequire } from "node:module";
import { dirname, join } from "node:path";

const require = createRequire(import.meta.url);

export function platformKey() {
  const key = `${process.platform}-${process.arch}`;
  const supported = new Set(["linux-x64", "linux-arm64", "win32-x64", "darwin-x64", "darwin-arm64"]);
  if (!supported.has(key)) throw new Error(`unsupported_platform: ${key}`);
  return key;
}

export function binaryName() {
  return process.platform === "win32" ? "wowdoc.exe" : "wowdoc";
}

export function releaseAsset(version) {
  const names = {
    "linux-x64": "linux-amd64",
    "linux-arm64": "linux-arm64",
    "win32-x64": "windows-amd64",
    "darwin-x64": "darwin-amd64",
    "darwin-arm64": "darwin-arm64",
  };
  return `wowdoc-${names[platformKey()]}${process.platform === "win32" ? ".exe" : ""}`;
}

export function optionalPackageName() {
  return `@follenfang/wowdoc-${platformKey()}`;
}

export function optionalBinary(root) {
  try {
    const packageJSON = require.resolve(`${optionalPackageName()}/package.json`, { paths: [root] });
    return join(dirname(packageJSON), "bin", binaryName());
  } catch {
    return null;
  }
}
