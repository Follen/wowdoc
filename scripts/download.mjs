import { createHash } from "node:crypto";
import { closeSync, createReadStream, createWriteStream, existsSync, mkdirSync, openSync, readFileSync, renameSync, rmSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { platformKey, releaseAsset } from "./platform.mjs";

const DEFAULT_ATTEMPTS = 5;
const DEFAULT_CONNECT_MS = 15 * 1000;
const DEFAULT_ATTEMPT_MS = 10 * 60 * 1000;
const DEFAULT_IDLE_MS = 45 * 1000;
const DEFAULT_TOTAL_MS = 45 * 60 * 1000;
const MAX_BINARY_BYTES = 128 * 1024 * 1024;
const MAX_CHECKSUM_BYTES = 1024 * 1024;

export async function downloadRelease({ version, stderr = process.stderr, fetchImpl = fetch, attempts = DEFAULT_ATTEMPTS, connectMs = DEFAULT_CONNECT_MS, attemptMs = DEFAULT_ATTEMPT_MS, idleMs = DEFAULT_IDLE_MS, totalMs = DEFAULT_TOTAL_MS, retryBaseMs = 1_000 } = {}) {
  const asset = releaseAsset(version);
  const cacheRoot = process.env.WOWDOC_CACHE_DIR || join(homedir(), ".cache", "wowdoc", "downloads");
  const dir = join(cacheRoot, version, platformKey());
  mkdirSync(dir, { recursive: true });
  const target = join(dir, asset);
  const part = `${target}.part`;
  const lock = `${target}.lock`;
  const checksumMarker = `${target}.sha256`;
  const started = Date.now();
  await acquireLock(lock, stderr, totalMs);
  try {
    if (existsSync(target)) {
      const localChecksum = readChecksumMarker(checksumMarker);
      if (localChecksum && await hashFile(target) === localChecksum) {
        progress(stderr, `using verified cache ${asset}`);
        return target;
      }
      if (await verifyFile(target, version, asset, fetchImpl, stderr, connectMs)) {
        writeChecksumMarker(checksumMarker, await hashFile(target));
        progress(stderr, `using verified cache ${asset}`);
        return target;
      }
    }
    const checksumBudget = remainingBudget(started, totalMs);
    const checksum = await getChecksum(version, asset, fetchImpl, stderr, attempts, connectMs, attemptMs, idleMs, checksumBudget);
    let lastError;
    for (let attempt = 1; attempt <= attempts; attempt++) {
      const remaining = totalMs - (Date.now() - started);
      if (remaining <= 0) throw new Error("download_total_timeout");
      try {
        await fetchToPart({ version, asset, part, checksum, fetchImpl, stderr, attempt, attempts, connectMs, timeoutMs: Math.min(attemptMs, remaining), idleMs });
        try { unlinkSync(target); } catch {}
        renameSync(part, target);
        writeChecksumMarker(checksumMarker, checksum);
        return target;
      } catch (error) {
        lastError = error;
        try { if (error.code === "checksum_mismatch" || error.code === "content_invalid") unlinkSync(part); } catch {}
        if (!retryable(error) || attempt === attempts) break;
        const waitMs = error.retryAfter ?? Math.min(30_000, retryBaseMs * 2 ** (attempt - 1)) + Math.floor(Math.random() * Math.min(250, retryBaseMs));
        progress(stderr, `retry ${attempt}/${attempts - 1} in ${waitMs}ms: ${safeMessage(error)}`);
        await sleepWithinBudget(waitMs, started, totalMs);
      }
    }
    throw lastError || new Error("download_failed");
  } finally {
    try { rmSync(lock, { recursive: true, force: true }); } catch {}
  }
}

async function fetchToPart({ version, asset, part, checksum, fetchImpl, stderr, attempt, attempts, connectMs, timeoutMs, idleMs }) {
  const url = releaseURL(version, asset);
  const attemptStarted = Date.now();
  let existing = existsSync(part) ? statSync(part).size : 0;
  const headers = existing > 0 ? { Range: `bytes=${existing}-`, "Accept-Encoding": "identity" } : { "Accept-Encoding": "identity" };
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(new Error("download_attempt_timeout")), timeoutMs);
  let connectTimer = setTimeout(() => controller.abort(new Error("download_connection_timeout")), Math.min(connectMs, timeoutMs));
  let idleTimer;
  let output;
  try {
    const response = await fetchImpl(url, { redirect: "follow", headers, signal: controller.signal });
    clearTimeout(connectTimer);
    connectTimer = null;
    assertReleaseURL(response.url);
    if (response.status === 416 && existing > 0) {
      if (await hashFile(part) === checksum) return;
      unlinkSync(part);
      existing = 0;
      const remaining = timeoutMs - (Date.now() - attemptStarted);
      if (remaining <= 0) throw new Error("download_attempt_timeout");
      return fetchToPart({ version, asset, part, checksum, fetchImpl, stderr, attempt, attempts, connectMs, timeoutMs: remaining, idleMs });
    }
    if (!response.ok) {
      const error = new Error(`http_${response.status}`);
      error.status = response.status;
      error.retryAfter = parseRetryAfter(response.headers.get("retry-after"));
      throw error;
    }
    const resumed = existing > 0 && response.status === 206 && contentRangeStartsAt(response.headers.get("content-range"), existing);
    if (!resumed) {
      existing = 0;
      const fd = openSync(part, "w");
      closeSync(fd);
    }
    const expected = expectedLength(response, existing);
    if (expected > MAX_BINARY_BYTES || existing > MAX_BINARY_BYTES) {
      const error = new Error("content_too_large");
      error.code = "content_too_large";
      throw error;
    }
    output = createWriteStream(part, { flags: resumed ? "a" : "w" });
    let received = existing;
    const transferStarted = Date.now();
    const transferOffset = existing;
    let lastReport = 0;
    const report = () => {
      const now = Date.now();
      if (now - lastReport < 500 && received !== expected) return;
      lastReport = now;
      const total = expected > 0 ? `${formatBytes(received)}/${formatBytes(expected)}` : formatBytes(received);
      const elapsedMs = Math.max(1, now - transferStarted);
      const speed = (received - transferOffset) * 1000 / elapsedMs;
      progress(stderr, `download ${asset} ${total} ${formatBytes(speed)}/s elapsed ${formatDuration(elapsedMs)} attempt ${attempt}/${attempts}`, true);
    };
    idleTimer = setInterval(() => controller.abort(new Error("download_idle_timeout")), idleMs);
    if (!response.body) throw new Error("download_empty_body");
    for await (const chunk of response.body) {
      clearInterval(idleTimer);
      idleTimer = setInterval(() => controller.abort(new Error("download_idle_timeout")), idleMs);
      if (!output.write(chunk)) await onceDrain(output);
      received += chunk.length;
      if (received > MAX_BINARY_BYTES) {
        const error = new Error("content_too_large");
        error.code = "content_too_large";
        throw error;
      }
      report();
    }
    await new Promise((resolve, reject) => { output.end(error => error ? reject(error) : resolve()); });
    if (expected > 0 && received !== expected) {
      const error = new Error(`content_length_mismatch: got ${received}, expected ${expected}`);
      error.code = "content_invalid";
      throw error;
    }
    const actual = await hashFile(part);
    if (actual !== checksum) {
      const error = new Error(`checksum_mismatch: got ${actual}`);
      error.code = "checksum_mismatch";
      throw error;
    }
    progress(stderr, `verified ${asset} sha256=${actual}`);
  } catch (error) {
    if (output && !output.closed) await closeWriteStream(output);
    if (controller.signal.aborted && controller.signal.reason) throw controller.signal.reason;
    throw error;
  } finally {
    clearTimeout(timer);
    if (connectTimer) clearTimeout(connectTimer);
    if (idleTimer) clearInterval(idleTimer);
  }
}

async function getChecksum(version, asset, fetchImpl, stderr, attempts, connectMs, attemptMs, idleMs, totalMs) {
  const checksums = await fetchTextWithRetry(releaseURL(version, "SHA256SUMS"), fetchImpl, stderr, attempts, connectMs, attemptMs, idleMs, totalMs);
  const row = checksums.split(/\r?\n/).find(line => {
    const match = line.trim().match(/^[a-fA-F0-9]{64}\s+[ *]?(.+)$/);
    return match && basename(match[1]) === asset;
  });
  if (!row) throw new Error(`checksum_missing: ${asset}`);
  const value = row.trim().split(/\s+/)[0].toLowerCase();
  if (!/^[a-f0-9]{64}$/.test(value)) throw new Error("checksum_invalid");
  return value;
}

async function fetchTextWithRetry(url, fetchImpl, stderr, attempts, connectMs, attemptMs, idleMs, totalMs) {
  let lastError;
  const started = Date.now();
  for (let attempt = 1; attempt <= attempts; attempt++) {
    const remaining = remainingBudget(started, totalMs);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(new Error("download_attempt_timeout")), Math.min(attemptMs, remaining));
    let connectTimer = setTimeout(() => controller.abort(new Error("download_connection_timeout")), Math.min(connectMs, attemptMs, remaining));
    try {
      const response = await fetchImpl(url, { redirect: "follow", signal: controller.signal });
      clearTimeout(connectTimer);
      connectTimer = null;
      assertReleaseURL(response.url);
      if (!response.ok) {
        const e = new Error(`http_${response.status}`);
        e.status = response.status;
        e.retryAfter = parseRetryAfter(response.headers.get("retry-after"));
        throw e;
      }
      const declared = Number(response.headers.get("content-length"));
      if (Number.isFinite(declared) && declared > MAX_CHECKSUM_BYTES) throw new Error("checksum_manifest_too_large");
      return await readTextBody(response, controller, idleMs, MAX_CHECKSUM_BYTES);
    } catch (error) {
      lastError = controller.signal.aborted && controller.signal.reason ? controller.signal.reason : error;
      if (!retryable(lastError) || attempt === attempts) throw lastError;
      const waitMs = lastError.retryAfter ?? Math.min(30_000, 1_000 * 2 ** (attempt - 1));
      progress(stderr, `checksum retry ${attempt}/${attempts - 1} in ${waitMs}ms: ${safeMessage(lastError)}`);
      await sleepWithinBudget(waitMs, started, totalMs);
    } finally {
      clearTimeout(timer);
      if (connectTimer) clearTimeout(connectTimer);
    }
  }
  throw lastError;
}

async function verifyFile(path, version, asset, fetchImpl, stderr, connectMs) {
  try {
    const checksum = await getChecksum(version, asset, fetchImpl, stderr, 2, connectMs, 30_000, 15_000, 60_000);
    return await hashFile(path) === checksum;
  } catch { return false; }
}

function releaseURL(version, asset) { return `https://github.com/Follen/wowdoc/releases/download/v${version}/${asset}`; }
function contentRangeStartsAt(value, offset) { return Boolean(value && new RegExp(`^bytes ${offset}-\\d+\\/\\d+$`).test(value)); }
function expectedLength(response, offset) { const length = Number(response.headers.get("content-length")); return Number.isFinite(length) && length >= 0 ? offset + length : 0; }
function retryable(error) { return error?.name === "AbortError" || error?.code === "ETIMEDOUT" || error?.code === "ECONNRESET" || error?.code === "checksum_mismatch" || error?.code === "content_invalid" || error?.status === 408 || error?.status === 429 || error?.status >= 500 || /timeout|reset|network|fetch failed|dns|tls/i.test(String(error?.message)); }
function safeMessage(error) { return String(error?.message || error).replace(/https?:\/\/\S+/g, "<url>"); }
const progressState = new WeakMap();
function progress(stderr, message, transient = false) {
  if (!stderr?.write) return;
  const line = `wowdoc: ${message}`;
  if (stderr.isTTY !== true) {
    stderr.write(`${line}\n`);
    return;
  }
  const state = progressState.get(stderr) || { active: false, width: 0, lastAt: 0 };
  const now = Date.now();
  if (transient) {
    if (state.active && now - state.lastAt < 250) return;
    stderr.write(`\r${line.padEnd(state.width)}`);
    state.active = true;
    state.width = Math.max(state.width, line.length);
    state.lastAt = now;
  } else {
    stderr.write(state.active ? `\r${line.padEnd(state.width)}\n` : `${line}\n`);
    state.active = false;
    state.width = 0;
    state.lastAt = now;
  }
  progressState.set(stderr, state);
}
function parseRetryAfter(value) {
  if (!value) return undefined;
  const seconds = Number(value);
  if (Number.isFinite(seconds) && seconds >= 0) return Math.min(30_000, seconds * 1000);
  const date = Date.parse(value);
  return Number.isFinite(date) ? Math.min(30_000, Math.max(0, date - Date.now())) : undefined;
}
function remainingBudget(started, totalMs) {
  const remaining = totalMs - (Date.now() - started);
  if (remaining <= 0) throw new Error("download_total_timeout");
  return remaining;
}
async function sleepWithinBudget(waitMs, started, totalMs) {
  if (waitMs >= remainingBudget(started, totalMs)) throw new Error("download_total_timeout");
  await sleep(waitMs);
}
async function readTextBody(response, controller, idleMs, maxBytes) {
  if (!response.body) throw new Error("download_empty_body");
  const chunks = [];
  let size = 0;
  let idleTimer;
  const resetIdle = () => {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = setTimeout(() => controller.abort(new Error("download_idle_timeout")), idleMs);
  };
  resetIdle();
  try {
    for await (const chunk of response.body) {
      resetIdle();
      size += chunk.length;
      if (size > maxBytes) throw new Error("checksum_manifest_too_large");
      chunks.push(chunk);
    }
    return Buffer.concat(chunks, size).toString("utf8");
  } finally {
    if (idleTimer) clearTimeout(idleTimer);
  }
}
function formatBytes(value) { if (value < 1024) return `${Math.round(value)} B`; if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KiB`; return `${(value / 1024 ** 2).toFixed(1)} MiB`; }
function formatDuration(value) { return value < 1000 ? `${value}ms` : `${(value / 1000).toFixed(1)}s`; }
function hashFile(path) { return new Promise((resolve, reject) => { const hash = createHash("sha256"); const stream = createReadStream(path); stream.on("data", chunk => hash.update(chunk)); stream.on("error", reject); stream.on("end", () => resolve(hash.digest("hex"))); }); }
function onceDrain(stream) { return new Promise(resolve => stream.once("drain", resolve)); }
function closeWriteStream(stream) { return new Promise(resolve => { stream.once("close", resolve); stream.destroy(); if (stream.closed) resolve(); }); }
function assertReleaseURL(value) {
  if (!value) return;
  const url = new URL(value);
  if (url.protocol !== "https:" || !(url.hostname === "github.com" || url.hostname.endsWith(".githubusercontent.com"))) {
    throw new Error("download_redirect_not_allowed");
  }
}
function readChecksumMarker(path) {
  try {
    const value = readFileSync(path, "utf8").trim().toLowerCase();
    return /^[a-f0-9]{64}$/.test(value) ? value : null;
  } catch { return null; }
}
function writeChecksumMarker(path, checksum) {
  const temporary = `${path}.tmp-${process.pid}`;
  writeFileSync(temporary, `${checksum}\n`, { mode: 0o600 });
  try { renameSync(temporary, path); }
  catch (error) {
    if (error.code !== "EEXIST" && error.code !== "EPERM") throw error;
    rmSync(path, { force: true });
    renameSync(temporary, path);
  } finally { rmSync(temporary, { force: true }); }
}

async function acquireLock(path, stderr, timeoutMs) {
  const started = Date.now();
  while (true) {
    try {
      mkdirSync(path);
      writeFileSync(join(path, "owner.json"), JSON.stringify({ pid: process.pid, startedAt: new Date().toISOString() }));
      return;
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
      try {
        if (Date.now() - statSync(path).mtimeMs > 60 * 60 * 1000) {
          rmSync(path, { recursive: true, force: true });
          progress(stderr, "removed stale installer lock");
          continue;
        }
      } catch {}
      if (Date.now() - started > timeoutMs) throw new Error("download_lock_timeout");
      progress(stderr, "waiting for another installer");
      await sleep(1_000);
    }
  }
}
